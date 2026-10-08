//go:build integration

package answersheet

import (
	"errors"
	"reflect"
	"testing"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func retirementSheet(id uint64) AnswerSheetPO {
	at := time.Date(2026, 10, 8, 2, 3, 4, 567891234, time.UTC)
	return AnswerSheetPO{BaseDocument: base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.FromUint64(id), CreatedAt: at, UpdatedAt: at}, QuestionnaireCode: "questionnaire", QuestionnaireVersion: "v1", FillerID: 31, FillerType: "testee", OrgID: 7, TesteeID: 31, TaskID: "task:original", FilledAt: at, Admission: &AdmissionPO{Purpose: "assessment", ModelKind: "scale", ModelSubKind: "frozen-subkind", ModelCode: "model", ModelVersion: "v1"}, Attribution: &AttributionSnapshotPO{OriginType: "direct", CapturedAt: at, Version: 1}, SubmitMeta: &SubmitMetaPO{IdempotencyKey: "original-key", WriterID: 31, Fingerprint: "original-submit-fingerprint", RequestID: "request:original", AcceptedAt: at}, DurableAcceptance: &DurableAcceptancePO{SchemaVersion: 1, EventID: "answersheet:original", RequestID: "request:original", AcceptedAt: at}}
}

func retirementSheetProof(baseline *RetirementEvidenceBaseline) *evidence.EventEvidenceV1 {
	return &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: baseline.EventID(), Digest: evidence.SourceDigest("mongo-selected-source-bson-v1", []byte("selected historic source")), BusinessBindingSHA256: baseline.BindingSHA256(), Origin: "retirement", Verification: evidence.Verification{Method: "source-and-business-facts", Version: "v1", OperationID: "test-retirement", VerifiedAt: time.Date(2026, 10, 8, 3, 4, 5, 678901234, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}
}

func TestAnswerSheetHistoricalEvidenceBorrowedTransactionPreservesOriginalFacts(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	row := retirementSheet(1)
	collection := db.Collection("answersheets")
	if _, err := collection.InsertOne(ctx, row); err != nil {
		t.Fatal(err)
	}
	store, err := NewRetirementEvidenceStore(db)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := store.Prepare(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !baseline.Payload().SubmittedAt.Equal(row.FilledAt.Truncate(time.Millisecond)) || baseline.Payload().RequestID != row.DurableAcceptance.RequestID {
		t.Fatal("projection did not use original stored millisecond clock and trace")
	}
	copyPayload := baseline.Payload()
	copyPayload.Admission.ModelCode = "edited"
	copyPayload.Attribution.OriginType = "edited"
	if baseline.Payload().Admission.ModelCode != "model" || baseline.Payload().Attribution.OriginType != "direct" {
		t.Fatal("verifier can mutate private business projection")
	}
	proof := retirementSheetProof(baseline)
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	abort := errors.New("caller rollback")
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
		if err := store.Backfill(tx, baseline, proof); err != nil {
			return nil, err
		}
		return nil, abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	var before AnswerSheetPO
	if err := collection.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&before); err != nil {
		t.Fatal(err)
	}
	if before.DurableAcceptance.EventEvidence != nil {
		t.Fatal("maintenance adapter committed caller's transaction")
	}
	for range 2 {
		if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) { return nil, store.Backfill(tx, baseline, proof.Clone()) }); err != nil {
			t.Fatal(err)
		}
	}
	var after AnswerSheetPO
	if err := collection.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.DurableAcceptance.EventEvidence == nil || after.DurableAcceptance.EventEvidence.Class != evidence.RetiredVerified {
		t.Fatal("missing historic conclusion")
	}
	after.DurableAcceptance.EventEvidence = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("evidence maintenance altered original business or submission facts")
	}
	different := proof.Clone()
	different.Digest = evidence.SourceDigest("mongo-selected-source-bson-v1", []byte("different source"))
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) { return nil, store.Backfill(tx, baseline, different) })
	if !errors.Is(err, retirementevidence.ErrConflict) {
		t.Fatalf("different conclusion accepted: %v", err)
	}
}

