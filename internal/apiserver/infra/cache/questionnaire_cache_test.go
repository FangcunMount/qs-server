package cache

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

type questionnaireRepoStub struct {
	head                   map[string]*domainQuestionnaire.Questionnaire
	published              map[string]*domainQuestionnaire.Questionnaire
	versioned              map[string]*domainQuestionnaire.Questionnaire
	findByCodeCalls        int
	findPublishedCalls     int
	findByCodeVersionCalls int
}

func (s *questionnaireRepoStub) Create(_ context.Context, q *domainQuestionnaire.Questionnaire) error {
	s.head[q.GetCode().Value()] = q
	return nil
}

func (s *questionnaireRepoStub) CreatePublishedSnapshot(_ context.Context, q *domainQuestionnaire.Questionnaire, active bool) error {
	key := q.GetCode().Value() + ":" + q.GetVersion().Value()
	s.versioned[key] = q
	if active {
		s.published[q.GetCode().Value()] = q
	}
	return nil
}

func (s *questionnaireRepoStub) FindByCode(_ context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	s.findByCodeCalls++
	return s.head[code], nil
}

func (s *questionnaireRepoStub) FindPublishedByCode(_ context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	s.findPublishedCalls++
	return s.published[code], nil
}

func (s *questionnaireRepoStub) FindLatestPublishedByCode(_ context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return s.published[code], nil
}

func (s *questionnaireRepoStub) FindByCodeVersion(_ context.Context, code, version string) (*domainQuestionnaire.Questionnaire, error) {
	s.findByCodeVersionCalls++
	return s.versioned[code+":"+version], nil
}

func (s *questionnaireRepoStub) FindBaseByCode(_ context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return s.head[code], nil
}

func (s *questionnaireRepoStub) FindBasePublishedByCode(_ context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	return s.published[code], nil
}

func (s *questionnaireRepoStub) FindBaseByCodeVersion(_ context.Context, code, version string) (*domainQuestionnaire.Questionnaire, error) {
	return s.versioned[code+":"+version], nil
}

func (s *questionnaireRepoStub) LoadQuestions(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}

func (s *questionnaireRepoStub) FindBaseList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}

func (s *questionnaireRepoStub) FindBasePublishedList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}

func (s *questionnaireRepoStub) CountWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}

func (s *questionnaireRepoStub) CountPublishedWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}

func (s *questionnaireRepoStub) Update(_ context.Context, q *domainQuestionnaire.Questionnaire) error {
	s.head[q.GetCode().Value()] = q
	return nil
}

func (s *questionnaireRepoStub) SetActivePublishedVersion(_ context.Context, code, version string) error {
	s.published[code] = s.versioned[code+":"+version]
	return nil
}

func (s *questionnaireRepoStub) ClearActivePublishedVersion(_ context.Context, code string) error {
	delete(s.published, code)
	return nil
}

func (s *questionnaireRepoStub) Remove(_ context.Context, code string) error {
	delete(s.head, code)
	delete(s.published, code)
	return nil
}

func (s *questionnaireRepoStub) HardDelete(_ context.Context, code string) error {
	delete(s.head, code)
	return nil
}

func (s *questionnaireRepoStub) HardDeleteFamily(_ context.Context, code string) error {
	delete(s.head, code)
	delete(s.published, code)
	for key := range s.versioned {
		if len(key) >= len(code)+1 && key[:len(code)+1] == code+":" {
			delete(s.versioned, key)
		}
	}
	return nil
}

func (s *questionnaireRepoStub) ExistsByCode(_ context.Context, code string) (bool, error) {
	_, ok := s.head[code]
	return ok, nil
}

func (s *questionnaireRepoStub) HasPublishedSnapshots(_ context.Context, code string) (bool, error) {
	_, ok := s.published[code]
	return ok, nil
}

func TestCachedQuestionnaireRepositoryFindByCodeCachesHeadKey(t *testing.T) {
	cachedRepo, _, repo, cleanup := newQuestionnaireCacheTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	got, err := cachedRepo.FindByCode(ctx, "Q-001")
	if err != nil {
		t.Fatalf("FindByCode() error = %v", err)
	}
	if got == nil {
		t.Fatal("FindByCode() returned nil questionnaire")
	}

	waitFor(t, func() bool {
		return hasRedisKey(t, cachedRepo.client, cachedRepo.headKey("Q-001"))
	})
	if repo.findByCodeCalls != 1 {
		t.Fatalf("FindByCode() repo calls = %d, want 1", repo.findByCodeCalls)
	}
}

