package migration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// OfflineConfig borrows the host's two pools. SourceSHA must come from the
// compiled binary, not an operator-provided flag. This path is installed-only.
type OfflineConfig struct {
	Pair                PairConfig
	AuthorizationFile   string
	AuthorizationSHA256 string
}

type OfflineSnapshot struct {
	SourceSHA                string `json:"source_sha"`
	MigrationResourcesSHA256 string `json:"migration_resources_sha256"`
	MySQLDatabase            string `json:"mysql_database"`
	MongoDatabase            string `json:"mongo_database"`
	MySQLIdentitySHA256      string `json:"mysql_identity_sha256"`
	MongoClusterSHA256       string `json:"mongo_cluster_sha256"`
	MySQLVersion             uint   `json:"mysql_version"`
	MongoVersion             uint   `json:"mongo_version"`
	TargetMySQLVersion       uint   `json:"target_mysql_version"`
	TargetMongoVersion       uint   `json:"target_mongo_version"`
}

type OfflineResult struct {
	Initial                 OfflineSnapshot `json:"initial"`
	Stage                   string          `json:"stage"`
	OperationID             string          `json:"operation_id,omitempty"`
	AuthorizationConsumed   bool            `json:"authorization_consumed"`
	MySQLChanged            *bool           `json:"mysql_changed,omitempty"`
	MongoChanged            *bool           `json:"mongo_changed,omitempty"`
	MigrationOutcomeUnknown bool            `json:"migration_outcome_unknown"`
	Verified                bool            `json:"verified"`
}

// All evidence references are operator-approved claims, not automatically
// certified backups, runtime freezes, or permission to start a business host.
type offlineAuthorization struct {
	FormatVersion            int    `json:"format_version"`
	Kind                     string `json:"kind"`
	OperationID              string `json:"operation_id"`
	ExpiresAt                string `json:"expires_at"`
	ApprovedSourceSHA        string `json:"approved_source_sha"`
	MigrationResourcesSHA256 string `json:"migration_resources_sha256"`
	MySQLDatabase            string `json:"mysql_database"`
	MongoDatabase            string `json:"mongo_database"`
	MySQLIdentitySHA256      string `json:"mysql_identity_sha256"`
	MongoClusterSHA256       string `json:"mongo_cluster_sha256"`
	InitialMySQLVersion      uint   `json:"initial_mysql_version"`
	InitialMongoVersion      uint   `json:"initial_mongo_version"`
	TargetMySQLVersion       uint   `json:"target_mysql_version"`
	TargetMongoVersion       uint   `json:"target_mongo_version"`
	ApprovalSummarySHA256    string `json:"approval_summary_sha256"`
	FreezeScopeSHA256        string `json:"freeze_scope_sha256"`
	BackupPairManifestSHA256 string `json:"backup_pair_manifest_sha256"`
	RestoreReceiptSHA256     string `json:"restore_receipt_sha256"`
	StartupReceiptSHA256     string `json:"startup_receipt_sha256"`
	ResponsibilitySHA256     string `json:"responsibility_sha256"`
	BackupPairLocation       string `json:"backup_pair_location"`
	DataOwner                string `json:"data_owner"`
	RecoveryOwner            string `json:"recovery_owner"`
	RuntimeFrozen            bool   `json:"runtime_frozen"`
	RuntimeNotStarted        bool   `json:"runtime_not_started"`
	WindowStartedAt          string `json:"window_started_at"`
	RestoreDurationSeconds   int    `json:"restore_duration_seconds"`
	StartupReserveSeconds    int    `json:"startup_reserve_seconds"`
	BackupRetentionDays      int    `json:"backup_retention_days"`
}

var offlineSourceSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// InspectOffline reads only identities and guarded installed migration metadata.
// It does not read/consume an approval or initialize any application repository.
func InspectOffline(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig) (OfflineSnapshot, error) {
	r, err := compiledMigrationResources()
	if err != nil {
		return OfflineSnapshot{}, err
	}
	_, snapshot, err := inspectOffline(ctx, db, client, cfg, r)
	return snapshot, err
}

