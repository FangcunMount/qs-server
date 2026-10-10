package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// This public host producer owns connections and executes actual Capture and
// isolated restores. It neither retires historical responsibility nor fences
// production writers. No returned JSON summary is adopted as a restore proof.
type lifecyclePreparationOwner struct {
	originalSQL               *sql.DB
	originalMongo             *mongo.Client
	restoreSQL                *sql.DB
	restoreMongo              *mongo.Client
	originalConn, restoreConn *sql.Conn
	originalDB, restoreDB     *mongo.Database
	engines                   []*lifecycleOwnedEngine
	restoreContext            context.Context
	restoreCancel             context.CancelFunc
	combinedStarted           time.Time
	materialRecords           map[string]string
}

// The original exclusive writer records its own complete encoded bytes only
// after write, sync and close succeed. This is a file handoff, never a proof of
// restore, database acceptance or permission to remove a resource.
func writeLifecycleMaterialJSON(path string, value any, records map[string]string) error {
	name := filepath.Base(path)
	if records == nil || records[name] != "" {
		return lifecycleError("lifecycle_material_registration_rejected")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = writeJSON(path, value); err != nil {
		return err
	}
	records[name] = digestRaw(append(raw, '\n'))
	return nil
}

func lifecycleConnectionValue(prefix, name string) (string, error) {
	value := os.Getenv(prefix + name)
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", lifecycleError("lifecycle_connection_input_rejected")
	}
	return value, nil
}
func lifecycleSQLPool(ctx context.Context, prefix string) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	var err error
	if cfg.User, err = lifecycleConnectionValue(prefix, "MYSQL_USERNAME"); err != nil {
		return nil, err
	}
	if cfg.Passwd, err = lifecycleConnectionValue(prefix, "MYSQL_PASSWORD"); err != nil {
		return nil, err
	}
	if cfg.DBName, err = lifecycleConnectionValue(prefix, "MYSQL_DATABASE"); err != nil {
		return nil, err
	}
	host, err := lifecycleConnectionValue(prefix, "MYSQL_HOST")
	if err != nil {
		return nil, err
	}
	port, err := envPort(prefix+"MYSQL_PORT", 3306)
	if err != nil {
		return nil, lifecycleError("lifecycle_connection_input_rejected")
	}
	cfg.Net, cfg.Addr = "tcp", net.JoinHostPort(host, strconv.Itoa(port))
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 10*time.Second, 30*time.Second, 30*time.Second
	cfg.MultiStatements = true // Exact captured DDL; no splitting or DSN server-variable charset.
	if cfg.Apply(mysql.Charset("utf8mb4", "")) != nil {
		return nil, lifecycleError("lifecycle_connection_input_rejected")
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, lifecycleError("lifecycle_sql_connect_failed")
	}
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	if pool.PingContext(ctx) != nil {
		_ = pool.Close()
		return nil, lifecycleError("lifecycle_sql_connect_failed")
	}
	return pool, nil
}
func lifecycleMongoClient(ctx context.Context, prefix string) (*mongo.Client, *mongo.Database, error) {
	host, err := lifecycleConnectionValue(prefix, "MONGODB_HOST")
	if err != nil {
		return nil, nil, err
	}
	credentialPrefix := prefix
	if prefix == "" && (os.Getenv("MONGODB_METADATA_ADMIN_USERNAME") != "" || os.Getenv("MONGODB_METADATA_ADMIN_PASSWORD") != "") {
		credentialPrefix = "MONGODB_METADATA_ADMIN_"
	}
	userKey, passwordKey := "MONGODB_USERNAME", "MONGODB_PASSWORD"
	if credentialPrefix == "MONGODB_METADATA_ADMIN_" {
		userKey, passwordKey = "USERNAME", "PASSWORD"
	}
	user, err := lifecycleConnectionValue(credentialPrefix, userKey)
	if err != nil {
		return nil, nil, err
	}
	password, err := lifecycleConnectionValue(credentialPrefix, passwordKey)
	if err != nil {
		return nil, nil, err
	}
	database, err := lifecycleConnectionValue(prefix, "MONGODB_DBNAME")
	if err != nil {
		return nil, nil, err
	}
	port, err := envPort(prefix+"MONGODB_PORT", 27017)
	if err != nil {
		return nil, nil, lifecycleError("lifecycle_connection_input_rejected")
	}
	opts := options.Client().SetHosts([]string{net.JoinHostPort(host, strconv.Itoa(port))}).SetAuth(options.Credential{Username: user, Password: password, AuthSource: "admin"}).SetConnectTimeout(10 * time.Second).SetServerSelectionTimeout(10 * time.Second).SetSocketTimeout(30 * time.Second).SetMaxPoolSize(1).SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority())
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, nil, lifecycleError("lifecycle_mongo_connect_failed")
	}
	if client.Ping(ctx, readpref.Primary()) != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, lifecycleError("lifecycle_mongo_connect_failed")
	}
	return client, client.Database(database), nil
}
func openLifecyclePreparationOwner(ctx context.Context, r lifecycleRequest) (owner *lifecyclePreparationOwner, result error) {
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 {
		return nil, lifecycleError("lifecycle_fixed_root_host_required")
	}
	owner = &lifecyclePreparationOwner{materialRecords: map[string]string{}}
	defer func() {
		if result != nil {
			_ = owner.Close()
			owner = nil
		}
	}()
	owner.originalSQL, result = lifecycleSQLPool(ctx, "")
	if result != nil {
		return owner, result
	}
	owner.originalMongo, owner.originalDB, result = lifecycleMongoClient(ctx, "")
	return owner, result
}
func (o *lifecyclePreparationOwner) closeHandles(ctx context.Context) error {
	if o == nil {
		return nil
	}
	var result error
	for _, conn := range []*sql.Conn{o.originalConn, o.restoreConn} {
		if conn != nil && conn.Close() != nil {
			result = lifecycleError("lifecycle_owned_handle_close_failed")
		}
	}
	o.originalConn, o.restoreConn = nil, nil
	for _, pool := range []*sql.DB{o.originalSQL, o.restoreSQL} {
		if pool != nil && pool.Close() != nil {
			result = lifecycleError("lifecycle_owned_handle_close_failed")
		}
	}
	o.originalSQL, o.restoreSQL = nil, nil
	for _, client := range []*mongo.Client{o.originalMongo, o.restoreMongo} {
		if client != nil && client.Disconnect(ctx) != nil {
			result = lifecycleError("lifecycle_owned_handle_close_failed")
		}
	}
	o.originalMongo, o.restoreMongo = nil, nil
	for _, engine := range o.engines {
		if engine.closeWires() != nil {
			result = lifecycleError("lifecycle_restore_wire_reap_failed")
		}
	}
	return result
}

