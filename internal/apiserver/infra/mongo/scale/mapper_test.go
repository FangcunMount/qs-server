package scale

import (
	"context"
	"reflect"
	"testing"

	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/definition"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestScaleMapperMapScoringParamsToDomainHandlesStoredArrayShapes(t *testing.T) {
	t.Parallel()

	mapper := NewScaleMapper()
	cases := []struct {
		name   string
		params map[string]interface{}
		want   []string
	}{
		{
			name: "mongo primitive array",
			params: map[string]interface{}{
				"cnt_option_contents": primitive.A{"yes", "often"},
			},
			want: []string{"yes", "often"},
		},
		{
			name: "generic interface array skips non strings",
			params: map[string]interface{}{
				"cnt_option_contents": []interface{}{"yes", 42, "often"},
			},
			want: []string{"yes", "often"},
		},
		{
			name: "string array",
			params: map[string]interface{}{
				"cnt_option_contents": []string{"yes", "often"},
			},
			want: []string{"yes", "often"},
		},
		{
			name:   "missing params",
			params: nil,
			want:   []string{},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := mapper.mapScoringParamsToDomain(context.Background(), tc.params, scaledefinition.ScoringStrategyCnt)
			if got == nil {
				t.Fatal("expected scoring params")
			}
			if !reflect.DeepEqual(got.GetCntOptionContents(), tc.want) {
				t.Fatalf("cnt option contents = %#v, want %#v", got.GetCntOptionContents(), tc.want)
			}
		})
	}
}

func TestScaleMapperMapScoringParamsToDomainIgnoresParamsForStrategiesWithoutConfig(t *testing.T) {
	t.Parallel()

	mapper := NewScaleMapper()
	got := mapper.mapScoringParamsToDomain(context.Background(), map[string]interface{}{
		"cnt_option_contents": primitive.A{"yes"},
	}, scaledefinition.ScoringStrategySum)
	if got == nil {
		t.Fatal("expected scoring params")
	}
	if len(got.GetCntOptionContents()) != 0 {
		t.Fatalf("cnt option contents = %#v, want empty", got.GetCntOptionContents())
	}
}

func TestScoringParamsToStoredMapKeepsPersistenceShapeInInfra(t *testing.T) {
	t.Parallel()

	params := scaledefinition.NewScoringParams().WithCntOptionContents([]string{"yes", "often"})
	got := scoringParamsToStoredMap(params, scaledefinition.ScoringStrategyCnt)
	if !reflect.DeepEqual(got["cnt_option_contents"], []string{"yes", "often"}) {
		t.Fatalf("cnt_option_contents = %#v, want string slice", got["cnt_option_contents"])
	}

	if got := scoringParamsToStoredMap(params, scaledefinition.ScoringStrategySum); len(got) != 0 {
		t.Fatalf("sum scoring params = %#v, want empty", got)
	}
	if got := scoringParamsToStoredMap(nil, scaledefinition.ScoringStrategyCnt); len(got) != 0 {
		t.Fatalf("nil scoring params = %#v, want empty", got)
	}
}

func TestScaleMapperNormalizeRiskLevelSupportsLegacyValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want scaledefinition.RiskLevel
	}{
		{name: "normal alias", raw: "normal", want: scaledefinition.RiskLevelNone},
		{name: "low chinese", raw: "低风险", want: scaledefinition.RiskLevelLow},
		{name: "medium legacy", raw: "中度", want: scaledefinition.RiskLevelMedium},
		{name: "high legacy", raw: "重度", want: scaledefinition.RiskLevelHigh},
		{name: "severe legacy", raw: "严重", want: scaledefinition.RiskLevelSevere},
		{name: "unknown stays raw", raw: "custom", want: scaledefinition.RiskLevel("custom")},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeRiskLevel(tc.raw); got != tc.want {
				t.Fatalf("normalizeRiskLevel(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestScaleMapperPersistsAndBackfillsScaleVersion(t *testing.T) {
	t.Parallel()

	mapper := NewScaleMapper()
	domain, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCALE_V"),
		"Scale V",
		scaledefinition.WithScaleVersion("2.3.0"),
		scaledefinition.WithQuestionnaire(meta.NewCode("Q-V"), "9.9.0"),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale returned error: %v", err)
	}

	po := mapper.ToPO(domain)
	if po.ScaleVersion != "2.3.0" {
		t.Fatalf("po scale version = %q, want 2.3.0", po.ScaleVersion)
	}

	backfilled := mapper.ToDomain(context.Background(), &ScalePO{
		Code:                 "SCALE_OLD",
		Title:                "Scale Old",
		QuestionnaireVersion: "1.8.0",
		Status:               scaledefinition.StatusDraft.String(),
	})
	if backfilled.GetScaleVersion() != "1.8.0" {
		t.Fatalf("backfilled scale version = %q, want questionnaire version", backfilled.GetScaleVersion())
	}
}

func TestScaleMapperToDomainBackfillsLegacyFactorDefaults(t *testing.T) {
	t.Parallel()

	mapper := NewScaleMapper()
	got := mapper.ToDomain(context.Background(), &ScalePO{
		Code:   "SCALE_LEGACY",
		Title:  "Scale Legacy",
		Status: scaledefinition.StatusDraft.String(),
		Factors: []FactorPO{
			{
				Code:            "F_LEGACY",
				Title:           "Legacy Factor",
				IsShow:          true,
				QuestionCodes:   []string{"Q1", "Q2"},
				InterpretRules:  nil,
				FactorType:      "first_grade",
				ScoringStrategy: "",
			},
		},
	})
	if got == nil {
		t.Fatal("expected scale domain model")
	}

	snapshots := got.FactorSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("factor count = %d, want 1", len(snapshots))
	}
	if snapshots[0].FactorType != scaledefinition.FactorTypePrimary {
		t.Fatalf("factor type = %q, want %q", snapshots[0].FactorType, scaledefinition.FactorTypePrimary)
	}
	if snapshots[0].ScoringStrategy != scaledefinition.ScoringStrategySum {
		t.Fatalf("scoring strategy = %q, want %q", snapshots[0].ScoringStrategy, scaledefinition.ScoringStrategySum)
	}
}

func TestScaleMapperToDomainSkipsLegacyCntFactorWithoutParams(t *testing.T) {
	t.Parallel()

	mapper := NewScaleMapper()
	got := mapper.ToDomain(context.Background(), &ScalePO{
		Code:   "SCALE_A",
		Title:  "Scale A",
		Status: scaledefinition.StatusDraft.String(),
		Factors: []FactorPO{
			{
				Code:            "F_CNT",
				Title:           "Cnt Factor",
				FactorType:      scaledefinition.FactorTypePrimary.String(),
				IsShow:          true,
				QuestionCodes:   []string{"Q1"},
				ScoringStrategy: scaledefinition.ScoringStrategyCnt.String(),
			},
		},
	})
	if got == nil {
		t.Fatal("expected scale domain model")
	}
	if len(got.FactorSnapshots()) != 0 {
		t.Fatalf("factor count = %d, want 0", len(got.FactorSnapshots()))
	}
}
