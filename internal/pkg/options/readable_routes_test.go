package options

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/reliable-messaging/transport"
)

func TestReadableDualStoreOwnerConfigurationExcludesHistory(t *testing.T) {
	const topic = "qs.evaluation.lifecycle"
	var active, history []transport.RouteBinding
	group, _ := transport.ConsumerGroupV3(topic, "qs-worker")
	oldGroup, _ := transport.ConsumerGroupV2(topic, "qs-worker")
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		physical, _ := transport.PhysicalTopicV3(topic, "TRANSACTION", owner, "m3-1")
		old, _ := transport.PhysicalTopicV2(topic, "TRANSACTION", owner, "m3-1")
		active = append(active, transport.RouteBinding{Topic: topic, Subscription: "qs-worker", ConsumerGroup: group, PhysicalTopic: physical, MessageType: "TRANSACTION", Owner: owner, RouteRevision: "m3-1", NamingScheme: transport.ReadableNamingV1})
		history = append(history, transport.RouteBinding{Topic: topic, Subscription: "qs-worker", ConsumerGroup: oldGroup, PhysicalTopic: old, MessageType: "TRANSACTION", Owner: owner, RouteRevision: "m3-1"})
	}
	raw, _ := json.Marshal(transport.RouteManifest{Version: 3, Routes: active, HistoricalRoutes: history})
	got, err := decodeRouteManifest(raw)
	if err != nil || len(got) != 2 {
		t.Fatal("history became active or v3 rejected", err)
	}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		binding := TransactionOptions{StoreID: "fixture-primary", WriterEpoch: "m3-1", RouteRevision: "m3-1"}
		selected, err := transactionRoutes(got, owner, binding, []string{topic})
		if err != nil || len(selected) != 1 || selected[0].NamingScheme != transport.ReadableNamingV1 {
			t.Fatal("owner isolation lost", err)
		}
	}
}