func inspectOffline(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig, r *migrationResources) (*PairPreflight, OfflineSnapshot, error) {
	if ctx == nil || ctx.Err() != nil || db == nil || client == nil || cfg.MySQLDatabase == "" || cfg.MongoDatabase == "" ||
		!offlineSourceSHA.MatchString(cfg.ExpectedSourceSHA) || cfg.BootstrapAuthorizationFile != "" || cfg.BootstrapAuthorizationSHA256 != "" {
		return nil, OfflineSnapshot{}, retirementError("offline compiled source and installed pair required")
	}
	// Reject pristine before preflight can even read a bootstrap authorization.
	sqlState, err := observeSQLPair(ctx, db, cfg.MySQLDatabase, r)
	if err != nil || sqlState.pristine || sqlState.version < compatibilitySQLVersion {
		return nil, OfflineSnapshot{}, retirementError("offline installed clean mysql required")
	}
	mongoState, err := observeMongoPair(ctx, client, cfg.MongoDatabase, r)
	if err != nil || mongoState.pristine || mongoState.version < compatibilityMongoVersion {
		return nil, OfflineSnapshot{}, retirementError("offline installed clean mongo required")
	}
	p, err := preflightCompatibilityPair(ctx, db, client, cfg, r)
	if err != nil {
		return nil, OfflineSnapshot{}, err
	}
	return p, OfflineSnapshot{
		SourceSHA: cfg.ExpectedSourceSHA, MigrationResourcesSHA256: r.sha256,
		MySQLDatabase: cfg.MySQLDatabase, MongoDatabase: cfg.MongoDatabase,
		MySQLIdentitySHA256: p.sqlHash, MongoClusterSHA256: p.mongoHash,
		MySQLVersion: p.initialSQLVersion, MongoVersion: p.initialMongoVersion,
		TargetMySQLVersion: r.sql.head, TargetMongoVersion: r.mongo.head,
	}, nil
}

func validOfflineText(s string, max int) bool {
	return s != "" && len(s) <= max && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n") &&
		!strings.EqualFold(s, "unknown") && !strings.EqualFold(s, "pending") && !strings.EqualFold(s, "todo")
}

func validateOfflineAuthorization(a offlineAuthorization, initial OfflineSnapshot, now time.Time) error {
	expires, expireErr := time.Parse(time.RFC3339, a.ExpiresAt)
	started, startErr := time.Parse(time.RFC3339, a.WindowStartedAt)
	if expireErr != nil || startErr != nil || !strings.HasSuffix(a.ExpiresAt, "Z") || !strings.HasSuffix(a.WindowStartedAt, "Z") ||
		!now.Before(expires) || expires.After(now.Add(24*time.Hour)) || started.After(now) ||
		a.StartupReserveSeconds < 30 || a.StartupReserveSeconds >= 1800 || !now.Before(started.Add(30*time.Minute-time.Duration(a.StartupReserveSeconds)*time.Second)) ||
		a.FormatVersion != 1 || a.Kind != "qs_schema_pair_offline_upgrade" || !retirementOpRE.MatchString(a.OperationID) ||
		a.ApprovedSourceSHA != initial.SourceSHA || a.MigrationResourcesSHA256 != initial.MigrationResourcesSHA256 ||
		a.MySQLDatabase != initial.MySQLDatabase || a.MongoDatabase != initial.MongoDatabase ||
		a.MySQLIdentitySHA256 != initial.MySQLIdentitySHA256 || a.MongoClusterSHA256 != initial.MongoClusterSHA256 ||
		a.InitialMySQLVersion != initial.MySQLVersion || a.InitialMongoVersion != initial.MongoVersion ||
		a.TargetMySQLVersion != initial.TargetMySQLVersion || a.TargetMongoVersion != initial.TargetMongoVersion ||
		!a.RuntimeFrozen || !a.RuntimeNotStarted || !validOfflineText(a.BackupPairLocation, 1024) ||
		!validOfflineText(a.DataOwner, 128) || !validOfflineText(a.RecoveryOwner, 128) ||
		a.RestoreDurationSeconds < 1 || a.RestoreDurationSeconds > 3600 || a.BackupRetentionDays < 30 {
		return retirementError("offline approval binding or recovery evidence missing")
	}
	for _, hash := range []string{a.ApprovalSummarySHA256, a.FreezeScopeSHA256, a.BackupPairManifestSHA256, a.RestoreReceiptSHA256, a.StartupReceiptSHA256, a.ResponsibilitySHA256} {
		if !retirementHashRE.MatchString(hash) || hash == strings.Repeat("0", 64) {
			return retirementError("offline evidence digest missing")
		}
	}
	return nil
}

