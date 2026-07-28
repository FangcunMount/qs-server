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
)

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

// PrepareRun installs the non-partial unique index required by MongoDB's
// $merge in migration 6. The migration's durable partial index correctly
// represents the final schema, but MongoDB does not accept a partial index as
// the uniqueness proof for $merge. Keeping this index process-scoped avoids
// rewriting an already released migration and leaves no schema artifact.
func (d *MongoDriver) PrepareRun(parent context.Context, config *Config, versionBefore uint) (func(context.Context) error, error) {
	if d == nil || d.client == nil || config == nil || versionBefore >= scaleSnapshotMergeMigrationVersion {
		return func(context.Context) error { return nil }, nil
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
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
		var commandErr mongo.CommandError
		if errors.As(err, &commandErr) && commandErr.Code == 27 {
			return nil
		}
		return err
	}, nil
}
