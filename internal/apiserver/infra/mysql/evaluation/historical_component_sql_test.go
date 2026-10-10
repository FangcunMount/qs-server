package evaluation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSQLHistoricalComponentClosedScopeAndNoImportedAuthority(t *testing.T) {
	s := SQLCrossStoreSelectors{EventIDs: []string{"exact-event"}, AssessmentIDs: []uint64{42}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}}}
	for _, spec := range sqlResponsibilityTables {
		q, args, err := componentPredicate(spec, s, s.EventIDs)
		if err != nil || q == "" {
			t.Fatal("fixed ledger selector missing", spec.name, err)
		}
		if spec.name == "rm_outbox" || spec.name == "retry_event_hold" || spec.name == "event_delivery_dead_letter" {
			if !strings.Contains(q, "FROM_BASE64") || !strings.Contains(q, "COALESCE(JSON_VALID") || !strings.Contains(q, "$.data.assessment_id") || len(args) < 4 {
				t.Fatal("actual revision/invalid/owner selectors omitted", spec.name)
			}
			if strings.Contains(q, "org_id IN") || strings.Contains(q, "deleted_at") {
				t.Fatal("organization or deletion hides contradictory owner")
			}
		}
	}
	r := &SQLHistoricalComponentRecipe{}
	if json.Unmarshal([]byte(`{}`), r) == nil {
		t.Fatal("editable recipe imported")
	}
	if o, err := PrepareSQLHistoricalComponentObservation(context.Background(), r, time.Second, false); err == nil || o != nil {
		t.Fatal("forged input creates native observation")
	}
	if o, err := PrepareSQLHistoricalComponentObservation(context.Background(), r, 21*time.Second, true); err == nil || o != nil {
		t.Fatal("oversized epoch accepted")
	}
	o := &SQLHistoricalComponentObservation{}
	o.self = o
	if o.Report().BusinessMatched || o.Report().DropReady {
		t.Fatal("empty seal called successful")
	}
	if _, err := o.apply(context.Background()); err == nil {
		t.Fatal("empty observation writes")
	}
	if _, err := (&SQLHistoricalComponentStatement{}).VerifyIndependentPersisted(context.Background(), time.Second); err == nil {
		t.Fatal("forged statement readback accepted")
	}
	copied := componentCopySelectors(s)
	copied.EventIDs[0] = "changed"
	copied.AssessmentIDs[0] = 9
	copied.MongoOwners[0].ID = "other"
	if s.EventIDs[0] != "exact-event" || s.AssessmentIDs[0] != 42 || s.MongoOwners[0].ID != "10042" {
		t.Fatal("recipe selectors share editable source slice")
	}
}
