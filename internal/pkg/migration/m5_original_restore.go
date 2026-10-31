package migration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const m5OriginalTargetDDLHash = "1950d488e5e83afdacce15b2bc47fb1126a9d622519c5da3a3b8bbaf672735da"
const m5OriginalLedgerDDLHash = "e663736774931e5f988b7fa72a0849fa725ae3ba97f083df404b676d64b616ee"

func m5LedgerContentHash(version uint, dirty bool) string {
	flag := "30"
	if dirty {
		flag = "31"
	}
	value := strings.ToUpper(hex.EncodeToString([]byte(strconv.FormatUint(uint64(version), 10))))
	return sortedDigestRows([][32]byte{digestSQLRow([]sql.RawBytes{[]byte(value), []byte(flag)})})
}

type M5OriginalLedgerRow struct {
	Version uint `json:"version"`
	Dirty   bool `json:"dirty"`
}
type M5OriginalMaterial struct {
	FormatVersion   int                   `json:"format_version"`
	Kind            string                `json:"kind"`
	OperationID     string                `json:"operation_id"`
	TargetDDL       string                `json:"target_ddl"`
	TargetDDLHash   string                `json:"target_ddl_sha256"`
	OriginalEOFHash string                `json:"original_eof_sha256"`
	LedgerDDL       string                `json:"ledger_ddl"`
	LedgerDDLHash   string                `json:"ledger_ddl_sha256"`
	LedgerRows      []M5OriginalLedgerRow `json:"ledger_rows"`
	Baseline        RetainedBaseline      `json:"baseline"`
}
type M5RestoreConfig struct {
	Pair                                   PairConfig
	OriginalFile, OriginalSHA256           string
	Apply                                  bool
	AuthorizationFile, AuthorizationSHA256 string
}
type M5RestoreApproval struct {
	FormatVersion         int    `json:"format_version"`
	Kind                  string `json:"kind"`
	OperationID           string `json:"operation_id"`
	ApprovedSourceSHA     string `json:"approved_source_sha"`
	OriginalSHA256        string `json:"original_sha256"`
	ObservedStateSHA256   string `json:"observed_state_sha256"`
	FreezeReceiptSHA256   string `json:"freeze_receipt_sha256"`
	BoundaryReceiptSHA256 string `json:"boundary_receipt_sha256"`
	ReviewSHA256          string `json:"review_sha256"`
	ExpiresAt             string `json:"expires_at"`
	RecoveryStartedAt     string `json:"recovery_started_at"`
	DataOwner             string `json:"data_owner"`
	RecoveryOwner         string `json:"recovery_owner"`
	RuntimeFrozen         bool   `json:"runtime_frozen"`
	RuntimeNotStarted     bool   `json:"runtime_not_started"`
}
type M5RestoreState struct {
	SQLVersion    uint                      `json:"mysql_version"`
	SQLDirty      bool                      `json:"mysql_dirty"`
	TargetPresent bool                      `json:"target_present"`
	Objects       map[string]RetainedObject `json:"objects"`
}
type M5RestoreResult struct {
	Stage                 string          `json:"stage"`
	ReadOnly              bool            `json:"read_only"`
	OriginalSHA256        string          `json:"original_sha256"`
	ObservedStateSHA256   string          `json:"observed_state_sha256,omitempty"`
	State                 *M5RestoreState `json:"state,omitempty"`
	AuthorizationConsumed bool            `json:"authorization_consumed"`
	MutationAttempted     bool            `json:"mutation_attempted"`
	LedgerReplayAttempted bool            `json:"ledger_replay_attempted"`
	CommitOutcomeUnknown  bool            `json:"commit_outcome_unknown"`
	AlreadyOriginal       bool            `json:"already_original"`
	Verified              bool            `json:"verified"`
}

