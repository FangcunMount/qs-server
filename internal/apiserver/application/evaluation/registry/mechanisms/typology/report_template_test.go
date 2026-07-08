package typology

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
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
			spec: ReportSpecForAlgorithm(modelcatalog.AlgorithmMBTI),
			want: "mbti",
		},
		{
			name: "sbti via template_id",
			spec: ReportSpecForAlgorithm(modelcatalog.AlgorithmSBTI),
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
	derivedMBTI := personalityTypeTemplateForSpec(ReportSpecForAlgorithm(modelcatalog.AlgorithmMBTI))
	if derivedMBTI.Kind != legacyMBTI.Kind || derivedMBTI.DefaultModelCode != legacyMBTI.DefaultModelCode {
		t.Fatalf("derived MBTI template drifted from legacy fixture")
	}
}

func TestTraitProfileTemplateForSpecCharacterization(t *testing.T) {
	t.Parallel()

	spec := ReportSpecForAlgorithm(modelcatalog.AlgorithmBigFive)
	tmpl := traitProfileTemplateForSpec(spec)
	if tmpl.Kind != "bigfive" {
		t.Fatalf("template kind = %q, want bigfive", tmpl.Kind)
	}

	legacy := reporttypology.BigFiveTraitProfileTemplate()
	if tmpl.DefaultModelCode != legacy.DefaultModelCode || tmpl.TypeName != legacy.TypeName {
		t.Fatalf("derived BigFive template drifted from legacy fixture")
	}
}
