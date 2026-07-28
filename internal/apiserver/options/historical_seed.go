package options

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"github.com/spf13/pflag"
)

const DefaultHistoricalSeedSecretEnv = "QS_HISTORICAL_CONTEXT_SECRET"

// HistoricalSeedOptions controls the protected, one-off business-time channel.
// The HMAC key is deliberately referenced by environment variable name and is
// never a config-file field.
type HistoricalSeedOptions struct {
	Enabled            bool          `json:"enabled" mapstructure:"enabled"`
	PausePlanScheduler bool          `json:"pause_plan_scheduler" mapstructure:"pause_plan_scheduler"`
	AllowedOrgIDs      []int64       `json:"allowed_org_ids" mapstructure:"allowed_org_ids"`
	EarliestDate       string        `json:"earliest_date" mapstructure:"earliest_date"`
	LatestDate         string        `json:"latest_date" mapstructure:"latest_date"`
	Timezone           string        `json:"timezone" mapstructure:"timezone"`
	Freshness          time.Duration `json:"freshness" mapstructure:"freshness"`
	SecretEnv          string        `json:"secret_env" mapstructure:"secret_env"`
}

func NewHistoricalSeedOptions() *HistoricalSeedOptions {
	return &HistoricalSeedOptions{
		Enabled:      false,
		EarliestDate: "2025-01-01",
		LatestDate:   "2026-07-27",
		Timezone:     "Asia/Shanghai",
		Freshness:    5 * time.Minute,
		SecretEnv:    DefaultHistoricalSeedSecretEnv,
	}
}

func (o *HistoricalSeedOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	fs.BoolVar(&o.Enabled, "historical_seed.enabled", o.Enabled, "Enable verified historical business-time requests.")
	fs.BoolVar(&o.PausePlanScheduler, "historical_seed.pause-plan-scheduler", o.PausePlanScheduler, "Do not start the built-in Plan scheduler while historical backfill is enabled.")
	fs.Int64SliceVar(&o.AllowedOrgIDs, "historical_seed.allowed-org-ids", o.AllowedOrgIDs, "Organization IDs allowed to use historical business time.")
	fs.StringVar(&o.EarliestDate, "historical_seed.earliest-date", o.EarliestDate, "Earliest allowed historical business date (YYYY-MM-DD).")
	fs.StringVar(&o.LatestDate, "historical_seed.latest-date", o.LatestDate, "Latest allowed historical business date (YYYY-MM-DD).")
	fs.StringVar(&o.Timezone, "historical_seed.timezone", o.Timezone, "Timezone used to validate historical natural-day boundaries.")
	fs.DurationVar(&o.Freshness, "historical_seed.freshness", o.Freshness, "Maximum skew for signed historical requests.")
	fs.StringVar(&o.SecretEnv, "historical_seed.secret-env", o.SecretEnv, "Environment variable containing the historical HMAC key.")
}

func (o *HistoricalSeedOptions) Verifier() (*historicalseed.Verifier, error) {
	if o == nil {
		o = NewHistoricalSeedOptions()
	}
	location, err := time.LoadLocation(strings.TrimSpace(o.Timezone))
	if err != nil {
		return nil, fmt.Errorf("historical_seed.timezone: %w", err)
	}
	earliest, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(o.EarliestDate), location)
	if err != nil {
		return nil, fmt.Errorf("historical_seed.earliest_date: %w", err)
	}
	latest, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(o.LatestDate), location)
	if err != nil {
		return nil, fmt.Errorf("historical_seed.latest_date: %w", err)
	}
	if latest.Before(earliest) {
		return nil, fmt.Errorf("historical_seed.latest_date must not be before earliest_date")
	}
	if o.Freshness <= 0 {
		return nil, fmt.Errorf("historical_seed.freshness must be positive")
	}
	allowed, err := historicalseed.ParseOrgIDs(o.AllowedOrgIDs)
	if err != nil {
		return nil, err
	}
	secretEnv := strings.TrimSpace(o.SecretEnv)
	if secretEnv == "" {
		return nil, fmt.Errorf("historical_seed.secret_env is required")
	}
	secret := strings.TrimSpace(os.Getenv(secretEnv))
	if o.Enabled {
		if len(allowed) == 0 {
			return nil, fmt.Errorf("historical_seed.allowed_org_ids is required when enabled")
		}
		if secret == "" {
			return nil, fmt.Errorf("%s is required when historical_seed.enabled is true", secretEnv)
		}
	}
	return &historicalseed.Verifier{
		Enabled: o.Enabled, Secret: []byte(secret), AllowedOrgIDs: allowed,
		Earliest: earliest, Latest: latest, Location: location, MaxSkew: o.Freshness,
	}, nil
}

func (o *HistoricalSeedOptions) Validate() []error {
	if o != nil && o.PausePlanScheduler && !o.Enabled {
		return []error{fmt.Errorf("historical_seed.pause_plan_scheduler requires historical_seed.enabled")}
	}
	_, err := o.Verifier()
	if err == nil {
		return nil
	}
	return []error{err}
}