// CaptureM5OriginalMaterial exports only the original empty-target DDL, complete
// one-row ledger and full retained hashes. The caller preserves these bytes in
// a private file while the independently evidenced writer exclusion is held.
func CaptureM5OriginalMaterial(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig, operation string) (m M5OriginalMaterial, err error) {
	if !retirementOpRE.MatchString(operation) {
		return m, retirementError("explicit original operation required")
	}
	m.Baseline, err = CaptureRetainedBaseline(ctx, db, client, cfg, 1000000)
	if err != nil {
		return m, err
	}
	m.FormatVersion, m.Kind, m.OperationID = 1, "qs_m5_original_empty_publisher", operation
	var name string
	if err = db.QueryRowContext(ctx, "SHOW CREATE TABLE ai_messaging_outbox").Scan(&name, &m.TargetDDL); err != nil {
		return m, err
	}
	if err = db.QueryRowContext(ctx, "SHOW CREATE TABLE schema_migrations").Scan(&name, &m.LedgerDDL); err != nil {
		return m, err
	}
	m.TargetDDLHash, m.LedgerDDLHash, m.OriginalEOFHash = retirementHash([]byte(m.TargetDDL)), retirementHash([]byte(m.LedgerDDL)), retirementHash(nil)
	rows, err := db.QueryContext(ctx, "SELECT version,dirty FROM schema_migrations")
	if err != nil {
		return m, err
	}
	for rows.Next() {
		var row M5OriginalLedgerRow
		if err = rows.Scan(&row.Version, &row.Dirty); err != nil {
			_ = rows.Close()
			return m, err
		}
		m.LedgerRows = append(m.LedgerRows, row)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return m, err
	}
	r, err := compiledMigrationResources()
	if err != nil {
		return m, err
	}
	return m, validateM5Original(m, cfg, r)
}

func decodeM5Private(path, digest string, limit int64, out any) error {
	raw, err := readPrivateRetirementFile(path, digest, limit)
	if err != nil {
		return err
	}
	if err = rejectRetirementDuplicateJSON(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return retirementError("exact M5 private input schema required")
	}
	return nil
}

func validateM5Original(m M5OriginalMaterial, cfg PairConfig, resources *migrationResources) error {
	s := m.Baseline.Snapshot
	if m.FormatVersion != 1 || m.Kind != "qs_m5_original_empty_publisher" || !retirementOpRE.MatchString(m.OperationID) || len(cfg.ExpectedSourceSHA) != 40 || strings.Trim(cfg.ExpectedSourceSHA, "0123456789abcdef") != "" ||
		!m.Baseline.Complete || m.Baseline.Phase != "complete" || !m.Baseline.ReadOnly || m.Baseline.Mutation || s.SourceSHA != cfg.ExpectedSourceSHA || s.MigrationResourcesSHA256 != resources.sha256 ||
		s.MySQLDatabase != cfg.MySQLDatabase || s.MongoDatabase != cfg.MongoDatabase || s.MySQLVersion != 110 || s.MongoVersion != 44 || s.TargetMySQLVersion != 111 || s.TargetMongoVersion != 44 || resources.sql.head != 111 || resources.mongo.head != 44 ||
		m.TargetDDLHash != m5OriginalTargetDDLHash || retirementHash([]byte(m.TargetDDL)) != m.TargetDDLHash || m.LedgerDDLHash != m5OriginalLedgerDDLHash || retirementHash([]byte(m.LedgerDDL)) != m.LedgerDDLHash || m.OriginalEOFHash != retirementHash(nil) ||
		len(m.LedgerRows) != 1 || m.LedgerRows[0] != (M5OriginalLedgerRow{110, false}) {
		return retirementError("exact original110/44 complete material required")
	}
	for _, spec := range []struct {
		name, ddl string
		rows      uint64
	}{{"ai_messaging_outbox", m.TargetDDL, 0}, {"schema_migrations", m.LedgerDDL, 1}} {
		o, ok := m.Baseline.Objects["qs.mysql."+spec.name]
		expected := sortedDigestRows([][32]byte{digestSQLRow([]sql.RawBytes{[]byte(spec.name), []byte(spec.ddl)})})
		if !ok || o.Kind != "BASE TABLE" || o.Rows == nil || *o.Rows != spec.rows || o.DefinitionSHA256 != expected {
			return retirementError("original object baseline not bound to full material")
		}
	}
	if m.Baseline.Objects["qs.mysql.schema_migrations"].ContentSHA256 != m5LedgerContentHash(110, false) || m.Baseline.Objects["qs.mysql.ai_messaging_outbox"].ContentSHA256 != retirementHash(nil) {
		return retirementError("complete original ledger/empty stream content differs")
	}
	return nil
}

