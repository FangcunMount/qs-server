package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	mysqlinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	admissionCollection  = "interpretation_admission_failures"
	checkpointCollection = "interpretation_catalog_audit_checkpoints"
	attentionCollection  = "interpretation_attention_projections"
)

type config struct {
	mysqlDSN               string
	mysqlDatabase          string
	mongoURI               string
	mongoDB                string
	timeout                time.Duration
	batchSize              int
	apply                  bool
	prepareSchema          bool
	confirmServicesStopped bool
	dropSource             bool
	confirmDropSource      bool
}

type migrationStats struct {
	Source  int64
	Before  int64
	Changed int64
	After   int64
}

type mongoAdmissionFailure struct {
	DomainID       uint64    `bson:"domain_id"`
	OutcomeID      uint64    `bson:"outcome_id"`
	OrgID          int64     `bson:"org_id,omitempty"`
	AssessmentID   uint64    `bson:"assessment_id,omitempty"`
	TesteeID       uint64    `bson:"testee_id,omitempty"`
	EventID        string    `bson:"event_id,omitempty"`
	TraceID        string    `bson:"trace_id,omitempty"`
	Kind           string    `bson:"kind"`
	Code           string    `bson:"code"`
	SafeMessage    string    `bson:"safe_message"`
	Retryable      bool      `bson:"retryable"`
	Fingerprint    string    `bson:"fingerprint"`
	GenerationID   uint64    `bson:"generation_id,omitempty"`
	OutcomeVersion string    `bson:"outcome_version,omitempty"`
	Attempt        uint      `bson:"attempt"`
	Decision       string    `bson:"decision"`
	FirstFailedAt  time.Time `bson:"first_failed_at"`
	LastFailedAt   time.Time `bson:"last_failed_at"`
	OccurredAt     time.Time `bson:"occurred_at"`
}

type mongoAttentionProjection struct {
	EventID      string                     `bson:"event_id"`
	ReportID     string                     `bson:"report_id"`
	AssessmentID string                     `bson:"assessment_id"`
	TesteeID     uint64                     `bson:"testee_id"`
	RiskLevel    string                     `bson:"risk_level"`
	MarkKeyFocus bool                       `bson:"mark_key_focus"`
	Status       attentionprojection.Status `bson:"status"`
	Attempt      int                        `bson:"attempt"`
	LastError    string                     `bson:"last_error,omitempty"`
	CreatedAt    time.Time                  `bson:"created_at"`
	UpdatedAt    time.Time                  `bson:"updated_at"`
}

type mysqlAttentionProjectionRow struct {
	EventID      string                     `gorm:"column:event_id;primaryKey;size:128"`
	ReportID     string                     `gorm:"column:report_id;size:64;not null"`
	AssessmentID string                     `gorm:"column:assessment_id;size:64;not null"`
	TesteeID     uint64                     `gorm:"column:testee_id;not null"`
	RiskLevel    string                     `gorm:"column:risk_level;size:32;not null"`
	MarkKeyFocus bool                       `gorm:"column:mark_key_focus;not null"`
	Status       attentionprojection.Status `gorm:"column:status;size:32;not null"`
	Attempt      int                        `gorm:"column:attempt;not null"`
	LastError    string                     `gorm:"column:last_error;type:text"`
	CreatedAt    time.Time                  `gorm:"column:created_at;not null"`
	UpdatedAt    time.Time                  `gorm:"column:updated_at;not null"`
}

func (mysqlAttentionProjectionRow) TableName() string {
	return "interpretation_attention_projection"
}

type mongoDriftCounts struct {
	Missing             int64 `bson:"missing"`
	Dangling            int64 `bson:"dangling"`
	AssociationMismatch int64 `bson:"association_mismatch"`
	WrongWinner         int64 `bson:"wrong_winner"`
}

type mongoCompletedAuditSnapshot struct {
	CycleID     string                      `bson:"cycle_id"`
	CompletedAt time.Time                   `bson:"completed_at"`
	Counts      mongoDriftCounts            `bson:"counts"`
	OrgCounts   map[string]mongoDriftCounts `bson:"org_counts"`
}

