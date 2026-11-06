package gitlab

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestQueryVariablesOmitZeroUpdatedBefore(t *testing.T) {
	vars := newLastSuccessfulJobQueryVariables("mautrix/whatsapp", "main", "build amd64", time.Time{})
	data, err := json.Marshal(vars)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "updatedBefore") {
		t.Errorf("zero updatedBefore must be omitted, got %s", data)
	}
}

func TestQueryVariablesUpdatedBefore(t *testing.T) {
	before := time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC)
	vars := newLastSuccessfulJobQueryVariables("mautrix/whatsapp", "main", "build amd64", before)
	data, err := json.Marshal(vars)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"updatedBefore":"2026-10-30T22:00:00Z"`) {
		t.Errorf("updatedBefore missing from %s", data)
	}
}

func TestLastBuildFromResponse(t *testing.T) {
	resp := json.RawMessage(`{"project":{"pipelines":{"nodes":[{"sha":"5dd621fd","updatedAt":"2026-10-30T16:31:30Z","job":{"webPath":"/mautrix/whatsapp/-/jobs/1"}}]}}}`)
	build, err := lastBuildFromResponse(resp)
	if err != nil {
		t.Fatal(err)
	}
	if build.Commit != "5dd621fd" {
		t.Errorf("Commit = %q", build.Commit)
	}
	if build.JobURL != "/mautrix/whatsapp/-/jobs/1" {
		t.Errorf("JobURL = %q", build.JobURL)
	}
	want := time.Date(2026, 10, 30, 16, 31, 30, 0, time.UTC)
	if !build.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", build.UpdatedAt, want)
	}
}

func TestLastBuildFromResponseEmpty(t *testing.T) {
	_, err := lastBuildFromResponse(json.RawMessage(`{"project":{"pipelines":{"nodes":[]}}}`))
	if !errors.Is(err, ErrNoBuild) {
		t.Errorf("err = %v, want ErrNoBuild", err)
	}
}