func compareM5Retained(original, current map[string]RetainedObject) error {
	ignored := func(name string) bool {
		return name == "qs.mysql.ai_messaging_outbox" || name == "qs.mysql.schema_migrations"
	}
	for name, before := range original {
		if ignored(name) {
			continue
		}
		after, ok := current[name]
		if !ok || !reflect.DeepEqual(before, after) {
			return retirementError("retained original asset changed")
		}
	}
	for name := range current {
		if ignored(name) {
			continue
		}
		if _, ok := original[name]; !ok {
			return retirementError("unreviewed retained asset appeared")
		}
	}
	return nil
}

func inspectM5RestoreState(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig, m M5OriginalMaterial) (state M5RestoreState, err error) {
	a, b, err := pairIdentity(ctx, db, client, cfg.MySQLDatabase, cfg.MongoDatabase)
	if err != nil || a != m.Baseline.Snapshot.MySQLIdentitySHA256 || b != m.Baseline.Snapshot.MongoClusterSHA256 {
		return state, retirementError("M5 original pair identity changed")
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil || count != 1 {
		return state, retirementError("complete one-row original ledger required")
	}
	if err = db.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&state.SQLVersion, &state.SQLDirty); err != nil || (state.SQLVersion != 111 && (state.SQLVersion != 110 || state.SQLDirty)) {
		return state, retirementError("known interrupted111 or original110 required")
	}
	var mongoHead struct {
		Version uint `bson:"version"`
		Dirty   bool `bson:"dirty"`
	}
	n, err := client.Database(cfg.MongoDatabase).Collection("schema_migrations").CountDocuments(ctx, bson.D{})
	if err != nil || n != 1 || client.Database(cfg.MongoDatabase).Collection("schema_migrations").FindOne(ctx, bson.D{}).Decode(&mongoHead) != nil || mongoHead.Version != 44 || mongoHead.Dirty {
		return state, retirementError("original Mongo44 metadata required")
	}
	state.Objects = map[string]RetainedObject{}
	if err = captureSQLRetained(ctx, db, cfg.MySQLDatabase, 1000000, state.Objects); err != nil {
		return state, err
	}
	if err = captureMongoRetained(ctx, client.Database(cfg.MongoDatabase), 1000000, state.Objects); err != nil {
		return state, err
	}
	if err = compareM5Retained(m.Baseline.Objects, state.Objects); err != nil {
		return state, err
	}
	ledger := state.Objects["qs.mysql.schema_migrations"]
	if ledger.DefinitionSHA256 != m.Baseline.Objects["qs.mysql.schema_migrations"].DefinitionSHA256 || ledger.Rows == nil || *ledger.Rows != 1 || ledger.ContentSHA256 != m5LedgerContentHash(state.SQLVersion, state.SQLDirty) {
		return state, retirementError("original complete ledger structure/state changed")
	}
	if target, ok := state.Objects["qs.mysql.ai_messaging_outbox"]; ok {
		if !reflect.DeepEqual(target, m.Baseline.Objects["qs.mysql.ai_messaging_outbox"]) {
			return state, retirementError("existing original target is not exact empty material")
		}
		state.TargetPresent = true
	}
	if state.SQLVersion == 110 && (!state.TargetPresent || state.SQLDirty) {
		return state, retirementError("incomplete original110 state")
	}
	return state, nil
}

