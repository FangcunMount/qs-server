package main

import (
	"context"
	"database/sql"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"go.mongodb.org/mongo-driver/mongo"
)

// This is the completed comparison's same-process fact, not an acceptance,
// full-writer fence or purge capability. Only compareCompleteDataBeforeB can
// produce it, after all stored non-target rows reached EOF and the original
// host ended both new RO scopes. No observation/JSON/callback can supply one.
type lifecyclePreBDataComparison struct {
	self     *lifecyclePreBDataComparison
	host     *lifecycleFixedHost
	owner    *lifecyclePreparationOwner
	services *lifecycleServiceController
	api      *lifecycleAPITransition
	window   *fence.MaintenanceWindow
	baseline *backup.NonTargetDataBaseline
	plan     *backup.TargetRecoveryPlan
	pair     *migration.CompatibilityPairMigrationProof
	conn     *sql.Conn
	client   *mongo.Client
	db       *mongo.Database
	binding  lifecyclePreBComparisonBinding
	seal     string
}

type lifecyclePreBComparisonBinding struct {
	Window          fence.WindowBinding
	WindowStart     string
	RequestSHA256   string
	RequestBody     string
	PrepareRoot     string
	PairObservation string
}

func (*lifecyclePreBDataComparison) MarshalJSON() ([]byte, error) {
	return nil, lifecycleError("lifecycle_private_comparison_serialization_forbidden")
}
func (*lifecyclePreBDataComparison) UnmarshalJSON([]byte) error {
	return lifecycleError("lifecycle_private_comparison_serialization_forbidden")
}
func (*lifecyclePreBDataComparison) String() string {
	return "opaque completed pre-B non-target comparison; no acceptance or mutation authority"
}

func lifecyclePreBComparisonExpected(r lifecycleRequest, start string, p *migration.CompatibilityPairMigrationProof) lifecyclePreBComparisonBinding {
	return lifecyclePreBComparisonBinding{Window: lifecycleWindowBinding(r), WindowStart: start, RequestSHA256: r.requestSHA256,
		RequestBody: digest(r), PrepareRoot: r.prepareRoot, PairObservation: digest(p.Observation())}
}

