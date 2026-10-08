//go:build integration

package interpretation

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type retirementOutcomeFacts struct {
	record *evaluationfact.Record
	err    error
}

func (r *retirementOutcomeFacts) FindByID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return r.record, r.err
}
func (r *retirementOutcomeFacts) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return r.record, r.err
}

func retirementFact(org int64, testee uint64) *evaluationfact.Record {
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: meta.FromUint64(50), OrgID: org, AssessmentID: meta.FromUint64(60), TesteeID: testee, RunID: "original-evaluation-run", SchemaVersion: 1, Payload: []byte(`{"score":5}`), ReportInput: []byte(`{"label":"original"}`), EvaluatedAt: time.Date(2026, 10, 8, 1, 2, 3, 4, time.UTC)})
}

func retirementGraph(t *testing.T, db *mongo.Database) (*RetirementEvidenceStore, *retirementOutcomeFacts) {
	t.Helper()
	at := time.Date(2026, 10, 8, 2, 3, 4, 567891234, time.UTC)
	baseRow := func(id uint64) base.BaseDocument {
		return base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.FromUint64(id), CreatedAt: at, UpdatedAt: at}
	}
	maximum := 10.0
	g := ReportGenerationPO{BaseDocument: baseRow(1), OutcomeID: 50, ReportType: "assessment", TemplateVersion: "v1", Status: "generated", LatestRunID: 11, ReportID: 20, Version: 7, GeneratedEventID: "report-generated:original", TransactionSchemaVersion: 1}
	latest := InterpretationRunPO{BaseDocument: baseRow(11), GenerationID: 1, Attempt: 2, Status: "succeeded", StartedAt: &at, FinishedAt: &at, AttemptOrigin: "force"}
	failed := InterpretationRunPO{BaseDocument: baseRow(10), GenerationID: 1, Attempt: 1, Status: "failed", Failure: &InterpretationFailurePO{Kind: "transient", Code: "original-code", Retryable: true}, StartedAt: &at, FinishedAt: &at, AttemptOrigin: "initial", RetryDisposition: "automatic", NextAttemptAt: &at, PolicyMaxAttempts: 3, RetryPolicyVersion: "policy-original", RetryEventID: "interpret-retry:1:1:force:original-action", ActionRequestID: "original-action", RecoveryCount: 1}
	artifact := InterpretReportPO{BaseDocument: baseRow(20), GenerationID: 1, OutcomeID: 50, InterpretationRunID: 11, ReportType: "assessment", TemplateVersion: "v1", BuilderIdentity: "original-builder", ContentSchemaVersion: "v1", GeneratedAt: at, OrgID: 7, AssessmentID: 60, TesteeID: 31, Model: &ModelIdentityPO{Kind: "scale", Code: "model", Version: "v1", Title: "Original model"}, PrimaryScore: &ScoreValuePO{Kind: "raw_total", Value: 5, Label: "score", Max: &maximum}, Level: &ResultLevelPO{Code: "normal", Label: "Normal", Severity: "low"}, Conclusion: "immutable body"}
	for _, target := range []struct {
		collection string
		record     any
	}{{"report_generations", g}, {"interpretation_runs", latest}, {"interpretation_runs", failed}, {"interpret_report_artifacts", artifact}} {
		if _, err := db.Collection(target.collection).InsertOne(t.Context(), target.record); err != nil {
			t.Fatal(err)
		}
	}
	facts := &retirementOutcomeFacts{record: retirementFact(7, 31)}
	store, err := NewRetirementEvidenceStore(db, facts)
	if err != nil {
		t.Fatal(err)
	}
	return store, facts
}

