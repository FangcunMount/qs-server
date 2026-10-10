package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"regexp"

	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	buildversion "github.com/FangcunMount/qs-server/pkg/version"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This is the fixed host's expected material binding, not source approval,
// writer quiescence or mutation permission. Run proves actual migration only.
type CompatibilityPairRunBinding struct {
	ApprovedSourceSHA string `json:"approved_source_sha"`
	OriginalSourceSHA string `json:"original_source_sha"`
	OperationID       string `json:"operation_id"`
	OriginalRunID     string `json:"original_run_id"`
	ActualRunID       string `json:"actual_run_id"`
	ManifestSHA256    string `json:"manifest_sha256"`
	ArchiveSHA256     string `json:"archive_sha256"`
	RequestSHA256     string `json:"request_sha256"`
	WindowStartSHA256 string `json:"window_start_sha256"`
	ResourcesSHA256   string `json:"resources_sha256"`
}
type CompatibilityPairMigrationObservation struct {
	Binding                     CompatibilityPairRunBinding `json:"binding"`
	SQLBefore                   uint                        `json:"mysql_before"`
	SQLAfter                    uint                        `json:"mysql_after"`
	MongoBefore                 uint                        `json:"mongodb_before"`
	MongoAfter                  uint                        `json:"mongodb_after"`
	SQLIdentitySHA256           string                      `json:"mysql_identity_sha256"`
	MongoBeforeIdentitySHA256   string                      `json:"mongodb_before_identity_sha256"`
	MongoAfterIdentitySHA256    string                      `json:"mongodb_after_identity_sha256"`
	MongoBeforeGenerationSHA256 string                      `json:"mongodb_before_generation_sha256"`
	MongoAfterGenerationSHA256  string                      `json:"mongodb_after_generation_sha256"`
	MongoBeforeMigrationUUID    string                      `json:"mongodb_before_migration_uuid"`
	MongoAfterMigrationUUID     string                      `json:"mongodb_after_migration_uuid"`
	SQLChanged                  bool                        `json:"mysql_changed"`
	MongoChanged                bool                        `json:"mongodb_changed"`
}

// No serialized observation can reconstruct this native, same-process proof.
// It does not prove host approval, writer fencing, business acceptance or DROP.
type CompatibilityPairMigrationProof struct {
	self        *CompatibilityPairMigrationProof
	pair        *PairPreflight
	observation CompatibilityPairMigrationObservation
	anchor      *identitymeta.MongoNamespaceAnchor
}

func (*CompatibilityPairMigrationProof) MarshalJSON() ([]byte, error) {
	return nil, retirementError("migration proof serialization refused")
}
func (p *CompatibilityPairMigrationProof) Observation() CompatibilityPairMigrationObservation {
	if p == nil || p.self != p {
		return CompatibilityPairMigrationObservation{}
	}
	return p.observation
}
func transitionBindingValid(b CompatibilityPairRunBinding, expectedSource string) bool {
	source := regexp.MustCompile(`^[0-9a-f]{40}$`)
	if !source.MatchString(expectedSource) || b.ApprovedSourceSHA != expectedSource || !source.MatchString(b.OriginalSourceSHA) || !retirementOpRE.MatchString(b.OperationID) || !retirementOpRE.MatchString(b.OriginalRunID) || !retirementOpRE.MatchString(b.ActualRunID) || b.ResourcesSHA256 != CompatibilityMigrationResourcesSHA256() {
		return false
	}
	for _, h := range []string{b.ManifestSHA256, b.ArchiveSHA256, b.RequestSHA256, b.WindowStartSHA256} {
		if !retirementHashRE.MatchString(h) {
			return false
		}
	}
	return true
}

// PreflightCompatibilityPairConnection borrows the fixed host's dedicated
// autocommit connection. It neither acquires a pool connection nor closes one.
// Cold bootstrap remains a separate externally approved startup path.
func PreflightCompatibilityPairConnection(ctx context.Context, conn *sql.Conn, client *mongo.Client, cfg PairConfig) (*PairPreflight, error) {
	if ctx == nil || ctx.Err() != nil || conn == nil || client == nil || mongo.SessionFromContext(ctx) != nil {
		return nil, retirementError("controlled pair borrowed context rejected")
	}
	if e := controlledSQLContext(ctx, conn); e != nil {
		return nil, e
	}
	a, b, e := pairIdentity(ctx, conn, client, cfg.MySQLDatabase, cfg.MongoDatabase)
	if e != nil {
		return nil, e
	}
	s, e := observeSQLPair(ctx, conn, cfg.MySQLDatabase)
	if e != nil {
		return nil, e
	}
	m, e := observeMongoPair(ctx, client, cfg.MongoDatabase)
	if e != nil {
		return nil, e
	}
	if s.pristine || m.pristine || s.version != 99 || m.version != 38 {
		return nil, retirementError("controlled migration requires exact installed 99/38 pair")
	}
	return &PairPreflight{sqlConn: conn, mongo: client, config: cfg, sqlHash: a, mongoHash: b, initialSQLVersion: 99, initialMongoVersion: 38}, nil
}

