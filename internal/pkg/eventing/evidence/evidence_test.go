package evidence

import (
	"strings"
	"testing"
	"time"
)

func testReference() StandardReference {
	return StandardReference{EventID: "event-1", Producer: "qs-server", Destination: "qs.evaluation.lifecycle", EventType: "answersheet.submitted", SchemaVersion: "v1", Scope: "org:7", ContentType: "application/json", OccurredAt: "2026-10-08T00:00:00Z", Fingerprint: strings.Repeat("a", 64)}
}

func TestEvidenceCloneAndClassBoundaries(t *testing.T) {
	original, err := NewStandard(testReference(), strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	cloned := original.Clone()
	cloned.Reference.EventID = "changed"
	if original.Reference.EventID != "event-1" {
		t.Fatal("clone mutated original reference")
	}
	if cloned.Validate() == nil {
		t.Fatal("reference identity mismatch accepted")
	}
	historical := &EventEvidenceV1{Version: 1, Class: RetiredVerified, EventID: "old-event", Digest: SourceDigest("mysql_select_binary_source_sha256_v1", []byte("old source")), BusinessBindingSHA256: strings.Repeat("b", 64), Origin: "legacy_mysql", Verification: Verification{OperationID: "retire-1", Method: "business-and-source", Version: "v1", VerifiedAt: time.Now(), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}
	if err := historical.Validate(); err != nil {
		t.Fatal(err)
	}
	historical.Reference = &StandardReference{}
	if historical.Validate() == nil {
		t.Fatal("historical conclusion impersonated standard evidence")
	}
	historical.Reference = nil
	historical.Verification.ResponsibilityClosed = false
	if historical.Validate() == nil {
		t.Fatal("open responsibility retired")
	}
	historical.Verification.ResponsibilityClosed = true
	historical.Class = Unverifiable
	historical.EventID = ""
	historical.Digest = Digest{}
	if historical.Validate() == nil {
		t.Fatal("gap without reason accepted")
	}
	historical.Verification.Reason = "source_message_missing"
	if err := historical.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBindingDigestPreservesPresenceTypeOrderAndStoredTime(t *testing.T) {
	values := []string{BindingDigest("type", nil), BindingDigest("type", String("")), BindingDigest("other", String("")), BindingDigest("type", String("a"), String("bc")), BindingDigest("type", String("ab"), String("c"))}
	seen := map[string]bool{}
	for _, value := range values {
		if !ValidSHA256(value) || seen[value] {
			t.Fatal("ambiguous binding digest")
		}
		seen[value] = true
	}
	stamp := time.Date(2026, 10, 8, 8, 0, 0, 123456789, time.FixedZone("UTC+8", 28800))
	if MillisecondTime(stamp) != "2026-10-08T00:00:00.123Z" {
		t.Fatal("stored time precision changed")
	}
}
