package options

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

const (
	DefaultAIExplanationProviderRoute                   = "balanced_text_v1"
	DefaultAIExplanationSemanticProviderRoute           = "semantic_judge_v1"
	AIExplanationProviderOpenAI                         = "openai"
	AIExplanationProviderDeepSeek                       = "deepseek"
	AIExplanationProviderProtocolResponses              = "responses"
	AIExplanationProviderProtocolDeepSeekStrictToolCall = "deepseek_strict_tool_call"
	DefaultAIExplanationDeepSeekModel                   = "deepseek-v4-flash"
	AIExplanationStructuredOutputJSONSchema             = "json_schema"
	AIExplanationStructuredOutputJSONObject             = "json_object"
)

// AIExplanationOptions controls the optional, manually triggered AI
// explanation runtime. APIKey is deliberately excluded from JSON rendering so
// startup configuration diagnostics cannot print it.
type AIExplanationOptions struct {
	Enabled              bool                                    `json:"enabled" mapstructure:"enabled"`
	ParticipantEnabled   bool                                    `json:"participant_enabled" mapstructure:"participant_enabled"`
	Provider             string                                  `json:"provider" mapstructure:"provider"`
	ProviderProtocol     string                                  `json:"provider_protocol" mapstructure:"provider_protocol"`
	Model                string                                  `json:"model" mapstructure:"model"`
	RouteRevision        string                                  `json:"route_revision" mapstructure:"route_revision"`
	StructuredOutputMode string                                  `json:"structured_output_mode" mapstructure:"structured_output_mode"`
	Endpoint             string                                  `json:"endpoint,omitempty" mapstructure:"endpoint"`
	APIKey               string                                  `json:"-" mapstructure:"api_key"`
	Timeout              time.Duration                           `json:"timeout" mapstructure:"timeout"`
	RunLeaseDuration     time.Duration                           `json:"run_lease_duration" mapstructure:"run_lease_duration"`
	MaxOutputTokens      int                                     `json:"max_output_tokens" mapstructure:"max_output_tokens"`
	ReasoningEffort      string                                  `json:"reasoning_effort,omitempty" mapstructure:"reasoning_effort"`
	MaxResponseBytes     int64                                   `json:"max_response_bytes" mapstructure:"max_response_bytes"`
	DataLifecycle        AIExplanationDataLifecycleOptions       `json:"data_lifecycle" mapstructure:"data_lifecycle"`
	ParticipantCapacity  AIExplanationParticipantCapacityOptions `json:"participant_capacity" mapstructure:"participant_capacity"`
	Evaluation           AIExplanationEvaluationOptions          `json:"evaluation" mapstructure:"evaluation"`
}

// AIExplanationDataLifecycleOptions records an explicitly approved retention
// policy. The application deliberately has no legal/product default: a runtime
// cannot be enabled until all durations and the policy version are supplied.
type AIExplanationDataLifecycleOptions struct {
	PolicyVersion              string        `json:"policy_version" mapstructure:"policy_version"`
	ParticipantRecordRetention time.Duration `json:"participant_record_retention" mapstructure:"participant_record_retention"`
	PromptEvaluationRetention  time.Duration `json:"prompt_evaluation_retention" mapstructure:"prompt_evaluation_retention"`
	CapacityLedgerRetention    time.Duration `json:"capacity_ledger_retention" mapstructure:"capacity_ledger_retention"`
}

// AIExplanationParticipantCapacityOptions controls conservative per-attempt
// cost and execution admission. The initial Generation and every governed
// retry reserve exactly one Provider invocation against all three UTC-day
// ceilings; each Run also acquires one distributed active slot before dispatch.
type AIExplanationParticipantCapacityOptions struct {
	DailyProviderInvocationBudgetPerOrg        int `json:"daily_provider_invocation_budget_per_org" mapstructure:"daily_provider_invocation_budget_per_org"`
	DailyProviderInvocationBudgetPerUser       int `json:"daily_provider_invocation_budget_per_user" mapstructure:"daily_provider_invocation_budget_per_user"`
	DailyProviderInvocationBudgetPerAssessment int `json:"daily_provider_invocation_budget_per_assessment" mapstructure:"daily_provider_invocation_budget_per_assessment"`
	MaxActiveProviderExecutionsPerOrg          int `json:"max_active_provider_executions_per_org" mapstructure:"max_active_provider_executions_per_org"`
	MaxActiveProviderExecutionsPerUser         int `json:"max_active_provider_executions_per_user" mapstructure:"max_active_provider_executions_per_user"`
	MaxActiveProviderExecutionsPerAssessment   int `json:"max_active_provider_executions_per_assessment" mapstructure:"max_active_provider_executions_per_assessment"`
}

