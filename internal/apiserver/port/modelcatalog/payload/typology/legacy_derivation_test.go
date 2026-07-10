package typology_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestLegacyOutcomeMappingFromAlgorithm(t *testing.T) {
	mbti := typology.LegacyOutcomeMappingFromAlgorithm(modelcatalog.AlgorithmMBTI)
	if mbti.DetailAdapterKey != typology.DetailAdapterPersonalityType {
		t.Fatalf("mbti adapter = %s, want personality_type", mbti.DetailAdapterKey)
	}
	sbti := typology.LegacyOutcomeMappingFromAlgorithm(modelcatalog.AlgorithmSBTI)
	if sbti.DetailAdapterKey != typology.DetailAdapterPersonalityType {
		t.Fatalf("sbti adapter = %s, want personality_type", sbti.DetailAdapterKey)
	}
	bigfive := typology.LegacyOutcomeMappingFromAlgorithm(modelcatalog.AlgorithmBigFive)
	if bigfive.DetailAdapterKey != typology.DetailAdapterTraitProfile {
		t.Fatalf("bigfive adapter = %s, want trait_profile", bigfive.DetailAdapterKey)
	}
}

func TestLegacyReportSpecFromAlgorithm(t *testing.T) {
	spec := typology.LegacyReportSpecFromAlgorithm(modelcatalog.AlgorithmSBTI)
	if spec.AdapterKey != typology.ReportAdapterPersonalityType || spec.CategoryLabel != "SBTI" {
		t.Fatalf("report spec = %#v", spec)
	}
}
