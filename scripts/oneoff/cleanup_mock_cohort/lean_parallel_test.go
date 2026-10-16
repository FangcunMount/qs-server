package main

import (
	"context"
	"database/sql/driver"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

type leanPoolTestReader struct {
	read func(context.Context, func(record) error) error
}

func (r *leanPoolTestReader) sqlPages(ctx context.Context, _ string, _ string, _ []any, emit func(record) error) error {
	return r.read(ctx, emit)
}
func (r *leanPoolTestReader) mongoPages(ctx context.Context, _ string, _ bson.D, emit func(record) error) error {
	return r.read(ctx, emit)
}
func (r *leanPoolTestReader) sqlOwnerPages(context.Context, string, []string, func(record) error) error {
	return errors.New("unexpected ownership query")
}
func (r *leanPoolTestReader) mongoOwnerPages(context.Context, string, []string, func(record) error) error {
	return errors.New("unexpected ownership query")
}

func TestLeanReadPoolBoundsWorkersAndSerializesOwnershipAggregation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var active, maximum, factories atomic.Int32
	var once sync.Once
	release := make(chan struct{})
	makeReader := func() graphReader {
		factories.Add(1)
		return &leanPoolTestReader{read: func(ctx context.Context, emit func(record) error) error {
			n := active.Add(1)
			defer active.Add(-1)
			for {
				old := maximum.Load()
				if n <= old || maximum.CompareAndSwap(old, n) {
					break
				}
			}
			if n == 4 {
				once.Do(func() { close(release) })
			}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return emit(record{Object: keyRoot("1")})
		}}
	}
	jobs := make([]leanReadJob, 20)
	for i := range jobs {
		jobs[i] = leanReadJob{store: "mysql", name: "testee"}
	}
	aggregated := 0
	var inAggregate atomic.Bool
	e := runLeanReadPool(ctx, 4, jobs, makeReader, nil, func(record, leanReadJob) error {
		if !inAggregate.CompareAndSwap(false, true) {
			t.Fatal("ownership writer was concurrently entered")
		}
		aggregated++
		inAggregate.Store(false)
		return nil
	})
	if e != nil || maximum.Load() != 4 || factories.Load() != 4 || active.Load() != 0 || aggregated != 20 {
		t.Fatal("bounded parallel reads did not drain into one aggregation owner", e, maximum.Load(), factories.Load(), active.Load(), aggregated)
	}
}
func TestLeanReadPoolPreservesPrimaryFailureAndJoinsWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	sentinel := errors.New("original aggregation conflict")
	var active atomic.Int32
	makeReader := func() graphReader {
		return &leanPoolTestReader{read: func(ctx context.Context, emit func(record) error) error {
			active.Add(1)
			defer active.Add(-1)
			if e := emit(record{Object: keyRoot("1")}); e != nil {
				return e
			}
			<-ctx.Done()
			return ctx.Err()
		}}
	}
	jobs := []leanReadJob{{store: "mysql", name: "testee"}, {store: "mysql", name: "testee"}, {store: "mysql", name: "testee"}}
	e := runLeanReadPool(ctx, 3, jobs, makeReader, nil, func(record, leanReadJob) error { return sentinel })
	if !errors.Is(e, sentinel) || active.Load() != 0 {
		t.Fatal("cancellation hid the primary failure or left a reader running", e, active.Load())
	}
}
func TestLeanWorkerMetadataMustMatchOriginalReaderBeforeAnyRecord(t *testing.T) {
	s, mock := graphBatchMock(t)
	s.live.workers = 1
	s.sqlShapes["assessment"] = graphSQLShape{keys: []cell{{Name: "id", Type: "uint64"}}, definition: []byte("frozen original namespace")}
	graphBatchExpectShape(mock, "assessment", "id", "bigint", "bigint unsigned")
	seen := false
	e := readLeanJobs(t.Context(), newLeanReader(s), []leanReadJob{{store: "mysql", name: "assessment", where: "x.id IN (?)", args: []any{"1"}}}, func(record, leanReadJob) error { seen = true; return nil })
	if e == nil || seen || string(s.sqlShapes["assessment"].definition) != "frozen original namespace" {
		t.Fatal("a worker replaced frozen metadata or emitted values before matching it", e)
	}
}
func TestLeanRetainedCompositeMongoIdentityNeverEntersMutationCodec(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("composite_original_key", func(mt *mtest.T) {
		id := bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "event"}, {Key: "destination", Value: "evaluation"}}
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "owned.rm_outbox", mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "message_id", Value: "event"}, {Key: "scope", Value: "org:1"}, {Key: "state", Value: "published"}}))
		s := newGraphStore(&liveStore{mongo: mt.Client, mongoDB: "owned"})
		s.mongoDefinitions["rm_outbox"] = []byte("original UUID and indexes")
		seen := false
		e := newLeanReader(s).mongoPages(mt.Context(), "rm_outbox", bson.D{}, func(r record) error {
			seen = r.Object.Key[0].Type == "bson:03"
			if _, e := mongoKey(r.Object); e == nil {
				mt.Fatal("read-only composite identity was enabled for mutation")
			}
			return nil
		})
		if e != nil || !seen {
			mt.Fatal("retained wire could not be safely identified without full payload", e)
		}
		event := mt.GetStartedEvent()
		if event == nil || event.Command.Lookup("projection").Document().Lookup("payload").Type != 0 {
			mt.Fatal("retained identity reader fetched full payload")
		}
	})
}

