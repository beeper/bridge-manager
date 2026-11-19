package gitlab

import (
	"errors"
	"fmt"
	"time"
)

// InstalledBuild describes the bridge binary that is currently installed.
// The zero value means no binary is installed.
type InstalledBuild struct {
	Commit    string
	BuildTime time.Time
}

func (ib InstalledBuild) exists() bool {
	return ib.Commit != ""
}

// UpdateAction is what DownloadMautrixBridgeBinary should do with a candidate build.
type UpdateAction int

const (
	// ActionKeep keeps the installed binary.
	ActionKeep UpdateAction = iota
	// ActionInstall downloads and installs the candidate build.
	ActionInstall
	// ActionFail aborts, because there is no usable binary.
	ActionFail
)

func (a UpdateAction) String() string {
	switch a {
	case ActionKeep:
		return "keep"
	case ActionInstall:
		return "install"
	case ActionFail:
		return "fail"
	default:
		return fmt.Sprintf("UpdateAction(%d)", int(a))
	}
}

// DecideUpdate decides whether the candidate build should replace the installed one.
//
// With minAge 0 it keeps the upstream behaviour: a failed lookup is fatal and
// any different commit is installed. With minAge > 0 the installed binary is
// never replaced by an older build, and a failed lookup or a missing candidate
// keeps the installed binary running instead of stopping the bridge.
// The returned string explains the decision and is meant for the log.
func DecideUpdate(installed InstalledBuild, candidate *LastBuild, lookupErr error, minAge time.Duration) (UpdateAction, string) {
	if minAge <= 0 {
		return decideLatest(installed, candidate, lookupErr)
	}
	if lookupErr != nil {
		return decideLookupFailed(installed, lookupErr, minAge)
	}
	switch {
	case !installed.exists():
		return ActionInstall, fmt.Sprintf("installing the newest build older than %s", minAge)
	case installed.Commit == candidate.Commit:
		return ActionKeep, fmt.Sprintf("up to date with the newest build older than %s", minAge)
	case installed.BuildTime.IsZero():
		return ActionKeep, "keeping the installed build: unknown build time, so it may be newer than the candidate"
	case installed.BuildTime.After(candidate.UpdatedAt):
		return ActionKeep, fmt.Sprintf("keeping the installed build: it is newer than the newest build older than %s", minAge)
	default:
		return ActionInstall, fmt.Sprintf("updating to the newest build older than %s", minAge)
	}
}

func decideLatest(installed InstalledBuild, candidate *LastBuild, lookupErr error) (UpdateAction, string) {
	if lookupErr != nil {
		return ActionFail, fmt.Sprintf("failed to get last build info: %v", lookupErr)
	}
	if installed.exists() && installed.Commit == candidate.Commit {
		return ActionKeep, "up to date with the latest build"
	}
	return ActionInstall, "installing the latest build"
}

func decideLookupFailed(installed InstalledBuild, lookupErr error, minAge time.Duration) (UpdateAction, string) {
	if installed.exists() {
		return ActionKeep, fmt.Sprintf("keeping the installed build: could not find a build older than %s: %v", minAge, lookupErr)
	}
	if errors.Is(lookupErr, ErrNoBuild) {
		return ActionFail, fmt.Sprintf("no successful build older than %s exists and no binary is installed (set BEEPER_BRIDGE_MIN_AGE=0 to install the latest build)", minAge)
	}
	return ActionFail, fmt.Sprintf("failed to get last build info: %v", lookupErr)
}
