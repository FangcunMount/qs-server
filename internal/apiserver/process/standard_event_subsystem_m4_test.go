//go:build reliable_messaging_m4

package process

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestStandardBuildRejectsRetiredLegacyConfiguration(t *testing.T) {
	cases := []struct {
		name   string
		config *config.Config
	}{
		{"missing_config", nil},
		{"missing_options", &config.Config{}},
		{"missing_eventing", &config.Config{Options: options.NewOptions()}},
		{"missing_selection", &config.Config{Options: options.NewOptions()}},
	}
	cases[2].config.Eventing = nil
	cases[3].config.Eventing.StandardOutbox = nil
	cases = append(cases, struct {
		name   string
		config *config.Config
	}{"disabled_profiles", &config.Config{Options: options.NewOptions()}})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No catalog or database is provided: rejection must precede all
			// old-store construction, storage preflight and publisher ownership.
			subsystem, err := configuredEventSubsystem(tc.config)(eventsubsystem.Options{})
			if err == nil || !strings.Contains(err.Error(), "legacy outbox configuration is retired") {
				t.Fatalf("retired selection was not rejected before I/O: subsystem=%v err=%v", subsystem, err)
			}
			if subsystem != nil {
				t.Fatal("retired selection constructed a subsystem")
			}
		})
	}
}

func TestCandidateStandardOutboxRequiresNSQAndHostDatabase(t *testing.T) {
	cfg := &config.Config{Options: options.NewOptions()}
	cfg.Eventing.StandardOutbox.Mongo = true
	_, err := configuredEventSubsystem(cfg)(eventsubsystem.Options{})
	if err == nil || !strings.Contains(err.Error(), "live NSQ-backed") {
		t.Fatalf("standard profile accepted logging/fallback mode: %v", err)
	}
}