// AIExplanationEvaluationOptions controls the operator-only synthetic release
// evaluation. It reuses the configured Provider endpoint and credential, but
// resolves a distinct Route/model and is never enabled implicitly with the
// participant runtime.
type AIExplanationEvaluationOptions struct {
	Enabled              bool                                   `json:"enabled" mapstructure:"enabled"`
	Model                string                                 `json:"model" mapstructure:"model"`
	ProviderProtocol     string                                 `json:"provider_protocol" mapstructure:"provider_protocol"`
	RouteRevision        string                                 `json:"route_revision" mapstructure:"route_revision"`
	StructuredOutputMode string                                 `json:"structured_output_mode" mapstructure:"structured_output_mode"`
	Endpoint             string                                 `json:"endpoint,omitempty" mapstructure:"endpoint"`
	Timeout              time.Duration                          `json:"timeout" mapstructure:"timeout"`
	AttemptLeaseDuration time.Duration                          `json:"attempt_lease_duration" mapstructure:"attempt_lease_duration"`
	MaxOutputTokens      int                                    `json:"max_output_tokens" mapstructure:"max_output_tokens"`
	ReasoningEffort      string                                 `json:"reasoning_effort,omitempty" mapstructure:"reasoning_effort"`
	Capacity             AIExplanationEvaluationCapacityOptions `json:"capacity" mapstructure:"capacity"`
}

// AIExplanationEvaluationCapacityOptions is a conservative v1 admission
// policy. One start reserves the complete 70-call generation/judge ceiling;
// any remainder below 70 cannot admit another run, and reservations are not
// refunded after cancellation.
type AIExplanationEvaluationCapacityOptions struct {
	MaxActiveRunsPerOrg                 int `json:"max_active_runs_per_org" mapstructure:"max_active_runs_per_org"`
	DailyProviderInvocationBudgetPerOrg int `json:"daily_provider_invocation_budget_per_org" mapstructure:"daily_provider_invocation_budget_per_org"`
}

func NewAIExplanationOptions() *AIExplanationOptions {
	return &AIExplanationOptions{
		Enabled: false, ParticipantEnabled: false,
		Provider: AIExplanationProviderDeepSeek, ProviderProtocol: AIExplanationProviderProtocolResponses,
		Model: DefaultAIExplanationDeepSeekModel, RouteRevision: "v1",
		StructuredOutputMode: AIExplanationStructuredOutputJSONSchema,
		Timeout:              60 * time.Second, RunLeaseDuration: 2 * time.Minute,
		MaxOutputTokens: 3000, MaxResponseBytes: 4 << 20,
		DataLifecycle: AIExplanationDataLifecycleOptions{},
		ParticipantCapacity: AIExplanationParticipantCapacityOptions{
			DailyProviderInvocationBudgetPerOrg: 500, DailyProviderInvocationBudgetPerUser: 5,
			DailyProviderInvocationBudgetPerAssessment: 3, MaxActiveProviderExecutionsPerOrg: 10,
			MaxActiveProviderExecutionsPerUser: 2, MaxActiveProviderExecutionsPerAssessment: 1,
		},
		Evaluation: AIExplanationEvaluationOptions{
			Enabled: false, ProviderProtocol: AIExplanationProviderProtocolResponses,
			RouteRevision: "v1", StructuredOutputMode: AIExplanationStructuredOutputJSONSchema, Timeout: 60 * time.Second,
			AttemptLeaseDuration: 4 * time.Minute, MaxOutputTokens: 2500,
			Capacity: AIExplanationEvaluationCapacityOptions{
				MaxActiveRunsPerOrg: 1, DailyProviderInvocationBudgetPerOrg: 140,
			},
		},
	}
}

func (o *AIExplanationOptions) completeAPIKey(getenv func(string) string) {
	if o == nil || strings.TrimSpace(o.APIKey) != "" || getenv == nil {
		return
	}
	switch o.Provider {
	case AIExplanationProviderOpenAI:
		o.APIKey = getenv("OPENAI_API_KEY")
	case AIExplanationProviderDeepSeek:
		o.APIKey = getenv("DEEPSEEK_API_KEY")
	}
}

