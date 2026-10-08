//go:build integration

package answersheet

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func legacyHistoryIndex(t *testing.T, c *mongo.Collection) {
	t.Helper()
	key := "legacy_submission_evidence.entries.event_id"
	if _, err := c.Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: key, Value: 1}}, Options: options.Index().SetUnique(true).SetCollation(&options.Collation{Locale: "simple"}).SetPartialFilterExpression(bson.M{key: bson.M{"$type": "string"}})}); err != nil {
		t.Fatal(err)
	}
}

func legacyRow(id uint64) AnswerSheetPO {
	row := retirementSheet(id)
	row.DurableAcceptance = nil
	row.Admission = &AdmissionPO{Purpose: "independent_questionnaire", QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion}
	return row
}

func legacyReference(id, binding string) evidence.HistoricalReferenceEntryV1 {
	digest := evidence.SourceDigest("mongo-selected-source-bson-v1", []byte(id))
	return evidence.HistoricalReferenceEntryV1{EventID: id, EventType: "answersheet.submitted", Source: evidence.HistoricalSourceReferenceV1{Database: "mongodb", Object: "domain_event_outbox", PrimaryKeyKind: "mongodb_objectid", PrimaryKeySHA256: evidence.SourceDigest("objectid", []byte(id)).SHA256, Digest: digest}, Proof: &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: id, Digest: digest, BusinessBindingSHA256: binding, Origin: "retirement", Verification: evidence.Verification{Method: "original-source-and-frozen-admission", Version: "v1", OperationID: "12345-1", VerifiedAt: time.Date(2026, 10, 8, 3, 4, 5, 678000000, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}}
}

func TestLegacySubmissionSeparateSetCASAndOrdinaryUpdatePreservation(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	c := db.Collection("answersheets")
	legacyHistoryIndex(t, c)
	row := legacyRow(1)
	if _, err := c.InsertOne(ctx, row); err != nil {
		t.Fatal(err)
	}
	store, err := NewRetirementEvidenceStore(db)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := store.PrepareLegacySubmission(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	entry := legacyReference("legacy-original-1", baseline.BindingSHA256())
	if err := store.BackfillLegacySubmission(ctx, baseline, entry); err == nil {
		t.Fatal("accepted missing borrowed transaction")
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	abort := errors.New("host rollback")
	if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
		if err := store.BackfillLegacySubmission(tx, baseline, entry); err != nil {
			return nil, err
		}
		return nil, abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	var before AnswerSheetPO
	if err := c.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&before); err != nil {
		t.Fatal(err)
	}
	if before.LegacySubmissionEvidence != nil {
		t.Fatal("helper committed host transaction")
	}
	for _, value := range []evidence.HistoricalReferenceEntryV1{entry, entry, legacyReference("legacy-original-2", baseline.BindingSHA256())} {
		if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
			return nil, store.BackfillLegacySubmission(tx, baseline, value)
		}); err != nil {
			t.Fatal(err)
		}
	}
	var after AnswerSheetPO
	if err := c.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.DurableAcceptance != nil || after.LegacySubmissionEvidence == nil || len(after.LegacySubmissionEvidence.Entries) != 2 {
		t.Fatal("history manufactured acceptance or lost an identity")
	}
	saved := after.LegacySubmissionEvidence
	after.LegacySubmissionEvidence = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("history changed original sheet facts")
	}
	repo, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	sheet := repo.mapper.ToBO(&after)
	if err := repo.Update(ctx, sheet); err != nil {
		t.Fatal(err)
	}
	var ordinary AnswerSheetPO
	if err := c.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&ordinary); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, ordinary.LegacySubmissionEvidence) || ordinary.DurableAcceptance != nil {
		t.Fatal("ordinary writer overwrote maintenance history")
	}
}

