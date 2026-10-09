package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"database/sql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type identityRequest struct {
	FormatVersion      int               `json:"format_version"`
	Kind               string            `json:"kind"`
	SourceSHA          string            `json:"source_sha"`
	OperationID        string            `json:"operation_id"`
	TargetHash         string            `json:"target_hash"`
	DatabaseScope      string            `json:"database_scope"`
	Protocols          map[string]string `json:"identity_protocols"`
	MongoAnchorProfile string            `json:"mongo_anchor_profile,omitempty"`
	Limits             struct {
		QuerySeconds int `json:"query_seconds"`
		TotalSeconds int `json:"total_seconds"`
	} `json:"limits"`
}
type identityState struct {
	IdentityHash            string                             `json:"identity_hash"`
	DatabaseAnchorHash      string                             `json:"database_anchor_hash"`
	NamespaceAnchor         *identitymeta.MongoNamespaceAnchor `json:"namespace_anchor,omitempty"`
	MigrationGenerationHash string                             `json:"migration_generation_hash"`
	IdentityObserved        bool                               `json:"identity_observed"`
	Version                 uint64                             `json:"migration_version"`
	HeadObserved            bool                               `json:"migration_head_observed"`
	Dirty                   *bool                              `json:"migration_dirty"`
	Clean                   bool                               `json:"migration_clean"`
	PermissionsSufficient   bool                               `json:"metadata_permissions_sufficient"`
	PermissionScope         string                             `json:"permission_scope"`
	ErrorCategory           string                             `json:"error_category"`
}
type identityReport struct {
	FormatVersion  int                      `json:"format_version"`
	Kind           string                   `json:"kind"`
	SourceSHA      string                   `json:"source_sha"`
	OperationID    string                   `json:"operation_id"`
	RunID          string                   `json:"run_id"`
	RequestHash    string                   `json:"request_hash"`
	TargetHash     string                   `json:"target_hash"`
	DiagnosticOnly bool                     `json:"diagnostic_only"`
	DropReady      bool                     `json:"drop_ready"`
	Complete       bool                     `json:"complete"`
	Protocols      map[string]string        `json:"identity_protocols"`
	States         map[string]identityState `json:"database_states"`
	Histograms     []diagnosticHistogram    `json:"diagnostic_histograms"`
	ErrorCategory  string                   `json:"error_category"`
}

