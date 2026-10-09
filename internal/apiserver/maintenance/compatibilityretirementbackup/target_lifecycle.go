package compatibilityretirementbackup

import (
	"context"
	"encoding/hex"
	"reflect"
	"strconv"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TargetLifecycleResult is actual readback, not an execution permit. The fixed
// host must establish approval and actual writer quiescence before calling the
// effectful kernel below. Current Action mutation capabilities remain disabled.
// A budget-only Window or this result cannot satisfy that host responsibility.
type TargetLifecycleResult struct {
	self    *TargetLifecycleResult
	summary TargetLifecycleSummary
}

type TargetLifecycleSummary struct {
	Targets             TargetRecoverySummary        `json:"targets"`
	Journal             TargetRecoveryJournalSummary `json:"journal"`
	ActualRunID         string                       `json:"actual_run_id"`
	WriterFenceRequired bool                         `json:"writer_fence_required"`
}

func (*TargetLifecycleResult) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (*TargetLifecycleResult) String() string {
	return "opaque actual four-target lifecycle readback; host writer fence required"
}
func (v *TargetLifecycleResult) Summary() TargetLifecycleSummary {
	if v == nil || v.self != v {
		return TargetLifecycleSummary{}
	}
	s := v.summary
	s.Targets.Targets = append([]TargetRecoveryObservation(nil), s.Targets.Targets...)
	return s
}

func targetWindowMatches(ctx context.Context, w *fence.MaintenanceWindow, r TargetRecoveryRequest, start string) (string, error) {
	if ctx == nil || ctx.Err() != nil || w == nil {
		return "", ErrBudget
	}
	d, e := w.Diagnostic(ctx)
	want := fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: r.SourceSHA, OperationID: r.OperationID, ManifestSHA256: r.ManifestSHA256, OriginalRunID: r.OriginalRunID}
	if e != nil || d.Binding != want || !d.BudgetOnly || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 || !hashPattern.MatchString(d.StartSHA256) || (start != "" && d.StartSHA256 != start) {
		return "", ErrBudget
	}
	return d.StartSHA256, nil
}

func targetBoundContextValid(q context.Context, c context.CancelFunc) bool {
	if q == nil || c == nil || q.Err() != nil {
		return false
	}
	_, ok := q.Deadline()
	return ok
}

