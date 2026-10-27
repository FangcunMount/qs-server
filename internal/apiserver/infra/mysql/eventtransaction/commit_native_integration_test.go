//go:build integration && m3_evaluation_integration

package eventtransaction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	outcomerepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This sender simulates confirmed half preparation to isolate the real MySQL
// adapter. It is NOT Broker, packet-loss or process-exit evidence.
type nativeSender struct {
	store       *txmysql.Store
	halfUnknown bool
}

func (s *nativeSender) Reserve(ctx context.Context, q transaction.Request, p transaction.FrozenPlan) (transaction.Reservation, error) {
	return s.store.Reserve(ctx, q, p, transaction.ReserveOptions{Lease: time.Minute})
}
func (s *nativeSender) Execute(ctx context.Context, e transaction.Execution, local transaction.LocalExecutor) (transaction.Result, error) {
	r := transaction.InitialResult(e)
	if err := s.store.BeginPrepare(ctx, e); err != nil {
		return r, err
	}
	if s.halfUnknown {
		return r, errors.New("simulated half unknown; no Broker used")
	}
	r.Local, _ = local(ctx, e)
	return r, nil
}

func TestM3MySQLOriginalEventRoot(t *testing.T) {
	config, err := driver.ParseDSN(os.Getenv("QS_M3_EVALUATION_DSN"))
	require.NoError(t, err)
	require.Equal(t, "tcp", config.Net)
	host, _, err := net.SplitHostPort(config.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback(), "owned loopback fixture only")
	require.Empty(t, config.DBName, "test creates and drops its own schema")
	config.MultiStatements, config.ParseTime = true, true
	admin, err := sql.Open("mysql", config.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	name := fmt.Sprintf("qs_m3_evaluation_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
		require.NoError(t, err)
	})
	config.DBName = name
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "pkg", "migration", "migrations", "mysql", "000106_transaction_messaging.up.sql"))
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), string(schema))
	require.NoError(t, err)
	binding := transaction.Binding{Owner: owner, StoreID: "owned-original-primary", WriterEpoch: "m3-1"}
	_, err = pool.ExecContext(t.Context(), `INSERT INTO rm_tx_binding VALUES(?,?,?,1)`, []byte(binding.Owner), []byte(binding.StoreID), []byte(binding.WriterEpoch))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&outcomerepo.EvaluationOutcomePO{}))
	_, err = pool.ExecContext(t.Context(), `CREATE TABLE root_fact(id VARBINARY(128) PRIMARY KEY, value BLOB NOT NULL) ENGINE=InnoDB`)
	require.NoError(t, err)
	store, err := txmysql.New(pool, binding)
	require.NoError(t, err)
	c := &Committer{primary: pool, store: store, uow: hostmysql.NewUnitOfWork(db), resolver: fixtureResolver{}, topics: map[string]bool{"qs.evaluation.lifecycle": true}}
	physical, err := transport.PhysicalTopicV2("qs.evaluation.lifecycle", transport.TransactionMessageType, owner, revision)
	require.NoError(t, err)
	c.routes, err = transport.NewRouteResolver([]transport.RouteBinding{{Topic: "qs.evaluation.lifecycle", PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: revision}}, nil)
	require.NoError(t, err)
	sender := &nativeSender{store: store}
	c.sender = sender
	t.Run("lookup-without-inventing-input-hash", func(t *testing.T) {
		q := fixtureRequest("lookup-original")
		r, err := c.Reserve(t.Context(), q)
		require.NoError(t, err)
		found, err := c.Find(t.Context(), r.Execution.Plan.Input().Scope, q.Key)
		require.NoError(t, err)
		require.False(t, found.Acquired)
		require.Equal(t, r.Execution.Plan.Digest(), found.Execution.Plan.Digest())
		require.Equal(t, r.Execution.References(), found.Execution.References())
		_, err = c.Find(t.Context(), "org:8", q.Key)
		require.ErrorIs(t, err, transaction.ErrNotFound)
		request := transaction.RequestFor(r.Execution.Plan)
		key := request.KeyDigest()
		_, err = pool.ExecContext(t.Context(), `UPDATE rm_tx_request SET idempotency_key=? WHERE request_key=?`, []byte("different-original-key"), key[:])
		require.NoError(t, err)
		_, err = c.Find(t.Context(), request.Scope, request.Key)
		require.ErrorIs(t, err, transaction.ErrRequestIdentity)
	})
	for _, tc := range []struct {
		name           string
		abort, unknown bool
	}{{"commit", false, false}, {"rollback", true, false}, {"half-unknown", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			sender.halfUnknown = tc.unknown
			r, err := c.Reserve(t.Context(), fixtureRequest(tc.name))
			require.NoError(t, err)
			require.True(t, r.Acquired)
			calls := 0
			result, commitErr := c.Commit(t.Context(), r.Execution, func(ctx context.Context) error {
				calls++
				tx, err := hostmysql.RequireTx(ctx)
				require.NoError(t, err)
				require.NoError(t, tx.Exec(`INSERT INTO root_fact VALUES(?,?)`, tc.name, []byte{0, 255, 3}).Error)
				if tc.abort {
					return errors.New("business rejects original root")
				}
				return nil
			})
			var count int
			require.NoError(t, pool.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM root_fact WHERE id=?`, tc.name).Scan(&count))
			if tc.abort || tc.unknown {
				require.ErrorIs(t, commitErr, port.ErrUnresolved)
				require.NotEqual(t, transaction.LocalCommitted, result.Local)
				require.Zero(t, count)
				require.Equal(t, transaction.Unknown, (transaction.StoreChecker{Store: store}).Check(t.Context(), r.Execution.References()[0]))
			} else {
				require.NoError(t, commitErr)
				require.Equal(t, transaction.LocalCommitted, result.Local)
				require.Equal(t, 1, count)
				require.Equal(t, transaction.Commit, (transaction.StoreChecker{Store: store}).Check(t.Context(), r.Execution.References()[0]))
			}
			if tc.unknown {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			duplicate, err := c.Reserve(t.Context(), fixtureRequest(tc.name))
			require.NoError(t, err)
			require.False(t, duplicate.Acquired)
			require.Equal(t, r.Execution.Plan.Digest(), duplicate.Execution.Plan.Digest())
		})
	}
	t.Run("actual-json-column-and-original-witness", func(t *testing.T) {
		sender.halfUnknown = false
		input := domainoutcome.NewRecordInput{ID: meta.FromUint64(901), OrgID: 7, AssessmentID: meta.FromUint64(42), TesteeID: 8, RunID: "original-run", Model: domainoutcome.ModelIdentity{Kind: modelcatalog.KindScale, Code: "fixture"}, Runtime: domainoutcome.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindScoreRange}, SchemaVersion: 2, EvaluatedAt: time.Now().UTC().Truncate(time.Millisecond), Payload: []byte("{ \"result\": \"<>&\", \"n\":1.00 }\n"), ReportInput: []byte("{\n \"frozen\":true\n}\n")}
		q := fixtureRequest("json-column")
		frozen := input
		frozen.Payload, frozen.ReportInput = nil, nil
		q.Artifact, err = json.Marshal(struct {
			Record               domainoutcome.NewRecordInput
			Payload, ReportInput []byte
		}{frozen, input.Payload, input.ReportInput})
		require.NoError(t, err)
		q.Events = []event.DomainEvent{event.New("evaluation.outcome.committed", "Assessment", "42", eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 8, OutcomeID: "901", EvaluationRunID: input.RunID, CommittedAt: input.EvaluatedAt})}
		r, err := c.Reserve(t.Context(), q)
		require.NoError(t, err)
		input.CommittedTransactionEvidence, err = evidence.NewTransactional(standard.ReferenceFromMessage(r.Execution.Plan.Input().Branches[0].Message), r.Execution.References()[0], domainoutcome.BusinessBindingSHA256(input), input.EvaluatedAt)
		require.NoError(t, err)
		record, err := domainoutcome.NewRecord(input)
		require.NoError(t, err)
		repo := outcomerepo.NewOutcomeRepository(db)
		result, err := c.Commit(t.Context(), r.Execution, func(ctx context.Context) error { return repo.Save(ctx, record) })
		require.NoError(t, err)
		require.Equal(t, transaction.LocalCommitted, result.Local)
		stored, err := repo.FindByID(t.Context(), record.ID())
		require.NoError(t, err)
		require.Equal(t, []byte(input.Payload), []byte(stored.Payload()))
		require.Equal(t, []byte(input.ReportInput), []byte(stored.ReportInput()))
		require.Equal(t, input.CommittedTransactionEvidence.TransactionReference, stored.CommittedTransactionEvidence().TransactionReference)
		var version int
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT JSON_EXTRACT(committed_event_evidence,'$.version') FROM evaluation_outcome WHERE id=901").Scan(&version))
		require.Equal(t, 2, version)
		require.Equal(t, transaction.LocalCommitted, c.Confirm(t.Context(), r.Execution))
	})
}