// Failure cleanup uses a bounded independent fallback, never a success proof.
func (o *lifecyclePreparationOwner) Close() error {
	if o == nil {
		return nil
	}
	if o.restoreCancel != nil {
		defer o.restoreCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return o.closeHandles(ctx)
}
func (o *lifecyclePreparationOwner) finishPreparation() (int64, error) {
	if o == nil || o.restoreContext == nil || o.restoreCancel == nil || o.combinedStarted.IsZero() {
		return 0, lifecycleError("lifecycle_restore_budget_lifetime_missing")
	}
	defer o.restoreCancel()
	if e := o.closeHandles(o.restoreContext); e != nil {
		return 0, e
	}
	if o.restoreContext.Err() != nil || time.Since(o.combinedStarted) > backup.MaxRestoreSeconds*time.Second {
		return 0, lifecycleError("lifecycle_combined_restore_budget_exceeded")
	}
	for _, engine := range o.engines {
		if e := engine.check(o.restoreContext, true, true); e != nil {
			return 0, e
		}
	}
	elapsed := time.Since(o.combinedStarted)
	if o.restoreContext.Err() != nil || elapsed > backup.MaxRestoreSeconds*time.Second {
		return 0, lifecycleError("lifecycle_combined_restore_budget_exceeded")
	}
	return elapsed.Milliseconds(), nil
}
func lifecycleInputFile(path string) (*os.File, error) {
	if privateDir(filepath.Dir(path)) != nil {
		return nil, lifecycleError("lifecycle_capture_private_source_rejected")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, lifecycleError("lifecycle_capture_private_source_rejected")
	}
	info, err := f.Stat()
	stat, ok := infoStat(info)
	if err != nil || info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ok || stat.Uid != 0 || stat.Nlink != 1 {
		_ = f.Close()
		return nil, lifecycleError("lifecycle_capture_private_source_rejected")
	}
	return f, nil
}
func infoStat(info os.FileInfo) (*syscall.Stat_t, bool) {
	if info == nil {
		return nil, false
	}
	v, ok := info.Sys().(*syscall.Stat_t)
	return v, ok
}
func captureLifecycleArchive(ctx context.Context, r lifecycleRequest) (archive *backup.Archive, result error) {
	owner, err := openLifecyclePreparationOwner(ctx, r)
	if err != nil {
		return nil, err
	}
	defer func() {
		if e := owner.Close(); result == nil && e != nil {
			result = e
		}
	}()
	var files []*os.File
	defer func() {
		for _, f := range files {
			if f.Close() != nil && result == nil {
				result = lifecycleError("lifecycle_capture_private_close_failed")
			}
		}
	}()
	names := []string{"inventory.private.json", "mysql-metadata.private.json", "mongodb-metadata.private.json", "mysql-domain_event_outbox.source.ndjson", "mysql-ai_bridge_commands.source.ndjson", "mysql-ai_messaging_legacy_commands.source.ndjson", "mongodb-domain_event_outbox.source.bsonframes"}
	for _, name := range names {
		f, e := lifecycleInputFile(filepath.Join(r.SourceDirectory, name))
		if e != nil {
			return nil, e
		}
		files = append(files, f)
	}
	in := backup.Inputs{Inventory: files[0], SQLMetadata: files[1], MongoMetadata: files[2]}
	for i := 0; i < 4; i++ {
		in.Sources[i] = io.Reader(files[3+i])
	}
	tx, err := owner.originalSQL.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, lifecycleError("lifecycle_capture_original_read_epoch_failed")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && e != sql.ErrTxDone && result == nil {
			result = lifecycleError("lifecycle_capture_original_epoch_close_failed")
		}
	}()
	// Capture authenticates every source to actual EOF and independently reads
	// all original rows/schema/identity/head twice in this host-owned read epoch.
	return backup.Capture(ctx, backup.BorrowedSources{SQL: tx, Mongo: owner.originalDB}, r.Approval, in, r.ArchiveDirectory)
}