func transitionParts(parts ...string) string {
	h := sha256.New()
	for _, s := range parts {
		var frame [9]byte
		frame[0] = 1
		binary.BigEndian.PutUint64(frame[1:], uint64(len(s)))
		_, _ = h.Write(frame[:])
		_, _ = h.Write([]byte(s))
	}
	return hex.EncodeToString(h.Sum(nil))
}

type transitionState struct {
	sqlVersion, mongoVersion                                            uint
	sqlIdentity, mongoIdentity, mongoGeneration, mongoUUID, mongoStable string
}

func (p *PairPreflight) transitionState(ctx context.Context, anchor *identitymeta.MongoNamespaceAnchor, absent bool) (transitionState, error) {
	var s transitionState
	if p == nil || p.sqlConn == nil || p.mongo == nil {
		return s, retirementError("controlled pair connection missing")
	}
	q, c := retirementContext(ctx)
	defer c()
	if e := controlledSQLContext(q, p.sqlConn); e != nil {
		return s, e
	}
	var uuid, name string
	if p.sqlConn.QueryRowContext(q, "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &name) != nil || name != p.config.MySQLDatabase || !retirementUUIDRE.MatchString(uuid) {
		return s, retirementError("transition mysql identity rejected")
	}
	s.sqlIdentity = transitionParts("mysql_database_identity_v1", uuid, name)
	if absent {
		a, e := observeSQLPair(q, p.sqlConn, name)
		if e != nil || a.pristine || a.dirty {
			return s, retirementError("transition mysql state rejected")
		}
		s.sqlVersion = a.version
	} else {
		rows, e := p.sqlConn.QueryContext(q, "SELECT version,dirty FROM schema_migrations LIMIT 2")
		if e != nil {
			return s, retirementError("transition mysql clean head unknown")
		}
		n := 0
		valid := true
		for rows.Next() {
			var dirty bool
			if rows.Scan(&s.sqlVersion, &dirty) != nil || dirty || s.sqlVersion != 100 {
				valid = false
			}
			n++
		}
		re, ce := rows.Err(), rows.Close()
		if re != nil || ce != nil || n != 1 || !valid {
			return s, retirementError("transition mysql clean head rejected")
		}
	}
	db := p.mongo.Database(p.config.MongoDatabase)
	var hello bson.Raw
	if p.mongo.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		return s, retirementError("transition mongo identity unknown")
	}
	stable := bson.D{}
	for _, key := range []string{"setName", "hosts", "me"} {
		value := hello.Lookup(key)
		if value.Type != 0 {
			var v any
			if value.Unmarshal(&v) != nil {
				return s, retirementError("transition mongo identity rejected")
			}
			stable = append(stable, bson.E{Key: key, Value: v})
		}
	}
	encoded, e := json.Marshal(stable)
	if e != nil {
		return s, retirementError("transition mongo identity rejected")
	}
	s.mongoStable = retirementHash(encoded)
	cursor, e := db.ListCollections(q, bson.D{}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		return s, retirementError("transition mongo visibility unknown")
	}
	var rows []bson.Raw
	e = cursor.All(q, &rows)
	ce := cursor.Close(q)
	if e != nil || ce != nil || len(rows) > 500 {
		return s, retirementError("transition mongo catalog rejected")
	}
	for _, row := range rows {
		n, ok := row.Lookup("name").StringValueOK()
		if !ok {
			return s, retirementError("transition mongo namespace malformed")
		}
		if absent && n == "domain_event_outbox" {
			return s, retirementError("transition mongo target present")
		}
		if n == "schema_migrations" {
			subtype, raw, ok := row.Lookup("info", "uuid").BinaryOK()
			if !ok || subtype != 4 || len(raw) != 16 || s.mongoUUID != "" {
				return s, retirementError("transition mongo migration uuid rejected")
			}
			s.mongoUUID = hex.EncodeToString(raw)
		}
	}
	if s.mongoUUID == "" {
		return s, retirementError("transition mongo migration namespace absent")
	}
	if anchor != nil {
		if anchor.Validate() != nil || anchor.Database != db.Name() {
			return s, retirementError("transition namespace profile rejected")
		}
		observed, e := identitymeta.MongoNamespaceAnchorFromMetadata(hello, rows, db.Name(), anchor.EndpointSHA256)
		if e != nil || !identitymeta.MatchMongoNamespaceAnchors(anchor, observed) {
			return s, retirementError("transition kept namespace changed")
		}
	}
	cursor, e = db.Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2))
	if e != nil {
		return s, retirementError("transition mongo head unknown")
	}
	var heads []bson.Raw
	e = cursor.All(q, &heads)
	ce = cursor.Close(q)
	if e != nil || ce != nil || len(heads) != 1 {
		return s, retirementError("transition mongo head rejected")
	}
	var v int64
	field := heads[0].Lookup("version")
	if x, ok := field.Int32OK(); ok {
		v = int64(x)
	} else if x, ok := field.Int64OK(); ok {
		v = x
	} else {
		return s, retirementError("transition mongo head malformed")
	}
	dirty, ok := heads[0].Lookup("dirty").BooleanOK()
	if !ok || dirty || v < 0 || (v != 38 && v != 39) {
		return s, retirementError("transition mongo head rejected")
	}
	s.mongoVersion = uint(v)
	s.mongoIdentity = transitionParts("mongodb_database_identity_v1", string(encoded), db.Name(), s.mongoUUID)
	s.mongoGeneration = transitionParts("mongodb_migration_generation_v1", s.mongoUUID)
	return s, nil
}

