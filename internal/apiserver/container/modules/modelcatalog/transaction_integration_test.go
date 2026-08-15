//go:build integration

package modelcatalog

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appmodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appbinding "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/binding"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/lifecycle"
	apprelease "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/release"
	appquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelconclusion "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	domainquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	modelrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	questionnairerepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	modelport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestMongoReleasePairRollsBackEveryFailureBoundary(t *testing.T) {
	t.Parallel()
	_, db := mongodbtest.ReplicaSetDatabase(t)
	runner := modtx.NewMongoRunner(db, modtx.MongoRunnerOptions{
		Boundary: "assessment_release_test",
		Limiter:  backpressure.NewLimiter(32, 5*time.Second),
	})

	for _, tc := range []struct {
		name           string
		failAfterWrite int
		cancelCommit   bool
	}{
		{name: "questionnaire save failure", failAfterWrite: 1},
		{name: "model save failure", failAfterWrite: 2},
		{name: "commit failure", cancelCommit: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			code := fmt.Sprintf("PAIR-%d", time.Now().UnixNano())
			questionnaireCode := "Q-" + code
			questionnaire, err := domainquestionnaire.NewQuestionnaire(meta.NewCode(questionnaireCode), "Contract", domainquestionnaire.WithRevision(1))
			if err != nil {
				t.Fatal(err)
			}
			model, err := domainmodel.NewAssessmentModel(domainmodel.NewAssessmentModelInput{
				Code: code, Kind: domainmodel.KindScale, Algorithm: domainmodel.AlgorithmScaleDefault,
				Title: "Contract", Now: time.Now().UTC(),
			})
			if err != nil {
				t.Fatal(err)
			}
			qRepo := questionnairerepo.NewRepository(db)
			mRepo := modelrepo.NewDraftRepository(db)

			ctx := t.Context()
			cancelCommit := func() {}
			if tc.cancelCommit {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancelCommit = cancel
				defer cancel()
			}
			writeCount := 0
			err = runner.WithinTransaction(ctx, func(txCtx context.Context) error {
				if err := qRepo.Create(txCtx, questionnaire); err != nil {
					return err
				}
				writeCount++
				if writeCount == tc.failAfterWrite {
					return errors.New("injected questionnaire repository failure")
				}
				if err := mRepo.Create(txCtx, model); err != nil {
					return err
				}
				writeCount++
				if writeCount == tc.failAfterWrite {
					return errors.New("injected model repository failure")
				}
				if tc.cancelCommit {
					cancelCommit()
				}
				return nil
			})
			if tc.cancelCommit && err == nil {
				t.Fatal("commit failure error = nil")
			}
			if !tc.cancelCommit && err == nil {
				t.Fatal("repository failure error = nil")
			}
			assertPairAbsent(t, db, code, questionnaireCode)
		})
	}
}

func TestPublishReleaseRollsBackEveryPersistedBoundary(t *testing.T) {
	t.Parallel()
	_, db := mongodbtest.ReplicaSetDatabase(t)
	baseRunner := modtx.NewMongoRunner(db, modtx.MongoRunnerOptions{
		Boundary: "assessment_release_test",
		Limiter:  backpressure.NewLimiter(32, 5*time.Second),
	})

	for _, fault := range []string{
		"questionnaire_head_cas",
		"questionnaire_snapshot",
		"questionnaire_active_switch",
		"model_snapshot",
		"model_head_cas",
		"commit",
	} {
		fault := fault
		t.Run(fault, func(t *testing.T) {
			code := fmt.Sprintf("RELEASE-%s-%d", fault, time.Now().UnixNano())
			questionnaireCode := "Q-" + code
			qBase := questionnairerepo.NewRepository(db)
			mBase := modelrepo.NewDraftRepository(db)
			publishedBase := modelrepo.NewRepository(db)
			questionnaire := releaseQuestionnaire(t, questionnaireCode)
			if err := qBase.Create(t.Context(), questionnaire); err != nil {
				t.Fatalf("create questionnaire head: %v", err)
			}
			model := releaseModel(t, code, questionnaireCode)
			if err := mBase.Create(t.Context(), model); err != nil {
				t.Fatalf("create model head: %v", err)
			}

			qFault := &releaseFaultQuestionnaireRepo{Repository: qBase, fault: fault}
			mFault := &releaseFaultModelRepo{ModelRepository: mBase, fault: fault}
			pFault := &releaseFaultPublishedRepo{PublishedSnapshotRepository: publishedBase, fault: fault}
			query := appquestionnaire.NewQueryService(qFault, nil, nil, nil)
			qLifecycle := appquestionnaire.NewLifecycleService(
				qFault, releaseBindingSyncer{}, domainquestionnaire.Validator{}, domainquestionnaire.NewLifecycle(),
				event.NewNopEventPublisher(), baseRunner,
			)
			runner := apptransaction.Runner(baseRunner)
			if fault == "commit" {
				runner = cancelBeforeCommitRunner{inner: baseRunner}
			}
			effectCalls := 0
			service := apprelease.Service{
				Transactions: runner, Models: mFault, Published: pFault,
				Authorizer: releaseAllowAuthorizer{}, Registry: appdefinition.NewRegistry(releaseDefinitionHandler{}),
				Bindings:       appbinding.NewPolicies(releaseBindingPolicy{}),
				Questionnaires: qLifecycle, QuestionnaireQuery: query,
				Effects: lifecycle.NewEffectsRegistry(lifecycle.EffectFunc{
					Match: func(domainmodel.Identity) bool { return true },
					Run:   func(context.Context, *domainmodel.AssessmentModel, lifecycle.Action) { effectCalls++ },
				}),
				Now: func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
			}
			if _, err := service.PublishRelease(t.Context(), appmodelcatalog.ActorContext{}, code); err == nil {
				t.Fatalf("PublishRelease fault %s returned nil error", fault)
			}
			if effectCalls != 0 {
				t.Fatalf("post-commit effects = %d, want 0", effectCalls)
			}
			assertReleaseUnchanged(t, db, code, questionnaireCode)
		})
	}
}

