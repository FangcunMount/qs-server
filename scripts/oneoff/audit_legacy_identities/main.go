// audit_legacy_identities inventories published assessment snapshots whose
// Kind/Algorithm are not canonical new-write identities (MC-R018 batch 1).
//
// Read-only. Exit 0 always when scan succeeds; use --json for machine counts.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

type config struct {
	mongoURI string
	mongoDB  string
	jsonOut  bool
	timeout  time.Duration
}

type bucket struct {
	Kind      string `json:"kind"`
	Algorithm string `json:"algorithm"`
	Policy    string `json:"policy"`
	Count     int    `json:"count"`
}

type report struct {
	PublishedCount int      `json:"published_count"`
	Canonical      int      `json:"canonical"`
	RetainedRead   int      `json:"retained_read"`
	DraftEmpty     int      `json:"draft_empty"`
	Unknown        int      `json:"unknown"`
	Buckets        []bucket `json:"buckets"`
}

type snapshotDoc struct {
	Kind      string `bson:"kind"`
	Algorithm string `bson:"algorithm"`
	Code      string `bson:"code"`
	Version   string `bson:"version"`
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "audit legacy identities failed: --mongo-uri is required (or set MONGO_URI)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit legacy identities failed: connect mongo:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "audit legacy identities failed: ping mongo:", err)
		os.Exit(1)
	}
	result, err := audit(ctx, client.Database(cfg.mongoDB))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit legacy identities failed:", err)
		os.Exit(1)
	}
	if cfg.jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(result)
		return
	}
	printReport(result)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs_server"), "MongoDB database")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit JSON report")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "operation timeout")
	flag.Parse()
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func audit(ctx context.Context, db *mongo.Database) (*report, error) {
	cur, err := db.Collection("assessment_model_snapshots").Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{
		"kind": 1, "algorithm": 1, "code": 1, "version": 1,
	}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	counts := map[string]*bucket{}
	out := &report{}
	for cur.Next(ctx) {
		var doc snapshotDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out.PublishedCount++
		kind := modelcatalog.Kind(doc.Kind)
		algorithm := modelcatalog.Algorithm(doc.Algorithm)
		policy := identity.ClassifyAlgorithmWritePolicy(kind, algorithm)
		switch policy {
		case identity.AlgorithmWriteCanonical:
			out.Canonical++
		case identity.AlgorithmWriteRetainedRead:
			out.RetainedRead++
		case identity.AlgorithmWriteDraftOK:
			out.DraftEmpty++
		default:
			out.Unknown++
		}
		key := string(kind) + "|" + string(algorithm) + "|" + string(policy)
		if b, ok := counts[key]; ok {
			b.Count++
			continue
		}
		counts[key] = &bucket{
			Kind: string(kind), Algorithm: string(algorithm), Policy: string(policy), Count: 1,
		}
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	out.Buckets = make([]bucket, 0, len(counts))
	for _, b := range counts {
		out.Buckets = append(out.Buckets, *b)
	}
	sort.Slice(out.Buckets, func(i, j int) bool {
		if out.Buckets[i].Policy != out.Buckets[j].Policy {
			return out.Buckets[i].Policy < out.Buckets[j].Policy
		}
		if out.Buckets[i].Kind != out.Buckets[j].Kind {
			return out.Buckets[i].Kind < out.Buckets[j].Kind
		}
		return out.Buckets[i].Algorithm < out.Buckets[j].Algorithm
	})
	return out, nil
}

func printReport(r *report) {
	fmt.Printf("published=%d canonical=%d retained_read=%d draft_empty=%d unknown=%d\n",
		r.PublishedCount, r.Canonical, r.RetainedRead, r.DraftEmpty, r.Unknown)
	for _, b := range r.Buckets {
		if b.Policy == string(identity.AlgorithmWriteCanonical) {
			continue
		}
		fmt.Printf("  %-12s %-28s %-14s count=%d\n", b.Kind, b.Algorithm, b.Policy, b.Count)
	}
}
