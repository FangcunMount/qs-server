package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"go.mongodb.org/mongo-driver/bson"
)

type lifecycleSQLAuditFixture struct {
	forward, reverse []uint64
	stall, unknown   bool
}

func (*lifecycleSQLAuditFixture) BusinessUpperBound(context.Context) (uint64, error) { return 7, nil }
func (*lifecycleSQLAuditFixture) OutboxUpperBound(context.Context) (uint64, error)   { return 8, nil }
func (s *lifecycleSQLAuditFixture) ReadBatchTo(_ context.Context, after, upper uint64, limit int) (evaluationconsistency.Batch, error) {
	s.forward = append(s.forward, after)
	if upper != 7 || limit != 100 {
		return evaluationconsistency.Batch{}, errors.New("changed frozen bound or limit")
	}
	if s.unknown {
		return evaluationconsistency.Batch{}, errors.New("read result unknown")
	}
	if after == 0 {
		return evaluationconsistency.Batch{Items: []evaluationconsistency.AssessmentEvidence{{AssessmentID: 7, Status: "submitted"}}, NextCursor: 7, CycleComplete: true}, nil
	}
	if s.stall {
		return evaluationconsistency.Batch{Items: []evaluationconsistency.AssessmentEvidence{{AssessmentID: 7}}, NextCursor: 7}, nil
	}
	return evaluationconsistency.Batch{CycleComplete: true}, nil
}
func (s *lifecycleSQLAuditFixture) ReadOutboxBatch(_ context.Context, after, upper uint64, limit int) (evaluationconsistency.ReverseBatch, error) {
	s.reverse = append(s.reverse, after)
	if upper != 8 || limit != 100 {
		return evaluationconsistency.ReverseBatch{}, errors.New("changed reverse frozen bound or limit")
	}
	if after == 0 {
		return evaluationconsistency.ReverseBatch{Scanned: 1, NextCursor: 8, CycleComplete: true}, nil
	}
	return evaluationconsistency.ReverseBatch{CycleComplete: true}, nil
}

func TestRuntimeStandardSQLRequiresBothRealEmptyPages(t *testing.T) {
	s := new(lifecycleSQLAuditFixture)
	var counts lifecycleRuntimeLedgerCounts
	if e := lifecycleScanStandardSQL(t.Context(), s, &counts); e != nil {
		t.Fatal(e)
	}
	if counts.SQLForward != 1 || counts.SQLReverse != 1 || len(s.forward) != 2 || s.forward[1] != 7 || len(s.reverse) != 2 || s.reverse[1] != 8 {
		t.Fatal("exhausted marker replaced actual bidirectional EOF", counts, s)
	}
	for _, s := range []*lifecycleSQLAuditFixture{{stall: true}, {unknown: true}} {
		if lifecycleScanStandardSQL(t.Context(), s, new(lifecycleRuntimeLedgerCounts)) == nil {
			t.Fatal("incomplete/stalled observation succeeded")
		}
	}
}

type lifecycleMongoAuditFixture struct {
	token []byte
	pages map[appaudit.Phase]int
	stall bool
}

func (*lifecycleMongoAuditFixture) UpperBound(context.Context, appaudit.Phase, time.Duration) (uint64, error) {
	return 9, nil
}
func (s *lifecycleMongoAuditFixture) OutboxUpperBound(context.Context, time.Duration) ([]byte, error) {
	return append([]byte(nil), s.token...), nil
}
func (s *lifecycleMongoAuditFixture) ScanBatch(_ context.Context, r appaudit.BatchRequest) (appaudit.BatchResult, error) {
	s.pages[r.Phase]++
	if r.Limit != 200 || r.MaxTime != 3*time.Second || !bytes.Equal(r.OutboxUpperBound, s.token) {
		return appaudit.BatchResult{}, errors.New("changed bounded scanner contract")
	}
	if r.Phase == appaudit.PhaseOutboxAnswerSheet {
		if len(r.OutboxCursor) == 0 || s.stall {
			return appaudit.BatchResult{Scanned: 1, NextOutboxCursor: append([]byte(nil), s.token...), Exhausted: true}, nil
		}
		if !bytes.Equal(r.OutboxCursor, s.token) {
			return appaudit.BatchResult{}, errors.New("real BSON token converted or rebuilt")
		}
	} else {
		if r.UpperBound != 9 {
			return appaudit.BatchResult{}, errors.New("numeric bound changed")
		}
		if r.AfterID == 0 {
			return appaudit.BatchResult{Scanned: 1, NextID: 9, Exhausted: true, EvidenceClasses: map[string]int64{"unverifiable": 1}}, nil
		}
	}
	return appaudit.BatchResult{Exhausted: true}, nil
}

