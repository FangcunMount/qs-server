package evaluationinput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modelport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestStrictProjectionPreservesStoredReadIdentityAndFrozenReportBytes(t *testing.T) {
	var frozenBaseline []byte
	var identityBaseline port.InputSnapshotIdentity
	for _, version := range []interpretationassets.ProjectionVersion{1, 2} {
		def := &domain.Definition{
			Measure:  modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Title: "Total", Role: factor.FactorRoleTotal}}},
			Outcomes: []conclusion.Outcome{{Code: "low", Summary: "retained"}},
		}
		published := &modelport.PublishedModel{Code: "M", Version: "v1", DefinitionV2: def, ReportAssetsProjectionVersion: version}
		input := &port.InputSnapshot{
			Model:         &port.ModelSnapshot{Kind: port.EvaluationModelKindScale, Code: "M", Version: "v1", Algorithm: "scale_default", DecisionKind: "score_range"},
			Questionnaire: &port.QuestionnaireSnapshot{Code: "Q", Version: "v1"},
			AnswerSheet:   &port.AnswerSheetSnapshot{ID: 1, QuestionnaireCode: "Q", QuestionnaireVersion: "v1"},
		}
		legacy := *input
		legacy.DefinitionV2 = def
		legacyAssets, _ := modeldefinition.ResolveReportAssetsProjection(def, version)
		legacy.InterpretationAssets = &legacyAssets
		legacyIdentity, _ := port.NewInputSnapshotIdentity(&legacy)
		legacyRef := port.ModelRef{Kind: port.EvaluationModelKindScale, Code: "M", Version: "v1"}
		legacyFrozen, legacyErr := port.MarshalReportInput(port.BuildFreezeOptionsFromSnapshot(&legacy, legacyRef, domain.DecisionKindScoreRange))
		if legacyErr != nil {
			t.Fatal(legacyErr)
		}
		if err := attachScaleCanonical(context.Background(), canonicalReaderStub{model: published}, port.InputRef{ModelRef: port.ModelRef{Code: "M", Version: "v1"}}, input); err != nil {
			t.Fatal(err)
		}
		assets, ok := port.InterpretationAssetsFromSnapshot(input)
		if !ok || assets.Outcomes[0].Summary != "retained" {
			t.Fatal("strict projection changed retained asset content")
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
		if identity != legacyIdentity || !bytes.Equal(frozen, legacyFrozen) {
			t.Fatal("strict read changed legacy identity or frozen bytes")
		}
		hash, hashErr := modeldefinition.CanonicalContentHash(def)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		published.Source = map[string]any{modelport.SourceDefinitionContentHash: hash, modelport.SourceDefinitionHashSchema: "definition-v2/v1"}
		afterMetadata := *input
		afterMetadata.DefinitionV2 = nil
		afterMetadata.InterpretationAssets = nil
		if err := attachScaleCanonical(context.Background(), canonicalReaderStub{model: published}, port.InputRef{ModelRef: modelRef}, &afterMetadata); err != nil {
			t.Fatal(err)
		}
		afterIdentity, _ := port.NewInputSnapshotIdentity(&afterMetadata)
		afterFrozen, err := port.MarshalReportInput(port.BuildFreezeOptionsFromSnapshot(&afterMetadata, modelRef, domain.DecisionKindScoreRange))
		if err != nil || afterIdentity != identity || !bytes.Equal(afterFrozen, frozen) {
			t.Fatal("missing hash metadata enrichment changed input identity or report bytes", err)
		}
		if version == 1 {
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
		if err := attachCanonicalWithProjection(replayed, published); err != nil {
			t.Fatal(err)
		}
		assets, _ = port.InterpretationAssetsFromSnapshot(replayed)
		if assets.Outcomes[0].Summary != "retained" {
			t.Fatal("historical frozen input replaced by canonical assets")
		}
	}
}

func TestStrictProjectionRejectsUnassignedAndUnknownVersions(t *testing.T) {
	for _, version := range []interpretationassets.ProjectionVersion{0, 99} {
		for _, family := range []string{"scale", "typology", "behavioral", "cognitive"} {
			t.Run(fmt.Sprint(version)+"/"+family, func(t *testing.T) {
				def := &domain.Definition{Outcomes: []conclusion.Outcome{{Code: "low", Summary: "canonical"}}}
				input := &port.InputSnapshot{}
				published := &modelport.PublishedModel{Code: "M", Version: "v1", DefinitionV2: def, ReportAssetsProjectionVersion: version}
				ref := port.InputRef{ModelRef: port.ModelRef{Code: "M", Version: "v1"}}
				reader := canonicalReaderStub{model: published}
				var err error
				switch family {
				case "scale":
					err = attachScaleCanonical(context.Background(), reader, ref, input)
				case "typology":
					err = attachTypologyCanonical(context.Background(), reader, ref, domain.AlgorithmPersonalityTypology, input)
				case "behavioral":
					published.Algorithm = domain.AlgorithmBrief2
					ref.ModelRef.Algorithm = string(domain.AlgorithmBrief2)
					err = attachBehavioralCanonical(context.Background(), reader, ref, input)
				case "cognitive":
					published.Algorithm = domain.AlgorithmSPM
					ref.ModelRef.Algorithm = string(domain.AlgorithmSPM)
					err = attachCognitiveCanonical(context.Background(), reader, ref, input)
				}
				var resolve *port.ResolveError
				if !errors.As(err, &resolve) || resolve.FailureKind() != port.FailureKindUnsupportedModel || resolve.Retryable() {
					t.Fatalf("invalid projection was executable or retryable: %v", err)
				}
				if input.DefinitionV2 != nil || input.InterpretationAssets != nil {
					t.Fatal("failed projection partially changed input")
				}
			})
		}
	}
}

func TestStrictProjectionKeepsPreviouslyFrozenReportAssets(t *testing.T) {
	def := &domain.Definition{Outcomes: []conclusion.Outcome{{Code: "low", Summary: "canonical"}}}
	frozen := &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "low", Summary: "historical"}}}
	input := &port.InputSnapshot{InterpretationAssets: frozen}
	published := &modelport.PublishedModel{Code: "M", Version: "v1", DefinitionV2: def, ReportAssetsProjectionVersion: 1}
	if err := attachScaleCanonical(context.Background(), canonicalReaderStub{model: published}, port.InputRef{ModelRef: port.ModelRef{Code: "M", Version: "v1"}}, input); err != nil {
		t.Fatal(err)
	}
	if input.InterpretationAssets != frozen || frozen.Outcomes[0].Summary != "historical" {
		t.Fatal("canonical projection replaced existing frozen report assets")
	}
}
