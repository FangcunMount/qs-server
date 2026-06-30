package survey

import (
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// WireInput carries composition-root inputs for survey module installation.
type WireInput struct {
	MongoDB                           *mongo.Database
	EventPublisher                    event.EventPublisher
	RankRedisClient                   redis.UniversalClient
	RankCacheBuilder                  *keyspace.Builder
	IdentityService                   *iam.IdentityService
	HotsetRecorder                    cachetarget.HotsetRecorder
	TopicResolver                     eventcatalog.TopicResolver
	OutboxRelayBatchSize              int
	OutboxRelayPublishWorkers         int
	OutboxRelayImmediateMaxConcurrent int
	CacheSignalNotifier               quesApp.CacheSignalNotifier
	OpsHandle                         *cacheplane.Handle
	ScaleInfra                        *ScaleInfra
}

// Wire builds and bootstraps the survey module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	bootstrap := BootstrapInput{
		MongoDB:                           in.MongoDB,
		EventPublisher:                    in.EventPublisher,
		RankRedisClient:                   in.RankRedisClient,
		RankCacheBuilder:                  in.RankCacheBuilder,
		IdentityService:                   in.IdentityService,
		HotsetRecorder:                    in.HotsetRecorder,
		TopicResolver:                     in.TopicResolver,
		ScaleSyncer:                       scaleApp.NewQuestionnaireBindingSyncer(nil),
		OutboxRelayBatchSize:              in.OutboxRelayBatchSize,
		OutboxRelayPublishWorkers:         in.OutboxRelayPublishWorkers,
		OutboxRelayImmediateMaxConcurrent: in.OutboxRelayImmediateMaxConcurrent,
		CacheSignalNotifier:               in.CacheSignalNotifier,
		OpsHandle:                         in.OpsHandle,
	}
	if infra := in.ScaleInfra; infra != nil {
		bootstrap.ScaleSyncer = scaleApp.NewQuestionnaireBindingSyncer(infra.ScaleRepo)
		bootstrap.QuestionnaireRepo = infra.QuestionnaireRepo
		bootstrap.QuestionnaireReader = infra.QuestionnaireReader
		bootstrap.AnswerSheetRepo = infra.AnswerSheetRepo
		bootstrap.AnswerSheetReader = infra.AnswerSheetReader
	}
	return Bootstrap(bootstrap)
}
