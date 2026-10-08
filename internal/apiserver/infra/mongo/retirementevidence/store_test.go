package retirementevidence

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
)

func TestHistoricalSnapshotPreservesBSONPresenceAndRejectsAmbiguousFacts(t *testing.T) {
	encode := func(value any) bson.Raw {
		t.Helper()
		raw, err := bson.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	absent, _, err := withoutEvidence(encode(bson.D{{Key: "_id", Value: "1"}}), []string{"generated_event_evidence"})
	if err != nil {
		t.Fatal(err)
	}
	null, _, err := withoutEvidence(encode(bson.D{{Key: "_id", Value: "1"}, {Key: "optional", Value: nil}}), []string{"generated_event_evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(absent, null) {
		t.Fatal("missing and null business fields became equivalent")
	}
	for _, value := range []bson.D{
		{{Key: "_id", Value: "1"}, {Key: "org_id", Value: 7}, {Key: "org_id", Value: 8}},
		{{Key: "_id", Value: "1"}, {Key: "model", Value: bson.D{{Key: "code", Value: "one"}, {Key: "code", Value: "other"}}}},
		{{Key: "_id", Value: "1"}, {Key: "frozen_array", Value: bson.A{bson.D{{Key: "identity", Value: "one"}, {Key: "identity", Value: "other"}}}}},
	} {
		if _, _, err := withoutEvidence(encode(value), []string{"generated_event_evidence"}); !errors.Is(err, ErrUnverifiable) {
			t.Fatalf("ambiguous stored facts were trusted: %v", err)
		}
	}
}

func TestHistoricalMaintenanceProofRequiresExactIndependentIDBindingAndClosure(t *testing.T) {
	binding := evidence.BindingDigest("business", evidence.String("original"))
	proof := &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: "original-event", Digest: evidence.SourceDigest("mongo-selected-source-bson-v1", []byte("original")), BusinessBindingSHA256: binding, Origin: "retirement", Verification: evidence.Verification{Method: "verified-original-source", Version: "v1", OperationID: "batch", VerifiedAt: time.Now().UTC(), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}
	if err := ValidateHistorical(proof, proof.EventID, binding); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*evidence.EventEvidenceV1){func(p *evidence.EventEvidenceV1) { p.EventID = "different" }, func(p *evidence.EventEvidenceV1) { p.BusinessBindingSHA256 = evidence.BindingDigest("different") }, func(p *evidence.EventEvidenceV1) { p.Digest.Kind = evidence.SDKFingerprintKind }, func(p *evidence.EventEvidenceV1) { p.Verification.OwnershipVerified = false }, func(p *evidence.EventEvidenceV1) { p.Verification.ResponsibilityClosed = false }} {
		bad := proof.Clone()
		mutate(bad)
		if err := ValidateHistorical(bad, proof.EventID, binding); err == nil {
			t.Fatal("inconsistent or open historical responsibility was accepted")
		}
	}
	ref := evidence.StandardReference{EventID: "original-event", Producer: "qs-server", Destination: "topic", EventType: "business", SchemaVersion: "v1", Scope: "org:7", ContentType: "application/json", OccurredAt: "2026-10-08T00:00:00Z", Fingerprint: evidence.SourceDigest("source", []byte("bytes")).SHA256}
	standard, err := evidence.NewStandard(ref, binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHistorical(standard, ref.EventID, binding); err == nil {
		t.Fatal("historical adapter pretended to verify a current standard message")
	}
}