func TestCachedQuestionnaireRepositoryFindPublishedByCodeCachesPublishedKey(t *testing.T) {
	cachedRepo, _, repo, cleanup := newQuestionnaireCacheTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	got, err := cachedRepo.FindPublishedByCode(ctx, "Q-001")
	if err != nil {
		t.Fatalf("FindPublishedByCode() error = %v", err)
	}
	if got == nil {
		t.Fatal("FindPublishedByCode() returned nil questionnaire")
	}

	waitFor(t, func() bool {
		return hasRedisKey(t, cachedRepo.client, cachedRepo.publishedKey("Q-001"))
	})
	if repo.findPublishedCalls != 1 {
		t.Fatalf("FindPublishedByCode() repo calls = %d, want 1", repo.findPublishedCalls)
	}
}

func TestCachedQuestionnaireRepositoryFindByCodeVersionCachesExactVersionAndNegativeResult(t *testing.T) {
	cachedRepo, _, repo, cleanup := newQuestionnaireCacheTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	got, err := cachedRepo.FindByCodeVersion(ctx, "Q-001", "1.0.1")
	if err != nil {
		t.Fatalf("FindByCodeVersion() error = %v", err)
	}
	if got == nil {
		t.Fatal("FindByCodeVersion() returned nil questionnaire")
	}

	waitFor(t, func() bool {
		return hasRedisKey(t, cachedRepo.client, cachedRepo.versionKey("Q-001", "1.0.1"))
	})

	got, err = cachedRepo.FindByCodeVersion(ctx, "Q-001", "9.9.9")
	if err != nil {
		t.Fatalf("FindByCodeVersion() negative error = %v", err)
	}
	if got != nil {
		t.Fatal("FindByCodeVersion() should return nil on unknown version")
	}

	waitFor(t, func() bool {
		return hasRedisKey(t, cachedRepo.client, cachedRepo.versionKey("Q-001", "9.9.9"))
	})
	if repo.findByCodeVersionCalls != 2 {
		t.Fatalf("FindByCodeVersion() repo calls = %d, want 2", repo.findByCodeVersionCalls)
	}
}

func TestCachedQuestionnaireRepositoryDeleteCacheByCodeRemovesAllKeys(t *testing.T) {
	cachedRepo, _, _, cleanup := newQuestionnaireCacheTestRepoWithNamespace(t, "test-ns")
	defer cleanup()

	ctx := context.Background()
	head := newTestQuestionnaire(t, "Q-001", "1.0.2", domainQuestionnaire.RecordRoleHead, false)
	published := newTestQuestionnaire(t, "Q-001", "1.0.1", domainQuestionnaire.RecordRolePublishedSnapshot, true)

	if err := cachedRepo.setCache(ctx, cachedRepo.headKey("Q-001"), head, cachedRepo.ttl); err != nil {
		t.Fatalf("set head cache error = %v", err)
	}
	if err := cachedRepo.setCache(ctx, cachedRepo.publishedKey("Q-001"), published, cachedRepo.ttl); err != nil {
		t.Fatalf("set published cache error = %v", err)
	}
	if err := cachedRepo.client.Set(ctx, cachedRepo.versionKey("Q-001", "9.9.9"), []byte{}, time.Minute).Err(); err != nil {
		t.Fatalf("set version cache error = %v", err)
	}

	if err := cachedRepo.deleteCacheByCode(ctx, "Q-001"); err != nil {
		t.Fatalf("deleteCacheByCode() error = %v", err)
	}

	if hasRedisKey(t, cachedRepo.client, cachedRepo.headKey("Q-001")) {
		t.Fatal("head cache key should be deleted")
	}
	if hasRedisKey(t, cachedRepo.client, cachedRepo.publishedKey("Q-001")) {
		t.Fatal("published cache key should be deleted")
	}
	if hasRedisKey(t, cachedRepo.client, cachedRepo.versionKey("Q-001", "9.9.9")) {
		t.Fatal("version cache key should be deleted")
	}
}

