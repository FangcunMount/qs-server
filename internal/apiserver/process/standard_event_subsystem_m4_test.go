package process

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
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

func TestStandardBuildKeepsValidationOrderBeforeResourceAcquisition(t *testing.T) {
	client, err := mongo.NewClient(mongooptions.Client().ApplyURI("mongodb://127.0.0.1:1")) //nolint:staticcheck // SA1019: A disconnected client proves rejection precedes storage I/O.
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Options: options.NewOptions()}
	cfg.MessagingOptions.Enabled = true
	cfg.MessagingOptions.Provider = "nsq"
	cfg.MessagingOptions.NSQAddr = "127.0.0.1:1"
	for _, test := range []struct {
		name     string
		config   *config.Config
		opts     eventsubsystem.Options
		selected options.StandardOutboxOptions
		want     string
	}{
		{"messaging_before_missing_host", nil, eventsubsystem.Options{}, options.StandardOutboxOptions{Mongo: true}, "M4 standard outbox requires a live NSQ-backed messaging configuration"},
		{"missing_host_before_mixed_profile", cfg, eventsubsystem.Options{MongoDB: client.Database("no_io")}, options.StandardOutboxOptions{Assessment: true}, "M4 standard outbox requires its host database"},
		{"catalog_before_storage", cfg, eventsubsystem.Options{MySQLDB: &gorm.DB{}}, options.StandardOutboxOptions{Assessment: true}, "event catalog is not loaded"},
		{"routes_before_storage", cfg, eventsubsystem.Options{Catalog: eventcatalog.NewCatalog(events)}, options.StandardOutboxOptions{}, "M4 standard profiles have no reliable NSQ routes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.opts.WirePublisher = &fakePublisher{}
			test.opts.PublisherMode = eventruntime.PublishModeMQ
			subsystem, err := buildM4StandardEventSubsystem(test.opts, test.config, test.selected)
			if subsystem != nil || err == nil || err.Error() != test.want {
				t.Fatalf("subsystem=%v err=%v, want first error %q", subsystem, err, test.want)
			}
		})
	}
}
