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

func TestSQLHistoricalOriginalRunCandidatesAreNotGlobalAbsence(t *testing.T) {
	text := func(v string) *string { return &v }
	base := func() map[string][]historicalSQLRow {
		return map[string][]historicalSQLRow{"evaluation_outcome": {{"id": text("9001"), "evaluation_run_id": text("42:1")}}, "runtime_checkpoint": {}}
	}
	for _, tc := range []struct {
		name    string
		change  func(map[string][]historicalSQLRow)
		want    int
		invalid bool
	}{
		{"candidate only", func(map[string][]historicalSQLRow) {}, 1, false},
		{"other retained Run", func(rows map[string][]historicalSQLRow) {
			rows["runtime_checkpoint"] = []historicalSQLRow{{"resource_id": text("42:2")}}
		}, 1, false},
		{"original retained different scope deleted", func(rows map[string][]historicalSQLRow) {
			rows["runtime_checkpoint"] = []historicalSQLRow{{"resource_id": text("42:1"), "scope": text("other_scope"), "deleted_at": text("2026-10-09 00:00:00")}}
		}, 0, false},
		{"duplicate outcome", func(rows map[string][]historicalSQLRow) {
			rows["evaluation_outcome"] = append(rows["evaluation_outcome"], rows["evaluation_outcome"][0])
		}, 0, true},
		{"missing declared identity", func(rows map[string][]historicalSQLRow) { rows["evaluation_outcome"][0]["evaluation_run_id"] = nil }, 0, true},
		{"bounded owners", func(rows map[string][]historicalSQLRow) {
			rows["evaluation_outcome"] = append(rows["evaluation_outcome"], historicalSQLRow{"id": text("9002"), "evaluation_run_id": text("43:1")})
		}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := base()
			tc.change(rows)
			got, err := batchOriginalRunCandidates(rows, 1)
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid declaration selected")
				}
				return
			}
			if err != nil || len(got) != tc.want || tc.want == 1 && got[9001] != "42:1" {
				t.Fatal("original declaration lost or an unrelated Run substituted", err)
			}
		})
	}
}

func TestSQLHistoricalOriginalRunAbsenceRejectsCallerConstructedBatch(t *testing.T) {
	for _, facts := range []*SQLHistoricalBatchOwnerFacts{nil, {}, {batch: &SQLHistoricalOwnerBatch{report: SQLHistoricalOwnerBatchReport{Complete: true}, originalOutcomeRunAbsence: map[uint64]string{9001: "42:1"}}, id: 42}} {
		if err := facts.OriginalOutcomeRunAbsent(t.Context(), 9001, "42:1"); err == nil {
			t.Fatal("editable completion/cache stood in for an actual RR-RO epoch")
		}
	}
}
