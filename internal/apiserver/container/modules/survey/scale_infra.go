package survey

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	answerSheetMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	questionnaireMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	scaleMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// ScaleInfra holds shared survey/scale Mongo repositories and caches for module wiring.
type ScaleInfra struct {
	QuestionnaireRepo   questionnaire.Repository
	QuestionnaireReader surveyreadmodel.QuestionnaireReader
	AnswerSheetRepo     *answerSheetMongo.Repository
	AnswerSheetReader   surveyreadmodel.AnswerSheetReader
	ScaleRepo           scaledefinition.Repository
	ScaleReader         scalereadmodel.ScaleReader
	ScaleListCache      scalelistcache.PublishedListCache
	ScaleHotListCache   scalelistcache.HotListCache
}

// ScaleInfraDeps collects infrastructure inputs for EnsureScaleInfra.
type ScaleInfraDeps struct {
	MongoDB             *mongo.Database
	EventCatalog        *eventcatalog.Catalog
	MongoLimiter        backpressure.Acquirer
	StaticRedis         redis.UniversalClient
	StaticBuilder       *keyspace.Builder
	QuestionnairePolicy cachepolicy.CachePolicy
	ScalePolicy         cachepolicy.CachePolicy
	ScaleListPolicy     cachepolicy.CachePolicy
	Observer            *observability.ComponentObserver
	IdentityService     *iam.IdentityService
}

// EnsureScaleInfraCached returns cached scale infra or builds it once.
func EnsureScaleInfraCached(existing *ScaleInfra, deps ScaleInfraDeps) (*ScaleInfra, error) {
	if existing != nil {
		return existing, nil
	}
	return EnsureScaleInfra(deps)
}

// EnsureScaleInfra builds shared survey/scale repositories and caches.
func EnsureScaleInfra(deps ScaleInfraDeps) (*ScaleInfra, error) {
	if deps.MongoDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: deps.MongoLimiter}

	questionnaireBaseRepo := questionnaireMongo.NewRepository(deps.MongoDB, mongoOpts)
	questionnaireReader := questionnaireMongo.NewQuestionnaireReadModel(questionnaireBaseRepo)
	var questionnaireRepo questionnaire.Repository = questionnaireBaseRepo
	if deps.StaticRedis != nil {
		questionnaireRepo = scaleCache.NewCachedQuestionnaireRepositoryWithBuilderPolicyAndObserver(
			questionnaireBaseRepo,
			deps.StaticRedis,
			deps.StaticBuilder,
			deps.QuestionnairePolicy,
			deps.Observer,
		)
	}

	answerSheetRepo, err := answerSheetMongo.NewRepositoryWithTopicResolver(deps.MongoDB, deps.EventCatalog, mongoOpts)
	if err != nil {
		return nil, err
	}
	answerSheetReader := answerSheetMongo.NewAnswerSheetReadModel(answerSheetRepo)

	scaleBaseRepo := scaleMongo.NewRepository(deps.MongoDB, mongoOpts)
	scaleReader := scaleMongo.NewScaleReadModel(scaleBaseRepo)
	var scaleRepo scaledefinition.Repository = scaleBaseRepo
	if deps.StaticRedis != nil {
		scaleRepo = scaleCache.NewCachedScaleRepositoryWithBuilderPolicyAndObserver(
			scaleBaseRepo,
			deps.StaticRedis,
			deps.StaticBuilder,
			deps.ScalePolicy,
			deps.Observer,
		)
	}

	var scaleListCache scalelistcache.PublishedListCache
	var scaleHotListCache scalelistcache.HotListCache
	if deps.StaticRedis != nil {
		staticCacheEntry := cacheentry.NewRedisCache(deps.StaticRedis)
		scaleListCache = cachequery.NewPublishedScaleListCacheWithPolicyAndKeyBuilder(
			staticCacheEntry,
			scaleReader,
			deps.IdentityService,
			deps.StaticBuilder,
			deps.ScaleListPolicy,
		)
		scaleHotListCache = cachequery.NewPublishedScaleHotListCacheWithPolicyAndKeyBuilder(
			staticCacheEntry,
			deps.StaticBuilder,
			deps.ScaleListPolicy,
		)
	}

	return &ScaleInfra{
		QuestionnaireRepo:   questionnaireRepo,
		QuestionnaireReader: questionnaireReader,
		AnswerSheetRepo:     answerSheetRepo,
		AnswerSheetReader:   answerSheetReader,
		ScaleRepo:           scaleRepo,
		ScaleReader:         scaleReader,
		ScaleListCache:      scaleListCache,
		ScaleHotListCache:   scaleHotListCache,
	}, nil
}
