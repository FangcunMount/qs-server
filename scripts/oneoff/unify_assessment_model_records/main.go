// unify_assessment_model_records migrates modelcatalog drafts and published
// runtime snapshots into the single assessment_models collection. It is an
// operator tool for a maintenance window; application startup must never run it.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	headCollection      = "assessment_models"
	publishedCollection = "published_assessment_models"
	defaultTemp         = "assessment_models_unified_staging"
	roleHead            = "head"
	roleSnapshot        = "published_snapshot"
)

type config struct {
	mongoURI, mongoDB, mode, temp, legacy string
	timeout                               time.Duration
}

type report struct {
	Heads, Snapshots, RetiredSnapshots, OrphanSnapshots int
	Issues                                              []string
	LegacyCollection                                    string
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fail("--mongo-uri is required (or set MONGO_URI)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fail("connect mongo: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fail("ping mongo: %v", err)
	}
	if err := run(ctx, client, client.Database(cfg.mongoDB), cfg, os.Stdout); err != nil {
		fail("unify assessment model records: %v", err)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.StringVar(&cfg.mode, "mode", "dry-run", "dry-run, apply, verify, cutover, or finalize")
	flag.StringVar(&cfg.temp, "temp-collection", defaultTemp, "temporary unified collection")
	flag.StringVar(&cfg.legacy, "legacy-collection", "", "legacy assessment_models collection name for finalize")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "overall timeout")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, client *mongo.Client, db *mongo.Database, cfg config, out *os.File) error {
	switch cfg.mode {
	case "dry-run":
		rep, err := audit(ctx, db, headCollection, publishedCollection)
		if err != nil {
			return err
		}
		printReport(out, rep)
		if len(rep.Issues) != 0 {
			return fmt.Errorf("audit found %d issue(s)", len(rep.Issues))
		}
		return nil
	case "apply":
		rep, err := audit(ctx, db, headCollection, publishedCollection)
		if err != nil {
			return err
		}
		if len(rep.Issues) != 0 {
			printReport(out, rep)
			return fmt.Errorf("refusing apply with %d issue(s)", len(rep.Issues))
		}
		if err := buildTemp(ctx, db, cfg.temp); err != nil {
			return err
		}
		if err := verifyTemp(ctx, db, cfg.temp, rep); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "prepared and verified %s\n", cfg.temp)
		return nil
	case "verify":
		rep, err := audit(ctx, db, headCollection, publishedCollection)
		if err != nil {
			return err
		}
		if err := verifyTemp(ctx, db, cfg.temp, rep); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "verified %s\n", cfg.temp)
		return nil
	case "cutover":
		legacy, err := cutover(ctx, client, db, cfg.temp)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "cutover complete; legacy collection: %s\n", legacy)
		return nil
	case "finalize":
		if cfg.legacy == "" {
			return fmt.Errorf("--legacy-collection is required for finalize")
		}
		if err := db.Collection(cfg.legacy).Drop(ctx); err != nil {
			return fmt.Errorf("drop %s: %w", cfg.legacy, err)
		}
		if err := db.Collection(publishedCollection).Drop(ctx); err != nil {
			return fmt.Errorf("drop %s: %w", publishedCollection, err)
		}
		_, _ = fmt.Fprintln(out, "finalize complete")
		return nil
	default:
		return fmt.Errorf("unsupported --mode %q", cfg.mode)
	}
}

