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
	if errs := opts.AIExplanation.Validate(); len(errs) != 0 {
		t.Fatalf("disabled validation errors = %v", errs)
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
	opts.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg = 100
	if joined := errorsText(opts.Validate()); !strings.Contains(joined, "positive multiple of 70") {
		t.Fatalf("daily budget validation errors = %v", opts.Validate())
	}

	opts.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg = 140
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("valid v1 capacity options = %v", errs)
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

func TestAIExplanationEnabledRequiresCredentialAndDelegatedSubject(t *testing.T) {
	opts := NewOptions()
	opts.AIExplanation.Enabled = true
	opts.AIExplanation.Model = "gpt-test-snapshot"
	errs := opts.Validate()
	joined := errorsText(errs)
	if !strings.Contains(joined, "API key") || !strings.Contains(joined, "delegated_subject.enabled") {
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
