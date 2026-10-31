package migration

import (
	"context"
	"database/sql"

	"go.mongodb.org/mongo-driver/mongo"
)

// InspectPairIdentity borrows the canonical selected-primary identity readers
// without inspecting or changing migration state. Recovery/drain tools can
// therefore reject a wrong pair even while a schema transition is interrupted.
func InspectPairIdentity(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig) (sqlIdentity, mongoIdentity string, err error) {
	return pairIdentity(ctx, db, client, cfg.MySQLDatabase, cfg.MongoDatabase)
}
