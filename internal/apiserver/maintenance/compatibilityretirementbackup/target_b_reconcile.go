package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"reflect"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	buildversion "github.com/FangcunMount/qs-server/pkg/version"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// This is expected material approved by the existing fixed host, not an
// execution capability. Recovery retains the original request/window. The
// digest inputs must come from an independently registered physical journal,
// never be filled from a newly observed report and immediately auto-approved.
type TargetBRecoveryResumeRequest struct {
	Recovery              TargetRecoveryResumeRequest
	ApprovedBSourceSHA    string
	MigrationIntentSHA256 string
	MigrationResultSHA256 string
}

// A new process observes a new native proof. It does not deserialize the old
// Pair proof, infer a successful response from a clean head, or retry Up.
// Its original files and actual borrowed handles are private native identities.
type targetBReconciledMigrationProof struct {
	self                     *targetBReconciledMigrationProof
	archive                  *Archive
	borrowed                 TargetRecoveryBorrowed
	request                  TargetBRecoveryResumeRequest
	directory                string
	window                   *fence.MaintenanceWindow
	stamp                    targetJournalDirectoryBinding
	entries                  []targetJournalEntryBinding
	sqlBinding, mongoBinding Binding
	stableMongoSchema        string
	mongoBeforeUUID          string
	sqlConnection            string
	journalKind              targetBJournalKind
}

func (*targetBReconciledMigrationProof) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }

func targetBReconcileInput(r TargetBRecoveryResumeRequest, s *targetJournalSnapshot) (*targetBMigrationIntent, *targetBMigrationResult, error) {
	return targetBReconcileInputKind(r, s, targetBJournalComplete)
}
func targetBReconcileInputKind(r TargetBRecoveryResumeRequest, s *targetJournalSnapshot, kind targetBJournalKind) (*targetBMigrationIntent, *targetBMigrationResult, error) {
	if !sourcePattern.MatchString(r.ApprovedBSourceSHA) || !hashPattern.MatchString(r.MigrationIntentSHA256) || !hashPattern.MatchString(r.MigrationResultSHA256) || !runPattern.MatchString(r.Recovery.CurrentRunID) || !hashPattern.MatchString(r.Recovery.JournalSHA256) || !hashPattern.MatchString(r.Recovery.WindowStartSHA256) || s == nil || s.request != r.Recovery.Original || s.windowStart != r.Recovery.WindowStartSHA256 || s.hash != r.Recovery.JournalSHA256 {
		return nil, nil, ErrRecoveryBinding
	}
	if !targetBMigrationJournalPresent(s) {
		return nil, nil, ErrRecoveryHead
	}
	if e := targetValidateBMigrationJournalKind(s, kind); e != nil {
		return nil, nil, e
	}
	raw := s.files["target-recovery-b-migration-intent.json"]
	resultRaw := s.files["target-recovery-b-migration-result.json"]
	if sha(raw) != r.MigrationIntentSHA256 || sha(resultRaw) != r.MigrationResultSHA256 {
		return nil, nil, ErrRecoveryJournal
	}
	var intent targetBMigrationIntent
	var result targetBMigrationResult
	if exactJSON(raw, &intent) != nil || exactJSON(resultRaw, &result) != nil || intent.Binding.ApprovedSourceSHA != r.ApprovedBSourceSHA {
		return nil, nil, ErrRecoveryBinding
	}
	return &intent, &result, nil
}

// Full current non-target metadata is compared to the approved original hash
// after rewinding only schema_migrations UUID to the recorded original value.
// This is schema comparison, not migration rollback: nothing is written.
// The old UUID is additionally bound to the approved original generation and
// identity. Collection options, indexes and all kept UUIDs remain exact.
func targetBOriginalMongoSchema(defs map[string]any, oldUUID string) (string, error) {
	if !targetBMigrationUUID(oldUUID) {
		return "", ErrStructure
	}
	if _, e := targetBStableMongoNonTarget(defs); e != nil {
		return "", e
	}
	raw, e := json.Marshal(defs)
	if e != nil || len(raw) > metadataBudget {
		return "", ErrStructure
	}
	var copied map[string]any
	if json.Unmarshal(raw, &copied) != nil {
		return "", ErrStructure
	}
	col, ok := copied["collection:schema_migrations"].(map[string]any)
	if !ok {
		return "", ErrStructure
	}
	info, ok := col["info"].(map[string]any)
	if !ok {
		return "", ErrStructure
	}
	uuid, e := hex.DecodeString(oldUUID)
	if e != nil || len(uuid) != 16 {
		return "", ErrStructure
	}
	info["uuid"] = map[string]any{"$binary": map[string]any{"base64": base64.StdEncoding.EncodeToString(uuid), "subType": "04"}}
	return targetMongoNonTarget(copied), nil
}

