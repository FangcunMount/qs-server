package main

import (
	"bytes"
	"reflect"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestParseConfigDoesNotRequireMySQL(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseConfig(nil, &stderr, func(key string) string {
		if key == "MONGO_URI" {
			return "mongodb://mongo"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.apply || cfg.mongoDB != "qs_server" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestPreparePlanRepairsOnlyExactEnneagramTemplateMismatch(t *testing.T) {
	model := exactEnneagramModel()
	beforeDefinition := model.DefinitionV2

	plan, desired, err := preparePlan(model)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != "update" || plan.BeforeTemplate != oldTemplate || plan.AfterTemplate != newTemplate || plan.AfterHash == "" {
		t.Fatalf("plan = %#v", plan)
	}
	if model.DefinitionV2 != beforeDefinition || model.DefinitionV2.ReportMap.Sections[0].TemplateID != oldTemplate {
		t.Fatal("source model was mutated")
	}
	if desired.DefinitionV2.ReportMap.Sections[0].TemplateID != newTemplate ||
		desired.DefinitionV2.DecisionSpec.TypeDecision == nil ||
		desired.DefinitionV2.DecisionSpec.TypeDecision.Kind != binding.DecisionKindTraitProfile ||
		desired.DefinitionV2.InterpretationAssets.ReportSpec.Sections[0].TemplateID != newTemplate {
		t.Fatalf("desired definition layers = %#v", desired.DefinitionV2)
	}

	model = desired
	plan, _, err = preparePlan(model)
	if err != nil || plan.Action != "noop" || plan.BeforeHash != plan.AfterHash {
		t.Fatalf("idempotent plan = %#v err=%v", plan, err)
	}
}

func TestPreparePlanRejectsIdentityOrAuthoredPresentationDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*modelcatalogport.PublishedModel)
	}{
		{name: "version", mutate: func(model *modelcatalogport.PublishedModel) { model.Version = "v17" }},
		{name: "decision", mutate: func(model *modelcatalogport.PublishedModel) { model.DecisionKind = domain.DecisionKindDominantFactor }},
		{name: "template", mutate: func(model *modelcatalogport.PublishedModel) {
			model.DefinitionV2.ReportMap.Sections[0].TemplateID = "bigfive"
		}},
		{name: "outcome", mutate: func(model *modelcatalogport.PublishedModel) {
			model.DefinitionV2.Outcomes = []conclusion.Outcome{{Code: "type_1", Title: "完美型"}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := exactEnneagramModel()
			tc.mutate(model)
			if _, _, err := preparePlan(model); err == nil {
				t.Fatal("drifted model was accepted")
			}
		})
	}
}

func exactEnneagramModel() *modelcatalogport.PublishedModel {
	definition := &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "type_1", Title: "完美型", Role: factor.FactorRoleDimension}}},
		Conclusions: []conclusion.Conclusion{conclusion.TypeConclusion{
			FactorCodes: []string{"type_1"},
			Decision:    conclusion.TypeDecision{Kind: binding.DecisionKindTraitProfile},
			OutcomeMapping: conclusion.TypeOutcomeMapping{
				DetailKind: "trait_profile", DetailAdapterKey: "trait_profile", Algorithm: binding.AlgorithmPersonalityTypology,
			},
		}},
		ReportMap: modeldefinition.ReportMap{Sections: []modeldefinition.ReportSection{{
			Code: "trait_profile", Title: "九型人格", Kind: "trait_profile", AdapterKey: "trait_profile", TemplateID: oldTemplate, CategoryLabel: "九型人格",
		}}},
		InterpretationAssets: interpretationassets.Assets{ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{
			Code: "trait_profile", Title: "九型人格", Kind: "trait_profile", AdapterKey: "trait_profile", TemplateID: oldTemplate, CategoryLabel: "九型人格",
		}}}},
	}
	model := &modelcatalogport.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2, Kind: domain.KindTypology, SubKind: domain.SubKindTypology,
		Algorithm: domain.AlgorithmPersonalityTypology, AlgorithmFamily: domain.AlgorithmFamilyFactorClassification,
		DecisionKind: domain.DecisionKindTraitProfile, Code: targetCode, Version: targetVersion,
		ReleaseStatus: domain.ReleaseStatusActive, Status: "published", DefinitionV2: definition,
		Source: map[string]any{},
	}
	hash, _ := modeldefinition.CanonicalContentHash(definition)
	modelcatalogport.AttachDefinitionHash(model, hash)
	return model
}

func TestExactFixtureHasStableSourceClone(t *testing.T) {
	model := exactEnneagramModel()
	clone, err := clonePublishedModel(model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(model.Source, clone.Source) {
		t.Fatalf("source = %#v clone = %#v", model.Source, clone.Source)
	}
}
