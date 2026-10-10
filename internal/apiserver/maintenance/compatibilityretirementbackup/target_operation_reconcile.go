package compatibilityretirementbackup

import (
	"context"
	"encoding/hex"
	"reflect"
	"regexp"
	"strconv"
	"sync"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Actual inspection is reusable by the fixed host. It is deliberately not a
// second execution-permission system and cannot be consumed as DROP authority.
type TargetRecoveryReconciliation struct {
	self       *TargetRecoveryReconciliation
	mu         sync.Mutex
	archive    *Archive
	borrowed   TargetRecoveryBorrowed
	request    TargetRecoveryResumeRequest
	journalDir string
	window     *fence.MaintenanceWindow
	summary    TargetRecoveryReconciliationSummary
	bMigration *targetBMigrationTransition
	bResume    *TargetBRecoveryResumeRequest
}

type TargetRecoveryReconciliationSummary struct {
	SourceSHA                     string
	OperationID                   string
	OriginalRunID                 string
	CurrentRunID                  string
	ManifestSHA256                string
	ArchiveSHA256                 string
	JournalSHA256                 string
	WindowStartSHA256             string
	Targets                       []TargetRecoveryReconciledTarget
	Unresolved                    int
	WriterFenceRequired           bool
	ProductionAuthorityIntegrated bool
	MutationAllowed               bool
	DropReady                     bool
}

type TargetRecoveryReconciledTarget struct {
	Database       string
	Name           string
	State          string
	SourceRecords  uint64
	BlockingReason string
}

func (*TargetRecoveryReconciliation) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (*TargetRecoveryReconciliation) String() string {
	return "opaque four-target journal reconciliation; writer fence required"
}
func (v *TargetRecoveryReconciliation) Summary() TargetRecoveryReconciliationSummary {
	if v == nil || v.self != v {
		return TargetRecoveryReconciliationSummary{}
	}
	s := v.summary
	s.Targets = append([]TargetRecoveryReconciledTarget(nil), s.Targets...)
	return s
}

var targetOriginalConnectionID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

type targetJournalTargetState struct {
	state  string
	reason string
}

func targetExpectedStatement(a *Archive, i int, phase string) (string, error) {
	if a == nil || i < 0 || i >= 4 {
		return "", ErrRecoveryBinding
	}
	switch phase {
	case "drop":
		if i == 3 {
			return "drop collection domain_event_outbox", nil
		}
		return "DROP TABLE " + quote(targetNames[i]), nil
	case "create":
		if i < 3 {
			return a.data.SQL[i].DDL, nil
		}
		opts, ok := bson.Raw(a.data.Mongo.Collection).Lookup("options").DocumentOK()
		if !ok {
			return "", ErrStructure
		}
		fields, e := opts.Elements()
		if e != nil {
			return "", ErrStructure
		}
		cmd := bson.D{{Key: "create", Value: targetNames[3]}}
		for _, field := range fields {
			switch field.Key() {
			case "uuid", "viewOn", "pipeline", "create", "$db":
				return "", ErrStructure
			}
			cmd = append(cmd, bson.E{Key: field.Key(), Value: field.Value()})
		}
		raw, e := bson.Marshal(cmd)
		if e != nil {
			return "", ErrStructure
		}
		return string(raw), nil
	case "load":
		if i == 3 {
			return "ordered original BSON InsertMany", nil
		}
		return "bounded original source INSERT", nil
	case "index":
		if i != 3 {
			return "", ErrRecoveryJournal
		}
		indexes := bson.A{}
		for _, raw := range a.data.Mongo.Indexes {
			index := bson.Raw(raw)
			name, ok := index.Lookup("name").StringValueOK()
			if !ok {
				return "", ErrStructure
			}
			if name != "_id_" {
				indexes = append(indexes, index)
			}
		}
		if len(indexes) == 0 {
			return "", ErrRecoveryJournal
		}
		raw, e := bson.Marshal(bson.D{{Key: "createIndexes", Value: targetNames[3]}, {Key: "indexes", Value: indexes}})
		if e != nil {
			return "", ErrStructure
		}
		return string(raw), nil
	case "verify":
		if i == 3 {
			return "full original ordered schema and BSON readback", nil
		}
		return "full original structure and content readback", nil
	}
	return "", ErrRecoveryJournal
}

func targetRequiresIndex(a *Archive, i int) (bool, error) {
	if i != 3 {
		return false, nil
	}
	for _, raw := range a.data.Mongo.Indexes {
		name, ok := bson.Raw(raw).Lookup("name").StringValueOK()
		if !ok {
			return false, ErrStructure
		}
		if name != "_id_" {
			return true, nil
		}
	}
	return false, nil
}

func targetValidateRecord(r *targetStatementRecord, request TargetRecoveryRequest, a *Archive, i int, phase string, result string, records uint64) error {
	if r == nil {
		return ErrRecoveryJournal
	}
	statement, e := targetExpectedStatement(a, i, phase)
	database := "mysql"
	if i == 3 {
		database = "mongodb"
	}
	if e != nil || r.RequestSHA256 != jsonSHA(request) || r.Index != i || r.Database != database || r.Name != targetNames[i] || r.Phase != phase || r.StatementSHA256 != sha([]byte(statement)) || r.Result != result || r.Records != records || !targetOriginalConnectionID.MatchString(r.SQLConnectionID) {
		return ErrRecoveryJournal
	}
	return nil
}

func classifyTargetJournal(s *targetJournalSnapshot, a *Archive, i int) (targetJournalTargetState, error) {
	state := targetJournalTargetState{state: "no_drop_record"}
	if s == nil || a == nil || i < 0 || i >= 4 || len(a.data.Inventory.Targets) != 4 {
		return state, ErrRecoveryBinding
	}
	prefix := "target-recovery-" + strconv.Itoa(i) + "-"
	get := func(name string) (*targetStatementRecord, error) { return targetJournalRecord(s, prefix+name+".json") }
	dropIntent, e := get("drop-intent")
	if e != nil {
		return state, e
	}
	dropResult, e := get("drop-result")
	if e != nil {
		return state, e
	}
	restoreRequest, restoreConnection, resumed, e := targetJournalRestoreBinding(s, i)
	if e != nil {
		return state, e
	}
	var dropConnection string
	for _, phase := range []string{"drop", "create", "load", "index", "verify"} {
		for _, suffix := range []string{"intent", "result"} {
			if phase == "verify" && suffix == "intent" {
				continue
			}
			r, e := get(phase + "-" + suffix)
			if e != nil {
				return state, e
			}
			if r == nil {
				continue
			}
			if phase == "index" && i != 3 {
				return state, ErrRecoveryJournal
			}
			if phase == "drop" {
				if dropConnection == "" {
					dropConnection = r.SQLConnectionID
				}
				if dropConnection != r.SQLConnectionID {
					return state, ErrRecoveryJournal
				}
			} else {
				if !resumed {
					restoreConnection = dropConnection
				}
				if restoreConnection != r.SQLConnectionID {
					return state, ErrRecoveryJournal
				}
			}
		}
	}
	if dropIntent == nil {
		if dropResult != nil || resumed {
			return state, ErrRecoveryJournal
		}
		for _, phase := range []string{"create-intent", "create-result", "load-intent", "load-result", "index-intent", "index-result", "verify-result"} {
			r, e := get(phase)
			if e != nil {
				return state, e
			}
			if r != nil {
				return state, ErrRecoveryJournal
			}
		}
		return state, nil
	}
	if targetValidateRecord(dropIntent, s.request, a, i, "drop", "intent", 0) != nil {
		return state, ErrRecoveryJournal
	}
	if dropResult == nil {
		return targetJournalTargetState{"unresolved_drop_intent", "drop_result_missing"}, nil
	}
	if dropResult.Result == "unknown" {
		if targetValidateRecord(dropResult, s.request, a, i, "drop", "unknown", 0) != nil {
			return state, ErrRecoveryJournal
		}
		return targetJournalTargetState{"unresolved_drop_result", "drop_result_unknown"}, nil
	}
	if targetValidateRecord(dropResult, s.request, a, i, "drop", "native_success_and_absent", 0) != nil {
		return state, ErrRecoveryJournal
	}
	state.state = "logged_drop_success"
	createIntent, e := get("create-intent")
	if e != nil {
		return state, e
	}
	createResult, e := get("create-result")
	if e != nil {
		return state, e
	}
	loadIntent, e := get("load-intent")
	if e != nil {
		return state, e
	}
	loadResult, e := get("load-result")
	if e != nil {
		return state, e
	}
	indexIntent, e := get("index-intent")
	if e != nil {
		return state, e
	}
	indexResult, e := get("index-result")
	if e != nil {
		return state, e
	}
	verified, e := get("verify-result")
	if e != nil {
		return state, e
	}
	if createIntent == nil {
		if createResult != nil || loadIntent != nil || loadResult != nil || indexIntent != nil || indexResult != nil || verified != nil {
			return state, ErrRecoveryJournal
		}
		if resumed {
			return targetJournalTargetState{"unresolved_resume_intent", "resume_create_intent_missing"}, nil
		}
		return state, nil
	}
	if targetValidateRecord(createIntent, restoreRequest, a, i, "create", "intent", 0) != nil {
		return state, ErrRecoveryJournal
	}
	if createResult == nil {
		return targetJournalTargetState{"unresolved_restore_intent", "create_result_missing"}, nil
	}
	if createResult.Result == "unknown" {
		if targetValidateRecord(createResult, restoreRequest, a, i, "create", "unknown", 0) != nil {
			return state, ErrRecoveryJournal
		}
		return targetJournalTargetState{"unresolved_restore_result", "create_result_unknown"}, nil
	}
	if targetValidateRecord(createResult, restoreRequest, a, i, "create", "native_success", 0) != nil {
		return state, ErrRecoveryJournal
	}
	if loadIntent == nil {
		return targetJournalTargetState{"unresolved_restore_sequence", "load_intent_missing"}, nil
	}
	if targetValidateRecord(loadIntent, restoreRequest, a, i, "load", "intent", 0) != nil {
		return state, ErrRecoveryJournal
	}
	if loadResult == nil {
		return targetJournalTargetState{"unresolved_restore_intent", "load_result_missing"}, nil
	}
	if loadResult.Result == "unknown" {
		if loadResult.Records > a.data.Inventory.Targets[i].Records || targetValidateRecord(loadResult, restoreRequest, a, i, "load", "unknown", loadResult.Records) != nil {
			return state, ErrRecoveryJournal
		}
		return targetJournalTargetState{"unresolved_restore_result", "load_result_unknown"}, nil
	}
	n := a.data.Inventory.Targets[i].Records
	if targetValidateRecord(loadResult, restoreRequest, a, i, "load", "native_success", n) != nil {
		return state, ErrRecoveryJournal
	}
	requiresIndex, e := targetRequiresIndex(a, i)
	if e != nil {
		return state, e
	}
	if !requiresIndex && (indexIntent != nil || indexResult != nil) {
		return state, ErrRecoveryJournal
	}
	if requiresIndex {
		if indexIntent == nil {
			return targetJournalTargetState{"unresolved_restore_sequence", "index_intent_missing"}, nil
		}
		if targetValidateRecord(indexIntent, restoreRequest, a, i, "index", "intent", 0) != nil {
			return state, ErrRecoveryJournal
		}
		if indexResult == nil {
			return targetJournalTargetState{"unresolved_restore_intent", "index_result_missing"}, nil
		}
		if indexResult.Result == "unknown" {
			if targetValidateRecord(indexResult, restoreRequest, a, i, "index", "unknown", 0) != nil {
				return state, ErrRecoveryJournal
			}
			return targetJournalTargetState{"unresolved_restore_result", "index_result_unknown"}, nil
		}
		if targetValidateRecord(indexResult, restoreRequest, a, i, "index", "native_success", 0) != nil {
			return state, ErrRecoveryJournal
		}
	}
	if verified == nil {
		return targetJournalTargetState{"unresolved_restore_sequence", "full_verify_result_missing"}, nil
	}
	if targetValidateRecord(verified, restoreRequest, a, i, "verify", "equal", n) != nil {
		return state, ErrRecoveryJournal
	}
	return targetJournalTargetState{state: "logged_restore_success"}, nil
}

func targetActualNamespace(ctx context.Context, p *TargetRecoveryPlan, i int) (bool, error) {
	if i < 3 {
		r, e := readSQL(ctx, p.borrowed.SQL, "SELECT TABLE_TYPE FROM information_schema.tables WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ?", targetNames[i])
		if e != nil {
			return false, ErrRead
		}
		if len(r) == 0 {
			return false, nil
		}
		if len(r) != 1 || cell(r[0], 0) != "BASE TABLE" {
			return false, ErrRecoveryState
		}
		return true, nil
	}
	collections, _, e := mongoCatalog(ctx, p.borrowed.Mongo)
	if e != nil {
		return false, e
	}
	row, ok := collections[targetNames[3]]
	if !ok {
		return false, nil
	}
	typeName, ok := row.Lookup("type").StringValueOK()
	if !ok || typeName != "collection" {
		return false, ErrRecoveryState
	}
	return true, nil
}

// ReconcileTargetRecovery verifies an independently registered journal and the
// actual four namespaces. It never reconstructs targetDropProof, re-drops,
// writes a missing result, takes over a connection, or changes a migration head.
// Current-head migration-generation transitions must be separately approved;
// the existing checkBases deliberately rejects an unbound generation change.
func ReconcileTargetRecovery(ctx context.Context, a *Archive, b TargetRecoveryBorrowed, r TargetRecoveryResumeRequest, dir string, w *fence.MaintenanceWindow) (*TargetRecoveryReconciliation, error) {
	return reconcileTargetRecovery(ctx, a, b, r, dir, w, nil)
}

func reconcileTargetRecovery(ctx context.Context, a *Archive, b TargetRecoveryBorrowed, r TargetRecoveryResumeRequest, dir string, w *fence.MaintenanceWindow, transition *targetBMigrationTransition) (*TargetRecoveryReconciliation, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || b.SQL == nil || b.Mongo == nil || w == nil || mongo.SessionFromContext(ctx) != nil || !runPattern.MatchString(r.CurrentRunID) || !hashPattern.MatchString(r.JournalSHA256) || !hashPattern.MatchString(r.WindowStartSHA256) {
		return nil, ErrRecoveryBinding
	}
	original := r.Original
	if !hashPattern.MatchString(original.ManifestSHA256) || !hashPattern.MatchString(original.SQLNonTargetSHA256) || !hashPattern.MatchString(original.MongoNonTargetSHA256) || !runPattern.MatchString(original.ActualRunID) || original.SourceSHA != a.data.Approval.SourceSHA || original.OperationID != a.data.Approval.OperationID || original.OriginalRunID != a.data.Approval.RunID || original.ArchiveSHA256 != a.digest || !sourcePattern.MatchString(original.SourceSHA) || !runPattern.MatchString(original.OperationID) || !targetSupportedHead(original.SQLHead, a.data.Inventory.Bindings["mysql"].Version) || !targetSupportedHead(original.MongoHead, a.data.Inventory.Bindings["mongodb"].Version) {
		return nil, ErrRecoveryBinding
	}
	if a.verifyAssets(ctx) != nil {
		return nil, ErrSource
	}
	for _, sql := range a.data.SQL {
		if targetRecoveryForeignKeys(sql.DDL) != nil {
			return nil, ErrStructure
		}
	}
	window, e := w.Diagnostic(ctx)
	want := fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: original.SourceSHA, OperationID: original.OperationID, ManifestSHA256: original.ManifestSHA256, OriginalRunID: original.OriginalRunID}
	if e != nil || window.Binding != want || !window.DirectoryLeaseHeld || !window.BudgetOnly || window.StartSHA256 != r.WindowStartSHA256 || window.RemainingMilliseconds <= 0 {
		return nil, ErrRecoveryBinding
	}
	s, e := inspectTargetJournalKind(ctx, dir, original, r.WindowStartSHA256, targetTransitionJournalKind(transition))
	if e != nil {
		return nil, e
	}
	if s.hash != r.JournalSHA256 {
		return nil, ErrRecoveryJournal
	}
	// The ordinary API still refuses a B report. Only the separate actual
	// journal+borrowed-handle producer can supply a private fresh transition.
	if targetBMigrationJournalPresent(s) != (transition != nil) {
		return nil, ErrRecoveryHead
	}
	plan := &TargetRecoveryPlan{archive: a, borrowed: b, request: original, window: w, bMigration: transition}
	if transition != nil {
		plan.journal, e = reopenTargetRecoveryJournalKind(ctx, dir, s, targetTransitionJournalKind(transition))
		if e != nil {
			return nil, e
		}
	}
	plan.self = plan
	if e = plan.checkBases(ctx, false); e != nil {
		return nil, e
	}
	out := &TargetRecoveryReconciliation{archive: a, borrowed: b, request: r, journalDir: dir, window: w, bMigration: transition}
	out.self = out
	out.summary = TargetRecoveryReconciliationSummary{SourceSHA: original.SourceSHA, OperationID: original.OperationID, OriginalRunID: original.OriginalRunID, CurrentRunID: r.CurrentRunID, ManifestSHA256: original.ManifestSHA256, ArchiveSHA256: a.digest, JournalSHA256: s.hash, WindowStartSHA256: r.WindowStartSHA256, WriterFenceRequired: true}
	for i := 0; i < 4; i++ {
		state, e := classifyTargetJournal(s, a, i)
		if e != nil {
			return nil, e
		}
		database := "mysql"
		if i == 3 {
			database = "mongodb"
		}
		target := TargetRecoveryReconciledTarget{Database: database, Name: targetNames[i], SourceRecords: a.data.Inventory.Targets[i].Records, State: state.state, BlockingReason: state.reason}
		plan.observations[i] = TargetRecoveryObservation{Database: database, Name: targetNames[i]}
		present, e := targetActualNamespace(ctx, plan, i)
		if e != nil {
			return nil, e
		}
		switch state.state {
		case "no_drop_record":
			if !present {
				target.State = "missing_without_drop_record"
				target.BlockingReason = "missing_target_is_not_drop_authority"
			} else {
				exact, e := plan.checkTarget(ctx, i)
				if e != nil || !exact {
					target.State = "existing_conflicting_or_partial"
					target.BlockingReason = "actual_target_not_original_exact"
				} else {
					target.State = "original_exact_no_drop"
				}
			}
		case "logged_drop_success":
			if present {
				target.State = "unexpected_target_reappearance"
				target.BlockingReason = "drop_success_target_reappeared"
			} else {
				target.State = "logged_drop_reconciled_absent"
			}
		case "logged_restore_success":
			if !present {
				target.State = "restored_target_missing"
				target.BlockingReason = "verified_restore_target_now_missing"
			} else {
				// An already restored Mongo target must have a new UUID. Complete
				// original ordered schema/content, not UUID equality, is checked.
				if i == 3 {
					ordered, e := ReadOrderedMongoSchema(ctx, b.Mongo)
					if e != nil {
						return nil, e
					}
					kind, id, ok := bson.Raw(ordered.data.Collection).Lookup("info", "uuid").BinaryOK()
					oldKind, oldID, oldOK := bson.Raw(a.data.Mongo.Collection).Lookup("info", "uuid").BinaryOK()
					if !ok || !oldOK || kind != 4 || oldKind != 4 || len(id) != 16 || len(oldID) != 16 || hex.EncodeToString(id) == hex.EncodeToString(oldID) {
						return nil, ErrIdentity
					}
					plan.mongoTargetUUID = hex.EncodeToString(id)
				}
				exact, e := plan.checkTarget(ctx, i)
				if e != nil || !exact {
					target.State = "restored_conflicting_or_partial"
					target.BlockingReason = "actual_restore_not_original_exact"
				} else {
					target.State = "logged_restore_reconciled_exact"
				}
			}
		default:
			// Observation does not decide which client executed the DDL. Keep
			// the original unknown responsibility even when absence is actual.
			if present {
				target.State += "_actual_present"
			} else {
				target.State += "_actual_absent"
			}
		}
		if target.BlockingReason != "" {
			out.summary.Unresolved++
		}
		out.summary.Targets = append(out.summary.Targets, target)
	}
	if e = plan.checkBases(ctx, true); e != nil {
		return nil, e
	}
	final, e := inspectTargetJournalKind(ctx, dir, original, r.WindowStartSHA256, targetTransitionJournalKind(transition))
	if e != nil || final.hash != s.hash {
		return nil, ErrRecoveryJournal
	}
	end, e := w.Diagnostic(ctx)
	if e != nil || end.Binding != want || end.StartSHA256 != r.WindowStartSHA256 || end.RemainingMilliseconds <= 0 || !end.DirectoryLeaseHeld {
		return nil, ErrBudget
	}
	if out.summary.Unresolved > 0 {
		return out, ErrRecoveryUnknown
	}
	return out, nil
}

// Recheck uses actual fresh host-provided handles. Stable diagnostic equality is
// not accepted in place of another complete physical/namespace/content read.
func (v *TargetRecoveryReconciliation) Recheck(ctx context.Context, b TargetRecoveryBorrowed, w *fence.MaintenanceWindow) error {
	if v == nil || v.self != v {
		return ErrRecoveryBinding
	}
	fresh, e := v.reconcileActual(ctx, b, w)
	if e != nil {
		return e
	}
	if fresh == nil || !reflect.DeepEqual(fresh.summary, v.summary) {
		return ErrRecoveryState
	}
	return nil
}
