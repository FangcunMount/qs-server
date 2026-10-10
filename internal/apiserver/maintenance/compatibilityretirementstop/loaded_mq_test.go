package compatibilityretirementstop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	reader "github.com/FangcunMount/qs-server/internal/pkg/runtimefactsreader"
)

func TestLoadedMQDiagnosticCannotClaimCompleteScope(t *testing.T) {
	good := LoadedMQDiagnostic{ObservationSHA256: strings.Repeat("a", 64), Workers: 1, Nodes: 2, Clients: 3, ExternalAIUnproven: true, SharedHandoffUnproven: true}
	if !loadedMQDiagnosticValid(&good) {
		t.Fatal("local incomplete observation rejected")
	}
	for name, mutate := range map[string]func(*LoadedMQDiagnostic){"full": func(d *LoadedMQDiagnostic) { d.BrokerScopeComplete = true }, "missing-external": func(d *LoadedMQDiagnostic) { d.ExternalAIUnproven = false }, "zero-worker": func(d *LoadedMQDiagnostic) { d.Workers = 0 }, "no-node": func(d *LoadedMQDiagnostic) { d.Nodes = 0 }, "negative-clients": func(d *LoadedMQDiagnostic) { d.Clients = -1 }, "bad-hash": func(d *LoadedMQDiagnostic) { d.ObservationSHA256 = "" }, "oversize": func(d *LoadedMQDiagnostic) { d.Nodes = 2049 }} {
		t.Run(name, func(t *testing.T) {
			d := good
			mutate(&d)
			if loadedMQDiagnosticValid(&d) {
				t.Fatal("ambiguous/complete scope accepted")
			}
		})
	}
	raw, e := json.Marshal(good)
	if e != nil || len(raw) > 1024 || strings.Contains(string(raw), "endpoint") || strings.Contains(string(raw), "client_id") {
		t.Fatal("wire is not bounded and body-free")
	}
}
func TestLoadedMQSummaryPreservesAllReaderGaps(t *testing.T) {
	makeObservation := func(gaps []string) reader.Observation {
		return reader.Observation{Snapshot: runtimefacts.Snapshot{Component: "worker", ObservationComplete: true}, Nodes: []reader.NodeObservation{{Clients: []reader.ClientObservation{{ClientID: "first"}, {ClientID: "second"}}}}, Gaps: gaps}
	}
	var d LoadedMQDiagnostic
	o := makeObservation([]string{"publisher_topic_history_not_exhaustive:publisher", "shared_handoff_connection_unbound:consumer", "external_qs_ai_not_locally_proven"})
	if accumulateLoadedMQ(&d, o) != nil || d.Workers != 1 || d.Nodes != 1 || d.Clients != 2 || !d.ExternalAIUnproven || !d.SharedHandoffUnproven || !d.PublisherHistoryUnproven || d.BrokerScopeComplete {
		t.Fatal("reader scope was silently promoted or omitted")
	}
	for _, gaps := range [][]string{nil, {"external_qs_ai_not_locally_proven", "new_unknown_gap"}, {"shared_handoff_connection_unbound:consumer"}} {
		var x LoadedMQDiagnostic
		if accumulateLoadedMQ(&x, makeObservation(gaps)) == nil {
			t.Fatal("unsupported/missing gap accepted")
		}
	}
	o.Snapshot.BrokerConnectionsVerified = true
	if accumulateLoadedMQ(&d, o) == nil {
		t.Fatal("complete broker boolean was accepted")
	}
}
func TestLoadedMQOriginalSessionEligibility(t *testing.T) {
	action := "observe_loaded_mq"
	if !serviceAction(action) || !controlledAction(action) || recoveryAction(action) {
		t.Fatal("scope is not a fixed forward read-only action")
	}
	for _, row := range []struct{ bound, stopped, refused, recovery, issued, resumed, want bool }{{true, true, false, false, true, true, true}, {false, true, false, false, true, true, false}, {true, false, false, false, true, true, false}, {true, true, false, false, true, false, false}, {true, true, true, false, true, true, false}, {true, true, false, true, true, true, false}} {
		if remoteSessionRequestAllowed(action, row.bound, row.stopped, row.refused, row.recovery, row.issued, row.resumed) != row.want {
			t.Fatal("native request gate changed")
		}
	}
	raw, _ := json.Marshal(SessionRequest{sessionProtocol, 4, action})
	if _, e := parseRemoteSessionRequest(raw, 4); e != nil {
		t.Fatal(e)
	}
	if _, e := parseSessionRequest(raw, 4); e == nil {
		t.Fatal("nonremote producer allowed D observation")
	}
}
func TestLoadedMQWireRequiresTheExplicitIncompleteObservation(t *testing.T) {
	mq := &LoadedMQDiagnostic{ObservationSHA256: strings.Repeat("a", 64), Workers: 1, Nodes: 1, ExternalAIUnproven: true}
	if !remoteLoadedMQDiagnosticValid(SessionDiagnostic{Action: "observe_loaded_mq", Outcome: "observed", LoadedMQ: mq}) {
		t.Fatal("actual incomplete diagnostic rejected")
	}
	for _, v := range []SessionDiagnostic{{Action: "check_running", Outcome: "observed", LoadedMQ: mq}, {Action: "observe_loaded_mq", Outcome: "refused", LoadedMQ: mq}, {Action: "observe_loaded_mq", Outcome: "observed"}} {
		if remoteLoadedMQDiagnosticValid(v) {
			t.Fatal("wrong/missing action observation accepted")
		}
	}
	if sessionCategory(ErrLoadedMQ) != "loaded_mq_read_failed" || !remoteDiagnosticOutcomeValid(SessionDiagnostic{Action: "observe_loaded_mq", Outcome: "refused", ErrorCategory: "loaded_mq_read_failed"}) || remoteDiagnosticOutcomeValid(SessionDiagnostic{Action: "stop", Outcome: "refused", ErrorCategory: "loaded_mq_read_failed"}) {
		t.Fatal("native error was assigned to a different capability")
	}
	if _, e := (&Lease{}).ObserveLoadedMQ(context.Background(), nil); e == nil {
		t.Fatal("blank lease granted process ownership")
	}
	if _, e := (&RemoteController{}).ObserveLoadedMQ(context.Background()); e == nil {
		t.Fatal("blank controller recreated a live observation")
	}
}
