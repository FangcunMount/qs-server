// observe_identity_retirement_gate reports whether MC-R018 compatibility
// branches may be deleted (batch 5). Read-only.
//
// Checks:
//  1. published Mongo snapshots retained_read count
//  2. optional MySQL Assessment/Outcome:
//       - retained aliases (mbti|sbti|bigfive|behavioral_rating_default)
//       - empty algorithm rows (separate bucket)
//  3. optional --metrics-ok attestation for Prometheus 14d rates
//
// Default exit 0; use --fail-on-gate to exit 1 when status != PASS.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
)

type config struct {
	mongoURI   string
	mongoDB    string
	mysqlDSN   string
	metricsOK  bool
	failOnGate bool
	jsonOut    bool
	timeout    time.Duration
}

type report struct {
	PublishedCount           int                     `json:"published_count"`
	PublishedRetainedRead    int                     `json:"published_retained_read"`
	AssessmentRetainedAlias  int                     `json:"assessment_retained_alias"`
	AssessmentEmptyAlgorithm int                     `json:"assessment_empty_algorithm"`
	AssessmentBuckets        map[string]int          `json:"assessment_buckets,omitempty"`
	MetricsAttested          bool                    `json:"metrics_attested"`
	DualIdentityGate         identity.RetirementGate `json:"dual_identity_gate"`
	FullGate                 identity.RetirementGate `json:"full_gate"`
	DualIdentityChecklist    []string                `json:"dual_identity_checklist"`
	FullDeleteChecklist      []string                `json:"full_delete_checklist"`
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "observe identity retirement gate failed: --mongo-uri is required (or set MONGO_URI)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect mongo:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	out := &report{
		MetricsAttested:       cfg.metricsOK,
		DualIdentityChecklist: identity.DualIdentityDeleteChecklist(),
		FullDeleteChecklist:   identity.RetirementDeleteChecklist(),
	}
	if err := fillPublished(ctx, client.Database(cfg.mongoDB), out); err != nil {
		fmt.Fprintln(os.Stderr, "audit published:", err)
		os.Exit(1)
	}
	if cfg.mysqlDSN != "" {
		if err := fillAssessment(ctx, cfg.mysqlDSN, out); err != nil {
			fmt.Fprintln(os.Stderr, "audit assessment:", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "note: --mysql-dsn omitted; assessment inventory treated as 0 for gate math (incomplete)")
	}

	inputs := identity.RetirementGateInputs{
		PublishedRetainedRead:    out.PublishedRetainedRead,
		AssessmentRetainedAlias:  out.AssessmentRetainedAlias,
		AssessmentEmptyAlgorithm: out.AssessmentEmptyAlgorithm,
		MetricsRetainedReadOK:    cfg.metricsOK,
		MetricsFallbackOK:        cfg.metricsOK,
	}
	out.DualIdentityGate = identity.EvaluateDualIdentityRetirementGate(inputs)
	out.FullGate = identity.EvaluateRetirementGate(inputs)
	if cfg.mysqlDSN == "" {
		if out.DualIdentityGate.Status != "FAIL" {
			out.DualIdentityGate = identity.RetirementGate{
				Status:  "WARN",
				Reasons: append([]string{"mysql_dsn_omitted"}, out.DualIdentityGate.Reasons...),
			}
		}
		if out.FullGate.Status != "FAIL" {
			out.FullGate = identity.RetirementGate{
				Status:  "WARN",
				Reasons: append([]string{"mysql_dsn_omitted"}, out.FullGate.Reasons...),
			}
		}
	}

	if cfg.jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(out)
	} else {
		printReport(out)
	}
	if cfg.failOnGate && out.DualIdentityGate.Status != "PASS" {
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database")
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "optional MySQL DSN for Assessment/Outcome inventory")
	flag.BoolVar(&cfg.metricsOK, "metrics-ok", false, "attest Prometheus 14d retained_read + algorithm_fallback rates ≈ 0")
	flag.BoolVar(&cfg.failOnGate, "fail-on-gate", false, "exit 1 when gate status is not PASS")
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

