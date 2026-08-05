//go:build integration

package questionnaire_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/component-base/pkg/event"
	appquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	mongoquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var errInjectedLifecyclePersistence = errors.New("injected questionnaire lifecycle persistence failure")

type independentBindingSyncer struct{}

func (independentBindingSyncer) SyncQuestionnaireVersion(context.Context, string, string) error {
	return nil
}

func (independentBindingSyncer) IsQuestionnaireBound(context.Context, string) (bool, error) {
	return false, nil
}

type failingLifecycleRepository struct {
	domainquestionnaire.Repository
	failSetActive   bool
	failClearActive bool
}

func (r failingLifecycleRepository) SetActivePublishedVersion(ctx context.Context, code, version string) error {
	if r.failSetActive {
		return errInjectedLifecyclePersistence
	}
	return r.Repository.SetActivePublishedVersion(ctx, code, version)
}

func (r failingLifecycleRepository) ClearActivePublishedVersion(ctx context.Context, code string) error {
	if r.failClearActive {
		return errInjectedLifecyclePersistence
	}
	return r.Repository.ClearActivePublishedVersion(ctx, code)
}

func TestStandaloneQuestionnaireLifecycleRollsBackOnPartialFailure(t *testing.T) {
	t.Run("publish rolls back head and snapshot when active switch fails", func(t *testing.T) {
		_, db := mongodbtest.ReplicaSetDatabase(t)
		repo := mongoquestionnaire.NewRepository(db)
		code := "Q-STANDALONE-PUBLISH-ROLLBACK"
		originalVersion := createDraftQuestionnaire(t, repo, code)
		service := newLifecycleService(db, failingLifecycleRepository{Repository: repo, failSetActive: true})

		if _, err := service.Publish(t.Context(), code); err == nil {
			t.Fatal("Publish() error = nil, want injected active-switch failure")
		}

		head, err := repo.FindByCode(t.Context(), code)
		if err != nil {
			t.Fatalf("FindByCode() after rollback: %v", err)
		}
		if !head.IsDraft() || head.GetVersion().String() != originalVersion {
			t.Fatalf("head after rollback = status=%s version=%s, want draft/%s", head.GetStatus(), head.GetVersion(), originalVersion)
		}
		assertSnapshotCount(t, db, code, 0)
	})

	for _, transition := range []struct {
		name string
		run  func(appquestionnaire.QuestionnaireLifecycleService, context.Context, string) error
	}{
		{
			name: "unpublish",
			run: func(service appquestionnaire.QuestionnaireLifecycleService, ctx context.Context, code string) error {
				_, err := service.Unpublish(ctx, code)
				return err
			},
		},
		{
			name: "archive",
			run: func(service appquestionnaire.QuestionnaireLifecycleService, ctx context.Context, code string) error {
				_, err := service.Archive(ctx, code)
				return err
			},
		},
	} {
		transition := transition
		t.Run(transition.name+" rolls back head when active clear fails", func(t *testing.T) {
			_, db := mongodbtest.ReplicaSetDatabase(t)
			repo := mongoquestionnaire.NewRepository(db)
			code := "Q-STANDALONE-" + transition.name + "-ROLLBACK"
			createDraftQuestionnaire(t, repo, code)
			if _, err := newLifecycleService(db, repo).Publish(t.Context(), code); err != nil {
				t.Fatalf("prepare published questionnaire: %v", err)
			}
			published, err := repo.FindPublishedByCode(t.Context(), code)
			if err != nil || published == nil {
				t.Fatalf("FindPublishedByCode() before failure = published=%v err=%v", published != nil, err)
			}
			publishedVersion := published.GetVersion().String()

			failingService := newLifecycleService(db, failingLifecycleRepository{Repository: repo, failClearActive: true})
			if err := transition.run(failingService, t.Context(), code); err == nil {
				t.Fatalf("%s() error = nil, want injected active-clear failure", transition.name)
			}

			head, err := repo.FindByCode(t.Context(), code)
			if err != nil {
				t.Fatalf("FindByCode() after rollback: %v", err)
			}
			if !head.IsPublished() || head.GetVersion().String() != publishedVersion {
				t.Fatalf("head after rollback = status=%s version=%s, want published/%s", head.GetStatus(), head.GetVersion(), publishedVersion)
			}
			active, err := repo.FindPublishedByCode(t.Context(), code)
			if err != nil || active == nil || active.GetVersion().String() != publishedVersion {
				t.Fatalf("active snapshot after rollback = active=%v err=%v, want version %s", active != nil, err, publishedVersion)
			}
			assertSnapshotCount(t, db, code, 1)
		})
	}
}

func newLifecycleService(db *mongo.Database, repo domainquestionnaire.Repository) appquestionnaire.QuestionnaireLifecycleService {
	return appquestionnaire.NewLifecycleService(
		repo,
		independentBindingSyncer{},
		domainquestionnaire.Validator{},
		domainquestionnaire.NewLifecycle(),
		event.NewNopEventPublisher(),
		mongoTransactionRunner(db),
	)
}

func mongoTransactionRunner(db *mongo.Database) apptransaction.Runner {
	return apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		session, err := db.Client().StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
			return nil, fn(txCtx)
		})
		return err
	})
}

func createDraftQuestionnaire(t *testing.T, repo domainquestionnaire.Repository, code string) string {
	t.Helper()
	q, err := domainquestionnaire.NewQuestionnaire(
		meta.NewCode(code),
		"Standalone lifecycle transaction",
		domainquestionnaire.WithType(domainquestionnaire.TypeSurvey),
		domainquestionnaire.WithVersion(domainquestionnaire.Version("1.0.0")),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire(): %v", err)
	}
	question, err := domainquestionnaire.NewQuestion(
		domainquestionnaire.WithCode(meta.NewCode("Q1")),
		domainquestionnaire.WithStem("Question 1"),
		domainquestionnaire.WithQuestionType(domainquestionnaire.TypeText),
	)
	if err != nil {
		t.Fatalf("NewQuestion(): %v", err)
	}
	if err := q.AddQuestion(question); err != nil {
		t.Fatalf("AddQuestion(): %v", err)
	}
	if err := repo.Create(t.Context(), q); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	return q.GetVersion().String()
}

func assertSnapshotCount(t *testing.T, db *mongo.Database, code string, want int64) {
	t.Helper()
	got, err := db.Collection("questionnaires").CountDocuments(t.Context(), bson.M{
		"code":        code,
		"record_role": domainquestionnaire.RecordRolePublishedSnapshot.String(),
	})
	if err != nil {
		t.Fatalf("CountDocuments(): %v", err)
	}
	if got != want {
		t.Fatalf("published snapshot count = %d, want %d", got, want)
	}
}
