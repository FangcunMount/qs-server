package assessmentmodel

import (
	scaleLifecycle "github.com/FangcunMount/qs-server/internal/apiserver/application/scale/lifecycle"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoassessmentmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/assessmentmodel"
	mongoruleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// WireInput carries composition-root inputs for assessment-model installation.
type WireInput struct {
	MongoDB                *mongo.Database
	MongoLimiter           backpressure.Acquirer
	EventPublisher         event.EventPublisher
	RankRedisClient        redis.UniversalClient
	RankCacheBuilder       *keyspace.Builder
	IdentityService        *iam.IdentityService
	HotsetRecorder         cachetarget.HotsetRecorder
	CacheSignalNotifier    scaleLifecycle.CacheSignalNotifier
	ScaleInfra             *surveymod.ScaleInfra
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
}

// Wire builds and bootstraps the assessment-model module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	surveyPorts := SurveyBootstrapPorts{}
	if in.QuestionnairePublisher != nil {
		surveyPorts.QuestionnairePublisher = in.QuestionnairePublisher
	}
	if infra := in.ScaleInfra; infra != nil {
		surveyPorts.QuestionnaireCatalog = quesApp.NewPublishedQuestionnaireCatalog(infra.QuestionnaireRepo)
	}
	return Bootstrap(BootstrapInput{
		Scale:       buildScaleDeps(in),
		Personality: buildPersonalityDeps(in.MongoDB, in.MongoLimiter),
		Survey:      surveyPorts,
	})
}

func buildScaleDeps(in WireInput) ScaleDeps {
	deps := ScaleDeps{
		EventPublisher:      in.EventPublisher,
		RankRedisClient:     in.RankRedisClient,
		RankCacheBuilder:    in.RankCacheBuilder,
		IdentityService:     in.IdentityService,
		HotsetRecorder:      in.HotsetRecorder,
		CacheSignalNotifier: in.CacheSignalNotifier,
	}
	if infra := in.ScaleInfra; infra != nil {
		deps.Repo = infra.ScaleRepo
		deps.Reader = infra.ScaleReader
		deps.ListCache = infra.ScaleListCache
		deps.HotListCache = infra.ScaleHotListCache
		deps.QuestionnaireCatalog = quesApp.NewPublishedQuestionnaireCatalog(infra.QuestionnaireRepo)
	}
	if in.MongoDB != nil {
		mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: in.MongoLimiter}
		v2Repo := mongoassessmentmodel.NewRepository(in.MongoDB, mongoOpts)
		deps.RuleSetPublisher = rulesetInfra.NewScaleRuleSetPublisher(v2Repo)
	}
	return deps
}

func buildPersonalityDeps(mongoDB *mongo.Database, mongoLimiter backpressure.Acquirer) PersonalityDeps {
	if mongoDB == nil {
		return PersonalityDeps{}
	}
	mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: mongoLimiter}
	v2Repo := mongoassessmentmodel.NewRepository(mongoDB, mongoOpts)
	draftRepo := mongoassessmentmodel.NewDraftRepository(mongoDB, mongoOpts)
	publishedRepo := mongoassessmentmodel.NewPublishedModelRepoAdapter(v2Repo)
	legacyRepo := mongoruleset.NewRepository(mongoDB, mongoOpts)
	dualStore := aminfra.NewDualStore(v2Repo, legacyRepo)
	return PersonalityDeps{
		PublishedLister:          dualStore,
		PublishedAlgorithmLister: dualStore,
		ModelRepo:                draftRepo,
		PublishedRepo:            publishedRepo,
	}
}
