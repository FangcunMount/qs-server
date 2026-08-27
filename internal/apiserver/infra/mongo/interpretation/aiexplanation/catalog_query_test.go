package aiexplanation

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestPromptEvaluationCatalogProjectionCannotRepresentProviderEvidence(t *testing.T) {
	for _, candidate := range []reflect.Type{
		reflect.TypeOf(promptEvaluationCatalogPO{}),
		reflect.TypeOf(promptEvaluationCatalogAttemptPO{}),
		reflect.TypeOf(promptEvaluationCatalogReviewPO{}),
	} {
		for _, forbidden := range []string{
			"RawOutput", "NormalizedOutput", "ProviderReceipt", "Assertions", "Semantic", "Reviewer", "Reason",
		} {
			if _, exists := candidate.FieldByName(forbidden); exists {
				t.Fatalf("catalog projection %s unexpectedly contains %s", candidate.Name(), forbidden)
			}
		}
	}
}

func TestAdministrationCatalogCursorRoundTripBindsQueryIdentity(t *testing.T) {
	want := administrationCatalogCursor{
		Version:   administrationCatalogCursorVersion,
		Kind:      evaluationCatalogCursorKind,
		OrgID:     17,
		Status:    "awaiting_review",
		CreatedAt: time.Date(2026, time.August, 27, 3, 4, 5, 600, time.UTC),
		DomainID:  meta.ID(91),
	}
	raw, err := encodeAdministrationCatalogCursor(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeAdministrationCatalogCursor(raw, want.Kind, want.OrgID, want.Status)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != want.Version || got.Kind != want.Kind || got.OrgID != want.OrgID || got.Status != want.Status ||
		!got.CreatedAt.Equal(want.CreatedAt) || got.DomainID != want.DomainID {
		t.Fatalf("cursor = %#v, want %#v", got, want)
	}

	for name, query := range map[string]struct {
		kind   string
		orgID  int64
		status string
	}{
		"resource": {kind: profileCatalogCursorKind, orgID: want.OrgID, status: want.Status},
		"org":      {kind: want.Kind, orgID: want.OrgID + 1, status: want.Status},
		"status":   {kind: want.Kind, orgID: want.OrgID, status: "approved"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeAdministrationCatalogCursor(raw, query.kind, query.orgID, query.status); err == nil {
				t.Fatal("cursor must not be reusable for a different governance query")
			}
		})
	}
	if _, err := decodeAdministrationCatalogCursor("not-base64", want.Kind, want.OrgID, want.Status); err == nil {
		t.Fatal("malformed cursor must be rejected")
	}
}

func TestApplyAdministrationKeysetUsesCreatedAtAndDomainIDTieBreaker(t *testing.T) {
	createdAt := time.Date(2026, time.August, 27, 3, 4, 5, 0, time.UTC)
	filter := bson.M{"requested_org_id": int64(17)}
	applyAdministrationKeyset(filter, administrationCatalogCursor{CreatedAt: createdAt, DomainID: meta.ID(91)})

	clauses, ok := filter["$or"].(bson.A)
	if !ok || len(clauses) != 2 {
		t.Fatalf("keyset clauses = %#v", filter["$or"])
	}
	older, ok := clauses[0].(bson.M)
	if !ok || older["created_at"] == nil {
		t.Fatalf("older clause = %#v", clauses[0])
	}
	tie, ok := clauses[1].(bson.M)
	if !ok || tie["created_at"] != createdAt || tie["domain_id"] == nil {
		t.Fatalf("tie-breaker clause = %#v", clauses[1])
	}
	if filter["requested_org_id"] != int64(17) {
		t.Fatalf("organization filter changed: %#v", filter)
	}
}
