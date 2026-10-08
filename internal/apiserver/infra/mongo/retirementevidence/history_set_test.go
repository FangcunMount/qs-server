package retirementevidence

import (
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"testing"
	"time"
)

func TestHistoricalSetBSONRejectsHiddenBodiesAndAmbiguousFields(t *testing.T) {
	digest := evidence.SourceDigest("selected-original-bson-v1", []byte("source"))
	set := &evidence.HistoricalReferenceSetV1{Version: 1, Entries: []evidence.HistoricalReferenceEntryV1{{EventID: "old-id", EventType: "answersheet.submitted", Source: evidence.HistoricalSourceReferenceV1{Database: "mongodb", Object: "domain_event_outbox", PrimaryKeyKind: "mongodb_objectid", PrimaryKeySHA256: evidence.SourceDigest("objectid", []byte("id")).SHA256, Digest: digest}, Proof: &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: "old-id", Digest: digest, BusinessBindingSHA256: evidence.BindingDigest("frozen-business"), Origin: "retirement", Verification: evidence.Verification{Method: "original-business-source", Version: "v1", OperationID: "12345-1", VerifiedAt: time.Date(2026, 10, 8, 3, 4, 5, 678000000, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}}}}
	body, err := bson.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	var fields bson.D
	if err := bson.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"valid", "unknown_body", "duplicate_slot", "duplicate_nested", "null_slot", "wrong_type", "wrong_version"} {
		t.Run(kind, func(t *testing.T) {
			value := any(fields)
			copyFields := append(bson.D(nil), fields...)
			switch kind {
			case "unknown_body":
				value = append(copyFields, bson.E{Key: "body", Value: "forbidden"})
			case "duplicate_nested":
				value = append(copyFields, bson.E{Key: "version", Value: 1})
			case "null_slot":
				value = nil
			case "wrong_type":
				value = "body"
			case "wrong_version":
				copyFields[0].Value = 2
				value = copyFields
			}
			row := bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "legacy_submission_evidence", Value: value}}
			if kind == "duplicate_slot" {
				row = append(row, bson.E{Key: "legacy_submission_evidence", Value: value})
			}
			raw, err := bson.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := DecodeHistoricalSetDocument(raw, "legacy_submission_evidence")
			if kind == "valid" {
				if err != nil || actual == nil || len(actual.Entries) != 1 {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("malformed compact history accepted")
			}
		})
	}
}