type mongoAuditCheckpoint struct {
	ID                       string                       `bson:"_id"`
	SchemaVersion            int                          `bson:"schema_version"`
	Revision                 int64                        `bson:"revision"`
	CycleID                  string                       `bson:"cycle_id"`
	Phase                    string                       `bson:"phase"`
	AfterAssessmentID        uint64                       `bson:"after_assessment_id"`
	SourceUpperAssessmentID  uint64                       `bson:"source_upper_assessment_id"`
	CatalogUpperAssessmentID uint64                       `bson:"catalog_upper_assessment_id"`
	WorkingCounts            mongoDriftCounts             `bson:"working_counts"`
	WorkingOrgCounts         map[string]mongoDriftCounts  `bson:"working_org_counts"`
	LastCompleted            *mongoCompletedAuditSnapshot `bson:"last_completed,omitempty"`
	NextCycleAt              time.Time                    `bson:"next_cycle_at,omitempty"`
	UpdatedAt                time.Time                    `bson:"updated_at"`
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	if err := run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}

func parseFlags() (config, error) {
	cfg := config{}
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", os.Getenv("QS_MYSQL_DSN"), "target MySQL DSN; defaults to QS_MYSQL_DSN")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("QS_MONGO_URI"), "source MongoDB URI; defaults to QS_MONGO_URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", firstNonEmpty(os.Getenv("QS_MONGO_DB"), os.Getenv("QS_APISERVER_MONGODB_DATABASE"), "qs"), "source MongoDB database")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "overall migration timeout")
	flag.IntVar(&cfg.batchSize, "batch-size", 500, "Mongo cursor batch size")
	flag.BoolVar(&cfg.apply, "apply", false, "write and verify MySQL rows; default is read-only preflight")
	flag.BoolVar(&cfg.prepareSchema, "prepare-schema", false, "run embedded MySQL migrations through 000068 before applying data")
	flag.BoolVar(&cfg.confirmServicesStopped, "confirm-services-stopped", false, "confirm apiserver and workers are stopped for the apply window")
	flag.BoolVar(&cfg.dropSource, "drop-source", false, "drop the three Mongo source collections after verified apply")
	flag.BoolVar(&cfg.confirmDropSource, "confirm-drop-source", false, "confirm permanent removal of the three verified Mongo source collections")
	flag.Parse()
	var err error
	if cfg.mysqlDSN == "" {
		cfg.mysqlDSN, err = componentMySQLDSNFromEnv()
		if err != nil {
			return config{}, err
		}
	}
	if cfg.mongoURI == "" {
		cfg.mongoURI, err = componentMongoURIFromEnv(cfg.mongoDB)
		if err != nil {
			return config{}, err
		}
	}
	if cfg.mysqlDSN == "" || cfg.mongoURI == "" || cfg.mongoDB == "" {
		return config{}, fmt.Errorf("--mysql-dsn, --mongo-uri and --mongo-db are required; component QS_APISERVER database variables are also accepted")
	}
	mysqlConfig, err := drivermysql.ParseDSN(cfg.mysqlDSN)
	if err != nil || mysqlConfig.DBName == "" {
		return config{}, fmt.Errorf("--mysql-dsn must contain a valid database name")
	}
	cfg.mysqlDatabase = mysqlConfig.DBName
	if cfg.batchSize <= 0 || cfg.batchSize > 5000 {
		return config{}, fmt.Errorf("--batch-size must be between 1 and 5000")
	}
	if cfg.apply && !cfg.confirmServicesStopped {
		return config{}, fmt.Errorf("--apply requires --confirm-services-stopped to prevent cross-store write races")
	}
	if cfg.prepareSchema && !cfg.apply {
		return config{}, fmt.Errorf("--prepare-schema requires --apply")
	}
	if cfg.dropSource && (!cfg.apply || !cfg.confirmDropSource) {
		return config{}, fmt.Errorf("--drop-source requires --apply and --confirm-drop-source")
	}
	if cfg.confirmDropSource && !cfg.dropSource {
		return config{}, fmt.Errorf("--confirm-drop-source requires --drop-source")
	}
	return cfg, nil
}