func targetBOriginalMongoIdentity(ctx context.Context, db *mongo.Database, oldUUID string) (string, error) {
	if db == nil || !targetBMigrationUUID(oldUUID) {
		return "", ErrIdentity
	}
	var hello bson.Raw
	if db.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		return "", ErrIdentity
	}
	stable := bson.D{}
	for _, name := range []string{"setName", "hosts", "me"} {
		v := hello.Lookup(name)
		if v.Type != 0 {
			var value any
			if v.Unmarshal(&value) != nil {
				return "", ErrIdentity
			}
			stable = append(stable, bson.E{Key: name, Value: value})
		}
	}
	raw, e := json.Marshal(stable)
	if e != nil {
		return "", ErrIdentity
	}
	return parts("mongodb_database_identity_v1", string(raw), db.Name(), oldUUID), nil
}

func targetBOriginalEntriesEqual(original []targetJournalEntryBinding, current *targetJournalSnapshot, stamp targetJournalDirectoryBinding) bool {
	if current == nil || len(original) == 0 || current.directory != stamp {
		return false
	}
	byName := make(map[string]targetJournalEntryBinding, len(current.entries))
	for _, entry := range current.entries {
		if _, found := byName[entry.Name]; found {
			return false
		}
		byName[entry.Name] = entry
	}
	for _, entry := range original {
		if actual, found := byName[entry.Name]; !found || actual != entry {
			return false
		}
	}
	return true
}

// VerifyAfter repeats real reads using this producer's exact native handles.
// Original journal files cannot change, but canonical restore records may be
// appended by the existing recovery kernel. The original Window remains the
// only budget source; this proof cannot restart or extend it.
func (p *targetBReconciledMigrationProof) VerifyAfter(ctx context.Context, conn *sql.Conn, db *mongo.Database) error {
	if ctx == nil || ctx.Err() != nil || p == nil || p.self != p || p.archive == nil || p.window == nil || conn == nil || conn != p.borrowed.SQL || db == nil || p.borrowed.Mongo == nil || db.Client() != p.borrowed.Mongo.Client() || db.Name() != p.borrowed.Mongo.Name() || mongo.SessionFromContext(ctx) != nil {
		return ErrRecoveryBinding
	}
	r := p.request
	if r.ApprovedBSourceSHA != buildversion.Get().GitCommit {
		return ErrRecoveryBinding
	}
	if _, e := targetWindowMatches(ctx, p.window, r.Recovery.Original, r.Recovery.WindowStartSHA256); e != nil {
		return e
	}
	s, e := inspectTargetJournalKind(ctx, p.directory, r.Recovery.Original, r.Recovery.WindowStartSHA256, p.journalKind)
	if e != nil || !targetBOriginalEntriesEqual(p.entries, s, p.stamp) {
		return ErrRecoveryJournal
	}
	// The whole-journal hash may change only after the approved initial proof,
	// when the existing kernel appends exact restore records. Original migration
	// entries remain immutable and are validated with their original digest.
	currentRequest := r
	currentRequest.Recovery.JournalSHA256 = s.hash
	if _, _, e = targetBReconcileInputKind(currentRequest, s, p.journalKind); e != nil {
		return e
	}
	if _, e = sqlState(ctx, conn, p.sqlBinding); e != nil {
		return ErrRecoveryHead
	}
	settings, e := readSQL(ctx, conn, "SELECT @@session.foreign_key_checks,@@session.autocommit,CONNECTION_ID(),DATABASE()")
	if e != nil || len(settings) != 1 || len(settings[0]) != 4 || cell(settings[0], 0) != "1" || cell(settings[0], 1) != "1" || cell(settings[0], 2) != p.sqlConnection || cell(settings[0], 3) == "" {
		return ErrRecoveryState
	}
	temporary := &TargetRecoveryPlan{borrowed: p.borrowed}
	if e = temporary.checkSQLNoTransaction(ctx); e != nil {
		return e
	}
	defs, e := readSQLCatalog(ctx, conn)
	if e != nil {
		return e
	}
	non, e := targetSQLNonTarget(defs, cell(settings[0], 3))
	if e != nil || non != r.Recovery.Original.SQLNonTargetSHA256 {
		return ErrStructure
	}
	cols, mdefs, e := mongoCatalog(ctx, db)
	if e != nil {
		return e
	}
	if _, e = mongoState(ctx, db, p.mongoBinding, cols); e != nil {
		return ErrRecoveryHead
	}
	stable, e := targetBStableMongoNonTarget(mdefs)
	if e != nil || stable != p.stableMongoSchema || targetMongoNonTarget(mdefs) != p.mongoBinding.NonTargetHash {
		return ErrStructure
	}
	original, e := targetBOriginalMongoSchema(mdefs, p.mongoBeforeUUID)
	if e != nil || original != r.Recovery.Original.MongoNonTargetSHA256 {
		return ErrStructure
	}
	oldIdentity, e := targetBOriginalMongoIdentity(ctx, db, p.mongoBeforeUUID)
	if e != nil || oldIdentity != p.archive.data.Inventory.Bindings["mongodb"].IdentityHash {
		return ErrIdentity
	}
	if _, e = targetWindowMatches(ctx, p.window, r.Recovery.Original, r.Recovery.WindowStartSHA256); e != nil {
		return e
	}
	return nil
}

