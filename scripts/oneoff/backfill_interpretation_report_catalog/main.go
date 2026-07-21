// backfill_interpretation_report_catalog builds the compact assessment-level
// report query catalog from immutable archives and current report artifacts.
// It is dry-run by default and is safe to resume and repeat.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/errgroup"
)

type config struct {
	mongoURI, mongoDB, mysqlDSN, source string
	batchSize, maxDocs                  int64
	afterID, toID                       uint64
	workers                             int
	progressInterval, timeout           time.Duration
	apply, verifyOnly, noProgress       bool
}

type summary struct {
	scanned, inserted, updated, unchanged int64
	missingAssessment, missingTestee      int64
	missingOrg                            int64
	conflict, failed                      int64
}

type concurrentSummary struct {
	mu sync.Mutex
	s  summary
}

func (s *concurrentSummary) add(delta summary) {
	s.mu.Lock()
	s.s.scanned += delta.scanned
	s.s.inserted += delta.inserted
	s.s.updated += delta.updated
	s.s.unchanged += delta.unchanged
	s.s.missingAssessment += delta.missingAssessment
	s.s.missingTestee += delta.missingTestee
	s.s.missingOrg += delta.missingOrg
	s.s.conflict += delta.conflict
	s.s.failed += delta.failed
	s.mu.Unlock()
}

func (s *concurrentSummary) snapshot() summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s
}

type phaseResult struct {
	phase      string
	checkpoint uint64
	complete   bool
	summary    summary
}