func retirementInterpretationProof(id, binding string) *evidence.EventEvidenceV1 {
	return &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: id, Digest: evidence.SourceDigest("mongo-selected-source-bson-v1", []byte("selected original source")), BusinessBindingSHA256: binding, Origin: "retirement", Verification: evidence.Verification{Method: "original-source-and-immutable-business", Version: "v1", OperationID: "test-retirement", VerifiedAt: time.Date(2026, 10, 8, 3, 4, 5, 678901234, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}
}

func TestGeneratedHistoricalEvidencePreservesVersionGraphAndCallerRollback(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	store, _ := retirementGraph(t, db)
	baseline, err := store.PrepareGenerated(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	copyPayload := baseline.Payload()
	copyPayload.PrimaryScore.Value = 99
	*copyPayload.PrimaryScore.Max = 99
	copyPayload.Level.Code = "edited"
	if baseline.Payload().PrimaryScore.Value != 5 || *baseline.Payload().PrimaryScore.Max != 10 || baseline.Payload().Level.Code != "normal" {
		t.Fatal("source verifier can edit private generated projection")
	}
	proof := retirementInterpretationProof(baseline.EventID(), baseline.BindingSHA256())
	var before ReportGenerationPO
	if err := db.Collection("report_generations").FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&before); err != nil {
		t.Fatal(err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	abort := errors.New("caller aborts")
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
		if err := store.BackfillGenerated(tx, baseline, proof); err != nil {
			return nil, err
		}
		return nil, abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	var rolledBack ReportGenerationPO
	if err := db.Collection("report_generations").FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&rolledBack); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, rolledBack) {
		t.Fatal("maintenance layer committed caller's transaction")
	}
	for range 2 {
		if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
			return nil, store.BackfillGenerated(tx, baseline, proof.Clone())
		}); err != nil {
			t.Fatal(err)
		}
	}
	var after ReportGenerationPO
	if err := db.Collection("report_generations").FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.GeneratedEventEvidence == nil {
		t.Fatal("missing generated historical conclusion")
	}
	after.GeneratedEventEvidence = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("generated evidence CAS changed Version or other lifecycle facts")
	}
}

func TestGeneratedHistoricalEvidenceCASProtectsFullRelatedFacts(t *testing.T) {
	cases := []struct {
		name, collection, field string
		id                      uint64
		value                   any
	}{
		{"version", "report_generations", "version", 1, uint64(8)}, {"original_event_id", "report_generations", "generated_event_id", 1, "changed"}, {"artifact_body", "interpret_report_artifacts", "conclusion", 20, "changed"},
		{"artifact_owner", "interpret_report_artifacts", "org_id", 20, int64(99)}, {"artifact_model", "interpret_report_artifacts", "model.version", 20, "changed"}, {"artifact_added_field", "interpret_report_artifacts", "unknown_frozen_payload", 20, nil},
		{"run_attempt", "interpretation_runs", "attempt", 11, 3}, {"run_clock", "interpretation_runs", "finished_at", 11, time.Now().UTC()},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			client, db := mongodbtest.ReplicaSetDatabase(t)
			store, _ := retirementGraph(t, db)
			baseline, err := store.PrepareGenerated(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Collection(item.collection).UpdateOne(t.Context(), bson.M{"domain_id": item.id}, bson.M{"$set": bson.M{item.field: item.value}}); err != nil {
				t.Fatal(err)
			}
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(t.Context())
			_, err = session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
				return nil, store.BackfillGenerated(tx, baseline, retirementInterpretationProof(baseline.EventID(), baseline.BindingSHA256()))
			})
			if !errors.Is(err, retirementevidence.ErrConflict) {
				t.Fatalf("changed related baseline accepted: %v", err)
			}
		})
	}
	t.Run("outcome_ownership", func(t *testing.T) {
		client, db := mongodbtest.ReplicaSetDatabase(t)
		store, facts := retirementGraph(t, db)
		baseline, err := store.PrepareGenerated(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		facts.record = retirementFact(7, 999) // VersionToken alone does not include TesteeID.
		session, err := client.StartSession()
		if err != nil {
			t.Fatal(err)
		}
		defer session.EndSession(t.Context())
		_, err = session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
			return nil, store.BackfillGenerated(tx, baseline, retirementInterpretationProof(baseline.EventID(), baseline.BindingSHA256()))
		})
		if !errors.Is(err, retirementevidence.ErrConflict) {
			t.Fatalf("changed Outcome ownership accepted: %v", err)
		}
	})
}