type releaseFaultQuestionnaireRepo struct {
	domainquestionnaire.Repository
	fault string
}

func (r *releaseFaultQuestionnaireRepo) Update(ctx context.Context, questionnaire *domainquestionnaire.Questionnaire) error {
	if r.fault == "questionnaire_head_cas" {
		return errors.New("injected questionnaire head CAS failure")
	}
	return r.Repository.Update(ctx, questionnaire)
}

func (r *releaseFaultQuestionnaireRepo) CreatePublishedSnapshot(ctx context.Context, questionnaire *domainquestionnaire.Questionnaire, active bool) error {
	if r.fault == "questionnaire_snapshot" {
		return errors.New("injected questionnaire snapshot failure")
	}
	return r.Repository.CreatePublishedSnapshot(ctx, questionnaire, active)
}

func (r *releaseFaultQuestionnaireRepo) SetActivePublishedVersion(ctx context.Context, code, version string) error {
	if r.fault == "questionnaire_active_switch" {
		return errors.New("injected questionnaire active switch failure")
	}
	return r.Repository.SetActivePublishedVersion(ctx, code, version)
}

type releaseFaultModelRepo struct {
	modelport.ModelRepository
	fault string
}

func (r *releaseFaultModelRepo) Update(ctx context.Context, model *domainmodel.AssessmentModel) error {
	if r.fault == "model_head_cas" {
		return errors.New("injected model head CAS failure")
	}
	return r.ModelRepository.Update(ctx, model)
}

type releaseFaultPublishedRepo struct {
	modelport.PublishedSnapshotRepository
	fault string
}

func (r *releaseFaultPublishedRepo) Save(ctx context.Context, snapshot *modelport.PublishedModel) error {
	if r.fault == "model_snapshot" {
		return errors.New("injected model snapshot failure")
	}
	return r.PublishedSnapshotRepository.Save(ctx, snapshot)
}

type cancelBeforeCommitRunner struct{ inner apptransaction.Runner }

func (r cancelBeforeCommitRunner) WithinTransaction(ctx context.Context, callback func(context.Context) error) error {
	txCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	return r.inner.WithinTransaction(txCtx, func(sessionCtx context.Context) error {
		if err := callback(sessionCtx); err != nil {
			return err
		}
		cancel()
		return nil
	})
}

type releaseBindingSyncer struct{}

func (releaseBindingSyncer) SyncQuestionnaireVersion(context.Context, string, string) error {
	return nil
}
func (releaseBindingSyncer) IsQuestionnaireBound(context.Context, string) (bool, error) {
	return false, nil
}

type releaseAllowAuthorizer struct{}

func (releaseAllowAuthorizer) Authorize(context.Context, appmodelcatalog.ActorContext, appmodelcatalog.Action, appmodelcatalog.Resource) error {
	return nil
}

type releaseBindingPolicy struct{}

func (releaseBindingPolicy) Supports(identity domainmodel.Identity) bool {
	return identity.Kind == domainmodel.KindCognitive
}
func (releaseBindingPolicy) Validate(_ context.Context, _ *domainmodel.AssessmentModel, binding domainmodel.QuestionnaireBinding) (domainmodel.QuestionnaireBinding, error) {
	return binding, nil
}
func (releaseBindingPolicy) BeforePublish(context.Context, *domainmodel.AssessmentModel) error {
	return nil
}

type releaseDefinitionHandler struct{}

