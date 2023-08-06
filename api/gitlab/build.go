package gitlab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

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
	case "imessage", "whatsapp":
		return "master", nil
	case "discord", "slack", "gmessages":
		return "main", nil
	default:
		return "", fmt.Errorf("unknown bridge %s", bridge)
	}
}

func getJobFromBridge(bridge string) (string, error) {
	switch bridge {
	case "imessage":
		if runtime.GOOS != "darwin" {
			return "", fmt.Errorf("mautrix-imessage can only run on Macs")
		}
		return "build universal", nil
	default:
		osAndArch := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
		switch osAndArch {
		case "linux/amd64":
			return "build amd64", nil
		case "linux/arm64":
			return "build arm64", nil
		case "linux/arm":
			if bridge == "signal" {
				return "", fmt.Errorf("mautrix-signal does not support 32-bit arm")
			}
			return "build arm", nil
		case "darwin/arm64":
			return "build macos arm64", nil
		case "darwin/amd64":
			return "build macos amd64", nil
		default:
			return "", fmt.Errorf("binaries for %s are not yet built in the CI", osAndArch)
		}
	}
}

func linkifyCommit(repo, commit string) string {
	return hyper.Link(commit[:8], fmt.Sprintf("https://github.com/%s/commit/%s", repo, commit), false)
}

func linkifyDiff(repo, fromCommit, toCommit string) string {
	formattedDiff := fmt.Sprintf("%s...%s", fromCommit[:8], toCommit[:8])
	return hyper.Link(formattedDiff, fmt.Sprintf("https://github.com/%s/compare/%s...%s", repo, fromCommit, toCommit), false)
}

func DownloadMautrixBridgeBinary(ctx context.Context, bridge, path string, noUpdate bool, branchOverride, currentCommit string) error {
	domain := "mau.dev"
	repo := fmt.Sprintf("mautrix/%s", bridge)
	fileName := fmt.Sprintf("mautrix-%s", bridge)
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
	}
	if currentCommit == "" {
		log.Printf("Installing [cyan]%s[reset] (commit: %s)", fileName, linkifyCommit(repo, build.Commit))
	} else {
		log.Printf("Updating [cyan]%s[reset] (diff: %s)", fileName, linkifyDiff(repo, currentCommit, build.Commit))
	}
	file, err := os.CreateTemp(filepath.Dir(path), "tmp-"+fileName+"-*")
	if err != nil {
		return fmt.Errorf("failed to open temp file: %w", err)
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()
	artifactURL := (&url.URL{
		Scheme: "https",
		Host:   domain,
		Path:   filepath.Join(build.JobURL, "artifacts", "raw", fileName),
	}).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return fmt.Errorf("failed to prepare download request: %w", err)
	}
	resp, err := noTimeoutCli.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download artifact: %w", err)
	}
	defer resp.Body.Close()
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
	log.Printf("Successfully installed [cyan]%s[reset] commit %s", fileName, linkifyCommit(domain, build.Commit))
	return nil
}