func (o *AIExplanationOptions) Validate() []error {
	if o == nil {
		return nil
	}
	if !o.Enabled {
		if o.ParticipantEnabled {
			return []error{fmt.Errorf("ai_explanation.participant_enabled requires ai_explanation.enabled")}
		}
		if o.Evaluation.Enabled {
			return []error{fmt.Errorf("ai_explanation.evaluation requires ai_explanation.enabled")}
		}
		return nil
	}
	var errs []error
	switch o.Provider {
	case AIExplanationProviderOpenAI, AIExplanationProviderDeepSeek:
	default:
		errs = append(errs, fmt.Errorf("ai_explanation.provider must be one of openai or deepseek"))
	}
	if !validAIExplanationProviderProtocol(o.Provider, o.ProviderProtocol) {
		errs = append(errs, fmt.Errorf("ai_explanation.provider_protocol is incompatible with the configured provider"))
	}
	if strings.TrimSpace(o.Model) == "" {
		errs = append(errs, fmt.Errorf("ai_explanation.model is required when enabled"))
	}
	if strings.TrimSpace(o.APIKey) == "" {
		errs = append(errs, fmt.Errorf("ai_explanation API key is required when enabled"))
	}
	if strings.TrimSpace(o.RouteRevision) == "" {
		errs = append(errs, fmt.Errorf("ai_explanation.route_revision is required when enabled"))
	}
	if !validAIExplanationStructuredOutputMode(o.StructuredOutputMode) {
		errs = append(errs, fmt.Errorf("ai_explanation.structured_output_mode must be one of json_schema or json_object"))
	}
	if strings.TrimSpace(o.ProviderProtocol) == AIExplanationProviderProtocolDeepSeekStrictToolCall &&
		o.StructuredOutputMode != AIExplanationStructuredOutputJSONSchema {
		errs = append(errs, fmt.Errorf("ai_explanation deepseek strict tool protocol requires json_schema"))
	}
	if strings.TrimSpace(o.ProviderProtocol) == AIExplanationProviderProtocolDeepSeekStrictToolCall &&
		!isNonThinkingAIExplanationEffort(o.ReasoningEffort) {
		errs = append(errs, fmt.Errorf("ai_explanation deepseek strict tool protocol requires non-thinking reasoning_effort while named tool_choice is forced"))
	}
	if o.Timeout <= 0 || o.RunLeaseDuration <= o.Timeout {
		errs = append(errs, fmt.Errorf("ai_explanation.run_lease_duration must be greater than timeout"))
	}
	if o.MaxOutputTokens < 1 {
		errs = append(errs, fmt.Errorf("ai_explanation.max_output_tokens must be positive"))
	}
	if !validAIExplanationReasoningEffort(o.ReasoningEffort) {
		errs = append(errs, fmt.Errorf("ai_explanation.reasoning_effort is invalid"))
	}
	if o.MaxResponseBytes < 1 {
		errs = append(errs, fmt.Errorf("ai_explanation.max_response_bytes must be positive"))
	}
	errs = append(errs, o.DataLifecycle.validate()...)
	participantCapacity := o.ParticipantCapacity
	if participantCapacity.DailyProviderInvocationBudgetPerOrg < 1 ||
		participantCapacity.DailyProviderInvocationBudgetPerUser < 1 ||
		participantCapacity.DailyProviderInvocationBudgetPerAssessment < 1 {
		errs = append(errs, fmt.Errorf("ai_explanation.participant_capacity daily Provider invocation budgets must be positive"))
	}
	if participantCapacity.DailyProviderInvocationBudgetPerOrg < participantCapacity.DailyProviderInvocationBudgetPerUser ||
		participantCapacity.DailyProviderInvocationBudgetPerOrg < participantCapacity.DailyProviderInvocationBudgetPerAssessment {
		errs = append(errs, fmt.Errorf("ai_explanation.participant_capacity organization budget must cover user and Assessment budgets"))
	}
	if participantCapacity.MaxActiveProviderExecutionsPerOrg < 1 ||
		participantCapacity.MaxActiveProviderExecutionsPerUser < 1 ||
		participantCapacity.MaxActiveProviderExecutionsPerAssessment < 1 {
		errs = append(errs, fmt.Errorf("ai_explanation.participant_capacity active Provider execution limits must be positive"))
	}
	if participantCapacity.MaxActiveProviderExecutionsPerOrg < participantCapacity.MaxActiveProviderExecutionsPerUser ||
		participantCapacity.MaxActiveProviderExecutionsPerOrg < participantCapacity.MaxActiveProviderExecutionsPerAssessment {
		errs = append(errs, fmt.Errorf("ai_explanation.participant_capacity organization active limit must cover user and Assessment limits"))
	}
	if endpoint := strings.TrimSpace(o.Endpoint); endpoint != "" {
		parsed, err := url.ParseRequestURI(endpoint)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			errs = append(errs, fmt.Errorf("ai_explanation.endpoint must be an absolute https URL"))
		}
	}
	if o.Evaluation.Enabled {
		if strings.TrimSpace(o.Evaluation.Model) == "" {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.model is required when evaluation is enabled"))
		}
		if strings.TrimSpace(o.Evaluation.RouteRevision) == "" {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.route_revision is required when evaluation is enabled"))
		}
		if !validAIExplanationProviderProtocol(o.Provider, o.Evaluation.ProviderProtocol) {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.provider_protocol is incompatible with the configured provider"))
		}
		if !validAIExplanationStructuredOutputMode(o.Evaluation.StructuredOutputMode) {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.structured_output_mode must be one of json_schema or json_object"))
		}
		if strings.TrimSpace(o.Evaluation.ProviderProtocol) == AIExplanationProviderProtocolDeepSeekStrictToolCall &&
			o.Evaluation.StructuredOutputMode != AIExplanationStructuredOutputJSONSchema {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation deepseek strict tool protocol requires json_schema"))
		}
		if strings.TrimSpace(o.Evaluation.ProviderProtocol) == AIExplanationProviderProtocolDeepSeekStrictToolCall &&
			!isNonThinkingAIExplanationEffort(o.Evaluation.ReasoningEffort) {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation deepseek strict tool protocol requires non-thinking reasoning_effort while named tool_choice is forced"))
		}
		if endpoint := strings.TrimSpace(o.Evaluation.Endpoint); endpoint != "" {
			parsed, err := url.ParseRequestURI(endpoint)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				errs = append(errs, fmt.Errorf("ai_explanation.evaluation.endpoint must be an absolute https URL"))
			}
		}
		if o.Evaluation.Timeout <= 0 {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.timeout must be positive"))
		}
		minimumAttemptLease := o.Timeout + o.Evaluation.Timeout + time.Minute
		if o.Evaluation.AttemptLeaseDuration < minimumAttemptLease {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.attempt_lease_duration must be at least generation timeout plus semantic timeout plus 1m"))
		}
		if o.Evaluation.MaxOutputTokens < 1 {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.max_output_tokens must be positive"))
		}
		if !validAIExplanationReasoningEffort(o.Evaluation.ReasoningEffort) {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.reasoning_effort is invalid"))
		}
		if o.Evaluation.Capacity.MaxActiveRunsPerOrg != 1 {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.capacity.max_active_runs_per_org must equal 1 in v1"))
		}
		if budget := o.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg; budget < 70 {
			errs = append(errs, fmt.Errorf("ai_explanation.evaluation.capacity.daily_provider_invocation_budget_per_org must be at least 70"))
		}
	}
	return errs
}

