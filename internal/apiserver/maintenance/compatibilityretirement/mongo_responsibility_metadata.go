package retirement

import (
	"context"
	"encoding/hex"
	"sort"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoCycleCollections = []string{"answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts", "report_query_catalog", "rm_outbox", "qs_rm_replay_requests", "evaluation_acceptance_failure_claims", "interpretation_acceptance_failure_claims", "qrcode_acceptance_failure_claims", "interpretation_catalog_repair_plans"}

type mongoCycleMetadata struct {
	identity, hash string
	definitions    map[string]mongoCycleDefinition
	unknown        []string
}
type mongoCycleDefinition struct {
	uuid    string
	raw     bson.Raw
	indexes []bson.Raw
}

func observeMongoCycleMetadata(parent context.Context, db *mongo.Database, config MongoOwnerConfig) (mongoCycleMetadata, error) {
	result := mongoCycleMetadata{definitions: map[string]mongoCycleDefinition{}}
	owner, err := observeMongoOwnerMetadata(parent, db, config)
	if err != nil {
		return result, err
	}
	result.identity = owner.identity
	ctx, cancel := mongoMetadataContext(parent)
	defer cancel()
	cur, err := db.ListCollections(ctx, bson.D{}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if err != nil {
		return result, ErrMongoCycleRead
	}
	var rows []bson.Raw
	if err = cur.All(ctx, &rows); err != nil {
		return result, ErrMongoCycleRead
	}
	if len(rows) > 128 {
		return result, ErrMongoCycleBounds
	}
	known := map[string]bool{"schema_migrations": true, "domain_event_outbox": true, "assessment_models": true, "assessment_norms": true, "questionnaires": true, "interpretation_report_templates": true, "system.profile": true}
	for _, name := range mongoCycleCollections {
		known[name] = true
	}
	for _, raw := range rows {
		if mongoUniqueBSON(raw, 0) != nil {
			return result, ErrMongoCycleSchema
		}
		var row struct {
			Name string `bson:"name"`
			Type string `bson:"type"`
			Info struct {
				UUID primitive.Binary `bson:"uuid"`
			} `bson:"info"`
		}
		if bson.Unmarshal(raw, &row) != nil || row.Name == "" || row.Type != "collection" || row.Info.UUID.Subtype != 4 || len(row.Info.UUID.Data) != 16 {
			return result, ErrMongoCycleSchema
		}
		if _, exists := result.definitions[row.Name]; exists {
			return result, ErrMongoCycleSchema
		}
		indexcur, e := db.Collection(row.Name).Indexes().List(ctx)
		if e != nil {
			return result, ErrMongoCycleRead
		}
		var indices []bson.Raw
		if e = indexcur.All(ctx, &indices); e != nil {
			return result, ErrMongoCycleRead
		}
		if len(indices) > 128 {
			return result, ErrMongoCycleBounds
		}
		names := map[string]bool{}
		idIndex := false
		for _, idx := range indices {
			if mongoUniqueBSON(idx, 0) != nil {
				return result, ErrMongoCycleSchema
			}
			f, e := exactBSONFields(idx)
			if e != nil || f["name"].Type != bson.TypeString || f["key"].Type != bson.TypeEmbeddedDocument {
				return result, ErrMongoCycleSchema
			}
			name := f["name"].StringValue()
			if names[name] {
				return result, ErrMongoCycleSchema
			}
			names[name] = true
			if name == "_id_" {
				key, e := exactBSONFields(f["key"].Document())
				n, ok := mongoExactInteger(key["_id"])
				if e != nil || len(key) != 1 || !ok || n != 1 {
					return result, ErrMongoCycleSchema
				}
				idIndex = true
				// The fixed BSON token protocol is binary/simple ordering. A
				// non-simple _id index cannot serve that range/sort efficiently;
				// real executionStats shows a full scan even with an _id_ hint.
				// Never silently substitute locale ordering or page full scans.
				for _, scanned := range mongoCycleCollections {
					if scanned == row.Name {
						if collation, ok := f["collation"]; ok {
							if collation.Type != bson.TypeEmbeddedDocument || collation.Document().Lookup("locale").Type != bson.TypeString || collation.Document().Lookup("locale").StringValue() != "simple" {
								return result, ErrMongoCycleSchema
							}
						}
					}
				}
			}
		}
		// A capped profiling collection has no _id index and is a metadata-only
		// nonbusiness observation. Every scanned responsibility needs _id_.
		for _, name := range mongoCycleCollections {
			if name == row.Name && !idIndex {
				return result, ErrMongoCycleSchema
			}
		}
		sort.Slice(indices, func(i, j int) bool {
			return indices[i].Lookup("name").StringValue() < indices[j].Lookup("name").StringValue()
		})
		result.definitions[row.Name] = mongoCycleDefinition{uuid: hex.EncodeToString(row.Info.UUID.Data), raw: append(bson.Raw(nil), raw...), indexes: indices}
		if !known[row.Name] {
			result.unknown = append(result.unknown, row.Name)
		}
	}
	// Actual migration head is also read through the borrowed snapshot. A
	// stale head outside it cannot certify the transaction's selected schema.
	var snapshotHead bson.Raw
	if err = db.Collection("schema_migrations").FindOne(parent, bson.D{}, options.FindOne().SetCollation(&options.Collation{Locale: "simple"})).Decode(&snapshotHead); err != nil {
		return result, ErrMongoCycleRead
	}
	if string(snapshotHead) != string(owner.head) {
		return result, ErrMongoCycleConflict
	}
	names := make([]string, 0, len(result.definitions))
	for name := range result.definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := []string{"mongo-global-catalog/v1", result.identity, string(snapshotHead)}
	for _, name := range names {
		d := result.definitions[name]
		parts = append(parts, name, string(d.raw))
		for _, idx := range d.indexes {
			parts = append(parts, string(idx))
		}
	}
	result.hash = mongoOwnerHashParts(parts...)
	sort.Strings(result.unknown)
	return result, nil
}
