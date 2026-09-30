// interpretation_retry_gap_audit performs a bounded, read-only audit of
// scheduled interpretation retries. A finding is never replay authorization.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	mongoURI   string
	mongoDB    string
	afterID    uint64
	upperID    uint64
	dueBefore  time.Time
	maxRows    int
	timeout    time.Duration
	includeIDs bool
}

type runRecord struct {
	ID              uint64     `bson:"domain_id"`
	GenerationID    uint64     `bson:"generation_id"`
	Attempt         int        `bson:"attempt"`
	Status          string     `bson:"status"`
	Disposition     string     `bson:"retry_disposition"`
	NextAttemptAt   *time.Time `bson:"next_attempt_at"`
	RetryEventID    string     `bson:"retry_event_id"`
	ActionRequestID string     `bson:"action_request_id"`
}

type generationRecord struct {
	Status      string `bson:"status"`
	LatestRunID uint64 `bson:"latest_run_id"`
}

type outboxRecord struct {
	EventType            string     `bson:"event_type"`
	State                string     `bson:"state"`
	TransportConfirmedAt *time.Time `bson:"transport_confirmed_at"`
}

const (
	other                = "other"
	notDue               = "not_due"
	successorPresent     = "successor_present"
	relayPending         = "relay_pending_or_unknown"
	suspectedDeliveryGap = "suspected_post_confirm_gap"
	manualRequired       = "manual_required"
)

type finding struct {
	RunID        string `json:"run_id"`
	GenerationID string `json:"generation_id"`
	RetryEventID string `json:"retry_event_id,omitempty"`
	Category     string `json:"category"`
}

type report struct {
	AfterID      string         `json:"after_id,omitempty"`
	UpperID      string         `json:"upper_id,omitempty"`
	NextAfterID  string         `json:"next_after_id,omitempty"`
	DueBefore    time.Time      `json:"due_before"`
	Scanned      int            `json:"scanned"`
	Complete     bool           `json:"complete"`
	Counts       map[string]int `json:"counts"`
	Findings     []finding      `json:"findings,omitempty"`
	SnapshotNote string         `json:"snapshot_note"`
}

func main() { os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func runCLI(parent context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stdin, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "interpretation retry gap audit: invalid bounded input")
		return 1
	}
	ctx, cancel := context.WithTimeout(parent, cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "interpretation retry gap audit: Mongo connection failed")
		return 1
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		_, _ = fmt.Fprintln(stderr, "interpretation retry gap audit: Mongo unavailable")
		return 1
	}
	db := client.Database(cfg.mongoDB)
	runs := db.Collection("interpretation_runs")
	outbox := db.Collection("rm_outbox")
	if err := requireIndexes(ctx, runs, db.Collection("report_generations"), outbox); err != nil {
		_, _ = fmt.Fprintln(stderr, "interpretation retry gap audit: required bounded lookup index unavailable")
		return 1
	}
	result, err := scan(ctx, db, cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "interpretation retry gap audit: read failed")
		return 1
	}
	if !cfg.includeIDs {
		result.AfterID, result.UpperID, result.NextAfterID = "", "", ""
		result.Findings = nil
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintln(stderr, "interpretation retry gap audit: output failed")
		return 1
	}
	if !result.Complete {
		return 3
	}
	if result.Counts[suspectedDeliveryGap]+result.Counts[manualRequired]+result.Counts[relayPending] > 0 {
		return 2
	}
	return 0
}