func TestRetryHistoricalEvidencePreservesForceOriginRequestAndOriginalSchedule(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	store, _ := retirementGraph(t, db)
	baseline, err := store.PrepareRetry(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	payload := baseline.Payload()
	if payload.AttemptOrigin != "force" || payload.ActionRequestID != "original-action" || payload.ExpectedAttempt != 1 || payload.Mode != "next_attempt" || payload.RequestedAt.Nanosecond()%int(time.Millisecond) != 0 {
		t.Fatal("retry projection fabricated origin/action or used a new scheduling clock")
	}
	proof := retirementInterpretationProof(baseline.EventID(), baseline.BindingSHA256())
	var before InterpretationRunPO
	if err := db.Collection("interpretation_runs").FindOne(ctx, bson.M{"domain_id": uint64(10)}).Decode(&before); err != nil {
		t.Fatal(err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	for range 2 {
		if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
			return nil, store.BackfillRetry(tx, baseline, proof.Clone())
		}); err != nil {
			t.Fatal(err)
		}
	}
	var after InterpretationRunPO
	if err := db.Collection("interpretation_runs").FindOne(ctx, bson.M{"domain_id": uint64(10)}).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.RetryEventEvidence == nil {
		t.Fatal("missing retry conclusion")
	}
	after.RetryEventEvidence = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("retry CAS altered ID, budget, schedule, failure, lease or origin fields")
	}
	if _, err := db.Collection("interpretation_runs").UpdateOne(ctx, bson.M{"domain_id": uint64(10)}, bson.M{"$set": bson.M{"policy_max_attempts": 8}}); err != nil {
		t.Fatal(err)
	}
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) { return nil, store.BackfillRetry(tx, baseline, proof) })
	if !errors.Is(err, retirementevidence.ErrConflict) {
		t.Fatalf("idempotent retry ignored changed budget: %v", err)
	}
}

func TestRetryHistoricalEvidenceCASRejectsChangedOriginalScheduleOrLifecycle(t *testing.T) {
	cases := []struct {
		collection, field string
		id                uint64
		value             any
	}{
		{"interpretation_runs", "retry_event_id", 10, "different"}, {"interpretation_runs", "action_request_id", 10, "new-action"}, {"interpretation_runs", "next_attempt_at", 10, time.Now().UTC()},
		{"interpretation_runs", "retry_policy_version", 10, "new-policy"}, {"interpretation_runs", "attempt_origin", 10, "manual"}, {"interpretation_runs", "failure.code", 10, "changed"}, {"interpretation_runs", "extra_history", 10, nil},
		{"report_generations", "version", 1, uint64(8)}, {"interpretation_runs", "status", 11, "running"},
	}
	for _, item := range cases {
		t.Run(item.collection+"."+item.field, func(t *testing.T) {
			client, db := mongodbtest.ReplicaSetDatabase(t)
			store, _ := retirementGraph(t, db)
			baseline, err := store.PrepareRetry(t.Context(), 10)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Collection(item.collection).UpdateOne(t.Context(), bson.M{"domain_id": item.id}, bson.M{"$set": bson.M{item.field: item.value}}); err != nil {
				t.Fatal(err)
			}
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(t.Context())
			_, err = session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
				return nil, store.BackfillRetry(tx, baseline, retirementInterpretationProof(baseline.EventID(), baseline.BindingSHA256()))
			})
			if !errors.Is(err, retirementevidence.ErrConflict) {
				t.Fatalf("changed retry baseline accepted: %v", err)
			}
		})
	}
}

