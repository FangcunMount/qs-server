package options

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/reliable-messaging/transport"
)

func TestRouteManifestVersionAndTypeCannotBeMixed(t *testing.T) {
	const topic = "iam.authz.version.v2"
	physical, err := transport.PhysicalTopicV2(topic, "TRANSACTION", "iam.mysql", "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	v2 := transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: "TRANSACTION", Owner: "iam.mysql", RouteRevision: "m3-1"}
	legacy := transport.RouteBinding{Topic: topic, PhysicalTopic: transport.PhysicalTopic(topic)}
	for _, tc := range []struct {
		name    string
		version int
		route   transport.RouteBinding
		valid   bool
	}{
		{"v2", 2, v2, true}, {"legacy", 1, legacy, true},
		{"v2-missing-type", 2, legacy, false}, {"v1-hidden-v2", 1, v2, false}, {"unknown-version", 3, v2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(struct {
				Version int                      `json:"version"`
				Routes  []transport.RouteBinding `json:"routes"`
			}{tc.version, []transport.RouteBinding{tc.route}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = decodeRouteManifest(raw)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestTransactionConfigRequiresExactOwnerRevisionAndAllTopics(t *testing.T) {
	const owner = "qs.mysql"
	opts := TransactionOptions{StoreID: "primary-test", WriterEpoch: "m3-epoch", RouteRevision: "m3-1"}
	limits, err := opts.ProtocolLimits(owner)
	if err != nil {
		t.Fatal(err)
	}
	if limits.Sends != 4 || limits.CheckWorkers != 2 || limits.CheckQueue != 16 {
		t.Fatalf("unexpected budget: %+v", limits)
	}
	topic := "qs.evaluation.lifecycle"
	physical, _ := transport.PhysicalTopicV2(topic, "TRANSACTION", owner, "m3-1")
	routes := []transport.RouteBinding{{Topic: topic, PhysicalTopic: physical, MessageType: "TRANSACTION", Owner: owner, RouteRevision: "m3-1"}}
	got, err := transactionRoutes(routes, owner, opts, []string{topic})
	if err != nil || len(got) != 1 {
		t.Fatalf("valid routes=%v, err=%v", got, err)
	}
	if _, err = transactionRoutes(routes, owner, opts, []string{topic, "absent"}); err == nil {
		t.Fatal("missing topic accepted")
	}
	routes[0].Owner = "wrong.mysql"
	if _, err = transactionRoutes(routes, owner, opts, []string{topic}); err == nil {
		t.Fatal("wrong owner accepted")
	}
	opts.WriterEpoch = ""
	if _, err = opts.ProtocolLimits(owner); err == nil {
		t.Fatal("missing binding accepted")
	}
	opts.WriterEpoch = "m3-epoch"
	opts.Limits = &TransactionLimits{Sends: 1, Branches: 17, CheckWorkers: 1, CheckQueue: 1}
	if _, err = opts.ProtocolLimits(owner); err == nil {
		t.Fatal("invalid budget accepted")
	}
}
