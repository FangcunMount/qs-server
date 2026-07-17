package options

import "time"

// SystemGovernanceOptions configures the unified governance facade.
type SystemGovernanceOptions struct {
	Prometheus *SystemGovernancePrometheusOptions     `json:"prometheus" mapstructure:"prometheus"`
	Components map[string]*GovernanceComponentOptions `json:"components" mapstructure:"components"`
	Resilience *ResilienceGovernanceOptions           `json:"resilience" mapstructure:"resilience"`
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
	}
}
