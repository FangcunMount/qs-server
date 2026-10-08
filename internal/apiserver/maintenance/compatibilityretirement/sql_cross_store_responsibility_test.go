package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSQLMongoCrossStoreRejectsMissingOpaqueCapabilities(t *testing.T) {
	if _, err := PrepareSQLMongoCrossStoreResponsibilityPage(context.Background(), nil, nil, nil, nil); err == nil {
		t.Fatal("editable empty facts treated as closure")
	}
	for _, value := range []any{&SQLCrossStoreResponsibilityCatalog{}, &SQLMongoCrossStoreResponsibilityPage{}, SQLMongoCrossStoreResponsibilityView{}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private source/ledger JSON escaped")
		}
		if _, err := bson.Marshal(value); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private source/ledger BSON escaped")
		}
	}
	var p *SQLMongoCrossStoreResponsibilityPage
	if err := p.RecheckBusiness(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("nil fresh capability accepted")
	}
}

func TestSQLMongoCrossStoreOriginalWireDigestLayersAndIdentity(t *testing.T) {
	raw := wireFixture(t, "answersheet.submitted")
	source, err := decodeDomain(raw, outerEventFacts{engine: "mongodb", id: "original-event-1", eventType: "answersheet.submitted", aggregateType: "AnswerSheet", aggregateID: "sheet-1", org: strInt64(fixtureLargeID)})
	if err != nil {
		t.Fatal(err)
	}
	var inner originalDomainEnvelope
	if strictTyped(raw, &inner) != nil {
		t.Fatal("fixture wire")
	}
	base := sqlevaluation.SQLCrossStoreRow{Observation: sqlevaluation.SQLResponsibilityObservation{Store: "retry_event_hold", EventID: source.EventID, EventType: source.EventType, OwnerKind: source.AggregateType, OwnerID: source.AggregateID, OrgID: source.OrgID, TesteeID: source.Submitted.TesteeID, SDKFingerprintSHA256: sourceSHA([]byte("different SDK identity digest"))}, LegacyContentSHA256: source.ContentDigest.SHA256, InnerDataSHA256: sourceSHA(inner.Data)}
	// SQL's real domain decoder returns the same full wire contract. Source
	// row, domain content, body, and SDK digests deliberately remain distinct.
	var env domainwire.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	base.Inner = &env
	local := MongoLocalResolution{OrgID: source.OrgID, TesteeID: source.Submitted.TesteeID, OwnerLocalTerminal: true}
	for _, name := range []string{"valid", "event_type", "event_id", "time", "content_digest", "inner_body", "sql_namespace"} {
		t.Run(name, func(t *testing.T) {
			row := base
			copy := *base.Inner
			copy.Data = append([]byte(nil), base.Inner.Data...)
			row.Inner = &copy
			switch name {
			case "event_type":
				row.Observation.EventType = "evaluation.requested"
			case "event_id":
				row.Inner.ID = "other-id"
			case "time":
				row.Inner.OccurredAt = row.Inner.OccurredAt.Add(time.Millisecond)
			case "content_digest":
				row.LegacyContentSHA256 = sourceSHA([]byte("other bytes"))
			case "inner_body":
				row.Inner.Data = []byte(`{"unexpected":true}`)
			case "sql_namespace":
				row.Observation.Store = "rm_outbox"
			}
			var v SQLMongoCrossStoreResponsibilityView
			err := (&SQLMongoCrossStoreResponsibilityPage{}).verifyMongoMessage(&v, source, local, row)
			if name == "valid" {
				if err != nil || len(v.OwnerBoundObservationKeys) != 1 {
					t.Fatal("independent SDK digest mistaken for legacy source SHA", err)
				}
			} else if name == "sql_namespace" {
				if err != nil || !containsString(v.BlockingReasons, "cross_store_mongo_event_in_sql_rm_namespace") {
					t.Fatal("non-target namespace became empty/closed")
				}
			} else if err == nil {
				t.Fatal("conflicting original immutable wire accepted")
			}
		})
	}
}

func strInt64(v int64) *int64 { return &v }

func TestSQLMongoCrossStoreProvisionalCauseIsExact(t *testing.T) {
	o := sqlevaluation.SQLResponsibilityObservation{Store: "retry_event_hold", PrimaryKeySHA256: sourceSHA([]byte("original pk")), EventID: "original-event", EventType: "interpretation.report.generated", OrgID: 7, AssessmentID: 42, TesteeID: 9, State: "replayed", OwnerUnproven: true}
	r := sqlevaluation.SQLHistoricalResponsibility{Store: o.Store, ID: o.PrimaryKeySHA256, EventID: o.EventID, EventType: o.EventType, OrgID: o.OrgID, AssessmentID: o.AssessmentID, TesteeID: o.TesteeID, State: o.State, Invalid: true}
	if !sqlMongoProvisionalMatches(r, o) {
		t.Fatal("exact provisional owner cause not recognized")
	}
	for _, name := range []string{"real_invalid", "another_reason", "another_pk", "another_type", "another_org", "already_valid", "different_state", "different_pending", "different_lease"} {
		t.Run(name, func(t *testing.T) {
			current, observed := r, o
			switch name {
			case "real_invalid":
				observed.Invalid = true
			case "another_reason":
				observed.Reasons = []string{"sdk_fingerprint_or_wire_conflict"}
			case "another_pk":
				observed.PrimaryKeySHA256 = sourceSHA([]byte("another pk"))
			case "another_type":
				observed.EventType = "answersheet.submitted"
			case "another_org":
				observed.OrgID = 8
			case "already_valid":
				current.Invalid = false
			case "different_state":
				observed.State = "blocked"
			case "different_pending":
				observed.Unfinished = true
			case "different_lease":
				observed.LeasePresent = true
			}
			if sqlMongoProvisionalMatches(current, observed) {
				t.Fatal("unrelated/real invalid cause cleared")
			}
		})
	}
}