func audit(ctx context.Context, db *mongo.Database, headsName, snapshotsName string) (report, error) {
	heads, err := loadAll(ctx, db.Collection(headsName), bson.M{"deleted_at": nil})
	if err != nil {
		return report{}, fmt.Errorf("load heads: %w", err)
	}
	snapshots, err := loadAll(ctx, db.Collection(snapshotsName), bson.M{})
	if err != nil {
		return report{}, fmt.Errorf("load snapshots: %w", err)
	}
	rep := report{Heads: len(heads), Snapshots: len(snapshots)}
	seenHeads, seenActive, seenRelease, seenActiveQuestionnaire := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, head := range heads {
		code := stringField(head, "code")
		if code == "" {
			rep.Issues = append(rep.Issues, "head without code")
			continue
		}
		if _, ok := seenHeads[code]; ok {
			rep.Issues = append(rep.Issues, "duplicate head "+code)
		}
		seenHeads[code] = struct{}{}
	}
	for _, row := range snapshots {
		code, kind, version := stringField(row, "model_code"), stringField(row, "model_kind"), stringField(row, "model_version")
		if code == "" || kind == "" || version == "" || len(bytesField(row, "payload")) == 0 {
			rep.Issues = append(rep.Issues, fmt.Sprintf("invalid published snapshot code=%q kind=%q version=%q", code, kind, version))
			continue
		}
		if stringField(row, "payload_format") == "" || stringField(row, "decision_kind") == "" || row["definition_v2"] == nil {
			rep.Issues = append(rep.Issues, fmt.Sprintf("incomplete published snapshot %s@%s", code, version))
		}
		key := strings.Join([]string{kind, stringField(row, "model_sub_kind"), stringField(row, "model_algorithm"), code, version}, "|")
		if _, ok := seenRelease[key]; ok {
			rep.Issues = append(rep.Issues, "duplicate published release "+key)
		}
		seenRelease[key] = struct{}{}
		if row["deleted_at"] == nil && stringField(row, "status") == "published" {
			if _, ok := seenActive[code]; ok {
				rep.Issues = append(rep.Issues, "multiple active snapshots for "+code)
			}
			seenActive[code] = struct{}{}
			bindingKey := stringField(row, "questionnaire_code") + "@" + stringField(row, "questionnaire_version")
			if bindingKey == "@" {
				rep.Issues = append(rep.Issues, "active snapshot without questionnaire binding "+code)
			} else if _, ok := seenActiveQuestionnaire[bindingKey]; ok {
				rep.Issues = append(rep.Issues, "multiple active snapshots for questionnaire "+bindingKey)
			} else {
				seenActiveQuestionnaire[bindingKey] = struct{}{}
			}
		} else {
			rep.RetiredSnapshots++
		}
		if _, ok := seenHeads[code]; !ok {
			rep.OrphanSnapshots++
		}
	}
	sort.Strings(rep.Issues)
	return rep, nil
}

func buildTemp(ctx context.Context, db *mongo.Database, temp string) error {
	if exists, err := collectionExists(ctx, db, temp); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("temporary collection %s already exists", temp)
	}
	target := db.Collection(temp)
	heads, err := loadAll(ctx, db.Collection(headCollection), bson.M{"deleted_at": nil})
	if err != nil {
		return err
	}
	for _, row := range heads {
		row["record_role"] = roleHead
		row["is_active_published"] = false
		row["revision"] = row["version"]
		delete(row, "version")
		if _, err := target.InsertOne(ctx, row); err != nil {
			return fmt.Errorf("insert head %s: %w", stringField(row, "code"), err)
		}
	}
	snapshots, err := loadAll(ctx, db.Collection(publishedCollection), bson.M{})
	if err != nil {
		return err
	}
	for _, row := range snapshots {
		converted := convertSnapshot(row)
		if _, err := target.InsertOne(ctx, converted); err != nil {
			return fmt.Errorf("insert snapshot %s@%s: %w", stringField(converted, "code"), stringField(converted, "release_version"), err)
		}
	}
	return createIndexes(ctx, target)
}

