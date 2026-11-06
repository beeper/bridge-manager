package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
	"github.com/tidwall/gjson"

	"github.com/beeper/bridge-manager/cli/hyper"
	"github.com/beeper/bridge-manager/log"
)

// language=graphql
const getLastSuccessfulJobQuery = `
query($repo: ID!, $ref: String!, $job: String!, $updatedBefore: Time) {
  project(fullPath: $repo) {
    pipelines(status: SUCCESS, ref: $ref, first: 1, updatedBefore: $updatedBefore) {
      nodes {
        sha
        updatedAt
        job(name: $job) {
          webPath
        }
      }
    }
  }
}
`

type lastSuccessfulJobQueryVariables struct {
	Repo          string     `json:"repo"`
	Ref           string     `json:"ref"`
	Job           string     `json:"job"`
	UpdatedBefore *time.Time `json:"updatedBefore,omitempty"`
}

func newLastSuccessfulJobQueryVariables(repo, ref, job string, updatedBefore time.Time) lastSuccessfulJobQueryVariables {
	vars := lastSuccessfulJobQueryVariables{
		Repo: repo,
		Ref:  ref,
		Job:  job,
	}
	if !updatedBefore.IsZero() {
		utc := updatedBefore.UTC()
		vars.UpdatedBefore = &utc
	}
	return vars
}

// ErrNoBuild is returned by GetLastBuild when no successful pipeline matches the query.
var ErrNoBuild = errors.New("no successful build found")

type LastBuild struct {
	Commit string
	JobURL string
	// UpdatedAt is when the pipeline was last updated, which is when it finished for a successful pipeline.
	UpdatedAt time.Time
}

// GetLastBuild finds the newest successful pipeline on the given branch.
// If updatedBefore is not zero, only pipelines last updated before that time are considered.
func GetLastBuild(domain, repo, mainBranch, job string, updatedBefore time.Time) (*LastBuild, error) {
	resp, err := graphqlQuery(domain, getLastSuccessfulJobQuery, newLastSuccessfulJobQueryVariables(repo, mainBranch, job, updatedBefore))
	if err != nil {
		return nil, err
	}
	return lastBuildFromResponse(resp)
}

func lastBuildFromResponse(resp json.RawMessage) (*LastBuild, error) {
	res := gjson.GetBytes(resp, "project.pipelines.nodes.0")
	if !res.Exists() {
		return nil, ErrNoBuild
	}
	build := &LastBuild{
		Commit: gjson.Get(res.Raw, "sha").Str,
		JobURL: gjson.Get(res.Raw, "job.webPath").Str,
	}
	if updatedAt := gjson.Get(res.Raw, "updatedAt").Str; updatedAt != "" {
		parsed, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pipeline update time: %w", err)
		}
		build.UpdatedAt = parsed
	}
	return build, nil
}

func getRefFromBridge(bridge string) (string, error) {
	switch bridge {
	case "imessage":
		return "master", nil
	case "whatsapp", "discord", "slack", "gmessages", "gvoice", "signal",
		"imessagego", "meta", "twitter", "bluesky", "linkedin", "telegram":
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
	case "imessage", "whatsapp", "discord", "slack", "gmessages", "gvoice", "signal",
		"imessagego", "meta", "twitter", "bluesky", "linkedin", "telegram":
		return runtime.GOOS == "darwin"
	default:
		return false
	}
}

// DownloadMautrixBridgeBinary installs or updates a mautrix bridge binary from the mau.dev CI.
//
// If minAge is greater than zero, only builds whose pipeline finished at least
// minAge ago are considered, and the installed binary is never replaced by an
// older build. See DecideUpdate for the exact rules.
func DownloadMautrixBridgeBinary(ctx context.Context, bridge, path string, v2, noUpdate bool, branchOverride string, installed InstalledBuild, minAge time.Duration) error {
	domain := "mau.dev"
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

	if !installed.exists() {
		log.Printf("Finding latest version of [cyan]%s[reset] from [cyan]%s[reset]", fileName, domain)
	} else {
		log.Printf("Checking for updates to [cyan]%s[reset] from [cyan]%s[reset]", fileName, domain)
	}
	var updatedBefore time.Time
	if minAge > 0 {
		updatedBefore = time.Now().Add(-minAge)
		log.Printf("Only considering builds that finished before [cyan]%s[reset] (min age %s)", updatedBefore.UTC().Format(time.RFC3339), minAge)
	}
	build, lookupErr := GetLastBuild(domain, repo, ref, job, updatedBefore)
	action, reason := DecideUpdate(installed, build, lookupErr, minAge)
	switch action {
	case ActionFail:
		if minAge <= 0 {
			return fmt.Errorf("failed to get last build info: %w", lookupErr)
		}
		return errors.New(reason)
	case ActionKeep:
		if lookupErr != nil {
			log.Printf("[cyan]%s[reset] [yellow]%s[reset]", fileName, reason)
		} else if build.Commit == installed.Commit {
			log.Printf("[cyan]%s[reset] is up to date (commit: %s)", fileName, linkifyCommit(repo, installed.Commit))
		} else {
			log.Printf("[cyan]%s[reset] [yellow]%s (installed: %s, candidate: %s)[reset]", fileName, reason, linkifyCommit(repo, installed.Commit), linkifyCommit(repo, build.Commit))
		}
		logUsingBuild(fileName, repo, installed.Commit, installed.BuildTime, "built", minAge)
		return nil
	}

	if installed.exists() && noUpdate {
		log.Printf("[cyan]%s[reset] [yellow]is out of date, latest commit is %s (diff: %s)[reset]", fileName, linkifyCommit(repo, build.Commit), linkifyDiff(repo, installed.Commit, build.Commit))
		logUsingBuild(fileName, repo, installed.Commit, installed.BuildTime, "built", minAge)
		return nil
	} else if build.JobURL == "" {
		return fmt.Errorf("failed to find URL for job %q on branch %s of %s", job, ref, repo)
	}
	if !installed.exists() {
		log.Printf("Installing [cyan]%s[reset] (commit: %s)", fileName, linkifyCommit(repo, build.Commit))
	} else {
		log.Printf("Updating [cyan]%s[reset] (diff: %s)", fileName, linkifyDiff(repo, installed.Commit, build.Commit))
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

	log.Printf("Successfully installed [cyan]%s[reset] commit %s", fileName, linkifyCommit(repo, build.Commit))
	logUsingBuild(fileName, repo, build.Commit, build.UpdatedAt, "pipeline finished", minAge)
	return nil
}

// logUsingBuild logs which build will run, so the effect of --min-age is visible in the logs.
func logUsingBuild(fileName, repo, commit string, at time.Time, atLabel string, minAge time.Duration) {
	when := "unknown"
	if !at.IsZero() {
		when = at.UTC().Format(time.RFC3339)
	}
	log.Printf("Using [cyan]%s[reset] commit %s (%s %s, min age %s)", fileName, linkifyCommit(repo, commit), atLabel, when, minAge)
}
