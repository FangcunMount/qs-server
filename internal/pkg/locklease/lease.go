package locklease

import (
	"context"
	"errors"
	"time"

	base "github.com/FangcunMount/component-base/pkg/locklease"
)

type Spec = base.Spec
type Identity = base.Identity
type Lease = base.Lease
type Manager = base.Manager
type Renewer = base.Renewer
type RenewableManager = base.RenewableManager

var (
	ErrLeaseAcquireFailed = errors.New("lock lease acquire failed")
	ErrLeaseRenewFailed   = errors.New("lock lease renew failed")
	ErrLeaseLost          = errors.New("lock lease ownership lost")
)

// RunResult describes an executed lease-protected workload. ReleaseErr is
// informational: token-safe release remains best-effort and never replaces the
// body or renewal result.
type RunResult struct {
	Acquired   bool
	ReleaseErr error
}

// Runner executes a workload while owning and, when enabled, renewing its lease.
type Runner interface {
	Run(
		ctx context.Context,
		workload WorkloadID,
		key string,
		ttl time.Duration,
		body func(context.Context) error,
	) (RunResult, error)
}
