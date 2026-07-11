package typology

import (
	"testing"

	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
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
				s, _, _ := reportBuildContextFromAlgorithm(modelcatalog.AlgorithmMBTI)
				return s
			}(),
			want: "mbti",
		},
		{
			name: "sbti via template_id",
			spec: func() modeltypology.ReportSpec {
				s, _, _ := reportBuildContextFromAlgorithm(modelcatalog.AlgorithmSBTI)
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
	mbtiSpec, _, _ := reportBuildContextFromAlgorithm(modelcatalog.AlgorithmMBTI)
	derivedMBTI := personalityTypeTemplateForSpec(mbtiSpec)
	if derivedMBTI.Kind != legacyMBTI.Kind || derivedMBTI.DefaultModelCode != legacyMBTI.DefaultModelCode {
		t.Fatalf("derived MBTI template drifted from legacy fixture")
	}
}

func TestTraitProfileTemplateForSpecCharacterization(t *testing.T) {
	t.Parallel()

	bigFiveSpec, _, _ := reportBuildContextFromAlgorithm(modelcatalog.AlgorithmBigFive)
	tmpl := traitProfileTemplateForSpec(bigFiveSpec)
	if tmpl.Kind != "bigfive" {
		t.Fatalf("template kind = %q, want bigfive", tmpl.Kind)
	}

	legacy := reporttypology.BigFiveTraitProfileTemplate()
	if tmpl.DefaultModelCode != legacy.DefaultModelCode || tmpl.TypeName != legacy.TypeName {
		t.Fatalf("derived BigFive template drifted from legacy fixture")
	}
}