func componentMySQLDSNFromEnv() (string, error) {
	host := strings.TrimSpace(os.Getenv("QS_APISERVER_MYSQL_HOST"))
	username := os.Getenv("QS_APISERVER_MYSQL_USERNAME")
	password := os.Getenv("QS_APISERVER_MYSQL_PASSWORD")
	database := strings.TrimSpace(os.Getenv("QS_APISERVER_MYSQL_DATABASE"))
	if host == "" && username == "" && password == "" && database == "" {
		return "", nil
	}
	if host == "" || username == "" || password == "" || database == "" {
		return "", fmt.Errorf("QS_APISERVER_MYSQL_HOST/USERNAME/PASSWORD/DATABASE must be configured together")
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return "", fmt.Errorf("load MySQL timezone: %w", err)
	}
	cfg := drivermysql.NewConfig()
	cfg.User = username
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = host
	cfg.DBName = database
	cfg.ParseTime = true
	cfg.Loc = location
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	return cfg.FormatDSN(), nil
}

func componentMongoURIFromEnv(database string) (string, error) {
	host := strings.TrimSpace(os.Getenv("QS_APISERVER_MONGODB_HOST"))
	username := os.Getenv("QS_APISERVER_MONGODB_USERNAME")
	password := os.Getenv("QS_APISERVER_MONGODB_PASSWORD")
	if host == "" && username == "" && password == "" {
		return "", nil
	}
	if host == "" || username == "" || password == "" || database == "" {
		return "", fmt.Errorf("QS_APISERVER_MONGODB_HOST/USERNAME/PASSWORD/DATABASE must be configured together")
	}
	if strings.Contains(host, "://") {
		return "", fmt.Errorf("QS_APISERVER_MONGODB_HOST must be host:port, got a URI")
	}
	u := &url.URL{Scheme: "mongodb", Host: host, Path: "/" + database, User: url.UserPassword(username, password)}
	return u.String(), nil
}

