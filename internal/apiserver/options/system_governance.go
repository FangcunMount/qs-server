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
	ManualActionsEnabled bool                                  `json:"manual_actions_enabled" mapstructure:"manual_actions_enabled"`
	Lease                *InterpretationLeaseGovernanceOptions `json:"lease" mapstructure:"lease"`
	Business             *RetryPolicyOptions                   `json:"business" mapstructure:"business"`
	Outbox               *RetryPolicyOptions                   `json:"outbox" mapstructure:"outbox"`
}

// InterpretationLeaseGovernanceOptions owns the Interpretation Run lease
// duration. Recovery cadence belongs to interpretation_lease_recovery.
type InterpretationLeaseGovernanceOptions struct {
	RunDuration time.Duration `json:"run_duration" mapstructure:"run_duration"`
}

func NewInterpretationLeaseGovernanceOptions() *InterpretationLeaseGovernanceOptions {
	return &InterpretationLeaseGovernanceOptions{
		RunDuration: 5 * time.Minute,
	}
}

func (o *InterpretationLeaseGovernanceOptions) normalized() *InterpretationLeaseGovernanceOptions {
	defaults := NewInterpretationLeaseGovernanceOptions()
	if o == nil {
		return defaults
	}
	normalized := *o
	if normalized.RunDuration <= 0 {
		normalized.RunDuration = defaults.RunDuration
	}
	return &normalized
}

// RunLeaseDuration returns the configured Interpretation Run lease, defaulting to 5m.
func (o *InterpretationLeaseGovernanceOptions) RunLeaseDuration() time.Duration {
	return o.normalized().RunDuration
}

type RetryPolicyOptions struct {
	MaxAutomaticAttempts int           `json:"max_automatic_attempts" mapstructure:"max_automatic_attempts"`
	BaseDelay            time.Duration `json:"base_delay" mapstructure:"base_delay"`
	MaxDelay             time.Duration `json:"max_delay" mapstructure:"max_delay"`
	JitterFraction       float64       `json:"jitter_fraction" mapstructure:"jitter_fraction"`
}

type ResilienceGovernanceOptions struct {
	TuneRateLimit bool `json:"tune_rate_limit" mapstructure:"tune_rate_limit"`
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
	Discovery          string        `json:"discovery" mapstructure:"discovery"`
	MinimumInstances   int           `json:"minimum_instances" mapstructure:"minimum_instances"`
	ResilienceURL      string        `json:"resilience_url" mapstructure:"resilience_url"`
	CacheURL           string        `json:"cache_url" mapstructure:"cache_url"`
	CacheGovernanceURL string        `json:"cache_governance_url" mapstructure:"cache_governance_url"`
	Timeout            time.Duration `json:"timeout" mapstructure:"timeout"`
}

func (o *GovernanceComponentOptions) DiscoveryMode() string {
	if o == nil || o.Discovery == "" {
		return "single"
	}
	return o.Discovery
}

func (o *GovernanceComponentOptions) RequiredInstances() int {
	if o == nil || o.MinimumInstances <= 0 {
		return 1
	}
	return o.MinimumInstances
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
			ManualActionsEnabled: true,
			Lease:                NewInterpretationLeaseGovernanceOptions(),
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
