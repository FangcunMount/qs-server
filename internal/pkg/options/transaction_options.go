package options

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

// TransactionOptions is an explicit host/database binding, never message metadata.
// Presence in the owner's map opts into the separate transaction entrypoint.
type TransactionOptions struct {
	// Optional for a single owner; required per owner when a host binds both stores.
	CredentialsFile string             `json:"credentials_file,omitempty" mapstructure:"credentials_file"`
	StoreID         string             `json:"store_id" mapstructure:"store_id"`
	WriterEpoch     string             `json:"writer_epoch" mapstructure:"writer_epoch"`
	RouteRevision   string             `json:"route_revision" mapstructure:"route_revision"`
	Limits          *TransactionLimits `json:"limits,omitempty" mapstructure:"limits"`
}

// A nonnil override is complete; zero/negative budgets are not silently defaulted.
type TransactionLimits struct {
	Sends             int           `json:"sends" mapstructure:"sends"`
	Branches          int           `json:"branches" mapstructure:"branches"`
	CheckWorkers      int           `json:"check_workers" mapstructure:"check_workers"`
	CheckQueue        int           `json:"check_queue" mapstructure:"check_queue"`
	ReconcileBatch    int           `json:"reconcile_batch" mapstructure:"reconcile_batch"`
	CheckTimeout      time.Duration `json:"check_timeout" mapstructure:"check_timeout"`
	RPCTimeout        time.Duration `json:"rpc_timeout" mapstructure:"rpc_timeout"`
	Lease             time.Duration `json:"lease" mapstructure:"lease"`
	ReconcileInterval time.Duration `json:"reconcile_interval" mapstructure:"reconcile_interval"`
}

func (o TransactionOptions) ProtocolLimits(owner string) (transaction.Limits, error) {
	if (owner != "qs.mysql" && owner != "qs.mongo") || o.RouteRevision != "m3-1" || !(transaction.Binding{Owner: owner, StoreID: o.StoreID, WriterEpoch: o.WriterEpoch}).Valid() {
		return transaction.Limits{}, errors.New("explicit host transaction owner, primary store, epoch and m3-1 revision required")
	}
	limits := transaction.DefaultLimits()
	limits.Sends = 4
	limits.CheckWorkers = 2
	limits.CheckQueue = 16
	limits.ReconcileBatch = 50
	if o.Limits != nil {
		l := o.Limits
		limits = transaction.Limits{Sends: l.Sends, Branches: l.Branches, CheckWorkers: l.CheckWorkers, CheckQueue: l.CheckQueue, ReconcileBatch: l.ReconcileBatch, CheckTimeout: l.CheckTimeout, RPCTimeout: l.RPCTimeout, Lease: l.Lease, ReconcileInterval: l.ReconcileInterval}
	}
	if !limits.Valid() {
		return transaction.Limits{}, errors.New("invalid bounded host transaction limits")
	}
	return limits, nil
}

func decodeRouteManifest(raw []byte) ([]transport.RouteBinding, error) {
	var manifest struct {
		Version int                      `json:"version"`
		Routes  []transport.RouteBinding `json:"routes"`
	}
	if json.Unmarshal(raw, &manifest) != nil || (manifest.Version != 1 && manifest.Version != 2) {
		return nil, errors.New("invalid RocketMQ route manifest version")
	}
	for _, r := range manifest.Routes {
		if (manifest.Version == 1 && (r.MessageType != "" || r.Owner != "" || r.RouteRevision != "")) || (manifest.Version == 2 && r.MessageType == "") {
			return nil, errors.New("RocketMQ route fields conflict with manifest version")
		}
	}
	if err := transport.ValidateRoutes(manifest.Routes); err != nil {
		return nil, err
	}
	return manifest.Routes, nil
}

func transactionRoutes(routes []transport.RouteBinding, owner string, o TransactionOptions, topics []string) ([]rocketmq.TransactionRoute, error) {
	if _, err := o.ProtocolLimits(owner); err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return nil, errors.New("explicit transaction topic list required")
	}
	wanted := map[string]bool{}
	for _, topic := range topics {
		if topic == "" {
			return nil, errors.New("empty transaction topic")
		}
		wanted[topic] = false
	}
	selected := []rocketmq.TransactionRoute{}
	seen := map[string]bool{}
	for _, r := range routes {
		if _, ok := wanted[r.Topic]; !ok || r.Owner != owner || r.MessageType != transport.TransactionMessageType || r.RouteRevision != o.RouteRevision {
			continue
		}
		if seen[r.PhysicalTopic] {
			continue
		}
		canonical, err := transport.PhysicalTopicV2(r.Topic, r.MessageType, r.Owner, r.RouteRevision)
		if err != nil || canonical != r.PhysicalTopic {
			return nil, errors.New("noncanonical transaction route")
		}
		seen[r.PhysicalTopic] = true
		wanted[r.Topic] = true
		selected = append(selected, rocketmq.TransactionRoute{LogicalTopic: r.Topic, PhysicalTopic: r.PhysicalTopic, Revision: r.RouteRevision})
	}
	for _, found := range wanted {
		if !found {
			return nil, errors.New("required owner transaction topic absent from manifest")
		}
	}
	return selected, nil
}

// TransactionConfig only reads explicit host files. The caller owns Start/Close.
func (o *RocketMQOptions) TransactionConfig(owner string, topics []string) (rocketmq.TransactionConfig, error) {
	if o == nil {
		return rocketmq.TransactionConfig{}, errors.New("transaction configuration required")
	}
	binding, ok := o.Transactions[owner]
	if !ok {
		return rocketmq.TransactionConfig{}, errors.New("explicit owner transaction binding required")
	}
	limits, err := binding.ProtocolLimits(owner)
	if err != nil {
		return rocketmq.TransactionConfig{}, err
	}
	ownerOptions := *o
	if binding.CredentialsFile != "" {
		ownerOptions.CredentialsFile = binding.CredentialsFile
	}
	base, err := ownerOptions.Load(topics, limits.Sends, limits.RPCTimeout)
	if err != nil {
		return rocketmq.TransactionConfig{}, err
	}
	routes, err := transactionRoutes(base.Routes, owner, binding, topics)
	if err != nil {
		return rocketmq.TransactionConfig{}, err
	}
	return rocketmq.TransactionConfig{TLS: base.TLS, Endpoint: base.Endpoint, AccessKey: base.AccessKey, SecretKey: base.SecretKey, Owner: owner, Routes: routes, Limits: limits}, nil
}
