package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

func sameBusinessNamespaces(m material, o observation) bool {
	for i, old := range m.Collections {
		now := o.Collections[i]
		if validateNamespace(now, old.Name) != nil || !bytes.Equal(old.UUID, now.UUID) || !bytes.Equal(old.Options, now.Options) || *old.Count != *now.Count {
			return false
		}
		a, b := indexSpecs(old), indexSpecs(now)
		if len(a) != len(b) {
			return false
		}
		for name, raw := range a {
			if !bytes.Equal(raw, b[name]) {
				return false
			}
		}
	}
	return true
}
func verifyTargetObservation(m material, o observation) error {
	missing, e := planRestore(m, o)
	v, dirty, le := ledgerVersion(o.Ledger.Document)
	if e != nil || le != nil || v != 40 || dirty || len(missing) != 4 {
		return errors.New("target_clean10140_or_retained_scope_rejected")
	}
	return nil
}
func verifyRestoredObservation(m material, before, after observation) error {
	if after.SQLVersion != 101 || after.SQLDirty || after.SQLIdentity != m.SQLIdentity || after.MongoIdentity != m.MongoIdentity || !sameBusinessNamespaces(m, after) || validateLedger(after.Ledger.Namespace) != nil || !bytes.Equal(after.Ledger.Namespace.UUID, before.Ledger.Namespace.UUID) || !bytes.Equal(after.Ledger.Namespace.Options, m.Ledger.Namespace.Options) || !bytes.Equal(after.Ledger.Namespace.Indexes[0], m.Ledger.Namespace.Indexes[0]) || !bytes.Equal(after.Ledger.Document, m.Ledger.Document) {
		return errors.New("restored_objects_original_bson_or_current_ledger_uuid_unproven")
	}
	return nil
}
func consumeApproval(path string, a approval, hash string) error {
	raw, _ := json.Marshal([]string{a.Operation, hash, a.MaterialHash})
	return writeExclusive(path+".consumed", raw)
}
func recoveryBudget(ctx context.Context, at time.Time) (time.Duration, error) {
	deadline, ok := ctx.Deadline()
	left := deadline.Sub(at)
	if !ok || ctx.Err() != nil || left < time.Millisecond {
		return 0, errors.New("restore_deadline_exhausted")
	}
	return left, nil
}
func (d *database) verifyTarget(ctx context.Context, m material) (observation, error) {
	o, e := d.snapshot(ctx)
	if e != nil {
		return o, e
	}
	return o, verifyTargetObservation(m, o)
}
func rereadApproval(m material, a approval, materialPath, materialHash, approvalPath, approvalHash, tool string) error {
	raw, e := readPrivate(materialPath, materialHash, maxMaterialBytes)
	var again material
	if e != nil || strictDecode(raw, &again) != nil || validateMaterial(again, tool, m.SQLName, m.MongoName) != nil {
		return errors.New("frozen_material_changed_before_claim")
	}
	raw, e = readPrivate(approvalPath, approvalHash, 32768)
	var approved approval
	if e != nil || strictDecode(raw, &approved) != nil {
		return errors.New("frozen_approval_changed_before_claim")
	}
	// Hash equality binds both files' original bytes. The parsed object passed
	// to the write path must remain exactly that independently frozen approval.
	before, _ := json.Marshal(a)
	after, _ := json.Marshal(approved)
	if !bytes.Equal(before, after) {
		return errors.New("frozen_approval_changed_before_claim")
	}
	return validateApproval(approved, again, materialHash, tool, time.Now())
}
func (d *database) createOriginalIndex(ctx context.Context, m material, position int) error {
	left, e := recoveryBudget(ctx, time.Now())
	if e != nil {
		return e
	}
	cmd, e := originalIndexCommand(m, position, left)
	if e != nil {
		return e
	}
	if d.mongo.Database(d.mongoName).RunCommand(ctx, cmd).Err() != nil {
		return errors.New("index_restore_failed_or_unknown")
	}
	return nil
}

func originalIndexCommand(m material, position int, left time.Duration) (bson.D, error) {
	if position < 0 || position >= len(targets) || left < time.Millisecond || left > 20*time.Minute {
		return nil, errors.New("compiled_index_restore_spec_or_budget_rejected")
	}
	target := targets[position]
	var spec []byte
	for _, ns := range m.Collections {
		if ns.Name == target.collection {
			spec = indexSpecs(ns)[target.name]
		}
	}
	if validateTargetSpec(spec, position) != nil {
		return nil, errors.New("compiled_index_restore_spec_rejected")
	}
	// Replay only pinned original metadata, not caller-provided commands/DDL.
	return bson.D{{Key: "createIndexes", Value: target.collection}, {Key: "indexes", Value: bson.A{bson.Raw(spec)}}, {Key: "maxTimeMS", Value: left.Milliseconds()}, {Key: "writeConcern", Value: bson.D{{Key: "w", Value: "majority"}, {Key: "wtimeout", Value: left.Milliseconds()}}}}, nil
}

