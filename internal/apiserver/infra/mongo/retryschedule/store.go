// Package retryschedule persists future retry authorizations in the original
// Mongo session. It never starts a transaction, installs schema or publishes.
package retryschedule

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

const collection = "qs_retry_schedule"
const readBudget = 2 * time.Second

type Store struct {
	db      *mongo.Database
	primary *mongo.Database
	binding transaction.Binding
	limiter backpressure.Acquirer
}

var _ port.Store = (*Store)(nil)

func New(db *mongo.Database, binding transaction.Binding, opts ...base.BaseRepositoryOptions) (*Store, error) {
	if db == nil || db.Client() == nil || db.Name() == "" || binding.Owner != "qs.mongo" || !binding.Valid() {
		return nil, transaction.ErrBinding
	}
	primary := db.Client().Database(db.Name(), options.Database().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority()).SetWriteConcern(writeconcern.Majority()))
	s := &Store{db: db, primary: primary, binding: binding}
	if len(opts) > 0 {
		s.limiter = opts[0].Limiter
	}
	return s, nil
}

func (s *Store) admitted(ctx context.Context) (context.Context, func(), error) {
	// The original host Mongo runner owns its slot for the entire session.
	if s.limiter == nil || mongo.SessionFromContext(ctx) != nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}

func (s *Store) validateIn(ctx context.Context, db *mongo.Database) error {
	var schema struct {
		Version int `bson:"version"`
	}
	if err := db.Collection("qs_retry_schedule_schema").FindOne(ctx, bson.M{"_id": "v1"}).Decode(&schema); err != nil {
		return errors.Join(transaction.ErrSchema, err)
	}
	if schema.Version != 1 {
		return transaction.ErrSchema
	}
	var binding struct {
		Store   string `bson:"store_id"`
		Epoch   string `bson:"writer_epoch"`
		Version int    `bson:"schema_version"`
	}
	if err := db.Collection("rm_tx_binding").FindOne(ctx, bson.M{"_id": s.binding.Owner}).Decode(&binding); err != nil {
		return errors.Join(transaction.ErrBinding, err)
	}
	if binding.Version != 1 {
		return transaction.ErrSchema
	}
	if binding.Store != s.binding.StoreID || binding.Epoch != s.binding.WriterEpoch {
		return transaction.ErrBinding
	}
	return nil
}

func (s *Store) Validate(ctx context.Context) error {
	if mongo.SessionFromContext(ctx) != nil {
		return transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, readBudget)
	defer cancel()
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.validateIn(ctx, s.primary)
}

func (s *Store) root(ctx context.Context) error {
	session := mongo.SessionFromContext(ctx)
	if session == nil {
		return transaction.ErrTransactionRequired
	}
	if session.Client() != s.db.Client() {
		return transaction.ErrBinding
	}
	bridge, ok := session.(mongo.XSession) //nolint:staticcheck // Pinned v1.17.6 active original-session bridge, as used by the SDK.
	if !ok || bridge.ClientSession() == nil || !bridge.ClientSession().TransactionRunning() {
		return transaction.ErrTransactionRequired
	}
	return nil
}

type document struct {
	ID           []byte      `bson:"_id"`
	Identity     []byte      `bson:"key_identity"`
	Input        []byte      `bson:"input_hash"`
	Digest       []byte      `bson:"plan_hash"`
	Owner        string      `bson:"owner"`
	Scope        string      `bson:"scope"`
	Kind         string      `bson:"kind"`
	Subject      string      `bson:"subject_id"`
	Run          string      `bson:"run_id"`
	Message      string      `bson:"message_id"`
	Due          time.Time   `bson:"due_index_at"`
	Frozen       []byte      `bson:"frozen_plan"`
	State        model.State `bson:"state"`
	Version      int64       `bson:"revision"`
	GuardVersion int64       `bson:"guard_version"`
	Proof        []byte      `bson:"dispatch_evidence,omitempty"`
}

