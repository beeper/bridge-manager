package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseMinAge(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{input: "", want: 0},
		{input: "0", want: 0},
		{input: "7d", want: 168 * time.Hour},
		{input: "1d12h", wantErr: true},
		{input: "168h", want: 168 * time.Hour},
		{input: "90m", want: 90 * time.Minute},
		{input: "7", wantErr: true},
		{input: "1w", wantErr: true},
		{input: "-1d", wantErr: true},
		{input: "-5h", wantErr: true},
		{input: "abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMinAge(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMinAge(%q) = %v, want error", tt.input, got)
				}
				if !strings.Contains(err.Error(), `use a number of days like "7d" or a Go duration like "168h"`) {
					t.Errorf("ParseMinAge(%q) error %q does not explain the allowed forms", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMinAge(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseMinAge(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