func TestLeanSQLDiscoveryQueriesOnlyTechnicalOutcomeColumns(t *testing.T) {
	s, mock := graphBatchMock(t)
	graphBatchExpectShape(mock, "evaluation_outcome", "id", "bigint", "bigint unsigned")
	columns := []string{"id", "org_id", "testee_id", "assessment_id", "evaluation_run_id", "committed_event_id", "committed_event_evidence"}
	mock.ExpectQuery("SELECT x.`id`,x.`org_id`,x.`testee_id`,x.`assessment_id`,x.`evaluation_run_id`,x.`committed_event_id`,x.`committed_event_evidence` FROM `evaluation_outcome` x WHERE x.assessment_id IN (?) ORDER BY x.`id` LIMIT 1000").WithArgs("101").WillReturnRows(ownerSQLRows(columns, []driver.Value{"201", "1", "44", "101", "101:1", nil, nil}))
	seen := false
	e := newLeanReader(s).sqlPages(t.Context(), "evaluation_outcome", "x.assessment_id IN (?)", []any{"101"}, func(r record) error {
		seen = true
		for _, c := range r.Columns {
			if c.Name == "payload_json" || c.Name == "report_input_json" {
				t.Fatal("discovery read business/report body")
			}
		}
		return nil
	})
	if e != nil || !seen {
		t.Fatal("technical outcome query failed", e)
	}
}
func TestLeanMongoDiscoveryUsesLeafIdentityProjection(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("independent_answersheet", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "owned.answersheets", mtest.FirstBatch, bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "domain_id", Value: int64(7)}}))
		s := newGraphStore(&liveStore{mongo: mt.Client, mongoDB: "owned"})
		s.mongoDefinitions["answersheets"] = []byte("original UUID/indexes")
		if e := newLeanReader(s).mongoPages(mt.Context(), "answersheets", bson.D{}, func(record) error { return nil }); e != nil {
			mt.Fatal(e)
		}
		projection := mt.GetStartedEvent().Command.Lookup("projection").Document()
		for _, field := range []string{"_id", "domain_id", "testee_id", "filler_id", "admission.purpose", "start_context.id", "attribution.task_id", "durable_acceptance.event_id"} {
			if projection.Lookup(field).Type == 0 {
				mt.Fatal("technical relationship omitted", field)
			}
		}
		for _, body := range []string{"answers", "admission", "start_context", "attribution", "definition", "dimensions", "suggestions"} {
			if projection.Lookup(body).Type != 0 {
				mt.Fatal("whole business/context body was requested", body)
			}
		}
	})
}

func TestLeanStatisticsSourceJobsUseOneTechnicalCursorAcrossPages(t *testing.T) {
	for _, tc := range []struct{ name, columns string }{
		{"statistics_access_fact", "x.`id`,x.`org_id`,x.`testee_id`,x.`source_type`,x.`source_ref`,x.`stat_date`,x.`clinician_id`,x.`entry_id`"},
		{"statistics_plan_fact", "x.`id`,x.`org_id`,x.`testee_id`,x.`source_type`,x.`source_ref`,x.`stat_date`,x.`plan_id`,x.`enrollment_id`,x.`task_id`"},
		{"statistics_assessment_fact", "x.`id`,x.`org_id`,x.`testee_id`,x.`filler_id`,x.`source_type`,x.`source_ref`,x.`stat_date`,x.`answersheet_id`,x.`assessment_id`,x.`outcome_id`,x.`report_id`,x.`plan_id`,x.`enrollment_id`,x.`task_id`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			s.live.workers = 4
			s.sqlShapes[tc.name] = graphSQLShape{keys: []cell{{Name: "id", Type: "uint64"}}, definition: graphBatchTestDefinition()}
			graphBatchExpectShape(mock, tc.name, "id", "bigint", "bigint unsigned")
			rows := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""), sqlmock.NewColumn("source_ref").OfType("VARCHAR", ""), sqlmock.NewColumn("testee_id").OfType("BIGINT", ""))
			for n := 1; n <= 2001; n++ {
				rows.AddRow(strconv.Itoa(n), "001", nil)
			}
			where := "x.source_type=? AND x.source_ref IN (?)"
			mock.ExpectQuery("SELECT "+tc.columns+" FROM `"+tc.name+"` x WHERE "+where+" ORDER BY x.`id`").WithArgs("fixed_source_kind", "001").WillReturnRows(rows).RowsWillBeClosed()
			count := 0
			err := readLeanJobs(t.Context(), newLeanReader(s), []leanReadJob{{store: "mysql", name: tc.name, where: where, args: []any{"fixed_source_kind", "001"}}}, func(r record, _ leanReadJob) error {
				count++
				if id, err := statisticPhysicalID(r.Object); err != nil || id != uint64(count) {
					return errors.New("physical identity or single-cursor order changed")
				}
				if source, _ := graphSQLValue(r, "source_ref"); source != "001" || string(r.Definition) != string(graphBatchTestDefinition()) {
					return errors.New("source string or original namespace was changed")
				}
				if _, exists := graphSQLValue(r, "testee_id"); exists {
					return errors.New("SQL NULL became a parent identity")
				}
				return nil
			})
			if err != nil || count != 2001 {
				t.Fatal("statistics source job repeated or lost its bounded cursor", count, err)
			}
		})
	}
}

