package migration

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
)

func TestOfflineProtocolUnknownOrUnresolvedIsNeverZero(t *testing.T) {
	base := transaction.ProtocolObservation{AsOf: time.Now().UTC()}
	if zero, err := unresolvedProtocol(base); err != nil || !zero {
		t.Fatal(zero, err)
	}
	for _, change := range []func(*transaction.ProtocolObservation){
		func(p *transaction.ProtocolObservation) { p.ReservedExecutions = 1 },
		func(p *transaction.ProtocolObservation) { p.PreparingExecutions = 1 },
		func(p *transaction.ProtocolObservation) { p.ConfirmationGapBranches = 1 },
		func(p *transaction.ProtocolObservation) { p.UnagedConfirmationGapBranches = 1 },
		func(p *transaction.ProtocolObservation) { p.UntrackedAbortedBranches = 1 },
		func(p *transaction.ProtocolObservation) { p.UnobservedCommittedBranches = 1 },
		func(p *transaction.ProtocolObservation) { p.MetadataConflicts = 1 },
		func(p *transaction.ProtocolObservation) { p.PreparedExpiryAgeSeconds = 1 },
		func(p *transaction.ProtocolObservation) { p.OldestConfirmationObservationAgeSeconds = 1 },
		func(p *transaction.ProtocolObservation) { p.AsOf = time.Time{} },
		func(p *transaction.ProtocolObservation) { p.MetadataConflicts = -1 },
		func(p *transaction.ProtocolObservation) { p.PreparedExpiryAgeSeconds = math.NaN() },
	} {
		p := base
		change(&p)
		if zero, _ := unresolvedProtocol(p); zero {
			t.Fatal("unknown or unresolved observation accepted", p)
		}
	}
	r, err := ObserveOfflineProtocol(context.Background(), nil, nil, PairConfig{})
	if err == nil || r.Available || r.NoObservedUnresolved || r.MySQL != nil || r.Mongo != nil || !r.ReadOnly || r.Mutation || r.RuntimeFrozen || r.BrokerPendingKnown {
		t.Fatal("missing metadata inferred as available", r, err)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]any
	if json.Unmarshal(raw, &public) != nil || public["qs_mysql"] != nil || public["qs_mongo"] != nil {
		t.Fatal("unavailable owner must remain null")
	}
}
