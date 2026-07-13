// observe_outbox_by_event_type reports outbox backlog and recent writes grouped by event_type.
// Read-only; used for legacy assessment/report event retirement Phase 0 gates.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	statusPending    = "pending"
	statusPublishing = "publishing"
	statusFailed     = "failed"
	statusPublished  = "published"

	mysqlOutboxTable = "domain_event_outbox"
	mongoOutboxColl  = "domain_event_outbox"
)

var unfinishedStatuses = []string{statusPending, statusPublishing, statusFailed}

var legacyEventTypes []string

var outcomeEventTypes = []string{
	eventcatalog.EvaluationOutcomeCommitted,
	eventcatalog.InterpretationReportGenerated,
	eventcatalog.InterpretationReportFailed,
}

type config struct {
	mysqlDSN   string
	mongoURI   string
	mongoDB    string
	recentDays int
	timeout    time.Duration
	jsonOut    bool
}

type unfinishedRow struct {
	Store     string    `json:"store"`
	EventType string    `json:"event_type"`
	Status    string    `json:"status"`
	Count     int64     `json:"count"`
	Oldest    time.Time `json:"oldest_created_at"`
}

type recentRow struct {
	Store     string    `json:"store"`
	EventType string    `json:"event_type"`
	Count     int64     `json:"count"`
	Newest    time.Time `json:"newest_created_at"`
}

type gateResult struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons,omitempty"`
}

type report struct {
	ObservedAt        time.Time       `json:"observed_at"`
	RecentSince       time.Time       `json:"recent_since"`
	RecentDays        int             `json:"recent_days"`
	Unfinished        []unfinishedRow `json:"unfinished"`
	RecentWrites      []recentRow     `json:"recent_writes"`
	LegacyUnfinished  int64           `json:"legacy_unfinished_total"`
	LegacyRecent      int64           `json:"legacy_recent_writes_total"`
	OutcomeUnfinished int64           `json:"outcome_unfinished_total"`
	OutcomeRecent     int64           `json:"outcome_recent_writes_total"`
	Gate              gateResult      `json:"gate"`
}

func main() {
	cfg := parseFlags()
	now := time.Now().UTC()
	recentSince := now.AddDate(0, 0, -cfg.recentDays)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	rep := report{
		ObservedAt:  now,
		RecentSince: recentSince,
		RecentDays:  cfg.recentDays,
	}

	if strings.TrimSpace(cfg.mysqlDSN) != "" {
		db, err := openMySQL(cfg.mysqlDSN)
		if err != nil {
			log.Fatalf("open mysql: %v", err)
		}
		defer func() { _ = db.Close() }()
		if err := db.PingContext(ctx); err != nil {
			log.Fatalf("ping mysql: %v", err)
		}
		unfinished, err := queryMySQLUnfinished(ctx, db)
		if err != nil {
			log.Fatalf("mysql unfinished: %v", err)
		}
		recent, err := queryMySQLRecent(ctx, db, recentSince)
		if err != nil {
			log.Fatalf("mysql recent: %v", err)
		}
		rep.Unfinished = append(rep.Unfinished, unfinished...)
		rep.RecentWrites = append(rep.RecentWrites, recent...)
	}

	if strings.TrimSpace(cfg.mongoURI) != "" {
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
		if err != nil {
			log.Fatalf("connect mongo: %v", err)
		}
		defer func() { _ = client.Disconnect(context.Background()) }()
		if err := client.Ping(ctx, nil); err != nil {
			log.Fatalf("ping mongo: %v", err)
		}
		coll := client.Database(cfg.mongoDB).Collection(mongoOutboxColl)
		unfinished, err := queryMongoUnfinished(ctx, coll)
		if err != nil {
			log.Fatalf("mongo unfinished: %v", err)
		}
		recent, err := queryMongoRecent(ctx, coll, recentSince)
		if err != nil {
			log.Fatalf("mongo recent: %v", err)
		}
		rep.Unfinished = append(rep.Unfinished, unfinished...)
		rep.RecentWrites = append(rep.RecentWrites, recent...)
	}

	summarize(&rep)
	rep.Gate = evaluateGate(rep)

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
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", "", "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", "qs", "MongoDB database name")
	flag.IntVar(&cfg.recentDays, "recent-days", 7, "count writes with created_at within this many days")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "overall script timeout")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit machine-readable JSON")
	flag.Parse()

	if strings.TrimSpace(cfg.mysqlDSN) == "" && strings.TrimSpace(cfg.mongoURI) == "" {
		log.Fatal("at least one of --mysql-dsn or --mongo-uri is required")
	}
	if cfg.recentDays <= 0 {
		log.Fatal("--recent-days must be greater than 0")
	}
	return cfg
}

