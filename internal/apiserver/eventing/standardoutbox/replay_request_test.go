package standardoutbox

import "testing"

func TestReplayRequestFingerprintBindsFullInputAndOrder(t *testing.T) {
	original := ReplayRequest{
		OrgID: 7, RequestID: "request-1", Store: "assessment-mysql-outbox", Reason: "operator reviewed",
		Targets: []ReplayTarget{{EventID: "event-a", ExpectedFailureCount: 30}, {EventID: "event-b", ExpectedFailureCount: 31}},
	}
	first, err := original.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	again, err := original.Fingerprint()
	if err != nil || again != first {
		t.Fatalf("same request changed identity: hash_equal=%t err=%v", again == first, err)
	}
	variants := []ReplayRequest{
		{OrgID: 8, RequestID: original.RequestID, Store: original.Store, Reason: original.Reason, Targets: original.Targets},
		{OrgID: original.OrgID, RequestID: original.RequestID, Store: "mongo-domain-events", Reason: original.Reason, Targets: original.Targets},
		{OrgID: original.OrgID, RequestID: original.RequestID, Store: original.Store, Reason: "changed", Targets: original.Targets},
		{OrgID: original.OrgID, RequestID: original.RequestID, Store: original.Store, Reason: original.Reason, Targets: []ReplayTarget{original.Targets[1], original.Targets[0]}},
		{OrgID: original.OrgID, RequestID: original.RequestID, Store: original.Store, Reason: original.Reason, Targets: []ReplayTarget{{EventID: "event-a", ExpectedFailureCount: 29}, original.Targets[1]}},
	}
	for i, variant := range variants {
		hash, err := variant.Fingerprint()
		if err != nil || hash == first {
			t.Fatalf("variant %d did not bind complete input: equal=%t err=%v", i, hash == first, err)
		}
	}
}

func TestReplayRequestRejectsAmbiguousOrEmptyTargets(t *testing.T) {
	base := ReplayRequest{OrgID: 7, RequestID: "request-1", Store: "assessment-mysql-outbox", Reason: "operator reviewed"}
	for _, targets := range [][]ReplayTarget{
		nil,
		{{EventID: "event-a", ExpectedFailureCount: 0}},
		{{EventID: "event-a", ExpectedFailureCount: 30}, {EventID: "event-a", ExpectedFailureCount: 30}},
	} {
		base.Targets = targets
		if _, err := base.Fingerprint(); err == nil {
			t.Fatalf("ambiguous targets accepted: %+v", targets)
		}
	}
}