func validateM5RestoreApproval(a M5RestoreApproval, m M5OriginalMaterial, cfg M5RestoreConfig, stateHash string, now time.Time) error {
	expires, e := time.Parse(time.RFC3339, a.ExpiresAt)
	started, se := time.Parse(time.RFC3339, a.RecoveryStartedAt)
	if e != nil || se != nil || !strings.HasSuffix(a.ExpiresAt, "Z") || !strings.HasSuffix(a.RecoveryStartedAt, "Z") || !now.Before(expires) || started.After(now) || !now.Before(started.Add(time.Hour)) || expires.After(started.Add(time.Hour)) ||
		a.FormatVersion != 1 || a.Kind != "qs_m5_original_empty_restore" || a.OperationID != m.OperationID || a.ApprovedSourceSHA != cfg.Pair.ExpectedSourceSHA || a.OriginalSHA256 != cfg.OriginalSHA256 || a.ObservedStateSHA256 != stateHash || !a.RuntimeFrozen || !a.RuntimeNotStarted || !validOfflineText(a.DataOwner, 128) || !validOfflineText(a.RecoveryOwner, 128) {
		return retirementError("independent bounded pre-start restore approval required")
	}
	for _, hash := range []string{a.FreezeReceiptSHA256, a.BoundaryReceiptSHA256, a.ReviewSHA256} {
		if !retirementHashRE.MatchString(hash) || hash == strings.Repeat("0", 64) {
			return retirementError("actual restore evidence required")
		}
	}
	return nil
}

