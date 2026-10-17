package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type clearanceReadStore struct {
	graphBatchProvider
	mu           sync.Mutex
	active, peak int
	calls        int
	hook         func(context.Context, []object) error
}

func (s *clearanceReadStore) readExactBatch(ctx context.Context, objects []object) ([]graphReadResult, error) {
	s.mu.Lock()
	s.active++
	s.calls++
	s.peak = max(s.peak, s.active)
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active--; s.mu.Unlock() }()
	if e := s.hook(ctx, objects); e != nil {
		return nil, e
	}
	return s.graphBatchProvider.readExactBatch(ctx, objects)
}

func clearanceParallelFixture(t *testing.T, n int) (*clearanceReadStore, graphClearanceBatch) {
	t.Helper()
	names := make([]string, n)
	for i := range names {
		names[i] = "assessment_score"
	}
	s, m, dir, _ := graphParallelFixture(t, names...)
	s.rows = map[string]record{}
	m.Graph.ArchiveDirectory = dir
	return &clearanceReadStore{graphBatchProvider: s}, graphClearanceBatch{material: m}
}

func clearanceReadContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	return graphClearanceNamespaceContext(graphFrozenRuleDeleteContext(ctx, true)), cancel
}

func TestGraphClearanceTargetReadersHaveActualBoundedOverlap(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			s, b := clearanceParallelFixture(t, workers*2+1)
			entered, release := make(chan struct{}, workers), make(chan struct{})
			s.hook = func(ctx context.Context, _ []object) error {
				select {
				case entered <- struct{}{}:
				default:
				}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			ctx, cancel := clearanceReadContext(t)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- graphClearanceTargetShards(ctx, s, b, workers) }()
			for range workers {
				select {
				case <-entered:
				case <-ctx.Done():
					close(release)
					<-done
					t.Fatal("bounded target readers did not overlap")
				}
			}
			close(release)
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			if s.peak != workers || s.active != 0 || s.calls != len(b.material.Targets) {
				t.Fatalf("reader bound or drain changed: peak=%d active=%d calls=%d", s.peak, s.active, s.calls)
			}
		})
	}
}

func TestGraphClearanceLaterFailureDoesNotCancelOrMaskEarlierShard(t *testing.T) {
	s, b := clearanceParallelFixture(t, 5)
	earlier, later := errors.New("earlier shard live namespace changed"), errors.New("later shard targets remain")
	entered, laterFailed, finish := make(chan struct{}, 3), make(chan struct{}), make(chan struct{})
	s.hook = func(ctx context.Context, objects []object) error {
		entered <- struct{}{}
		if string(objects[0].Key[0].Raw) == "2" {
			close(laterFailed)
			return later
		}
		select {
		case <-finish:
			if string(objects[0].Key[0].Raw) == "1" {
				return earlier
			}
			return nil
		case <-ctx.Done():
			return errors.New("later failure canceled an earlier live read")
		}
	}
	ctx, cancel := clearanceReadContext(t)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- graphClearanceTargetShards(ctx, s, b, 3) }()
	for range 3 {
		select {
		case <-entered:
		case <-ctx.Done():
			close(finish)
			<-done
			t.Fatal("initial readers did not start")
		}
	}
	<-laterFailed
	select {
	case <-done:
		close(finish)
		t.Fatal("later failure returned before earlier reads drained")
	default:
	}
	close(finish)
	if e := <-done; e != earlier {
		t.Fatalf("original shard error order lost: %v", e)
	}
	if s.calls != 3 || s.active != 0 {
		t.Fatalf("failure dispatched a later wave or leaked reads: calls=%d active=%d", s.calls, s.active)
	}
}