func TestCachedQuestionnaireRepositoryWarmupCacheWritesHeadAndPublishedKeys(t *testing.T) {
	cachedRepo, _, _, cleanup := newQuestionnaireCacheTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	if err := cachedRepo.WarmupCache(ctx, []string{"Q-001"}); err != nil {
		t.Fatalf("WarmupCache() error = %v", err)
	}

	if !hasRedisKey(t, cachedRepo.client, cachedRepo.headKey("Q-001")) {
		t.Fatal("head cache key should exist after warmup")
	}
	if !hasRedisKey(t, cachedRepo.client, cachedRepo.publishedKey("Q-001")) {
		t.Fatal("published cache key should exist after warmup")
	}
}

func TestCachedQuestionnaireRepositorySupportsExplicitBuilderNamespace(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	head := newTestQuestionnaire(t, "Q-001", "1.0.2", domainQuestionnaire.RecordRoleHead, false)
	published := newTestQuestionnaire(t, "Q-001", "1.0.1", domainQuestionnaire.RecordRolePublishedSnapshot, true)
	repo := &questionnaireRepoStub{
		head: map[string]*domainQuestionnaire.Questionnaire{
			"Q-001": head,
		},
		published: map[string]*domainQuestionnaire.Questionnaire{
			"Q-001": published,
		},
		versioned: map[string]*domainQuestionnaire.Questionnaire{
			"Q-001:1.0.1": published,
			"Q-001:1.0.2": head,
		},
	}

	cachedRepo := NewCachedQuestionnaireRepositoryWithBuilderAndPolicy(repo, client, rediskey.NewBuilderWithNamespace("prod:cache:static"), cachepolicy.CachePolicy{
		Negative: cachepolicy.PolicySwitchEnabled,
	}).(*CachedQuestionnaireRepository)
	if _, err := cachedRepo.FindPublishedByCode(context.Background(), "Q-001"); err != nil {
		t.Fatalf("FindPublishedByCode() error = %v", err)
	}

	waitFor(t, func() bool {
		return hasRedisKey(t, cachedRepo.client, "prod:cache:static:questionnaire:published:q-001")
	})
}

func newQuestionnaireCacheTestRepo(t *testing.T) (*CachedQuestionnaireRepository, *miniredis.Miniredis, *questionnaireRepoStub, func()) {
	return newQuestionnaireCacheTestRepoWithNamespace(t, "")
}

func newQuestionnaireCacheTestRepoWithNamespace(t *testing.T, namespace string) (*CachedQuestionnaireRepository, *miniredis.Miniredis, *questionnaireRepoStub, func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	head := newTestQuestionnaire(t, "Q-001", "1.0.2", domainQuestionnaire.RecordRoleHead, false)
	published := newTestQuestionnaire(t, "Q-001", "1.0.1", domainQuestionnaire.RecordRolePublishedSnapshot, true)

	repo := &questionnaireRepoStub{
		head: map[string]*domainQuestionnaire.Questionnaire{
			"Q-001": head,
		},
		published: map[string]*domainQuestionnaire.Questionnaire{
			"Q-001": published,
		},
		versioned: map[string]*domainQuestionnaire.Questionnaire{
			"Q-001:1.0.1": published,
			"Q-001:1.0.2": head,
		},
	}
	cachedRepo := NewCachedQuestionnaireRepositoryWithBuilderAndPolicy(repo, client, rediskey.NewBuilderWithNamespace(namespace), cachepolicy.CachePolicy{
		Negative: cachepolicy.PolicySwitchEnabled,
	}).(*CachedQuestionnaireRepository)

	cleanup := func() {
		_ = client.Close()
		mr.Close()
	}
	return cachedRepo, mr, repo, cleanup
}

func newTestQuestionnaire(t *testing.T, code, version string, role domainQuestionnaire.RecordRole, active bool) *domainQuestionnaire.Questionnaire {
	t.Helper()

	status := domainQuestionnaire.STATUS_DRAFT
	if role == domainQuestionnaire.RecordRolePublishedSnapshot {
		status = domainQuestionnaire.STATUS_PUBLISHED
	}

	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode(code),
		"Test Questionnaire",
		domainQuestionnaire.WithVersion(domainQuestionnaire.NewVersion(version)),
		domainQuestionnaire.WithStatus(status),
		domainQuestionnaire.WithRecordRole(role),
		domainQuestionnaire.WithActivePublished(active),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	return q
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func hasRedisKey(t *testing.T, client redis.UniversalClient, key string) bool {
	t.Helper()
	return client.Exists(context.Background(), key).Val() > 0
}
