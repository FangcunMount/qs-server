//go:build !reliable_messaging_m4

package process

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestOrdinaryBuildRejectsStandardOutboxSelection(t *testing.T) {
	cfg := &config.Config{Options: options.NewOptions()}
	cfg.Eventing.StandardOutbox.Mongo = true
	_, err := configuredEventSubsystem(cfg)(eventsubsystem.Options{})
	if err == nil || !strings.Contains(err.Error(), "requires the reviewed M4 candidate build") {
		t.Fatalf("ordinary build silently accepted M4 profile: %v", err)
	}
}
