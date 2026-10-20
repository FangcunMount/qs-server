// Package retryschedule borrows the original QS MySQL root for future retry
// authorizations. Schema installation and connection lifecycle stay host-owned.
package retryschedule

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

const schemaVersion = 1
const readBudget = 2 * time.Second
const columns = "key_identity,input_hash,plan_hash,owner,scope,kind,subject_id,run_id,message_id,due_index_at,frozen_plan,state,revision,dispatch_evidence"

type Store struct {
	primary *sql.DB
	binding transaction.Binding
	limiter backpressure.Acquirer
}

var _ port.Store = (*Store)(nil)

// New is inert and rejects a transaction as a long-lived primary.
func New(db *gorm.DB, binding transaction.Binding, opts ...hostmysql.BaseRepositoryOptions) (*Store, error) {
	if db == nil || db.Error != nil || db.Config == nil || db.Dialector == nil || db.Name() != "mysql" || db.Statement == nil || binding.Owner != "qs.mysql" || !binding.Valid() {
		return nil, transaction.ErrBinding
	}
	if _, active := db.Statement.ConnPool.(gorm.TxCommitter); active {
		return nil, transaction.ErrTransactionRequired
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	s := &Store{primary: pool, binding: binding}
	if len(opts) > 0 {
		s.limiter = opts[0].Limiter
	}
	return s, nil
}

type reader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) validateIn(ctx context.Context, q reader, root bool) error {
	lock := ""
	if root {
		lock = " FOR SHARE"
	}
	var version int
	if err := q.QueryRowContext(ctx, "SELECT version FROM qs_retry_schedule_schema WHERE id=1"+lock).Scan(&version); err != nil {
		return errors.Join(transaction.ErrSchema, err)
	}
	if version != schemaVersion {
		return transaction.ErrSchema
	}
	var id, epoch string
	if err := q.QueryRowContext(ctx, "SELECT store_id,writer_epoch,schema_version FROM rm_tx_binding WHERE owner=?"+lock, []byte(s.binding.Owner)).Scan(&id, &epoch, &version); err != nil {
		return errors.Join(transaction.ErrBinding, err)
	}
	if version != 1 {
		return transaction.ErrSchema
	}
	if id != s.binding.StoreID || epoch != s.binding.WriterEpoch {
		return transaction.ErrBinding
	}
	return nil
}

func (s *Store) admitted(ctx context.Context) (context.Context, func(), error) {
	if s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}

func (s *Store) Validate(ctx context.Context) error {
	if _, active := hostmysql.TxFromContext(ctx); active {
		return transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, readBudget)
	defer cancel()
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.validateIn(ctx, s.primary, false)
}

// root verifies the actual SQL transaction's parent pool, including the pinned
// supported GORM prepared wrapper. It neither begins nor commits anything.
func (s *Store) root(ctx context.Context) (*sql.Tx, error) {
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx.Error != nil || tx.Statement == nil {
		return nil, transaction.ErrTransactionRequired
	}
	pool := tx.Statement.ConnPool
	if prepared, ok := pool.(*gorm.PreparedStmtTX); ok {
		pool = prepared.Tx
	}
	raw, ok := pool.(*sql.Tx)
	if !ok || raw == nil {
		return nil, transaction.ErrTransactionRequired
	}
	view := tx.WithContext(ctx)
	view.Statement.ConnPool = raw
	parent, err := view.DB()
	if err != nil || parent != s.primary {
		return nil, transaction.ErrBinding
	}
	return raw, nil
}