// A failed/unknown comparison is not replayed in the same host. The original
// plan/Window recovery remains responsible; an API failure retains this fact
// and cannot use it to start a second API or to mint acceptance.
func (h *lifecycleFixedHost) compareCompleteDataBeforeB(ctx context.Context, r lifecycleRequest, p *migration.CompatibilityPairMigrationProof, w *fence.MaintenanceWindow) (*lifecyclePreBDataComparison, error) {
	if h == nil || ctx == nil || ctx.Err() != nil || h.owner == nil || h.owner.originalConn == nil || h.owner.originalMongo == nil || h.owner.originalDB == nil ||
		h.services == nil || !h.services.managementReady || h.services.window != w || !h.services.identity.matches(r) || w == nil ||
		h.api == nil || h.api.self != h.api || h.api.unknown || h.api.bCID != "" || h.api.rollbackCID != "" || h.api.removedOriginal ||
		h.dataBaseline == nil || h.acceptancePlan == nil || p == nil || h.comparisonAttempted || h.preBComparison != nil || h.acceptancePair != nil || !hashRE.MatchString(r.requestSHA256) {
		return nil, lifecycleError("lifecycle_actual_pre_b_data_comparison_missing")
	}
	if e := h.services.Check(ctx); e != nil {
		return nil, e // Full original catalog: no post-deployment API exemption.
	}
	if e := h.CheckWholeWriterFence(ctx, r); e != nil {
		return nil, e
	}
	if _, e := backup.VerifyDroppedTargets(ctx, h.acceptancePlan); e != nil {
		return nil, e
	}
	if e := p.VerifyAfter(ctx, h.owner.originalConn, h.owner.originalDB); e != nil || !lifecycleInlineMigrationBindingMatches(r, p.Observation(), w, ctx) {
		return nil, lifecycleError("lifecycle_actual_pair_acceptance_missing")
	}
	d, e := w.Diagnostic(ctx)
	if e != nil || !hashRE.MatchString(d.StartSHA256) || d.RecoverySHA256 != "" {
		return nil, lifecycleError("lifecycle_actual_pre_b_data_comparison_missing")
	}
	h.comparisonAttempted = true
	// This call returns only after Mongo Abort/EndSession and SQL Rollback.
	// No fact is assigned by a defer or while an RO metadata lock remains live.
	if e = h.verifyCompleteDataBeforeInternalResume(ctx, p); e != nil {
		return nil, e
	}
	// VerifyAfter also observes the dedicated SQL connection back in actual
	// autocommit/inactive-transaction state. DTO cleanup flags are insufficient.
	if e = p.VerifyAfter(ctx, h.owner.originalConn, h.owner.originalDB); e != nil {
		return nil, e
	}
	if _, e = backup.VerifyDroppedTargets(ctx, h.acceptancePlan); e != nil {
		return nil, e
	}
	if e = h.services.Check(ctx); e != nil {
		return nil, e
	}
	if e = h.CheckWholeWriterFence(ctx, r); e != nil {
		return nil, e
	}
	after, e := w.Diagnostic(ctx)
	if e != nil || after.StartSHA256 != d.StartSHA256 || after.Binding != d.Binding || !after.DirectoryLeaseHeld || after.RemainingMilliseconds <= 0 || after.RecoverySHA256 != "" || ctx.Err() != nil {
		return nil, lifecycleError("lifecycle_actual_pre_b_data_comparison_missing")
	}
	f := &lifecyclePreBDataComparison{host: h, owner: h.owner, services: h.services, api: h.api, window: w, baseline: h.dataBaseline,
		plan: h.acceptancePlan, pair: p, conn: h.owner.originalConn, client: h.owner.originalMongo, db: h.owner.originalDB,
		binding: lifecyclePreBComparisonExpected(r, d.StartSHA256, p)}
	f.self, f.seal = f, digest(f.binding)
	return f, nil
}

// The comparison covers the frozen stopped interval. After B starts its normal
// A-equivalent background logic, acceptance continues native schema/absence,
// runtime/audit/MQ and external-fence checks instead of rescanning live rows as
// though every current internal writer were still stopped.
func (h *lifecycleFixedHost) verifyPreBDataComparison(ctx context.Context, r lifecycleRequest) error {
	if ctx == nil || ctx.Err() != nil || h == nil || h.services == nil || h.services.window == nil || !h.services.identity.matches(r) || h.api == nil || h.api.self != h.api || h.api.unknown || !hashRE.MatchString(h.api.bCID) {
		return lifecycleError("lifecycle_actual_pre_b_data_comparison_missing")
	}
	d, e := h.services.window.Diagnostic(ctx)
	if e != nil || d.Binding != lifecycleWindowBinding(r) || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 || d.RecoverySHA256 != "" || !h.preBComparison.matches(h, r, d.StartSHA256) {
		return lifecycleError("lifecycle_actual_pre_b_data_comparison_missing")
	}
	return ctx.Err()
}

func (f *lifecyclePreBDataComparison) matches(h *lifecycleFixedHost, r lifecycleRequest, start string) bool {
	return f != nil && f.self == f && h != nil && h.comparisonAttempted && f.host == h && f.owner != nil && f.owner == h.owner &&
		f.services != nil && f.services == h.services && f.window != nil && f.window == h.services.window && f.api != nil && f.api == h.api &&
		f.baseline != nil && f.baseline == h.dataBaseline && f.plan != nil && f.plan == h.acceptancePlan && f.pair != nil && f.pair == h.acceptancePair &&
		f.conn != nil && f.conn == h.owner.originalConn && f.client != nil && f.client == h.owner.originalMongo && f.db != nil && f.db == h.owner.originalDB &&
		hashRE.MatchString(r.requestSHA256) && hashRE.MatchString(start) && f.binding == lifecyclePreBComparisonExpected(r, start, h.acceptancePair) && f.seal == digest(f.binding)
}
