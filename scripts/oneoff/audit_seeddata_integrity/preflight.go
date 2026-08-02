package main

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	minimumMySQLMigration = 63
	minimumMongoMigration = 19
)

type storageIdentity struct {
	MySQLDatabase   string `json:"mysql_database"`
	MySQLHost       string `json:"mysql_host"`
	MySQLPort       int    `json:"mysql_port"`
	MySQLMigration  int64  `json:"mysql_migration"`
	MongoDatabase   string `json:"mongo_database"`
	MongoReplicaSet string `json:"mongo_replica_set,omitempty"`
	MongoMembers    string `json:"mongo_members"`
	MongoMigration  int64  `json:"mongo_migration"`
}

func loadStorageIdentity(ctx context.Context, mysqlDB *sql.DB, mongoDB *mongo.Database) (storageIdentity, error) {
	var identity storageIdentity
	var mysqlDirty bool
	if err := mysqlDB.QueryRowContext(ctx, `SELECT DATABASE(),@@hostname,@@port,version,dirty FROM schema_migrations LIMIT 1`).Scan(
		&identity.MySQLDatabase, &identity.MySQLHost, &identity.MySQLPort, &identity.MySQLMigration, &mysqlDirty,
	); err != nil {
		return storageIdentity{}, fmt.Errorf("read MySQL storage identity: %w", err)
	}
	if identity.MySQLDatabase == "" || mysqlDirty || identity.MySQLMigration < minimumMySQLMigration {
		return storageIdentity{}, fmt.Errorf("MySQL schema is not ready: database=%q version=%d dirty=%t; require clean version >= %d", identity.MySQLDatabase, identity.MySQLMigration, mysqlDirty, minimumMySQLMigration)
	}

	var mongoMigration struct {
		Version int64 `bson:"version"`
		Dirty   bool  `bson:"dirty"`
	}
	if err := mongoDB.Collection("schema_migrations").FindOne(ctx, bson.M{}).Decode(&mongoMigration); err != nil {
		return storageIdentity{}, fmt.Errorf("read Mongo storage identity: %w", err)
	}
	if mongoMigration.Dirty || mongoMigration.Version < minimumMongoMigration {
		return storageIdentity{}, fmt.Errorf("mongo schema is not ready: version=%d dirty=%t; require clean version >= %d", mongoMigration.Version, mongoMigration.Dirty, minimumMongoMigration)
	}

	var hello struct {
		SetName string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
		Me      string   `bson:"me"`
	}
	if err := mongoDB.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return storageIdentity{}, fmt.Errorf("read Mongo deployment identity: %w", err)
	}
	members := append([]string(nil), hello.Hosts...)
	if len(members) == 0 && hello.Me != "" {
		members = append(members, hello.Me)
	}
	sort.Strings(members)
	identity.MongoDatabase = mongoDB.Name()
	identity.MongoReplicaSet = hello.SetName
	identity.MongoMembers = strings.Join(members, ",")
	identity.MongoMigration = mongoMigration.Version
	if identity.MongoDatabase == "" || identity.MongoMembers == "" {
		return storageIdentity{}, fmt.Errorf("mongo deployment identity is incomplete: database=%q members=%q", identity.MongoDatabase, identity.MongoMembers)
	}
	return identity, nil
}