func fillPublished(ctx context.Context, db *mongo.Database, out *report) error {
	collName := (&mongomodelcatalog.PublishedAssessmentModelPO{}).CollectionName()
	cur, err := db.Collection(collName).Find(ctx, bson.M{
		"deleted_at": nil, "record_role": "published_snapshot",
	}, options.Find().SetProjection(bson.M{"kind": 1, "algorithm": 1}))
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
		var doc struct {
			Kind      string `bson:"kind"`
			Algorithm string `bson:"algorithm"`
		}
		if err := cur.Decode(&doc); err != nil {
			return err
		}
		out.PublishedCount++
		if identity.ClassifyAlgorithmWritePolicy(modelcatalog.Kind(doc.Kind), modelcatalog.Algorithm(doc.Algorithm)) == identity.AlgorithmWriteRetainedRead {
			out.PublishedRetainedRead++
		}
	}
	return cur.Err()
}

func fillAssessment(ctx context.Context, dsn string, out *report) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	out.AssessmentBuckets = map[string]int{}

	q1 := `
SELECT COALESCE(evaluation_model_kind, ''), COALESCE(evaluation_model_algorithm, ''), COUNT(*)
FROM assessment
WHERE deleted_at IS NULL
  AND (
    evaluation_model_algorithm IN ('mbti','sbti','bigfive','behavioral_rating_default')
    OR evaluation_model_algorithm IS NULL
    OR evaluation_model_algorithm = ''
  )
GROUP BY evaluation_model_kind, evaluation_model_algorithm`
	if err := scanBuckets(ctx, db, q1, "assessment", out); err != nil {
		return err
	}

	q2 := `
SELECT COALESCE(model_kind, ''), COALESCE(model_algorithm, ''), COUNT(*)
FROM evaluation_outcome
WHERE model_algorithm IN ('mbti','sbti','bigfive','behavioral_rating_default')
   OR model_algorithm IS NULL
   OR model_algorithm = ''
GROUP BY model_kind, model_algorithm`
	if err := scanBuckets(ctx, db, q2, "evaluation_outcome", out); err != nil {
		return err
	}
	return nil
}

func scanBuckets(ctx context.Context, db *sql.DB, query, source string, out *report) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind, algorithm sql.NullString
		var cnt int
		if err := rows.Scan(&kind, &algorithm, &cnt); err != nil {
			return err
		}
		key := source + "|" + kind.String + "|" + algorithm.String
		out.AssessmentBuckets[key] = cnt
		if identity.IsRetainedReadAliasAlgorithm(identity.Algorithm(algorithm.String)) {
			out.AssessmentRetainedAlias += cnt
			continue
		}
		if algorithm.String == "" {
			out.AssessmentEmptyAlgorithm += cnt
		}
	}
	return rows.Err()
}

func printReport(out *report) {
	fmt.Printf("published=%d published_retained_read=%d assessment_retained_alias=%d assessment_empty_algorithm=%d metrics_attested=%v\n",
		out.PublishedCount, out.PublishedRetainedRead, out.AssessmentRetainedAlias, out.AssessmentEmptyAlgorithm, out.MetricsAttested)
	printGate("dual_identity_gate", out.DualIdentityGate)
	printGate("full_gate", out.FullGate)
	if len(out.AssessmentBuckets) > 0 {
		keys := make([]string, 0, len(out.AssessmentBuckets))
		for k := range out.AssessmentBuckets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Println("assessment buckets:")
		for _, k := range keys {
			fmt.Printf("  %s count=%d\n", k, out.AssessmentBuckets[k])
		}
	}
	fmt.Println("dual-identity checklist (after dual_identity_gate=PASS):")
	for _, item := range out.DualIdentityChecklist {
		fmt.Printf("  - %s\n", item)
	}
	fmt.Println("full checklist (after full_gate=PASS):")
	for _, item := range out.FullDeleteChecklist {
		fmt.Printf("  - %s\n", item)
	}
}

func printGate(name string, gate identity.RetirementGate) {
	fmt.Printf("%s=%s", name, gate.Status)
	if len(gate.Reasons) > 0 {
		fmt.Printf(" reasons=%s", strings.Join(gate.Reasons, ","))
	}
	fmt.Println()
}
