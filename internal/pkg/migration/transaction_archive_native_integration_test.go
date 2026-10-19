//go:build integration

package migration

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	archive "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
)

// Installation proof over the owned 105/40 -> 106/41 pair, not a business cutover.
func m3ArchiveSQLProof(t *testing.T, db *sql.DB, binding transaction.Binding) {
	t.Helper()
	ctx := t.Context()
	storage, err := archive.NewTransactionArchive(db, binding)
	if err != nil {
		t.Fatal(err)
	}
	sdk, err := txmysql.New(db, binding)
	if err != nil {
		t.Fatal(err)
	}
	key := func(id string) jose.JSONWebKey {
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		return jose.JSONWebKey{Key: k, KeyID: id}
	}
	sign, recipient := key("sign"), key("recipient")
	for _, commit := range []bool{false, true} {
		id := uuid.NewString()
		org := "18446744073709551615"
		body := &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &pb.StartCommand{RequestId: id, Actor: &pb.Actor{OrgId: org, SubjectId: "42"}, Goal: "首次正文：不可重新加密"}}}
		protected, err := app.ProtectMessaging(pb.MessagingKind_START, id, id, "", org, "2026-10-19T12:00:00.123456789+08:00", body, sign, recipient.Public())
		if err != nil {
			t.Fatal(err)
		}
		branch, err := app.FreezeMessagingBranch(protected, org, ^uint64(0), true, true, "command", "m3.archive.test", "m3-1")
		if err != nil {
			t.Fatal(err)
		}
		plan, err := transaction.NewPlan(transaction.PlanInput{Owner: binding.Owner, Scope: branch.Message.Input().Scope, OperationID: id, IdempotencyKey: id, InputHash: sha256.Sum256([]byte(id)), Branches: []transaction.BranchInput{branch}})
		if err != nil {
			t.Fatal(err)
		}
		r, err := sdk.Reserve(ctx, transaction.RequestFor(plan), plan, transaction.ReserveOptions{Lease: time.Minute})
		if err != nil || !r.Acquired {
			t.Fatal("reservation", err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		guard, err := storage.Bind(ctx, tx, r.Execution)
		if err != nil {
			t.Fatal(err)
		}
		if err = guard.Put(ctx, "command"); err != nil {
			t.Fatal(err)
		}
		if err = guard.Put(ctx, "command"); err != nil {
			t.Fatal("same guard duplicate changed original", err)
		}
		if _, found, err := storage.ReadCommitted(ctx, id); found || err != nil {
			t.Fatal("uncommitted archive became visible", found, err)
		}
		if err = guard.CommitWitness(ctx); err != nil {
			t.Fatal(err)
		}
		if err = guard.Put(ctx, "command"); err == nil {
			t.Fatal("sealed guard accepted writes")
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
		value, found, err := storage.ReadCommitted(ctx, id)
		if err != nil || found != commit {
			t.Fatal("archive did not share original root", found, err)
		}
		if !commit {
			continue
		}
		if !bytes.Equal(value.Wire, protected.Wire) || !bytes.Equal(value.Body, protected.Body) || value.OrganizationID != ^uint64(0) {
			t.Fatal("original archive changed")
		}
		ref := &pb.MessagePayloadReference{Producer: "qs-server", Destination: "qs-ai", MessageId: id, OrganizationId: org, BodySha256: protected.Envelope.BodySha256, BodyLength: uint64(len(protected.Body))}
		payload, found, err := storage.Payload(ctx, ref, "qs-ai")
		if err != nil || !found || !bytes.Equal(payload, protected.Body) {
			t.Fatal("committed reference inaccessible", err)
		}
		if _, _, err = storage.Payload(ctx, ref, "wrong"); err == nil {
			t.Fatal("unauthenticated workload read archive")
		}
		duplicate, err := sdk.Reserve(ctx, transaction.RequestFor(plan), plan, transaction.ReserveOptions{Lease: time.Minute})
		if err != nil || duplicate.Acquired {
			t.Fatal("duplicate obtained execution", err)
		}
		tx2, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = storage.Bind(ctx, tx2, duplicate.Execution)
		_ = tx2.Rollback()
		if err == nil {
			t.Fatal("observed execution wrote archive")
		}
		if _, err = db.ExecContext(ctx, `UPDATE ai_messaging_archive SET first_wire=0x00 WHERE message_id=?`, []byte(id)); err != nil {
			t.Fatal(err)
		}
		if _, found, err = storage.ReadCommitted(ctx, id); err == nil || !found {
			t.Fatal("damaged archive allowed legacy fallback", found, err)
		}
		if _, err = db.ExecContext(ctx, `UPDATE ai_messaging_archive SET first_wire=? WHERE message_id=?`, protected.Wire, []byte(id)); err != nil {
			t.Fatal(err)
		}
		ek := transaction.ExecutionKey(binding.Owner, id, binding.WriterEpoch, r.Execution.Fence)
		if _, err = db.ExecContext(ctx, `UPDATE rm_tx_witness SET state='prepared' WHERE execution_key=?`, ek[:]); err != nil {
			t.Fatal(err)
		}
		if _, found, err = storage.ReadCommitted(ctx, id); err == nil || !found {
			t.Fatal("noncommitted witness exposed archive", found, err)
		}
		if _, err = db.ExecContext(ctx, `UPDATE rm_tx_witness SET state='committed' WHERE execution_key=?`, ek[:]); err != nil {
			t.Fatal(err)
		}
	}
}
