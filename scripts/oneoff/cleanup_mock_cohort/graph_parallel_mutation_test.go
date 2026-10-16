package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

type graphParallelStore struct {
	mu          sync.Mutex
	rows        map[string]record
	definitions map[string][]byte
	calls       []string
	active      int
	peak        int
	reads       int
	activeNS    map[string]int
	crossed     bool
	hook        func(context.Context, []record) error
}

func (s *graphParallelStore) readExactBatch(_ context.Context, objects []object) ([]graphReadResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	out := make([]graphReadResult, len(objects))
	for i, o := range objects {
		r, exists := s.rows[objectID(o)]
		if !exists {
			r = record{Object: o, Definition: s.definitions[o.Store+"/"+o.Name]}
		}
		out[i] = graphReadResult{Record: r, Exists: exists}
	}
	return out, nil
}
func (*graphParallelStore) preflightExactBatch(context.Context, []record, bool) error {
	return errors.New("scheduler must not repeat the caller's global preflight")
}
func (s *graphParallelStore) mutateExactBatch(ctx context.Context, records []record, restore bool) error {
	ns := records[0].Object.Store + "/" + records[0].Object.Name
	id := objectID(records[0].Object)
	s.mu.Lock()
	for other, n := range s.activeNS {
		if other != ns && n > 0 {
			s.crossed = true
		}
	}
	s.activeNS[ns]++
	s.active++
	if s.active > s.peak {
		s.peak = s.active
	}
	s.calls = append(s.calls, id)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.active--
		s.activeNS[ns]--
		s.mu.Unlock()
	}()
	if s.hook != nil {
		if err := s.hook(ctx, records); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range records {
		if restore {
			s.rows[objectID(r.Object)] = r
		} else {
			delete(s.rows, objectID(r.Object))
		}
	}
	return nil
}

func graphParallelFixture(t *testing.T, names ...string) (*graphParallelStore, graphArchive, string, *graphJournal) {
	t.Helper()
	dir := privateTestDir(t)
	m := graphArchive{Namespaces: map[string][]byte{}, Counts: map[string]int64{}}
	s := &graphParallelStore{rows: map[string]record{}, definitions: map[string][]byte{}, activeNS: map[string]int{}}
	for i, name := range names {
		r := graphSQLFixture(name, fmt.Sprint(i+1), map[string]string{"testee_id": "1"})
		s.rows[objectID(r.Object)] = r
		s.definitions["mysql/"+name] = r.Definition
		if err := archiveGraphPage(dir, &m, []record{r}, graphSQLTables[name], false); err != nil {
			t.Fatal(err)
		}
	}
	j, err := openGraphJournal(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.close() })
	return s, m, dir, j
}

func graphParallelJournal(dir string) ([]journalEntry, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "graph-journal.jsonl"))
	if err != nil {
		return nil, err
	}
	// A different dispatch may be appending. Every already-dispatched intent
	// must be in the complete, hash-linked prefix visible to this worker.
	end := bytes.LastIndexByte(raw, '\n')
	if end < 0 {
		return nil, errors.New("worker entered without a durable complete intent")
	}
	return readJournal(bytes.NewReader(raw[:end+1]))
}

