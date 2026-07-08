package evaluation

import (
	"fmt"
	"os"

	redis "github.com/redis/go-redis/v9"

	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	rediseval "github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

type scoringSnapshotStoreConfig struct {
	AsyncInterpretation  bool
	SingleProcessAsync   bool
	OpsRedis             redis.UniversalClient
	OpsUnavailableReason string
}

func resolveScoringSnapshotStore(cfg scoringSnapshotStoreConfig) (outcomescoring.SnapshotStore, error) {
	if cfg.OpsRedis != nil {
		return rediseval.NewRedisScoringSnapshotStore(cfg.OpsRedis), nil
	}
	if cfg.AsyncInterpretation && !cfg.SingleProcessAsync {
		reason := cfg.OpsUnavailableReason
		if reason == "" {
			reason = "ops_runtime redis client is unavailable"
		}
		return nil, fmt.Errorf(
			"async interpretation requires durable scoring snapshot store (ops redis): %s; "+
				"set EVALUATION_SINGLE_PROCESS_ASYNC=true for single-process dev/test only",
			reason,
		)
	}
	return outcomescoring.NewMemorySnapshotStore(), nil
}

func opsUnavailableReason(handle *cacheplane.Handle) string {
	if handle == nil {
		return "ops_runtime handle is nil"
	}
	if handle.Client != nil {
		return ""
	}
	if handle.LastError != nil {
		return handle.LastError.Error()
	}
	if handle.Degraded {
		return fmt.Sprintf("ops_runtime redis is degraded (profile=%s mode=%s)", handle.Profile, handle.Mode)
	}
	return "ops_runtime redis client is nil"
}

func singleProcessAsyncFromEnv() bool {
	switch os.Getenv("EVALUATION_SINGLE_PROCESS_ASYNC") {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	default:
		return false
	}
}
