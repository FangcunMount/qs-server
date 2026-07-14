package survey

import (
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	modelcatalogApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	asApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	ruleengineInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Module assembles survey application services.
type Module struct {
	Questionnaire *QuestionnaireSubModule
	AnswerSheet   *AnswerSheetSubModule
	bindingSyncer *catalogBindingSyncer

	eventPublisher event.EventPublisher
}

// Deps defines explicit constructor dependencies for the survey module.
type Deps struct {
	MongoDB             *mongo.Database
	EventPublisher      event.EventPublisher
	IdentityService     *iam.IdentityService
	HotsetRecorder      cachetarget.HotsetRecorder
	QuestionnaireRepo   questionnaire.Repository
	QuestionnaireReader surveyreadmodel.QuestionnaireReader
	AnswerSheetRepo     AnswerSheetStore
	AnswerSheetReader   surveyreadmodel.AnswerSheetReader
	CacheSignalNotifier quesApp.CacheSignalNotifier
	OutboxProfile       appEventing.ProfileBinding
}

// AnswerSheetStore exposes answer-sheet persistence only; Outbox capability is
// supplied separately by EventSubsystem's profile binding.
type AnswerSheetStore interface {
	answersheet.Repository
	asApp.SubmissionDurableWriter
}

// QuestionnaireSubModule hosts questionnaire application services.
type QuestionnaireSubModule struct {
	LifecycleService quesApp.QuestionnaireLifecycleService
	ContentService   quesApp.QuestionnaireContentService
	QueryService     quesApp.QuestionnaireQueryService
}

// AnswerSheetSubModule hosts answer-sheet application services.
type AnswerSheetSubModule struct {
	SubmissionService asApp.AnswerSheetSubmissionService
	ManagementService asApp.AnswerSheetManagementService
	ScoringService    asApp.AnswerSheetScoringService
}

// New assembles the survey module.
func New(deps Deps) (*Module, error) {
	normalized, err := normalizeDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &Module{
		Questionnaire: &QuestionnaireSubModule{},
		AnswerSheet:   &AnswerSheetSubModule{},
		bindingSyncer: &catalogBindingSyncer{},
	}

	module.eventPublisher = normalized.EventPublisher
	if err := module.initQuestionnaireSubModule(
		normalized.IdentityService,
		normalized.HotsetRecorder,
		module.bindingSyncer,
		normalized.QuestionnaireRepo,
		normalized.QuestionnaireReader,
		normalized.CacheSignalNotifier,
	); err != nil {
		return nil, err
	}

	if err := module.initAnswerSheetSubModule(
		normalized.MongoDB,
		normalized.AnswerSheetRepo,
		normalized.AnswerSheetReader,
		normalized.QuestionnaireRepo,
		normalized.OutboxProfile,
	); err != nil {
		return nil, err
	}

	return module, nil
}

func normalizeDeps(deps Deps) (Deps, error) {
	if deps.MongoDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	if deps.QuestionnaireRepo == nil || deps.QuestionnaireReader == nil || deps.AnswerSheetRepo == nil || deps.AnswerSheetReader == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "survey repositories and read models are required")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

func (m *Module) initQuestionnaireSubModule(identitySvc *iam.IdentityService, hotset cachetarget.HotsetRecorder, bindingSyncer quesApp.QuestionnaireBindingVersionSyncer, repo questionnaire.Repository, reader surveyreadmodel.QuestionnaireReader, cacheSignalNotifier quesApp.CacheSignalNotifier) error {
	sub := m.Questionnaire

	validator := questionnaire.Validator{}
	lifecycle := questionnaire.NewLifecycle()

	sub.LifecycleService = quesApp.NewLifecycleService(
		repo,
		bindingSyncer,
		validator,
		lifecycle,
		m.eventPublisher,
		quesApp.WithCacheSignalNotifier(cacheSignalNotifier),
	)
	sub.ContentService = quesApp.NewContentService(repo)
	sub.QueryService = quesApp.NewQueryService(repo, identitySvc, hotset, reader)

	return nil
}

// SetCatalogManagementService completes questionnaire-to-catalog binding
// synchronization after the assessment-model module is available.
func (m *Module) SetCatalogManagementService(service modelcatalogApp.CatalogManagementService) {
	if m == nil {
		return
	}
	m.bindingSyncer.SetCatalogManagementService(service)
}

func (m *Module) initAnswerSheetSubModule(mongoDB *mongo.Database, repo AnswerSheetStore, reader surveyreadmodel.AnswerSheetReader, questionnaireRepo questionnaire.Repository, profile appEventing.ProfileBinding) error {
	sub := m.AnswerSheet

	answerScorer := ruleengineInfra.NewAnswerScorer()

	mongoTxRunner := modtx.NewMongoRunner(mongoDB)
	if profile.Stager == nil || profile.PostCommit == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "mongo domain event profile is required")
	}
	durableStore := asApp.NewTransactionalSubmissionDurableStore(mongoTxRunner, repo, profile.Stager, profile.PostCommit)
	sub.SubmissionService = asApp.NewSubmissionService(repo, durableStore, questionnaireRepo, reader)
	sub.ManagementService = asApp.NewManagementService(repo, reader)
	sub.ScoringService = asApp.NewAnswerSheetScoringService(repo, questionnaireRepo, answerScorer)
	return nil
}

// Cleanup releases module resources.
func (m *Module) Cleanup() error {
	return nil
}

// CheckHealth verifies module health.
func (m *Module) CheckHealth() error {
	return nil
}

// ModuleInfo returns module metadata.
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        string(Name),
		Version:     "1.0.0",
		Description: "问卷量表模块（包含问卷和答卷子模块）",
	}
}
