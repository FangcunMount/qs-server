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
	ManualActionsEnabled  bool                                  `json:"manual_actions_enabled" mapstructure:"manual_actions_enabled"`
	LeaseReconcileEnabled bool                                  `json:"lease_reconcile_enabled" mapstructure:"lease_reconcile_enabled"`
	Lease                 *InterpretationLeaseGovernanceOptions `json:"lease" mapstructure:"lease"`
	Business              *RetryPolicyOptions                   `json:"business" mapstructure:"business"`
	Outbox                *RetryPolicyOptions                   `json:"outbox" mapstructure:"outbox"`
}

// InterpretationLeaseGovernanceOptions keeps Interpretation Run lease duration and
// the HA lease-recovery scan cadence in one governance block (IR-R011).
//
// Worst-case recovery after a worker crash (immediate post-claim crash):
//
//	run_duration + reconcile_interval + reconcile_interval*reconcile_jitter_fraction
//
// run_duration bounds how long a dead worker keeps the attempt; after expiry the
// evaluation_consistency_reconcile scheduler may wait up to one scan period
// (plus jitter) before reclaiming the same attempt number.
type InterpretationLeaseGovernanceOptions struct {
	RunDuration             time.Duration `json:"run_duration" mapstructure:"run_duration"`
	ReconcileInterval       time.Duration `json:"reconcile_interval" mapstructure:"reconcile_interval"`
	ReconcileJitterFraction float64       `json:"reconcile_jitter_fraction" mapstructure:"reconcile_jitter_fraction"`
}

func NewInterpretationLeaseGovernanceOptions() *InterpretationLeaseGovernanceOptions {
	return &InterpretationLeaseGovernanceOptions{
		RunDuration:             5 * time.Minute,
		ReconcileInterval:       10 * time.Second,
		ReconcileJitterFraction: 0,
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
	if normalized.ReconcileInterval <= 0 {
		normalized.ReconcileInterval = defaults.ReconcileInterval
	}
	if normalized.ReconcileJitterFraction < 0 {
		normalized.ReconcileJitterFraction = defaults.ReconcileJitterFraction
	}
	return &normalized
}

// RunLeaseDuration returns the configured Interpretation Run lease, defaulting to 5m.
func (o *InterpretationLeaseGovernanceOptions) RunLeaseDuration() time.Duration {
	return o.normalized().RunDuration
}

// WorstCaseRecoveryWindowAfterExpiry upper-bounds the delay from lease expiry to
// the next scheduled lease-recovery reclaim.
func (o *InterpretationLeaseGovernanceOptions) WorstCaseRecoveryWindowAfterExpiry() time.Duration {
	normalized := o.normalized()
	jitter := time.Duration(float64(normalized.ReconcileInterval) * normalized.ReconcileJitterFraction)
	return normalized.ReconcileInterval + jitter
}

// WorstCaseRecoveryWindowAfterCrash upper-bounds crash-to-reclaim when the worker
// dies immediately after claiming the attempt.
func (o *InterpretationLeaseGovernanceOptions) WorstCaseRecoveryWindowAfterCrash() time.Duration {
	normalized := o.normalized()
	return normalized.RunDuration + normalized.WorstCaseRecoveryWindowAfterExpiry()
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
	Discovery        string        `json:"discovery" mapstructure:"discovery"`
	MinimumInstances int           `json:"minimum_instances" mapstructure:"minimum_instances"`
	ResilienceURL    string        `json:"resilience_url" mapstructure:"resilience_url"`
	CacheURL         string        `json:"cache_url" mapstructure:"cache_url"`
	Timeout          time.Duration `json:"timeout" mapstructure:"timeout"`
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
			ManualActionsEnabled:  true,
			LeaseReconcileEnabled: true,
			Lease:                 NewInterpretationLeaseGovernanceOptions(),
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
