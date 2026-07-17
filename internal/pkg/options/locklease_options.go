package options

// LockLeaseOptions controls shared lease lifecycle behavior. The zero value is
// intentionally renewal-disabled for compatibility with older configuration.
type LockLeaseOptions struct {
	RenewalEnabled bool `json:"renewal_enabled" mapstructure:"renewal_enabled"`
}

func NewLockLeaseOptions() *LockLeaseOptions {
	return &LockLeaseOptions{}
}
