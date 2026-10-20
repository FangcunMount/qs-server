package evidence

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestEvidenceUnionKeepsFlatHistoricalJSONAndExactTransactionReference(t *testing.T) {
	current, _ := transactionEvidence(t)
	legacy, err := NewStandard(current.Reference, current.BusinessBindingSHA256)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []Record{{V1: legacy}, {V2: current}} {
		raw, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		var expected []byte
		if entry.V1 != nil {
			expected, err = json.Marshal(entry.V1)
		} else {
			expected, err = json.Marshal(entry.V2)
		}
		if err != nil || !bytes.Equal(raw, expected) {
			t.Fatal("existing JSON column shape was wrapped or changed")
		}
		var decoded Record
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.EventID() != entry.EventID() || (decoded.V1 == nil) != (entry.V1 == nil) {
			t.Fatal("historical evidence was promoted or identity lost")
		}
		if decoded.V2 != nil {
			if !bytes.Equal(decoded.V2.TransactionReference, current.TransactionReference) {
				t.Fatal("reference bytes changed")
			}
			copy := decoded.Clone()
			copy.V2.TransactionReference[0] ^= 1
			if decoded.V2.Validate() != nil {
				t.Fatal("clone shares mutable reference bytes")
			}
		}
	}
}

func TestEvidenceUnionRejectsAmbiguousAndUnknownVersions(t *testing.T) {
	current, _ := transactionEvidence(t)
	legacy, err := NewStandard(current.Reference, current.BusinessBindingSHA256)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []Record{{}, {V1: legacy, V2: current}} {
		if _, err := json.Marshal(entry); err == nil {
			t.Fatal("ambiguous evidence persisted")
		}
	}
	var entry Record
	if err := json.Unmarshal([]byte(`{"version":99}`), &entry); err == nil {
		t.Fatal("unknown version accepted")
	}
}
