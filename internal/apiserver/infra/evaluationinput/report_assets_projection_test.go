package evaluationinput

import (
	"bytes"
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modelport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestProjectionShadowPreservesStoredReadIdentityAndFrozenReportBytes(t *testing.T) {
	var frozenBaseline []byte
	var identityBaseline port.InputSnapshotIdentity
	for _, version := range []interpretationassets.ProjectionVersion{0, 1, 2, 99} {
		def := &domain.Definition{
			Measure:              modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Title: "Total", Role: factor.FactorRoleTotal}}},
			Outcomes:             []conclusion.Outcome{{Code: "low", Summary: "canonical"}},
			InterpretationAssets: interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "low", Summary: "retained"}}},
		}
		published := &modelport.PublishedModel{Code: "M", Version: "v1", DefinitionV2: def, ReportAssetsProjectionVersion: version}
		input := &port.InputSnapshot{
			Model:         &port.ModelSnapshot{Kind: port.EvaluationModelKindScale, Code: "M", Version: "v1", Algorithm: "scale_default", DecisionKind: "score_range"},
			Questionnaire: &port.QuestionnaireSnapshot{Code: "Q", Version: "v1"},
			AnswerSheet:   &port.AnswerSheetSnapshot{ID: 1, QuestionnaireCode: "Q", QuestionnaireVersion: "v1"},
		}
		if err := attachScaleCanonical(context.Background(), canonicalReaderStub{model: published}, port.InputRef{ModelRef: port.ModelRef{Code: "M", Version: "v1"}}, input); err != nil {
			t.Fatal(err)
		}
		assets, ok := port.InterpretationAssetsFromSnapshot(input)
		if !ok || assets.Outcomes[0].Summary != "retained" {
			t.Fatal("shadow switched stored-first asset read")
		}
		identity, ok := port.NewInputSnapshotIdentity(input)
		if !ok {
			t.Fatal("identity unavailable")
		}
		modelRef := port.ModelRef{Kind: port.EvaluationModelKindScale, Code: "M", Version: "v1"}
		frozen, err := port.MarshalReportInput(port.BuildFreezeOptionsFromSnapshot(input, modelRef, domain.DecisionKindScoreRange))
		if err != nil {
			t.Fatal(err)
		}
		if version == 0 {
			frozenBaseline, identityBaseline = frozen, identity
		}
		if !bytes.Equal(frozenBaseline, frozen) || identity != identityBaseline {
			t.Fatal("projection marker changed ReportInput bytes or input identity")
		}
		def.Outcomes[0].Summary = "new revision copy"
		replayed, err := port.SnapshotFromReportInput(frozen, modelRef)
		if err != nil {
			t.Fatal(err)
		}
		port.AttachCanonicalDefinition(replayed, def)
		assets, _ = port.InterpretationAssetsFromSnapshot(replayed)
		if assets.Outcomes[0].Summary != "retained" {
			t.Fatal("historical frozen input replaced by canonical assets")
		}
	}
}
