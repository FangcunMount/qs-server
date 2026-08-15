package migration

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	scaleSnapshotMergeMigrationVersion = 6
	scaleSnapshotMergeIndexName        = "idx_scales_snapshot_merge_migration"
	compatibilityRetirementVersion     = 22
	runtimeLedgerRetirementVersion     = 23
)

var compatibilityRetirementCollections = []string{
	"answersheet_submit_idempotency",
	"archived_reports",
}

var runtimeLedgerRetirementCollections = []string{
	"interpretation_admission_failures",
	"interpretation_attention_projections",
	"interpretation_catalog_audit_checkpoints",
}

// MongoDriver implements the Driver interface for MongoDB databases.
type MongoDriver struct {
	embeddedMigrationDriver
	client *mongo.Client
}

// NewMongoDriver creates a new MongoDB migration driver.
func NewMongoDriver(client *mongo.Client) *MongoDriver {
	return &MongoDriver{
		embeddedMigrationDriver: newEmbeddedMigrationDriver(BackendMongo, "migrations/mongodb", "mongodb"),
		client:                  client,
	}
}

// CreateInstance creates a migrate.Migrate instance for MongoDB.
func (d *MongoDriver) CreateInstance(fs embed.FS, config *Config) (*migrate.Migrate, error) {
	if d.client == nil {
		return nil, fmt.Errorf("mongodb: client is nil")
	}

	// Create MongoDB database driver
	databaseDriver, err := mongodb.WithInstance(d.client, &mongodb.Config{
		DatabaseName:         config.Database,
		MigrationsCollection: config.MigrationsCollection,
	})
	if err != nil {
		return nil, fmt.Errorf("mongodb: failed to create database driver: %w", err)
	}

	return d.createInstance(fs, databaseDriver)
}

// PrepareRun enforces data-dependent migration preconditions and installs the
// non-partial unique index required by MongoDB's $merge in migration 6. The
// migration's durable partial index correctly represents the final schema, but
// MongoDB does not accept a partial index as the uniqueness proof for $merge.
// Keeping this index process-scoped avoids rewriting an already released
// migration and leaves no schema artifact.
func (d *MongoDriver) PrepareRun(parent context.Context, config *Config, versionBefore uint) (func(context.Context) error, error) {
	if d == nil || d.client == nil || config == nil {
		return func(context.Context) error { return nil }, nil
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if versionBefore < compatibilityRetirementVersion {
		if err := d.ensureCompatibilityRetirementCollections(ctx, config.Database); err != nil {
			return nil, err
		}
		if err := d.verifyCompatibilityRetirement(ctx, config.Database); err != nil {
			return nil, err
		}
	}
	if versionBefore < runtimeLedgerRetirementVersion {
		// Fresh databases create these collections in versions 14, 16 and 21
		// before version 23 drops them. A production database at version 22 may
		// already have dropped them during the bounded cutover, so recreate only
		// empty namespaces to keep the final drop migration idempotent.
		if versionBefore >= compatibilityRetirementVersion {
			if err := d.ensureMongoCollections(ctx, config.Database, runtimeLedgerRetirementCollections); err != nil {
				return nil, err
			}
		}
		if err := d.verifyEmptyMongoCollections(ctx, config.Database, runtimeLedgerRetirementCollections, "runtime ledger"); err != nil {
			return nil, err
		}
	}
	if versionBefore >= scaleSnapshotMergeMigrationVersion {
		return func(context.Context) error { return nil }, nil
	}
	collection := d.client.Database(config.Database).Collection("scales")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}, {Key: "scale_version", Value: 1}, {Key: "record_role", Value: 1}},
		Options: options.Index().SetName(scaleSnapshotMergeIndexName).SetUnique(true),
	})
	if err != nil {
		return nil, fmt.Errorf("create temporary scales snapshot merge index: %w", err)
	}
	return func(parent context.Context) error {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		_, err := collection.Indexes().DropOne(ctx, scaleSnapshotMergeIndexName)
		if isIgnorableMongoIndexCleanupError(err) {
			return nil
		}
		return err
	}, nil
}

// ensureCompatibilityRetirementCollections makes the drop migration safe on a
// brand-new database. MongoDB returns NamespaceNotFound when a drop command
// targets a collection that has never existed.
func (d *MongoDriver) ensureCompatibilityRetirementCollections(ctx context.Context, databaseName string) error {
	return d.ensureMongoCollections(ctx, databaseName, compatibilityRetirementCollections)
}

func (d *MongoDriver) ensureMongoCollections(ctx context.Context, databaseName string, collections []string) error {
	db := d.client.Database(databaseName)
	for _, name := range collections {
		if err := db.CreateCollection(ctx, name); err != nil && !isMongoNamespaceExists(err) {
			return fmt.Errorf("prepare Mongo retirement collection %s: %w", name, err)
		}
	}
	return nil
}

func (d *MongoDriver) verifyCompatibilityRetirement(ctx context.Context, databaseName string) error {
	if err := d.verifyEmptyMongoCollections(ctx, databaseName, compatibilityRetirementCollections, "compatibility"); err != nil {
		return err
	}
	db := d.client.Database(databaseName)
	archiveRefs, err := db.Collection("report_query_catalog").CountDocuments(ctx, bson.M{"source_kind": "archive"})
	if err != nil {
		return fmt.Errorf("verify Mongo retirement precondition report_query_catalog archive references: %w", err)
	}
	if archiveRefs != 0 {
		return fmt.Errorf("mongo retirement precondition failed: report_query_catalog contains %d archive references", archiveRefs)
	}
	return nil
}

func (d *MongoDriver) verifyEmptyMongoCollections(ctx context.Context, databaseName string, collections []string, kind string) error {
	db := d.client.Database(databaseName)
	for _, name := range collections {
		count, err := db.Collection(name).CountDocuments(ctx, bson.M{})
		if err != nil {
			return fmt.Errorf("verify Mongo retirement precondition %s: %w", name, err)
		}
		if count != 0 {
			return fmt.Errorf("mongo %s retirement precondition failed: %s contains %d documents", kind, name, count)
		}
	}
	return nil
}

func isMongoNamespaceExists(err error) bool {
	var commandErr mongo.CommandError
	return errors.As(err, &commandErr) && commandErr.Code == 48
}

func isIgnorableMongoIndexCleanupError(err error) bool {
	if err == nil {
		return true
	}
	var commandErr mongo.CommandError
	if !errors.As(err, &commandErr) {
		return false
	}
	return commandErr.Code == 26 || commandErr.Code == 27
}
