package answersheet

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAnswerSheetMapperPreservesSubmissionContext(t *testing.T) {
	t.Parallel()

	sheet := newMapperSubmittedSheet(t)
	po := NewAnswerSheetMapper().ToPO(sheet)
	if po.DomainID != sheet.ID() {
		t.Fatalf("DomainID = %s, want %s", po.DomainID, sheet.ID())
	}
	if po.TesteeID != 401 || po.OrgID != 501 || po.TaskID != "task-1" {
		t.Fatalf("submission context fields = testee:%d org:%d task:%q", po.TesteeID, po.OrgID, po.TaskID)
	}

	restored := NewAnswerSheetMapper().ToBO(po)
	if restored == nil {
		t.Fatal("ToBO() returned nil")
	}
	ctx := restored.SubmissionContext()
	if ctx.TesteeID().Uint64() != 401 || ctx.OrgID().Uint64() != 501 || ctx.TaskID() != "task-1" {
		t.Fatalf("restored submission context = %+v", ctx)
	}
}

func TestAnswerSheetPOBeforeInsertPreservesPreassignedDomainID(t *testing.T) {
	t.Parallel()

	po := &AnswerSheetPO{}
	po.DomainID = meta.FromUint64(1001)
	po.BeforeInsert()
	if po.DomainID != meta.FromUint64(1001) {
		t.Fatalf("DomainID = %s, want preassigned 1001", po.DomainID)
	}
}

func newMapperSubmittedSheet(t *testing.T) *domainAnswerSheet.AnswerSheet {
	t.Helper()
	ref, err := domainAnswerSheet.NewQuestionnaireRef("QNR-1", "1.0.0", "Questionnaire")
	if err != nil {
		t.Fatalf("NewQuestionnaireRef() error = %v", err)
	}
	ctx, err := domainAnswerSheet.NewSubmissionContext(
		actor.NewFillerRef(301, actor.FillerTypeSelf),
		actor.NewTesteeRef(meta.FromUint64(401)),
		meta.FromUint64(501),
		"task-1",
	)
	if err != nil {
		t.Fatalf("NewSubmissionContext() error = %v", err)
	}
	answer, err := domainAnswerSheet.NewAnswer(
		meta.NewCode("Q1"),
		domainQuestionnaire.TypeText,
		domainAnswerSheet.NewStringValue("hello"),
		0,
	)
	if err != nil {
		t.Fatalf("NewAnswer() error = %v", err)
	}
	sheet, err := domainAnswerSheet.Submit(
		meta.FromUint64(1001),
		ref,
		ctx,
		[]domainAnswerSheet.Answer{answer},
		time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	return sheet
}
