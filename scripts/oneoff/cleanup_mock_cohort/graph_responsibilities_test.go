package main

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestGraphOriginalStandardWireBindsPendingHoldToItsActualTargetRoot(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	evt := event.Event[map[string]any]{BaseEvent: event.BaseEvent{ID: "cohort-original-standard", EventTypeValue: eventcatalog.AnswerSheetSubmitted, AggregateTypeValue: "AnswerSheet", AggregateIDValue: "20", OccurredAtValue: time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC)}, Data: map[string]any{"org_id": 1, "testee_id": 1}}
	prepared, err := standardoutbox.PrepareIntents([]event.DomainEvent{evt}, eventcatalog.NewCatalog(cfg), "api-server")
	if err != nil {
		t.Fatal(err)
	}
	in := prepared[0].Message.Input()
	ref := standardoutbox.ReferenceFromMessage(prepared[0].Message)
	fp, err := hex.DecodeString(ref.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []string{"mysql", "mongo"} {
		for _, state := range []string{"pending", "published"} {
			t.Run(store+"/"+state, func(t *testing.T) {
				dir := privateTestDir(t)
				index := graphIndex{Counts: map[string]int64{}, Holds: map[string]int64{}, Retained: map[string]string{}, RequiredActions: map[string]int64{}}
				planner := cohortPlanner{writer: &graphWriter{dir: dir, index: &index, protected: map[string]bool{}}, selector: cohortSelector{Selection: selection{OrgID: "1"}}, dir: dir, referenceOwners: map[string]graphObject{referenceKey(ref.Producer, ref.EventID, ref.Destination): {Links: map[string]string{"testee_id": "1"}}}}
				if store == "mysql" {
					r := graphSQLFixture("rm_outbox", "1", nil)
					for name, value := range map[string]string{"message_id": in.ID, "producer": in.Producer, "destination": in.Destination, "event_type": in.EventType, "schema_version": in.SchemaVersion, "scope": in.Scope, "content_type": in.ContentType, "occurred_at": in.OccurredAt, "state": state} {
						r.Columns = append(r.Columns, cell{Name: name, Type: "VARBINARY", Raw: []byte(value)})
					}
					r.Columns = append(r.Columns, cell{Name: "payload", Type: "LONGBLOB", Raw: in.Payload}, cell{Name: "fingerprint", Type: "BINARY", Raw: fp})
					err = planner.standardSQL(r, ref)
				} else {
					raw, e := bson.Marshal(bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "message_id", Value: in.ID}, {Key: "producer", Value: in.Producer}, {Key: "destination", Value: in.Destination}, {Key: "event_type", Value: in.EventType}, {Key: "schema_version", Value: in.SchemaVersion}, {Key: "scope", Value: in.Scope}, {Key: "content_type", Value: in.ContentType}, {Key: "occurred_at", Value: in.OccurredAt}, {Key: "payload", Value: in.Payload}, {Key: "fingerprint", Value: fp}, {Key: "state", Value: state}, {Key: "lease_until", Value: nil}})
					if e != nil {
						t.Fatal(e)
					}
					r, e := graphMongoObject("rm_outbox", raw, []byte("fixed original namespace"))
					if e != nil {
						t.Fatal(e)
					}
					err = planner.standardMongo(r, ref)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err = planner.writer.flush(); err != nil {
					t.Fatal(err)
				}
				if err = planner.flushHolds(); err != nil {
					t.Fatal(err)
				}
				if len(index.Shards) != 1 || index.Shards[0].RestoreRank != 0 {
					t.Fatal("original wire must stay in a readonly retained scope")
				}
				page, e := readGraphPage(dir, index.Shards[0])
				if e != nil {
					t.Fatal(e)
				}
				if len(page.Objects[0].RootIDs) != 1 || page.Objects[0].RootIDs[0] != "1" {
					t.Fatal("wire lost its typed business root")
				}
				err = validateGraphHolds(dir, index, map[string]bool{"1": true})
				if state == "pending" {
					if err == nil || len(index.HoldShards) != 1 {
						t.Fatal("pending publish responsibility failed to block its selected root")
					}
					if e = validateGraphHolds(dir, index, map[string]bool{"2": true}); e != nil {
						t.Fatal("unrelated root was globally blocked", e)
					}
				} else if err != nil || len(index.Holds) != 0 || index.Retained[store+"/rm_outbox"] == "" {
					t.Fatal("retention alone must not invent pending replay or deletion work", err)
				}
			})
		}
	}
}

func TestGraphDerivedParentRootsExistBeforePendingAndEvidenceClassification(t *testing.T) {
	for _, kind := range []string{"runtime", "reminder", "generation", "run"} {
		t.Run(kind, func(t *testing.T) {
			reader, selector, protection, dir := graphFixture(t)
			reason := ""
			switch kind {
			case "runtime":
				r := graphSQLFixture("runtime_checkpoint", "90", map[string]string{"assessment_id": "100"})
				r.Columns = append(r.Columns, cell{Name: "scope", Type: "VARCHAR", Raw: []byte("unknown_scope")})
				reader.sqlRows["runtime_checkpoint"] = []record{r}
				reason = "runtime_checkpoint:unknown_scope"
			case "reminder":
				r := graphSQLFixture("task_opened_reminder_delivery", "91", map[string]string{"org_id": "1"})
				r.Columns = append(r.Columns, cell{Name: "task_id", Type: "VARBINARY", Raw: []byte("400")}, cell{Name: "state", Type: "VARCHAR", Raw: []byte("pending")})
				reader.sqlRows["task_opened_reminder_delivery"] = []record{r}
				reason = "task_opened_reminder_delivery:unsettled_external_responsibility"
			case "generation", "run":
				name, key := "report_generations", "generated_event_evidence"
				if kind == "run" {
					name, key = "interpretation_runs", "retry_event_evidence"
				}
				r := reader.mongoRows[name][0]
				var doc bson.D
				if err := bson.Unmarshal(r.BSON, &doc); err != nil {
					t.Fatal(err)
				}
				doc = append(doc, bson.E{Key: key, Value: bson.D{}})
				raw, err := bson.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				reader.mongoRows[name][0].BSON = raw
				reason = "mongo/" + name + ":invalid_event_evidence"
			}
			index, err := planCohort(t.Context(), reader, selector, protection, dir, digest(encode(selector)), strings.Repeat("a", 40))
			if err != nil {
				t.Fatal(err)
			}
			if index.Holds[reason] != 1 {
				t.Fatal("typed derived responsibility was not classified", reason, index.Holds)
			}
			if err = validateGraphHolds(dir, index, map[string]bool{"1": true}); err == nil {
				t.Fatal("pending/evidence failure lost its parent component before hold classification")
			}
			if err = validateGraphHolds(dir, index, map[string]bool{"999": true}); err != nil {
				t.Fatal("unrelated component was globally blocked", err)
			}
		})
	}
}
