//go:build reliable_messaging_m4

package process

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestCandidateStandardOutboxRequiresNSQAndHostDatabase(t *testing.T) {
	cfg := &config.Config{Options: options.NewOptions()}
	cfg.Eventing.StandardOutbox.Mongo = true
	_, err := configuredEventSubsystem(cfg)(eventsubsystem.Options{})
	if err == nil || !strings.Contains(err.Error(), "live NSQ-backed") {
		t.Fatalf("standard profile accepted logging/fallback mode: %v", err)
	}
}
