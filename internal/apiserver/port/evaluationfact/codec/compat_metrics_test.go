package codec

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestDecodeExecutionObservesSchemaVersion(t *testing.T) {
	t.Parallel()

	before0 := testutil.ToFloat64(outcomeSchemaDecodeTotal.WithLabelValues("0"))
	before1 := testutil.ToFloat64(outcomeSchemaDecodeTotal.WithLabelValues("1"))
	before2 := testutil.ToFloat64(outcomeSchemaDecodeTotal.WithLabelValues("2"))

	mustDecode := func(schema uint, payload string) {
		t.Helper()
		record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
			ID: meta.FromUint64(1), AssessmentID: meta.FromUint64(2), TesteeID: 3, RunID: "2:1",
			Model:         evaluationfact.ModelIdentity{Kind: modelcatalog.KindScale, Code: "SDS", Version: "1.0.0"},
			Runtime:       evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring},
			SchemaVersion: schema, EvaluatedAt: time.Unix(100, 0), Payload: []byte(payload),
		})
		if _, err := DecodeExecution(record); err != nil {
			t.Fatalf("schema %d: %v", schema, err)
		}
	}

	mustDecode(0, `{"Dimensions":[{"Code":"total","Score":{"Value":1}}]}`)
	mustDecode(1, `{"Dimensions":[{"Code":"total","Score":{"Value":1}}]}`)
	mustDecode(2, `{"Dimensions":[{"Code":"total","Score":{"Value":1}}]}`)

	if delta := testutil.ToFloat64(outcomeSchemaDecodeTotal.WithLabelValues("0")) - before0; delta != 1 {
		t.Fatalf("schema 0 delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(outcomeSchemaDecodeTotal.WithLabelValues("1")) - before1; delta != 1 {
		t.Fatalf("schema 1 delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(outcomeSchemaDecodeTotal.WithLabelValues("2")) - before2; delta != 1 {
		t.Fatalf("schema 2 delta = %v, want 1", delta)
	}
}

func TestDecodeExecutionDoesNotObserveUnsupportedSchema(t *testing.T) {
	t.Parallel()
	before := testutil.ToFloat64(outcomeSchemaDecodeTotal.WithLabelValues("99"))
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(1), SchemaVersion: 99, Payload: []byte(`{}`),
	})
	if _, err := DecodeExecution(record); err == nil {
		t.Fatal("expected unsupported schema error")
	}
	after := testutil.ToFloat64(outcomeSchemaDecodeTotal.WithLabelValues("99"))
	if after != before {
		t.Fatalf("unsupported schema must not increment metrics: before=%v after=%v", before, after)
	}
}
