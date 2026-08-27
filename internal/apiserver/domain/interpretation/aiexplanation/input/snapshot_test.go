package input

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

func TestSnapshotFreezesCanonicalJSON(t *testing.T) {
	payload := []byte(`{"schema_version":"ai-explanation-input/v1"}`)
	snapshot, err := NewSnapshot(payload)
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = '['
	got := snapshot.CanonicalJSON()
	got[0] = '['
	if string(snapshot.CanonicalJSON()) != `{"schema_version":"ai-explanation-input/v1"}` {
		t.Fatal("snapshot bytes were mutated through caller-owned slices")
	}
}

func TestRestoreSnapshotRejectsFingerprintMismatch(t *testing.T) {
	_, err := RestoreSnapshot(
		aiexplanation.InputSchemaVersionV1,
		[]byte(`{"schema_version":"ai-explanation-input/v1"}`),
		aiexplanation.NewFingerprint([]byte(`{"different":true}`)),
	)
	if err == nil {
		t.Fatal("expected fingerprint mismatch")
	}
}
