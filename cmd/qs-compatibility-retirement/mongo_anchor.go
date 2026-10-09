package main

import (
	"context"
	"encoding/hex"
	"errors"
	"os"

	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// mongoDatabaseAnchor identifies the replica set and selected namespace without
// depending on a primary election or the migration driver's Drop/Insert of its
// schema_migrations collection. A separate generation hash records that UUID;
// the full catalog and frozen business baseline still bind database contents.
// See MongoDB replica-configuration settings.replicaSetId, immutable since
// rs.initiate. Permission/network/standalone errors never fall back to a name.
func mongoDatabaseAnchor(ctx context.Context, db *mongo.Database, hello bson.Raw) (string, error) {
	var config bson.Raw
	q, cancel := queryContext(ctx)
	defer cancel()
	if err := db.Client().Database("admin").RunCommand(q, bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&config); err != nil {
		return "", mongoAnchorReadFailure(err)
	}
	return mongoAnchorFromMetadata(hello, config, db.Name())
}

// Only actual server codes establish a specific diagnosis. Error text is
// private and is never used as a permission/topology signal or forwarded.
// MongoDB documents 13 as Unauthorized and 76 as NoReplicationEnabled:
// https://www.mongodb.com/docs/manual/reference/error-codes/
func mongoAnchorReadFailure(err error) error {
	var command mongo.CommandError
	var pointer *mongo.CommandError
	var code int32
	if errors.As(err, &pointer) {
		// A typed nil pointer satisfies error but cannot be unwrapped as a
		// value CommandError. Keep it unknown without traversing it again.
		if pointer != nil {
			code = pointer.Code
		}
	} else if errors.As(err, &command) {
		code = command.Code
	}
	switch code {
	case 13:
		return category("mongo_replica_anchor_not_authorized")
	case 76:
		return category("mongo_replica_anchor_replication_not_enabled")
	default:
		return category("mongo_replica_anchor_permission_or_read_failed")
	}
}

func uniqueMongoMetadata(raw bson.Raw) bool {
	if raw.Validate() != nil {
		return false
	}
	elements, err := raw.Elements()
	if err != nil {
		return false
	}
	seen := make(map[string]bool, len(elements))
	for _, element := range elements {
		key := element.Key()
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func mongoAnchorFromMetadata(hello, response bson.Raw, selected string) (string, error) {
	if len(hello) > 64*1024 || len(response) > 512*1024 || !uniqueMongoMetadata(hello) || !uniqueMongoMetadata(response) ||
		selected == "" || len(selected) > 128 || !utf8.ValidString(selected) {
		return "", category("mongo_replica_anchor_metadata_rejected")
	}
	setName, ok := hello.Lookup("setName").StringValueOK()
	if !ok || setName == "" || len(setName) > 128 || !utf8.ValidString(setName) || hello.Lookup("msg").Type != 0 {
		return "", category("mongo_replica_anchor_topology_rejected")
	}
	config, ok := response.Lookup("config").DocumentOK()
	if !ok || !uniqueMongoMetadata(config) {
		return "", category("mongo_replica_anchor_metadata_rejected")
	}
	configuredName, ok := config.Lookup("_id").StringValueOK()
	if !ok || configuredName != setName {
		return "", category("mongo_replica_anchor_topology_rejected")
	}
	settings, ok := config.Lookup("settings").DocumentOK()
	if !ok || !uniqueMongoMetadata(settings) {
		return "", category("mongo_replica_anchor_metadata_rejected")
	}
	id, ok := settings.Lookup("replicaSetId").ObjectIDOK()
	if !ok || id.IsZero() {
		return "", category("mongo_replica_anchor_unavailable")
	}
	return hashParts("mongodb_database_anchor_v1", id.Hex(), setName, selected), nil
}

func mongoMigrationGeneration(collection bson.Raw) (string, error) {
	if !uniqueMongoMetadata(collection) {
		return "", category("mongo_migration_generation_rejected")
	}
	info, ok := collection.Lookup("info").DocumentOK()
	if !ok || !uniqueMongoMetadata(info) {
		return "", category("mongo_migration_generation_rejected")
	}
	subtype, value, ok := info.Lookup("uuid").BinaryOK()
	if !ok || subtype != 4 || len(value) != 16 {
		return "", category("mongo_migration_generation_rejected")
	}
	return hashParts("mongodb_migration_generation_v1", hex.EncodeToString(value)), nil
}

// Explicit namespace profile, not a permission-error fallback. The old replica
// anchor functions and their Unauthorized/standalone behavior are untouched.
func mongoNamespaceEndpoint(db *mongo.Database) (string, error) {
	port, err := envPort("MONGODB_PORT", 27017)
	if err != nil {
		return "", err
	}
	endpoint, err := identitymeta.MongoEndpointSHA256(os.Getenv("MONGODB_HOST"), port, db.Name())
	if err != nil {
		return "", category("mongo_namespace_anchor_rejected")
	}
	return endpoint, nil
}
func mongoNamespaceAnchor(ctx context.Context, db *mongo.Database) (*identitymeta.MongoNamespaceAnchor, error) {
	endpoint, err := mongoNamespaceEndpoint(db)
	if err != nil {
		return nil, err
	}
	observed, err := identitymeta.ObserveMongoNamespaceAnchor(ctx, db, endpoint)
	if err != nil {
		if errors.Is(err, identitymeta.ErrMongoNamespaceRead) {
			return nil, category("mongo_namespace_anchor_read_failed")
		}
		return nil, category("mongo_namespace_anchor_rejected")
	}
	return observed, nil
}
