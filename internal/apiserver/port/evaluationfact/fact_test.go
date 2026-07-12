package evaluationfact

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestRecordOwnsImmutablePayloadCopies(t *testing.T) {
	source := []byte(`{"value":1}`)
	record := NewRecord(NewRecordInput{ID: meta.FromUint64(1), Payload: source, ReportInput: source})
	source[2] = 'X'
	first := record.Payload()
	first[2] = 'Y'
	if got := string(record.Payload()); got != `{"value":1}` {
		t.Fatalf("payload mutated through caller copy: %s", got)
	}
	if got := string(record.ReportInput()); got != `{"value":1}` {
		t.Fatalf("report input mutated through source: %s", got)
	}
}
