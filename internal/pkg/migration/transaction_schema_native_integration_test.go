//go:build integration

package migration

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/message"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestM3TransactionSchemaNative(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	n := openNativeSchemaPairWithPrefix(t, "qs_m3_tx_test")
	old := migrationResourcesThrough(t, 105, 40)
	n.approve(t, old)
	n.runBoth(t, n.preflight(t, old))
	// Nonempty historical facts retain their original bytes and retry responsibility.
	_, err := n.db.ExecContext(t.Context(), `INSERT INTO rm_outbox(producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,state,next_attempt_at,attempt_count,failure_count) VALUES('qs-server','old','old.topic','changed','v1','org:7','application/json','2026-10-19T00:00:00Z',0x00ff03,UNHEX(REPEAT('ab',32)),'held',UTC_TIMESTAMP(6),7,3)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.mongoDB.Collection("m3_old_fact").InsertOne(t.Context(), bson.M{"_id": "old", "wire": []byte{0, 255, 3}, "attempts": 7})
	if err != nil {
		t.Fatal(err)
	}
	current, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	if current.accepts(106, 40) || current.accepts(105, 41) {
		t.Fatal("partial schema pair accepted")
	}
	cfg := offlineNativeApproval(t, n, current)
	result, err := runOffline(t.Context(), n.db, n.client, cfg, current)
	if err != nil || !result.Verified || !result.AuthorizationConsumed {
		t.Fatal("approved original 105/40 upgrade failed", result, err)
	}
	m3SQLProof(t, n.db, "qs.mysql")
	m3MongoProof(t, n)
	var payload, state string
	var attempts, failures int
	err = n.db.QueryRowContext(t.Context(), `SELECT HEX(payload),state,attempt_count,failure_count FROM rm_outbox WHERE message_id='old'`).Scan(&payload, &state, &attempts, &failures)
	if err != nil || payload != "00FF03" || state != "held" || attempts != 7 || failures != 3 {
		t.Fatal("legacy Outbox changed", err)
	}
	var fact struct {
		Wire     []byte `bson:"wire"`
		Attempts int    `bson:"attempts"`
	}
	if err = n.mongoDB.Collection("m3_old_fact").FindOne(t.Context(), bson.M{"_id": "old"}).Decode(&fact); err != nil || string(fact.Wire) != string([]byte{0, 255, 3}) || fact.Attempts != 7 {
		t.Fatal("legacy Mongo facts changed", err)
	}
	down, err := migrations.ReadFile("migrations/mysql/000106_transaction_messaging.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = n.db.ExecContext(t.Context(), string(down)); err == nil {
		t.Fatal("SQL downgrade erased protocol proof")
	}
}

func m3MongoProof(t *testing.T, n nativeSchemaPair) {
	t.Helper()
	ctx := t.Context()
	b := transaction.Binding{Owner: "qs.mongo", StoreID: "owned-test-primary", WriterEpoch: "m3-1"}
	s, err := txmongo.New(n.mongoDB, b)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ValidateSchema(ctx); err != nil {
		t.Fatal(err)
	}
	p := m3Plan(t, b.Owner, "binding")
	if _, err = s.Reserve(ctx, transaction.RequestFor(p), p, transaction.ReserveOptions{Lease: time.Minute}); err == nil {
		t.Fatal("Mongo migration authorized binding implicitly")
	}
	if _, err = n.mongoDB.Collection("rm_tx_binding").InsertOne(ctx, bson.M{"_id": b.Owner, "store_id": b.StoreID, "writer_epoch": b.WriterEpoch, "schema_version": 1}); err != nil {
		t.Fatal(err)
	}
	if err = n.mongoDB.CreateCollection(ctx, "m3_install_fact"); err != nil {
		t.Fatal(err)
	}
	for _, commit := range []bool{false, true} {
		id := fmt.Sprintf("root-%v", commit)
		p = m3Plan(t, b.Owner, id)
		r, err := s.Reserve(ctx, transaction.RequestFor(p), p, transaction.ReserveOptions{Lease: time.Minute})
		if err != nil || !r.Acquired {
			t.Fatal(r, err)
		}
		session, err := n.client.StartSession()
		if err != nil {
			t.Fatal(err)
		}
		defer session.EndSession(ctx)
		if err = session.StartTransaction(); err != nil {
			t.Fatal(err)
		}
		sc := mongo.NewSessionContext(ctx, session)
		guard, err := s.Bind(sc, r.Execution)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = n.mongoDB.Collection("m3_install_fact").InsertOne(sc, bson.M{"_id": id, "wire": []byte{0, 255, 3}}); err != nil {
			t.Fatal(err)
		}
		if err = guard.CommitWitness(sc); err != nil {
			t.Fatal(err)
		}
		check := transaction.StoreChecker{Store: s}
		ref := r.Execution.References()[0]
		if check.Check(ctx, ref) != transaction.Unknown {
			t.Fatal("Mongo uncommitted root visible")
		}
		want := transaction.Unknown
		if commit {
			err = session.CommitTransaction(ctx)
			want = transaction.Commit
		} else {
			err = session.AbortTransaction(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
		if check.Check(ctx, ref) != want {
			t.Fatal("Mongo root decision mismatch")
		}
		count, err := n.mongoDB.Collection("m3_install_fact").CountDocuments(ctx, bson.M{"_id": id})
		if err != nil || (count == 1) != commit {
			t.Fatal(count, err)
		}
		if !commit {
			ok, err := s.Abort(ctx, r.Execution)
			if err != nil || !ok || check.Check(ctx, ref) != transaction.Rollback {
				t.Fatal("Mongo abort proof failed", err)
			}
		}
	}
}

func m3Plan(t *testing.T, owner, id string) transaction.FrozenPlan {
	t.Helper()
	m, err := message.New(message.Input{Producer: "schema-test", ID: id, Destination: "schema.logical", EventType: "schema.fact", SchemaVersion: "v1", Scope: "org:7", ContentType: "application/octet-stream", OccurredAt: "2026-10-19T00:00:00Z", Payload: []byte{0, 255, 1}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := transaction.NewPlan(transaction.PlanInput{Owner: owner, Scope: "org:7", OperationID: "op-" + id, IdempotencyKey: id, InputHash: sha256.Sum256([]byte(id)), Branches: []transaction.BranchInput{{ID: "fact", PhysicalTopic: "schema.physical", RouteRevision: "m3-1", Message: m, Wire: []byte{0, 255, 3}}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// This is an installation/adapter proof, not proof that a business entry uses it.
func m3SQLProof(t *testing.T, db *sql.DB, owner string) {
	t.Helper()
	ctx := t.Context()
	b := transaction.Binding{Owner: owner, StoreID: "owned-test-primary", WriterEpoch: "m3-1"}
	s, err := txmysql.New(db, b)
	if err != nil {
		t.Fatal(err)
	}
	p := m3Plan(t, owner, "binding")
	if _, err = s.Reserve(ctx, transaction.RequestFor(p), p, transaction.ReserveOptions{Lease: time.Minute}); err == nil {
		t.Fatal("migration silently authorized a binding")
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO rm_tx_binding VALUES(?,?,?,1)`, []byte(b.Owner), []byte(b.StoreID), []byte(b.WriterEpoch)); err != nil {
		t.Fatal(err)
	}
	wrong, err := txmysql.New(db, transaction.Binding{Owner: owner, StoreID: "WRONG", WriterEpoch: b.WriterEpoch})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrong.Reserve(ctx, transaction.RequestFor(p), p, transaction.ReserveOptions{Lease: time.Minute}); !errors.Is(err, transaction.ErrBinding) {
		t.Fatal("wrong original store accepted", err)
	}
	if _, err = db.ExecContext(ctx, `CREATE TABLE m3_install_fact(id VARBINARY(128) PRIMARY KEY,value BLOB NOT NULL) ENGINE=InnoDB`); err != nil {
		t.Fatal(err)
	}
	for _, committed := range []bool{false, true} {
		id := fmt.Sprintf("root-%v", committed)
		p = m3Plan(t, owner, id)
		r, err := s.Reserve(ctx, transaction.RequestFor(p), p, transaction.ReserveOptions{Lease: time.Minute})
		if err != nil || !r.Acquired {
			t.Fatal(r, err)
		}
		raw, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer raw.Rollback()
		g, err := s.Bind(ctx, raw, r.Execution)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = raw.ExecContext(ctx, `INSERT INTO m3_install_fact VALUES(?,?)`, id, []byte{0, 255, 3}); err != nil {
			t.Fatal(err)
		}
		if err = g.CommitWitness(ctx); err != nil {
			t.Fatal(err)
		}
		check := transaction.StoreChecker{Store: s}
		ref := r.Execution.References()[0]
		if check.Check(ctx, ref) != transaction.Unknown {
			t.Fatal("uncommitted root visible")
		}
		want := transaction.Unknown
		if committed {
			err = raw.Commit()
			want = transaction.Commit
		} else {
			err = raw.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
		if check.Check(ctx, ref) != want {
			t.Fatal("original root decision mismatch")
		}
		var count int
		if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM m3_install_fact WHERE id=?`, id).Scan(&count); err != nil || (count == 1) != committed {
			t.Fatal(count, err)
		}
		if !committed {
			ok, err := s.Abort(ctx, r.Execution)
			if err != nil || !ok || check.Check(ctx, ref) != transaction.Rollback {
				t.Fatal("rollback evidence did not converge", err)
			}
		}
		duplicate, err := s.Reserve(ctx, transaction.RequestFor(p), p, transaction.ReserveOptions{Lease: time.Minute})
		if err != nil || duplicate.Acquired || duplicate.Execution.Token != [32]byte{} {
			t.Fatal("duplicate obtained execution permission", err)
		}
	}
	m3ArchiveSQLProof(t, db, b)
	if _, err = db.ExecContext(ctx, `UPDATE rm_tx_binding SET schema_version=2 WHERE owner=?`, []byte(owner)); err != nil {
		t.Fatal(err)
	}
	p = m3Plan(t, owner, "schema-version")
	if _, err = s.Reserve(ctx, transaction.RequestFor(p), p, transaction.ReserveOptions{Lease: time.Minute}); !errors.Is(err, transaction.ErrSchema) {
		t.Fatal("incompatible schema accepted", err)
	}
}