func parseConfig(args []string, stdin io.Reader, stderr io.Writer) (config, error) {
	var cfg config
	var dueBefore string
	var connectionsStdin bool
	flags := flag.NewFlagSet("interpretation_retry_gap_audit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Uint64Var(&cfg.afterID, "after-id", 0, "exclusive fixed Run ID lower bound")
	flags.Uint64Var(&cfg.upperID, "upper-id", 0, "inclusive fixed Run ID upper bound")
	flags.StringVar(&dueBefore, "due-before", "", "fixed RFC3339 retry maturity cutoff")
	flags.IntVar(&cfg.maxRows, "max-rows", 500, "maximum Run rows per invocation (1-1000)")
	flags.DurationVar(&cfg.timeout, "timeout", 2*time.Minute, "overall deadline (1s-5m)")
	flags.BoolVar(&cfg.includeIDs, "include-ids", false, "include private identity and cursor output")
	flags.BoolVar(&connectionsStdin, "connections-stdin", false, "read Mongo connection JSON from stdin")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if connectionsStdin {
		var input struct {
			URI string `json:"mongo_uri"`
			DB  string `json:"mongo_db"`
		}
		decoder := json.NewDecoder(io.LimitReader(stdin, 16385))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return config{}, err
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return config{}, errors.New("trailing connection input")
		}
		cfg.mongoURI, cfg.mongoDB = input.URI, input.DB
	} else {
		cfg.mongoURI, cfg.mongoDB = os.Getenv("MONGO_URI"), os.Getenv("MONGO_DB")
		if cfg.mongoURI == "" {
			cfg.mongoURI, cfg.mongoDB = qsRuntimeMongoConnection()
		}
	}
	var err error
	cfg.dueBefore, err = time.Parse(time.RFC3339, dueBefore)
	if err != nil || flags.NArg() != 0 || cfg.mongoURI == "" || cfg.mongoDB == "" ||
		cfg.afterID >= cfg.upperID || cfg.upperID > math.MaxInt64 || cfg.maxRows < 1 || cfg.maxRows > 1000 ||
		cfg.timeout < time.Second || cfg.timeout > 5*time.Minute || cfg.dueBefore.After(time.Now().Add(-10*time.Minute)) {
		return config{}, errors.New("invalid bounded read-only scan configuration")
	}
	return cfg, nil
}

// The one-off command may run beside the current API using its existing env
// file. Constructing the URI inside this process avoids printing credentials
// or placing them in a shell command line.
func qsRuntimeMongoConnection() (string, string) {
	host := os.Getenv("QS_APISERVER_MONGODB_HOST")
	user := os.Getenv("QS_APISERVER_MONGODB_USERNAME")
	password := os.Getenv("QS_APISERVER_MONGODB_PASSWORD")
	database := os.Getenv("QS_APISERVER_MONGODB_DATABASE")
	if host == "" || user == "" || password == "" || database == "" {
		return "", ""
	}
	uri := url.URL{Scheme: "mongodb", User: url.UserPassword(user, password), Host: host, Path: "/" + database}
	uri.RawQuery = url.Values{"authSource": {database}, "directConnection": {"true"}}.Encode()
	return uri.String(), database
}

func requireIndexes(ctx context.Context, runs, generations, outbox *mongo.Collection) error {
	for _, requirement := range []struct {
		collection *mongo.Collection
		name       string
	}{
		{runs, "uk_interpretation_run_domain_id"},
		{runs, "uk_interpretation_run_generation_attempt"},
		{generations, "uk_generation_domain_id"},
	} {
		cursor, err := requirement.collection.Indexes().List(ctx)
		if err != nil {
			return err
		}
		found := false
		for cursor.Next(ctx) {
			var index struct {
				Name string `bson:"name"`
			}
			if err := cursor.Decode(&index); err != nil {
				_ = cursor.Close(ctx)
				return err
			}
			found = found || index.Name == requirement.name
		}
		err = cursor.Err()
		_ = cursor.Close(ctx)
		if err != nil || !found {
			return errors.New("run index unavailable")
		}
	}
	cursor, err := outbox.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var index struct {
			Key bson.D `bson:"key"`
		}
		if err := cursor.Decode(&index); err != nil {
			return err
		}
		if len(index.Key) > 0 && index.Key[0].Key == "message_id" {
			return nil
		}
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	return errors.New("outbox message identity index unavailable")
}

