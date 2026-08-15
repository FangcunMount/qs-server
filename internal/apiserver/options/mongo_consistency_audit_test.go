package options

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestMongoConsistencyAuditDefaultsAreSafeAndDisabled(t *testing.T) {
	opts := NewMongoConsistencyAuditOptions()
	if opts.Enable || opts.InitialDelay != 30*time.Minute || opts.TickInterval != 5*time.Second ||
		opts.CycleInterval != 24*time.Hour || opts.BatchSize != 200 || opts.BatchTimeout != 3*time.Second ||
		opts.LockTTL != 30*time.Second || opts.MaxSamples != 10 {
		t.Fatalf("defaults = %#v", opts)
	}
}

func TestMongoConsistencyAuditFlagsAndValidation(t *testing.T) {
	opts := NewMongoConsistencyAuditOptions()
	flags := pflag.NewFlagSet("audit", pflag.ContinueOnError)
	opts.AddFlags(flags)
	if err := flags.Parse([]string{
		"--mongo_consistency_audit.enable=true", "--mongo_consistency_audit.initial-delay=1m",
		"--mongo_consistency_audit.tick-interval=2s", "--mongo_consistency_audit.cycle-interval=12h",
		"--mongo_consistency_audit.batch-size=50", "--mongo_consistency_audit.batch-timeout=1s",
		"--mongo_consistency_audit.max-samples=5", "--mongo_consistency_audit.lock-key=custom",
		"--mongo_consistency_audit.lock-ttl=10s",
	}); err != nil {
		t.Fatal(err)
	}
	if errs := validateMongoConsistencyAudit(opts); len(errs) != 0 {
		t.Fatalf("validation errors = %v", errs)
	}
	if !opts.Enable || opts.BatchSize != 50 || opts.MaxSamples != 5 || opts.LockKey != "custom" {
		t.Fatalf("flags = %#v", opts)
	}
	opts.BatchSize = 501
	if errs := validateMongoConsistencyAudit(opts); len(errs) == 0 {
		t.Fatal("unbounded batch validation errors = nil")
	}
}
