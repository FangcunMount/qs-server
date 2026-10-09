package evaluation

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSQLCrossStoreOpaqueAndResourceBounds(t *testing.T) {
	for _, value := range []any{&SQLHistoricalCrossStoreCatalog{}, &SQLHistoricalCrossStorePage{}, SQLCrossStoreRow{}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSQLHistoricalFactsSerialization) {
			t.Fatal("private row/catalog escaped JSON")
		}
		if _, err := bson.Marshal(value); !errors.Is(err, ErrSQLHistoricalFactsSerialization) {
			t.Fatal("private row/catalog escaped BSON")
		}
	}
	if !DefaultSQLCrossStoreLimits().valid() {
		t.Fatal("default limits invalid")
	}
	for _, edit := range []func(*SQLCrossStoreLimits){func(l *SQLCrossStoreLimits) { l.PageRows = 0 }, func(l *SQLCrossStoreLimits) { l.MaxPageRows = 131073 }, func(l *SQLCrossStoreLimits) { l.MaxKeys = 10_000_001 }, func(l *SQLCrossStoreLimits) { l.MaxKeyRetainedBytes = 0 }, func(l *SQLCrossStoreLimits) { l.MaxPageBytes = 0 }, func(l *SQLCrossStoreLimits) { l.MaxDuration = 0 }} {
		l := DefaultSQLCrossStoreLimits()
		edit(&l)
		if l.valid() {
			t.Fatal("unbounded/truncated scan accepted")
		}
	}
}

func crossUnitReplayPage(t *testing.T) *SQLHistoricalCrossStorePage {
	t.Helper()
	request := standard.ReplayRequest{OrgID: 7, RequestID: "original-request", Store: "mongo-domain-events", Reason: "original ordered replay", Targets: []standard.ReplayTarget{{EventID: "original-a", ExpectedFailureCount: 3}, {EventID: "original-b", ExpectedFailureCount: 4}}}
	hash, err := request.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	header := cycleTestRow(map[string]string{"org_id": "7", "request_id": request.RequestID, "store_name": request.Store, "reason": request.Reason, "input_hash": string(hash[:])})
	a := cycleTestRow(map[string]string{"org_id": "7", "request_id": request.RequestID, "ordinal": "0", "event_id": "original-a", "expected_failure_count": "3", "authorized": "0", "reason": "not_found"})
	b := cycleTestRow(map[string]string{"org_id": "7", "request_id": request.RequestID, "ordinal": "1", "event_id": "original-b", "expected_failure_count": "4", "authorized": "1", "reason": ""})
	key := cyclePair(7, request.RequestID)
	c := &SQLHistoricalCrossStoreCatalog{cycle: &SQLHistoricalResponsibilityCycle{observations: []SQLResponsibilityObservation{cycleDecode("qs_rm_replay_requests", header), cycleDecode("qs_rm_replay_items", a), cycleDecode("qs_rm_replay_items", b)}}, requestParents: map[string][]int{key: {0}}, byRequest: map[string][]int{key: {1, 2}}}
	return &SQLHistoricalCrossStorePage{catalog: c, replayPairs: map[string]bool{key: true}, rows: map[int]historicalSQLRow{0: header, 1: a, 2: b}, facts: map[int]SQLCrossStoreRow{0: {}, 1: {}, 2: {}}}
}

func TestSQLCrossStoreReplayFullInputAndOrder(t *testing.T) {
	for _, name := range []string{"valid", "wrong_input_hash", "wrong_store", "ordinal_gap", "duplicate_event", "missing_item", "missing_parent", "reason_drift"} {
		t.Run(name, func(t *testing.T) {
			p := crossUnitReplayPage(t)
			switch name {
			case "wrong_input_hash":
				p.rows[0]["input_hash"] = strptr(string(make([]byte, 32)))
			case "wrong_store":
				p.rows[0]["store_name"] = strptr("assessment-mysql-outbox")
			case "ordinal_gap":
				p.rows[2]["ordinal"] = strptr("2")
			case "duplicate_event":
				p.rows[2]["event_id"] = strptr("original-a")
			case "missing_item":
				delete(p.rows, 2)
			case "missing_parent":
				delete(p.rows, 0)
			case "reason_drift":
				p.rows[0]["reason"] = strptr("edited after original replay")
			}
			err := p.bindReplay()
			if name == "valid" {
				if err != nil || p.facts[1].Replay == nil || !p.facts[1].Replay.FingerprintVerified || len(p.facts[1].Replay.Items) != 2 || len(p.facts[1].Replay.InputSHA256) != hex.EncodedLen(32) {
					t.Fatal("whole original fingerprint not verified", err)
				}
			} else if err == nil && p.facts[1].Replay.FingerprintVerified {
				t.Fatal("drifted/incomplete full replay input accepted")
			}
		})
	}
}

func strptr(s string) *string { return &s }