func TestLegacySubmissionRefusesMissingOrAssessmentIntentAndCASMutations(t *testing.T) {
	for _, kind := range []string{"no_admission", "assessment", "wrong_questionnaire", "model_on_independent", "marker", "ownership", "clock"} {
		t.Run(kind, func(t *testing.T) {
			_, db := mongodbtest.ReplicaSetDatabase(t)
			c := db.Collection("answersheets")
			legacyHistoryIndex(t, c)
			row := legacyRow(1)
			switch kind {
			case "no_admission":
				row.Admission = nil
			case "assessment":
				row.Admission.Purpose = "assessment"
			case "wrong_questionnaire":
				row.Admission.QuestionnaireVersion = "other"
			case "model_on_independent":
				row.Admission.ModelCode = "model"
			case "marker":
				row.DurableAcceptance = retirementSheet(1).DurableAcceptance
			case "ownership":
				row.TesteeID = 0
			case "clock":
				row.FilledAt = time.Time{}
			}
			if _, err := c.InsertOne(t.Context(), row); err != nil {
				t.Fatal(err)
			}
			store, _ := NewRetirementEvidenceStore(db)
			if _, err := store.PrepareLegacySubmission(t.Context(), 1); !errors.Is(err, retirementevidence.ErrUnverifiable) {
				t.Fatalf("not refused: %v", err)
			}
		})
	}
	client, db := mongodbtest.ReplicaSetDatabase(t)
	c := db.Collection("answersheets")
	legacyHistoryIndex(t, c)
	if _, err := c.InsertOne(t.Context(), legacyRow(1)); err != nil {
		t.Fatal(err)
	}
	store, _ := NewRetirementEvidenceStore(db)
	baseline, err := store.PrepareLegacySubmission(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateOne(t.Context(), bson.M{"domain_id": uint64(1)}, bson.M{"$set": bson.M{"org_id": uint64(999)}}); err != nil {
		t.Fatal(err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(t.Context())
	if _, err := session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
		return nil, store.BackfillLegacySubmission(tx, baseline, legacyReference("old-id", baseline.BindingSHA256()))
	}); !errors.Is(err, retirementevidence.ErrConflict) {
		t.Fatalf("CAS mutation accepted: %v", err)
	}
}

func TestLegacySubmissionMultikeyCollisionDuplicatesAndCapacity(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	c := db.Collection("answersheets")
	legacyHistoryIndex(t, c)
	store, _ := NewRetirementEvidenceStore(db)
	for _, id := range []uint64{1, 2} {
		if _, err := c.InsertOne(t.Context(), legacyRow(id)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.PrepareLegacySubmission(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.PrepareLegacySubmission(t.Context(), 2)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(t.Context())
	write := func(b *LegacySubmissionBaseline, e evidence.HistoricalReferenceEntryV1) error {
		_, err := session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) { return nil, store.BackfillLegacySubmission(tx, b, e) })
		return err
	}
	if err := write(first, legacyReference("unique-event", first.BindingSHA256())); err != nil {
		t.Fatal(err)
	}
	if err := write(second, legacyReference("unique-event", second.BindingSHA256())); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("cross-document event collision accepted: %v", err)
	}
	changed := legacyReference("unique-event", first.BindingSHA256())
	changed.Proof.Verification.Reason = "changed conclusion"
	if err := write(first, changed); !errors.Is(err, retirementevidence.ErrConflict) {
		t.Fatal("same ID changed proof accepted", err)
	}
	for i := 1; i < evidence.HistoricalReferenceMaxEntries; i++ {
		if err := write(first, legacyReference(fmt.Sprintf("bounded-%d", i), first.BindingSHA256())); err != nil {
			t.Fatal(err)
		}
	}
	if err := write(first, legacyReference("overflow", first.BindingSHA256())); !errors.Is(err, retirementevidence.ErrConflict) {
		t.Fatal("capacity overflow accepted", err)
	}
	var actual AnswerSheetPO
	if err := c.FindOne(t.Context(), bson.M{"domain_id": uint64(1)}).Decode(&actual); err != nil {
		t.Fatal(err)
	}
	if len(actual.LegacySubmissionEvidence.Entries) != 128 {
		t.Fatal("capacity bound altered set")
	}
	duplicate := actual.LegacySubmissionEvidence.Clone()
	duplicate.Entries = append(duplicate.Entries[:1], duplicate.Entries[0])
	if _, err := c.UpdateOne(t.Context(), bson.M{"domain_id": uint64(1)}, bson.M{"$set": bson.M{"legacy_submission_evidence": duplicate}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareLegacySubmission(t.Context(), 1); err == nil {
		t.Fatal("multikey cannot detect within-array duplicates; application validation failed")
	}
}

func TestLegacySubmissionSimpleCollationPreservesExactSourceIDsAndCAS(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	if err := db.CreateCollection(ctx, "answersheets", options.CreateCollection().SetCollation(&options.Collation{Locale: "en", Strength: 2})); err != nil {
		t.Fatal(err)
	}
	c := db.Collection("answersheets")
	legacyHistoryIndex(t, c)
	row := legacyRow(1)
	row.QuestionnaireTitle = "OriginalCase"
	if _, err := c.InsertOne(ctx, row); err != nil {
		t.Fatal(err)
	}
	store, _ := NewRetirementEvidenceStore(db)
	baseline, err := store.PrepareLegacySubmission(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var exact bson.Raw
	if err := c.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&exact); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateOne(ctx, bson.M{"domain_id": uint64(1)}, bson.M{"$set": bson.M{"questionnaire_title": "originalcase"}}); err != nil {
		t.Fatal(err)
	}
	predicate := bson.M{"$expr": bson.M{"$eq": bson.A{"$$ROOT", bson.M{"$literal": exact}}}}
	if n, err := c.CountDocuments(ctx, predicate); err != nil || n != 1 {
		t.Fatalf("casefold adversary was not effective: %d %v", n, err)
	}
	if n, err := c.CountDocuments(ctx, predicate, options.Count().SetCollation(&options.Collation{Locale: "simple"})); err != nil || n != 0 {
		t.Fatalf("simple comparison did not distinguish original bytes: %d %v", n, err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
		return nil, store.BackfillLegacySubmission(tx, baseline, legacyReference("source-ID", baseline.BindingSHA256()))
	}); !errors.Is(err, retirementevidence.ErrConflict) {
		t.Fatal("case-only changed business baseline accepted", err)
	}
	baseline, err = store.PrepareLegacySubmission(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"source-ID", "source-id"} {
		if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
			return nil, store.BackfillLegacySubmission(tx, baseline, legacyReference(id, baseline.BindingSHA256()))
		}); err != nil {
			t.Fatal("case-distinct original source identity was merged", err)
		}
	}
	var actual AnswerSheetPO
	if err := c.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&actual); err != nil {
		t.Fatal(err)
	}
	if len(actual.LegacySubmissionEvidence.Entries) != 2 {
		t.Fatal("case-distinct identities not retained")
	}
	// A casefold unique index is stricter than the source-ID contract and is not
	// accepted merely because it has the expected key/name/unique shape.
	if _, err := c.Indexes().DropOne(ctx, "legacy_submission_evidence.entries.event_id_1"); err != nil {
		t.Fatal(err)
	}
	key := "legacy_submission_evidence.entries.event_id"
	if _, err := c.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: key, Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{key: bson.M{"$type": "string"}})}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareLegacySubmission(ctx, 1); !errors.Is(err, retirementevidence.ErrUnverifiable) {
		t.Fatal("non-simple historical identity index accepted", err)
	}
}
