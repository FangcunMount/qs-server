package evaluation

import (
	"os"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	rediseval "github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type scoringSnapshotStoreConfig struct {
	AsyncInterpretation bool
	SingleProcessAsync  bool
	OpsRedis            redis.UniversalClient
}

func resolveScoringSnapshotStore(cfg scoringSnapshotStoreConfig) (outcomescoring.SnapshotStore, error) {
	if cfg.OpsRedis != nil {
		return rediseval.NewRedisScoringSnapshotStore(cfg.OpsRedis), nil
	}
	if cfg.AsyncInterpretation && !cfg.SingleProcessAsync {
		return nil, errors.WithCode(
			code.ErrModuleInitializationFailed,
			"async interpretation requires durable scoring snapshot store (ops redis); "+
				"set EVALUATION_SINGLE_PROCESS_ASYNC=true for single-process dev/test only",
		)
	}
	return outcomescoring.NewMemorySnapshotStore(), nil
}

func singleProcessAsyncFromEnv() bool {
	switch os.Getenv("EVALUATION_SINGLE_PROCESS_ASYNC") {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	default:
		return false
	}
}