func (s *Store) SaveFrozen(ctx context.Context, candidate model.Frozen) (model.Frozen, error) {
	if !candidate.Valid() || candidate.Authority().Owner != s.binding.Owner {
		return model.Frozen{}, transaction.ErrBinding
	}
	tx, err := s.root(ctx)
	if err != nil {
		return model.Frozen{}, err
	}
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return model.Frozen{}, err
	}
	defer release()
	if err := s.validateIn(ctx, tx, true); err != nil {
		return model.Frozen{}, err
	}
	due, err := candidate.IndexDue(time.Microsecond)
	if err != nil {
		return model.Frozen{}, err
	}
	key, input, digest := candidate.Key(), candidate.InputHash(), candidate.Digest()
	a, in := candidate.Authority(), candidate.Message().Input()
	_, err = tx.ExecContext(ctx, `INSERT INTO qs_retry_schedule(schedule_key,key_identity,input_hash,plan_hash,owner,scope,kind,subject_id,run_id,message_id,due_index_at,frozen_plan,state,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,1) ON DUPLICATE KEY UPDATE schedule_key=schedule_key`, key[:], candidate.KeyIdentity(), input[:], digest[:], []byte(a.Owner), []byte(in.Scope), []byte(a.Kind), []byte(a.SubjectID), []byte(a.RunID), []byte(in.ID), due, candidate.Bytes(), []byte(model.Ready))
	if err != nil {
		return model.Frozen{}, err
	}
	record, err := s.readRow(tx.QueryRowContext(ctx, "SELECT "+columns+" FROM qs_retry_schedule WHERE schedule_key=? FOR UPDATE", key[:]))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Frozen{}, model.ErrConflict
	}
	if err != nil {
		return model.Frozen{}, err
	}
	if err := record.Plan.MatchInput(candidate); err != nil {
		return model.Frozen{}, err
	}
	return record.Plan, nil
}

type scanner interface{ Scan(...any) error }

func (s *Store) readRow(row scanner) (*model.Record, error) {
	var identity, input, digest, raw, proof []byte
	var owner, scope, kind, subject, run, id string
	var due time.Time
	var state model.State
	var version int64
	if err := row.Scan(&identity, &input, &digest, &owner, &scope, &kind, &subject, &run, &id, &due, &raw, &state, &version, &proof); err != nil {
		return nil, err
	}
	f, err := model.Decode(raw)
	if err != nil {
		return nil, err
	}
	a, in := f.Authority(), f.Message().Input()
	expectedInput, expectedDigest := f.InputHash(), f.Digest()
	index, err := f.IndexDue(time.Microsecond)
	if err != nil || owner != s.binding.Owner || owner != a.Owner || scope != in.Scope || kind != a.Kind || subject != a.SubjectID || run != a.RunID || id != in.ID || !due.Equal(index) || !bytes.Equal(identity, f.KeyIdentity()) || !bytes.Equal(input, expectedInput[:]) || !bytes.Equal(digest, expectedDigest[:]) {
		return nil, model.ErrInvalid
	}
	r := &model.Record{Plan: f, State: state, Version: version}
	if len(proof) > 0 {
		var e evidence.EventEvidenceV2
		if json.Unmarshal(proof, &e) != nil {
			return nil, model.ErrInvalid
		}
		canonical, err := json.Marshal(e)
		if err != nil || !bytes.Equal(canonical, proof) {
			return nil, model.ErrInvalid
		}
		r.Proof = &e
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) Find(ctx context.Context, candidate model.Frozen) (*model.Record, error) {
	if !candidate.Valid() || candidate.Authority().Owner != s.binding.Owner {
		return nil, transaction.ErrBinding
	}
	if _, active := hostmysql.TxFromContext(ctx); active {
		return nil, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, readBudget)
	defer cancel()
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.validateIn(ctx, s.primary, false); err != nil {
		return nil, err
	}
	key := candidate.Key()
	r, err := s.readRow(s.primary.QueryRowContext(ctx, "SELECT "+columns+" FROM qs_retry_schedule WHERE schedule_key=?", key[:]))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, transaction.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.Plan.MatchInput(candidate); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) Due(ctx context.Context, now time.Time, limit int) (result []model.Record, resultErr error) {
	if now.IsZero() || limit < 1 || limit > 100 {
		return nil, model.ErrInvalid
	}
	if _, active := hostmysql.TxFromContext(ctx); active {
		return nil, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, readBudget)
	defer cancel()
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.validateIn(ctx, s.primary, false); err != nil {
		return nil, err
	}
	rows, err := s.primary.QueryContext(ctx, "SELECT "+columns+" FROM qs_retry_schedule WHERE owner=? AND state=? AND due_index_at<=? ORDER BY due_index_at,schedule_key LIMIT ?", []byte(s.binding.Owner), []byte(model.Ready), now.UTC().Truncate(time.Microsecond), limit)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, rows.Close()) }()
	result = make([]model.Record, 0, limit)
	for rows.Next() {
		r, err := s.readRow(rows)
		if err != nil {
			return nil, err
		}
		if r.State != model.Ready || r.Plan.DueAt().After(now) {
			return nil, fmt.Errorf("%w: inconsistent due selector", model.ErrInvalid)
		}
		result = append(result, *r)
	}
	return result, rows.Err()
}
