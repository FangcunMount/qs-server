package typology_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func TestLegacyOutcomeMappingFromAlgorithm(t *testing.T) {
	mbti := typology.LegacyOutcomeMappingFromAlgorithm(assessmentmodel.AlgorithmMBTI)
	if mbti.DetailAdapterKey != typology.DetailAdapterMBTI {
		t.Fatalf("mbti adapter = %s", mbti.DetailAdapterKey)
	}
	sbti := typology.LegacyOutcomeMappingFromAlgorithm(assessmentmodel.AlgorithmSBTI)
	if sbti.DetailAdapterKey != typology.DetailAdapterSBTI {
		t.Fatalf("sbti adapter = %s", sbti.DetailAdapterKey)
	}
	bigfive := typology.LegacyOutcomeMappingFromAlgorithm(assessmentmodel.AlgorithmBigFive)
	if bigfive.DetailAdapterKey != typology.DetailAdapterBigFive {
		t.Fatalf("bigfive adapter = %s", bigfive.DetailAdapterKey)
	}
}

func TestLegacyReportSpecFromAlgorithm(t *testing.T) {
	spec := typology.LegacyReportSpecFromAlgorithm(assessmentmodel.AlgorithmSBTI)
	if spec.AdapterKey != typology.ReportAdapterSBTI || spec.CategoryLabel != "SBTI" {
		t.Fatalf("report spec = %#v", spec)
	}
}
