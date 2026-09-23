//go:build reliable_messaging_m4

package process

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

func TestCandidateBuildKeepsLegacySubsystemWhenStandardOutboxDisabled(t *testing.T) {
	wire, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Options: options.NewOptions()}
	subsystem, err := configuredEventSubsystem(cfg)(eventsubsystem.Options{Catalog: eventcatalog.NewCatalog(wire)})
	if err != nil {
		t.Fatalf("disabled standard outbox unexpectedly requires MQ or database preflight: %v", err)
	}
	if subsystem == nil {
		t.Fatal("disabled standard outbox did not construct the existing event subsystem")
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
