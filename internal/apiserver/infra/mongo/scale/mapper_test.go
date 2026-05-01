package scale

import (
	"context"
	"reflect"
	"testing"

	domainscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
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

			got := mapper.mapScoringParamsToDomain(context.Background(), tc.params, domainscale.ScoringStrategyCnt)
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
	}, domainscale.ScoringStrategySum)
	if got == nil {
		t.Fatal("expected scoring params")
	}
	if len(got.GetCntOptionContents()) != 0 {
		t.Fatalf("cnt option contents = %#v, want empty", got.GetCntOptionContents())
	}
}

func TestScaleMapperNormalizeRiskLevelSupportsLegacyValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want domainscale.RiskLevel
	}{
		{name: "normal alias", raw: "normal", want: domainscale.RiskLevelNone},
		{name: "low chinese", raw: "低风险", want: domainscale.RiskLevelLow},
		{name: "medium legacy", raw: "中度", want: domainscale.RiskLevelMedium},
		{name: "high legacy", raw: "重度", want: domainscale.RiskLevelHigh},
		{name: "severe legacy", raw: "严重", want: domainscale.RiskLevelSevere},
		{name: "unknown stays raw", raw: "custom", want: domainscale.RiskLevel("custom")},
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

func TestScaleMapperToDomainSkipsLegacyCntFactorWithoutParams(t *testing.T) {
	t.Parallel()

	mapper := NewScaleMapper()
	got := mapper.ToDomain(context.Background(), &ScalePO{
		Code:   "SCALE_A",
		Title:  "Scale A",
		Status: domainscale.StatusDraft.String(),
		Factors: []FactorPO{
			{
				Code:            "F_CNT",
				Title:           "Cnt Factor",
				FactorType:      domainscale.FactorTypePrimary.String(),
				IsShow:          true,
				QuestionCodes:   []string{"Q1"},
				ScoringStrategy: domainscale.ScoringStrategyCnt.String(),
			},
		},
	})
	if got == nil {
		t.Fatal("expected scale domain model")
	}
	if len(got.GetFactors()) != 0 {
		t.Fatalf("factor count = %d, want 0", len(got.GetFactors()))
	}
}
