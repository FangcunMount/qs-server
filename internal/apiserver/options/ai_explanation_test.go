package options

import (
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	"github.com/FangcunMount/qs-server/pkg/configmask"
)

func TestAIExplanationDisabledPreservesStandardRuntimeDefaults(t *testing.T) {
	opts := NewOptions()
	if opts.AIExplanation == nil || opts.AIExplanation.Enabled {
		t.Fatalf("AI explanation defaults = %#v", opts.AIExplanation)
	}
	if opts.AIExplanation.Evaluation.Enabled {
		t.Fatalf("AI explanation evaluation must default disabled: %#v", opts.AIExplanation.Evaluation)
	}
	if opts.AIExplanation.ParticipantEnabled {
		t.Fatal("AI explanation participant traffic must default disabled")
	}
	if opts.AIExplanation.Provider != AIExplanationProviderDeepSeek || opts.AIExplanation.Model != DefaultAIExplanationDeepSeekModel {
		t.Fatalf("AI explanation primary provider/model defaults = %#v", opts.AIExplanation)
	}
	if errs := opts.AIExplanation.Validate(); len(errs) != 0 {
		t.Fatalf("disabled validation errors = %v", errs)
	}
}

func TestAIExplanationParticipantTrafficRequiresParentRuntime(t *testing.T) {
	opts := NewAIExplanationOptions()
	opts.ParticipantEnabled = true
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "participant_enabled requires ai_explanation.enabled") {
		t.Fatalf("disabled parent validation errors = %v", opts.Validate())
	}

	opts.Enabled = true
	opts.APIKey = "provider-test-secret"
	configureAIExplanationTestLifecycle(opts)
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("valid participant rollout options = %v", errs)
	}
}

func TestAIExplanationSupportsOpenAIAndDeepSeekProviderCredentials(t *testing.T) {
	for _, testCase := range []struct {
		provider string
		envName  string
	}{
		{provider: AIExplanationProviderOpenAI, envName: "OPENAI_API_KEY"},
		{provider: AIExplanationProviderDeepSeek, envName: "DEEPSEEK_API_KEY"},
	} {
		t.Run(testCase.provider, func(t *testing.T) {
			opts := NewAIExplanationOptions()
			opts.Provider = testCase.provider
			opts.completeAPIKey(func(name string) string {
				if name == testCase.envName {
					return "provider-test-secret"
				}
				return "wrong-provider-secret"
			})
			if opts.APIKey != "provider-test-secret" {
				t.Fatalf("resolved API key for %s", testCase.provider)
			}
		})
	}

	opts := NewAIExplanationOptions()
	opts.Provider = AIExplanationProviderDeepSeek
	opts.APIKey = "explicit-test-secret"
	opts.completeAPIKey(func(string) string { return "environment-test-secret" })
	if opts.APIKey != "explicit-test-secret" {
		t.Fatalf("explicit API key must take precedence, got %q", opts.APIKey)
	}

	opts = NewAIExplanationOptions()
	opts.Provider = "unsupported"
	opts.Enabled = true
	opts.Model = "model"
	opts.APIKey = "secret"
	configureAIExplanationTestLifecycle(opts)
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "one of openai or deepseek") {
		t.Fatalf("unsupported Provider errors = %v", opts.Validate())
	}
}

func TestAIExplanationEvaluationRequiresIndependentModelAndParentRuntime(t *testing.T) {
	opts := NewAIExplanationOptions()
	opts.Evaluation.Enabled = true
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "requires ai_explanation.enabled") {
		t.Fatalf("disabled parent validation errors = %v", opts.Validate())
	}

	opts.Enabled = true
	opts.Model = "generation-model-snapshot"
	opts.APIKey = "sk-test-sensitive-value"
	configureAIExplanationTestLifecycle(opts)
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "evaluation.model") {
		t.Fatalf("missing evaluator model errors = %v", opts.Validate())
	}

	opts.Evaluation.Model = "independent-judge-model-snapshot"
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("valid independent evaluation options = %v", errs)
	}
}

func TestAIExplanationEvaluationAttemptLeaseCoversBothProviderStages(t *testing.T) {
	t.Parallel()

	opts := NewAIExplanationOptions()
	if opts.Evaluation.AttemptLeaseDuration != 4*time.Minute {
		t.Fatalf("default evaluation attempt lease = %s, want 4m", opts.Evaluation.AttemptLeaseDuration)
	}
	if opts.Evaluation.AttemptLeaseDuration < opts.Timeout+opts.Evaluation.Timeout+time.Minute {
		t.Fatalf("default evaluation attempt lease = %s, does not cover both Provider stages", opts.Evaluation.AttemptLeaseDuration)
	}

	opts.Enabled = true
	opts.APIKey = "provider-test-secret"
	configureAIExplanationTestLifecycle(opts)
	opts.Evaluation.Enabled = true
	opts.Evaluation.Model = "independent-judge-model-snapshot"
	opts.Evaluation.AttemptLeaseDuration = opts.Timeout + opts.Evaluation.Timeout
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "attempt_lease_duration") {
		t.Fatalf("short attempt lease validation errors = %v", opts.Validate())
	}
}

func TestAIExplanationRejectsUnknownReasoningEffort(t *testing.T) {
	opts := NewAIExplanationOptions()
	opts.Enabled = true
	opts.APIKey = "provider-test-secret"
	configureAIExplanationTestLifecycle(opts)
	opts.ReasoningEffort = "extreme"
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "reasoning_effort is invalid") {
		t.Fatalf("generation reasoning validation errors = %v", opts.Validate())
	}

	opts.ReasoningEffort = "low"
	opts.Evaluation.Enabled = true
	opts.Evaluation.Model = "independent-judge-model-snapshot"
	opts.Evaluation.ReasoningEffort = "extreme"
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "evaluation.reasoning_effort is invalid") {
		t.Fatalf("evaluation reasoning validation errors = %v", opts.Validate())
	}

	opts.Evaluation.ReasoningEffort = "none"
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("valid reasoning efforts = %v", errs)
	}
}

