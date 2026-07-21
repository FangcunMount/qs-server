package behavioral_test

import (
	"context"
	"testing"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralruntime "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestBehavioralMaterializationUsesDefinitionV2Identity(t *testing.T) {
	model := behavioralModel()
	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepositoryStub{table: behavioralNorm()}}
	result, err := handler.MaterializeSnapshot(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	if result.AlgorithmFamily != domain.AlgorithmFamilyFactorNorm || result.DecisionKind != domain.DecisionKindNormLookup {
		t.Fatalf("materialization = %#v", result)
	}
}

func TestBehavioralRuntimeIsBuiltDirectlyFromDefinitionAndNorm(t *testing.T) {
	model := behavioralModel()
	table := behavioralNorm()
	snapshot, err := behavioralruntime.SnapshotFromDefinition(behavioralruntime.DefinitionEnvelope{Code: model.Code, Version: "v1", Title: model.Title, Status: "published"}, model.DefinitionV2, map[string]*norm.Norm{table.TableVersion: table})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Norming == nil || snapshot.Norming.PrimaryDimensionCode != "bri" || snapshot.Norming.NormTableVersion != table.TableVersion || len(snapshot.Norming.RequiredFactorCodes) != 1 {
		t.Fatalf("norming = %#v", snapshot.Norming)
	}
}

func TestBehavioralMaterializationRejectsMismatchedNormVersion(t *testing.T) {
	_, err := (appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepositoryStub{table: behavioralNormWithVersion("other")}}).MaterializeSnapshot(context.Background(), behavioralModel())
	if err == nil {
		t.Fatal("expected norm version mismatch")
	}
}

func behavioralModel() *domain.AssessmentModel {
	return &domain.AssessmentModel{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2, Code: "brief2-v2", Version: 1, Title: "Brief-2 V2", DefinitionV2: &domain.Definition{
		Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex}}},
		Calibration: modeldefinition.Calibration{NormRefs: []norm.Ref{{FactorCode: "bri", NormTableVersion: "brief2-cn-2024"}}},
		Conclusions: []domain.Conclusion{domain.NormConclusion{FactorCode: "bri", ScoreBasis: domain.ScoreBasisTScore, Primary: true}},
		Execution: modeldefinition.ExecutionSpec{Brief2: &modeldefinition.Brief2Spec{PrimaryFactorCode: "bri"}},
	}}
}

func behavioralNorm() *norm.Norm { return behavioralNormWithVersion("brief2-cn-2024") }
func behavioralNormWithVersion(version string) *norm.Norm {
	return &norm.Norm{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2, TableVersion: version, FormVariant: "teacher", Factors: []norm.FactorTable{{FactorCode: "bri", Lookup: []norm.LookupEntry{{RawScoreMin: 0, RawScoreMax: 100, TScore: 50, Percentile: 50}}}}}
}

type normRepositoryStub struct{ table *norm.Norm; err error }
func (s normRepositoryStub) UpsertNorm(context.Context, *norm.Norm) error { return nil }
func (s normRepositoryStub) ListNorms(context.Context, modelcatalogport.NormListFilter) ([]*norm.Norm, int64, error) { return []*norm.Norm{s.table}, 1, s.err }
func (s normRepositoryStub) FindNorm(context.Context, string) (*norm.Norm, error) { return s.table, s.err }

var _ modelcatalogport.NormRepository = normRepositoryStub{}
