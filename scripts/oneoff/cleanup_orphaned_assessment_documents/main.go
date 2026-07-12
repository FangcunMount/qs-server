// cleanup_orphaned_assessment_documents removes Mongo report and answer-sheet
// documents whose owning MySQL assessment no longer exists.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/errgroup"
)

type config struct {
	mongoURI, mongoDB, mysqlDSN, source string
	backupSuffix                        string
	answerSheetCreatedBefore            time.Time
	batchSize, maxDocs                  int64
	afterID, toID                       uint64
	workers                             int
	timeout                             time.Duration
	apply, skipBackup, hardDelete       bool
}

type summary struct {
	scanned, candidates, backedUp  int64
	primaryDeleted, relatedDeleted int64
	failed                         int64
}

type safeSummary struct {
	mu sync.Mutex
	s  summary
}

func (s *safeSummary) add(v summary) {
	s.mu.Lock()
	s.s.scanned += v.scanned
	s.s.candidates += v.candidates
	s.s.backedUp += v.backedUp
	s.s.primaryDeleted += v.primaryDeleted
	s.s.relatedDeleted += v.relatedDeleted
	s.s.failed += v.failed
	s.mu.Unlock()
}

func (s *safeSummary) get() summary { s.mu.Lock(); defer s.mu.Unlock(); return s.s }

type phase struct {
	name, collection string
	filter           func(config, uint64) bson.M
	lookupSQL        func(int) string
	related          []relatedCollection
}

type relatedCollection struct {
	name, idField string
	softDelete    bool
}

func main() {
	cfg := parseConfig()
	if err := validateConfig(cfg); err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	db := mongoClient.Database(cfg.mongoDB)
	my, err := sql.Open("mysql", cfg.mysqlDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = my.Close() }()
	my.SetMaxOpenConns(cfg.workers)
	my.SetMaxIdleConns(cfg.workers)
	if err := my.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	if cfg.source == "reports" || cfg.source == "all" {
		if err := runPhase(ctx, db, my, cfg, reportPhase()); err != nil {
			log.Fatal(err)
		}
	}
	if cfg.source == "answersheets" || cfg.source == "all" {
		if err := runPhase(ctx, db, my, cfg, answerSheetPhase()); err != nil {
			log.Fatal(err)
		}
	}
}

func parseConfig() config {
	var c config
	var cutoff string
	flag.StringVar(&c.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&c.mongoDB, "mongo-db", os.Getenv("MONGO_DB"), "MongoDB database")
	flag.StringVar(&c.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN")
	flag.StringVar(&c.source, "source", "all", "reports|answersheets|all")
	flag.StringVar(&cutoff, "answersheet-created-before", "", "required for answersheets; RFC3339 or YYYY-MM-DD")
	flag.StringVar(&c.backupSuffix, "backup-suffix", time.Now().Format("20060102T150405"), "backup collection suffix")
	flag.Int64Var(&c.batchSize, "batch-size", 1000, "documents per page")
	flag.Int64Var(&c.maxDocs, "max-docs", 0, "maximum scanned documents per phase; 0 is unlimited")
	flag.Uint64Var(&c.afterID, "after-id", 0, "resume strictly after domain_id")
	flag.Uint64Var(&c.toID, "to-id", 0, "stop at domain_id inclusive")
	flag.IntVar(&c.workers, "workers", 8, "parallel page workers")
	flag.DurationVar(&c.timeout, "timeout", 24*time.Hour, "overall timeout; 0 disables")
	flag.BoolVar(&c.apply, "apply", false, "perform cleanup; default is dry-run")
	flag.BoolVar(&c.skipBackup, "skip-backup", false, "do not back up matching Mongo documents")
	flag.BoolVar(&c.hardDelete, "hard-delete", false, "physically delete primary report/answersheet documents")
	flag.Parse()
	if cutoff != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if parsed, err := time.Parse(layout, cutoff); err == nil {
				c.answerSheetCreatedBefore = parsed
				break
			}
		}
	}
	return c
}

