package configcontract

import (
	"path/filepath"
	"testing"
	"time"

	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
)

func TestProductionAIExplanationDeepSeekRouteV3Contract(t *testing.T) {
	apiOptions := apiserveroptions.NewOptions()
	loadConfig(t, filepath.Join(repoRoot(t), "configs", "apiserver.prod.yaml"), apiOptions)
	workerOptions := workeroptions.NewOptions()
	loadConfig(t, filepath.Join(repoRoot(t), "configs", "worker.prod.yaml"), workerOptions)

	ai := apiOptions.AIExplanation
	if ai.RouteRevision != "v3" || ai.Evaluation.RouteRevision != "v2" {
		t.Fatalf("production AI route revisions = %q/%q, want v3/v2", ai.RouteRevision, ai.Evaluation.RouteRevision)
	}
	if ai.ReasoningEffort != "none" || ai.Evaluation.ReasoningEffort != "low" {
		t.Fatalf("production AI reasoning efforts = %q/%q, want none/low", ai.ReasoningEffort, ai.Evaluation.ReasoningEffort)
	}
	if ai.MaxOutputTokens < 12000 || ai.Evaluation.MaxOutputTokens < 8000 {
		t.Fatalf("production AI output token limits = %d/%d, want at least 12000/8000", ai.MaxOutputTokens, ai.Evaluation.MaxOutputTokens)
	}
	if ai.Timeout < 2*time.Minute || ai.Evaluation.Timeout < 3*time.Minute {
		t.Fatalf("production AI Provider timeouts = %s/%s, want at least 2m/3m", ai.Timeout, ai.Evaluation.Timeout)
	}

	providerWindow := ai.Timeout + ai.Evaluation.Timeout
	workerDeadline := workerOptions.GRPC.AIExplanationTimeout
	if workerDeadline < providerWindow+time.Minute {
		t.Fatalf("production worker AI timeout = %s, must cover Provider window %s plus commit grace", workerDeadline, providerWindow)
	}
	if ai.Evaluation.AttemptLeaseDuration <= workerDeadline {
		t.Fatalf("production attempt lease = %s, must exceed worker deadline %s", ai.Evaluation.AttemptLeaseDuration, workerDeadline)
	}
	if workerOptions.Messaging.NSQMessageTimeout <= workerDeadline {
		t.Fatalf("production NSQ timeout = %s, must exceed worker deadline %s", workerOptions.Messaging.NSQMessageTimeout, workerDeadline)
	}
}
