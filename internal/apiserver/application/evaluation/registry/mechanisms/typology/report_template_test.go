package typology

import (
	"testing"

	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func TestPersonalityTypeTemplateForSpecCharacterization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		spec modeltypology.ReportSpec
		want string
	}{
		{
			name: "mbti via template_id",
			spec: func() modeltypology.ReportSpec {
				s, _, _ := typologylegacy.ReportBuildContextFromAlgorithm(modelcatalog.AlgorithmMBTI)
				return s
			}(),
			want: "mbti",
		},
		{
			name: "sbti via template_id",
			spec: func() modeltypology.ReportSpec {
				s, _, _ := typologylegacy.ReportBuildContextFromAlgorithm(modelcatalog.AlgorithmSBTI)
				return s
			}(),
			want: "sbti",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpl := personalityTypeTemplateForSpec(tc.spec)
			if tmpl.Kind != tc.want {
				t.Fatalf("template kind = %q, want %q", tmpl.Kind, tc.want)
			}
		})
	}

	legacyMBTI := reporttypology.MBTIPersonalityTypeTemplate()
	mbtiSpec, _, _ := typologylegacy.ReportBuildContextFromAlgorithm(modelcatalog.AlgorithmMBTI)
	derivedMBTI := personalityTypeTemplateForSpec(mbtiSpec)
	if derivedMBTI.Kind != legacyMBTI.Kind || derivedMBTI.DefaultModelCode != legacyMBTI.DefaultModelCode {
		t.Fatalf("derived MBTI template drifted from legacy fixture")
	}
}

func TestTraitProfileTemplateForSpecCharacterization(t *testing.T) {
	t.Parallel()

	bigFiveSpec, _, _ := typologylegacy.ReportBuildContextFromAlgorithm(modelcatalog.AlgorithmBigFive)
	tmpl := traitProfileTemplateForSpec(bigFiveSpec)
	if tmpl.Kind != "bigfive" {
		t.Fatalf("template kind = %q, want bigfive", tmpl.Kind)
	}

	legacy := reporttypology.BigFiveTraitProfileTemplate()
	if tmpl.DefaultModelCode != legacy.DefaultModelCode || tmpl.TypeName != legacy.TypeName {
		t.Fatalf("derived BigFive template drifted from legacy fixture")
	}
}