func validateConfig(c config) error {
	if c.mongoURI == "" || c.mongoDB == "" || c.mysqlDSN == "" {
		return fmt.Errorf("mongo-uri, mongo-db and mysql-dsn are required")
	}
	if c.source != "reports" && c.source != "answersheets" && c.source != "all" {
		return fmt.Errorf("source must be reports, answersheets or all")
	}
	if (c.source == "answersheets" || c.source == "all") && c.answerSheetCreatedBefore.IsZero() {
		return fmt.Errorf("answersheet-created-before is required when cleaning answersheets")
	}
	if c.batchSize < 1 || c.batchSize > 10000 || c.workers < 1 || c.workers > 64 || c.maxDocs < 0 {
		return fmt.Errorf("invalid batch-size, workers or max-docs")
	}
	if c.toID != 0 && c.toID <= c.afterID {
		return fmt.Errorf("to-id must be greater than after-id")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_]+$`).MatchString(c.backupSuffix) {
		return fmt.Errorf("backup-suffix must contain only letters, digits and underscore")
	}
	return nil
}

func reportPhase() phase {
	return phase{name: "reports", collection: "archived_reports", filter: activeRangeFilter,
		lookupSQL: func(n int) string { return inQuery("SELECT id FROM assessment WHERE id IN (", n) },
		related:   []relatedCollection{{"report_query_catalog", "assessment_id", false}}}
}

func answerSheetPhase() phase {
	return phase{name: "answersheets", collection: "answersheets",
		filter: func(c config, after uint64) bson.M {
			q := activeRangeFilter(c, after)
			q["created_at"] = bson.M{"$lt": c.answerSheetCreatedBefore}
			return q
		},
		lookupSQL: func(n int) string {
			return inQuery("SELECT answer_sheet_id FROM assessment WHERE answer_sheet_id IN (", n)
		},
		related: []relatedCollection{{"answersheet_submit_idempotency", "answersheet_id", false}}}
}

func inQuery(prefix string, count int) string {
	return prefix + strings.TrimSuffix(strings.Repeat("?,", count), ",") + ")"
}

func activeRangeFilter(c config, after uint64) bson.M {
	r := bson.M{"$gt": after}
	if c.toID > 0 {
		r["$lte"] = c.toID
	}
	filter := bson.M{"domain_id": r}
	if !c.hardDelete {
		filter["deleted_at"] = nil
	}
	return filter
}

func runPhase(ctx context.Context, db *mongo.Database, my *sql.DB, c config, p phase) error {
	coll := db.Collection(p.collection)
	total, err := coll.CountDocuments(ctx, p.filter(c, c.afterID))
	if err != nil {
		return err
	}
	if c.maxDocs > 0 && total > c.maxDocs {
		total = c.maxDocs
	}
	stats := &safeSummary{}
	cursor, checkpoint, remaining := c.afterID, c.afterID, c.maxDocs
	complete := false
	started := time.Now()
	for {
		wave := make([][]bson.M, 0, c.workers)
		waveLast := cursor
		for len(wave) < c.workers {
			limit := c.batchSize
			if remaining > 0 && remaining < limit {
				limit = remaining
			}
			docs, fetchErr := fetchPage(ctx, coll, p.filter(c, cursor), limit)
			if fetchErr != nil {
				return fetchErr
			}
			if len(docs) == 0 {
				complete = true
				break
			}
			wave = append(wave, docs)
			cursor = asUint64(docs[len(docs)-1]["domain_id"])
			waveLast = cursor
			if remaining > 0 {
				remaining -= int64(len(docs))
				if remaining == 0 {
					break
				}
			}
			if int64(len(docs)) < limit {
				complete = true
				break
			}
		}
		if len(wave) == 0 {
			break
		}
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(c.workers)
		for _, docs := range wave {
			docs := docs
			g.Go(func() error { s, e := processPage(gctx, db, my, c, p, docs); stats.add(s); return e })
		}
		if err := g.Wait(); err != nil {
			printResult(c, p.name, false, checkpoint, stats.get())
			return err
		}
		checkpoint = waveLast
		renderProgress(p.name, total, checkpoint, stats.get(), started)
		if remaining == 0 && c.maxDocs > 0 {
			break
		}
		if complete {
			break
		}
	}
	fmt.Println()
	printResult(c, p.name, complete, checkpoint, stats.get())
	return nil
}

func fetchPage(ctx context.Context, coll *mongo.Collection, filter bson.M, limit int64) ([]bson.M, error) {
	cur, err := coll.Find(ctx, filter, options.Find().SetProjection(bson.M{"domain_id": 1}).SetSort(bson.D{{Key: "domain_id", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var docs []bson.M
	err = cur.All(ctx, &docs)
	return docs, err
}

func processPage(ctx context.Context, db *mongo.Database, my *sql.DB, c config, p phase, docs []bson.M) (s summary, err error) {
	s = summary{scanned: int64(len(docs))}
	defer func() {
		if err != nil && s.failed == 0 {
			s.failed = 1
		}
	}()
	ids := make([]uint64, 0, len(docs))
	args := make([]any, 0, len(docs))
	for _, d := range docs {
		id := asUint64(d["domain_id"])
		ids = append(ids, id)
		args = append(args, id)
	}
	rows, err := my.QueryContext(ctx, p.lookupSQL(len(args)), args...)
	if err != nil {
		s.failed = int64(len(docs))
		return s, err
	}
	existing := map[uint64]struct{}{}
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return s, err
		}
		existing[id] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return s, err
	}
	orphans := make([]uint64, 0)
	for _, id := range ids {
		if _, ok := existing[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	s.candidates = int64(len(orphans))
	if len(orphans) == 0 || !c.apply {
		return s, nil
	}
	if !c.skipBackup {
		collections := append([]relatedCollection{{p.collection, "domain_id", false}}, p.related...)
		for _, item := range collections {
			n, e := backupMatching(ctx, db, item, orphans, c.backupSuffix)
			s.backedUp += n
			if e != nil {
				s.failed++
				return s, e
			}
		}
	}
	now := time.Now().UTC()
	primary := db.Collection(p.collection)
	filter := bson.M{"domain_id": bson.M{"$in": orphans}}
	if !c.hardDelete {
		filter["deleted_at"] = nil
	}
	if c.hardDelete {
		res, e := primary.DeleteMany(ctx, filter)
		if e != nil {
			return s, e
		}
		s.primaryDeleted += res.DeletedCount
	} else {
		res, e := primary.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"deleted_at": now, "updated_at": now}})
		if e != nil {
			return s, e
		}
		s.primaryDeleted += res.ModifiedCount
	}
	for _, item := range p.related {
		q := bson.M{item.idField: bson.M{"$in": orphans}}
		if item.softDelete && !c.hardDelete {
			q["deleted_at"] = nil
			res, e := db.Collection(item.name).UpdateMany(ctx, q, bson.M{"$set": bson.M{"deleted_at": now, "updated_at": now}})
			if e != nil {
				return s, e
			}
			s.relatedDeleted += res.ModifiedCount
		} else {
			res, e := db.Collection(item.name).DeleteMany(ctx, q)
			if e != nil {
				return s, e
			}
			s.relatedDeleted += res.DeletedCount
		}
	}
	return s, nil
}

func backupMatching(ctx context.Context, db *mongo.Database, item relatedCollection, ids []uint64, suffix string) (int64, error) {
	src := db.Collection(item.name)
	dst := db.Collection("cleanup_bak_orphans_" + item.name + "_" + suffix)
	cur, err := src.Find(ctx, bson.M{item.idField: bson.M{"$in": ids}})
	if err != nil {
		return 0, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var docs []interface{}
	for cur.Next(ctx) {
		var d bson.M
		if err := cur.Decode(&d); err != nil {
			return 0, err
		}
		docs = append(docs, d)
	}
	if err := cur.Err(); err != nil || len(docs) == 0 {
		return 0, err
	}
	res, err := dst.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil && !onlyDuplicateErrors(err) {
		return 0, err
	}
	if res == nil {
		return 0, nil
	}
	return int64(len(res.InsertedIDs)), nil
}

func onlyDuplicateErrors(err error) bool {
	if err == nil {
		return true
	}
	var e mongo.BulkWriteException
	if !errors.As(err, &e) || len(e.WriteErrors) == 0 || e.WriteConcernError != nil {
		return false
	}
	for _, w := range e.WriteErrors {
		if w.Code != 11000 {
			return false
		}
	}
	return true
}

func renderProgress(name string, total int64, checkpoint uint64, s summary, started time.Time) {
	pct := float64(0)
	if total > 0 {
		pct = float64(s.scanned) / float64(total)
		if pct > 1 {
			pct = 1
		}
	}
	filled := int(pct * 30)
	rate := float64(s.scanned) / time.Since(started).Seconds()
	fmt.Printf("\r%s [%s%s] %6.2f%% %d/%d rate=%.0f/s checkpoint=%d orphan=%d deleted=%d related=%d", name, strings.Repeat("=", filled), strings.Repeat("-", 30-filled), pct*100, s.scanned, total, rate, checkpoint, s.candidates, s.primaryDeleted, s.relatedDeleted)
}

func printResult(c config, phase string, complete bool, checkpoint uint64, s summary) {
	fmt.Printf("mode=%s phase=%s complete=%t next_after_id=%d scanned=%d candidates=%d backed_up=%d primary_deleted=%d related_deleted=%d failed=%d\n", map[bool]string{true: "apply", false: "dry-run"}[c.apply], phase, complete, checkpoint, s.scanned, s.candidates, s.backedUp, s.primaryDeleted, s.relatedDeleted, s.failed)
}

func asUint64(v any) uint64 {
	switch n := v.(type) {
	case int64:
		return uint64(n)
	case int32:
		return uint64(n)
	case uint64:
		return n
	case float64:
		return uint64(n)
	}
	return 0
}