// ApplyTargets is the fixed four-target native DDL kernel. It accepts only the
// actual prepared opaque plan, never a table list, serialized proof, boolean
// fence assertion, or reconstructed native authority. Its exported availability
// does not integrate production authorization: the approved fixed host caller
// must already hold the shared deployment lock and have stopped actual writers.
// All four original targets are checked before the first DROP. Partial success
// is journalled and remains available to RecoverTargets under the original
// recovery Window; an unknown statement/result is never adopted or re-DROPped.
func ApplyTargets(ctx context.Context, p *TargetRecoveryPlan) (*TargetLifecycleResult, error) {
	if p == nil || p.self != p || ctx == nil || ctx.Err() != nil {
		return nil, ErrRecoveryBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.archive == nil || p.journal == nil || p.window == nil || p.blocked {
		return nil, ErrRecoveryBinding
	}
	start, e := targetWindowMatches(ctx, p.window, p.request, "")
	if e != nil {
		return nil, e
	}
	q, c, e := p.window.ForwardContext(ctx)
	if e != nil || !targetBoundContextValid(q, c) {
		if c != nil {
			c()
		}
		return nil, ErrBudget
	}
	defer c()
	if p.archive.verifyAssets(q) != nil {
		return nil, ErrSource
	}
	if e = p.checkBases(q, true); e != nil {
		return nil, e
	}
	for i := 0; i < 4; i++ {
		if p.drop[i] != nil || p.observations[i].State != "existing_exact" {
			return nil, ErrRecoveryState
		}
		present, e := p.checkTarget(q, i)
		if e != nil || !present {
			return nil, ErrRecoveryState
		}
	}
	for i := 0; i < 4; i++ {
		if _, e = targetWindowMatches(q, p.window, p.request, start); e != nil {
			return nil, e
		}
		if e = executeTargetDropLocked(q, p, i); e != nil {
			return nil, e
		}
	}
	return verifyDroppedTargetsLocked(q, p, start)
}

// VerifyDroppedTargets requires the process-bound native producer for every
// target as well as a new actual namespace/identity/head/non-target readback.
// Four absent namespaces or a caller JSON report are never sufficient.
func VerifyDroppedTargets(ctx context.Context, p *TargetRecoveryPlan) (*TargetLifecycleResult, error) {
	if p == nil || p.self != p || ctx == nil || ctx.Err() != nil {
		return nil, ErrRecoveryBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.archive == nil || p.journal == nil || p.window == nil || p.blocked {
		return nil, ErrRecoveryBinding
	}
	start, e := targetWindowMatches(ctx, p.window, p.request, "")
	if e != nil {
		return nil, e
	}
	q, c, e := p.window.ForwardContext(ctx)
	if e != nil || !targetBoundContextValid(q, c) {
		if c != nil {
			c()
		}
		return nil, ErrBudget
	}
	defer c()
	return verifyDroppedTargetsLocked(q, p, start)
}

func verifyDroppedTargetsLocked(ctx context.Context, p *TargetRecoveryPlan, start string) (*TargetLifecycleResult, error) {
	if p == nil || p.self != p || p.blocked || p.archive == nil || p.journal == nil {
		return nil, ErrRecoveryBinding
	}
	if e := p.checkBases(ctx, true); e != nil {
		return nil, e
	}
	if p.archive.verifyAssets(ctx) != nil {
		return nil, ErrSource
	}
	for i := 0; i < 4; i++ {
		if !p.validDrop(i) || p.observations[i].State != "dropped_native" {
			return nil, ErrRecoveryAuthority
		}
		present, e := p.checkTarget(ctx, i)
		if e != nil || present {
			return nil, ErrRecoveryState
		}
	}
	if e := p.checkBases(ctx, true); e != nil {
		return nil, e
	}
	return targetLifecycleReadback(ctx, p, p.request, start)
}

func targetLifecycleReadback(ctx context.Context, p *TargetRecoveryPlan, original TargetRecoveryRequest, start string) (*TargetLifecycleResult, error) {
	if _, e := targetWindowMatches(ctx, p.window, original, start); e != nil {
		return nil, e
	}
	if p.journal == nil || p.journal.validate() != nil {
		return nil, ErrRecoveryJournal
	}
	s, e := inspectTargetJournal(ctx, p.journal.dir, original, start)
	if e != nil {
		return nil, e
	}
	for i := 0; i < 4; i++ {
		state, e := classifyTargetJournal(s, p.archive, i)
		if e != nil {
			return nil, e
		}
		if state.state != "logged_drop_success" && state.state != "logged_restore_success" && state.state != "no_drop_record" {
			return nil, ErrRecoveryUnknown
		}
	}
	if _, e = targetWindowMatches(ctx, p.window, original, start); e != nil {
		return nil, e
	}
	v := &TargetLifecycleResult{summary: TargetLifecycleSummary{Targets: p.summaryLocked(), Journal: TargetRecoveryJournalSummary{JournalSHA256: s.hash, RequestSHA256: jsonSHA(original), WindowStartSHA256: start, Files: len(s.entries)}, ActualRunID: p.request.ActualRunID, WriterFenceRequired: true}}
	v.self = v
	return v, nil
}

// ResumeTargetRecovery consumes only an actual physical-journal + four-namespace
// reconciliation. It never constructs targetDropProof. A missing result, unknown
// DROP, incomplete restore, target reappearance or an unbound migration
// generation remains blocked even if a target currently happens to be absent.
// It restores only logged successful DROPs that have a newly confirmed absence.
// Host-owned handles are neither opened, committed, rolled back nor closed here.
func ResumeTargetRecovery(ctx context.Context, v *TargetRecoveryReconciliation, b TargetRecoveryBorrowed, w *fence.MaintenanceWindow) (*TargetLifecycleResult, error) {
	if v == nil || v.self != v || ctx == nil || ctx.Err() != nil || b.SQL == nil || b.Mongo == nil || w == nil || mongo.SessionFromContext(ctx) != nil {
		return nil, ErrRecoveryBinding
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.summary.Unresolved != 0 || len(v.summary.Targets) != 4 {
		return nil, ErrRecoveryUnknown
	}
	fresh, e := ReconcileTargetRecovery(ctx, v.archive, b, v.request, v.journalDir, w)
	if e != nil {
		return nil, e
	}
	if fresh == nil || !reflect.DeepEqual(fresh.summary, v.summary) {
		return nil, ErrRecoveryState
	}
	start, e := targetWindowMatches(ctx, w, v.request.Original, v.request.WindowStartSHA256)
	if e != nil {
		return nil, e
	}
	q, c, e := w.RecoveryContext(ctx)
	if e != nil || !targetBoundContextValid(q, c) {
		if c != nil {
			c()
		}
		return nil, ErrBudget
	}
	defer c()
	s, e := inspectTargetJournal(q, v.journalDir, v.request.Original, start)
	if e != nil || s.hash != v.request.JournalSHA256 {
		return nil, ErrRecoveryJournal
	}
	j, e := reopenTargetRecoveryJournal(q, v.journalDir, s)
	if e != nil {
		return nil, e
	}
	r := v.request.Original
	r.ActualRunID = v.request.CurrentRunID
	p := &TargetRecoveryPlan{archive: v.archive, borrowed: b, request: r, journal: j, window: w, budget: w.RecoveryContext}
	p.self = p
	// A resumed plan has no native process-bound DROP capabilities, including
	// after a successful restore. Logged DROP readback is a distinct evidence tier.
	if e = p.checkBases(q, false); e != nil {
		return nil, e
	}
	for i, target := range fresh.summary.Targets {
		if target.Database != targetDatabase(i) || target.Name != targetNames[i] || target.SourceRecords != p.archive.data.Inventory.Targets[i].Records || target.BlockingReason != "" {
			return nil, ErrRecoveryBinding
		}
		p.observations[i] = TargetRecoveryObservation{Database: target.Database, Name: target.Name, Records: target.SourceRecords}
		switch target.State {
		case "original_exact_no_drop":
			p.observations[i].State = "existing_exact"
		case "logged_restore_reconciled_exact":
			p.observations[i].State = "restored_exact"
			if i == 3 {
				if e = targetRestoredMongoIdentity(q, p); e != nil {
					return nil, e
				}
			}
		case "logged_drop_reconciled_absent":
			p.observations[i].State = "resume_absent"
		default:
			return nil, ErrRecoveryUnknown
		}
	}
	// Verify every target before the first restore so another unknown/partial
	// target cannot be hidden by a successful first CREATE.
	for i := 0; i < 4; i++ {
		present, e := p.checkTarget(q, i)
		if e != nil {
			return nil, e
		}
		if present != (p.observations[i].State != "resume_absent") {
			return nil, ErrRecoveryState
		}
	}
	for i := 0; i < 4; i++ {
		if p.observations[i].State != "resume_absent" {
			continue
		}
		if _, e = targetWindowMatches(q, w, v.request.Original, start); e != nil {
			return nil, e
		}
		if e = p.checkBases(q, true); e != nil {
			return nil, e
		}
		if p.archive.verifyAssets(q) != nil {
			return nil, ErrSource
		}
		present, e := targetActualNamespace(q, p, i)
		if e != nil || present {
			return nil, ErrRecoveryState
		}
		before, e := inspectTargetJournal(q, v.journalDir, v.request.Original, start)
		if e != nil {
			return nil, e
		}
		state, e := classifyTargetJournal(before, p.archive, i)
		if e != nil || state.state != "logged_drop_success" {
			return nil, ErrRecoveryUnknown
		}
		binding := targetResumeBindingRecord{OriginalRequestSHA256: jsonSHA(v.request.Original), WindowStartSHA256: start, Request: r, SQLConnectionID: p.sqlConnection}
		if _, e = p.journal.write(strconv.Itoa(i)+"-resume-binding", binding); e != nil {
			return nil, e
		}
		// Reuse exactly the same CREATE/load/index/full-readback kernels used
		// by RecoverTargets. Their borrowed connection lifecycle is unchanged.
		if i < 3 {
			e = p.recoverSQL(q, i)
		} else {
			e = p.recoverMongo(q)
		}
		if e != nil {
			p.blocked = true
			return nil, e
		}
	}
	if e = p.checkBases(q, true); e != nil {
		return nil, e
	}
	for i := 0; i < 4; i++ {
		present, e := p.checkTarget(q, i)
		if e != nil || !present {
			return nil, ErrContent
		}
	}
	if p.archive.verifyAssets(q) != nil {
		return nil, ErrSource
	}
	return targetLifecycleReadback(q, p, v.request.Original, start)
}

func targetDatabase(i int) string {
	if i == 3 {
		return "mongodb"
	}
	return "mysql"
}

func targetRestoredMongoIdentity(ctx context.Context, p *TargetRecoveryPlan) error {
	ordered, e := ReadOrderedMongoSchema(ctx, p.borrowed.Mongo)
	if e != nil {
		return e
	}
	kind, id, ok := bson.Raw(ordered.data.Collection).Lookup("info", "uuid").BinaryOK()
	oldKind, oldID, oldOK := bson.Raw(p.archive.data.Mongo.Collection).Lookup("info", "uuid").BinaryOK()
	if !ok || !oldOK || kind != 4 || oldKind != 4 || len(id) != 16 || len(oldID) != 16 || hex.EncodeToString(id) == hex.EncodeToString(oldID) {
		return ErrIdentity
	}
	p.mongoTargetUUID = hex.EncodeToString(id)
	return nil
}