func readOfflineAuthorization(cfg OfflineConfig, initial OfflineSnapshot) (offlineAuthorization, error) {
	raw, err := readPrivateRetirementAuthorization(cfg.AuthorizationFile, cfg.AuthorizationSHA256)
	if err != nil {
		return offlineAuthorization{}, err
	}
	if err := rejectRetirementDuplicateJSON(raw); err != nil {
		return offlineAuthorization{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var a offlineAuthorization
	if decoder.Decode(&a) != nil || decoder.Decode(new(any)) != io.EOF {
		return a, retirementError("offline approval schema rejected")
	}
	if err := validateOfflineAuthorization(a, initial, time.Now()); err != nil {
		return a, err
	}
	if _, err := os.Lstat(cfg.AuthorizationFile + ".consumed"); !errors.Is(err, os.ErrNotExist) {
		return a, retirementError("offline approval consumed or unknown")
	}
	return a, nil
}

// RunOffline executes only the two embedded migrators on borrowed connections.
// It cannot bootstrap a new database, force a version, recover dirty metadata,
// reconcile unrelated indexes, seed business data, or start business runtimes.
func RunOffline(ctx context.Context, db *sql.DB, client *mongo.Client, cfg OfflineConfig) (OfflineResult, error) {
	r, err := compiledMigrationResources()
	if err != nil {
		return OfflineResult{Stage: "resources"}, err
	}
	return runOffline(ctx, db, client, cfg, r)
}

func runOffline(ctx context.Context, db *sql.DB, client *mongo.Client, cfg OfflineConfig, r *migrationResources) (result OfflineResult, resultErr error) {
	defer func() {
		if resultErr != nil && result.AuthorizationConsumed {
			result.MigrationOutcomeUnknown = true
		}
	}()
	result.Stage = "preflight"
	_, initial, err := inspectOffline(ctx, db, client, cfg.Pair, r)
	if err != nil {
		return result, err
	}
	result.Initial, result.Stage = initial, "approval"
	a, err := readOfflineAuthorization(cfg, initial)
	if err != nil {
		return result, err
	}
	result.OperationID = a.OperationID
	// Bind again immediately before claiming; a replacement/drift cannot adopt an
	// earlier inspection. Claiming is durable and exclusive, even for a no-op run.
	p, fresh, err := inspectOffline(ctx, db, client, cfg.Pair, r)
	if err != nil || fresh != initial {
		return result, retirementError("offline initial pair changed")
	}
	observed, err := readOfflineAuthorization(cfg, initial)
	if err != nil || observed != a {
		return result, retirementError("offline approval changed")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.Stage = "claim"
	if err := claimPrivateRetirementAuthorization(cfg.AuthorizationFile, a.OperationID, cfg.AuthorizationSHA256); err != nil {
		return result, err
	}
	result.AuthorizationConsumed = true
	expires, _ := time.Parse(time.RFC3339, a.ExpiresAt)
	started, _ := time.Parse(time.RFC3339, a.WindowStartedAt)
	if windowEnd := started.Add(30*time.Minute - time.Duration(a.StartupReserveSeconds)*time.Second); windowEnd.Before(expires) {
		expires = windowEnd
	}
	ctx, cancel := context.WithDeadline(ctx, expires)
	defer cancel()
	result.Stage = "mysql"
	sqlConfig := p.MySQLConfig(false)
	sqlConfig.runContext = ctx
	sqlConfig.offline = true
	_, mysqlChanged, err := NewMigrator(db, sqlConfig).Run()
	if err != nil {
		return result, err
	}
	result.MySQLChanged = &mysqlChanged
	result.Stage = "mongodb"
	if err := validateOfflineAuthorization(a, initial, time.Now()); err != nil {
		return result, err
	}
	mongoConfig := p.MongoConfig(false)
	mongoConfig.runContext = ctx
	mongoConfig.offline = true
	_, mongoChanged, err := NewMongoMigrator(client, mongoConfig).Run()
	if err != nil {
		return result, err
	}
	result.MongoChanged = &mongoChanged
	result.Stage = "verify"
	if err := p.VerifyComplete(ctx); err != nil {
		return result, err
	}
	result.Verified, result.Stage = true, "complete"
	return result, nil
}

// Upstream drivers use Background/TODO for migration bodies. Only the explicit
// offline path executes the same embedded body on its borrowed run connection
// with the approved deadline. Metadata transitions still use the guarded driver.
func (d *retirementDatabaseDriver) runOfflineStep(raw []byte) error {
	if d.backend == BackendMySQL {
		if d.sqlConn == nil {
			return retirementError("offline sql run connection missing")
		}
		_, err := d.sqlConn.ExecContext(d.ctx, string(raw))
		return err
	}
	if d.client == nil {
		return retirementError("offline mongo run connection missing")
	}
	var commands []bson.D
	if err := bson.UnmarshalExtJSON(raw, true, &commands); err != nil {
		return err
	}
	for _, command := range commands {
		if err := d.client.Database(d.database).RunCommand(d.ctx, command).Err(); err != nil {
			return err
		}
	}
	return nil
}
