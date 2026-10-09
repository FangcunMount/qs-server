package retirement

import (
	"encoding/json"
	"errors"
	"testing"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
)

func TestSQLHistoricalOwnerBatchWrapperRejectsUnboundCycleAndSource(t *testing.T) {
	request := sqlevaluation.SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}
	for _, snapshot := range []*SQLResponsibilitySnapshot{nil, {}, {cycle: &sqlevaluation.SQLHistoricalResponsibilityCycle{}}} {
		if batch, err := PrepareSQLBusinessOwnerBatch(t.Context(), snapshot, request, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits()); err == nil || batch != nil {
			t.Fatal("unbound current snapshot accepted")
		}
	}
	for _, batch := range []*SQLBusinessOwnerBatch{nil, {}} {
		if _, err := batch.ResolveUntrustedSQLSource(t.Context(), &DecodedSourceEvent{}); !errors.Is(err, ErrSQLOwnerResolution) {
			t.Fatal("caller-created batch/source acquired local resolution")
		}
		if batch.Report().DropReady || !batch.Report().SourceAuthenticationRequired {
			t.Fatal("zero batch acquired drop approval")
		}
	}
}

func TestSQLHistoricalOwnerBatchWrapperPrivateResolution(t *testing.T) {
	resolution := &SQLBatchOwnerResolution{local: SQLLocalResolution{Gaps: []string{"missing_original_run_identity"}, BlockingReasons: []string{"external_closure_required"}, SourceAuthenticationRequired: true, MongoDBResponsibilityRequired: true, GlobalUnboundResponsibilityCoverageRequired: true}}
	copy := resolution.Local()
	copy.Gaps[0] = "edited"
	copy.BlockingReasons[0] = "edited"
	if resolution.Local().Gaps[0] == "edited" || resolution.Local().BlockingReasons[0] == "edited" {
		t.Fatal("caller changed private local classification")
	}
	for _, value := range []any{resolution, &SQLBusinessOwnerBatch{}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private bounded local evidence serialized")
		}
	}
}
