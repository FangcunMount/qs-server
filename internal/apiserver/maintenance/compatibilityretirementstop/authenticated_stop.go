package compatibilityretirementstop

import (
	"context"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"time"
)

// StopAndDrainAuthenticated is the fixed production service-stop caller after
// an actual AuthorizeProbe invocation. Root approval authorizes only the exact
// observed service inventory; the probe supplies actual signed source/run origin.
// Challenge consumption occurs here, in the same process, never from imported
// JSON. This deliberately does not promote the probe into DROP/global-fence
// authority. All old write credential routes still require separate installation
// and observed denial before any database mutation caller may use a service lease.
// policy.SourceSHA binds the executing B tool; descriptor.SourceSHA remains the
// original A manifest binding. They must not be rewritten to each other's SHA.
func consumeAuthenticatedStop(ctx context.Context, a *Approval, policy fence.Policy, permit *fence.Permit, challengeDir string, window *fence.MaintenanceWindow) error {
	if checkWindow(ctx, a, window) != nil {
		return ErrBinding
	}
	return consumeAuthenticatedStopOrigin(ctx, a, policy, permit, challengeDir)
}

func consumeAuthenticatedStopOrigin(ctx context.Context, a *Approval, policy fence.Policy, permit *fence.Permit, challengeDir string) error {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil || policy.Validate(time.Now()) != nil || policy.SourceSHA != a.descriptor.ToolSourceSHA || policy.OperationID != a.descriptor.OperationID || policy.RequestSHA256 != a.rawHash || permit == nil {
		return ErrBinding
	}
	// Complete policy, expiry, run binding and single-use persistence are checked
	// by the existing root API in this process; no imported receipt is accepted.
	_, e := fence.ConsumeProbeChallenge(ctx, challengeDir, policy, permit)
	return e
}
func StopAndDrainAuthenticated(ctx context.Context, a *Approval, policy fence.Policy, permit *fence.Permit, challengeDir, journalDir string, window *fence.MaintenanceWindow) (*Lease, error) {
	if e := consumeAuthenticatedStop(ctx, a, policy, permit, challengeDir, window); e != nil {
		return nil, e
	}
	return StopAndDrain(ctx, a, journalDir, window)
}
