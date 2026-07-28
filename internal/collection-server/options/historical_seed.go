package options

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"github.com/spf13/pflag"
)

const defaultHistoricalSeedSecretEnv = "QS_HISTORICAL_CONTEXT_SECRET"

// HistoricalSeedOptions mirrors the apiserver verification boundary because
// collection-server is the public AnswerSheet ingress that forwards the
// verified context explicitly over gRPC.
type HistoricalSeedOptions struct {
	Enabled       bool          `json:"enabled" mapstructure:"enabled"`
	AllowedOrgIDs []int64       `json:"allowed_org_ids" mapstructure:"allowed_org_ids"`
	EarliestDate  string        `json:"earliest_date" mapstructure:"earliest_date"`
	LatestDate    string        `json:"latest_date" mapstructure:"latest_date"`
	Timezone      string        `json:"timezone" mapstructure:"timezone"`
	Freshness     time.Duration `json:"freshness" mapstructure:"freshness"`
	SecretEnv     string        `json:"secret_env" mapstructure:"secret_env"`
}

func NewHistoricalSeedOptions() *HistoricalSeedOptions {
	return &HistoricalSeedOptions{Enabled: false, EarliestDate: "2025-01-01", LatestDate: "2026-07-27", Timezone: "Asia/Shanghai", Freshness: 5 * time.Minute, SecretEnv: defaultHistoricalSeedSecretEnv}
}

func (o *HistoricalSeedOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	fs.BoolVar(&o.Enabled, "historical_seed.enabled", o.Enabled, "Enable verified historical business-time requests.")
	fs.Int64SliceVar(&o.AllowedOrgIDs, "historical_seed.allowed-org-ids", o.AllowedOrgIDs, "Organization IDs allowed to use historical business time.")
	fs.StringVar(&o.EarliestDate, "historical_seed.earliest-date", o.EarliestDate, "Earliest allowed historical business date.")
	fs.StringVar(&o.LatestDate, "historical_seed.latest-date", o.LatestDate, "Latest allowed historical business date.")
	fs.StringVar(&o.Timezone, "historical_seed.timezone", o.Timezone, "Timezone used to validate historical natural days.")
	fs.DurationVar(&o.Freshness, "historical_seed.freshness", o.Freshness, "Maximum signed-request skew.")
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
	if latest.Before(earliest) || o.Freshness <= 0 {
		return nil, fmt.Errorf("historical_seed date range and freshness must be valid")
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
	if o.Enabled && (len(allowed) == 0 || secret == "") {
		return nil, fmt.Errorf("historical_seed requires allowed_org_ids and %s when enabled", secretEnv)
	}
	return &historicalseed.Verifier{Enabled: o.Enabled, Secret: []byte(secret), AllowedOrgIDs: allowed, Earliest: earliest, Latest: latest, Location: location, MaxSkew: o.Freshness}, nil
}

func (o *HistoricalSeedOptions) Validate() []error {
	_, err := o.Verifier()
	if err == nil {
		return nil
	}
	return []error{err}
}