func run(ctx context.Context, cfg config) error {
	mysqlDB, err := gorm.Open(gormmysql.Open(cfg.mysqlDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("connect mysql: %w", err)
	}
	sqlDB, err := mysqlDB.DB()
	if err != nil {
		return fmt.Errorf("open mysql pool: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}
	if cfg.prepareSchema {
		version, migrated, err := migration.NewMigrator(sqlDB, &migration.Config{Enabled: true, Database: cfg.mysqlDatabase}).Run()
		if err != nil {
			return fmt.Errorf("prepare mysql schema: %w", err)
		}
		if version < 68 {
			return fmt.Errorf("prepare mysql schema stopped at version %d, want at least 68", version)
		}
		log.Printf("mysql schema ready: version=%d migrated=%t", version, migrated)
	}

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		return fmt.Errorf("ping mongo: %w", err)
	}
	mongoDB := mongoClient.Database(cfg.mongoDB)

	stats, err := preflight(ctx, mongoDB, mysqlDB)
	if err != nil {
		return err
	}
	printStats("preflight", stats)
	if !cfg.apply {
		log.Print("dry-run complete; stop apiserver/workers, then rerun with --apply --confirm-services-stopped")
		return nil
	}

	admissionRepo := mysqlinterpretation.NewAdmissionFailureRepository(mysqlDB)
	checkpointRepo := mysqlinterpretation.NewCatalogAuditCheckpointRepository(mysqlDB)
	changed, err := migrateAdmissions(ctx, mongoDB, admissionRepo, cfg.batchSize)
	if err != nil {
		return err
	}
	setChanged(stats, admissionCollection, changed)
	changed, err = migrateCheckpoint(ctx, mongoDB, checkpointRepo)
	if err != nil {
		return err
	}
	setChanged(stats, checkpointCollection, changed)
	changed, err = migrateAttention(ctx, mongoDB, mysqlDB, cfg.batchSize)
	if err != nil {
		return err
	}
	setChanged(stats, attentionCollection, changed)

	for source, table := range targetTables() {
		count, countErr := mysqlRowCount(ctx, mysqlDB, table)
		if countErr != nil {
			return countErr
		}
		item := stats[source]
		item.After = count
		stats[source] = item
	}
	printStats("apply", stats)
	if cfg.dropSource {
		if err := dropSourceCollections(ctx, mongoDB); err != nil {
			return err
		}
		log.Print("verified Mongo source collections dropped")
		return nil
	}
	log.Print("interpretation runtime ledger migration verified; keep Mongo source collections for rollback until the observation window closes")
	return nil
}

func preflight(ctx context.Context, mongoDB *mongo.Database, mysqlDB *gorm.DB) (map[string]migrationStats, error) {
	stats := make(map[string]migrationStats, 3)
	for source, table := range targetTables() {
		sourceCount, err := mongoDB.Collection(source).CountDocuments(ctx, bson.M{})
		if err != nil {
			return nil, fmt.Errorf("count mongo %s: %w", source, err)
		}
		targetCount, err := mysqlRowCount(ctx, mysqlDB, table)
		if err != nil {
			return nil, fmt.Errorf("count mysql %s (run migration 000068 first): %w", table, err)
		}
		stats[source] = migrationStats{Source: sourceCount, Before: targetCount, After: targetCount}
	}
	return stats, nil
}

func targetTables() map[string]string {
	return map[string]string{
		admissionCollection:  "interpretation_admission_failure",
		checkpointCollection: "interpretation_catalog_audit_checkpoint",
		attentionCollection:  "interpretation_attention_projection",
	}
}

func mysqlRowCount(ctx context.Context, db *gorm.DB, table string) (int64, error) {
	var count int64
	if err := db.WithContext(ctx).Table(table).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func migrateAdmissions(ctx context.Context, db *mongo.Database, repo *mysqlinterpretation.AdmissionFailureRepository, batchSize int) (int64, error) {
	cursor, err := db.Collection(admissionCollection).Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "domain_id", Value: 1}}).SetBatchSize(int32(batchSize)))
	if err != nil {
		return 0, fmt.Errorf("scan mongo admissions: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	var changed int64
	for cursor.Next(ctx) {
		var source mongoAdmissionFailure
		if err := cursor.Decode(&source); err != nil {
			return changed, fmt.Errorf("decode mongo admission: %w", err)
		}
		failure, err := source.domain()
		if err != nil {
			return changed, err
		}
		imported, err := repo.ImportFailure(ctx, failure)
		if err != nil {
			return changed, err
		}
		if imported {
			changed++
		}
		stored, err := repo.FindByFingerprint(ctx, failure.Fingerprint())
		if err != nil {
			return changed, fmt.Errorf("verify admission %s: %w", failure.Fingerprint(), err)
		}
		if stored.ID() != failure.ID() || stored.Attempt() < failure.Attempt() || stored.LastFailedAt().Before(failure.LastFailedAt()) {
			return changed, fmt.Errorf("verify admission %s: target evidence is older or mismatched", failure.Fingerprint())
		}
	}
	return changed, cursor.Err()
}

func (source mongoAdmissionFailure) domain() (*admission.Failure, error) {
	failure, err := admission.NewFailure(admission.Input{
		ID: meta.FromUint64(source.DomainID), OutcomeID: meta.FromUint64(source.OutcomeID), OrgID: source.OrgID,
		AssessmentID: meta.FromUint64(source.AssessmentID), TesteeID: source.TesteeID, EventID: source.EventID,
		TraceID: source.TraceID, Kind: admission.Kind(source.Kind), Code: source.Code, SafeMessage: source.SafeMessage,
		Retryable: source.Retryable, GenerationID: meta.FromUint64(source.GenerationID), OutcomeVersion: source.OutcomeVersion,
		Attempt: source.Attempt, Decision: source.Decision, FirstFailedAt: source.FirstFailedAt,
		LastFailedAt: source.LastFailedAt, OccurredAt: source.OccurredAt,
	})
	if err != nil {
		return nil, fmt.Errorf("restore admission %d: %w", source.DomainID, err)
	}
	if failure.Fingerprint() != source.Fingerprint {
		return nil, fmt.Errorf("admission %d fingerprint mismatch", source.DomainID)
	}
	return failure, nil
}

func migrateCheckpoint(ctx context.Context, db *mongo.Database, repo *mysqlinterpretation.CatalogAuditCheckpointRepository) (int64, error) {
	var source mongoAuditCheckpoint
	err := db.Collection(checkpointCollection).FindOne(ctx, bson.M{"_id": catalogreconcile.AuditCheckpointID}).Decode(&source)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load mongo catalog audit checkpoint: %w", err)
	}
	checkpoint, err := source.application()
	if err != nil {
		return 0, err
	}
	changed, err := repo.ImportAuditCheckpoint(ctx, checkpoint)
	if err != nil {
		return 0, err
	}
	stored, err := repo.LoadAuditCheckpoint(ctx)
	if err != nil {
		return 0, fmt.Errorf("verify catalog audit checkpoint: %w", err)
	}
	if stored.UpdatedAt.Before(checkpoint.UpdatedAt) {
		return 0, fmt.Errorf("verify catalog audit checkpoint: target updated_at %s is behind source %s", stored.UpdatedAt, checkpoint.UpdatedAt)
	}
	if checkpoint.LastCompleted != nil && (stored.LastCompleted == nil || stored.LastCompleted.CompletedAt.Before(checkpoint.LastCompleted.CompletedAt)) {
		return 0, fmt.Errorf("verify catalog audit checkpoint: target completed snapshot is behind source")
	}
	if changed {
		return 1, nil
	}
	return 0, nil
}

