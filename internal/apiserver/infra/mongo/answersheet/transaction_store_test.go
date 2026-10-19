package answersheet

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

func TestSubmissionProtocolCannotDowngradeTransactionEvidenceToLegacyAcceptance(t *testing.T) {
	legacy := &DurableAcceptancePO{SchemaVersion: 1, EventEvidence: &evidence.EventEvidenceV1{}}
	if tx, err := submissionProtocol(legacy); tx || err != nil {
		t.Fatal("historical read contract changed")
	}
	legacy.TransactionEvidence = &evidence.EventEvidenceV2{}
	if _, err := submissionProtocol(legacy); !errors.Is(err, ErrSubmissionUnresolved) {
		t.Fatal("mixed version bypasses exact witness confirmation")
	}
	legacy.SchemaVersion = 2
	if _, err := submissionProtocol(legacy); !errors.Is(err, ErrSubmissionUnresolved) {
		t.Fatal("V1 evidence accepted in a transaction marker")
	}
	legacy.EventEvidence = nil
	if tx, err := submissionProtocol(legacy); !tx || err != nil {
		t.Fatal("transaction marker must select the precise authority reader")
	}
	legacy.TransactionEvidence = nil
	if _, err := submissionProtocol(legacy); !errors.Is(err, ErrSubmissionUnresolved) {
		t.Fatal("missing transaction evidence selects historical acceptance")
	}
	legacy.SchemaVersion = 99
	if _, err := submissionProtocol(legacy); !errors.Is(err, ErrSubmissionUnresolved) {
		t.Fatal("unknown version accepted")
	}
	if tx, err := submissionProtocol(nil); tx || err != nil {
		t.Fatal("pre-marker historical rows changed")
	}
}