func validAIExplanationReasoningEffort(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func isNonThinkingAIExplanationEffort(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "none":
		return true
	default:
		return false
	}
}

func validAIExplanationStructuredOutputMode(value string) bool {
	switch strings.TrimSpace(value) {
	case AIExplanationStructuredOutputJSONSchema, AIExplanationStructuredOutputJSONObject:
		return true
	default:
		return false
	}
}

func validAIExplanationProviderProtocol(provider, protocol string) bool {
	switch strings.TrimSpace(provider) {
	case AIExplanationProviderOpenAI:
		return strings.TrimSpace(protocol) == AIExplanationProviderProtocolResponses
	case AIExplanationProviderDeepSeek:
		switch strings.TrimSpace(protocol) {
		case AIExplanationProviderProtocolResponses, AIExplanationProviderProtocolDeepSeekStrictToolCall:
			return true
		}
	}
	return false
}

func (o AIExplanationDataLifecycleOptions) validate() []error {
	var errs []error
	if strings.TrimSpace(o.PolicyVersion) == "" {
		errs = append(errs, fmt.Errorf("ai_explanation.data_lifecycle.policy_version is required when enabled"))
	}
	if o.ParticipantRecordRetention <= 0 {
		errs = append(errs, fmt.Errorf("ai_explanation.data_lifecycle.participant_record_retention must be positive"))
	}
	if o.PromptEvaluationRetention <= 0 {
		errs = append(errs, fmt.Errorf("ai_explanation.data_lifecycle.prompt_evaluation_retention must be positive"))
	}
	if o.CapacityLedgerRetention <= 0 {
		errs = append(errs, fmt.Errorf("ai_explanation.data_lifecycle.capacity_ledger_retention must be positive"))
	}
	return errs
}

