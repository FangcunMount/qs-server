package migration

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
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