func TestHistoricalInterpretationEvidenceDoesNotInventMissingFactsOrSwallowInfrastructureErrors(t *testing.T) {
	for _, item := range []struct {
		collection, field string
		id                uint64
		value             any
	}{
		{"interpretation_runs", "retry_event_id", 10, ""}, {"interpretation_runs", "action_request_id", 10, ""}, {"interpretation_runs", "next_attempt_at", 10, nil}, {"report_generations", "status", 1, "generating"}, {"interpretation_runs", "status", 11, "running"},
	} {
		t.Run(item.collection+"."+item.field, func(t *testing.T) {
			_, db := mongodbtest.ReplicaSetDatabase(t)
			store, _ := retirementGraph(t, db)
			if _, err := db.Collection(item.collection).UpdateOne(t.Context(), bson.M{"domain_id": item.id}, bson.M{"$set": bson.M{item.field: item.value}}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.PrepareRetry(t.Context(), 10); !errors.Is(err, retirementevidence.ErrUnverifiable) {
				t.Fatalf("missing or active fact was not explicitly blocked: %v", err)
			}
		})
	}
	_, db := mongodbtest.ReplicaSetDatabase(t)
	store, facts := retirementGraph(t, db)
	failure := errors.New("read-only SQL dependency unavailable")
	facts.err = failure
	if _, err := store.PrepareGenerated(t.Context(), 1); !errors.Is(err, failure) {
		t.Fatalf("infrastructure error became a historical gap: %v", err)
	}
	if _, err := store.PrepareRetry(t.Context(), 10); !errors.Is(err, failure) {
		t.Fatalf("infrastructure error became a historical gap: %v", err)
	}
	var typedNil *retirementOutcomeFacts
	if _, err := NewRetirementEvidenceStore(db, typedNil); !errors.Is(err, retirementevidence.ErrUnverifiable) {
		t.Fatalf("typed nil fact reader accepted: %v", err)
	}
}

func originalSourceFor(proof *evidence.EventEvidenceV1) OriginalSourceReference {
	return OriginalSourceReference{EventID: proof.EventID, Digest: proof.Digest, BusinessBindingSHA256: proof.BusinessBindingSHA256, Method: proof.Verification.Method, Version: proof.Verification.Version, OperationID: proof.Verification.OperationID}
}

func TestLegacyGeneratedOriginalIDIsCopiedFromExplicitVerifiedSourceWithAtomicEvidence(t *testing.T) {
	for _, initiallyPresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "empty"}[initiallyPresent], func(t *testing.T) {
			client, db := mongodbtest.ReplicaSetDatabase(t)
			ctx := t.Context()
			store, _ := retirementGraph(t, db)
			collection := db.Collection("report_generations")
			change := bson.M{"$unset": bson.M{"generated_event_id": ""}}
			if initiallyPresent {
				change = bson.M{"$set": bson.M{"generated_event_id": ""}}
			}
			if _, err := collection.UpdateOne(ctx, bson.M{"domain_id": uint64(1)}, change); err != nil {
				t.Fatal(err)
			}
			baseline, err := store.PrepareGenerated(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			proof := retirementInterpretationProof("original-dce-row-id", baseline.BindingSHA256())
			source := originalSourceFor(proof)
			if err := baseline.ValidateOriginalEvidence(source, proof); err != nil {
				t.Fatal(err)
			}
			if err := baseline.ValidateEvidence(proof); !errors.Is(err, retirementevidence.ErrConflict) {
				t.Fatalf("ordinary CAS silently copied an ID: %v", err)
			}
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(ctx)
			for range 2 {
				if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
					return nil, store.BackfillGeneratedOriginal(tx, baseline, source, proof.Clone())
				}); err != nil {
					t.Fatal(err)
				}
			}
			var actual ReportGenerationPO
			if err := collection.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&actual); err != nil {
				t.Fatal(err)
			}
			if actual.GeneratedEventID != source.EventID || actual.GeneratedEventEvidence.EventID != source.EventID || actual.Version != 7 || actual.ReportID != 20 || actual.LatestRunID != 11 {
				t.Fatal("original ID copy changed facts or lost independent identity")
			}
			if _, err := collection.UpdateOne(ctx, bson.M{"domain_id": uint64(1)}, bson.M{"$set": bson.M{"version": uint64(8)}}); err != nil {
				t.Fatal(err)
			}
			_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
				return nil, store.BackfillGeneratedOriginal(tx, baseline, source, proof)
			})
			if !errors.Is(err, retirementevidence.ErrConflict) {
				t.Fatalf("copied ID idempotence ignored changed business version: %v", err)
			}
		})
	}
}

