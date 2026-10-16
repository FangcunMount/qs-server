package assessmentintake

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type missingPersistedSubmission struct {
	reads []meta.ID
}

func (r *missingPersistedSubmission) FindByID(_ context.Context, id meta.ID) (*domainanswersheet.AnswerSheet, error) {
	r.reads = append(r.reads, id)
	// The Mongo repository returns nil, nil when the durable sheet is absent.
	return nil, nil
}

func TestEnsureMissingPersistedAnswerSheetDoesNotWriteBusinessFacts(t *testing.T) {
	calls := []string{}
	intake := &intakeStub{
		calls:   &calls,
		created: &evaluationintake.Assessment{ID: 91, Status: "pending"},
	}
	submissions := &missingPersistedSubmission{}
	svc := NewService(scoringStub{calls: &calls}, boundScaleBinding(), nil, nil, intake, nil, submissions)
	command := Command{OrgID: 9, AnswerSheetID: 3, QuestionnaireCode: "Q", QuestionnaireVersion: "1", TesteeID: 7, FillerID: 8}

	// Repeating the original command must not recreate an Assessment after its
	// durable parent has been removed, even with a valid live model binding.
	for attempt := 0; attempt < 2; attempt++ {
		result, err := svc.Ensure(context.Background(), command)
		if result != nil || !cberrors.IsCode(err, code.ErrDatabase) {
			t.Fatalf("attempt %d: result=%+v error=%v, want missing durable submission", attempt, result, err)
		}
	}
	if len(submissions.reads) != 2 || submissions.reads[0] != meta.FromUint64(command.AnswerSheetID) || submissions.reads[1] != submissions.reads[0] {
		t.Fatalf("durable parent reads=%v, want original AnswerSheet twice", submissions.reads)
	}
	if len(calls) != 0 || intake.submitted {
		t.Fatalf("missing parent reached scoring/intake writes: calls=%v submitted=%t", calls, intake.submitted)
	}
}
