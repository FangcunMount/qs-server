//go:build integration

package interpretation

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func generatedHistoryIndex(t *testing.T, c *mongo.Collection) {
	t.Helper()
	if _, err := NewReportRepository(c.Database()); err != nil {
		t.Fatal(err)
	}
	key := "historical_generated_evidence.entries.event_id"
	if _, err := c.Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: key, Value: 1}}, Options: options.Index().SetUnique(true).SetCollation(&options.Collation{Locale: "simple"}).SetPartialFilterExpression(bson.M{key: bson.M{"$type": "string"}})}); err != nil {
		t.Fatal(err)
	}
}
func generatedHistoryReference(id string, b *HistoricalGeneratedBaseline) evidence.HistoricalReferenceEntryV1 {
	digest := evidence.SourceDigest("mongo-selected-source-bson-v1", []byte(id))
	p := b.Payload()
	return evidence.HistoricalReferenceEntryV1{EventID: id, EventType: "interpretation.report.generated", Source: evidence.HistoricalSourceReferenceV1{Database: "mongodb", Object: "domain_event_outbox", PrimaryKeyKind: "mongodb_objectid", PrimaryKeySHA256: evidence.SourceDigest("objectid", []byte(id)).SHA256, Digest: digest}, Run: &evidence.HistoricalRunReferenceV1{RunID: p.RunID, Attempt: p.Attempt}, Proof: &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: id, Digest: digest, BusinessBindingSHA256: b.BindingSHA256(), Origin: "retirement", Verification: evidence.Verification{Method: "original-source-and-original-report-graph", Version: "v1", OperationID: "12345-1", VerifiedAt: time.Date(2026, 10, 8, 3, 4, 5, 678000000, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}}
}

func TestHistoricalGeneratedMultipleOriginalIDsDoNotChooseSingleSlot(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	store, _ := retirementGraph(t, db)
	c := db.Collection("report_generations")
	generatedHistoryIndex(t, c)
	baseline, err := store.PrepareHistoricalGenerated(t.Context(), 1, 20, 11)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(t.Context())
	var before ReportGenerationPO
	if err := c.FindOne(t.Context(), bson.M{"domain_id": uint64(1)}).Decode(&before); err != nil {
		t.Fatal(err)
	}
	abort := errors.New("host rollback")
	first := generatedHistoryReference("original-source-one", baseline)
	if _, err := session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
		if err := store.BackfillHistoricalGenerated(tx, baseline, first); err != nil {
			return nil, err
		}
		return nil, abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	for _, entry := range []evidence.HistoricalReferenceEntryV1{first, first, generatedHistoryReference("original-source-two", baseline)} {
		if _, err := session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
			return nil, store.BackfillHistoricalGenerated(tx, baseline, entry)
		}); err != nil {
			t.Fatal(err)
		}
	}
	var after ReportGenerationPO
	if err := c.FindOne(t.Context(), bson.M{"domain_id": uint64(1)}).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.HistoricalGeneratedEvidence == nil || len(after.HistoricalGeneratedEvidence.Entries) != 2 {
		t.Fatal("lost an original source identity")
	}
	saved := after.HistoricalGeneratedEvidence
	after.HistoricalGeneratedEvidence = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("history chose an ID or changed business graph")
	}
	// A normal writer reconstructed without the maintenance slot preserves it.
	repo, err := NewGenerationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	native, err := generation.Restore(generation.RestoreInput{ID: meta.FromUint64(1), Key: generation.Key{OutcomeID: meta.FromUint64(50), ReportType: "assessment", TemplateVersion: "v1"}, Status: generation.StatusFailed, LatestRunID: meta.FromUint64(11), Version: 8, CreatedAt: after.CreatedAt, UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(t.Context(), native, 7); err != nil {
		t.Fatal(err)
	}
	var ordinary ReportGenerationPO
	if err := c.FindOne(t.Context(), bson.M{"domain_id": uint64(1)}).Decode(&ordinary); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, ordinary.HistoricalGeneratedEvidence) || ordinary.GeneratedEventID != before.GeneratedEventID || ordinary.GeneratedEventEvidence != nil {
		t.Fatal("normal writer nil-overwrote history or rewrote standard slot")
	}
}