func TestGraphParallelMutationBoundsConcurrencyAndFsyncsPhaseBarriers(t *testing.T) {
	for _, restore := range []bool{false, true} {
		t.Run(fmt.Sprint(restore), func(t *testing.T) {
			s, m, dir, j := graphParallelFixture(t, "assessment_score", "assessment_score", "assessment_score", "runtime_checkpoint", "evaluation_outcome", "evaluation_outcome", "testee", "testee")
			ordered := graphOrderedShards(m, restore)
			if restore {
				s.rows = map[string]record{}
			}
			entered, release := make(chan string, len(ordered)), make(chan struct{})
			s.hook = func(_ context.Context, records []record) error {
				id := objectID(records[0].Object)
				entries, err := graphParallelJournal(dir)
				if err != nil {
					return err
				}
				found := false
				for _, ref := range ordered {
					rows, err := readGraphArchivePage(dir, m, ref)
					if err != nil {
						return err
					}
					if objectID(rows[0].Object) == id {
						for _, entry := range entries {
							found = found || entry.Chunk == graphChunkID(ref) && entry.State == "intent"
						}
						break
					}
					// A different dependency phase must have fsynced its terminal.
					if ref.Name != records[0].Object.Name {
						terminal := false
						for _, entry := range entries {
							terminal = terminal || entry.Chunk == graphChunkID(ref) && entry.State == "committed"
						}
						if !terminal {
							return errors.New("later namespace entered before a previous terminal was persisted")
						}
					}
				}
				if !found {
					return errors.New("transaction entered before its exact durable intent")
				}
				entered <- id
				<-release
				return nil
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- graphMutateOrdered(ctx, s, m, dir, j, strings.Repeat("a", 64), restore, func() error { return nil }, 2)
			}()
			wantPeak := 2
			if restore {
				wantPeak = 1 // SQL missing-PK restore phases must not overlap.
			}
			for range wantPeak {
				select {
				case <-entered:
				case <-ctx.Done():
					close(release)
					t.Fatal("required phase workers did not enter")
				}
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if s.peak != wantPeak || s.crossed || len(s.calls) != len(ordered) || s.reads != 0 || j.seq != len(ordered)*2 {
				t.Fatalf("peak=%d crossed=%t calls=%d reads=%d journal=%d", s.peak, s.crossed, len(s.calls), s.reads, j.seq)
			}
		})
	}
}

func TestGraphParallelMutationSerializesOnlyMySQLRestore(t *testing.T) {
	for _, tc := range []struct {
		name, store string
		restore     bool
		peak        int
	}{
		{"mysql_restore", "mysql", true, 1},
		{"mysql_delete", "mysql", false, 2},
		{"mongo_restore", "mongo", true, 2},
		{"mongo_delete", "mongo", false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m, dir, j := graphParallelFixture(t)
			for i := range 3 {
				r := graphSQLFixture("assessment_score", fmt.Sprint(i+1), map[string]string{"testee_id": "1"})
				rank := graphSQLTables[r.Object.Name]
				if tc.store == "mongo" {
					r = graphMongoFixture(t, "answersheets", bson.D{{Key: "domain_id", Value: int64(i + 1)}})
					rank = graphMongoTables[r.Object.Name]
				}
				s.definitions[r.Object.Store+"/"+r.Object.Name] = r.Definition
				if !tc.restore {
					s.rows[objectID(r.Object)] = r
				}
				if err := archiveGraphPage(dir, &m, []record{r}, rank, false); err != nil {
					t.Fatal(err)
				}
			}
			entered, release := make(chan struct{}, len(m.Targets)), make(chan struct{})
			s.hook = func(ctx context.Context, _ []record) error {
				entered <- struct{}{}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- graphMutateOrdered(ctx, s, m, dir, j, strings.Repeat("a", 64), tc.restore, func() error { return nil }, 2)
			}()
			for range tc.peak {
				select {
				case <-entered:
				case <-ctx.Done():
					close(release)
					<-done
					t.Fatal("required mutation concurrency did not enter")
				}
			}
			overlapped := false
			if tc.peak == 1 {
				// Keep the first missing-PK restore transaction in flight. The next
				// chunk must not enter while it can hold an overlapping gap lock.
				select {
				case <-entered:
					overlapped = true
				case <-time.After(100 * time.Millisecond):
				}
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if overlapped || s.peak != tc.peak || len(s.calls) != 3 || s.active != 0 || j.seq != 6 {
				t.Fatalf("restore-only SQL limit was not preserved: overlap=%t peak=%d calls=%d active=%d journal=%d", overlapped, s.peak, len(s.calls), s.active, j.seq)
			}
		})
	}
}

func TestGraphParallelMutationFirstErrorDrainsAndPersistsEveryInflightOutcome(t *testing.T) {
	s, m, dir, j := graphParallelFixture(t, "assessment_score", "assessment_score", "assessment_score", "assessment_score", "testee")
	primary := errors.New("original commit response lost")
	secondary := errors.New("second transaction result unproven")
	entered, failNow := make(chan struct{}, 3), make(chan struct{})
	s.hook = func(ctx context.Context, records []record) error {
		id, _ := graphSQLValue(records[0], "id")
		entered <- struct{}{}
		if id == "1" {
			<-failNow
			return primary
		}
		<-ctx.Done()
		if id == "3" {
			return secondary
		}
		return nil // An already-issued transaction can still return an actual ACK.
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- graphMutateOrdered(ctx, s, m, dir, j, strings.Repeat("a", 64), false, func() error { return nil }, 3)
	}()
	for range 3 {
		select {
		case <-entered:
		case <-ctx.Done():
			close(failNow)
			t.Fatal("three transactions did not enter")
		}
	}
	close(failNow)
	err := <-done
	if !errors.Is(err, primary) || !errors.Is(err, secondary) || len(s.calls) != 3 || s.active != 0 || j.seq != 6 {
		t.Fatalf("lost outcome or dispatched after failure: calls=%d active=%d journal=%d error=%v", len(s.calls), s.active, j.seq, err)
	}
	for i, state := range []string{"unknown", "committed", "unknown"} {
		if j.latest[graphChunkID(m.Targets[i])].State != state {
			t.Fatalf("chunk %d terminal is not %s", i, state)
		}
	}
	if err := j.close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openGraphJournal(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.close() }()
	if err = graphMutateOrdered(ctx, s, m, dir, reopened, strings.Repeat("b", 64), false, func() error { return nil }, 3); err == nil || len(s.calls) != 3 || reopened.seq != 6 {
		t.Fatal("a fresh approval replayed the original unknown transaction:", err)
	}
}

func TestGraphParallelMutationKnownStateCannotBeReclassifiedAsFresh(t *testing.T) {
	for _, tc := range []struct {
		name, action, state string
		restore, exists     bool
		fail, mutate        bool
	}{
		{"ack_apply_absent_skip", "apply", "committed", false, false, false, false},
		{"ack_apply_reappeared_reject", "apply", "committed", false, true, true, false},
		{"ack_restore_original_skip", "restore", "committed", true, true, false, false},
		{"ack_restore_disappeared_reject", "restore", "committed", true, false, true, false},
		{"reconciled_absent_skip", "apply", "reconciled_absent", false, false, false, false},
		{"reconciled_original_skip", "restore", "reconciled_original", true, true, false, false},
		{"ack_apply_then_restore", "apply", "committed", true, false, false, true},
		{"ack_restore_then_apply", "restore", "committed", false, true, false, true},
		{"unknown_never_replayed", "apply", "unknown", false, true, true, false},
		{"intent_never_replayed", "restore", "intent", true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m, dir, j := graphParallelFixture(t, "assessment_score")
			if !tc.exists {
				s.rows = map[string]record{}
			}
			if err := j.append(journalEntry{Action: tc.action, Chunk: graphChunkID(m.Targets[0]), ApprovalSHA: strings.Repeat("a", 64), State: tc.state}); err != nil {
				t.Fatal(err)
			}
			err := graphMutateOrdered(t.Context(), s, m, dir, j, strings.Repeat("b", 64), tc.restore, func() error { return nil }, 4)
			if (err != nil) != tc.fail || (len(s.calls) == 1) != tc.mutate || len(s.calls) > 1 {
				t.Fatalf("known state was weakened: calls=%d error=%v", len(s.calls), err)
			}
		})
	}
}