func (s *Store) decode(d document) (*model.Record, error) {
	f, err := model.Decode(d.Frozen)
	if err != nil {
		return nil, err
	}
	a, in := f.Authority(), f.Message().Input()
	key, input, digest := f.Key(), f.InputHash(), f.Digest()
	due, err := f.IndexDue(time.Millisecond)
	if err != nil || d.GuardVersion < 0 || d.Owner != s.binding.Owner || d.Owner != a.Owner || d.Scope != in.Scope || d.Kind != a.Kind || d.Subject != a.SubjectID || d.Run != a.RunID || d.Message != in.ID || !d.Due.Equal(due) || !bytes.Equal(d.ID, key[:]) || !bytes.Equal(d.Identity, f.KeyIdentity()) || !bytes.Equal(d.Input, input[:]) || !bytes.Equal(d.Digest, digest[:]) {
		return nil, model.ErrInvalid
	}
	r := &model.Record{Plan: f, State: d.State, Version: d.Version}
	if len(d.Proof) > 0 {
		var proof evidence.EventEvidenceV2
		if json.Unmarshal(d.Proof, &proof) != nil {
			return nil, model.ErrInvalid
		}
		canonical, err := json.Marshal(proof)
		if err != nil || !bytes.Equal(canonical, d.Proof) {
			return nil, model.ErrInvalid
		}
		r.Proof = &proof
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) SaveFrozen(ctx context.Context, candidate model.Frozen) (model.Frozen, error) {
	if !candidate.Valid() || candidate.Authority().Owner != s.binding.Owner {
		return model.Frozen{}, transaction.ErrBinding
	}
	if err := s.root(ctx); err != nil {
		return model.Frozen{}, err
	}
	if err := s.validateIn(ctx, s.db); err != nil {
		return model.Frozen{}, err
	}
	key := candidate.Key()
	var existing document
	err := s.db.Collection(collection).FindOne(ctx, bson.M{"_id": key[:]}).Decode(&existing)
	if err == nil {
		r, err := s.decode(existing)
		if err != nil {
			return model.Frozen{}, err
		}
		if err := r.Plan.MatchInput(candidate); err != nil {
			return model.Frozen{}, err
		}
		return r.Plan, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return model.Frozen{}, err
	}
	due, err := candidate.IndexDue(time.Millisecond)
	if err != nil {
		return model.Frozen{}, err
	}
	input, digest := candidate.InputHash(), candidate.Digest()
	a, in := candidate.Authority(), candidate.Message().Input()
	d := document{ID: key[:], Identity: candidate.KeyIdentity(), Input: input[:], Digest: digest[:], Owner: a.Owner, Scope: in.Scope, Kind: a.Kind, Subject: a.SubjectID, Run: a.RunID, Message: in.ID, Due: due, Frozen: candidate.Bytes(), State: model.Ready, Version: 1}
	_, err = s.db.Collection(collection).InsertOne(ctx, d)
	if mongo.IsDuplicateKeyError(err) {
		return model.Frozen{}, port.ErrReservationRace
	}
	if err != nil {
		return model.Frozen{}, err
	}
	return candidate, nil
}

func (s *Store) Find(ctx context.Context, candidate model.Frozen) (*model.Record, error) {
	if !candidate.Valid() || candidate.Authority().Owner != s.binding.Owner {
		return nil, transaction.ErrBinding
	}
	if mongo.SessionFromContext(ctx) != nil {
		return nil, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, readBudget)
	defer cancel()
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.validateIn(ctx, s.primary); err != nil {
		return nil, err
	}
	key := candidate.Key()
	var d document
	err = s.primary.Collection(collection).FindOne(ctx, bson.M{"_id": key[:]}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, transaction.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r, err := s.decode(d)
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
	if mongo.SessionFromContext(ctx) != nil {
		return nil, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, readBudget)
	defer cancel()
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.validateIn(ctx, s.primary); err != nil {
		return nil, err
	}
	cursor, err := s.primary.Collection(collection).Find(ctx, bson.M{"owner": s.binding.Owner, "state": model.Ready, "due_index_at": bson.M{"$lte": now.UTC().Truncate(time.Millisecond)}}, options.Find().SetLimit(int64(limit)).SetSort(bson.D{{Key: "due_index_at", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, cursor.Close(ctx)) }()
	result = make([]model.Record, 0, limit)
	for cursor.Next(ctx) {
		var d document
		if err := cursor.Decode(&d); err != nil {
			return nil, err
		}
		r, err := s.decode(d)
		if err != nil {
			return nil, err
		}
		if r.State != model.Ready || r.Plan.DueAt().After(now) {
			return nil, fmt.Errorf("%w: inconsistent due selector", model.ErrInvalid)
		}
		result = append(result, *r)
	}
	return result, cursor.Err()
}