func main() {
	c := parseConfig()
	if err := validateConfig(c); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(c.mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := client.Database(c.mongoDB)
	if err := ensureIndexes(ctx, db); err != nil {
		log.Fatal(err)
	}

	var mysqlDB *sql.DB
	if c.verifyOnly || c.source == "archive" || c.source == "all" {
		mysqlDB, err = sql.Open("mysql", c.mysqlDSN)
		if err != nil {
			log.Fatal(err)
		}
		defer func() { _ = mysqlDB.Close() }()
		mysqlDB.SetMaxOpenConns(c.workers)
		mysqlDB.SetMaxIdleConns(c.workers)
		mysqlDB.SetConnMaxLifetime(10 * time.Minute)
		if err := mysqlDB.PingContext(ctx); err != nil {
			log.Fatalf("ping mysql: %v", err)
		}
	}
	if c.verifyOnly {
		if err := verify(ctx, db, mysqlDB, c.batchSize); err != nil {
			log.Fatal(err)
		}
		return
	}

	if c.source == "archive" || c.source == "all" {
		result, runErr := runPhase(ctx, db, mysqlDB, c, archivePhase())
		printPhaseResult(c, result)
		if runErr != nil {
			log.Fatal(runErr)
		}
	}
	if c.source == "artifact" || c.source == "all" {
		result, runErr := runPhase(ctx, db, nil, c, artifactPhase())
		printPhaseResult(c, result)
		if runErr != nil {
			log.Fatal(runErr)
		}
	}
}

func parseConfig() config {
	var c config
	flag.StringVar(&c.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&c.mongoDB, "mongo-db", os.Getenv("MONGO_DB"), "MongoDB database")
	flag.StringVar(&c.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN; required for archive")
	flag.StringVar(&c.source, "source", "all", "archive|artifact|all")
	flag.Int64Var(&c.batchSize, "batch-size", 1000, "documents per Mongo read and BulkWrite batch")
	flag.Int64Var(&c.batchSize, "page-size", 1000, "deprecated alias of --batch-size")
	flag.Int64Var(&c.maxDocs, "max-docs", 0, "maximum documents per source in this invocation; 0 means unlimited")
	flag.Uint64Var(&c.afterID, "after-id", 0, "resume strictly after domain_id")
	flag.Uint64Var(&c.toID, "to-id", 0, "stop at this domain_id inclusive; 0 means unbounded")
	flag.IntVar(&c.workers, "workers", 8, "parallel page workers")
	flag.DurationVar(&c.progressInterval, "progress-interval", 2*time.Second, "progress refresh interval")
	flag.DurationVar(&c.timeout, "timeout", 24*time.Hour, "overall timeout; 0 disables timeout")
	flag.BoolVar(&c.apply, "apply", false, "write catalog entries")
	flag.BoolVar(&c.verifyOnly, "verify-only", false, "only reconcile catalog")
	flag.BoolVar(&c.noProgress, "no-progress", false, "disable progress bar")
	flag.Parse()
	return c
}

func validateConfig(c config) error {
	if c.mongoURI == "" || c.mongoDB == "" {
		return fmt.Errorf("mongo-uri and mongo-db are required")
	}
	if c.source != "archive" && c.source != "artifact" && c.source != "all" {
		return fmt.Errorf("source must be archive, artifact or all")
	}
	if (c.verifyOnly || c.source == "archive" || c.source == "all") && c.mysqlDSN == "" {
		return fmt.Errorf("mysql-dsn is required for archive backfill and verification")
	}
	if c.batchSize < 1 || c.batchSize > 10000 {
		return fmt.Errorf("batch-size must be between 1 and 10000")
	}
	if c.workers < 1 || c.workers > 64 {
		return fmt.Errorf("workers must be between 1 and 64")
	}
	if c.maxDocs < 0 || c.progressInterval <= 0 || c.timeout < 0 {
		return fmt.Errorf("max-docs, progress-interval or timeout is invalid")
	}
	if c.toID != 0 && c.toID <= c.afterID {
		return fmt.Errorf("to-id must be greater than after-id")
	}
	return nil
}

type phaseSpec struct {
	name       string
	collection string
	projection bson.M
	process    func(context.Context, *mongo.Database, *sql.DB, config, []bson.M) (summary, error)
}

func archivePhase() phaseSpec {
	return phaseSpec{
		name: "archive", collection: "archived_reports",
		projection: bson.M{"domain_id": 1, "scale_code": 1, "risk_level": 1, "created_at": 1},
		process:    processArchiveBatch,
	}
}

func artifactPhase() phaseSpec {
	return phaseSpec{
		name: "artifact", collection: "interpret_report_artifacts",
		projection: bson.M{"domain_id": 1, "assessment_id": 1, "org_id": 1, "testee_id": 1, "scale_code": 1, "risk_level": 1, "generated_at": 1},
		process:    processArtifactBatch,
	}
}

func runPhase(ctx context.Context, db *mongo.Database, mysqlDB *sql.DB, c config, spec phaseSpec) (phaseResult, error) {
	collection := db.Collection(spec.collection)
	if !c.noProgress {
		fmt.Printf("phase=%s counting source range...\n", spec.name)
	}
	total, err := collection.CountDocuments(ctx, sourceRangeFilter(spec, c.afterID, c.toID))
	if err != nil {
		return phaseResult{phase: spec.name, checkpoint: c.afterID}, fmt.Errorf("count %s source: %w", spec.name, err)
	}
	if c.maxDocs > 0 && total > c.maxDocs {
		total = c.maxDocs
	}
	stats := &concurrentSummary{}
	progress := newProgressReporter(spec.name, total, stats, c.noProgress, c.progressInterval)
	progress.start()
	defer progress.stop()
	workers := c.workers
	if spec.name == "artifact" {
		// Artifact pages must advance in source order. Concurrent upserts for two
		// reports of the same assessment can otherwise race on the unique key and
		// leave the older report in the catalog.
		workers = 1
	}

	cursorID := c.afterID
	checkpoint := c.afterID
	remaining := c.maxDocs
	complete := false
	for {
		wave := make([][]bson.M, 0, workers)
		waveLastID := cursorID
		for len(wave) < workers {
			limit := c.batchSize
			if remaining > 0 && remaining < limit {
				limit = remaining
			}
			docs, fetchErr := fetchPage(ctx, collection, sourceRangeFilter(spec, cursorID, c.toID), limit, spec.projection)
			if fetchErr != nil {
				progress.abort()
				return phaseResult{phase: spec.name, checkpoint: checkpoint, summary: stats.snapshot()}, fetchErr
			}
			if len(docs) == 0 {
				complete = true
				break
			}
			wave = append(wave, docs)
			cursorID = asUint64(docs[len(docs)-1]["domain_id"])
			waveLastID = cursorID
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

		group, groupCtx := errgroup.WithContext(ctx)
		group.SetLimit(workers)
		for _, docs := range wave {
			docs := docs
			group.Go(func() error {
				delta, processErr := spec.process(groupCtx, db, mysqlDB, c, docs)
				stats.add(delta)
				return processErr
			})
		}
		if err := group.Wait(); err != nil {
			progress.abort()
			return phaseResult{phase: spec.name, checkpoint: checkpoint, summary: stats.snapshot()}, fmt.Errorf("process %s wave after %d: %w", spec.name, checkpoint, err)
		}
		checkpoint = waveLastID
		progress.setCheckpoint(checkpoint)
		if remaining == 0 && c.maxDocs > 0 {
			complete, err = sourceExhausted(ctx, collection, sourceRangeFilter(spec, checkpoint, c.toID))
			if err != nil {
				progress.abort()
				return phaseResult{phase: spec.name, checkpoint: checkpoint, summary: stats.snapshot()}, err
			}
			break
		}
		if complete {
			break
		}
	}
	result := phaseResult{phase: spec.name, checkpoint: checkpoint, complete: complete, summary: stats.snapshot()}
	progress.stop()
	progress.finish(result)
	return result, nil
}

func fetchPage(ctx context.Context, collection *mongo.Collection, filter bson.M, limit int64, projection bson.M) ([]bson.M, error) {
	cur, err := collection.Find(ctx, filter, options.Find().
		SetProjection(projection).
		SetSort(bson.D{{Key: "domain_id", Value: 1}}).
		SetLimit(limit).
		SetBatchSize(int32(limit)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func rangeFilter(afterID, toID uint64) bson.M {
	rangeQuery := bson.M{"$gt": afterID}
	if toID > 0 {
		rangeQuery["$lte"] = toID
	}
	return bson.M{"domain_id": rangeQuery}
}

func sourceRangeFilter(spec phaseSpec, afterID, toID uint64) bson.M {
	filter := rangeFilter(afterID, toID)
	filter["deleted_at"] = nil
	return filter
}

func sourceExhausted(ctx context.Context, collection *mongo.Collection, filter bson.M) (bool, error) {
	count, err := collection.CountDocuments(ctx, filter, options.Count().SetLimit(1))
	return count == 0, err
}

func processArchiveBatch(ctx context.Context, db *mongo.Database, mysqlDB *sql.DB, c config, docs []bson.M) (summary, error) {
	delta := summary{scanned: int64(len(docs))}
	associations, err := loadAssessmentAssociations(ctx, mysqlDB, docs)
	if err != nil {
		delta.failed = int64(len(docs))
		return delta, err
	}
	models := make([]mongo.WriteModel, 0, len(docs))
	now := time.Now().UTC()
	for _, d := range docs {
		id := asUint64(d["domain_id"])
		association, ok := associations[id]
		if !ok {
			delta.missingAssessment++
			continue
		}
		if association.TesteeID == 0 {
			delta.missingTestee++
			continue
		}
		if association.OrgID == 0 {
			delta.missingOrg++
			continue
		}
		if !c.apply {
			continue
		}
		entry := archiveCatalogEntry(d, association)
		update := bson.M{"$set": entry, "$setOnInsert": bson.M{"updated_at": now}}
		models = append(models, mongo.NewUpdateOneModel().SetFilter(archiveCatalogFilter(id)).SetUpdate(update).SetUpsert(true))
	}
	if !c.apply || len(models) == 0 {
		return delta, nil
	}
	result, err := db.Collection("report_query_catalog").BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	applyBulkResult(&delta, int64(len(models)), result, duplicateWriteCount(err))
	if err != nil && !onlyDuplicateWriteErrors(err) {
		delta.failed++
		return delta, err
	}
	return delta, nil
}

func archiveCatalogFilter(assessmentID uint64) bson.M {
	return bson.M{
		"assessment_id": assessmentID,
		"$or": bson.A{
			bson.M{"source_kind": "archive"},
			bson.M{"source_kind": bson.M{"$exists": false}},
		},
	}
}

type assessmentAssociation struct {
	TesteeID uint64
	OrgID    int64
}

func archiveCatalogEntry(doc bson.M, association assessmentAssociation) bson.M {
	assessmentID := asUint64(doc["domain_id"])
	return bson.M{
		"assessment_id":  assessmentID,
		"org_id":         association.OrgID,
		"testee_id":      association.TesteeID,
		"source_kind":    "archive",
		"source_id":      assessmentID,
		"model_code":     asString(doc["scale_code"]),
		"risk_level":     asString(doc["risk_level"]),
		"sort_at":        asTime(doc["created_at"]),
		"sort_report_id": uint64(0),
	}
}

func processArtifactBatch(ctx context.Context, db *mongo.Database, _ *sql.DB, c config, docs []bson.M) (summary, error) {
	delta := summary{scanned: int64(len(docs))}
	if !c.apply {
		return delta, nil
	}
	docs = latestArtifactsByAssessment(docs)
	models := make([]mongo.WriteModel, 0, len(docs))
	now := time.Now().UTC()
	for _, d := range docs {
		reportID := asUint64(d["domain_id"])
		assessmentID := asUint64(d["assessment_id"])
		generatedAt := asTime(d["generated_at"])
		entry := bson.M{"assessment_id": assessmentID, "org_id": asInt64(d["org_id"]), "testee_id": asUint64(d["testee_id"]), "source_kind": "artifact", "source_id": reportID, "model_code": asString(d["scale_code"]), "risk_level": asString(d["risk_level"]), "sort_at": generatedAt, "sort_report_id": reportID, "updated_at": now}
		filter := bson.M{"assessment_id": assessmentID, "$or": bson.A{bson.M{"source_kind": "archive"}, bson.M{"sort_at": bson.M{"$lt": generatedAt}}, bson.M{"sort_at": generatedAt, "sort_report_id": bson.M{"$lt": reportID}}}}
		models = append(models, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(bson.M{"$set": entry}).SetUpsert(true))
	}
	result, err := db.Collection("report_query_catalog").BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	duplicates := duplicateWriteCount(err)
	applyBulkResult(&delta, int64(len(models)), result, duplicates)
	if err != nil && !onlyDuplicateWriteErrors(err) {
		delta.failed++
		return delta, err
	}
	return delta, nil
}

func latestArtifactsByAssessment(docs []bson.M) []bson.M {
	latest := make(map[uint64]bson.M, len(docs))
	order := make([]uint64, 0, len(docs))
	for _, doc := range docs {
		assessmentID := asUint64(doc["assessment_id"])
		current, exists := latest[assessmentID]
		if !exists {
			latest[assessmentID] = doc
			order = append(order, assessmentID)
			continue
		}
		generatedAt := asTime(doc["generated_at"])
		currentGeneratedAt := asTime(current["generated_at"])
		if generatedAt.After(currentGeneratedAt) ||
			(generatedAt.Equal(currentGeneratedAt) && asUint64(doc["domain_id"]) > asUint64(current["domain_id"])) {
			latest[assessmentID] = doc
		}
	}
	result := make([]bson.M, 0, len(latest))
	for _, assessmentID := range order {
		result = append(result, latest[assessmentID])
	}
	return result
}

func applyBulkResult(delta *summary, attempted int64, result *mongo.BulkWriteResult, duplicates int64) {
	if result != nil {
		delta.inserted += result.UpsertedCount
		delta.updated += result.ModifiedCount
	}
	delta.conflict += duplicates
	accounted := delta.inserted + delta.updated + duplicates
	if remaining := attempted - accounted; remaining > 0 {
		delta.unchanged += remaining
	}
}

func duplicateWriteCount(err error) int64 {
	var bulkErr mongo.BulkWriteException
	if !errors.As(err, &bulkErr) {
		return 0
	}
	var count int64
	for _, writeErr := range bulkErr.WriteErrors {
		if writeErr.Code == 11000 {
			count++
		}
	}
	return count
}

func onlyDuplicateWriteErrors(err error) bool {
	if err == nil {
		return true
	}
	var bulkErr mongo.BulkWriteException
	if !errors.As(err, &bulkErr) || len(bulkErr.WriteErrors) == 0 || bulkErr.WriteConcernError != nil {
		return false
	}
	for _, writeErr := range bulkErr.WriteErrors {
		if writeErr.Code != 11000 {
			return false
		}
	}
	return true
}

func loadAssessmentAssociations(ctx context.Context, db *sql.DB, docs []bson.M) (map[uint64]assessmentAssociation, error) {
	seen := make(map[uint64]struct{}, len(docs))
	for _, d := range docs {
		if id := asUint64(d["domain_id"]); id > 0 {
			seen[id] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return map[uint64]assessmentAssociation{}, nil
	}
	args := make([]any, 0, len(seen))
	for id := range seen {
		args = append(args, id)
	}
	rows, err := db.QueryContext(ctx, assessmentAssociationQuery(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[uint64]assessmentAssociation, len(seen))
	for rows.Next() {
		var assessmentID, testeeID uint64
		var orgID int64
		if err := rows.Scan(&assessmentID, &testeeID, &orgID); err != nil {
			return nil, err
		}
		result[assessmentID] = assessmentAssociation{TesteeID: testeeID, OrgID: orgID}
	}
	return result, rows.Err()
}

func assessmentAssociationQuery(count int) string {
	placeholders := make([]string, count)
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return "SELECT id, testee_id, org_id FROM assessment WHERE id IN (" + strings.Join(placeholders, ",") + ")"
}

type progressReporter struct {
	phase      string
	total      int64
	stats      *concurrentSummary
	disabled   bool
	interval   time.Duration
	started    time.Time
	done       chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
	mu         sync.Mutex
	checkpoint uint64
}

func newProgressReporter(phase string, total int64, stats *concurrentSummary, disabled bool, interval time.Duration) *progressReporter {
	return &progressReporter{phase: phase, total: total, stats: stats, disabled: disabled, interval: interval, started: time.Now(), done: make(chan struct{})}
}

func (p *progressReporter) start() {
	if p.disabled {
		return
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.render(false)
			case <-p.done:
				return
			}
		}
	}()
}

func (p *progressReporter) setCheckpoint(id uint64) {
	p.mu.Lock()
	p.checkpoint = id
	p.mu.Unlock()
}

func (p *progressReporter) stop() {
	p.stopOnce.Do(func() {
		if !p.disabled {
			close(p.done)
		}
	})
	p.wg.Wait()
}

func (p *progressReporter) finish(result phaseResult) {
	p.setCheckpoint(result.checkpoint)
	p.render(true)
}

func (p *progressReporter) abort() {
	p.stop()
	if !p.disabled {
		fmt.Println()
	}
}

func (p *progressReporter) render(final bool) {
	if p.disabled {
		return
	}
	p.mu.Lock()
	checkpoint := p.checkpoint
	p.mu.Unlock()
	line := formatProgressLine(p.phase, p.total, p.stats.snapshot(), checkpoint, time.Since(p.started))
	if final {
		fmt.Printf("\r%s\n", line)
		return
	}
	fmt.Printf("\r%s", line)
}

func formatProgressLine(phase string, total int64, s summary, checkpoint uint64, elapsed time.Duration) string {
	processed := s.scanned
	percent := float64(0)
	if total > 0 {
		percent = float64(processed) / float64(total)
		if percent > 1 {
			percent = 1
		}
	}
	const width = 30
	filled := int(percent * width)
	bar := strings.Repeat("=", filled) + strings.Repeat("-", width-filled)
	rate := float64(0)
	if elapsed > 0 {
		rate = float64(processed) / elapsed.Seconds()
	}
	eta := "--"
	if rate > 0 && total > processed {
		eta = formatDuration(time.Duration(float64(total-processed)/rate) * time.Second)
	}
	return fmt.Sprintf("%s [%s] %6.2f%% %d/%d rate=%.0f/s eta=%s checkpoint=%d ins=%d upd=%d same=%d miss=%d err=%d", phase, bar, percent*100, processed, total, rate, eta, checkpoint, s.inserted, s.updated, s.unchanged, s.missingAssessment+s.missingTestee+s.missingOrg, s.failed)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func printPhaseResult(c config, result phaseResult) {
	s := result.summary
	fmt.Printf("mode=%s phase=%s complete=%t next_after_id=%d scanned=%d inserted=%d updated=%d unchanged=%d missing_assessment=%d missing_testee=%d missing_org=%d conflict=%d failed=%d\n", map[bool]string{true: "apply", false: "dry-run"}[c.apply], result.phase, result.complete, result.checkpoint, s.scanned, s.inserted, s.updated, s.unchanged, s.missingAssessment, s.missingTestee, s.missingOrg, s.conflict, s.failed)
}

func ensureIndexes(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("report_query_catalog").Indexes().CreateMany(ctx, interpretation.ReportCatalogIndexModels())
	return err
}

func verify(ctx context.Context, db *mongo.Database, mysqlDB *sql.DB, batchSize int64) error {
	reconcileStore := interpretation.NewCatalogReconcileStore(db)
	drift, err := reconcileStore.CountDrifts(ctx, interpretation.CatalogReconcileFilter{})
	if err != nil {
		return err
	}
	cat := db.Collection("report_query_catalog")
	total, err := cat.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	missingAssessment, err := countMissingAssessmentReferences(ctx, cat, mysqlDB, batchSize)
	if err != nil {
		return err
	}
	expectedTotal, err := expectedCatalogCount(ctx, db)
	if err != nil {
		return err
	}
	missingOrg, err := cat.CountDocuments(ctx, bson.M{"org_id": bson.M{"$lte": 0}})
	if err != nil {
		return err
	}
	missingTestee, err := cat.CountDocuments(ctx, bson.M{"testee_id": bson.M{"$lte": 0}})
	if err != nil {
		return err
	}
	countMismatch := int64(0)
	if total != expectedTotal {
		countMismatch = 1
	}
	fmt.Printf("verify catalog=%d expected_catalog=%d count_mismatch=%d missing_assessment=%d missing_org=%d missing_testee=%d missing=%d dangling=%d association_mismatch=%d wrong_winner=%d\n",
		total, expectedTotal, countMismatch, missingAssessment, missingOrg, missingTestee,
		drift.Missing, drift.Dangling, drift.AssociationMismatch, drift.WrongWinner)
	if countMismatch+missingAssessment+missingOrg+missingTestee+drift.Total() > 0 {
		return fmt.Errorf("catalog reconciliation failed")
	}
	return nil
}

func countMissingAssessmentReferences(ctx context.Context, catalog *mongo.Collection, mysqlDB *sql.DB, batchSize int64) (int64, error) {
	if mysqlDB == nil {
		return 0, fmt.Errorf("mysql database is required for assessment reconciliation")
	}
	cursor, err := catalog.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"assessment_id": 1}).SetBatchSize(int32(batchSize)))
	if err != nil {
		return 0, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	ids := make([]uint64, 0, batchSize)
	var missing int64
	flush := func() error {
		if len(ids) == 0 {
			return nil
		}
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		rows, queryErr := mysqlDB.QueryContext(ctx, assessmentAssociationQuery(len(args)), args...)
		if queryErr != nil {
			return queryErr
		}
		found := 0
		for rows.Next() {
			var assessmentID, testeeID uint64
			var orgID int64
			if scanErr := rows.Scan(&assessmentID, &testeeID, &orgID); scanErr != nil {
				_ = rows.Close()
				return scanErr
			}
			found++
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			_ = rows.Close()
			return rowsErr
		}
		if closeErr := rows.Close(); closeErr != nil {
			return closeErr
		}
		missing += int64(len(ids) - found)
		ids = ids[:0]
		return nil
	}
	for cursor.Next(ctx) {
		var row struct {
			AssessmentID uint64 `bson:"assessment_id"`
		}
		if err := cursor.Decode(&row); err != nil {
			return 0, err
		}
		ids = append(ids, row.AssessmentID)
		if int64(len(ids)) == batchSize {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if err := cursor.Err(); err != nil {
		return 0, err
	}
	if err := flush(); err != nil {
		return 0, err
	}
	return missing, nil
}

func danglingSourcePipeline(sourceKind, collection string) mongo.Pipeline {
	return mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"source_kind": sourceKind}}},
		{{Key: "$lookup", Value: bson.M{
			"from": collection,
			"let":  bson.M{"source_id": "$source_id"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{"$expr": bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$domain_id", "$$source_id"}},
					bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$deleted_at", nil}}, nil}},
				}}}}},
			},
			"as": "source",
		}}},
		{{Key: "$match", Value: bson.M{"source": bson.M{"$size": 0}}}},
	}
}

