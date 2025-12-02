package migration

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mongodb"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoDriver implements the Driver interface for MongoDB databases.
type MongoDriver struct {
	client *mongo.Client
}

// NewMongoDriver creates a new MongoDB migration driver.
func NewMongoDriver(client *mongo.Client) *MongoDriver {
	return &MongoDriver{client: client}
}

// Backend returns the backend type.
func (d *MongoDriver) Backend() Backend {
	return BackendMongo
}

// SourcePath returns the path to MongoDB migration files.
func (d *MongoDriver) SourcePath() string {
	return "migrations/mongodb"
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

	// Create source driver from embedded filesystem
	sourceDriver, err := iofs.New(fs, d.SourcePath())
	if err != nil {
		return nil, fmt.Errorf("mongodb: failed to create source driver: %w", err)
	}

	// Create migrate instance
	instance, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		string(d.Backend()),
		databaseDriver,
	)
	if err != nil {
		return nil, fmt.Errorf("mongodb: failed to create migrate instance: %w", err)
	}

	return instance, nil
}
