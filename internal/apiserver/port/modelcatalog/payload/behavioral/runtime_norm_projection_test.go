package behavioral_test

import (
	"context"
	"testing"
	"time"

	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestSnapshotFromDefinitionPreservesNormLookupContract(t *testing.T) {
	t.Parallel()

	standardScore := 108.0
	table := &norm.Norm{
		TableVersion: "brief2-parent-2026",
		FormVariant:  "parent",
		Kind:         identity.KindBehavioralRating,
		Algorithm:    identity.AlgorithmBrief2,
		Factors: []norm.FactorTable{{
			FactorCode: "gec",
			Lookup: []norm.LookupEntry{{
				RawScoreMin: 10, RawScoreMax: 10,
				MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female",
				TScore: 55, Percentile: 69, StandardScore: &standardScore,
			}},
		}},
	}
	def := &definition.Definition{
		Calibration: definition.Calibration{NormRefs: []norm.Ref{{FactorCode: "gec", NormTableVersion: table.TableVersion}}},
		Conclusions: []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "gec", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true}},
	}

	snapshot, err := behavioral.SnapshotFromDefinition(
		behavioral.DefinitionEnvelope{Code: "BRIEF2", Version: "1", Status: "published"},
		def,
		map[string]*norm.Norm{table.TableVersion: table},
	)
	if err != nil {
		t.Fatalf("SnapshotFromDefinition: %v", err)
	}
	if snapshot.Norming == nil || snapshot.Norming.NormTables == nil || len(snapshot.Norming.NormTables.Factors) != 1 {
		t.Fatalf("norming = %#v", snapshot.Norming)
	}
	if len(snapshot.Norming.RequiredFactorCodes) != 1 || snapshot.Norming.RequiredFactorCodes[0] != "gec" {
		t.Fatalf("required factors = %#v", snapshot.Norming.RequiredFactorCodes)
	}
	rows := snapshot.Norming.NormTables.Factors[0].Lookup
	if len(rows) != 1 {
		t.Fatalf("lookup rows = %#v", rows)
	}
	got := rows[0]
	if got.MinAgeMonths != 60 || got.MaxAgeMonths != 95 || got.Gender != "female" {
		t.Fatalf("demographic scope = %#v", got)
	}
	if got.StandardScore == nil || *got.StandardScore != standardScore {
		t.Fatalf("standard score = %#v", got.StandardScore)
	}
	if got.StandardScore == table.Factors[0].Lookup[0].StandardScore {
		t.Fatal("standard score pointer aliases catalog storage")
	}
}

