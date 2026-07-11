// dedupe_interpret_reports_domain finds and soft-deletes duplicate active
// interpret_reports rows that block unique index uk_report_domain_deleted
// on (domain_id, deleted_at).
//
// Default mode is dry-run (read-only). Pass --apply to soft-delete extras.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
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
	collectionName  = "interpret_reports"
	uniqueIndex     = "uk_report_domain_deleted"
	statusGenerated = "generated"
)

type config struct {
	mongoURI    string
	mongoDB     string
	timeout     time.Duration
	apply       bool
	ensureIndex bool
	jsonOut     bool
	limit       int
}

type reportDoc struct {
	ID          primitive.ObjectID `bson:"_id"`
	DomainID    int64              `bson:"domain_id"`
	Status      string             `bson:"status"`
	TesteeID    int64              `bson:"testee_id"`
	ScaleCode   string             `bson:"scale_code"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
	GeneratedAt *time.Time         `bson:"generated_at"`
	DeletedAt   *time.Time         `bson:"deleted_at"`
}

type duplicateGroup struct {
	DomainID   int64       `json:"domain_id"`
	Count      int         `json:"count"`
	Keep       reportDoc   `json:"keep"`
	SoftDelete []reportDoc `json:"soft_delete"`
}

type summary struct {
	Mode             string           `json:"mode"`
	DuplicateGroups  int              `json:"duplicate_groups"`
	DocsToSoftDelete int              `json:"docs_to_soft_delete"`
	SoftDeleted      int64            `json:"soft_deleted,omitempty"`
	IndexEnsured     bool             `json:"index_ensured,omitempty"`
	Groups           []duplicateGroup `json:"groups"`
}

func main() {
	cfg := parseFlags()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("ping mongo: %v", err)
	}

	coll := client.Database(cfg.mongoDB).Collection(collectionName)
	groups, err := findDuplicateActiveGroups(ctx, coll, cfg.limit)
	if err != nil {
		log.Fatalf("find duplicates: %v", err)
	}

	rep := summary{
		Mode:             "dry-run",
		DuplicateGroups:  len(groups),
		DocsToSoftDelete: countSoftDelete(groups),
		Groups:           groups,
	}

	if cfg.apply {
		rep.Mode = "apply"
		n, err := softDeleteExtras(ctx, coll, groups)
		if err != nil {
			log.Fatalf("soft-delete: %v", err)
		}
		rep.SoftDeleted = n

		// Re-scan to confirm active duplicates are gone.
		remaining, err := findDuplicateActiveGroups(ctx, coll, 0)
		if err != nil {
			log.Fatalf("re-scan after apply: %v", err)
		}
		if len(remaining) > 0 {
			log.Fatalf("still have %d active duplicate group(s) after apply; abort ensure-index", len(remaining))
		}
	}

	if cfg.ensureIndex {
		if !cfg.apply && len(groups) > 0 {
			log.Fatal("--ensure-index requires --apply when active duplicates still exist")
		}
		if err := ensureUniqueIndex(ctx, coll); err != nil {
			log.Fatalf("ensure index: %v", err)
		}
		rep.IndexEnsured = true
	}

	if cfg.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			log.Fatalf("encode json: %v", err)
		}
		return
	}
	printHuman(rep)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", envOr("MONGO_URI", ""), "MongoDB URI (or MONGO_URI)")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "overall script timeout")
	flag.BoolVar(&cfg.apply, "apply", false, "soft-delete duplicate active docs (default: dry-run)")
	flag.BoolVar(&cfg.ensureIndex, "ensure-index", false, "create uk_report_domain_deleted after cleanup")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit machine-readable JSON")
	flag.IntVar(&cfg.limit, "limit", 0, "max duplicate groups to print/process (0 = all)")
	flag.Parse()

	if strings.TrimSpace(cfg.mongoURI) == "" {
		log.Fatal("--mongo-uri or MONGO_URI is required")
	}
	if cfg.limit < 0 {
		log.Fatal("--limit must be >= 0")
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func findDuplicateActiveGroups(ctx context.Context, coll *mongo.Collection, limit int) ([]duplicateGroup, error) {
	// deleted_at:null matches both null and missing fields in MongoDB.
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"deleted_at": nil}}},
		{{Key: "$sort", Value: bson.D{
			{Key: "domain_id", Value: 1},
			{Key: "created_at", Value: -1},
			{Key: "_id", Value: -1},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$domain_id",
			"count": bson.M{"$sum": 1},
			"docs":  bson.M{"$push": "$$ROOT"},
		}}},
		{{Key: "$match", Value: bson.M{"count": bson.M{"$gt": 1}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
	}
	if limit > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: limit}})
	}

	cur, err := coll.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	type aggRow struct {
		DomainID int64       `bson:"_id"`
		Count    int         `bson:"count"`
		Docs     []reportDoc `bson:"docs"`
	}

	out := make([]duplicateGroup, 0)
	for cur.Next(ctx) {
		var row aggRow
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		keep, extras := chooseKeep(row.Docs)
		out = append(out, duplicateGroup{
			DomainID:   row.DomainID,
			Count:      row.Count,
			Keep:       keep,
			SoftDelete: extras,
		})
	}
	return out, cur.Err()
}

func chooseKeep(docs []reportDoc) (reportDoc, []reportDoc) {
	if len(docs) == 0 {
		return reportDoc{}, nil
	}
	sorted := append([]reportDoc(nil), docs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return preferDoc(sorted[i], sorted[j])
	})
	return sorted[0], sorted[1:]
}

// preferDoc reports whether a should be kept over b.
func preferDoc(a, b reportDoc) bool {
	aGen := a.Status == statusGenerated
	bGen := b.Status == statusGenerated
	if aGen != bGen {
		return aGen
	}
	if cmp := compareTimePtr(a.GeneratedAt, b.GeneratedAt); cmp != 0 {
		return cmp > 0
	}
	if !a.UpdatedAt.Equal(b.UpdatedAt) {
		return a.UpdatedAt.After(b.UpdatedAt)
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID.Hex() > b.ID.Hex()
}

func compareTimePtr(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	case a.After(*b):
		return 1
	case a.Before(*b):
		return -1
	default:
		return 0
	}
}

func softDeleteExtras(ctx context.Context, coll *mongo.Collection, groups []duplicateGroup) (int64, error) {
	var total int64
	base := time.Now().UTC()
	seq := 0
	for _, g := range groups {
		for _, doc := range g.SoftDelete {
			// Stagger deleted_at so (domain_id, deleted_at) stays unique.
			deletedAt := base.Add(time.Duration(seq) * time.Millisecond)
			seq++
			res, err := coll.UpdateOne(ctx,
				bson.M{"_id": doc.ID, "deleted_at": nil},
				bson.M{"$set": bson.M{
					"deleted_at": deletedAt,
					"updated_at": deletedAt,
				}},
			)
			if err != nil {
				return total, fmt.Errorf("soft-delete _id=%s domain_id=%d: %w", doc.ID.Hex(), g.DomainID, err)
			}
			total += res.ModifiedCount
		}
	}
	return total, nil
}

func ensureUniqueIndex(ctx context.Context, coll *mongo.Collection) error {
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "domain_id", Value: 1},
			{Key: "deleted_at", Value: 1},
		},
		Options: options.Index().SetName(uniqueIndex).SetUnique(true),
	})
	return err
}

func countSoftDelete(groups []duplicateGroup) int {
	n := 0
	for _, g := range groups {
		n += len(g.SoftDelete)
	}
	return n
}

func printHuman(rep summary) {
	fmt.Printf("mode=%s duplicate_groups=%d docs_to_soft_delete=%d\n",
		rep.Mode, rep.DuplicateGroups, rep.DocsToSoftDelete)
	if rep.Mode == "apply" {
		fmt.Printf("soft_deleted=%d index_ensured=%v\n", rep.SoftDeleted, rep.IndexEnsured)
	}
	for _, g := range rep.Groups {
		fmt.Printf("\n--- domain_id=%d count=%d ---\n", g.DomainID, g.Count)
		fmt.Printf("KEEP  _id=%s status=%s testee_id=%d scale=%s created_at=%s updated_at=%s generated_at=%s\n",
			g.Keep.ID.Hex(), g.Keep.Status, g.Keep.TesteeID, g.Keep.ScaleCode,
			fmtTime(g.Keep.CreatedAt), fmtTime(g.Keep.UpdatedAt), fmtTimePtr(g.Keep.GeneratedAt))
		for _, d := range g.SoftDelete {
			fmt.Printf("DROP  _id=%s status=%s testee_id=%d scale=%s created_at=%s updated_at=%s generated_at=%s\n",
				d.ID.Hex(), d.Status, d.TesteeID, d.ScaleCode,
				fmtTime(d.CreatedAt), fmtTime(d.UpdatedAt), fmtTimePtr(d.GeneratedAt))
		}
	}
	if rep.DuplicateGroups == 0 {
		fmt.Println("\nno active duplicates; safe to create uk_report_domain_deleted")
	} else if rep.Mode == "dry-run" {
		fmt.Println("\ndry-run only; re-run with --apply to soft-delete DROP rows")
	}
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return fmtTime(*t)
}
