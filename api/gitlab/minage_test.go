package gitlab

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDecideUpdate(t *testing.T) {
	const week = 7 * 24 * time.Hour
	candidateTime := time.Date(2026, 10, 30, 16, 31, 30, 0, time.UTC)
	newer := time.Date(2026, 11, 5, 15, 33, 58, 0, time.UTC)
	older := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	candidate := &LastBuild{Commit: "candidate", JobURL: "/job/1", UpdatedAt: candidateTime}
	networkErr := errors.New("connection refused")

	tests := []struct {
		name         string
		installed    InstalledBuild
		candidate    *LastBuild
		lookupErr    error
		minAge       time.Duration
		want         UpdateAction
		wantInReason string
	}{
		{name: "no binary, no build old enough", candidate: nil, lookupErr: ErrNoBuild, minAge: week, want: ActionFail, wantInReason: "BEEPER_BRIDGE_MIN_AGE=0"},
		{name: "no binary, lookup failed", lookupErr: networkErr, minAge: week, want: ActionFail},
		{name: "no binary, candidate found", candidate: candidate, minAge: week, want: ActionInstall},
		{name: "binary, lookup failed", installed: InstalledBuild{Commit: "current", BuildTime: older}, lookupErr: networkErr, minAge: week, want: ActionKeep},
		{name: "binary, no build old enough", installed: InstalledBuild{Commit: "current", BuildTime: older}, lookupErr: ErrNoBuild, minAge: week, want: ActionKeep},
		{name: "binary is candidate", installed: InstalledBuild{Commit: "candidate", BuildTime: older}, candidate: candidate, minAge: week, want: ActionKeep},
		{name: "binary with unknown build time", installed: InstalledBuild{Commit: "current"}, candidate: candidate, minAge: week, want: ActionKeep, wantInReason: "unknown build time"},
		{name: "binary newer than candidate", installed: InstalledBuild{Commit: "current", BuildTime: newer}, candidate: candidate, minAge: week, want: ActionKeep, wantInReason: "newer"},
		{name: "binary older than candidate", installed: InstalledBuild{Commit: "current", BuildTime: older}, candidate: candidate, minAge: week, want: ActionInstall},
		{name: "binary built exactly at candidate time", installed: InstalledBuild{Commit: "current", BuildTime: candidateTime}, candidate: candidate, minAge: week, want: ActionInstall},
		{name: "min age 0, older binary", installed: InstalledBuild{Commit: "current", BuildTime: older}, candidate: candidate, want: ActionInstall},
		{name: "min age 0, newer binary still follows latest", installed: InstalledBuild{Commit: "current", BuildTime: newer}, candidate: candidate, want: ActionInstall},
		{name: "min age 0, lookup failed", installed: InstalledBuild{Commit: "current", BuildTime: older}, lookupErr: networkErr, want: ActionFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := DecideUpdate(tt.installed, tt.candidate, tt.lookupErr, tt.minAge)
			if got != tt.want {
				t.Errorf("action = %v, want %v (reason: %s)", got, tt.want, reason)
			}
			if reason == "" {
				t.Error("reason must not be empty")
			}
			if tt.wantInReason != "" && !strings.Contains(reason, tt.wantInReason) {
				t.Errorf("reason %q does not contain %q", reason, tt.wantInReason)
			}
		})
	}
}
