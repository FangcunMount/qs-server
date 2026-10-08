package eventevidencebinding

import (
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"math"
	"testing"
	"time"
)

func TestAnswerSheetBindingSurvivesBSONPrecisionButDetectsTraceAndOwnership(t *testing.T) {
	p := eventpayload.AnswerSheetSubmittedData{AnswerSheetID: "1", OrgID: 2, TesteeID: 3, SubmittedAt: time.Date(2026, 10, 8, 1, 2, 3, 123456789, time.FixedZone("offset", 8*3600)), RequestID: "original"}
	original, err := AnswerSheet(p)
	if err != nil {
		t.Fatal(err)
	}
	p.SubmittedAt = p.SubmittedAt.UTC().Truncate(time.Millisecond)
	if got, _ := AnswerSheet(p); got != original {
		t.Fatal("BSON millisecond storage changed business binding")
	}
	p.RequestID = "other"
	if got, _ := AnswerSheet(p); got == original {
		t.Fatal("request trace change was missed")
	}
	p.RequestID = "original"
	p.OrgID++
	if got, _ := AnswerSheet(p); got == original {
		t.Fatal("cross-org change was missed")
	}
}
func TestGeneratedBindingDistinguishesOptionalValueAndRejectsNonfinite(t *testing.T) {
	p := eventoutcome.ReportGeneratedPayload{OrgID: 1, GenerationID: "1", RunID: "2", ReportID: "3", AssessmentID: "4", OutcomeID: "5", TesteeID: 6, GeneratedAt: time.Now()}
	original, err := Generated(p)
	if err != nil {
		t.Fatal(err)
	}
	p.PrimaryScore = &eventoutcome.ScoreValue{}
	if got, _ := Generated(p); got == original {
		t.Fatal("optional score absence and zero conflated")
	}
	p.PrimaryScore.Value = math.NaN()
	if _, err := Generated(p); err == nil {
		t.Fatal("NaN accepted")
	}
}
func TestRetryBindingPinsOriginalBusinessScheduleAndAction(t *testing.T) {
	p := eventoutcome.InterpretationRetryRequestedPayload{OrgID: 1, GenerationID: "2", RunID: "3", AssessmentID: "4", OutcomeID: "5", TesteeID: 6, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: time.Now()}
	original, err := Retry(p)
	if err != nil {
		t.Fatal(err)
	}
	p.RequestedAt = p.RequestedAt.Add(time.Second)
	if got, _ := Retry(p); got == original {
		t.Fatal("original schedule change was missed")
	}
	p.RequestedAt = p.RequestedAt.Add(-time.Second)
	p.ExpectedAttempt++
	if got, _ := Retry(p); got == original {
		t.Fatal("attempt budget identity conflated")
	}
}