func TestAnswerSheetHistoricalEvidenceCannotChangeAnySubmissionBaseline(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	store, err := NewRetirementEvidenceStore(db)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	changes := []struct {
		field string
		value any
	}{
		{"org_id", uint64(99)}, {"durable_acceptance.event_id", "new-event"}, {"durable_acceptance.request_id", "new-request"}, {"durable_acceptance.accepted_at", time.Now().UTC()},
		{"submit_meta.request_id", "new-request"}, {"submit_meta.fingerprint", "different-input"}, {"filled_at", time.Now().UTC()}, {"admission.model_sub_kind", "changed"}, {"attribution.origin_id", "new-origin"}, {"unknown_business_field", nil},
	}
	for i, change := range changes {
		t.Run(change.field, func(t *testing.T) {
			id := uint64(i + 1)
			if _, err := db.Collection("answersheets").InsertOne(ctx, retirementSheet(id)); err != nil {
				t.Fatal(err)
			}
			baseline, err := store.Prepare(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Collection("answersheets").UpdateOne(ctx, bson.M{"domain_id": id}, bson.M{"$set": bson.M{change.field: change.value}}); err != nil {
				t.Fatal(err)
			}
			_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
				return nil, store.Backfill(tx, baseline, retirementSheetProof(baseline))
			})
			if !errors.Is(err, retirementevidence.ErrConflict) {
				t.Fatalf("changed baseline accepted: %v", err)
			}
		})
	}
}

func TestAnswerSheetHistoricalEvidenceMissingFactsAndGapClassification(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	store, err := NewRetirementEvidenceStore(db)
	if err != nil {
		t.Fatal(err)
	}
	for i, change := range []func(*AnswerSheetPO){func(row *AnswerSheetPO) { row.DurableAcceptance = nil }, func(row *AnswerSheetPO) { row.FilledAt = time.Time{} }, func(row *AnswerSheetPO) { row.OrgID = 0 }, func(row *AnswerSheetPO) { row.DurableAcceptance.RequestID = "contradiction" }} {
		row := retirementSheet(uint64(i + 1))
		change(&row)
		if _, err := db.Collection("answersheets").InsertOne(ctx, row); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Prepare(ctx, row.DomainID.Uint64()); err == nil {
			t.Fatal("missing or contradictory original business facts accepted")
		}
	}
	row := retirementSheet(10)
	row.DurableAcceptance.EventID = ""
	if _, err := db.Collection("answersheets").InsertOne(ctx, row); err != nil {
		t.Fatal(err)
	}
	baseline, err := store.Prepare(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	proof := retirementSheetProof(baseline)
	proof.Class = evidence.Unverifiable
	proof.Verification.Reason = "original_event_identity_absent"
	if err := baseline.ValidateEvidence(proof); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*evidence.EventEvidenceV1){func(p *evidence.EventEvidenceV1) { p.EventID = "invented" }, func(p *evidence.EventEvidenceV1) { p.Verification.ResponsibilityClosed = false }, func(p *evidence.EventEvidenceV1) { p.Verification.Reason = "" }, func(p *evidence.EventEvidenceV1) { p.BusinessBindingSHA256 = evidence.BindingDigest("different") }} {
		bad := proof.Clone()
		mutate(bad)
		if err := baseline.ValidateEvidence(bad); err == nil {
			t.Fatal("invalid identity, gap or closure conclusion accepted")
		}
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) { return nil, store.Backfill(tx, baseline, proof) }); err != nil {
		t.Fatal(err)
	}
	var actual AnswerSheetPO
	if err := db.Collection("answersheets").FindOne(ctx, bson.M{"domain_id": uint64(10)}).Decode(&actual); err != nil {
		t.Fatal(err)
	}
	if actual.DurableAcceptance.EventID != "" || actual.DurableAcceptance.EventEvidence.EventID != "" || actual.DurableAcceptance.EventEvidence.Class != evidence.Unverifiable {
		t.Fatal("gap backfill manufactured original event ID")
	}
}