// Attempt is diagnostic physical-journal material, never a completion proof.
// ReturnedVersion is the migrator's response, not a fresh head observation.
type CompatibilityPairRunAttempt struct {
	SQLAttempted         bool   `json:"mysql_attempted"`
	SQLResponse          string `json:"mysql_response"`
	SQLReturnedVersion   uint   `json:"mysql_returned_version"`
	SQLChanged           bool   `json:"mysql_changed"`
	MongoAttempted       bool   `json:"mongodb_attempted"`
	MongoResponse        string `json:"mongodb_response"`
	MongoReturnedVersion uint   `json:"mongodb_returned_version"`
	MongoChanged         bool   `json:"mongodb_changed"`
	Completion           string `json:"completion"`
}

// Run proves only one genuine, same-process 99/38 -> 100/39 transition. It
// refuses adoption of another process's completed/partial migration. The host
// must supervise its existing process deadline: upstream Mongo Run/SetVersion
// use context.TODO, so ctx expiration never becomes a completion proof.
func (p *PairPreflight) Run(ctx context.Context, b CompatibilityPairRunBinding, anchor *identitymeta.MongoNamespaceAnchor) (*CompatibilityPairMigrationProof, CompatibilityPairRunAttempt, error) {
	attempt := CompatibilityPairRunAttempt{SQLResponse: "not_attempted", MongoResponse: "not_attempted", Completion: "not_started"}
	if ctx == nil || ctx.Err() != nil || p == nil || p.sqlConn == nil || p.mongo == nil || mongo.SessionFromContext(ctx) != nil {
		return nil, attempt, retirementError("controlled pair context rejected")
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, attempt, retirementError("controlled migration deadline missing")
	}
	built := buildversion.Get().GitCommit
	if !transitionBindingValid(b, built) || p.config.ExpectedSourceSHA != built {
		return nil, attempt, retirementError("controlled migration material binding rejected")
	}
	anchor = anchor.Clone()
	p.mu.Lock()
	if p.controlledRun {
		p.mu.Unlock()
		return nil, attempt, retirementError("controlled migration already attempted")
	}
	p.controlledRun = true
	p.mu.Unlock()
	before, e := p.transitionState(ctx, anchor, true)
	if e != nil || before.sqlVersion != 99 || before.mongoVersion != 38 {
		return nil, attempt, retirementError("controlled before transition rejected")
	}
	attempt.Completion = "partial_or_unknown"
	sqlConfig := p.MySQLConfig(false)
	sqlConfig.runContext = ctx
	sqlMigrator := &Migrator{driver: &MySQLDriver{embeddedMigrationDriver: newEmbeddedMigrationDriver(BackendMySQL, "migrations/mysql", "mysql"), borrowedConn: p.sqlConn}, config: ensureConfigDefaults(sqlConfig)}
	attempt.SQLAttempted = true
	sv, sc, e := sqlMigrator.Run()
	attempt.SQLReturnedVersion = sv
	attempt.SQLChanged = sc
	attempt.SQLResponse = "error_or_unknown"
	if e == nil {
		attempt.SQLResponse = "success"
	}
	if e != nil || !sc || sv != 100 || ctx.Err() != nil {
		return nil, attempt, retirementError("controlled mysql result partial or unknown")
	}
	mongoConfig := p.MongoConfig(false)
	mongoConfig.runContext = ctx
	attempt.MongoAttempted = true
	mv, mc, e := NewMongoMigrator(p.mongo, mongoConfig).Run()
	attempt.MongoReturnedVersion = mv
	attempt.MongoChanged = mc
	attempt.MongoResponse = "error_or_unknown"
	if e == nil {
		attempt.MongoResponse = "success"
	}
	if e != nil || !mc || mv != 39 || ctx.Err() != nil {
		return nil, attempt, retirementError("controlled mongo result partial or unknown")
	}
	after, e := p.transitionState(ctx, anchor, true)
	if e != nil || ctx.Err() != nil || after.sqlVersion != 100 || after.mongoVersion != 39 || before.sqlIdentity != after.sqlIdentity || before.mongoStable != after.mongoStable || before.mongoUUID == after.mongoUUID {
		return nil, attempt, retirementError("controlled after transition rejected")
	}
	proof := &CompatibilityPairMigrationProof{pair: p, anchor: anchor.Clone(), observation: CompatibilityPairMigrationObservation{Binding: b, SQLBefore: 99, SQLAfter: 100, MongoBefore: 38, MongoAfter: 39, SQLIdentitySHA256: after.sqlIdentity, MongoBeforeIdentitySHA256: before.mongoIdentity, MongoAfterIdentitySHA256: after.mongoIdentity, MongoBeforeGenerationSHA256: before.mongoGeneration, MongoAfterGenerationSHA256: after.mongoGeneration, MongoBeforeMigrationUUID: before.mongoUUID, MongoAfterMigrationUUID: after.mongoUUID, SQLChanged: sc, MongoChanged: mc}}
	proof.self = proof
	attempt.Completion = "completed"
	return proof, attempt, nil
}