func scan(ctx context.Context, db *mongo.Database, cfg config) (report, error) {
	result := report{
		AfterID: strconv.FormatUint(cfg.afterID, 10), UpperID: strconv.FormatUint(cfg.upperID, 10),
		NextAfterID: strconv.FormatUint(cfg.afterID, 10), DueBefore: cfg.dueBefore,
		Complete: true, Counts: make(map[string]int),
		SnapshotNote: "nontransactional read-only snapshot; findings require fresh per-ID review and never authorize replay",
	}
	runs := db.Collection("interpretation_runs")
	filter := bson.M{"domain_id": bson.M{"$gt": cfg.afterID, "$lte": cfg.upperID}, "deleted_at": nil}
	cursor, err := runs.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "domain_id", Value: 1}}).
		SetHint("uk_interpretation_run_domain_id").SetLimit(int64(cfg.maxRows+1)).SetProjection(bson.M{
		"domain_id": 1, "generation_id": 1, "attempt": 1, "status": 1,
		"retry_disposition": 1, "next_attempt_at": 1, "retry_event_id": 1, "action_request_id": 1,
	}))
	if err != nil {
		return report{}, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		if result.Scanned == cfg.maxRows {
			result.Complete = false
			break
		}
		var row runRecord
		if err := cursor.Decode(&row); err != nil {
			return report{}, err
		}
		category, err := inspectRun(ctx, db, row, cfg.dueBefore)
		if err != nil {
			return report{}, err
		}
		result.Scanned++
		result.NextAfterID = strconv.FormatUint(row.ID, 10)
		result.Counts[category]++
		if cfg.includeIDs && (category == suspectedDeliveryGap || category == manualRequired || category == relayPending) {
			result.Findings = append(result.Findings, finding{
				RunID: strconv.FormatUint(row.ID, 10), GenerationID: strconv.FormatUint(row.GenerationID, 10),
				RetryEventID: row.RetryEventID, Category: category,
			})
		}
	}
	if err := cursor.Err(); err != nil {
		return report{}, err
	}
	return result, nil
}

func inspectRun(ctx context.Context, db *mongo.Database, row runRecord, dueBefore time.Time) (string, error) {
	if row.Status != "failed" || row.Disposition != "automatic" {
		return other, nil
	}
	if row.NextAttemptAt == nil || row.ID == 0 || row.GenerationID == 0 || row.Attempt < 1 ||
		row.RetryEventID == "" || row.ActionRequestID != "" {
		return manualRequired, nil
	}
	if row.NextAttemptAt.After(dueBefore) {
		return notDue, nil
	}
	runs := db.Collection("interpretation_runs")
	// Even a soft-deleted successor proves that the retry was once claimed.
	successor := runs.FindOne(ctx, bson.M{"generation_id": row.GenerationID, "attempt": bson.M{"$gt": row.Attempt}},
		options.FindOne().SetProjection(bson.M{"_id": 1}).SetHint("uk_interpretation_run_generation_attempt"))
	if successor.Err() == nil {
		return successorPresent, nil
	}
	if !errors.Is(successor.Err(), mongo.ErrNoDocuments) {
		return "", successor.Err()
	}
	var generation generationRecord
	err := db.Collection("report_generations").FindOne(ctx, bson.M{"domain_id": row.GenerationID, "deleted_at": nil},
		options.FindOne().SetProjection(bson.M{"status": 1, "latest_run_id": 1}).SetHint("uk_generation_domain_id")).Decode(&generation)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return manualRequired, nil
	}
	if err != nil {
		return "", err
	}
	if generation.Status != "failed" || generation.LatestRunID != row.ID {
		return manualRequired, nil
	}
	cursor, err := db.Collection("rm_outbox").Find(ctx, bson.M{"message_id": row.RetryEventID},
		options.Find().SetLimit(2).SetProjection(bson.M{"event_type": 1, "state": 1, "transport_confirmed_at": 1}))
	if err != nil {
		return "", err
	}
	defer func() { _ = cursor.Close(ctx) }()
	var intents []outboxRecord
	if err := cursor.All(ctx, &intents); err != nil {
		return "", err
	}
	return classifyHandoff(row, generation, intents), nil
}

func classifyHandoff(row runRecord, generation generationRecord, intents []outboxRecord) string {
	if generation.Status != "failed" || generation.LatestRunID != row.ID || len(intents) != 1 ||
		intents[0].EventType != "interpretation.retry.requested" {
		return manualRequired
	}
	if intents[0].State == "published" && intents[0].TransportConfirmedAt != nil {
		return suspectedDeliveryGap
	}
	if intents[0].State == "pending" || intents[0].State == "retry_wait" || intents[0].State == "publishing" {
		return relayPending
	}
	return manualRequired
}