// This field is produced only by Capture from the approved actual source
// catalog, and is sealed by the independently bound archive digest. Inventory
// retains target-owned foreign keys, so its NonTargetHash has another meaning.
// Missing older archive fields cannot be supplied by a recovery caller.
func targetBArchiveSQLRecoveryBaseline(a *Archive, r TargetRecoveryRequest) error {
	if a == nil || !hashPattern.MatchString(a.data.SQLRecoveryNonTargetHash) || r.SQLNonTargetSHA256 != a.data.SQLRecoveryNonTargetHash {
		return ErrRecoveryBinding
	}
	return nil
}

func prepareTargetBRecoveryTransitionKind(ctx context.Context, a *Archive, b TargetRecoveryBorrowed, r TargetBRecoveryResumeRequest, dir string, w *fence.MaintenanceWindow, s *targetJournalSnapshot, kind targetBJournalKind) (*targetBMigrationTransition, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || b.SQL == nil || b.Mongo == nil || w == nil || mongo.SessionFromContext(ctx) != nil || r.ApprovedBSourceSHA != buildversion.Get().GitCommit {
		return nil, ErrRecoveryBinding
	}
	intent, result, e := targetBReconcileInputKind(r, s, kind)
	if e != nil {
		return nil, e
	}
	original := r.Recovery.Original
	sb, sok := a.data.Inventory.Bindings["mysql"]
	mb, mok := a.data.Inventory.Bindings["mongodb"]
	if !sok || !mok || original.SQLHead != 99 || original.MongoHead != 38 || sb.Version != 99 || mb.Version != 38 || targetBArchiveSQLRecoveryBaseline(a, original) != nil || original.MongoNonTargetSHA256 != mb.NonTargetHash || original.SourceSHA != a.data.Approval.SourceSHA || original.OperationID != a.data.Approval.OperationID || original.OriginalRunID != a.data.Approval.RunID || original.ArchiveSHA256 != a.digest || intent.SQLIdentitySHA256 != sb.IdentityHash || intent.MongoIdentitySHA256 != mb.IdentityHash || intent.MongoGenerationSHA256 != mb.GenerationHash {
		return nil, ErrRecoveryBinding
	}
	if a.verifyAssets(ctx) != nil {
		return nil, ErrSource
	}
	if _, e = targetWindowMatches(ctx, w, original, r.Recovery.WindowStartSHA256); e != nil {
		return nil, e
	}
	// The fixed B runner was allowed to start only after all four actual DROP
	// responses. A clean new head cannot compensate for a missing or unknown
	// original response. Completed original restores are also allowed here.
	for i := 0; i < 4; i++ {
		state, e := classifyTargetJournal(s, a, i)
		if e != nil {
			return nil, e
		}
		if state.state != "logged_drop_success" && state.state != "logged_restore_success" {
			return nil, ErrRecoveryUnknown
		}
	}
	settings, e := readSQL(ctx, b.SQL, "SELECT @@session.foreign_key_checks,@@session.autocommit,CONNECTION_ID(),DATABASE()")
	if e != nil || len(settings) != 1 || len(settings[0]) != 4 || cell(settings[0], 0) != "1" || cell(settings[0], 1) != "1" || !targetOriginalConnectionID.MatchString(cell(settings[0], 2)) || cell(settings[0], 3) == "" {
		return nil, ErrRecoveryState
	}
	defs, e := readSQLCatalog(ctx, b.SQL)
	if e != nil {
		return nil, e
	}
	non, e := targetSQLNonTarget(defs, cell(settings[0], 3))
	if e != nil || non != original.SQLNonTargetSHA256 {
		return nil, ErrStructure
	}
	cols, mdefs, e := mongoCatalog(ctx, b.Mongo)
	if e != nil {
		return nil, e
	}
	stable, e := targetBStableMongoNonTarget(mdefs)
	if e != nil || stable != intent.MongoStableSchemaSHA256 {
		return nil, ErrStructure
	}
	beforeUUID := ""
	if kind == targetBJournalSQLOnly {
		beforeUUID, e = targetActualMigrationUUID(cols)
		if e != nil || beforeUUID != intent.MongoMigrationUUID {
			return nil, ErrIdentity
		}
		// The only accepted partial path has no Mongo migration attempt at all.
		// Its original migration UUID/head/identity must remain exact.
		mb.Version = 38
	} else {
		beforeUUID = result.Observation.MongoBeforeMigrationUUID
		mb.Version = 39
		mb.IdentityHash = result.Observation.MongoAfterIdentitySHA256
		mb.GenerationHash = result.Observation.MongoAfterGenerationSHA256
	}
	oldSchema, e := targetBOriginalMongoSchema(mdefs, beforeUUID)
	if e != nil || oldSchema != original.MongoNonTargetSHA256 {
		return nil, ErrStructure
	}
	sb.Version = 100
	sb.CatalogHash = jsonSHA(defs)
	sb.NonTargetHash = non
	mb.CatalogHash = jsonSHA(mdefs)
	mb.NonTargetHash = targetMongoNonTarget(mdefs)
	mb.NamespaceAnchor = mb.NamespaceAnchor.Clone()
	proof := &targetBReconciledMigrationProof{archive: a, borrowed: b, request: r, directory: dir, window: w, stamp: s.directory, entries: append([]targetJournalEntryBinding(nil), s.entries...), sqlBinding: sb, mongoBinding: mb, stableMongoSchema: stable, mongoBeforeUUID: beforeUUID, sqlConnection: cell(settings[0], 2), journalKind: kind}
	proof.self = proof
	if e = proof.VerifyAfter(ctx, b.SQL, b.Mongo); e != nil {
		return nil, e
	}
	return &targetBMigrationTransition{proof: proof, sqlBinding: sb, mongoBinding: mb, stableMongoSchema: stable, afterMongoSchema: mb.NonTargetHash, intentSHA: r.MigrationIntentSHA256, resultSHA: r.MigrationResultSHA256, journalKind: kind}, nil
}