func (source mongoAuditCheckpoint) application() (catalogreconcile.AuditCheckpoint, error) {
	working, err := convertOrgCounts(source.WorkingOrgCounts)
	if err != nil {
		return catalogreconcile.AuditCheckpoint{}, err
	}
	checkpoint := catalogreconcile.AuditCheckpoint{
		SchemaVersion: source.SchemaVersion, Revision: source.Revision, CycleID: source.CycleID, Phase: source.Phase,
		AfterAssessmentID: source.AfterAssessmentID, SourceUpperAssessmentID: source.SourceUpperAssessmentID,
		CatalogUpperAssessmentID: source.CatalogUpperAssessmentID, WorkingCounts: source.WorkingCounts.application(),
		WorkingOrgCounts: working, NextCycleAt: source.NextCycleAt, UpdatedAt: source.UpdatedAt,
	}
	if source.LastCompleted != nil {
		orgCounts, err := convertOrgCounts(source.LastCompleted.OrgCounts)
		if err != nil {
			return catalogreconcile.AuditCheckpoint{}, err
		}
		checkpoint.LastCompleted = &catalogreconcile.CompletedAuditSnapshot{
			CycleID: source.LastCompleted.CycleID, CompletedAt: source.LastCompleted.CompletedAt,
			Counts: source.LastCompleted.Counts.application(), OrgCounts: orgCounts,
		}
	}
	return checkpoint, nil
}

func convertOrgCounts(source map[string]mongoDriftCounts) (map[int64]catalogreconcile.DriftCounts, error) {
	result := make(map[int64]catalogreconcile.DriftCounts, len(source))
	for rawID, counts := range source {
		orgID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid catalog audit organization id %q", rawID)
		}
		result[orgID] = counts.application()
	}
	return result, nil
}

func (counts mongoDriftCounts) application() catalogreconcile.DriftCounts {
	return catalogreconcile.DriftCounts{
		Missing: counts.Missing, Dangling: counts.Dangling,
		AssociationMismatch: counts.AssociationMismatch, WrongWinner: counts.WrongWinner,
	}
}

