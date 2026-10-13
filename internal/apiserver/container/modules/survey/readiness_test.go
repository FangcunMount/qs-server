package survey

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	startapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answeringstart"
	asapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	qapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type readyCatalog struct {
	modelcatalog.CatalogManagementService
	bound bool
	err   error
}

func (c readyCatalog) IsQuestionnaireBound(context.Context, string) (bool, error) {
	return c.bound, c.err
}
func (c readyCatalog) SynchronizeQuestionnaireVersion(context.Context, modelcatalog.ActorContext, string, string) error {
	return c.err
}

func TestCatalogBindingRequiresConfiguredManagementAndReader(t *testing.T) {
	for name, syncer := range map[string]*catalogBindingSyncer{
		"nil adapter":        nil,
		"missing management": {},
	} {
		t.Run(name, func(t *testing.T) {
			if err := syncer.validateReadiness(); !errors.IsCode(err, code.ErrModuleInitializationFailed) {
				t.Fatalf("readiness: %v", err)
			}
			if err := syncer.SyncQuestionnaireVersion(t.Context(), "Q", "1"); !errors.IsCode(err, code.ErrModuleInitializationFailed) {
				t.Fatalf("sync: %v", err)
			}
			if _, err := syncer.IsQuestionnaireBound(t.Context(), "Q"); !errors.IsCode(err, code.ErrModuleInitializationFailed) {
				t.Fatalf("bound: %v", err)
			}
		})
	}
	syncer := &catalogBindingSyncer{}
	want := stderrors.New("catalog read unavailable")
	syncer.SetCatalogManagementService(readyCatalog{bound: true, err: want})
	if err := syncer.validateReadiness(); err != nil {
		t.Fatal(err)
	}
	if err := syncer.SyncQuestionnaireVersion(t.Context(), "Q", "1"); !stderrors.Is(err, want) {
		t.Fatalf("sync lost error: %v", err)
	}
	if _, err := syncer.IsQuestionnaireBound(t.Context(), "Q"); !stderrors.Is(err, want) {
		t.Fatalf("bound lost error: %v", err)
	}
	syncer.SetCatalogManagementService(readyCatalog{bound: true})
	if bound, err := syncer.IsQuestionnaireBound(t.Context(), "Q"); !bound || err != nil {
		t.Fatalf("bound=%v err=%v", bound, err)
	}
}

type readySubmission struct {
	asapp.AnswerSheetSubmissionService
	err error
}

func (s readySubmission) ValidateReadiness() error { return s.err }

type lifecycleFixture struct {
	qapp.QuestionnaireLifecycleService
}
type contentFixture struct {
	qapp.QuestionnaireContentService
}
type queryFixture struct{ qapp.QuestionnaireQueryService }
type managementFixture struct {
	asapp.AnswerSheetManagementService
}
type scoringFixture struct {
	asapp.AnswerSheetScoringService
}

func readySurveyFixture() *Module {
	syncer := &catalogBindingSyncer{}
	syncer.SetCatalogManagementService(readyCatalog{})
	return &Module{bindingSyncer: syncer, AnsweringStart: &startapp.Service{},
		Questionnaire: &QuestionnaireSubModule{LifecycleService: lifecycleFixture{}, ContentService: contentFixture{}, QueryService: queryFixture{}},
		AnswerSheet:   &AnswerSheetSubModule{SubmissionService: readySubmission{}, ManagementService: managementFixture{}, ScoringService: scoringFixture{}},
	}
}

func TestSurveyReadinessRunsAfterLateBindingAndExcludesOptionalServices(t *testing.T) {
	m := readySurveyFixture()
	m.bindingSyncer.SetCatalogManagementService(nil)
	if err := m.ValidateReadiness(); !errors.IsCode(err, code.ErrModuleInitializationFailed) {
		t.Fatalf("missing catalog: %v", err)
	}
	m.SetCatalogManagementService(readyCatalog{})
	want := stderrors.New("submission admission missing")
	m.AnswerSheet.SubmissionService = readySubmission{err: want}
	if err := m.ValidateReadiness(); !stderrors.Is(err, want) {
		t.Fatalf("submission readiness: %v", err)
	}
	m.AnswerSheet.SubmissionService = readySubmission{}
	if err := m.ValidateReadiness(); err != nil {
		t.Fatalf("optional QRCode, hotset and names must not block: %v", err)
	}
	m.AnsweringStart = nil
	if err := m.ValidateReadiness(); !errors.IsCode(err, code.ErrModuleInitializationFailed) {
		t.Fatalf("missing answering start: %v", err)
	}
}

func TestSurveyReadinessIdentifiesMissingApplicationService(t *testing.T) {
	for name, remove := range map[string]func(*Module){
		"questionnaire module":            func(m *Module) { m.Questionnaire = nil },
		"answer sheet module":             func(m *Module) { m.AnswerSheet = nil },
		"answering start service":         func(m *Module) { m.AnsweringStart = nil },
		"questionnaire lifecycle service": func(m *Module) { m.Questionnaire.LifecycleService = nil },
		"questionnaire content service":   func(m *Module) { m.Questionnaire.ContentService = nil },
		"questionnaire query service":     func(m *Module) { m.Questionnaire.QueryService = nil },
		"answer sheet submission service": func(m *Module) { m.AnswerSheet.SubmissionService = nil },
		"answer sheet management service": func(m *Module) { m.AnswerSheet.ManagementService = nil },
		"answer sheet scoring service":    func(m *Module) { m.AnswerSheet.ScoringService = nil },
		"questionnaire catalog management": func(m *Module) {
			m.bindingSyncer.SetCatalogManagementService(nil)
		},
		"questionnaire catalog binding reader": func(m *Module) {
			m.bindingSyncer.SetCatalogManagementService(struct {
				modelcatalog.CatalogManagementService
			}{})
		},
		"submission readiness validator": func(m *Module) {
			m.AnswerSheet.SubmissionService = struct {
				asapp.AnswerSheetSubmissionService
			}{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := readySurveyFixture()
			remove(m)
			err := m.ValidateReadiness()
			if !errors.IsCode(err, code.ErrModuleInitializationFailed) || !strings.Contains(fmt.Sprintf("%-v", err), name+" is not configured") {
				t.Fatalf("missing service must be individually identified: %v", err)
			}
		})
	}
}
