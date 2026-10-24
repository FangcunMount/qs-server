package options

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/spf13/viper"
)

func TestTransactionOwnersSurviveFileConfiguration(t *testing.T) {
	reader := viper.New()
	reader.SetConfigType("yaml")
	if err := reader.ReadConfig(strings.NewReader(`messaging:
  provider: rocketmq
  rocketmq:
    transactions:
      qs.mysql:
        store_id: MySQL-Primary
        writer_epoch: m3-1
        route_revision: m3-1
        credentials_file: /fixture/mysql.json
      qs.mongo:
        store_id: Mongo-Primary
        writer_epoch: m3-1
        route_revision: m3-1
        credentials_file: /fixture/mongo.json
`)); err != nil {
		t.Fatal(err)
	}
	opts := NewOptions()
	if err := app.DecodeOptions(reader, opts); err != nil {
		t.Fatal(err)
	}
	bindings := opts.MessagingOptions.RocketMQ.Transactions
	if len(bindings) != 2 || bindings["qs.mysql"].StoreID != "MySQL-Primary" || bindings["qs.mongo"].StoreID != "Mongo-Primary" || bindings["qs.mysql"].CredentialsFile != "/fixture/mysql.json" || bindings["qs.mongo"].CredentialsFile != "/fixture/mongo.json" {
		t.Fatalf("transaction owner identities or bindings changed: %#v", bindings)
	}
}

func TestAIWorkflowTransactionOwnerSurvivesFileConfiguration(t *testing.T) {
	reader := viper.New()
	reader.SetConfigType("yaml")
	if err := reader.ReadConfig(strings.NewReader(`ai_workflow:
  messaging:
    provider: rocketmq
    rocketmq:
      transactions:
        qs.mysql:
          store_id: AI-Workflow-Primary
          writer_epoch: m3-1
          route_revision: m3-1
          credentials_file: /fixture/command.json
`)); err != nil {
		t.Fatal(err)
	}
	opts := NewOptions()
	if err := app.DecodeOptions(reader, opts); err != nil {
		t.Fatal(err)
	}
	bindings := opts.AIWorkflow.Messaging.RocketMQ.Transactions
	if len(bindings) != 1 || bindings["qs.mysql"].StoreID != "AI-Workflow-Primary" || bindings["qs.mysql"].CredentialsFile != "/fixture/command.json" {
		t.Fatalf("AI workflow transaction identity lost: %#v", bindings)
	}
}
