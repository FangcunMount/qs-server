package process

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

func TestStandardProfileRetirementRejectsMixedConfigurationBeforeStorage(t *testing.T) {
	client, err := mongo.NewClient(mongooptions.Client().ApplyURI("mongodb://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Options: options.NewOptions()}
	cfg.MessagingOptions.Enabled = true
	cfg.MessagingOptions.Provider = "nsq"
	cfg.MessagingOptions.NSQAddr = "127.0.0.1:1"
	for _, tc := range []struct {
		name     string
		selected options.StandardOutboxOptions
		want     string
	}{
		{"missing_mongo", options.StandardOutboxOptions{Assessment: true}, "standard Mongo outbox is required"},
		{"missing_assessment", options.StandardOutboxOptions{Mongo: true}, "standard assessment outbox is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Neither handle can reach a real database. The former preflight would
			// touch the database; retirement must reject configuration first.
			subsystem, err := buildM4StandardEventSubsystem(eventsubsystem.Options{
				MongoDB: client.Database("no_io"), MySQLDB: &gorm.DB{},
				WirePublisher: &fakePublisher{}, PublisherMode: eventruntime.PublishModeMQ,
			}, cfg, tc.selected)
			if err == nil || subsystem != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("mixed configuration result: subsystem=%v error=%v", subsystem, err)
			}
		})
	}
}