func TestRuntimeStandardMongoPreservesOrderedBSONAndEveryPhaseEOF(t *testing.T) {
	token, e := bson.Marshal(bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "original-id"}, {Key: "destination", Value: "qs-worker"}})
	if e != nil {
		t.Fatal(e)
	}
	s := &lifecycleMongoAuditFixture{token: token, pages: make(map[appaudit.Phase]int)}
	var counts lifecycleRuntimeLedgerCounts
	if e = lifecycleScanStandardMongo(t.Context(), s, &counts); e != nil {
		t.Fatal(e)
	}
	if len(s.pages) != 7 || counts.MongoForward != 6 || counts.MongoReverse != 1 || counts.MongoUnverifiable != 6 || counts.MongoStandardReferences != 0 {
		t.Fatal("historical conclusions became standard references or phase disappeared", counts, s.pages)
	}
	for phase, n := range s.pages {
		if n != 2 {
			t.Fatal("phase lacks actual final EOF", phase, n)
		}
	}
	s.stall = true
	s.pages = make(map[appaudit.Phase]int)
	if lifecycleScanStandardMongo(t.Context(), s, new(lifecycleRuntimeLedgerCounts)) == nil {
		t.Fatal("unchanged BSON cursor accepted")
	}
}

func TestRuntimeLedgerUnknownCannotProduceSerializableAcceptance(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if lifecycleScanStandardSQL(ctx, new(lifecycleSQLAuditFixture), new(lifecycleRuntimeLedgerCounts)) == nil {
		t.Fatal("canceled scan accepted")
	}
	if (*lifecycleFixedHost)(nil).observeRuntimeLedgers(t.Context(), lifecycleRequest{}) == nil {
		t.Fatal("no native owners accepted")
	}
	if _, e := json.Marshal(new(lifecycleRuntimeLedgerObservation)); e == nil {
		t.Fatal("counter DTO became accepted capability")
	}
	n := uint64(math.MaxUint64)
	if lifecycleAddLedgerCount(&n, 1) == nil || n != math.MaxUint64 {
		t.Fatal("counter overflow silently wrapped")
	}
}

func TestRuntimeCurrentMQIncludesLedgerOnlyOrganizationsAndChecksBeforeCounting(t *testing.T) {
	for _, scenario := range []string{"both_directions", "invalid_organization", "ledger_only_orphan"} {
		t.Run(scenario, func(t *testing.T) {
			db, m, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = db.Close() }()
			m.ExpectBegin()
			tx, e := db.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
			if e != nil {
				t.Fatal(e)
			}
			invalid := 0
			if scenario == "invalid_organization" {
				invalid = 1
			}
			m.ExpectQuery(`SELECT COALESCE\(MAX\(organization_id\),0\)`).WillReturnRows(sqlmock.NewRows([]string{"upper", "invalid"}).AddRow(2, invalid))
			if invalid == 0 {
				m.ExpectQuery("SELECT organization_id FROM").WithArgs(int64(0), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"org"}).AddRow(1).AddRow(2))
				for _, org := range []int64{1, 2} {
					orphan := scenario == "ledger_only_orphan" && org == 2
					m.ExpectQuery("SELECT EXISTS").WithArgs(org, org, org, org).WillReturnRows(sqlmock.NewRows([]string{"invalid"}).AddRow(orphan))
					if !orphan {
						m.ExpectQuery(`SELECT COUNT\(\*\)`).WithArgs(org).WillReturnRows(sqlmock.NewRows([]string{"requests", "pending", "attempts"}).AddRow(1, 3, 15))
					}
				}
				if scenario == "both_directions" {
					m.ExpectQuery("SELECT organization_id FROM").WithArgs(int64(2), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"org"}))
				}
			}
			var counts lifecycleRuntimeLedgerCounts
			e = lifecycleReadCurrentMQ(t.Context(), tx, &counts)
			if scenario == "both_directions" && (e != nil || counts.Organizations != 2 || counts.Requests != 2 || counts.CommandsPending != 6 || counts.CommandAttempts != 30) {
				t.Fatal(counts, e)
			}
			if scenario != "both_directions" && e == nil {
				t.Fatal("invalid/orphan ledger hidden as zero backlog", counts)
			}
			m.ExpectRollback()
			if e = tx.Rollback(); e != nil {
				t.Fatal(e)
			}
			if e = m.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
