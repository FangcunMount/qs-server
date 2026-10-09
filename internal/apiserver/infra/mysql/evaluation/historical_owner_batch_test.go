package evaluation

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSQLHistoricalOwnerBatchRejectsEditableCapabilities(t *testing.T) {
	if _, err := PrepareSQLHistoricalOwnerBatch(t.Context(), &SQLHistoricalResponsibilityCycle{}, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, DefaultSQLHistoricalOwnerBatchLimits()); !errors.Is(err, ErrSQLHistoricalBatchInvalid) {
		t.Fatal("zero caller-constructed cycle accepted")
	}
	for _, ids := range [][]uint64{{0}, {42, 42}, {7, 0}} {
		if _, err := batchNormalizeIDs(ids); !errors.Is(err, ErrSQLHistoricalBatchInvalid) {
			t.Fatal("invalid owner page accepted")
		}
	}
	input := []uint64{43, 42}
	copy, err := batchNormalizeIDs(input)
	if err != nil || copy[0] != 42 || input[0] != 43 {
		t.Fatal("input page was not detached and canonicalized")
	}
	for _, value := range []any{&SQLHistoricalOwnerBatch{}, &SQLHistoricalBatchOwnerFacts{}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSQLHistoricalFactsSerialization) {
			t.Fatal("private owner page serialized")
		}
	}
}

func TestSQLHistoricalOwnerBatchDetachedSnapshotAndAssociation(t *testing.T) {
	facts := &SQLHistoricalOwnerFacts{assessmentID: 42, snapshot: SQLHistoricalFactsSnapshot{Owner: SQLHistoricalOwner{AssessmentID: 42, OrgID: 7, TesteeID: 21, AnswerSheetID: 10042, ConductingContextBytes: []byte("private raw context")}}}
	b := &SQLHistoricalOwnerBatch{owners: map[uint64]*SQLHistoricalOwnerFacts{42: facts}, sheets: map[uint64]uint64{10042: 42}, request: SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}, AnswerSheetIDs: []uint64{10042}}, report: SQLHistoricalOwnerBatchReport{Complete: true}}
	point, err := b.OwnerByAssessment(42)
	if err != nil || point.HasVerifiedAnswerSheetAssociation(10042) {
		t.Fatal("ID reader promoted to unique AnswerSheet capability")
	}
	sheet, err := b.OwnerByAnswerSheet(10042)
	if err != nil || !sheet.HasVerifiedAnswerSheetAssociation(10042) || sheet.HasVerifiedAnswerSheetAssociation(10043) {
		t.Fatal("actual selected association was lost or widened")
	}
	copy := sheet.Snapshot()
	copy.Owner.ConductingContextBytes[0] = '['
	copy.Owner.AssessmentID = 43
	if sheet.Snapshot().Owner.AssessmentID != 42 || sheet.Snapshot().Owner.ConductingContextBytes[0] == '[' {
		t.Fatal("editable snapshot changed opaque owner")
	}
	if _, err = b.OwnerByAssessment(43); !errors.Is(err, ErrSQLHistoricalBatchInvalid) {
		t.Fatal("unselected owner accepted")
	}
	if b.Report().DropReady || !b.Report().SourceAuthenticationRequired || !b.Report().CASRequired {
		t.Fatal("local facts acquired retirement approval")
	}
}