func TestGraphParallelMutationArchiveFailureStillWaitsForOriginalACK(t *testing.T) {
	s, m, dir, j := graphParallelFixture(t, "assessment_score", "assessment_score", "testee")
	if err := os.WriteFile(filepath.Join(dir, m.Targets[1].File), []byte("corrupt-original"), 0600); err != nil {
		t.Fatal(err)
	}
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s.hook = func(ctx context.Context, _ []record) error {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
		return nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	authorityCalls := 0
	go func() {
		done <- graphMutateOrdered(ctx, s, m, dir, j, strings.Repeat("a", 64), false, func() error {
			authorityCalls++
			if authorityCalls == 2 {
				// Ensure the first transaction is in flight before the original
				// second archive is read and rejected, cancelling that first job.
				select {
				case <-entered:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}, 2)
	}()
	<-entered
	<-cancelled
	// The archive error cancels the worker, but the real result must still
	// arrive before the coordinator can return or advance to another phase.
	select {
	case <-done:
		t.Fatal("returned without waiting for the original transaction outcome")
	default:
	}
	close(release)
	if err := <-done; err == nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || len(s.calls) != 1 || s.active != 0 || j.latest[graphChunkID(m.Targets[0])].State != "committed" {
		t.Fatal("cancelled coordinator lost a real in-flight ACK:", err)
	}
}
