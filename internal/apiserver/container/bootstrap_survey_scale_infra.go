package container

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	answerSheetMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	questionnaireMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	scaleMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type surveyScaleInfra struct {
	questionnaireRepo   questionnaire.Repository
	questionnaireReader surveyreadmodel.QuestionnaireReader
	answerSheetRepo     *answerSheetMongo.Repository
	answerSheetReader   surveyreadmodel.AnswerSheetReader
	scaleRepo           scale.Repository
	scaleReader         scalereadmodel.ScaleReader
	scaleListCache      scalelistcache.PublishedListCache
}

func (c *Container) ensureSurveyScaleInfra() (*surveyScaleInfra, error) {
	if c == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "container is nil")
	}
	if c.surveyScaleInfra != nil {
		return c.surveyScaleInfra, nil
	}
	if c.mongoDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: c.backpressure.Mongo}
	staticRedis := c.CacheClient(cacheplane.FamilyStatic)
	staticBuilder := c.CacheBuilder(cacheplane.FamilyStatic)
	observer := c.cacheObserver()

	questionnaireBaseRepo := questionnaireMongo.NewRepository(c.mongoDB, mongoOpts)
	questionnaireReader := questionnaireMongo.NewQuestionnaireReadModel(questionnaireBaseRepo)
	var questionnaireRepo questionnaire.Repository = questionnaireBaseRepo
	if staticRedis != nil {
		questionnaireRepo = scaleCache.NewCachedQuestionnaireRepositoryWithBuilderPolicyAndObserver(
			questionnaireBaseRepo,
			staticRedis,
			staticBuilder,
			c.CachePolicy(cachepolicy.PolicyQuestionnaire),
			observer,
		)
	}

	answerSheetRepo, err := answerSheetMongo.NewRepositoryWithTopicResolver(c.mongoDB, c.eventCatalog, mongoOpts)
	if err != nil {
		return nil, err
	}
	answerSheetReader := answerSheetMongo.NewAnswerSheetReadModel(answerSheetRepo)

	scaleBaseRepo := scaleMongo.NewRepository(c.mongoDB, mongoOpts)
	scaleReader := scaleMongo.NewScaleReadModel(scaleBaseRepo)
	var scaleRepo scale.Repository = scaleBaseRepo
	if staticRedis != nil {
		scaleRepo = scaleCache.NewCachedScaleRepositoryWithBuilderPolicyAndObserver(
			scaleBaseRepo,
			staticRedis,
			staticBuilder,
			c.CachePolicy(cachepolicy.PolicyScale),
			observer,
		)
	}

	var scaleListCache scalelistcache.PublishedListCache
	if staticRedis != nil {
		scaleListCache = cachequery.NewPublishedScaleListCacheWithPolicyAndKeyBuilder(
			cacheentry.NewRedisCache(staticRedis),
			scaleReader,
			c.resolveIdentityService(),
			staticBuilder,
			c.CachePolicy(cachepolicy.PolicyScaleList),
		)
	}

	c.surveyScaleInfra = &surveyScaleInfra{
		questionnaireRepo:   questionnaireRepo,
		questionnaireReader: questionnaireReader,
		answerSheetRepo:     answerSheetRepo,
		answerSheetReader:   answerSheetReader,
		scaleRepo:           scaleRepo,
		scaleReader:         scaleReader,
		scaleListCache:      scaleListCache,
	}
	return c.surveyScaleInfra, nil
}