func (releaseDefinitionHandler) Supports(identity domainmodel.Identity) bool {
	return identity.Kind == domainmodel.KindCognitive && identity.Algorithm == domainmodel.AlgorithmSPM
}
func (releaseDefinitionHandler) ValidateForPublish(context.Context, *domainmodel.AssessmentModel) []domainmodel.DomainValidationIssue {
	return nil
}
func (releaseDefinitionHandler) MaterializeSnapshot(context.Context, *domainmodel.AssessmentModel) (appdefinition.Materialization, error) {
	return appdefinition.Materialization{
		Kind: domainmodel.KindCognitive, Algorithm: domainmodel.AlgorithmSPM,
		AlgorithmFamily: domainmodel.AlgorithmFamilyTaskPerformance,
		DecisionKind:    domainmodel.DecisionKindAbilityLevel,
	}, nil
}

func releaseQuestionnaire(t *testing.T, code string) *domainquestionnaire.Questionnaire {
	t.Helper()
	questionnaire, err := domainquestionnaire.NewQuestionnaire(
		meta.NewCode(code), "Release transaction",
		domainquestionnaire.WithType(domainquestionnaire.TypeSurvey),
		domainquestionnaire.WithVersion(domainquestionnaire.Version("1.0.0")),
		domainquestionnaire.WithStatus(domainquestionnaire.STATUS_DRAFT),
	)
	if err != nil {
		t.Fatal(err)
	}
	question, err := domainquestionnaire.NewQuestion(
		domainquestionnaire.WithCode(meta.NewCode("Q1")),
		domainquestionnaire.WithStem("Question 1"),
		domainquestionnaire.WithQuestionType(domainquestionnaire.TypeText),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := questionnaire.AddQuestion(question); err != nil {
		t.Fatal(err)
	}
	return questionnaire
}

func releaseModel(t *testing.T, code, questionnaireCode string) *domainmodel.AssessmentModel {
	t.Helper()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	model, err := domainmodel.NewAssessmentModel(domainmodel.NewAssessmentModelInput{
		Code: code, Kind: domainmodel.KindCognitive, Algorithm: domainmodel.AlgorithmSPM,
		Title: "Release transaction", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.BindQuestionnaire(domainmodel.QuestionnaireBinding{QuestionnaireCode: questionnaireCode, QuestionnaireVersion: "1.0.0"}, now); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateDefinition(&modeldefinition.Definition{
		Conclusions: []modelconclusion.Conclusion{
			modelconclusion.AbilityConclusion{FactorCode: "total", ScoreBasis: modelconclusion.ScoreBasisRaw},
		},
	}, now); err != nil {
		t.Fatal(err)
	}
	return model
}

func assertReleaseUnchanged(t *testing.T, db *mongo.Database, modelCode, questionnaireCode string) {
	t.Helper()
	var modelHead bson.M
	if err := db.Collection("assessment_models").FindOne(t.Context(), bson.M{"code": modelCode, "record_role": "head"}).Decode(&modelHead); err != nil {
		t.Fatal(err)
	}
	if modelHead["status"] != "draft" {
		t.Fatalf("model head status = %v, want draft", modelHead["status"])
	}
	var questionnaireHead bson.M
	if err := db.Collection("questionnaires").FindOne(t.Context(), bson.M{"code": questionnaireCode, "record_role": "head"}).Decode(&questionnaireHead); err != nil {
		t.Fatal(err)
	}
	if questionnaireHead["status"] != "draft" {
		t.Fatalf("questionnaire head status = %v, want draft", questionnaireHead["status"])
	}
	modelSnapshots, err := db.Collection("assessment_models").CountDocuments(t.Context(), bson.M{"code": modelCode, "record_role": "published_snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	questionnaireSnapshots, err := db.Collection("questionnaires").CountDocuments(t.Context(), bson.M{"code": questionnaireCode, "record_role": "published_snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	if modelSnapshots != 0 || questionnaireSnapshots != 0 {
		t.Fatalf("release rollback left snapshots: model=%d questionnaire=%d", modelSnapshots, questionnaireSnapshots)
	}
}

func assertPairAbsent(t *testing.T, db *mongo.Database, modelCode, questionnaireCode string) {
	t.Helper()
	modelCount, err := db.Collection("assessment_models").CountDocuments(t.Context(), bson.M{"code": modelCode})
	if err != nil {
		t.Fatal(err)
	}
	questionnaireCount, err := db.Collection("questionnaires").CountDocuments(t.Context(), bson.M{"code": questionnaireCode})
	if err != nil {
		t.Fatal(err)
	}
	if modelCount != 0 || questionnaireCount != 0 {
		t.Fatalf("half-published pair remains: models=%d questionnaires=%d", modelCount, questionnaireCount)
	}
}