// RunM5OriginalRestore defaults to inspection. Only a separate consumed approval
// may restore this exact empty table and then replay the captured full ledger.
// The coordinator independently guards actual writer exclusion/server activity
// and the first-runtime boundary; assertions are never inferred from this read.
func RunM5OriginalRestore(ctx context.Context, db *sql.DB, client *mongo.Client, cfg M5RestoreConfig) (out M5RestoreResult, resultErr error) {
	out = M5RestoreResult{Stage: "input", ReadOnly: !cfg.Apply, OriginalSHA256: cfg.OriginalSHA256}
	if !cfg.Apply && (cfg.AuthorizationFile != "" || cfg.AuthorizationSHA256 != "") {
		return out, retirementError("readonly original inspection cannot consume approval")
	}
	resources, err := compiledMigrationResources()
	if err != nil {
		return out, err
	}
	var m M5OriginalMaterial
	if err = decodeM5Private(cfg.OriginalFile, cfg.OriginalSHA256, 1<<20, &m); err != nil {
		return out, err
	}
	if err = validateM5Original(m, cfg.Pair, resources); err != nil {
		return out, err
	}
	out.Stage = "inspect"
	state, err := inspectM5RestoreState(ctx, db, client, cfg.Pair, m)
	if err != nil {
		return out, err
	}
	stateRaw, err := json.Marshal(state)
	if err != nil {
		return out, err
	}
	out.State = &state
	out.ObservedStateSHA256 = retirementHash(stateRaw)
	out.AlreadyOriginal = state.SQLVersion == 110 && !state.SQLDirty && state.TargetPresent
	if !cfg.Apply {
		out.Stage = "inspection_complete"
		return out, nil
	}
	var approval M5RestoreApproval
	if err = decodeM5Private(cfg.AuthorizationFile, cfg.AuthorizationSHA256, 32768, &approval); err != nil {
		return out, err
	}
	if err = validateM5RestoreApproval(approval, m, cfg, out.ObservedStateSHA256, time.Now()); err != nil {
		return out, err
	}
	if _, err = os.Lstat(cfg.AuthorizationFile + ".consumed"); !errors.Is(err, os.ErrNotExist) {
		return out, retirementError("restore approval consumed or unknown")
	}
	if out.AlreadyOriginal {
		out.Stage = "already_original"
		out.ReadOnly = true
		out.Verified = true
		return out, nil
	}
	// Re-read original file/approval and the complete primary state immediately
	// before exclusive consumption. A changed row or material is not retried.
	var originalAgain M5OriginalMaterial
	var approvalAgain M5RestoreApproval
	if decodeM5Private(cfg.OriginalFile, cfg.OriginalSHA256, 1<<20, &originalAgain) != nil || decodeM5Private(cfg.AuthorizationFile, cfg.AuthorizationSHA256, 32768, &approvalAgain) != nil || !reflect.DeepEqual(originalAgain, m) || approvalAgain != approval {
		return out, retirementError("M5 recovery inputs changed")
	}
	stateAgain, err := inspectM5RestoreState(ctx, db, client, cfg.Pair, m)
	if err != nil {
		return out, err
	}
	rawAgain, _ := json.Marshal(stateAgain)
	if retirementHash(rawAgain) != out.ObservedStateSHA256 {
		return out, retirementError("M5 interrupted state changed")
	}
	if err = validateM5RestoreApproval(approval, m, cfg, out.ObservedStateSHA256, time.Now()); err != nil {
		return out, err
	}
	if err = claimPrivateRetirementAuthorization(cfg.AuthorizationFile, m.OperationID, cfg.AuthorizationSHA256); err != nil {
		return out, err
	}
	out.AuthorizationConsumed = true
	expires, _ := time.Parse(time.RFC3339, approval.ExpiresAt)
	ctx, cancel := context.WithDeadline(ctx, expires)
	defer cancel()
	connection, err := db.Conn(ctx)
	if err != nil {
		return out, err
	}
	defer func() { resultErr = errors.Join(resultErr, connection.Close()) }()
	if !state.TargetPresent {
		out.Stage = "original_target_ddl"
		out.MutationAttempted = true
		if _, err = connection.ExecContext(ctx, m.TargetDDL); err != nil {
			return out, retirementError("original target DDL outcome unknown")
		}
	}
	out.Stage = "verify_original_before_ledger"
	restored, err := inspectM5RestoreState(ctx, db, client, cfg.Pair, m)
	if err != nil || !restored.TargetPresent || restored.SQLVersion != 111 || restored.SQLDirty != state.SQLDirty {
		return out, retirementError("original target/retained verification failed; preserve interrupted ledger")
	}
	tx, err := connection.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	var version uint
	var dirty bool
	locked, err := tx.QueryContext(ctx, "SELECT version,dirty FROM schema_migrations FOR UPDATE")
	if err != nil {
		return out, err
	}
	lockedRows := 0
	for locked.Next() {
		if err = locked.Scan(&version, &dirty); err != nil {
			_ = locked.Close()
			return out, err
		}
		lockedRows++
	}
	if err = errors.Join(locked.Err(), locked.Close()); err != nil || lockedRows != 1 || version != 111 || dirty != state.SQLDirty {
		return out, retirementError("complete interrupted ledger changed before exact replay")
	}
	out.Stage = "original_complete_ledger"
	out.MutationAttempted = true
	out.LedgerReplayAttempted = true
	result, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version=? AND dirty=?", version, dirty)
	if err != nil {
		return out, retirementError("original ledger replay refused")
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return out, retirementError("exact original ledger replacement unconfirmed")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,dirty) VALUES(?,?)", m.LedgerRows[0].Version, m.LedgerRows[0].Dirty); err != nil {
		return out, retirementError("original complete ledger insert refused")
	}
	var restoredRows int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*),MIN(version),MAX(dirty) FROM schema_migrations").Scan(&restoredRows, &version, &dirty); err != nil || restoredRows != 1 || version != 110 || dirty {
		return out, retirementError("original complete ledger row differs")
	}
	out.Stage = "ledger_commit"
	out.CommitOutcomeUnknown = true
	if err = tx.Commit(); err != nil {
		return out, retirementError("original ledger commit outcome unknown; inspect only")
	}
	out.CommitOutcomeUnknown = false
	out.Stage = "verify_complete_original"
	final, err := inspectM5RestoreState(ctx, db, client, cfg.Pair, m)
	if err != nil || final.SQLVersion != 110 || final.SQLDirty || !final.TargetPresent || !reflect.DeepEqual(final.Objects, m.Baseline.Objects) {
		return out, retirementError("complete original material not restored")
	}
	out.State = &final
	out.Stage = "complete"
	out.Verified = true
	return out, nil
}