func (o *AIExplanationOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	fs.BoolVar(&o.Enabled, "ai_explanation.enabled", o.Enabled, "Enable AI explanation governance and configured evaluation runtime.")
	fs.BoolVar(&o.ParticipantEnabled, "ai_explanation.participant-enabled", o.ParticipantEnabled, "Expose participant AI explanation APIs and worker execution after release acceptance.")
	fs.StringVar(&o.Provider, "ai_explanation.provider", o.Provider, "AI explanation provider implementation.")
	fs.StringVar(&o.ProviderProtocol, "ai_explanation.provider-protocol", o.ProviderProtocol, "Frozen Provider wire protocol: responses or deepseek_strict_tool_call.")
	fs.StringVar(&o.Model, "ai_explanation.model", o.Model, "Exact model or model snapshot used for AI explanations.")
	fs.StringVar(&o.RouteRevision, "ai_explanation.route-revision", o.RouteRevision, "Immutable AI explanation provider route revision.")
	fs.StringVar(&o.StructuredOutputMode, "ai_explanation.structured-output-mode", o.StructuredOutputMode, "Frozen Provider structured output wire mode: json_schema or json_object.")
	fs.StringVar(&o.Endpoint, "ai_explanation.endpoint", o.Endpoint, "Optional Provider protocol endpoint override.")
	fs.DurationVar(&o.Timeout, "ai_explanation.timeout", o.Timeout, "Deadline for the single provider call.")
	fs.DurationVar(&o.RunLeaseDuration, "ai_explanation.run-lease-duration", o.RunLeaseDuration, "Durable ownership lease for one AI explanation run.")
	fs.IntVar(&o.MaxOutputTokens, "ai_explanation.max-output-tokens", o.MaxOutputTokens, "Maximum total output tokens for one provider response.")
	fs.StringVar(&o.ReasoningEffort, "ai_explanation.reasoning-effort", o.ReasoningEffort, "Optional Provider Responses API reasoning effort frozen into the route identity.")
	fs.Int64Var(&o.MaxResponseBytes, "ai_explanation.max-response-bytes", o.MaxResponseBytes, "Maximum Provider response body size.")
	fs.StringVar(&o.DataLifecycle.PolicyVersion, "ai_explanation.data-lifecycle.policy-version", o.DataLifecycle.PolicyVersion, "Approved AI explanation data lifecycle policy version.")
	fs.DurationVar(&o.DataLifecycle.ParticipantRecordRetention, "ai_explanation.data-lifecycle.participant-record-retention", o.DataLifecycle.ParticipantRecordRetention, "Retention after participant AI explanation records reach a terminal state.")
	fs.DurationVar(&o.DataLifecycle.PromptEvaluationRetention, "ai_explanation.data-lifecycle.prompt-evaluation-retention", o.DataLifecycle.PromptEvaluationRetention, "Retention after Prompt evaluation evidence reaches a terminal state.")
	fs.DurationVar(&o.DataLifecycle.CapacityLedgerRetention, "ai_explanation.data-lifecycle.capacity-ledger-retention", o.DataLifecycle.CapacityLedgerRetention, "Retention after a UTC-day capacity ledger closes.")
	fs.IntVar(&o.ParticipantCapacity.DailyProviderInvocationBudgetPerOrg, "ai_explanation.participant-capacity.daily-provider-invocation-budget-per-org", o.ParticipantCapacity.DailyProviderInvocationBudgetPerOrg, "UTC-day participant Provider invocation reservation budget per organization.")
	fs.IntVar(&o.ParticipantCapacity.DailyProviderInvocationBudgetPerUser, "ai_explanation.participant-capacity.daily-provider-invocation-budget-per-user", o.ParticipantCapacity.DailyProviderInvocationBudgetPerUser, "UTC-day participant Provider invocation reservation budget per user within one organization.")
	fs.IntVar(&o.ParticipantCapacity.DailyProviderInvocationBudgetPerAssessment, "ai_explanation.participant-capacity.daily-provider-invocation-budget-per-assessment", o.ParticipantCapacity.DailyProviderInvocationBudgetPerAssessment, "UTC-day participant Provider invocation reservation budget per Assessment.")
	fs.IntVar(&o.ParticipantCapacity.MaxActiveProviderExecutionsPerOrg, "ai_explanation.participant-capacity.max-active-provider-executions-per-org", o.ParticipantCapacity.MaxActiveProviderExecutionsPerOrg, "Maximum active participant Provider executions per organization.")
	fs.IntVar(&o.ParticipantCapacity.MaxActiveProviderExecutionsPerUser, "ai_explanation.participant-capacity.max-active-provider-executions-per-user", o.ParticipantCapacity.MaxActiveProviderExecutionsPerUser, "Maximum active participant Provider executions per user within one organization.")
	fs.IntVar(&o.ParticipantCapacity.MaxActiveProviderExecutionsPerAssessment, "ai_explanation.participant-capacity.max-active-provider-executions-per-assessment", o.ParticipantCapacity.MaxActiveProviderExecutionsPerAssessment, "Maximum active participant Provider executions per Assessment.")
	fs.BoolVar(&o.Evaluation.Enabled, "ai_explanation.evaluation.enabled", o.Evaluation.Enabled, "Enable operator-only synthetic AI explanation release evaluation.")
	fs.StringVar(&o.Evaluation.Model, "ai_explanation.evaluation.model", o.Evaluation.Model, "Exact independent model or model snapshot used to judge synthetic explanations.")
	fs.StringVar(&o.Evaluation.ProviderProtocol, "ai_explanation.evaluation.provider-protocol", o.Evaluation.ProviderProtocol, "Frozen semantic evaluator Provider wire protocol: responses or deepseek_strict_tool_call.")
	fs.StringVar(&o.Evaluation.RouteRevision, "ai_explanation.evaluation.route-revision", o.Evaluation.RouteRevision, "Immutable semantic evaluator route revision.")
	fs.StringVar(&o.Evaluation.StructuredOutputMode, "ai_explanation.evaluation.structured-output-mode", o.Evaluation.StructuredOutputMode, "Frozen semantic evaluator structured output wire mode: json_schema or json_object.")
	fs.StringVar(&o.Evaluation.Endpoint, "ai_explanation.evaluation.endpoint", o.Evaluation.Endpoint, "Optional semantic evaluator Provider protocol endpoint override.")
	fs.DurationVar(&o.Evaluation.Timeout, "ai_explanation.evaluation.timeout", o.Evaluation.Timeout, "Deadline for one semantic evaluator call.")
	fs.DurationVar(&o.Evaluation.AttemptLeaseDuration, "ai_explanation.evaluation.attempt-lease-duration", o.Evaluation.AttemptLeaseDuration, "Durable ownership lease for one generation and semantic evaluation attempt.")
	fs.IntVar(&o.Evaluation.MaxOutputTokens, "ai_explanation.evaluation.max-output-tokens", o.Evaluation.MaxOutputTokens, "Maximum output tokens for one semantic evaluator response.")
	fs.StringVar(&o.Evaluation.ReasoningEffort, "ai_explanation.evaluation.reasoning-effort", o.Evaluation.ReasoningEffort, "Optional semantic evaluator reasoning effort frozen into the route identity.")
	fs.IntVar(&o.Evaluation.Capacity.MaxActiveRunsPerOrg, "ai_explanation.evaluation.capacity.max-active-runs-per-org", o.Evaluation.Capacity.MaxActiveRunsPerOrg, "Maximum collecting Prompt evaluation runs per organization; v1 requires exactly one.")
	fs.IntVar(&o.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg, "ai_explanation.evaluation.capacity.daily-provider-invocation-budget-per-org", o.Evaluation.Capacity.DailyProviderInvocationBudgetPerOrg, "UTC-day Provider invocation reservation budget per organization.")
}
