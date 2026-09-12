package answersheet

import (
	"context"
	"errors"
	start "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answeringstart"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"testing"
	"time"
)

type startResolverStub struct {
	value  sheet.StartContext
	err    error
	intent start.Intent
	called int
}

func (r *startResolverStub) ResolveSubmission(_ context.Context, _ uint64, i start.Intent) (sheet.StartContext, error) {
	r.intent = i
	r.called++
	return r.value, r.err
}
func TestResolveStartDoesNotDowngradeReference(t *testing.T) {
	s := &submissionService{}
	dto := SubmitAnswerSheetDTO{OrgID: 1, FillerID: 2, TesteeID: 3, QuestionnaireCode: "Q", QuestionnaireVer: "1"}
	value, err := s.resolveStart(context.Background(), dto, sheet.Admission{})
	if err != nil || !value.IsZero() {
		t.Fatal("legacy request changed")
	}
	dto.AnsweringStartID = 99
	if _, err := s.resolveStart(context.Background(), dto, sheet.Admission{}); err == nil {
		t.Fatal("unconfigured resolver accepted a reference")
	}
	r := &startResolverStub{err: errors.New("missing start")}
	s.startResolver = r
	if _, err := s.resolveStart(context.Background(), dto, sheet.Admission{}); err == nil {
		t.Fatal("missing start downgraded")
	}
	r.err = nil
	if _, err := s.resolveStart(context.Background(), dto, sheet.Admission{}); err == nil {
		t.Fatal("empty context accepted")
	}
	store := uint64(10)
	r.value, _ = sheet.NewStartContext(99, time.Now(), &store, 2)
	value, err = s.resolveStart(context.Background(), dto, sheet.Admission{})
	if err != nil || *value.StoreID() != 10 || r.intent.UserID != 2 || r.intent.OrgID != 1 || r.intent.TesteeID != 3 || r.intent.Origin.Type != sheet.OriginTypeSelfService {
		t.Fatalf("resolve: %+v %v", r.intent, err)
	}
}