func TestHistoricalGeneratedOriginalGraphAndSourceConflictsRefused(t *testing.T) {
	for _, kind := range []string{"missing_original_report", "wrong_original_run", "artifact_ownership", "run_pending", "current_pending", "unknown_outcome", "ambiguous_original"} {
		t.Run(kind, func(t *testing.T) {
			_, db := mongodbtest.ReplicaSetDatabase(t)
			store, facts := retirementGraph(t, db)
			generatedHistoryIndex(t, db.Collection("report_generations"))
			report, run := uint64(20), uint64(11)
			switch kind {
			case "missing_original_report":
				report = 999
			case "wrong_original_run":
				run = 10
			case "artifact_ownership":
				if _, err := db.Collection("interpret_report_artifacts").UpdateOne(t.Context(), bson.M{"domain_id": uint64(20)}, bson.M{"$set": bson.M{"testee_id": uint64(999)}}); err != nil {
					t.Fatal(err)
				}
			case "run_pending":
				if _, err := db.Collection("interpretation_runs").UpdateOne(t.Context(), bson.M{"domain_id": uint64(11)}, bson.M{"$set": bson.M{"status": "running"}}); err != nil {
					t.Fatal(err)
				}
			case "current_pending":
				if _, err := db.Collection("report_generations").UpdateOne(t.Context(), bson.M{"domain_id": uint64(1)}, bson.M{"$set": bson.M{"status": "running"}}); err != nil {
					t.Fatal(err)
				}
			case "unknown_outcome":
				facts.record = nil
			case "ambiguous_original":
				if _, err := db.Collection("interpret_report_artifacts").Indexes().DropOne(t.Context(), "uk_artifact_generation_id"); err != nil {
					t.Fatal(err)
				}
				var a InterpretReportPO
				if err := db.Collection("interpret_report_artifacts").FindOne(t.Context(), bson.M{"domain_id": uint64(20)}).Decode(&a); err != nil {
					t.Fatal(err)
				}
				a.ID = [12]byte{}
				a.DomainID = meta.FromUint64(21)
				if _, err := db.Collection("interpret_report_artifacts").InsertOne(t.Context(), a); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.PrepareHistoricalGenerated(t.Context(), 1, report, run); !errors.Is(err, retirementevidence.ErrUnverifiable) {
				t.Fatalf("bad original graph accepted: %v", err)
			}
		})
	}
	client, db := mongodbtest.ReplicaSetDatabase(t)
	store, _ := retirementGraph(t, db)
	generatedHistoryIndex(t, db.Collection("report_generations"))
	b, err := store.PrepareHistoricalGenerated(t.Context(), 1, 20, 11)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(t.Context())
	for _, kind := range []string{"run", "attempt", "binding", "type"} {
		t.Run(kind, func(t *testing.T) {
			entry := generatedHistoryReference("original-id", b)
			switch kind {
			case "run":
				entry.Run.RunID = "10"
			case "attempt":
				entry.Run.Attempt = 9
			case "binding":
				entry.Proof.BusinessBindingSHA256 = evidence.BindingDigest("different")
			case "type":
				entry.EventType = "answersheet.submitted"
			}
			if _, err := session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
				return nil, store.BackfillHistoricalGenerated(tx, b, entry)
			}); err == nil {
				t.Fatal("different source/run binding accepted")
			}
		})
	}
	if _, err := db.Collection("interpret_report_artifacts").UpdateOne(t.Context(), bson.M{"domain_id": uint64(20)}, bson.M{"$set": bson.M{"conclusion": "changed body"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
		return nil, store.BackfillHistoricalGenerated(tx, b, generatedHistoryReference("original-id", b))
	}); !errors.Is(err, retirementevidence.ErrConflict) {
		t.Fatal("stale immutable original graph accepted", err)
	}
}

func TestHistoricalGeneratedConcurrentAppendKeepsBothOriginalIDs(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	store, _ := retirementGraph(t, db)
	generatedHistoryIndex(t, db.Collection("report_generations"))
	b, err := store.PrepareHistoricalGenerated(t.Context(), 1, 20, 11)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"concurrent-one", "concurrent-two"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			session, err := client.StartSession()
			if err != nil {
				results <- err
				return
			}
			defer session.EndSession(t.Context())
			<-start
			_, err = session.WithTransaction(t.Context(), func(tx mongo.SessionContext) (any, error) {
				return nil, store.BackfillHistoricalGenerated(tx, b, generatedHistoryReference(id, b))
			})
			results <- err
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var row ReportGenerationPO
	if err := db.Collection("report_generations").FindOne(t.Context(), bson.M{"domain_id": uint64(1)}).Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.HistoricalGeneratedEvidence == nil || len(row.HistoricalGeneratedEvidence.Entries) != 2 {
		t.Fatal("concurrent CAS lost a historical ID")
	}
}
