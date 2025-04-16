package gitlab

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
	"github.com/tidwall/gjson"

	"github.com/beeper/bridge-manager/cli/hyper"
	"github.com/beeper/bridge-manager/log"
)

// language=graphql
const getLastSuccessfulJobQuery = `
query($repo: ID!, $ref: String!, $job: String!) {
  project(fullPath: $repo) {
    pipelines(status: SUCCESS, ref: $ref, first: 1) {
      nodes {
        sha
        job(name: $job) {
          webPath
        }
      }
    }
  }
}
`

type lastSuccessfulJobQueryVariables struct {
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
	Job  string `json:"job"`
}

type LastBuild struct {
	Commit string
	JobURL string
}

func GetLastBuild(domain, repo, mainBranch, job string) (*LastBuild, error) {
	resp, err := graphqlQuery(domain, getLastSuccessfulJobQuery, lastSuccessfulJobQueryVariables{
		Repo: repo,
		Ref:  mainBranch,
		Job:  job,
	})
	if err != nil {
		return nil, err
	}
	res := gjson.GetBytes(resp, "project.pipelines.nodes.0")
	if !res.Exists() {
		return nil, fmt.Errorf("didn't get pipeline info in response")
	}
	return &LastBuild{
		Commit: gjson.Get(res.Raw, "sha").Str,
		JobURL: gjson.Get(res.Raw, "job.webPath").Str,
	}, nil
}

func getRefFromBridge(bridge string) (string, error) {
	switch bridge {
	case "imessage":
		return "master", nil
	case "whatsapp", "discord", "slack", "gmessages", "gvoice", "signal", "imessagego", "meta", "twitter", "bluesky", "linkedin":
		return "main", nil
	default:
		return "", fmt.Errorf("unknown bridge %s", bridge)
	}
}

var ErrNotBuiltInCI = errors.New("not built in the CI")

func getJobFromBridge(bridge string) (string, error) {
	osAndArch := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	switch osAndArch {
	case "linux/amd64":
		return "build amd64", nil
	case "linux/arm64":
		return "build arm64", nil
	case "linux/arm":
		if bridge == "signal" {
			return "", fmt.Errorf("mautrix-signal binaries for 32-bit arm are %w", ErrNotBuiltInCI)
		}
		return "build arm", nil
	case "darwin/arm64":
		if bridge == "imessage" {
			return "build universal", nil
		}
		return "build macos arm64", nil
	default:
		if bridge == "imessage" {
			return "build universal", nil
		}
		return "", fmt.Errorf("binaries for %s are %w", osAndArch, ErrNotBuiltInCI)
	}
}

func linkifyCommit(repo, commit string) string {
	return hyper.Link(commit[:8], fmt.Sprintf("https://github.com/%s/commit/%s", repo, commit), false)
}

func linkifyDiff(repo, fromCommit, toCommit string) string {
	formattedDiff := fmt.Sprintf("%s...%s", fromCommit[:8], toCommit[:8])
	return hyper.Link(formattedDiff, fmt.Sprintf("https://github.com/%s/compare/%s...%s", repo, fromCommit, toCommit), false)
}

func makeArtifactURL(domain, jobURL, fileName string) string {
	return (&url.URL{
		Scheme: "https",
		Host:   domain,
		Path:   filepath.Join(jobURL, "artifacts", "raw", fileName),
	}).String()
}

func downloadFile(ctx context.Context, artifactURL, path string) error {
	fileName := filepath.Base(path)
	file, err := os.CreateTemp(filepath.Dir(path), "tmp-"+fileName+"-*")
	if err != nil {
		return fmt.Errorf("failed to open temp file: %w", err)
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return fmt.Errorf("failed to prepare download request: %w", err)
	}
	resp, err := noTimeoutCli.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download artifact: unexpected response status %d", resp.StatusCode)
	}
	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		fmt.Sprintf("Downloading %s", color.CyanString(fileName)),
	)
	_, err = io.Copy(io.MultiWriter(file, bar), resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	_ = file.Close()
	err = os.Rename(file.Name(), path)
	if err != nil {
		return fmt.Errorf("failed to move temp file: %w", err)
	}
	err = os.Chmod(path, 0755)
	if err != nil {
		return fmt.Errorf("failed to chmod binary: %w", err)
	}
	return nil
}

func needsLibolmDylib(bridge string) bool {
	switch bridge {
	case "imessage", "whatsapp", "discord", "slack", "gmessages", "gvoice", "signal", "imessagego", "meta", "twitter", "bluesky", "linkedin":
		return runtime.GOOS == "darwin"
	default:
		return false
	}
}

func DownloadMautrixBridgeBinary(ctx context.Context, bridge, path string, v2, noUpdate bool, branchOverride, currentCommit string) error {
	domain := "mau.dev"
	bridge = strings.TrimSuffix(bridge, "v2")
	repo := fmt.Sprintf("mautrix/%s", bridge)
	fileName := filepath.Base(path)
	ref, err := getRefFromBridge(bridge)
	if err != nil {
		return err
	}
	if branchOverride != "" {
		ref = branchOverride
	}
	job, err := getJobFromBridge(bridge)
	if err != nil {
		return err
	}
	if v2 {
		job += " v2"
	}

	if currentCommit == "" {
		log.Printf("Finding latest version of [cyan]%s[reset] from [cyan]%s[reset]", fileName, domain)
	} else {
		log.Printf("Checking for updates to [cyan]%s[reset] from [cyan]%s[reset]", fileName, domain)
	}
	build, err := GetLastBuild(domain, repo, ref, job)
	if err != nil {
		return fmt.Errorf("failed to get last build info: %w", err)
	}
	if build.Commit == currentCommit {
		log.Printf("[cyan]%s[reset] is up to date (commit: %s)", fileName, linkifyCommit(repo, currentCommit))
		return nil
	} else if currentCommit != "" && noUpdate {
		log.Printf("[cyan]%s[reset] [yellow]is out of date, latest commit is %s (diff: %s)[reset]", fileName, linkifyCommit(repo, build.Commit), linkifyDiff(repo, currentCommit, build.Commit))
		return nil
	} else if build.JobURL == "" {
		return fmt.Errorf("failed to find URL for job %q on branch %s of %s", job, ref, repo)
	}
	if currentCommit == "" {
		log.Printf("Installing [cyan]%s[reset] (commit: %s)", fileName, linkifyCommit(repo, build.Commit))
	} else {
		log.Printf("Updating [cyan]%s[reset] (diff: %s)", fileName, linkifyDiff(repo, currentCommit, build.Commit))
	}
	artifactURL := makeArtifactURL(domain, build.JobURL, fileName)
	err = downloadFile(ctx, artifactURL, path)
	if err != nil {
		return err
	}
	if needsLibolmDylib(bridge) {
		libolmPath := filepath.Join(filepath.Dir(path), "libolm.3.dylib")
		// TODO redownload libolm if it's outdated?
		if _, err = os.Stat(libolmPath); err != nil {
			err = downloadFile(ctx, makeArtifactURL(domain, build.JobURL, "libolm.3.dylib"), libolmPath)
			if err != nil {
				return fmt.Errorf("failed to download libolm: %w", err)
			}
		}
	}

	log.Printf("Successfully installed [cyan]%s[reset] commit %s", fileName, linkifyCommit(domain, build.Commit))
	return nil
}