func TestAIExplanationEvaluationCapacityUsesFixedV1RunCeiling(t *testing.T) {
	opts := NewAIExplanationOptions()
	opts.Enabled = true
	opts.Model = "generation-model-snapshot"
	opts.APIKey = "sk-test-sensitive-value"
	configureAIExplanationTestLifecycle(opts)
	opts.Evaluation.Enabled = true
	opts.Evaluation.Model = "independent-judge-model-snapshot"

	opts.Evaluation.Capacity.MaxActiveRunsPerOrg = 2
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "max_active_runs_per_org must equal 1") {
		t.Fatalf("concurrency validation errors = %v", opts.Validate())
	}

	opts.Evaluation.Capacity.MaxActiveRunsPerOrg = 1
	opts.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg = 69
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "must be at least 70") {
		t.Fatalf("daily budget validation errors = %v", opts.Validate())
	}

	opts.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg = 1024
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("valid v1 capacity options with partial remainder = %v", errs)
	}
}

func TestAIExplanationParticipantCapacityRequiresNestedPositiveBudgets(t *testing.T) {
	opts := NewAIExplanationOptions()
	opts.Enabled = true
	opts.Model = "generation-model-snapshot"
	opts.APIKey = "sk-test-sensitive-value"
	configureAIExplanationTestLifecycle(opts)

	opts.ParticipantCapacity.DailyProviderInvocationBudgetPerUser = 0
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "participant_capacity daily Provider invocation budgets must be positive") {
		t.Fatalf("invalid participant capacity errors = %v", opts.Validate())
	}

	opts.ParticipantCapacity.DailyProviderInvocationBudgetPerUser = 501
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "organization budget must cover user and Assessment budgets") {
		t.Fatalf("invalid participant hierarchy errors = %v", opts.Validate())
	}

	opts.ParticipantCapacity.DailyProviderInvocationBudgetPerUser = 5
	opts.ParticipantCapacity.MaxActiveProviderExecutionsPerUser = 0
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "active Provider execution limits must be positive") {
		t.Fatalf("invalid participant active capacity errors = %v", opts.Validate())
	}

	opts.ParticipantCapacity.MaxActiveProviderExecutionsPerUser = 11
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "organization active limit must cover user and Assessment limits") {
		t.Fatalf("invalid participant active hierarchy errors = %v", opts.Validate())
	}

	opts.ParticipantCapacity.MaxActiveProviderExecutionsPerUser = 2
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("valid participant capacity options = %v", errs)
	}
}

func TestAIExplanationEnabledRequiresExplicitDataLifecyclePolicy(t *testing.T) {
	opts := NewAIExplanationOptions()
	opts.Enabled = true
	opts.Model = "generation-model-snapshot"
	opts.APIKey = "sk-test-sensitive-value"
	joined := errorsText(opts.Validate())
	for _, required := range []string{
		"data_lifecycle.policy_version", "participant_record_retention",
		"prompt_evaluation_retention", "capacity_ledger_retention",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing lifecycle validation %q in %v", required, opts.Validate())
		}
	}
	configureAIExplanationTestLifecycle(opts)
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("complete lifecycle validation errors = %v", errs)
	}
}

func TestAIExplanationParticipantEnabledRequiresCredentialAndDelegatedSubject(t *testing.T) {
	opts := NewOptions()
	opts.AIExplanation.Enabled = true
	opts.AIExplanation.ParticipantEnabled = true
	opts.AIExplanation.Model = "gpt-test-snapshot"
	errs := opts.Validate()
	joined := errorsText(errs)
	if !strings.Contains(joined, "API key") || !strings.Contains(joined, "delegated_subject.enabled must be true when ai_explanation.participant_enabled is true") {
		t.Fatalf("validation errors = %v", errs)
	}

	opts.AIExplanation.APIKey = "sk-test-sensitive-value"
	configureAIExplanationTestLifecycle(opts.AIExplanation)
	opts.DelegatedSubject = &delegatedsubject.Options{Enabled: true, CurrentKey: "delegated-test-key", TTL: time.Minute}
	if errs := opts.AIExplanation.Validate(); len(errs) != 0 {
		t.Fatalf("AI validation errors = %v", errs)
	}
	if rendered := configmask.String(opts); strings.Contains(rendered, opts.AIExplanation.APIKey) || strings.Contains(rendered, "sk-test") {
		t.Fatalf("rendered options leaked API key: %s", rendered)
	}
}

func TestAIExplanationGovernanceDoesNotRequireParticipantDelegation(t *testing.T) {
	opts := NewOptions()
	opts.AIExplanation.Enabled = true
	opts.AIExplanation.APIKey = "provider-test-secret"
	configureAIExplanationTestLifecycle(opts.AIExplanation)

	if joined := errorsText(opts.Validate()); strings.Contains(joined, "delegated_subject.enabled") {
		t.Fatalf("governance-only runtime must not require participant delegation: %v", opts.Validate())
	}
}

func configureAIExplanationTestLifecycle(opts *AIExplanationOptions) {
	opts.DataLifecycle.PolicyVersion = "test-policy-v1"
	opts.DataLifecycle.ParticipantRecordRetention = 30 * 24 * time.Hour
	opts.DataLifecycle.PromptEvaluationRetention = 90 * 24 * time.Hour
	opts.DataLifecycle.CapacityLedgerRetention = 7 * 24 * time.Hour
}

func errorsText(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "\n")
}