func TestGraphClearanceCancellationWaitsForEveryReaderCleanup(t *testing.T) {
	s, b := clearanceParallelFixture(t, 7)
	entered, closing, finish := make(chan struct{}, 3), make(chan struct{}, 3), make(chan struct{})
	s.hook = func(ctx context.Context, _ []object) error {
		entered <- struct{}{}
		<-ctx.Done()
		closing <- struct{}{}
		<-finish
		return ctx.Err()
	}
	ctx, cancel := clearanceReadContext(t)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- graphClearanceTargetShards(ctx, s, b, 3) }()
	for range 3 {
		select {
		case <-entered:
		case <-ctx.Done():
			close(finish)
			<-done
			t.Fatal("initial readers did not start")
		}
	}
	cancel()
	for range 3 {
		select {
		case <-closing:
		case <-time.After(5 * time.Second):
			close(finish)
			<-done
			t.Fatal("cancellation did not reach every in-flight reader")
		}
	}
	select {
	case <-done:
		close(finish)
		t.Fatal("caller could release resources before in-flight cleanup")
	default:
	}
	close(finish)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal("external cancellation lost:", e)
	}
	if s.active != 0 || s.calls != 3 {
		t.Fatalf("canceled readers leaked or dispatched more work: active=%d calls=%d", s.active, s.calls)
	}
}

func TestGraphClearanceEveryShardRereadsActualSQLMetadata(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			s, mock := graphBatchMock(t)
			mock.MatchExpectationsInOrder(false)
			dir := privateTestDir(t)
			m := graphArchive{Namespaces: map[string][]byte{}, Counts: map[string]int64{}}
			for _, id := range []string{"1", "2"} {
				if e := archiveGraphPage(dir, &m, []record{graphBatchRow(id, "original")}, 70, false); e != nil {
					t.Fatal(e)
				}
				if changed {
					mock.ExpectQuery("SELECT k.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.COLUMNS c ON c.TABLE_SCHEMA=k.TABLE_SCHEMA AND c.TABLE_NAME=k.TABLE_NAME AND c.COLUMN_NAME=k.COLUMN_NAME WHERE k.TABLE_SCHEMA=DATABASE() AND k.TABLE_NAME=? AND k.CONSTRAINT_NAME='PRIMARY' ORDER BY k.COLUMN_NAME").WithArgs("assessment_score").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME", "DATA_TYPE", "COLUMN_TYPE"}).AddRow("id", "bigint", "bigint unsigned"))
					mock.ExpectQuery("SHOW CREATE TABLE `assessment_score`").WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow("assessment_score", "CREATE TABLE changed_live_DDL"))
					mock.ExpectQuery("SELECT @@session.time_zone,@@session.character_set_results,@@session.sql_mode").WillReturnRows(sqlmock.NewRows([]string{"zone", "charset", "mode"}).AddRow("+00:00", "utf8mb4", "STRICT_TRANS_TABLES"))
				} else {
					graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
				}
				mock.ExpectQuery("SELECT x.`id` FROM `assessment_score` x WHERE (`id` = ?)").WithArgs([]byte(id)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			}
			m.Graph.ArchiveDirectory = dir
			// A cached original cannot substitute for either shard's actual metadata.
			s.sqlShapes["assessment_score"] = graphSQLShape{definition: graphBatchTestDefinition()}
			ctx, cancel := clearanceReadContext(t)
			defer cancel()
			e := graphClearanceTargetShards(ctx, s, graphClearanceBatch{material: m}, 2)
			if changed != (e != nil) {
				t.Fatalf("fresh metadata admission mismatch: changed=%t err=%v", changed, e)
			}
		})
	}
}

func TestGraphClearanceRejectsUnboundedReadersBeforeReading(t *testing.T) {
	s, b := clearanceParallelFixture(t, 1)
	for _, workers := range []int{0, 5} {
		if e := graphClearanceTargetShards(t.Context(), s, b, workers); e == nil {
			t.Fatal("unbounded target readers accepted")
		}
	}
	if s.calls != 0 {
		t.Fatal("invalid reader count read current data")
	}
}
