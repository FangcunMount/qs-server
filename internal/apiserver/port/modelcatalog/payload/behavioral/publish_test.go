package behavioral_test

import (
	"context"
	"encoding/json"
	"testing"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestBuildPublishedModelUsesDefinitionV2Decision(t *testing.T) {
	t.Parallel()

	model := &domain.AssessmentModel{
		Kind:       domain.KindBehavioralRating,
		Algorithm:  domain.AlgorithmBrief2,
		Code:       "brief2-v2",
		Version:    1,
		Title:      "Brief-2 V2",
		Definition: domain.DefinitionPayload{Data: []byte(`{"dimensions":[]}`)},
		DefinitionV2: &domain.Definition{
			Calibration: modeldefinition.Calibration{NormRefs: []norm.Ref{{FactorCode: "bri", NormTableVersion: "brief2-cn-2024"}}},
			Conclusions: []domain.Conclusion{
				domain.NormConclusion{FactorCode: "bri", Primary: true},
			},
		},
	}

	snapshot, err := (appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepositoryStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		Factors:      []norm.FactorTable{{FactorCode: "bri"}},
	}}}).BuildSnapshotPayload(context.Background(), model)
	if err != nil {
		t.Fatalf("BuildSnapshotPayload: %v", err)
	}
	if snapshot.DecisionKind != domain.DecisionKindNormLookup {
		t.Fatalf("decision kind = %q, want norm_lookup", snapshot.DecisionKind)
	}
}

func TestBuildPublishedModelPreservesConfiguredBrief2PrimaryDimension(t *testing.T) {
	t.Parallel()

	model := &domain.AssessmentModel{
		Kind:      domain.KindBehavioralRating,
		Algorithm: domain.AlgorithmBrief2,
		Code:      "brief2-demo",
		Version:   1,
		Title:     "Brief-2 Demo",
		Definition: domain.DefinitionPayload{
			Data: []byte(`{"dimensions":[],"brief2":{"primary_dimension_code":"bri"}}`),
		},
		DefinitionV2: &domain.Definition{
			Calibration: modeldefinition.Calibration{NormRefs: []norm.Ref{{FactorCode: "bri", NormTableVersion: "brief2-cn-2024"}}},
			Conclusions: []domain.Conclusion{
				domain.NormConclusion{FactorCode: "bri", Primary: true},
			},
			Execution: modeldefinition.ExecutionSpec{Brief2: &modeldefinition.Brief2Spec{
				PrimaryFactorCode: "bri",
			}},
		},
	}

	snapshot, err := (appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepositoryStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		Factors:      []norm.FactorTable{{FactorCode: "bri"}},
	}}}).BuildSnapshotPayload(context.Background(), model)
	if err != nil {
		t.Fatalf("Build published model: %v", err)
	}
	var body struct {
		Brief2 struct {
			PrimaryDimensionCode string `json:"primary_dimension_code"`
		} `json:"brief2"`
	}
	if err := json.Unmarshal(snapshot.Payload, &body); err != nil {
		t.Fatalf("decode published payload: %v", err)
	}
	if body.Brief2.PrimaryDimensionCode != "bri" {
		t.Fatalf("primary_dimension_code = %q, want bri", body.Brief2.PrimaryDimensionCode)
	}
}

func TestBuildPublishedModelProjectsBrief2NormFromRepository(t *testing.T) {
	t.Parallel()

	mean := 12.0
	model := &domain.AssessmentModel{
		Kind:       domain.KindBehavioralRating,
		Algorithm:  domain.AlgorithmBrief2,
		Code:       "brief2-norm",
		Version:    1,
		Title:      "Brief-2 Norm",
		Definition: domain.DefinitionPayload{Data: []byte(`not-json`)},
		DefinitionV2: &domain.Definition{
			Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{
				{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex},
				{Code: "inconsistency", Title: "Inconsistency", Role: factor.FactorRoleValidity},
			}},
			Calibration: modeldefinition.Calibration{NormRefs: []norm.Ref{{FactorCode: "bri", NormTableVersion: "brief2-cn-2024"}}},
			Conclusions: []domain.Conclusion{domain.NormConclusion{FactorCode: "bri", Primary: true}},
		},
	}
	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepositoryStub{table: &norm.Norm{
		TableVersion: "brief2-cn-2024",
		FormVariant:  "teacher",
		Factors: []norm.FactorTable{{
			FactorCode: "bri",
			Bands:      []norm.Band{{MinAgeMonths: 72, MaxAgeMonths: 84, Gender: "male", Mean: &mean}},
			Lookup:     []norm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 3, TScore: 50, Percentile: 50}},
		}},
	}}}

	snapshot, err := handler.BuildSnapshotPayload(context.Background(), model)
	if err != nil {
		t.Fatalf("BuildSnapshotPayload: %v", err)
	}
	var body struct {
		Brief2 struct {
			FormVariant      string   `json:"form_variant"`
			NormTableVersion string   `json:"norm_table_version"`
			IndexCodes       []string `json:"index_codes"`
			ValidityCodes    []string `json:"validity_codes"`
			Norms            []struct {
				FactorCode string `json:"factor_code"`
				Lookup     []struct {
					TScore float64 `json:"t_score"`
				} `json:"lookup"`
			} `json:"norms"`
		} `json:"brief2"`
	}
	if err := json.Unmarshal(snapshot.Payload, &body); err != nil {
		t.Fatalf("decode published payload: %v", err)
	}
	if body.Brief2.FormVariant != "teacher" || body.Brief2.NormTableVersion != "brief2-cn-2024" {
		t.Fatalf("brief2 table = %#v", body.Brief2)
	}
	if len(body.Brief2.IndexCodes) != 1 || body.Brief2.IndexCodes[0] != "bri" || len(body.Brief2.ValidityCodes) != 1 || body.Brief2.ValidityCodes[0] != "inconsistency" {
		t.Fatalf("brief2 roles = %#v", body.Brief2)
	}
	if len(body.Brief2.Norms) != 1 || body.Brief2.Norms[0].FactorCode != "bri" || len(body.Brief2.Norms[0].Lookup) != 1 || body.Brief2.Norms[0].Lookup[0].TScore != 50 {
		t.Fatalf("brief2 norms = %#v", body.Brief2.Norms)
	}
}

func TestBuildPublishedModelRejectsMismatchedBrief2NormVersion(t *testing.T) {
	t.Parallel()

	model := &domain.AssessmentModel{
		Kind:      domain.KindBehavioralRating,
		Algorithm: domain.AlgorithmBrief2,
		DefinitionV2: &domain.Definition{
			Calibration: modeldefinition.Calibration{NormRefs: []norm.Ref{{FactorCode: "bri", NormTableVersion: "brief2-cn-2024"}}},
		},
	}
	_, err := (appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepositoryStub{table: &norm.Norm{TableVersion: "other"}}}).BuildSnapshotPayload(context.Background(), model)
	if err == nil {
		t.Fatal("BuildSnapshotPayload error = nil, want norm version mismatch")
	}
}

type normRepositoryStub struct {
	table *norm.Norm
	err   error
}

func (s normRepositoryStub) UpsertNorm(context.Context, *norm.Norm) error { return nil }

func (s normRepositoryStub) ListNorms(context.Context, modelcatalogport.NormListFilter) ([]*norm.Norm, int64, error) {
	if s.table == nil {
		return nil, 0, s.err
	}
	return []*norm.Norm{s.table}, 1, s.err
}

func (s normRepositoryStub) FindNorm(context.Context, string) (*norm.Norm, error) {
	return s.table, s.err
}

var _ modelcatalogport.NormRepository = normRepositoryStub{}