func identityProtocols() map[string]string {
	return map[string]string{"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"}
}
func loadIdentityRequest(path, expected, op string) (identityRequest, error) {
	var r identityRequest
	if !hashRE.MatchString(expected) || !runRE.MatchString(op) || !shaRE.MatchString(sourceSHA) || filepath.Base(path) != "identity-request.json" {
		return r, category("identity_request_binding_invalid")
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() < 1 || info.Size() > 256*1024 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		return r, category("identity_request_private_invalid")
	}
	raw, e := os.ReadFile(path)
	if e != nil || digestRaw(raw) != expected {
		return r, category("identity_request_hash_mismatch")
	}
	if rejectDuplicateJSON(raw) != nil || identitymeta.ValidateOptionalMongoNamespaceJSON(raw, "mongo_anchor_profile") != nil {
		return r, category("identity_request_schema_invalid")
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF {
		return r, category("identity_request_schema_invalid")
	}
	if r.FormatVersion != 1 || r.Kind != "readonly_identity_discovery_request" || r.SourceSHA != sourceSHA || r.OperationID != op || r.TargetHash != digest(targets) || r.DatabaseScope != "mysql-and-mongodb" || digest(r.Protocols) != digest(identityProtocols()) || r.Limits.QuerySeconds != 15 || r.Limits.TotalSeconds != 90 || (r.MongoAnchorProfile != "" && r.MongoAnchorProfile != identitymeta.MongoReplicaAnchorKind && r.MongoAnchorProfile != identitymeta.MongoNamespaceAnchorKind) {
		return r, category("identity_request_binding_invalid")
	}
	return r, nil
}
func readIdentityRequest(path, expected, op string) error {
	_, err := loadIdentityRequest(path, expected, op)
	return err
}
func discoverMySQL(ctx context.Context) (state identityState, err error) {
	state.ErrorCategory = "none"
	state.PermissionScope = "identity_and_migration_head"
	db, e := mysqlOpen(ctx)
	if e != nil {
		return state, e
	}
	defer func() {
		if closeErr := db.Close(); err == nil && closeErr != nil {
			err = category("mysql_close_failed")
		}
	}()
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return state, category("mysql_readonly_transaction_failed")
	}
	defer func() {
		if closeErr := tx.Rollback(); err == nil && closeErr != nil && closeErr != sql.ErrTxDone {
			err = category("mysql_readonly_close_failed")
		}
	}()
	rows, e := scanSQL(ctx, tx, "SELECT @@server_uuid,DATABASE(),VERSION()")
	if e != nil || len(rows) != 1 || val(rows[0], 1) != os.Getenv("MYSQL_DATABASE") || !strings.HasPrefix(val(rows[0], 2), "8.") || !mysqlUUIDRE.MatchString(val(rows[0], 0)) {
		return state, category("mysql_identity_or_version_rejected")
	}
	state.IdentityHash = hashParts("mysql_database_identity_v1", val(rows[0], 0), val(rows[0], 1))
	state.DatabaseAnchorHash = state.IdentityHash
	state.IdentityObserved = true
	grants, e := scanSQL(ctx, tx, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil {
		return state, e
	}
	rights := map[string]bool{}
	revoked := false
	for _, g := range grants {
		text := val(g, 0)
		if strings.HasPrefix(text, "REVOKE ") {
			revoked = true
		}
		if strings.HasPrefix(text, "GRANT ") && strings.Contains(text, " ON *.* TO ") {
			priv := strings.TrimPrefix(strings.Split(text, " ON *.* TO ")[0], "GRANT ")
			for _, needed := range []string{"SELECT", "SHOW VIEW", "TRIGGER", "EVENT"} {
				if priv == "ALL PRIVILEGES" || containsPrivilege(priv, needed) {
					rights[needed] = true
				}
			}
		}
	}
	state.PermissionsSufficient = !revoked && rights["SELECT"] && rights["SHOW VIEW"] && rights["TRIGGER"] && rights["EVENT"]
	if !state.PermissionsSufficient {
		return state, category("mysql_global_metadata_visibility_unproven")
	}
	heads, e := scanSQL(ctx, tx, "SELECT version,dirty FROM schema_migrations LIMIT 2")
	if e != nil || len(heads) != 1 {
		return state, category("mysql_migration_head_invalid")
	}
	state.Version, e = strconv.ParseUint(val(heads[0], 0), 10, 64)
	if e != nil || state.Version == 0 || (val(heads[0], 1) != "0" && val(heads[0], 1) != "1") {
		return state, category("mysql_migration_head_invalid")
	}
	dirty := val(heads[0], 1) == "1"
	state.Dirty = &dirty
	state.HeadObserved = true
	state.Clean = !dirty
	if dirty {
		return state, category("migration_head_rejected")
	}
	return state, nil
}
func discoverMongo(ctx context.Context) (identityState, error) {
	return discoverMongoProfile(ctx, identitymeta.MongoReplicaAnchorKind)
}
func discoverMongoProfile(ctx context.Context, profile string) (state identityState, err error) {
	state.ErrorCategory = "none"
	state.PermissionScope = "identity_and_migration_head"
	client, e := mongoOpen(ctx)
	if e != nil {
		return state, e
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := client.Disconnect(closeCtx); err == nil && closeErr != nil {
			err = category("mongo_close_failed")
		}
	}()
	db := client.Database(os.Getenv("MONGODB_DBNAME"))
	var hello bson.Raw
	q, cancel := queryContext(ctx)
	e = client.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
	cancel()
	if e != nil {
		return state, category("mongo_identity_read_failed")
	}
	var build struct {
		Version string `bson:"version"`
	}
	q, cancel = queryContext(ctx)
	e = client.Database("admin").RunCommand(q, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build)
	cancel()
	if e != nil || !strings.HasPrefix(build.Version, "7.") {
		return state, category("mongo_version_rejected")
	}
	q, cancel = queryContext(ctx)
	cursor, e := db.ListCollections(q, bson.D{{Key: "name", Value: "schema_migrations"}}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		cancel()
		return state, category("mongo_identity_metadata_permission_or_missing")
	}
	var entries []bson.Raw
	e = cursor.All(q, &entries)
	closeErr := cursor.Close(q)
	cancel()
	if e != nil || closeErr != nil || len(entries) != 1 || entries[0].Lookup("type").StringValue() != "collection" {
		return state, category("mongo_identity_metadata_permission_or_missing")
	}
	uuid := entries[0].Lookup("info", "uuid")
	if uuid.Type != bson.TypeBinary {
		return state, category("mongo_database_uuid_unavailable")
	}
	subtype, bytes := uuid.Binary()
	if subtype != 4 || len(bytes) != 16 {
		return state, category("mongo_database_uuid_unavailable")
	}
	stable := bson.D{}
	for _, name := range []string{"setName", "hosts", "me"} {
		value := hello.Lookup(name)
		if value.Type != 0 {
			var v any
			if value.Unmarshal(&v) != nil {
				return state, category("mongo_identity_read_failed")
			}
			stable = append(stable, bson.E{Key: name, Value: v})
		}
	}
	canonical, _ := json.Marshal(stable)
	state.IdentityHash = hashParts("mongodb_database_identity_v1", string(canonical), os.Getenv("MONGODB_DBNAME"), hex.EncodeToString(bytes))
	switch profile {
	case identitymeta.MongoNamespaceAnchorKind:
		state.NamespaceAnchor, e = mongoNamespaceAnchor(ctx, db)
		if e != nil {
			return state, e
		}
		state.DatabaseAnchorHash = state.NamespaceAnchor.Hash
	case "", identitymeta.MongoReplicaAnchorKind:
		state.DatabaseAnchorHash, e = mongoDatabaseAnchor(ctx, db, hello)
		if e != nil {
			return state, e
		}
	default:
		return state, category("mongo_namespace_anchor_rejected")
	}
	state.MigrationGenerationHash, e = mongoMigrationGeneration(entries[0])
	if e != nil {
		return state, e
	}
	state.IdentityObserved = true
	var privileges bson.Raw
	q, cancel = queryContext(ctx)
	e = client.Database("admin").RunCommand(q, bson.D{{Key: "connectionStatus", Value: 1}, {Key: "showPrivileges", Value: true}}).Decode(&privileges)
	cancel()
	if e != nil {
		return state, category("mongo_privileges_read_failed")
	}
	q, cancel = queryContext(ctx)
	cursor, e = db.Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2))
	if e != nil {
		cancel()
		return state, category("mongo_migration_head_invalid")
	}
	var heads []bson.M
	e = cursor.All(q, &heads)
	closeErr = cursor.Close(q)
	cancel()
	if e != nil || closeErr != nil || len(heads) != 1 {
		return state, category("mongo_migration_head_invalid")
	}
	switch version := heads[0]["version"].(type) {
	case int32:
		if version < 1 {
			return state, category("mongo_migration_head_invalid")
		}
		state.Version = uint64(version)
	case int64:
		if version < 1 {
			return state, category("mongo_migration_head_invalid")
		}
		state.Version = uint64(version)
	default:
		return state, category("mongo_migration_head_invalid")
	}
	dirty, ok := heads[0]["dirty"].(bool)
	if !ok {
		return state, category("mongo_migration_head_invalid")
	}
	state.Dirty = &dirty
	state.HeadObserved = true
	state.Clean = !dirty
	state.PermissionsSufficient = true
	if dirty {
		return state, category("migration_head_rejected")
	}
	return state, nil
}
func runIdentity(path, expected, op, runID, output string) (identityReport, error) {
	r := identityReport{FormatVersion: 1, Kind: "readonly_identity_discovery", SourceSHA: sourceSHA, OperationID: op, RunID: runID, RequestHash: expected, TargetHash: digest(targets), DiagnosticOnly: true, DropReady: false, Protocols: identityProtocols(), States: map[string]identityState{}, ErrorCategory: "identity_discovery_incomplete"}
	if !runRE.MatchString(runID) || privateDir(filepath.Dir(path)) != nil || privateDir(output) != nil {
		return r, category("input_or_private_path_invalid")
	}
	approved, e := loadIdentityRequest(path, expected, op)
	if e != nil {
		return r, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, sqlErr := discoverMySQL(ctx)
	if sqlErr != nil {
		a.ErrorCategory = sqlErr.Error()
	}
	r.States["mysql"] = a
	var b identityState
	var mongoErr error
	if approved.MongoAnchorProfile == "" {
		b, mongoErr = discoverMongo(ctx)
	} else {
		b, mongoErr = discoverMongoProfile(ctx, approved.MongoAnchorProfile)
	}
	if mongoErr != nil {
		b.ErrorCategory = mongoErr.Error()
	}
	r.States["mongodb"] = b
	if sqlErr == nil && mongoErr == nil {
		r.Complete = true
		r.ErrorCategory = "none"
	}
	r.Histograms = discoverHistograms(ctx, sqlErr == nil, mongoErr == nil)
	if e := writeJSON(filepath.Join(output, "identity.private.json"), r); e != nil {
		return r, e
	}
	if !r.Complete {
		return r, category("identity_discovery_incomplete")
	}
	return r, nil
}
func identitySummary(r identityReport) map[string]any {
	encoded, _ := json.Marshal(r)
	return map[string]any{"format_version": r.FormatVersion, "kind": r.Kind, "source_sha": r.SourceSHA, "operation_id": r.OperationID, "run_id": r.RunID, "request_hash": r.RequestHash, "target_hash": r.TargetHash, "diagnostic_only": true, "drop_ready": false, "complete": r.Complete, "identity_protocols": r.Protocols, "database_states": r.States, "diagnostic_histograms": r.Histograms, "error_category": r.ErrorCategory, "private_report_hash": digestRaw(append(encoded, '\n'))}
}