func convertSnapshot(row bson.M) bson.M {
	converted := bson.M{}
	for key, value := range row {
		converted[key] = value
	}
	converted["legacy_source_id"] = legacySourceID(row["_id"])
	converted["_id"] = primitive.NewObjectID()
	converted["record_role"] = roleSnapshot
	active := row["deleted_at"] == nil && stringField(row, "status") == "published"
	converted["is_active_published"] = active
	converted["product_channel"] = row["model_product_channel"]
	converted["kind"] = row["model_kind"]
	converted["sub_kind"] = row["model_sub_kind"]
	converted["algorithm"] = row["model_algorithm"]
	converted["code"] = row["model_code"]
	converted["release_version"] = row["model_version"]
	if !active {
		// Historical soft-deleted rows remain queryable by their exact release
		// after cutover, but cannot be selected by any active runtime query.
		converted["legacy_deleted_at"] = row["deleted_at"]
		converted["retention_state"] = "legacy_soft_deleted"
		converted["deleted_at"] = nil
		converted["status"] = "published"
	}
	for _, key := range []string{"model_product_channel", "model_kind", "model_sub_kind", "model_algorithm", "model_code", "model_version"} {
		delete(converted, key)
	}
	return converted
}

func verifyTemp(ctx context.Context, db *mongo.Database, temp string, source report) error {
	rows, err := loadAll(ctx, db.Collection(temp), bson.M{})
	if err != nil {
		return err
	}
	var heads, snapshots, retired, orphaned int
	seenActive, seenQuestionnaire := map[string]struct{}{}, map[string]struct{}{}
	for _, row := range rows {
		switch stringField(row, "record_role") {
		case roleHead:
			heads++
		case roleSnapshot:
			snapshots++
			if len(bytesField(row, "payload")) == 0 || stringField(row, "release_version") == "" || row["definition_v2"] == nil {
				return fmt.Errorf("invalid converted snapshot %s", stringField(row, "code"))
			}
			if err := verifySnapshotSource(ctx, db, row); err != nil {
				return err
			}
			if active, _ := row["is_active_published"].(bool); active {
				code := stringField(row, "code")
				if _, ok := seenActive[code]; ok {
					return fmt.Errorf("multiple active converted snapshots for %s", code)
				}
				seenActive[code] = struct{}{}
				binding := stringField(row, "questionnaire_code") + "@" + stringField(row, "questionnaire_version")
				if _, ok := seenQuestionnaire[binding]; ok {
					return fmt.Errorf("multiple active converted snapshots for questionnaire %s", binding)
				}
				seenQuestionnaire[binding] = struct{}{}
			} else {
				retired++
			}
			if stringField(row, "legacy_source_id") != "" {
				// Orphan count is informational: retained snapshots can later rebuild
				// a missing head through RestoreDraftFromPublished.
				var head bson.M
				err := db.Collection(temp).FindOne(ctx, bson.M{"record_role": roleHead, "code": stringField(row, "code")}).Decode(&head)
				if err == mongo.ErrNoDocuments {
					orphaned++
				} else if err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unknown record_role %q", stringField(row, "record_role"))
		}
	}
	if heads != source.Heads || snapshots != source.Snapshots {
		return fmt.Errorf("temporary count mismatch heads=%d/%d snapshots=%d/%d", heads, source.Heads, snapshots, source.Snapshots)
	}
	if retired != source.RetiredSnapshots || orphaned != source.OrphanSnapshots {
		return fmt.Errorf("temporary retained/orphan mismatch retained=%d/%d orphaned=%d/%d", retired, source.RetiredSnapshots, orphaned, source.OrphanSnapshots)
	}
	return nil
}

func verifySnapshotSource(ctx context.Context, db *mongo.Database, row bson.M) error {
	id, ok := row["legacy_source_id"].(string)
	if !ok || id == "" {
		return fmt.Errorf("converted snapshot %s is missing legacy_source_id", stringField(row, "code"))
	}
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("converted snapshot %s has invalid legacy_source_id: %w", stringField(row, "code"), err)
	}
	var source bson.M
	if err := db.Collection(publishedCollection).FindOne(ctx, bson.M{"_id": objectID}).Decode(&source); err != nil {
		return fmt.Errorf("load legacy source %s: %w", id, err)
	}
	if payloadHash(bytesField(source, "payload")) != payloadHash(bytesField(row, "payload")) {
		return fmt.Errorf("payload hash mismatch for %s@%s", stringField(row, "code"), stringField(row, "release_version"))
	}
	if stringField(source, "model_code") != stringField(row, "code") || stringField(source, "model_version") != stringField(row, "release_version") || stringField(source, "questionnaire_code") != stringField(row, "questionnaire_code") || stringField(source, "questionnaire_version") != stringField(row, "questionnaire_version") {
		return fmt.Errorf("identity or questionnaire binding mismatch for %s", id)
	}
	return nil
}

