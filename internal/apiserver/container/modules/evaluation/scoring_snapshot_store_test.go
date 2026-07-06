package evaluation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	evaluationscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scoring"
	rediseval "github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestResolveScoringSnapshotStoreCapabilityMatrix(t *testing.T) {
	t.Parallel()

	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}
	t.Cleanup(mini.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	tests := []struct {
		name               string
		cfg                scoringSnapshotStoreConfig
		wantErrCode        int
		wantMemoryFallback bool
		wantRedisStore     bool
	}{
		{
			name: "sync mode allows in-process store without ops redis",
			cfg: scoringSnapshotStoreConfig{
				AsyncInterpretation: false,
			},
			wantMemoryFallback: true,
		},
		{
			name: "async mode rejects in-process store without explicit single-process opt-in",
			cfg: scoringSnapshotStoreConfig{
				AsyncInterpretation: true,
			},
			wantErrCode: code.ErrModuleInitializationFailed,
		},
		{
			name: "async mode allows in-process store when single-process opt-in is set",
			cfg: scoringSnapshotStoreConfig{
				AsyncInterpretation: true,
				SingleProcessAsync:  true,
			},
			wantMemoryFallback: true,
		},
		{
			name: "ops redis is preferred for async mode",
			cfg: scoringSnapshotStoreConfig{
				AsyncInterpretation: true,
				OpsRedis:            redisClient,
			},
			wantRedisStore: true,
		},
		{
			name: "ops redis is used even when sync mode",
			cfg: scoringSnapshotStoreConfig{
				AsyncInterpretation: false,
				OpsRedis:            redisClient,
			},
			wantRedisStore: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := resolveScoringSnapshotStore(tt.cfg)
			if tt.wantErrCode != 0 {
				if err == nil {
					t.Fatal("resolveScoringSnapshotStore() error = nil, want failure")
				}
				if !cberrors.IsCode(err, tt.wantErrCode) {
					t.Fatalf("resolveScoringSnapshotStore() code = %v, want %v", cberrors.ParseCoder(err), tt.wantErrCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveScoringSnapshotStore() error = %v", err)
			}
			if store == nil {
				t.Fatal("resolveScoringSnapshotStore() store = nil")
			}

			_, isMemory := store.(*evaluationscoring.MemoryScoringSnapshotStore)
			_, isRedis := store.(*rediseval.RedisScoringSnapshotStore)
			if tt.wantMemoryFallback && !isMemory {
				t.Fatalf("store type = %T, want memory fallback", store)
			}
			if tt.wantRedisStore && !isRedis {
				t.Fatalf("store type = %T, want redis store", store)
			}
		})
	}
}

func TestSingleProcessAsyncFromEnv(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "1", want: true},
		{value: "true", want: true},
		{value: "yes", want: true},
		{value: "0", want: false},
		{value: "", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("EVALUATION_SINGLE_PROCESS_ASYNC", tt.value)
			if got := singleProcessAsyncFromEnv(); got != tt.want {
				t.Fatalf("singleProcessAsyncFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssembleUsesScoringSnapshotResolver(t *testing.T) {
	t.Parallel()

	path := assembleSourcePath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "resolveScoringSnapshotStore") {
		t.Fatal("evaluation assemble must call resolveScoringSnapshotStore")
	}
	if !strings.Contains(text, "SingleProcessAsyncInterpretation") {
		t.Fatal("evaluation assemble must wire single-process async opt-in")
	}
}

func assembleSourcePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	return filepath.Join(filepath.Dir(file), "assemble.go")
}
