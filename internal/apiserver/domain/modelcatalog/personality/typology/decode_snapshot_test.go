package typology

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDecodeFromSnapshotMBTIMigration(t *testing.T) {
	legacy := &MBTILegacyModel{
		Code:                 "MBTI_TEST",
		Version:              "1.0.0",
		QuestionnaireCode:    "MBTI_TEST",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		DimensionOrder:       []string{"EI"},
		Dimensions: map[string]MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"},
		},
		TypeProfiles: []MBTILegacyTypeProfile{
			{TypeCode: "INTJ", TypeName: "建筑师"},
		},
	}
	payloadBytes, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	snapshot := &domain.Snapshot{
		Definition: domain.Definition{
			Kind:    domain.KindMBTIMigration,
			Code:    legacy.Code,
			Version: legacy.Version,
		},
		PayloadFormat: domain.PayloadFormatMBTIV1,
		Payload:       payloadBytes,
	}

	got, err := DecodeFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("DecodeFromSnapshot: %v", err)
	}
	if got.Algorithm != domain.AlgorithmMBTI || got.Code != "MBTI_TEST" {
		t.Fatalf("payload = %#v", got)
	}
}

func TestDecodeFromSnapshotTypologyV2(t *testing.T) {
	payload := &Payload{
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

	got, err := DecodeFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("DecodeFromSnapshot: %v", err)
	}
	if got.Algorithm != domain.AlgorithmBigFive {
		t.Fatalf("Algorithm = %s, want bigfive", got.Algorithm)
	}
}
