package options

// LockLeaseOptions controls shared lease lifecycle behavior. Runtime defaults
// to automatic renewal; an explicit false remains available as an operational
// rollback switch.
type LockLeaseOptions struct {
	RenewalEnabled bool `json:"renewal_enabled" mapstructure:"renewal_enabled"`
}

func NewLockLeaseOptions() *LockLeaseOptions {
	return &LockLeaseOptions{RenewalEnabled: true}
}
