package retrygovernance

import (
	"context"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
)

type selectedOutboxView struct {
	orgIDs []int64
	counts app.OutboxGovernanceSummary
	items  []app.RetryCandidate
}

func (v *selectedOutboxView) ReadOutboxGovernance(_ context.Context, orgID int64) (app.OutboxGovernanceSummary, error) {
	v.orgIDs = append(v.orgIDs, orgID)
	return v.counts, nil
}

func (v *selectedOutboxView) ListOutboxCandidates(_ context.Context, orgID int64, _ int) ([]app.RetryCandidate, error) {
	v.orgIDs = append(v.orgIDs, orgID)
	return v.items, nil
}

func TestSelectedStandardOutboxesReplaceHistoricalGovernanceQueries(t *testing.T) {
	mysql := &selectedOutboxView{
		counts: app.OutboxGovernanceSummary{Automatic: 2, ManualRequired: 3, Authorized: 1, Terminal: 4, BlockedRetryEvents: 1},
		items:  []app.RetryCandidate{{Kind: "outbox", Store: "assessment-mysql-outbox", ResourceID: "mysql-event"}},
	}
	mongo := &selectedOutboxView{
		counts: app.OutboxGovernanceSummary{Automatic: 5, ManualRequired: 6, Authorized: 2, Terminal: 7, BlockedRetryEvents: 2},
		items:  []app.RetryCandidate{{Kind: "outbox", Store: "mongo-domain-events", ResourceID: "mongo-event"}},
	}
	// No legacy database is configured. Any accidental old-table query fails
	// rather than silently adding historical mock counts to selected profiles.
	reader := NewReader(nil, nil).WithStandardOutboxes(map[string]app.OutboxGovernanceReader{
		"assessment-mysql-outbox": mysql, "mongo-domain-events": mongo,
	})
	var summary app.RetryGovernanceSummary
	for _, name := range []string{"assessment-mysql-outbox", "mongo-domain-events"} {
		if err := reader.addOutboxGovernance(t.Context(), 7, name, &summary); err != nil {
			t.Fatal(err)
		}
	}
	if summary.OutboxAutomatic != 7 || summary.OutboxManual != 9 || summary.OutboxAuthorized != 3 ||
		summary.OutboxTerminal != 11 || summary.BlockedRetryEvents != 3 {
		t.Fatalf("selected standard summary lost a class: %+v", summary)
	}
	var items []app.RetryCandidate
	if err := reader.appendMySQLOutboxCandidates(t.Context(), 7, 10, &items); err != nil {
		t.Fatal(err)
	}
	if err := reader.appendMongoOutboxCandidates(t.Context(), 7, 10, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ResourceID != "mysql-event" || items[1].ResourceID != "mongo-event" ||
		len(mysql.orgIDs) != 2 || len(mongo.orgIDs) != 2 {
		t.Fatalf("selected views were not the only owners: items=%+v mysql=%v mongo=%v", items, mysql.orgIDs, mongo.orgIDs)
	}
}
