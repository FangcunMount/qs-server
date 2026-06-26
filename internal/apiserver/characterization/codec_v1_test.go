package characterization_test

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
)

// V1 contract: legacy ruleset.* payload formats remain readable; v2 encode writes assessmentmodel.*.
func TestV1CodecLegacyDecodeAndV2EncodeFormats(t *testing.T) {
	t.Parallel()

	legacyDecodeCases := []struct {
		name       string
		wantFormat string
		wantKind   domain.Kind
		wantCode   string
		build      func(t *testing.T) (*domain.Snapshot, func(t *testing.T, snapshot *domain.Snapshot) error)
	}{
		{
			name:       "legacy scale decode",
			wantFormat: domain.PayloadFormatScaleV1,
			wantKind:   domain.KindScale,
			wantCode:   "PHQ9",
			build: func(t *testing.T) (*domain.Snapshot, func(t *testing.T, snapshot *domain.Snapshot) error) {
				payload, err := json.Marshal(&scalesnapshot.ScaleSnapshot{
					Code:         "PHQ9",
					ScaleVersion: "1.0.0",
					Status:       "published",
					Factors:      []scalesnapshot.FactorSnapshot{{Code: "total", IsTotalScore: true}},
				})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				snapshot := &domain.Snapshot{
					SchemaVersion: domain.SchemaVersionV1,
					PayloadFormat: domain.PayloadFormatScaleV1,
					Definition:    domain.Definition{Kind: domain.KindScale, Code: "PHQ9"},
					Payload:       payload,
				}
				return snapshot, func(t *testing.T, snapshot *domain.Snapshot) error {
					got, err := codec.DecodeScale(snapshot)
					if err != nil {
						return err
					}
					if got.Code != "PHQ9" {
						t.Fatalf("scale decode = %#v", got)
					}
					return nil
				}
			},
		},
		{
			name:       "legacy mbti decode",
			wantFormat: domain.PayloadFormatMBTIV1,
			wantKind:   domain.KindMBTIMigration,
			wantCode:   "MBTI_OEJTS",
			build: func(t *testing.T) (*domain.Snapshot, func(t *testing.T, snapshot *domain.Snapshot) error) {
				payload, err := json.Marshal(&modeltypology.MBTILegacyModel{Code: "MBTI_OEJTS", Version: "1.0.0"})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				snapshot := &domain.Snapshot{
					PayloadFormat: domain.PayloadFormatMBTIV1,
					Definition:    domain.Definition{Kind: domain.KindMBTIMigration, Code: "MBTI_OEJTS"},
					Payload:       payload,
				}
				return snapshot, func(t *testing.T, snapshot *domain.Snapshot) error {
					got, err := codec.DecodeMBTI(snapshot)
					if err != nil {
						return err
					}
					if got.Code != "MBTI_OEJTS" {
						t.Fatalf("mbti decode = %#v", got)
					}
					return nil
				}
			},
		},
		{
			name:       "legacy sbti decode",
			wantFormat: domain.PayloadFormatSBTIV1,
			wantKind:   domain.KindSBTIMigration,
			wantCode:   "SBTI_FUN",
			build: func(t *testing.T) (*domain.Snapshot, func(t *testing.T, snapshot *domain.Snapshot) error) {
				payload, err := json.Marshal(&modeltypology.SBTILegacyModel{Code: "SBTI_FUN", Version: "1.0.0"})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				snapshot := &domain.Snapshot{
					PayloadFormat: domain.PayloadFormatSBTIV1,
					Definition:    domain.Definition{Kind: domain.KindSBTIMigration, Code: "SBTI_FUN"},
					Payload:       payload,
				}
				return snapshot, func(t *testing.T, snapshot *domain.Snapshot) error {
					got, err := codec.DecodeSBTI(snapshot)
					if err != nil {
						return err
					}
					if got.Code != "SBTI_FUN" {
						t.Fatalf("sbti decode = %#v", got)
					}
					return nil
				}
			},
		},
	}

	for _, tc := range legacyDecodeCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			snapshot, decode := tc.build(t)
			if snapshot.PayloadFormat != tc.wantFormat {
				t.Fatalf("format = %s, want %s", snapshot.PayloadFormat, tc.wantFormat)
			}
			if err := decode(t, snapshot); err != nil {
				t.Fatalf("decode: %v", err)
			}
		})
	}

	v2EncodeCases := []struct {
		name       string
		wantFormat string
		encode     func(t *testing.T) (string, error)
	}{
		{
			name:       "scale encode",
			wantFormat: domain.PayloadFormatAssessmentScaleV1,
			encode: func(t *testing.T) (string, error) {
				_, format, err := codec.EncodeScale(&scalesnapshot.ScaleSnapshot{Code: "PHQ9"})
				return format, err
			},
		},
		{
			name:       "mbti encode",
			wantFormat: domain.PayloadFormatPersonalityTypologyV1,
			encode: func(t *testing.T) (string, error) {
				_, format, err := codec.EncodeMBTI(&modeltypology.MBTILegacyModel{Code: "MBTI_OEJTS"})
				return format, err
			},
		},
		{
			name:       "sbti encode",
			wantFormat: domain.PayloadFormatPersonalityTypologyV1,
			encode: func(t *testing.T) (string, error) {
				_, format, err := codec.EncodeSBTI(&modeltypology.SBTILegacyModel{Code: "SBTI_FUN"})
				return format, err
			},
		},
	}
	for _, tc := range v2EncodeCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			format, err := tc.encode(t)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if format != tc.wantFormat {
				t.Fatalf("format = %s, want %s", format, tc.wantFormat)
			}
		})
	}
}
