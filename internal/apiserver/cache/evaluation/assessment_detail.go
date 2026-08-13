package evaluationcache

import (
	"context"
	"encoding/json"

	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/internal/adapterkit"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

type assessmentOwnerEntry struct {
	TesteeID uint64 `json:"testee_id"`
}

// AssessmentCaches owns the two independent Evaluation L2 intents:
// immutable ownership and evaluated participant detail.
type AssessmentCaches struct {
	keys        *keyspace.Builder
	policies    sharedcache.PolicyProvider
	observer    *observability.ComponentObserver
	accessStore *adapterkit.ObjectCacheStore[assessmentOwnerEntry]
	detailStore *adapterkit.ObjectCacheStore[evaluationtestee.Assessment]
}

func NewAssessmentCaches(client redis.UniversalClient, builder *keyspace.Builder, policies sharedcache.PolicyProvider, observer *observability.ComponentObserver) *AssessmentCaches {
	if builder == nil {
		panic("redis builder is required")
	}
	store := adapterkit.NewRedisStoreIfAvailable(client)
	return &AssessmentCaches{
		keys:     builder,
		policies: policies,
		observer: observer,
		accessStore: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[assessmentOwnerEntry]{
			Cache: store, PolicyKey: cachepolicy.CapabilityEvaluationAssessmentAccess, Codec: jsonCodec[assessmentOwnerEntry](),
		}),
		detailStore: adapterkit.NewObjectCacheStore(adapterkit.ObjectCacheStoreOptions[evaluationtestee.Assessment]{
			Cache: store, PolicyKey: cachepolicy.CapabilityEvaluationAssessmentDetail, Codec: jsonCodec[evaluationtestee.Assessment](),
		}),
	}
}

func jsonCodec[T any]() adapterkit.CacheEntryCodec[T] {
	return adapterkit.CacheEntryCodec[T]{
		EncodeFunc: func(value *T) ([]byte, error) { return json.Marshal(value) },
		DecodeFunc: func(data []byte) (*T, error) {
			var value T
			if err := json.Unmarshal(data, &value); err != nil {
				return nil, err
			}
			return &value, nil
		},
	}
}

func (c *AssessmentCaches) ReadOwner(ctx context.Context, assessmentID uint64, load func(context.Context) (uint64, error)) (uint64, error) {
	if load == nil {
		return 0, nil
	}
	if c == nil || c.accessStore == nil || assessmentID == 0 {
		return load(ctx)
	}
	value, err := adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[assessmentOwnerEntry]{
		PolicyKey: cachepolicy.CapabilityEvaluationAssessmentAccess, CacheKey: c.keys.BuildAssessmentAccessKey(assessmentID),
		PolicyProvider: c.policies, Observer: c.observer, Store: c.accessStore,
		Load: func(loadCtx context.Context) (*assessmentOwnerEntry, error) {
			owner, loadErr := load(loadCtx)
			if loadErr != nil || owner == 0 {
				return nil, loadErr
			}
			return &assessmentOwnerEntry{TesteeID: owner}, nil
		},
	})
	if err != nil || value == nil {
		return 0, err
	}
	return value.TesteeID, nil
}

func (c *AssessmentCaches) ReadDetail(ctx context.Context, assessmentID uint64, load func(context.Context) (*evaluationtestee.Assessment, error)) (*evaluationtestee.Assessment, error) {
	if load == nil {
		return nil, nil
	}
	if c == nil || c.detailStore == nil || assessmentID == 0 {
		return load(ctx)
	}
	return adapterkit.ReadThroughObject(ctx, adapterkit.ObjectReadThroughOptions[evaluationtestee.Assessment]{
		PolicyKey: cachepolicy.CapabilityEvaluationAssessmentDetail, CacheKey: c.keys.BuildAssessmentOutcomeDetailKey(assessmentID),
		PolicyProvider: c.policies, Observer: c.observer, Store: c.detailStore, Load: load,
		ShouldCache: func(value *evaluationtestee.Assessment) bool {
			return value != nil && value.Status == assessment.StatusEvaluated.String()
		},
	})
}

func (c *AssessmentCaches) Evict(ctx context.Context, assessmentID uint64) {
	if c == nil || assessmentID == 0 {
		return
	}
	if c.accessStore != nil {
		_ = c.accessStore.Delete(ctx, c.keys.BuildAssessmentAccessKey(assessmentID))
	}
	if c.detailStore != nil {
		_ = c.detailStore.Delete(ctx, c.keys.BuildAssessmentOutcomeDetailKey(assessmentID))
	}
}

// InvalidatingAssessmentRepository keeps cache invalidation at the aggregate
// write boundary while all reads remain owned by the application services.
type InvalidatingAssessmentRepository struct {
	repo   assessment.Repository
	caches *AssessmentCaches
}

func NewInvalidatingAssessmentRepository(repo assessment.Repository, caches *AssessmentCaches) assessment.Repository {
	return &InvalidatingAssessmentRepository{repo: repo, caches: caches}
}

func (r *InvalidatingAssessmentRepository) FindByID(ctx context.Context, id assessment.ID) (*assessment.Assessment, error) {
	return r.repo.FindByID(ctx, id)
}

func (r *InvalidatingAssessmentRepository) FindByAnswerSheetID(ctx context.Context, answerSheetID assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return r.repo.FindByAnswerSheetID(ctx, answerSheetID)
}

func (r *InvalidatingAssessmentRepository) Save(ctx context.Context, value *assessment.Assessment) error {
	if err := r.repo.Save(ctx, value); err != nil {
		return err
	}
	if value != nil {
		r.caches.Evict(ctx, value.ID().Uint64())
	}
	return nil
}

func (r *InvalidatingAssessmentRepository) Delete(ctx context.Context, id assessment.ID) error {
	if err := r.repo.Delete(ctx, id); err != nil {
		return err
	}
	r.caches.Evict(ctx, id.Uint64())
	return nil
}

var _ evaluationtestee.AssessmentAccessCache = (*AssessmentCaches)(nil)
var _ evaluationtestee.AssessmentDetailCache = (*AssessmentCaches)(nil)
var _ assessment.Repository = (*InvalidatingAssessmentRepository)(nil)
