package main

import (
	"encoding/json"
	"testing"

	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestSplitCodesDeduplicatesAndSorts(t *testing.T) {
	got := splitCodes(" B, A, B ,, C ")
	want := []string{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("splitCodes() = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("splitCodes() = %#v, want %#v", got, want)
		}
	}
}

func TestValidateScalePayloadAcceptsDefinitionProjection(t *testing.T) {
	definition := &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{
		Factors: []factor.Factor{{Code: "total", Title: "总分", Role: factor.FactorRoleTotal}},
		Scoring: []factor.Scoring{{FactorCode: "total", Strategy: factor.ScoringStrategySum}},
	}}
	snapshot := &modelcatalogport.AssessmentSnapshot{
		Code:                 "SCALE-1",
		Version:              "v3",
		Title:                "Scale",
		QuestionnaireCode:    "Q-1",
		QuestionnaireVersion: "1.0.0",
		DefinitionV2:         definition,
	}
	payload := scalepayload.ScaleSnapshotFromDefinition(scalepayload.ExecutionEnvelope{Code: snapshot.Code, ScaleVersion: snapshot.Version, Title: snapshot.Title, QuestionnaireCode: snapshot.QuestionnaireCode, QuestionnaireVersion: snapshot.QuestionnaireVersion, Status: "published"}, definition)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Payload = encoded
	if issues := validateScalePayload(snapshot); len(issues) != 0 {
		t.Fatalf("validateScalePayload() = %#v", issues)
	}
}

func TestValidateScalePayloadRejectsMismatch(t *testing.T) {
	definition := &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}}}
	snapshot := &modelcatalogport.AssessmentSnapshot{Code: "SCALE-1", Version: "v1", Title: "Scale", QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0", DefinitionV2: definition, Payload: []byte(`{"Code":"wrong"}`)}
	if issues := validateScalePayload(snapshot); len(issues) != 1 || issues[0].Rule != "scale.payload.definition_mismatch" {
		t.Fatalf("validateScalePayload() = %#v", issues)
	}
}
