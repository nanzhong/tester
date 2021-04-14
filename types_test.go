package tester

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDuration_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name           string
		durationString string
		expectDuration time.Duration
		wantErr        bool
	}{
		{
			name:           "seconds",
			durationString: "5s",
			expectDuration: 5 * time.Second,
			wantErr:        false,
		},
		{
			name:           "minutes",
			durationString: "5m",
			expectDuration: 5 * time.Minute,
			wantErr:        false,
		},
		{
			name:           "hours",
			durationString: "5h",
			expectDuration: 5 * time.Hour,
			wantErr:        false,
		},
		{
			name:           "combination",
			durationString: "1h30m",
			expectDuration: 1*time.Hour + 30*time.Minute,
			wantErr:        false,
		},
		{
			name:           "should faile",
			durationString: "some non duration value",
			expectDuration: 0,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonInput := fmt.Sprintf(`{"run_delay": "%s"}`, tt.durationString)

			var actual struct {
				RunDelay RunDelay `json:"run_delay"`
			}

			err := json.NewDecoder(strings.NewReader(jsonInput)).Decode(&actual)

			require.Equal(t, tt.wantErr, err != nil)
			require.Equal(t, tt.expectDuration, actual.RunDelay.Duration)
		})
	}
}