func openMySQL(dsn string) (*sql.DB, error) {
	c, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	c.ParseTime = true
	if c.Collation == "" {
		c.Collation = "utf8mb4_unicode_ci"
	}
	return sql.Open("mysql", c.FormatDSN())
}

func queryMySQLUnfinished(ctx context.Context, db *sql.DB) ([]unfinishedRow, error) {
	placeholders := strings.Repeat("?,", len(unfinishedStatuses))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(`
SELECT event_type, status, COUNT(*) AS cnt, MIN(created_at) AS oldest
FROM %s
WHERE status IN (%s)
GROUP BY event_type, status
ORDER BY event_type, status`, mysqlOutboxTable, placeholders)

	args := make([]any, len(unfinishedStatuses))
	for i, s := range unfinishedStatuses {
		args[i] = s
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]unfinishedRow, 0)
	for rows.Next() {
		var row unfinishedRow
		row.Store = "mysql"
		if err := rows.Scan(&row.EventType, &row.Status, &row.Count, &row.Oldest); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func queryMySQLRecent(ctx context.Context, db *sql.DB, since time.Time) ([]recentRow, error) {
	query := fmt.Sprintf(`
SELECT event_type, COUNT(*) AS cnt, MAX(created_at) AS newest
FROM %s
WHERE created_at >= ?
GROUP BY event_type
ORDER BY event_type`, mysqlOutboxTable)

	rows, err := db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]recentRow, 0)
	for rows.Next() {
		var row recentRow
		row.Store = "mysql"
		if err := rows.Scan(&row.EventType, &row.Count, &row.Newest); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func queryMongoUnfinished(ctx context.Context, coll *mongo.Collection) ([]unfinishedRow, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"status": bson.M{"$in": unfinishedStatuses}}}},
		{{Key: "$group", Value: bson.M{
			"_id":    bson.M{"event_type": "$event_type", "status": "$status"},
			"count":  bson.M{"$sum": 1},
			"oldest": bson.M{"$min": "$created_at"},
		}}},
		{{Key: "$sort", Value: bson.M{"_id.event_type": 1, "_id.status": 1}}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	type aggRow struct {
		ID struct {
			EventType string `bson:"event_type"`
			Status    string `bson:"status"`
		} `bson:"_id"`
		Count  int64     `bson:"count"`
		Oldest time.Time `bson:"oldest"`
	}
	rows := make([]aggRow, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]unfinishedRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, unfinishedRow{
			Store:     "mongo",
			EventType: row.ID.EventType,
			Status:    row.ID.Status,
			Count:     row.Count,
			Oldest:    row.Oldest,
		})
	}
	return out, nil
}

func queryMongoRecent(ctx context.Context, coll *mongo.Collection, since time.Time) ([]recentRow, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"created_at": bson.M{"$gte": since}}}},
		{{Key: "$group", Value: bson.M{
			"_id":    "$event_type",
			"count":  bson.M{"$sum": 1},
			"newest": bson.M{"$max": "$created_at"},
		}}},
		{{Key: "$sort", Value: bson.M{"_id": 1}}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	type aggRow struct {
		EventType string    `bson:"_id"`
		Count     int64     `bson:"count"`
		Newest    time.Time `bson:"newest"`
	}
	rows := make([]aggRow, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]recentRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, recentRow{
			Store:     "mongo",
			EventType: row.EventType,
			Count:     row.Count,
			Newest:    row.Newest,
		})
	}
	return out, nil
}

