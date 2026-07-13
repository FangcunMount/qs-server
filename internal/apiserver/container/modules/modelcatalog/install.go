package modelcatalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost 扩展共享的容器组合接缝与模型目录绑定
type InstallHost interface {
	compose.Host
	EnsureSurveyRuntimeInfra() (*surveymod.SurveyRuntimeInfra, error)
	SetAssessmentModelModule(*Module)
}

// InstallFrom 使用容器组合根的输入连接和注册模型目录模块
func InstallFrom(host InstallHost) error {
	infra, err := host.EnsureSurveyRuntimeInfra()
	if err != nil {
		return err
	}
	module, err := Wire(WireInput{
		MongoDB:                host.MongoDB(),
		MongoLimiter:           host.MongoLimiter(),
		EventPublisher:         host.EventPublisher(),
		RankRedisClient:        host.CacheClient(redisruntime.FamilyRank),
		RankCacheBuilder:       host.CacheBuilder(redisruntime.FamilyRank),
		CacheSignalNotifier:    host.CacheSignalNotifier(),
		SurveyRuntimeInfra:     infra,
		QuestionnairePublisher: host.SurveyPorts().QuestionnairePublisher,
		QuestionnaireQuery:     host.SurveyPorts().QuestionnaireQuery,
		StaticRedisClient:      host.CacheClient(redisruntime.FamilyStatic),
		StaticCacheBuilder:     host.CacheBuilder(redisruntime.FamilyStatic),
		PublishedModelPolicy:   host.CachePolicy(cachepolicy.CapabilityModelCatalogPublished),
		CacheObserver:          host.CacheObserver(),
	})
	if err != nil {
		return err
	}
	host.SetAssessmentModelModule(module)
	host.Printf("📦 Assessment model module initialized (scale + typology catalog)\n")
	return nil
}
