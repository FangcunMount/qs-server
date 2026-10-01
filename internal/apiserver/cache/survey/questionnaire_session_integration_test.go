//go:build integration

package surveycache

import (
	"context"
	"errors"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestQuestionnaireCacheSessionReadsOwnPublicationAndRollback(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	cached, mr, _, cleanup := newQuestionnaireCacheTestRepo(t)
	defer cleanup()
	cached.repo = repository.NewRepository(db)
	q := newTestQuestionnaire(t, "SESSION-Q", "1.0.1", domain.RecordRoleHead, false)
	if err := cached.Create(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	key := cached.headKey("SESSION-Q")
	before, err := mr.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(context.Background())
	rollback := errors.New("rollback after publication reads")
	_, err = session.WithTransaction(t.Context(), func(ctx mongo.SessionContext) (any, error) {
		current, err := cached.FindByCode(ctx, "SESSION-Q")
		if err != nil {
			return nil, err
		}
		// This is the repository state written by PublishForRelease. Domain
		// validators are covered by the publication tests; here the contract is
		// reading the paired transaction's own state through the cached adapter.
		published, err := domain.NewQuestionnaire(current.GetCode(), current.GetTitle(),
			domain.WithVersion(domain.NewVersion("2.0.0")), domain.WithRevision(current.GetRevision()),
			domain.WithStatus(domain.STATUS_PUBLISHED))
		if err != nil {
			return nil, err
		}
		if err := cached.Update(ctx, published); err != nil {
			return nil, err
		}
		if err := cached.CreatePublishedSnapshot(ctx, published, false); err != nil {
			return nil, err
		}
		if err := cached.SetActivePublishedVersion(ctx, "SESSION-Q", "2.0.0"); err != nil {
			return nil, err
		}
		head, err := cached.FindByCode(ctx, "SESSION-Q")
		if err != nil {
			return nil, err
		}
		active, err := cached.FindPublishedByCode(ctx, "SESSION-Q")
		if err != nil {
			return nil, err
		}
		exact, err := cached.FindByCodeVersion(ctx, "SESSION-Q", "2.0.0")
		if err != nil {
			return nil, err
		}
		for _, got := range []*domain.Questionnaire{head, active, exact} {
			if got == nil || !got.IsPublished() || got.GetVersion().String() != "2.0.0" {
				t.Fatal("transaction did not observe its published questionnaire")
			}
		}
		return nil, rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error=%v", err)
	}
	after, err := mr.Get(key)
	if err != nil || after != before {
		t.Fatal("transaction mutated committed head cache")
	}
	if mr.Exists(cached.publishedKey("SESSION-Q")) || mr.Exists(cached.versionKey("SESSION-Q", "2.0.0")) {
		t.Fatal("rolled-back publication escaped to Redis")
	}
	head, err := cached.repo.FindByCode(t.Context(), "SESSION-Q")
	if err != nil || head == nil || !head.IsDraft() || head.GetVersion().String() != "1.0.1" {
		t.Fatalf("head after rollback=%#v err=%v", head, err)
	}
	active, err := cached.repo.FindPublishedByCode(t.Context(), "SESSION-Q")
	if err != nil || active != nil {
		t.Fatalf("active snapshot after rollback=%#v err=%v", active, err)
	}
}