func migrateAttention(ctx context.Context, db *mongo.Database, mysqlDB *gorm.DB, batchSize int) (int64, error) {
	cursor, err := db.Collection(attentionCollection).Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "event_id", Value: 1}}).SetBatchSize(int32(batchSize)))
	if err != nil {
		return 0, fmt.Errorf("scan mongo attention projections: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	var changed int64
	batch := make([]mysqlAttentionProjectionRow, 0, batchSize)
	for cursor.Next(ctx) {
		var source mongoAttentionProjection
		if err := cursor.Decode(&source); err != nil {
			return changed, fmt.Errorf("decode mongo attention projection: %w", err)
		}
		if source.EventID == "" || source.ReportID == "" {
			return changed, fmt.Errorf("invalid attention projection event=%q report=%q", source.EventID, source.ReportID)
		}
		batch = append(batch, mysqlAttentionProjectionRow{
			EventID: source.EventID, ReportID: source.ReportID, AssessmentID: source.AssessmentID,
			TesteeID: source.TesteeID, RiskLevel: source.RiskLevel, MarkKeyFocus: source.MarkKeyFocus,
			Status: source.Status, Attempt: source.Attempt, LastError: source.LastError,
			CreatedAt: source.CreatedAt.UTC(), UpdatedAt: source.UpdatedAt.UTC(),
		})
		if len(batch) == batchSize {
			imported, err := flushAttentionBatch(ctx, mysqlDB, batch)
			if err != nil {
				return changed, err
			}
			changed += imported
			batch = batch[:0]
		}
	}
	if err := cursor.Err(); err != nil {
		return changed, err
	}
	imported, err := flushAttentionBatch(ctx, mysqlDB, batch)
	return changed + imported, err
}

func flushAttentionBatch(ctx context.Context, db *gorm.DB, batch []mysqlAttentionProjectionRow) (int64, error) {
	if len(batch) == 0 {
		return 0, nil
	}
	result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&batch)
	if result.Error != nil {
		return 0, fmt.Errorf("import attention projection batch: %w", result.Error)
	}
	eventIDs := make([]string, 0, len(batch))
	for i := range batch {
		eventIDs = append(eventIDs, batch[i].EventID)
	}
	var stored []mysqlAttentionProjectionRow
	if err := db.WithContext(ctx).Select("event_id", "report_id", "updated_at").Where("event_id IN ?", eventIDs).Find(&stored).Error; err != nil {
		return result.RowsAffected, fmt.Errorf("verify attention projection batch: %w", err)
	}
	byEventID := make(map[string]mysqlAttentionProjectionRow, len(stored))
	for i := range stored {
		byEventID[stored[i].EventID] = stored[i]
	}
	for i := range batch {
		row, exists := byEventID[batch[i].EventID]
		if !exists || row.ReportID != batch[i].ReportID || row.UpdatedAt.Before(batch[i].UpdatedAt) {
			return result.RowsAffected, fmt.Errorf("verify attention projection %s: target state is missing, older or mismatched", batch[i].EventID)
		}
	}
	return result.RowsAffected, nil
}

func dropSourceCollections(ctx context.Context, db *mongo.Database) error {
	for _, name := range []string{admissionCollection, checkpointCollection, attentionCollection} {
		if err := db.Collection(name).Drop(ctx); err != nil && !isNamespaceNotFound(err) {
			return fmt.Errorf("drop Mongo source collection %s: %w", name, err)
		}
		log.Printf("dropped Mongo source collection=%s", name)
	}
	names, err := db.ListCollectionNames(ctx, bson.M{"name": bson.M{"$in": bson.A{admissionCollection, checkpointCollection, attentionCollection}}})
	if err != nil {
		return fmt.Errorf("verify dropped Mongo source collections: %w", err)
	}
	if len(names) != 0 {
		return fmt.Errorf("verify dropped Mongo source collections: still present=%v", names)
	}
	return nil
}

func isNamespaceNotFound(err error) bool {
	var commandError mongo.CommandError
	return errors.As(err, &commandError) && commandError.Code == 26
}

func printStats(phase string, stats map[string]migrationStats) {
	for _, source := range []string{admissionCollection, checkpointCollection, attentionCollection} {
		item := stats[source]
		log.Printf("phase=%s source=%s mongo=%d mysql_before=%d changed=%d mysql_after=%d", phase, source, item.Source, item.Before, item.Changed, item.After)
	}
}

func setChanged(stats map[string]migrationStats, source string, changed int64) {
	item := stats[source]
	item.Changed = changed
	stats[source] = item
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
