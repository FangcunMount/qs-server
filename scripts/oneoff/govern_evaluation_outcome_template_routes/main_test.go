package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestPlanReportInputMaterializesLegacyStandardRoute(t *testing.T) {
	raw := factorReportInput(t, "", "")
	plan, err := planReportInput(raw, "scale")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changed || plan.TemplateID != "standard" || plan.TemplateVersion != targetLegacyVersion || plan.SourceSemanticHash == plan.TargetSemanticHash {
		t.Fatalf("plan = %#v", plan)
	}
	section := firstSection(t, plan.TargetJSON)
	if section["TemplateID"] != "standard" || section["TemplateVersion"] != targetLegacyVersion {
		t.Fatalf("section = %#v", section)
	}

	restored, err := restoreReportInput(plan.TargetJSON, record{
		SourceSemanticHash: plan.SourceSemanticHash, Sections: plan.Sections,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalJSON(restored)
	if err != nil {
		t.Fatal(err)
	}
	if hashBytes(canonical) != plan.SourceSemanticHash {
		t.Fatalf("rollback semantic hash = %s, want %s", hashBytes(canonical), plan.SourceSemanticHash)
	}
}

func TestPlanReportInputMaterializesTypologyRouteInBothFrozenLayers(t *testing.T) {
	raw := typologyReportInput(t, "mbti", "", "mbti", "")
	plan, err := planReportInput(raw, "typology")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changed || plan.TemplateID != "mbti" || plan.TypologyRouting == nil {
		t.Fatalf("plan = %#v", plan)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(plan.TargetJSON), &root); err != nil {
		t.Fatal(err)
	}
	route := root["typology_routing"].(map[string]any)
	if route["template_id"] != "mbti" || route["template_version"] != targetLegacyVersion {
		t.Fatalf("typology route = %#v", route)
	}

	restored, err := restoreReportInput(plan.TargetJSON, record{
		SourceSemanticHash: plan.SourceSemanticHash, Sections: plan.Sections, TypologyRouting: plan.TypologyRouting,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := canonicalJSON(restored)
	if hashBytes(canonical) != plan.SourceSemanticHash {
		t.Fatal("typology rollback did not restore source semantics")
	}
}

func TestPlanReportInputLeavesExplicitCurrentRouteUnchanged(t *testing.T) {
	plan, err := planReportInput(factorReportInput(t, "standard", targetCurrentVersion), "scale")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changed || plan.TemplateVersion != targetCurrentVersion || plan.SourceSemanticHash != plan.TargetSemanticHash {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanReportInputMaterializesFrozenSectionFromMatchingArtifact(t *testing.T) {
	raw := reportInputWithoutSections(t, factorReportInput(t, "standard", targetLegacyVersion), "null")
	row := outcomeRow{ID: 99, ModelKind: "scale", ModelCode: "SCALE", ModelVersion: "v1", ReportInput: raw}
	candidate := artifactCandidate{Count: 1, Document: artifactDocument{
		DomainID: 501, OutcomeID: 99, TemplateVersion: targetLegacyVersion,
		BuilderIdentity: "factor-scoring", ContentSchemaVersion: "report-content/v1",
		Model:               &artifactModel{Kind: "scale", Code: "SCALE", Version: "v1"},
		PresentationProfile: &artifactPresentation{VisibleFactorCodes: []string{"total"}, Source: "legacy_artifact_dimensions/v1"},
	}}
	materialization, err := materializationFromArtifact(row, candidate)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planReportInputWithMaterialization(raw, row.ModelKind, materialization)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changed || plan.Materialization == nil || plan.Materialization.OriginalSectionsState != "null" {
		t.Fatalf("plan = %#v", plan)
	}
	section := firstSection(t, plan.TargetJSON)
	if section["TemplateID"] != "standard" || section["TemplateVersion"] != targetLegacyVersion || section["Kind"] != "factor_scores" {
		t.Fatalf("section = %#v", section)
	}
	restored, err := restoreReportInput(plan.TargetJSON, record{
		SourceSemanticHash: plan.SourceSemanticHash, Sections: plan.Sections, Materialization: plan.Materialization,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := canonicalJSON(restored)
	if hashBytes(canonical) != plan.SourceSemanticHash {
		t.Fatal("materialized report section rollback did not restore source semantics")
	}
}

func TestPlanReportInputRejectsConflictingTypologyTemplate(t *testing.T) {
	_, err := planReportInput(typologyReportInput(t, "mbti", "", "sbti", ""), "typology")
	if err == nil || !strings.Contains(err.Error(), "resolve exactly once") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadConfigRequiresExactWriteConfirmation(t *testing.T) {
	env := map[string]string{
		"OUTCOME_TEMPLATE_ROUTE_OPERATION": "apply", "OUTCOME_TEMPLATE_ROUTE_MANIFEST_PATH": "/work/manifest.json",
		"MYSQL_HOST": "mysql", "MYSQL_USERNAME": "user", "MYSQL_PASSWORD": "secret", "MYSQL_DATABASE": "qs",
		"MONGODB_HOST": "mongo", "MONGODB_USERNAME": "user", "MONGODB_PASSWORD": "secret", "MONGODB_DBNAME": "qs",
	}
	_, err := readConfig(func(key string) string { return env[key] })
	if err == nil || !strings.Contains(err.Error(), applyConfirmation) {
		t.Fatalf("error = %v", err)
	}
	env["OUTCOME_TEMPLATE_ROUTE_CONFIRM"] = applyConfirmation
	cfg, err := readConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Operation != "apply" || cfg.MySQL.DBName != "qs" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestManifestFingerprintRejectsTampering(t *testing.T) {
	records := []record{{
		OutcomeID: 1, ModelKind: "scale", TemplateID: "standard", TemplateVersion: targetLegacyVersion,
		SourceRawHash: strings.Repeat("a", 64), SourceSemanticHash: strings.Repeat("b", 64), TargetSemanticHash: strings.Repeat("c", 64),
		Sections: []sectionDelta{{Index: 0}},
	}}
	fingerprint, err := recordsFingerprint(records)
	if err != nil {
		t.Fatal(err)
	}
	value := manifest{
		SchemaVersion: governanceSchemaVersion, Database: "qs", Table: "evaluation_outcome", TargetLegacy: targetLegacyVersion,
		MongoDatabase: "qs", ArtifactCollection: artifactCollection,
		Records: records, RecordsFingerprint: fingerprint,
	}
	if err := validateManifest(value, "qs", "qs"); err != nil {
		t.Fatal(err)
	}
	value.Records[0].TemplateID = "mbti"
	if err := validateManifest(value, "qs", "qs"); err == nil {
		t.Fatal("tampered manifest was accepted")
	}
}

func TestSelectedRecordsUsesStableExclusiveCursor(t *testing.T) {
	got := selectedRecords([]record{{OutcomeID: 1}, {OutcomeID: 2}, {OutcomeID: 3}}, config{AfterID: 1, MaxRecords: 1})
	if len(got) != 1 || got[0].OutcomeID != 2 {
		t.Fatalf("selected = %#v", got)
	}
}

func factorReportInput(t *testing.T, templateID, templateVersion string) string {
	t.Helper()
	assets := &interpretationassets.Assets{
		Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "normal", Title: "正常"}},
		ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{
			Code: "result", Kind: "factor_scores", SourceRefs: []string{"total"}, TemplateID: templateID, TemplateVersion: templateVersion,
		}}},
	}
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets:        assets,
		ModelRef:      evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault), Code: "SCALE", Version: "v1"},
		DecisionKind:  modelcatalog.DecisionKindScoreRange,
		FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "total", Title: "总分", IsTotalScore: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func typologyReportInput(t *testing.T, sectionID, sectionVersion, routeID, routeVersion string) string {
	t.Helper()
	assets := &interpretationassets.Assets{
		Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "INTJ", Title: "建筑师"}},
		Profiles: []interpretationassets.TypeProfilePresentation{{OutcomeCode: "INTJ", Commentary: "冻结摘要"}},
		ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{
			Code: "personality", Kind: "personality_type", AdapterKey: "personality_type", TemplateID: sectionID, TemplateVersion: sectionVersion,
		}}},
	}
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets:       assets,
		ModelRef:     evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "MBTI", Version: "v1"},
		DecisionKind: modelcatalog.DecisionKindPoleComposition,
		TypologyRouting: &evaluationinput.TypologyRoutingFreeze{
			DecisionKind: string(modelcatalog.DecisionKindPoleComposition), ReportKind: string(modeltypology.ReportKindPersonalityType),
			AdapterKey: string(modeltypology.ReportAdapterPersonalityType), TemplateID: routeID, TemplateVersion: routeVersion,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func firstSection(t *testing.T, raw string) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatal(err)
	}
	assets := root["InterpretationAssets"].(map[string]any)
	report := assets["ReportSpec"].(map[string]any)
	return report["Sections"].([]any)[0].(map[string]any)
}

func reportInputWithoutSections(t *testing.T, raw, state string) string {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatal(err)
	}
	report := root["InterpretationAssets"].(map[string]any)["ReportSpec"].(map[string]any)
	switch state {
	case "missing":
		delete(report, "Sections")
	case "null":
		report["Sections"] = nil
	case "empty_array":
		report["Sections"] = []any{}
	default:
		t.Fatalf("unsupported state %q", state)
	}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
