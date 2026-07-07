package typology_test

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestDecodeFromSnapshotTypologyV2(t *testing.T) {
	payload := &typology.Payload{
		Code:      "BIGFIVE_V1",
		Version:   "1.0.0",
		Algorithm: domain.AlgorithmBigFive,
		Status:    "published",
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	snapshot := &domain.Snapshot{
		Definition: domain.Definition{
			Kind:    domain.KindPersonality,
			Code:    payload.Code,
			Version: payload.Version,
		},
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       payloadBytes,
	}

	got, err := typology.DecodeFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("DecodeFromSnapshot: %v", err)
	}
	if got.Algorithm != domain.AlgorithmBigFive {
		t.Fatalf("Algorithm = %s, want bigfive", got.Algorithm)
	}
}

func TestDecodeFromSnapshotRejectsLegacyFlatFormat(t *testing.T) {
	snapshot := &domain.Snapshot{
		Definition: domain.Definition{
			Kind:    domain.RuleSetKindMBTI,
			Code:    "MBTI_TEST",
			Version: "1.0.0",
		},
		PayloadFormat: domain.PayloadFormatMBTIV1,
		Payload:       []byte(`{"code":"MBTI_TEST"}`),
	}
	if _, err := typology.DecodeFromSnapshot(snapshot); err == nil {
		t.Fatal("expected error for legacy flat mbti payload format")
	}
}