func (d *database) restore(ctx context.Context, m material, a approval, materialPath, materialHash, approvalPath, approvalHash, tool string) (observation, error) {
	before, e := d.snapshot(ctx)
	if e != nil {
		return before, e
	}
	missing, e := planRestore(m, before)
	if e != nil {
		return before, e
	}
	if e = rereadApproval(m, a, materialPath, materialHash, approvalPath, approvalHash, tool); e != nil {
		return before, e
	}
	if e = consumeApproval(approvalPath, a, approvalHash); e != nil {
		return before, e
	}
	for _, position := range missing {
		if e = validateApproval(a, m, materialHash, tool, time.Now()); e != nil {
			return before, e
		}
		if e = d.createOriginalIndex(ctx, m, position); e != nil {
			return before, e
		}
	}
	restoredIndexes, e := d.snapshot(ctx)
	if e != nil {
		return before, e
	}
	if !sameBusinessNamespaces(m, restoredIndexes) || !bytes.Equal(before.Ledger.Document, restoredIndexes.Ledger.Document) || !bytes.Equal(before.Ledger.Namespace.UUID, restoredIndexes.Ledger.Namespace.UUID) || !bytes.Equal(before.Ledger.Namespace.Options, restoredIndexes.Ledger.Namespace.Options) || !bytes.Equal(before.Ledger.Namespace.Indexes[0], restoredIndexes.Ledger.Namespace.Indexes[0]) || before.SQLIdentity != restoredIndexes.SQLIdentity || before.MongoIdentity != restoredIndexes.MongoIdentity {
		return before, errors.New("objects_or_dirty_ledger_changed_before_last_replay")
	}
	if e = validateApproval(a, m, materialHash, tool, time.Now()); e != nil {
		return before, e
	}
	if e = d.replayOriginalLedger(ctx, m, before, a, materialHash, tool); e != nil {
		return before, e
	}
	after, e := d.snapshot(ctx)
	if e != nil {
		return after, e
	}
	return after, verifyRestoredObservation(m, before, after)
}

func (d *database) replayOriginalLedger(ctx context.Context, m material, before observation, a approval, materialHash, tool string) (err error) {
	session, e := d.mongo.StartSession(options.Session().SetCausalConsistency(false))
	if e != nil {
		return errors.New("ledger_transaction_session_failed")
	}
	defer func() {
		q, c := context.WithTimeout(context.Background(), queryLimit)
		session.EndSession(q)
		c()
		d.sessionEndCalled = true
	}()
	left, e := recoveryBudget(ctx, time.Now())
	if e != nil {
		return e
	}
	if session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary()).SetWriteConcern(writeconcern.Majority()).SetMaxCommitTime(&left)) != nil {
		return errors.New("ledger_transaction_start_failed")
	}
	d.mongoTransactionReleaseUnproven = true
	commitIssued := false
	defer func() {
		if !commitIssued {
			q, c := context.WithTimeout(context.Background(), queryLimit)
			sc := mongo.NewSessionContext(q, session)
			// AbortTransaction ignores its wire error in the pinned SDK. A call
			// or an outside read of ledger40 cannot certify server transaction release;
			// keep mongoTransactionReleaseUnproven true on every precommit failure.
			d.ledgerAbortCalled = true
			_ = session.AbortTransaction(sc)
			c()
		}
	}()
	sc := mongo.NewSessionContext(ctx, session)
	raw, e := d.ledger(sc)
	if e != nil {
		return e
	}
	if !bytes.Equal(raw, before.Ledger.Document) {
		return errors.New("current_ledger_changed_inside_transaction")
	}
	q, c := context.WithTimeout(sc, queryLimit)
	result, e := d.mongo.Database(d.mongoName).Collection("schema_migrations").DeleteMany(q, bson.D{})
	c()
	if e != nil || result.DeletedCount != 1 {
		return errors.New("ledger_transaction_delete_failed")
	}
	q, c = context.WithTimeout(sc, queryLimit)
	_, e = d.mongo.Database(d.mongoName).Collection("schema_migrations").InsertOne(q, bson.Raw(m.Ledger.Document))
	c()
	if e != nil {
		return errors.New("original_ledger_bson_insert_failed")
	}
	if e = validateApproval(a, m, materialHash, tool, time.Now()); e != nil {
		return e
	}
	sqlIdentity, _, _, e := d.sqlState(ctx, d.conn)
	if e != nil || sqlIdentity != m.SQLIdentity {
		return errors.New("selected_clean101_changed_before_commit")
	}
	left, e = recoveryBudget(ctx, time.Now())
	if e != nil {
		return e
	}
	commitIssued = true
	d.ledgerCommitIssued = true
	// Exactly one application call; no WithTransaction, transaction rerun or
	// application commit retry. mongo-driver v1.17.6 CommitTransaction itself
	// uses RetryOncePerCommand for the SAME lsid/txnNumber. Any returned error
	// remains unknown even if the server already committed the original BSON.
	if session.CommitTransaction(sc) != nil {
		d.ledgerCommitUnknown = true
		return errors.New("ledger_commit_failed_or_unknown")
	}
	d.mongoTransactionReleaseUnproven = false
	d.ledgerCommitAcknowledged = true
	return nil
}