func TestBehavioralOutcomeCodeSurvivesDefinitionToInterpretation(t *testing.T) {
	t.Parallel()

	for _, algorithm := range []identity.Algorithm{identity.AlgorithmBrief2, identity.AlgorithmSPMSensory} {
		algorithm := algorithm
		t.Run(string(algorithm), func(t *testing.T) {
			t.Parallel()

			const (
				factorCode  = "total"
				legacyLevel = "none"
				outcomeCode = "normal"
			)
			tableVersion := string(algorithm) + "-contract-v1"
			table := &norm.Norm{
				TableVersion: tableVersion,
				FormVariant:  "parent",
				Kind:         identity.KindBehavioralRating,
				Algorithm:    algorithm,
				Factors: []norm.FactorTable{{
					FactorCode: factorCode,
					Lookup: []norm.LookupEntry{{
						RawScoreMin: 0, RawScoreMax: 10, TScore: 55, Percentile: 69,
					}},
				}},
			}
			def := &definition.Definition{
				Calibration: definition.Calibration{NormRefs: []norm.Ref{{FactorCode: factorCode, NormTableVersion: tableVersion}}},
				Conclusions: []conclusion.Conclusion{conclusion.NormConclusion{
					FactorCode: factorCode,
					ScoreBasis: conclusion.ScoreBasisTScore,
					Primary:    true,
					Rules: []conclusion.ScoreRangeOutcome{{
						MinScore: 0, MaxScore: 100, MaxInclusive: true,
						Level: legacyLevel, OutcomeCode: outcomeCode,
						Summary: "状态正常", Description: "保持当前节奏",
					}},
					Outcomes: []conclusion.Outcome{{Code: outcomeCode, Title: "正常", Summary: "状态正常", Description: "保持当前节奏"}},
				}},
			}

			snapshot, err := behavioral.SnapshotFromDefinition(
				behavioral.DefinitionEnvelope{Code: "BEHAVIORAL", Version: "v1", Title: "行为测评", Status: "published"},
				def,
				map[string]*norm.Norm{tableVersion: table},
			)
			if err != nil {
				t.Fatalf("SnapshotFromDefinition: %v", err)
			}
			if got := snapshot.Norming.NormTables.TScoreRules[0].Ranges[0].Level; got != outcomeCode {
				t.Fatalf("runtime outcome code = %q, want %q (legacy Level=%q)", got, outcomeCode, legacyLevel)
			}

			execution, err := factornorm.ApplyNormProjection(&domainoutcome.Execution{
				Dimensions: []domainoutcome.DimensionResult{{
					Code:  factorCode,
					Name:  "总分",
					Kind:  domainoutcome.DimensionKindFactor,
					Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 5},
				}},
			}, snapshot, calcnorm.Subject{})
			if err != nil {
				t.Fatalf("ApplyNormProjection: %v", err)
			}
			if execution.Level == nil || execution.Level.Code != outcomeCode || execution.Dimensions[0].Level == nil || execution.Dimensions[0].Level.Code != outcomeCode {
				t.Fatalf("outcome levels = result:%#v dimension:%#v, want %q", execution.Level, execution.Dimensions[0].Level, outcomeCode)
			}

			dimension := execution.Dimensions[0]
			reportLevel := &report.ResultLevel{Code: dimension.Level.Code}
			reference := &report.NormReference{
				ScoreKind: string(dimension.NormReference.ScoreKind), Benchmark: dimension.NormReference.Benchmark,
				TableVersion: dimension.NormReference.TableVersion, FormVariant: dimension.NormReference.FormVariant,
			}
			assets := def.ResolvedInterpretationAssets()
			reportBuilder := rendering.NewNormProfileBuilder(builder.NewDefaultReportBuilder())
			draft, err := reportBuilder.Build(context.Background(), interpinput.InterpretationInput{
				OutcomeID:   meta.FromUint64(100),
				Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(101), TesteeID: 102},
				Model: report.ModelIdentity{
					Kind: string(identity.KindBehavioralRating), Algorithm: string(algorithm), Code: "BEHAVIORAL", Version: "v1", Title: "行为测评",
				},
				Runtime: interpinput.RuntimeIdentity{AlgorithmFamily: identity.AlgorithmFamilyFactorNorm, DecisionKind: identity.DecisionKindNormLookup},
				Result:  interpinput.ResultFacts{Primary: report.NewRawTotalScore(5, nil), Level: reportLevel},
				Report:  interpinput.ReportSpec{ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1},
				FactorScoring: &interpinput.FactorScoringFacts{
					Model:   &reportscore.ReportModel{Code: "BEHAVIORAL", Title: "行为测评", Assets: &assets, Factors: []reportscore.FactorReportModel{{Code: factorCode, Title: "总分", IsTotalScore: true}}},
					Factors: []reportscore.FactorReportScore{{FactorCode: factorCode, FactorName: "总分", RawScore: 5, Level: reportLevel, NormReference: reference, IsTotalScore: true}},
				},
			})
			if err != nil {
				t.Fatalf("Build Interpretation draft: %v", err)
			}
			artifact, err := report.NewInterpretReport(report.InterpretReportInput{
				ID: meta.FromUint64(103), GenerationID: meta.FromUint64(104), OutcomeID: meta.FromUint64(100), InterpretationRunID: meta.FromUint64(105),
				Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(101), TesteeID: 102},
				ReportType:  policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
				BuilderIdentity: reportBuilder.BuilderIdentity(), ContentSchemaVersion: reportBuilder.ContentSchemaVersion(),
				Content: draft.Content(), GeneratedAt: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("NewInterpretReport: %v", err)
			}
			content := artifact.Content()
			if content.Level == nil || content.Level.Code != outcomeCode || content.Conclusion != "状态正常" {
				t.Fatalf("artifact = level:%#v conclusion:%q", content.Level, content.Conclusion)
			}
		})
	}
}
