package options

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestReportCatalogAuditDefaults(t *testing.T) {
	t.Parallel()
	opts := NewReportCatalogAuditOptions()
	if !opts.Enable || opts.InitialDelay != 15*time.Minute || opts.TickInterval != 5*time.Second || opts.CycleInterval != 24*time.Hour || opts.BatchSize != 200 || opts.BatchTimeout != 3*time.Second || opts.LockTTL != 30*time.Second {
		t.Fatalf("defaults = %#v", opts)
	}
}

func TestReportCatalogAuditFlagsMapAllRuntimeControls(t *testing.T) {
	t.Parallel()
	opts := NewReportCatalogAuditOptions()
	flags := pflag.NewFlagSet("audit", pflag.ContinueOnError)
	opts.AddFlags(flags)
	err := flags.Parse([]string{
		"--report_catalog_audit.initial-delay=1m", "--report_catalog_audit.tick-interval=2s",
		"--report_catalog_audit.cycle-interval=12h", "--report_catalog_audit.batch-size=50",
		"--report_catalog_audit.batch-timeout=1s", "--report_catalog_audit.lock-key=custom", "--report_catalog_audit.lock-ttl=10s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.InitialDelay != time.Minute || opts.TickInterval != 2*time.Second || opts.CycleInterval != 12*time.Hour || opts.BatchSize != 50 || opts.BatchTimeout != time.Second || opts.LockKey != "custom" || opts.LockTTL != 10*time.Second {
		t.Fatalf("parsed options = %#v", opts)
	}
}

func TestReportCatalogAuditValidationRejectsUnboundedBatch(t *testing.T) {
	t.Parallel()
	opts := NewReportCatalogAuditOptions()
	opts.BatchSize = 501
	errs := validateReportCatalogAudit(opts)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "between 1 and 500") {
		t.Fatalf("errors = %v", errs)
	}
}
