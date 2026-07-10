package survey

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	answerSheetMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	questionnaireMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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
	AssessmentModelRepo modelcatalogport.ModelRepository
}

// ScaleInfraDeps collects infrastructure inputs for EnsureScaleInfra.
type ScaleInfraDeps struct {
	MongoDB             *mongo.Database
	EventCatalog        *eventcatalog.Catalog
	MongoLimiter        backpressure.Acquirer
	StaticRedis         redis.UniversalClient
	StaticBuilder       *keyspace.Builder
	QuestionnairePolicy cachepolicy.CachePolicy
	Observer            *observability.ComponentObserver
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

	assessmentModelRepo := mongomodelcatalog.NewDraftRepository(deps.MongoDB, mongoOpts)

	return &ScaleInfra{
		QuestionnaireRepo:   questionnaireRepo,
		QuestionnaireReader: questionnaireReader,
		AnswerSheetRepo:     answerSheetRepo,
		AnswerSheetReader:   answerSheetReader,
		AssessmentModelRepo: assessmentModelRepo,
	}, nil
}
