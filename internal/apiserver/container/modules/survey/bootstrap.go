package survey

import (
	"go.mongodb.org/mongo-driver/mongo"

	redis "github.com/redis/go-redis/v9"

	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// BootstrapInput carries container integration inputs for survey module bootstrap.
type BootstrapInput struct {
	MongoDB              *mongo.Database
	EventPublisher       event.EventPublisher
	RankRedisClient      redis.UniversalClient
	RankCacheBuilder     *keyspace.Builder
	IdentityService      *iam.IdentityService
	HotsetRecorder       cachetarget.HotsetRecorder
	TopicResolver        eventcatalog.TopicResolver
	ScaleSyncer          quesApp.ScaleQuestionnaireBindingSyncer
	QuestionnaireRepo    questionnaire.Repository
	QuestionnaireReader  surveyreadmodel.QuestionnaireReader
	AnswerSheetRepo      AnswerSheetStore
	AnswerSheetReader    surveyreadmodel.AnswerSheetReader
	OutboxRelayBatchSize int
	CacheSignalNotifier  quesApp.CacheSignalNotifier
	OpsHandle            *cacheplane.Handle
}

// Bootstrap assembles the survey module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	scaleSyncer := in.ScaleSyncer
	if scaleSyncer == nil {
		scaleSyncer = scaleApp.NewQuestionnaireBindingSyncer(nil)
	}
	return New(Deps{
		MongoDB:              in.MongoDB,
		EventPublisher:       in.EventPublisher,
		RankRedisClient:      in.RankRedisClient,
		RankCacheBuilder:     in.RankCacheBuilder,
		IdentityService:      in.IdentityService,
		HotsetRecorder:       in.HotsetRecorder,
		TopicResolver:        in.TopicResolver,
		ScaleSyncer:          scaleSyncer,
		QuestionnaireRepo:    in.QuestionnaireRepo,
		QuestionnaireReader:  in.QuestionnaireReader,
		AnswerSheetRepo:      in.AnswerSheetRepo,
		AnswerSheetReader:    in.AnswerSheetReader,
		OutboxRelayBatchSize: in.OutboxRelayBatchSize,
		CacheSignalNotifier:  in.CacheSignalNotifier,
		OpsHandle:            in.OpsHandle,
	})
}
