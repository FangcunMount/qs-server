package aimessagingbinding

import (
	"bytes"
	"net"
	"strconv"
	"time"

	options "github.com/FangcunMount/qs-server/internal/apiserver/options"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/spf13/viper"
)

const transportRoot = "/app/configs/rocketmq/"
const apiInstance = "qs-apiserver-1"

// ValidateFinalBinding is pure deployment policy. It must run before key reads,
// client construction or database access; original IDs are compared as strings.
func ValidateFinalBinding(b Binding) error {
	if b.Provider != "rocketmq" || !b.Enabled || b.MaxInFlight != 1 || len(b.NSQD) != 0 || b.PublishEnabled == nil || !finalTransport(b.RocketMQ) {
		return ErrBinding
	}
	if len(b.RocketMQ.Transactions) != 1 || !finalOwner("qs.mysql", b.RocketMQ.Transactions["qs.mysql"]) {
		return ErrBinding
	}
	owner := b.RocketMQ.Transactions["qs.mysql"]
	if owner.CredentialsFile != "" && owner.CredentialsFile != b.RocketMQ.CredentialsFile {
		return ErrBinding
	}
	if b.RocketMQ.PublishEnabled != nil && *b.RocketMQ.PublishEnabled != *b.PublishEnabled {
		return ErrBinding
	}
	if b.Options().Validate() != nil {
		return ErrBinding
	}
	if _, err := b.Files(); err != nil {
		return ErrBinding
	}
	return nil
}

func finalTransport(o *generic.RocketMQOptions) bool {
	if o == nil || o.Validate() != nil || o.CredentialsFile != transportRoot+"credentials.json" || o.CAFile != transportRoot+"ca.crt" || o.RoutesFile != transportRoot+"routes.json" || o.InstanceID != apiInstance {
		return false
	}
	host, port, err := net.SplitHostPort(o.Endpoint)
	n, numberErr := strconv.Atoi(port)
	return err == nil && host != "" && numberErr == nil && n > 0 && n <= 65535
}

func finalOwner(owner string, b generic.TransactionOptions) bool {
	if b.Limits == nil {
		return false
	}
	actual, err := b.ProtocolLimits(owner)
	expected := transaction.Limits{Sends: 4, Branches: 16, CheckWorkers: 2, CheckQueue: 16, ReconcileBatch: 50, CheckTimeout: 2 * time.Second, RPCTimeout: 5 * time.Second, Lease: time.Minute, ReconcileInterval: 5 * time.Second}
	return err == nil && actual == expected
}

func sameTransport(a, b *generic.RocketMQOptions) bool {
	return a.Endpoint == b.Endpoint && a.CredentialsFile == b.CredentialsFile && a.CAFile == b.CAFile && a.RoutesFile == b.RoutesFile && a.InstanceID == b.InstanceID
}

// RenderFinal freezes and validates the configuration through the application's
// real decoder. No environment is merged and no key, pool, broker or task starts.
// Generic Render remains available to separately tracked historical probes.
func RenderFinal(base []byte, b Binding) ([]byte, error) {
	if ValidateFinalBinding(b) != nil {
		return nil, ErrBinding
	}
	result, err := Render(base, b)
	if err != nil {
		return nil, ErrBinding
	}
	if _, err := finalOptions(result); err != nil {
		return nil, ErrBinding
	}
	return result, nil
}

func finalOptions(result []byte) (*options.Options, error) {
	v := viper.New()
	v.SetConfigType("json")
	if v.ReadConfig(bytes.NewReader(result)) != nil {
		return nil, ErrBinding
	}
	o := options.NewOptions()
	if app.DecodeOptions(v, o) != nil {
		return nil, ErrBinding
	}
	if o.MessagingOptions == nil || !o.MessagingOptions.Enabled || o.MessagingOptions.Provider != "rocketmq" || o.AIWorkflow == nil || !o.AIWorkflow.Enabled || o.IAMOptions == nil || o.IAMOptions.AuthzSync == nil || o.MigrationOptions == nil || o.MigrationOptions.Enabled || o.Eventing == nil || o.Eventing.StandardOutbox == nil || o.Eventing.StandardOutbox.Mongo || o.Eventing.StandardOutbox.Assessment {
		return nil, ErrBinding
	}
	root := o.MessagingOptions.RocketMQ
	if !finalTransport(root) || root.PublishEnabled == nil || len(root.Transactions) != 2 || o.AIWorkflow.Messaging.Provider != "rocketmq" || !o.AIWorkflow.Messaging.Enabled {
		return nil, ErrBinding
	}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		if !finalOwner(owner, root.Transactions[owner]) {
			return nil, ErrBinding
		}
	}
	sql, mongo := root.Transactions["qs.mysql"], root.Transactions["qs.mongo"]
	if sql.CredentialsFile != root.CredentialsFile || mongo.CredentialsFile != transportRoot+"mongo-credentials.json" {
		return nil, ErrBinding
	}
	bridgeOptions := o.AIWorkflow.Messaging.RocketMQ
	if !finalTransport(bridgeOptions) || bridgeOptions.PublishEnabled == nil || len(bridgeOptions.Transactions) != 1 || !sameTransport(root, bridgeOptions) || *root.PublishEnabled != *bridgeOptions.PublishEnabled || !finalOwner("qs.mysql", bridgeOptions.Transactions["qs.mysql"]) {
		return nil, ErrBinding
	}
	bridge := bridgeOptions.Transactions["qs.mysql"]
	if (bridge.CredentialsFile != "" && bridge.CredentialsFile != sql.CredentialsFile) || bridge.StoreID != sql.StoreID || bridge.WriterEpoch != sql.WriterEpoch || bridge.RouteRevision != sql.RouteRevision {
		return nil, ErrBinding
	}
	sub := o.IAMOptions.AuthzSync
	if !sub.Enabled || sub.Provider != "rocketmq" || sub.EphemeralNSQ || sub.Topic != generic.DefaultIAMAuthzSyncTopic || sub.ChannelPrefix != "qs-authz-sync" || !finalTransport(sub.RocketMQ) || sub.RocketMQ.PublishEnabled == nil || sub.RocketMQ.PublishingEnabled() || len(sub.RocketMQ.Transactions) != 0 || !sameTransport(root, sub.RocketMQ) {
		return nil, ErrBinding
	}
	return o, nil
}