func expectedCatalogCount(ctx context.Context, db *mongo.Database) (int64, error) {
	return aggregateCount(ctx, db.Collection("archived_reports"), mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"deleted_at": nil}}},
		{{Key: "$project", Value: bson.M{"assessment_id": "$domain_id"}}},
		{{Key: "$unionWith", Value: bson.M{"coll": "interpret_report_artifacts", "pipeline": mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"deleted_at": nil}}},
			{{Key: "$project", Value: bson.M{"assessment_id": 1}}},
		}}}},
		{{Key: "$group", Value: bson.M{"_id": "$assessment_id"}}},
	})
}

func aggregateCount(ctx context.Context, collection *mongo.Collection, pipeline mongo.Pipeline) (int64, error) {
	pipeline = append(pipeline, bson.D{{Key: "$count", Value: "count"}})
	cur, err := collection.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return 0, err
	}
	defer func() { _ = cur.Close(ctx) }()
	if !cur.Next(ctx) {
		return 0, cur.Err()
	}
	var row struct {
		Count int64 `bson:"count"`
	}
	if err := cur.Decode(&row); err != nil {
		return 0, err
	}
	return row.Count, nil
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

func asInt64(v any) int64   { return int64(asUint64(v)) }
func asString(v any) string { s, _ := v.(string); return s }
func asTime(v any) time.Time {
	if t, ok := v.(time.Time); ok {
		return t
	}
	return time.Time{}
}