// ReconcileTargetBRecovery is the real cross-process producer. It consumes the
// actual original private journal and backup, an independently approved digest
// binding, a reopened original Window and new host-owned native handles. It
// never accepts a imported Pair proof/report as authority, migrates a head,
// retries an unknown result, or supplies a writer-fence/production permit.
func ReconcileTargetBRecovery(ctx context.Context, a *Archive, b TargetRecoveryBorrowed, r TargetBRecoveryResumeRequest, dir string, w *fence.MaintenanceWindow) (*TargetRecoveryReconciliation, error) {
	return reconcileTargetBRecoveryKind(ctx, a, b, r, dir, w, targetBJournalComplete)
}
func reconcileTargetBRecoveryKind(ctx context.Context, a *Archive, b TargetRecoveryBorrowed, r TargetBRecoveryResumeRequest, dir string, w *fence.MaintenanceWindow, kind targetBJournalKind) (*TargetRecoveryReconciliation, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || b.SQL == nil || b.Mongo == nil || w == nil || mongo.SessionFromContext(ctx) != nil {
		return nil, ErrRecoveryBinding
	}
	q, c, e := w.RecoveryContext(ctx)
	if e != nil || !targetBoundContextValid(q, c) {
		if c != nil {
			c()
		}
		return nil, ErrBudget
	}
	defer c()
	s, e := inspectTargetJournalKind(q, dir, r.Recovery.Original, r.Recovery.WindowStartSHA256, kind)
	if e != nil {
		return nil, e
	}
	transition, e := prepareTargetBRecoveryTransitionKind(q, a, b, r, dir, w, s, kind)
	if e != nil {
		return nil, e
	}
	out, e := reconcileTargetRecovery(q, a, b, r.Recovery, dir, w, transition)
	if out != nil {
		preserved := r
		out.bResume = &preserved
	}
	return out, e
}

// Ordinary reconciliation stays legacy-only. Resuming this actual B capability
// goes through the complete native factory again on the supplied handles.
// Comparing its diagnostic summary cannot mint or refresh a proof.
func (v *TargetRecoveryReconciliation) reconcileActual(ctx context.Context, b TargetRecoveryBorrowed, w *fence.MaintenanceWindow) (*TargetRecoveryReconciliation, error) {
	if v == nil || v.self != v {
		return nil, ErrRecoveryBinding
	}
	if v.bResume == nil {
		if v.bMigration != nil {
			return nil, ErrRecoveryBinding
		}
		return ReconcileTargetRecovery(ctx, v.archive, b, v.request, v.journalDir, w)
	}
	if v.bMigration == nil || !reflect.DeepEqual(v.bResume.Recovery, v.request) {
		return nil, ErrRecoveryBinding
	}
	return reconcileTargetBRecoveryKind(ctx, v.archive, b, *v.bResume, v.journalDir, w, v.bMigration.journalKind)
}
