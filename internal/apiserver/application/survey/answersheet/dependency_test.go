package answersheet

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	questionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestMissingAdmissionResolverCannotAcceptUnclassifiedSubmission(t *testing.T) {
	service := &submissionService{}
	admission, err := service.resolveAdmission(context.Background(), "Q", "1")
	if !errors.IsCode(err, code.ErrModuleInitializationFailed) || !admission.IsZero() {
		t.Fatalf("missing binding must fail before accepting: admission=%#v err=%v", admission, err)
	}
}

func TestMissingSubmissionAdmissionPortsNeverCallDurableWriter(t *testing.T) {
	for _, missing := range []string{"binding", "attribution"} {
		t.Run(missing, func(t *testing.T) {
			store := &durableStoreCaptureStub{}
			service := &submissionService{durableStore: store, binding: admissionBindingStub{}, attribution: frozenAttributionStub{}}
			if missing == "binding" {
				service.binding = nil
			} else {
				service.attribution = nil
			}
			q, err := questionnaire.NewQuestionnaire(meta.NewCode("Q"), "Questionnaire")
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.createAndSaveAnswerSheet(t.Context(), logger.L(t.Context()), SubmitAnswerSheetDTO{
				QuestionnaireCode: "Q", QuestionnaireVer: "1", OrgID: 1, FillerID: 2, TesteeID: 3,
			}, q, mustAnswersForSubmissionTest(t))
			if !errors.IsCode(err, code.ErrModuleInitializationFailed) || store.lastSheet != nil {
				t.Fatalf("missing %s accepted a durable submission: err=%v sheet=%v", missing, err, store.lastSheet)
			}
		})
	}
}

type readinessSheetRepo struct{ domain.Repository }
type readinessQuestionnaireRepo struct{ questionnaire.Repository }
type readinessSheetReader struct {
	surveyreadmodel.AnswerSheetReader
}

func TestSubmissionReadinessRequiresCompletePortsAfterComposition(t *testing.T) {
	ready := func() *submissionService {
		return &submissionService{repo: readinessSheetRepo{}, reader: readinessSheetReader{},
			questionnaireRepo: readinessQuestionnaireRepo{}, durableStore: &durableStoreCaptureStub{},
			binding: admissionBindingStub{}, attribution: frozenAttributionStub{}, startResolver: &startResolverStub{},
		}
	}
	for name, remove := range map[string]func(*submissionService){
		"repository":      func(s *submissionService) { s.repo = nil },
		"reader":          func(s *submissionService) { s.reader = nil },
		"questionnaire":   func(s *submissionService) { s.questionnaireRepo = nil },
		"durable writer":  func(s *submissionService) { s.durableStore = nil },
		"binding":         func(s *submissionService) { s.binding = nil },
		"attribution":     func(s *submissionService) { s.attribution = nil },
		"answering start": func(s *submissionService) { s.startResolver = nil },
	} {
		t.Run(name, func(t *testing.T) {
			s := ready()
			remove(s)
			err := s.ValidateReadiness()
			if !errors.IsCode(err, code.ErrModuleInitializationFailed) {
				t.Fatalf("missing %s: %v", name, err)
			}
			dependency := map[string]string{
				"repository": "answer sheet repository", "reader": "answer sheet reader",
				"questionnaire": "questionnaire repository", "durable writer": "durable writer",
				"binding": "assessment binding resolver", "attribution": "attribution resolver",
				"answering start": "answering start resolver",
			}[name]
			if !strings.Contains(fmt.Sprintf("%-v", err), dependency+" is required") {
				t.Fatalf("missing %s must be individually identified: %v", dependency, err)
			}
		})
	}
	if err := ready().ValidateReadiness(); err != nil {
		t.Fatal(err)
	}
}

func TestMissingAttributionResolverCannotAcceptAnyOrigin(t *testing.T) {
	for _, origin := range []OriginRefDTO{{Type: "self_service"}, {Type: "plan_task", ID: "31"}, {Type: "assessment_entry", ID: "ae_entry"}} {
		t.Run(origin.Type, func(t *testing.T) {
			service := &submissionService{}
			got, err := service.resolveAttribution(context.Background(), SubmitAnswerSheetDTO{OriginRef: &origin}, domain.Admission{}, time.Now())
			if !errors.IsCode(err, code.ErrModuleInitializationFailed) || !got.IsZero() {
				t.Fatalf("missing attribution must fail before accepting: snapshot=%#v err=%v", got, err)
			}
		})
	}
}
