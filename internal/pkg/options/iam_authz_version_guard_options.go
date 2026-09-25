package options

import (
	"fmt"
	"time"
)

// IAMAuthzVersionGuardOptions bounds the age of a committed IAM policy-version
// proof used by QS authorization snapshots.
type IAMAuthzVersionGuardOptions struct {
	Enabled      bool          `json:"enabled" mapstructure:"enabled"`
	MaxAge       time.Duration `json:"max-age" mapstructure:"max-age"`
	PollInterval time.Duration `json:"poll-interval" mapstructure:"poll-interval"`
	ReadTimeout  time.Duration `json:"read-timeout" mapstructure:"read-timeout"`
}

func NewIAMAuthzVersionGuardOptions() *IAMAuthzVersionGuardOptions {
	return &IAMAuthzVersionGuardOptions{
		Enabled:      false,
		MaxAge:       10 * time.Second,
		PollInterval: 5 * time.Second,
		ReadTimeout:  2 * time.Second,
	}
}

func (o *IAMAuthzVersionGuardOptions) Validate() []error {
	if o == nil || !o.Enabled {
		return nil
	}
	var errs []error
	if o.MaxAge <= 0 || o.MaxAge > 10*time.Second {
		errs = append(errs, fmt.Errorf("iam.authz-version-guard.max-age must be positive and at most 10s"))
	}
	if o.PollInterval <= 0 || o.PollInterval >= o.MaxAge {
		errs = append(errs, fmt.Errorf("iam.authz-version-guard.poll-interval must be positive and shorter than max-age"))
	}
	if o.ReadTimeout <= 0 || o.ReadTimeout >= o.MaxAge {
		errs = append(errs, fmt.Errorf("iam.authz-version-guard.read-timeout must be positive and shorter than max-age"))
	}
	return errs
}