func TestLegacyGeneratedOriginalSourceIDConflictsAndUniqueCollisionRollback(t *testing.T) {
	for _, uniqueCollision := range []bool{false, true} {
		t.Run(map[bool]string{false: "independent-id-changed", true: "unique-id-collision"}[uniqueCollision], func(t *testing.T) {
			client, db := mongodbtest.ReplicaSetDatabase(t)
			ctx := t.Context()
			store, _ := retirementGraph(t, db)
			collection := db.Collection("report_generations")
			if _, err := collection.UpdateOne(ctx, bson.M{"domain_id": uint64(1)}, bson.M{"$unset": bson.M{"generated_event_id": ""}}); err != nil {
				t.Fatal(err)
			}
			baseline, err := store.PrepareGenerated(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			proof := retirementInterpretationProof("original-dce-row-id", baseline.BindingSHA256())
			source := originalSourceFor(proof)
			if uniqueCollision {
				if _, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "generated_event_id", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"generated_event_id": bson.M{"$type": "string", "$gt": ""}})}); err != nil {
					t.Fatal(err)
				}
				if _, err := collection.InsertOne(ctx, bson.M{"domain_id": uint64(2), "generated_event_id": source.EventID}); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := collection.UpdateOne(ctx, bson.M{"domain_id": uint64(1)}, bson.M{"$set": bson.M{"generated_event_id": "independent-other-id"}}); err != nil {
					t.Fatal(err)
				}
			}
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(ctx)
			_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
				return nil, store.BackfillGeneratedOriginal(tx, baseline, source, proof)
			})
			if uniqueCollision && !mongo.IsDuplicateKeyError(err) {
				t.Fatalf("unique source ID collision did not abort: %v", err)
			}
			if !uniqueCollision && !errors.Is(err, retirementevidence.ErrConflict) {
				t.Fatalf("changed independent ID accepted: %v", err)
			}
			var actual bson.Raw
			if err := collection.FindOne(ctx, bson.M{"domain_id": uint64(1)}).Decode(&actual); err != nil {
				t.Fatal(err)
			}
			if actual.Lookup("generated_event_evidence").Type != 0 {
				t.Fatal("failed copy persisted a partial conclusion")
			}
			if uniqueCollision && actual.Lookup("generated_event_id").Type != 0 {
				t.Fatal("unique collision persisted a fabricated or partial ID")
			}
		})
	}
}

func TestLegacyGeneratedEvidenceKnownSourceAttestationCannotBeSubstituted(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	store, _ := retirementGraph(t, db)
	if _, err := db.Collection("report_generations").UpdateOne(t.Context(), bson.M{"domain_id": uint64(1)}, bson.M{"$unset": bson.M{"generated_event_id": ""}}); err != nil {
		t.Fatal(err)
	}
	baseline, err := store.PrepareGenerated(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	proof := retirementInterpretationProof("original-dce-row-id", baseline.BindingSHA256())
	for _, mutate := range []func(*OriginalSourceReference){func(s *OriginalSourceReference) { s.EventID = "substitute" }, func(s *OriginalSourceReference) {
		s.Digest = evidence.SourceDigest("mongo-selected-source-bson-v1", []byte("other"))
	}, func(s *OriginalSourceReference) { s.BusinessBindingSHA256 = evidence.BindingDigest("other") }, func(s *OriginalSourceReference) { s.Method = "other-method" }, func(s *OriginalSourceReference) { s.Version = "other-version" }, func(s *OriginalSourceReference) { s.OperationID = "other-batch" }, func(s *OriginalSourceReference) { s.Digest.Kind = evidence.SDKFingerprintKind }} {
		source := originalSourceFor(proof)
		mutate(&source)
		if err := baseline.ValidateOriginalEvidence(source, proof); err == nil {
			t.Fatal("substituted source reference accepted")
		}
	}
}

func TestLegacyGeneratedSameConclusionRequiresItsCopiedIndependentID(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	store, _ := retirementGraph(t, db)
	collection := db.Collection("report_generations")
	if _, err := collection.UpdateOne(ctx, bson.M{"domain_id": uint64(1)}, bson.M{"$unset": bson.M{"generated_event_id": ""}}); err != nil {
		t.Fatal(err)
	}
	baseline, err := store.PrepareGenerated(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	proof := retirementInterpretationProof("original-dce-row-id", baseline.BindingSHA256())
	source := originalSourceFor(proof)
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	if _, err := session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
		return nil, store.BackfillGeneratedOriginal(tx, baseline, source, proof)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := collection.UpdateOne(ctx, bson.M{"domain_id": uint64(1)}, bson.M{"$unset": bson.M{"generated_event_id": ""}}); err != nil {
		t.Fatal(err)
	}
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
		return nil, store.BackfillGeneratedOriginal(tx, baseline, source, proof)
	})
	if !errors.Is(err, retirementevidence.ErrConflict) {
		t.Fatalf("same conclusion silently repaired a corrupted independent ID: %v", err)
	}
}
