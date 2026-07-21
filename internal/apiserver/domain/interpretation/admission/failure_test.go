package admission

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestNewFailureRequiresIdentityAndSafeFields(t *testing.T) {
	_, err := NewFailure(Input{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestFingerprintPrefersEventID(t *testing.T) {
	got := Fingerprint("evt-1", meta.FromUint64(9), KindMapping, "mapping_failed")
	if got != "event:evt-1" {
		t.Fatalf("fingerprint = %q", got)
	}
	a := Fingerprint("", meta.FromUint64(9), KindMapping, "mapping_failed")
	b := Fingerprint("", meta.FromUint64(9), KindMapping, "mapping_failed")
	if a == "" || a != b {
		t.Fatalf("hash fingerprint unstable: %q vs %q", a, b)
	}
}

func TestNewFailureBuildsDurableEvidence(t *testing.T) {
	at := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	failure, err := NewFailure(Input{
		ID: meta.FromUint64(1), OutcomeID: meta.FromUint64(9), OrgID: 7, AssessmentID: meta.FromUint64(3),
		TesteeID: 42, EventID: "evt-1", TraceID: "trace-1", Kind: KindReportInputDecode,
		Code: "report_input_decode", SafeMessage: "冻结报告输入无法解码", Retryable: false, OccurredAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if failure.Fingerprint() != "event:evt-1" || failure.Kind() != KindReportInputDecode || failure.Retryable() {
		t.Fatalf("failure = %#v", failure)
	}
}
