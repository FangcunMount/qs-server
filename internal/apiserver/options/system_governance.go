package options

import "time"

// SystemGovernanceOptions configures the unified governance facade.
type SystemGovernanceOptions struct {
	Prometheus *SystemGovernancePrometheusOptions     `json:"prometheus" mapstructure:"prometheus"`
	Components map[string]*GovernanceComponentOptions `json:"components" mapstructure:"components"`
	Resilience *ResilienceGovernanceOptions           `json:"resilience" mapstructure:"resilience"`
	Retry      *RetryGovernanceOptions                `json:"retry" mapstructure:"retry"`
}

type RetryGovernanceOptions struct {
	ManualActionsEnabled  bool                `json:"manual_actions_enabled" mapstructure:"manual_actions_enabled"`
	LeaseReconcileEnabled bool                `json:"lease_reconcile_enabled" mapstructure:"lease_reconcile_enabled"`
	Business              *RetryPolicyOptions `json:"business" mapstructure:"business"`
	Outbox                *RetryPolicyOptions `json:"outbox" mapstructure:"outbox"`
}

type RetryPolicyOptions struct {
	MaxAutomaticAttempts int           `json:"max_automatic_attempts" mapstructure:"max_automatic_attempts"`
	BaseDelay            time.Duration `json:"base_delay" mapstructure:"base_delay"`
	MaxDelay             time.Duration `json:"max_delay" mapstructure:"max_delay"`
	JitterFraction       float64       `json:"jitter_fraction" mapstructure:"jitter_fraction"`
}

type ResilienceGovernanceOptions struct {
	TuneRateLimit bool `json:"tune_rate_limit" mapstructure:"tune_rate_limit"`
	DrainQueue    bool `json:"drain_queue" mapstructure:"drain_queue"`
	ResumeQueue   bool `json:"resume_queue" mapstructure:"resume_queue"`
	ReleaseLock   bool `json:"release_lock" mapstructure:"release_lock"`
}

// SystemGovernancePrometheusOptions configures Prometheus query access.
type SystemGovernancePrometheusOptions struct {
	Enabled bool          `json:"enabled" mapstructure:"enabled"`
	BaseURL string        `json:"base_url" mapstructure:"base_url"`
	Timeout time.Duration `json:"timeout" mapstructure:"timeout"`
}

// GovernanceComponentOptions configures remote component governance endpoints.
type GovernanceComponentOptions struct {
	ResilienceURL string        `json:"resilience_url" mapstructure:"resilience_url"`
	CacheURL      string        `json:"cache_url" mapstructure:"cache_url"`
	Timeout       time.Duration `json:"timeout" mapstructure:"timeout"`
}

// NewSystemGovernanceOptions returns defaults for governance aggregation.
func NewSystemGovernanceOptions() *SystemGovernanceOptions {
	return &SystemGovernanceOptions{
		Prometheus: &SystemGovernancePrometheusOptions{
			Enabled: false,
			BaseURL: "http://127.0.0.1:9090",
			Timeout: 3 * time.Second,
		},
		Components: map[string]*GovernanceComponentOptions{},
		Resilience: &ResilienceGovernanceOptions{},
		Retry: &RetryGovernanceOptions{
			ManualActionsEnabled:  true,
			LeaseReconcileEnabled: true,
			Business: &RetryPolicyOptions{
				MaxAutomaticAttempts: 3,
				BaseDelay:            30 * time.Second,
				MaxDelay:             5 * time.Minute,
			},
			Outbox: &RetryPolicyOptions{
				MaxAutomaticAttempts: 30,
				BaseDelay:            10 * time.Second,
				MaxDelay:             time.Hour,
				JitterFraction:       0.20,
			},
		},
	}
}