func prepareLifecycleNative(ctx context.Context, r lifecycleRequest, archive *backup.Archive) (prepared *lifecyclePreparation, owner *lifecyclePreparationOwner, result error) {
	if archive == nil {
		return nil, nil, lifecycleError("lifecycle_archive_missing")
	}
	if !r.RestoreEngines.valid() {
		return nil, nil, lifecycleError("lifecycle_restore_engine_approval_missing")
	}
	owner, result = openLifecyclePreparationOwner(ctx, r)
	if result != nil {
		return nil, nil, result
	}
	defer func() {
		if result != nil {
			_ = owner.Close()
			owner = nil
		}
	}()
	// Restore namespace names are batch/run-specific and exact; the credentials
	// cannot select the original namespace. Real native producers also compare
	// original/restored database identity, server/process and actual contents.
	namespace := "qs_retirement_restore_" + digestRaw([]byte(r.OriginalSourceSHA + "\n" + r.OperationID + "\n" + r.ActualRunID + "\n" + r.ManifestSHA256))[:24]
	tx, err := owner.originalSQL.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, owner, lifecycleError("lifecycle_original_read_epoch_failed")
	}
	var originalUUID string
	readErr := backup.VerifyHostOriginalSources(ctx, archive, backup.BorrowedSources{SQL: tx, Mongo: owner.originalDB})
	if readErr == nil && tx.QueryRowContext(ctx, "SELECT @@server_uuid").Scan(&originalUUID) != nil {
		readErr = lifecycleError("lifecycle_original_identity_read_failed")
	}
	rollbackErr := tx.Rollback()
	if readErr != nil {
		return nil, owner, readErr
	}
	if rollbackErr != nil {
		return nil, owner, lifecycleError("lifecycle_original_epoch_close_failed")
	}
	// No original transaction remains before taking the max-one pool's
	// dedicated autocommit DDL connection. Never hold Tx and Conn together.
	owner.originalConn, err = owner.originalSQL.Conn(ctx)
	if err != nil {
		return nil, owner, lifecycleError("lifecycle_original_dedicated_connection_failed")
	}
	combinedStarted := time.Now()
	restoreCtx, cancel := context.WithTimeout(ctx, backup.MaxRestoreSeconds*time.Second)
	owner.restoreContext, owner.restoreCancel, owner.combinedStarted = restoreCtx, cancel, combinedStarted
	engine, err := startLifecycleOwnedEngine(restoreCtx, r, "mysql", r.RestoreEngines.MySQLImageID, namespace)
	if err == nil {
		owner.engines = append(owner.engines, engine)
		for name, hash := range engine.materialRecords {
			owner.materialRecords[name] = hash
		}
		owner.restoreSQL, err = lifecycleOwnedSQLPool(restoreCtx, engine)
	}
	if err != nil {
		return nil, owner, err
	}
	owner.restoreConn, err = owner.restoreSQL.Conn(restoreCtx)
	if err != nil {
		return nil, owner, lifecycleError("lifecycle_restore_dedicated_connection_failed")
	}
	var restoreUUID string
	if owner.restoreConn.QueryRowContext(restoreCtx, "SELECT @@server_uuid").Scan(&restoreUUID) != nil || originalUUID == "" || restoreUUID == "" || originalUUID == restoreUUID {
		return nil, owner, lifecycleError("lifecycle_restore_distinct_server_unproven")
	}
	engine, err = startLifecycleOwnedEngine(restoreCtx, r, "mongodb", r.RestoreEngines.MongoImageID, namespace)
	if err == nil {
		owner.engines = append(owner.engines, engine)
		for name, hash := range engine.materialRecords {
			owner.materialRecords[name] = hash
		}
		owner.restoreMongo, owner.restoreDB, err = lifecycleOwnedMongo(restoreCtx, engine)
	}
	if err != nil {
		return nil, owner, err
	}
	var originalHello, restoreHello bson.Raw
	hello := bson.D{{Key: "hello", Value: 1}}
	if owner.originalDB.Client().Database("admin").RunCommand(restoreCtx, hello).Decode(&originalHello) != nil || owner.restoreDB.Client().Database("admin").RunCommand(restoreCtx, hello).Decode(&restoreHello) != nil {
		return nil, owner, lifecycleError("lifecycle_restore_process_identity_failed")
	}
	originalProcess, originalOK := originalHello.Lookup("topologyVersion", "processId").ObjectIDOK()
	restoredProcess, restoredOK := restoreHello.Lookup("topologyVersion", "processId").ObjectIDOK()
	if !originalOK || !restoredOK || originalProcess.IsZero() || restoredProcess.IsZero() || originalProcess == restoredProcess {
		return nil, owner, lifecycleError("lifecycle_restore_distinct_process_unproven")
	}
	root := r.prepareRoot
	if root == "" {
		root = filepath.Join("/opt/backups/qs-server/compatibility-retirement", r.OperationID)
	}
	if privateDir(root) != nil {
		return nil, owner, lifecycleError("lifecycle_restore_registry_rejected")
	}
	// Record actual selected identities. Owned engines already have a durable
	// exact resource/namespace intent before their first engine/volume/DDL write.
	// Failed/partial restores remain registered; Close never
	// deletes a database or silently retries an existing run's copied bodies.
	registration := map[string]any{"format_version": 1, "kind": "temporary_isolated_restore_registration",
		"original_source_sha": r.OriginalSourceSHA, "tool_source_sha": r.ToolSourceSHA, "operation_id": r.OperationID,
		"original_run_id": r.Recovery.OriginalRunID, "actual_run_id": r.ActualRunID, "manifest_sha256": r.ManifestSHA256,
		"archive_sha256": archive.Summary().ArchiveSHA256, "namespace": namespace,
		"mysql_original_server_uuid_sha256": digestRaw([]byte(originalUUID)), "mysql_restore_server_uuid_sha256": digestRaw([]byte(restoreUUID)),
		"mongodb_original_process_sha256": digestRaw([]byte(originalProcess.Hex())), "mongodb_restore_process_sha256": digestRaw([]byte(restoredProcess.Hex())),
		"purge_after_acceptance_required": true, "drop_authority": false}
	if writeLifecycleMaterialJSON(filepath.Join(root, "lifecycle-restore-"+r.ActualRunID+".registration.private.json"), registration, owner.materialRecords) != nil {
		return nil, owner, lifecycleError("lifecycle_restore_registry_exists_or_unknown")
	}
	sqlProof, err := backup.RestoreSQL(restoreCtx, owner.restoreConn, archive)
	if err != nil {
		return nil, owner, err
	}
	mongoProof, err := backup.RestoreMongoWithOriginal(restoreCtx, owner.originalDB, owner.restoreDB, archive)
	if err != nil {
		return nil, owner, err
	}
	// Final original-source epoch is actual RO, host-owned, and within the same
	// original600-second context. Release max-one Conn before starting Tx.
	if owner.originalConn.Close() != nil {
		return nil, owner, lifecycleError("lifecycle_original_epoch_close_failed")
	}
	owner.originalConn = nil
	afterTx, afterErr := owner.originalSQL.BeginTx(restoreCtx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if afterErr != nil {
		return nil, owner, lifecycleError("lifecycle_original_read_epoch_failed")
	}
	afterErr = backup.VerifyHostOriginalSources(restoreCtx, archive, backup.BorrowedSources{SQL: afterTx, Mongo: owner.originalDB})
	afterRollback := afterTx.Rollback()
	if afterErr != nil {
		return nil, owner, afterErr
	}
	if afterRollback != nil {
		return nil, owner, lifecycleError("lifecycle_original_epoch_close_failed")
	}
	owner.originalConn, err = owner.originalSQL.Conn(restoreCtx)
	if err != nil {
		return nil, owner, lifecycleError("lifecycle_original_dedicated_connection_failed")
	}
	elapsed := time.Since(combinedStarted)
	if restoreCtx.Err() != nil || elapsed > backup.MaxRestoreSeconds*time.Second {
		return nil, owner, lifecycleError("lifecycle_combined_restore_budget_exceeded")
	}
	prepared = &lifecyclePreparation{combinedElapsedMillis: elapsed.Milliseconds(), Borrowed: backup.TargetRecoveryBorrowed{SQL: owner.originalConn, Mongo: owner.originalDB}, SQLRestore: sqlProof, MongoRestore: mongoProof}
	if !lifecyclePreparationMatches(archive, prepared) {
		return nil, owner, lifecycleError("lifecycle_actual_restore_proof_missing_or_budget_rejected")
	}
	return prepared, owner, nil
}