func TestLeanStatisticsSourceFailureClosesAllFourWorkerCursors(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(4)
	mock.MatchExpectationsInOrder(false)
	query := "SELECT x.`id`,x.`org_id`,x.`testee_id`,x.`source_type`,x.`source_ref`,x.`stat_date`,x.`plan_id`,x.`enrollment_id`,x.`task_id` FROM `statistics_plan_fact` x WHERE x.source_type=? AND x.source_ref IN (?) ORDER BY x.`id`"
	jobs := make([]leanReadJob, 4)
	for i := range jobs {
		id := strconv.Itoa(i + 1)
		jobs[i] = leanReadJob{store: "mysql", name: "statistics_plan_fact", where: "x.source_type=? AND x.source_ref IN (?)", args: []any{"assessment_task", id}}
		rows := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""))
		for n := 1; n <= 2001; n++ {
			rows.AddRow(strconv.Itoa(i*10000 + n))
		}
		mock.ExpectQuery(query).WithArgs("assessment_task", id).WillReturnRows(rows).RowsWillBeClosed()
	}
	makeReader := func() graphReader {
		s := newGraphStore(&liveStore{sqlPool: db})
		s.sqlShapes["statistics_plan_fact"] = graphSQLShape{keys: []cell{{Name: "id", Type: "uint64"}}, definition: graphBatchTestDefinition()}
		return newLeanReader(s)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	primary := errors.New("original typed-parent aggregation rejected")
	allOpen := false
	err = runLeanReadPool(ctx, 4, jobs, makeReader, nil, func(record, leanReadJob) error {
		for db.Stats().InUse != 4 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}
		}
		allOpen = true
		return primary
	})
	if !errors.Is(err, primary) || !allOpen || db.Stats().InUse != 0 {
		t.Fatal("primary failure or four-worker cursor release was lost", err, allOpen, db.Stats().InUse)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLeanStatisticsCursorLeavesOtherSQLJobsPaged(t *testing.T) {
	s, mock := graphBatchMock(t)
	s.live.workers = 4
	s.sqlShapes["statistics_store_activity_fact"] = graphSQLShape{keys: []cell{{Name: "id", Type: "uint64"}}, definition: graphBatchTestDefinition()}
	graphBatchExpectShape(mock, "statistics_store_activity_fact", "id", "bigint", "bigint unsigned")
	query := "SELECT x.`id`,x.`org_id`,x.`source_type`,x.`source_ref`,x.`stat_date`,x.`fact_type`,x.`business_object_id`,x.`conducting_store_id` FROM `statistics_store_activity_fact` x WHERE x.business_object_id IN (?) ORDER BY x.`id` LIMIT 1000"
	mock.ExpectQuery(query).WithArgs("17").WillReturnRows(sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", "")).AddRow("1")).RowsWillBeClosed()
	count := 0
	err := readLeanJobs(t.Context(), newLeanReader(s), []leanReadJob{{store: "mysql", name: "statistics_store_activity_fact", where: "x.business_object_id IN (?)", args: []any{"17"}}}, func(record, leanReadJob) error { count++; return nil })
	if err != nil || count != 1 {
		t.Fatal("three-table cursor whitelist widened to another namespace", count, err)
	}
}

func TestLeanStatisticsFakeReaderKeepsExistingPageContract(t *testing.T) {
	reads := 0
	reader := &leanPoolTestReader{read: func(_ context.Context, emit func(record) error) error { reads++; return emit(record{}) }}
	err := readLeanJobs(t.Context(), reader, []leanReadJob{{store: "mysql", name: "statistics_plan_fact"}}, func(record, leanReadJob) error { return nil })
	if err != nil || reads != 1 {
		t.Fatal("non-live reader acquired a new cursor interface requirement", reads, err)
	}
}
