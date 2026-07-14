// migrate_personality_kind_values audits retired `personality` catalog values
// by default and normalizes verified typology rows when --apply is supplied.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/scripts/oneoff/internal/personalitykind"
)

type config struct {
	mongoURI string
	mongoDB  string
	apply    bool
	json     bool
	timeout  time.Duration
}

type report struct {
	Findings []personalitykind.Finding `json:"findings"`
	Eligible int                       `json:"eligible"`
	Skipped  int                       `json:"skipped"`
	Applied  int                       `json:"applied"`
}

func main() {
	cfg := parseFlags()
	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "migrate personality kind values failed:", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.BoolVar(&cfg.apply, "apply", false, "apply verified changes; default is audit only")
	flag.BoolVar(&cfg.json, "json", false, "emit JSON")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "overall timeout")
	flag.Parse()
	return cfg
}

func run(cfg config, output *os.File) error {
	if cfg.mongoURI == "" {
		return fmt.Errorf("--mongo-uri is required (or set MONGO_URI)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database(cfg.mongoDB)
	items := make([]personalitykind.Finding, 0)
	for _, spec := range []personalitykind.CollectionSpec{personalitykind.Drafts, personalitykind.Published} {
		findings, err := personalitykind.Findings(ctx, db.Collection(spec.Name), spec)
		if err != nil {
			return err
		}
		items = append(items, findings...)
	}
	rep := report{Findings: items}
	for _, item := range items {
		if item.Eligible {
			rep.Eligible++
		} else {
			rep.Skipped++
		}
	}
	if cfg.apply {
		for _, item := range items {
			if !item.Eligible {
				continue
			}
			spec := personalitykind.Drafts
			if item.Collection == personalitykind.Published.Name {
				spec = personalitykind.Published
			}
			if err := personalitykind.Apply(ctx, db.Collection(spec.Name), spec, item); err != nil {
				return err
			}
			rep.Applied++
		}
	}
	if cfg.json {
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		return encoder.Encode(rep)
	}
	for _, item := range rep.Findings {
		state := "SKIP"
		if item.Eligible {
			state = "READY"
		}
		fmt.Fprintf(output, "%s collection=%s code=%s kind=%s sub_kind=%s algorithm=%s product_channel=%s reason=%s\n", state, item.Collection, item.Code, item.Kind, item.SubKind, item.Algorithm, item.ProductChannel, item.Reason)
	}
	if cfg.apply {
		fmt.Fprintf(output, "applied %d verified normalization(s); skipped %d unsafe row(s)\n", rep.Applied, rep.Skipped)
	} else {
		fmt.Fprintf(output, "audit complete: %d verified normalization(s), %d unsafe row(s); re-run with --apply after review\n", rep.Eligible, rep.Skipped)
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