func createIndexes(ctx context.Context, c *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_head_code").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleHead, "deleted_at": nil})},
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "sub_kind", Value: 1}, {Key: "algorithm", Value: 1}, {Key: "code", Value: 1}, {Key: "release_version", Value: 1}, {Key: "record_role", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_snapshot_identity_version").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleSnapshot, "deleted_at": nil})},
		{Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "is_active_published", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_active_code").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleSnapshot, "is_active_published": true, "deleted_at": nil})},
		{Keys: bson.D{{Key: "questionnaire_code", Value: 1}, {Key: "questionnaire_version", Value: 1}, {Key: "record_role", Value: 1}, {Key: "is_active_published", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_active_questionnaire").SetUnique(true).SetPartialFilterExpression(bson.M{"record_role": roleSnapshot, "is_active_published": true, "deleted_at": nil})},
		{Keys: bson.D{{Key: "record_role", Value: 1}, {Key: "is_active_published", Value: 1}, {Key: "status", Value: 1}, {Key: "kind", Value: 1}, {Key: "category", Value: 1}, {Key: "code", Value: 1}}, Options: options.Index().SetName("idx_assessment_models_active_catalog")},
	}
	_, err := c.Indexes().CreateMany(ctx, indexes)
	return err
}

func cutover(ctx context.Context, client *mongo.Client, db *mongo.Database, temp string) (string, error) {
	if ok, err := collectionExists(ctx, db, temp); err != nil || !ok {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("temporary collection %s does not exist", temp)
	}
	legacy := headCollection + "_legacy_" + time.Now().UTC().Format("20060102_150405")
	if err := renameCollection(ctx, client, db.Name(), headCollection, legacy); err != nil {
		return "", err
	}
	if err := renameCollection(ctx, client, db.Name(), temp, headCollection); err != nil {
		_ = renameCollection(ctx, client, db.Name(), legacy, headCollection)
		return "", err
	}
	return legacy, nil
}

func renameCollection(ctx context.Context, client *mongo.Client, dbName, from, to string) error {
	result := client.Database("admin").RunCommand(ctx, bson.D{{Key: "renameCollection", Value: dbName + "." + from}, {Key: "to", Value: dbName + "." + to}})
	return result.Err()
}

func collectionExists(ctx context.Context, db *mongo.Database, name string) (bool, error) {
	items, err := db.ListCollectionNames(ctx, bson.M{"name": name})
	return len(items) != 0, err
}

func loadAll(ctx context.Context, c *mongo.Collection, filter bson.M) ([]bson.M, error) {
	cursor, err := c.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	items := make([]bson.M, 0)
	for cursor.Next(ctx) {
		var row bson.M
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, cursor.Err()
}

func stringField(row bson.M, key string) string { value, _ := row[key].(string); return value }
func bytesField(row bson.M, key string) []byte  { value, _ := row[key].([]byte); return value }
func legacySourceID(value any) string {
	if id, ok := value.(primitive.ObjectID); ok {
		return id.Hex()
	}
	return fmt.Sprint(value)
}
func payloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func printReport(out *os.File, rep report) {
	_, _ = fmt.Fprintf(out, "heads=%d snapshots=%d retired_snapshots=%d orphan_snapshots=%d issues=%d\n", rep.Heads, rep.Snapshots, rep.RetiredSnapshots, rep.OrphanSnapshots, len(rep.Issues))
	for _, issue := range rep.Issues {
		_, _ = fmt.Fprintln(out, "-", issue)
	}
}
func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