func summarize(rep *report) {
	for _, row := range rep.Unfinished {
		if isLegacyEventType(row.EventType) {
			rep.LegacyUnfinished += row.Count
		}
		if isOutcomeEventType(row.EventType) {
			rep.OutcomeUnfinished += row.Count
		}
	}
	for _, row := range rep.RecentWrites {
		if isLegacyEventType(row.EventType) {
			rep.LegacyRecent += row.Count
		}
		if isOutcomeEventType(row.EventType) {
			rep.OutcomeRecent += row.Count
		}
	}
}

func isLegacyEventType(eventType string) bool {
	for _, legacy := range legacyEventTypes {
		if eventType == legacy {
			return true
		}
	}
	return false
}

func isOutcomeEventType(eventType string) bool {
	for _, outcome := range outcomeEventTypes {
		if eventType == outcome {
			return true
		}
	}
	return false
}

func evaluateGate(rep report) gateResult {
	var reasons []string
	if rep.LegacyUnfinished > 0 {
		reasons = append(reasons, fmt.Sprintf("legacy unfinished backlog = %d (want 0)", rep.LegacyUnfinished))
	}
	if rep.LegacyRecent > 0 {
		reasons = append(reasons, fmt.Sprintf("legacy recent writes in last %d days = %d (want 0)", rep.RecentDays, rep.LegacyRecent))
	}
	if len(reasons) == 0 {
		return gateResult{Status: "PASS"}
	}
	return gateResult{Status: "WARN", Reasons: reasons}
}

func printHuman(rep report) {
	fmt.Printf("observed_at=%s recent_since=%s recent_days=%d\n",
		rep.ObservedAt.Format(time.RFC3339),
		rep.RecentSince.Format(time.RFC3339),
		rep.RecentDays,
	)
	fmt.Println()
	fmt.Println("=== unfinished by event_type / status ===")
	printUnfinished(rep.Unfinished)
	fmt.Println()
	fmt.Println("=== recent writes by event_type ===")
	printRecent(rep.RecentWrites)
	fmt.Println()
	fmt.Printf("legacy summary: unfinished=%d recent_writes=%d\n", rep.LegacyUnfinished, rep.LegacyRecent)
	fmt.Printf("outcome summary: unfinished=%d recent_writes=%d\n", rep.OutcomeUnfinished, rep.OutcomeRecent)
	fmt.Println()
	fmt.Printf("gate: %s\n", rep.Gate.Status)
	for _, reason := range rep.Gate.Reasons {
		fmt.Printf("  - %s\n", reason)
	}
}

func printUnfinished(rows []unfinishedRow) {
	if len(rows) == 0 {
		fmt.Println("(none)")
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Store != rows[j].Store {
			return rows[i].Store < rows[j].Store
		}
		if rows[i].EventType != rows[j].EventType {
			return rows[i].EventType < rows[j].EventType
		}
		return rows[i].Status < rows[j].Status
	})
	fmt.Printf("%-6s %-36s %-12s %8s %s\n", "store", "event_type", "status", "count", "oldest")
	for _, row := range rows {
		marker := ""
		if isLegacyEventType(row.EventType) {
			marker = " [legacy]"
		} else if isOutcomeEventType(row.EventType) {
			marker = " [outcome]"
		}
		fmt.Printf("%-6s %-36s %-12s %8d %s%s\n",
			row.Store, row.EventType, row.Status, row.Count, row.Oldest.Format(time.RFC3339), marker)
	}
}

func printRecent(rows []recentRow) {
	if len(rows) == 0 {
		fmt.Println("(none)")
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Store != rows[j].Store {
			return rows[i].Store < rows[j].Store
		}
		return rows[i].EventType < rows[j].EventType
	})
	fmt.Printf("%-6s %-36s %8s %s\n", "store", "event_type", "count", "newest")
	for _, row := range rows {
		marker := ""
		if isLegacyEventType(row.EventType) {
			marker = " [legacy]"
		} else if isOutcomeEventType(row.EventType) {
			marker = " [outcome]"
		}
		fmt.Printf("%-6s %-36s %8d %s%s\n",
			row.Store, row.EventType, row.Count, row.Newest.Format(time.RFC3339), marker)
	}
}
