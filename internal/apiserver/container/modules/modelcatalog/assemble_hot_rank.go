package modelcatalog

import (
	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	modelcatalogHotRankInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
)

// HotRank assembles the catalog hot-rank read model. It contains no authoring
// or lifecycle command service.
type HotRank struct {
	ReadModel hotrank.ReadModel
}

type HotRankDeps struct {
	RedisClient redis.UniversalClient
	KeyBuilder  *keyspace.Builder
}

func NewHotRank(deps HotRankDeps) *HotRank {
	return &HotRank{ReadModel: modelcatalogHotRankInfra.NewRedisScaleHotRankProjection(deps.RedisClient, deps.KeyBuilder)}
}

func (*HotRank) Cleanup() error     { return nil }
func (*HotRank) CheckHealth() error { return nil }

func (*HotRank) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{Name: "modelcatalog.hot_rank", Version: "2.0.0", Description: "测评模型目录热度读模型"}
}