// VerifyAfter does not require the four namespaces still absent: the same
// approved recovery may restore them while retaining these migration heads.
func (p *CompatibilityPairMigrationProof) VerifyAfter(ctx context.Context, conn *sql.Conn, db *mongo.Database) error {
	if ctx == nil || ctx.Err() != nil || p == nil || p.self != p || p.pair == nil || conn == nil || conn != p.pair.sqlConn || db == nil || db.Client() != p.pair.mongo || db.Name() != p.pair.config.MongoDatabase || mongo.SessionFromContext(ctx) != nil {
		return retirementError("transition proof binding rejected")
	}
	s, e := p.pair.transitionState(ctx, p.anchor, false)
	o := p.observation
	if e != nil || s.sqlVersion != 100 || s.mongoVersion != 39 || s.sqlIdentity != o.SQLIdentitySHA256 || s.mongoIdentity != o.MongoAfterIdentitySHA256 || s.mongoGeneration != o.MongoAfterGenerationSHA256 || s.mongoUUID != o.MongoAfterMigrationUUID {
		return retirementError("transition after generation drift")
	}
	return nil
}

// Autocommit by itself cannot prove no START TRANSACTION. The fixed borrowed
// connection must expose actual instrumented state; permissions/disabled
// instrumentation are failures, and this observer never enables them.
func controlledSQLContext(ctx context.Context, conn *sql.Conn) error {
	if conn == nil || ctx == nil || ctx.Err() != nil {
		return retirementError("controlled sql context missing")
	}
	q, c := retirementContext(ctx)
	defer c()
	var auto, foreign int
	var enabled, consumer string
	var thread uint64
	if conn.QueryRowContext(q, "SELECT @@session.autocommit,@@session.foreign_key_checks").Scan(&auto, &foreign) != nil || auto != 1 || foreign != 1 || conn.QueryRowContext(q, "SELECT ENABLED FROM performance_schema.setup_instruments WHERE NAME='transaction'").Scan(&enabled) != nil || enabled != "YES" || conn.QueryRowContext(q, "SELECT ENABLED FROM performance_schema.setup_consumers WHERE NAME='events_transactions_current'").Scan(&consumer) != nil || consumer != "YES" || conn.QueryRowContext(q, "SELECT THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=CONNECTION_ID()").Scan(&thread) != nil || thread == 0 {
		return retirementError("controlled sql transaction visibility unproven")
	}
	rows, e := conn.QueryContext(q, "SELECT STATE,END_EVENT_ID FROM performance_schema.events_transactions_current WHERE THREAD_ID=? LIMIT 2", thread)
	if e != nil {
		return retirementError("controlled sql transaction unknown")
	}
	n := 0
	valid := true
	for rows.Next() {
		var state string
		var end sql.NullInt64
		if rows.Scan(&state, &end) != nil || (state != "COMMITTED" && state != "ROLLED BACK") || !end.Valid {
			valid = false
		}
		n++
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || n > 1 || !valid {
		return retirementError("controlled sql transaction active or unknown")
	}
	var active uint64
	if conn.QueryRowContext(q, "SELECT COUNT(*) FROM information_schema.innodb_trx WHERE trx_mysql_thread_id=CONNECTION_ID()").Scan(&active) != nil || active != 0 {
		return retirementError("controlled sql transaction active or unknown")
	}
	return nil
}
