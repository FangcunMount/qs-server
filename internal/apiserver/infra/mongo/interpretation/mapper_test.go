package interpretation

import (
	"testing"
	"time"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestReportMapperPersistsIndependentFailureState(t *testing.T) {
	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	rpt, err := domainReport.NewPendingInterpretReport(meta.FromUint64(42), meta.FromUint64(99), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := rpt.BeginGenerating(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := rpt.Fail("template unavailable", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	got := NewReportMapper().ToDomain(NewReportMapper().ToPO(rpt, 7))
	if got.Status() != domainReport.ReportStatusFailed || got.Attempt() != 1 || got.OutcomeID().Uint64() != 99 || got.FailureReason() != "template unavailable" || got.FailedAt() == nil {
		t.Fatalf("lifecycle round trip = status:%s attempt:%d outcome:%s reason:%q failed:%v", got.Status(), got.Attempt(), got.OutcomeID(), got.FailureReason(), got.FailedAt())
	}
}

func TestReportMapperRoundTripPreservesInterpretReportFields(t *testing.T) {
	mapper := NewReportMapper()
	createdAt := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	maxScore := 100.0
	original := domainReport.ReconstructInterpretReport(
		domainReport.ID(42),
		"SCL-90",
		"SCL-90",
		72.5,
		domainReport.RiskLevelMedium,
		"medium risk",
		[]domainReport.DimensionInterpret{
			domainReport.NewDimensionInterpret(
				domainReport.NewFactorCode("total"),
				"总分",
				72.5,
				&maxScore,
				domainReport.RiskLevelMedium,
				"dim",
				"watch",
			),
		},
		[]domainReport.Suggestion{
			{Category: domainReport.SuggestionCategoryGeneral, Content: "follow-up"},
		},
		&domainReport.ModelExtra{
			Kind:     "mbti",
			TypeCode: "MBTI_OEJTS",
			TypeName: "MBTI",
		},
		createdAt,
		&updatedAt,
	)

	po := mapper.ToPO(original, 9)
	if po == nil {
		t.Fatal("ToPO returned nil")
	}
	got := mapper.ToDomain(po)
	if got == nil {
		t.Fatal("ToDomain returned nil")
	}
	if got.ID() != original.ID() ||
		got.ModelName() != original.ModelName() ||
		got.ModelCode() != original.ModelCode() ||
		got.TotalScore() != original.TotalScore() ||
		got.RiskLevel() != original.RiskLevel() ||
		got.Conclusion() != original.Conclusion() {
		t.Fatalf("round trip summary mismatch: got=%#v want=%#v", got, original)
	}
	if len(got.Dimensions()) != 1 || got.Dimensions()[0].Code().String() != "total" {
		t.Fatalf("dimensions = %#v", got.Dimensions())
	}
	if len(got.Suggestions()) != 1 || got.Suggestions()[0].Content != "follow-up" {
		t.Fatalf("suggestions = %#v", got.Suggestions())
	}
	if got.ModelExtra() == nil || got.ModelExtra().TypeCode != "MBTI_OEJTS" {
		t.Fatalf("model extra = %#v", got.ModelExtra())
	}
	if po.Model == nil || po.PrimaryScore == nil || po.Level == nil {
		t.Fatalf("v2 fields missing: model=%#v primary=%#v level=%#v", po.Model, po.PrimaryScore, po.Level)
	}
	if po.Model.Kind == "" && po.Model.Code == "" {
		t.Fatalf("model identity = %#v", po.Model)
	}
}

func TestReportMapperRoundTripPreservesDimensionHierarchy(t *testing.T) {
	mapper := NewReportMapper()
	original := domainReport.ReconstructInterpretReport(
		domainReport.ID(44),
		"BRIEF-2",
		"BRIEF2",
		10,
		domainReport.RiskLevelNone,
		"ok",
		[]domainReport.DimensionInterpret{
			domainReport.NewDimensionInterpret(
				domainReport.NewFactorCode("bri"),
				"BRI",
				10,
				nil,
				domainReport.RiskLevelNone,
				"index",
				"",
			).WithHierarchy("index", "gec", 2, 1),
		},
		nil,
		nil,
		time.Now(),
		nil,
	)

	got := mapper.ToDomain(mapper.ToPO(original, 1))
	if len(got.Dimensions()) != 1 {
		t.Fatalf("dimensions = %#v", got.Dimensions())
	}
	dim := got.Dimensions()[0]
	if dim.ParentCode() != "gec" || dim.HierarchyLevel() != 2 || dim.Role() != "index" {
		t.Fatalf("hierarchy = role=%q parent=%q level=%d", dim.Role(), dim.ParentCode(), dim.HierarchyLevel())
	}
}

func TestReportMapperRoundTripPreservesTraitDimensionKind(t *testing.T) {
	mapper := NewReportMapper()
	original := domainReport.ReconstructInterpretReport(
		domainReport.ID(43),
		"Big Five",
		"BIGFIVE_V1",
		0,
		domainReport.RiskLevelNone,
		"trait profile",
		[]domainReport.DimensionInterpret{
			domainReport.NewNeutralDimensionInterpret(
				domainReport.NewDimensionCode("O"),
				domainReport.DimensionKindTrait,
				"Openness",
				6,
				nil,
				nil,
				"Openness：原始分 6",
				"",
			),
		},
		nil,
		nil,
		time.Now(),
		nil,
	)

	got := mapper.ToDomain(mapper.ToPO(original, 10))
	if got.Dimensions()[0].Kind() != domainReport.DimensionKindTrait {
		t.Fatalf("dimension kind = %s, want trait", got.Dimensions()[0].Kind())
	}
}
