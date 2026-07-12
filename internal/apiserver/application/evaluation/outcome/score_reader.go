package outcome

import (
	"context"
	"fmt"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type FactorScoreFact struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	MaxScore     *float64
	RiskLevel    string
	IsTotalScore bool
}

type ScoreFact struct {
	AssessmentID uint64
	TotalScore   float64
	RiskLevel    string
	FactorScores []FactorScoreFact
}

type TrendPointFact struct {
	AssessmentID uint64
	RawScore     float64
	RiskLevel    string
}
type FactorTrendFact struct {
	TesteeID   uint64
	FactorCode string
	FactorName string
	DataPoints []TrendPointFact
}

type ScoreFactReader interface {
	Get(context.Context, uint64) (*ScoreFact, error)
	Trend(context.Context, uint64, string, int) (*FactorTrendFact, error)
}

type scoreFactReader struct {
	outcomes    domainoutcome.Repository
	projections evaluationreadmodel.ScoreProjectionReader
	assessments evaluationreadmodel.AssessmentReader
	scales      evaluationinput.ScaleCatalog
}

func NewScoreFactReader(outcomes domainoutcome.Repository, projections evaluationreadmodel.ScoreProjectionReader, assessments evaluationreadmodel.AssessmentReader, scales evaluationinput.ScaleCatalog) ScoreFactReader {
	return &scoreFactReader{outcomes: outcomes, projections: projections, assessments: assessments, scales: scales}
}

func (r *scoreFactReader) Get(ctx context.Context, assessmentID uint64) (*ScoreFact, error) {
	if r == nil || r.outcomes == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation outcome repository is not configured")
	}
	record, err := r.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("evaluation outcome not found")
	}
	execution, err := RestoreExecution(record)
	if err != nil {
		return nil, err
	}
	projection := ScaleScoreProjectionFromExecution(record.AssessmentID(), execution)
	if projection == nil {
		return nil, fmt.Errorf("evaluation outcome does not project scale scores")
	}
	return scoreFactFromProjection(projection, r.loadScale(ctx, assessmentID)), nil
}

func (r *scoreFactReader) Trend(ctx context.Context, testeeID uint64, factorCode string, limit int) (*FactorTrendFact, error) {
	if testeeID == 0 || factorCode == "" {
		return nil, evalerrors.InvalidArgument("受试者ID和因子编码不能为空")
	}
	if r == nil || r.projections == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment score projection read model is not configured")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := r.projections.ListFactorTrend(ctx, evaluationreadmodel.FactorTrendFilter{TesteeID: testeeID, FactorCode: factorCode, Limit: limit})
	if err != nil {
		return nil, evalerrors.Database(err, "查询得分趋势失败")
	}
	result := &FactorTrendFact{TesteeID: testeeID, FactorCode: factorCode, DataPoints: make([]TrendPointFact, 0, len(rows))}
	for _, row := range rows {
		for _, factor := range row.FactorScores {
			if factor.FactorCode == factorCode {
				if result.FactorName == "" {
					result.FactorName = factor.FactorName
				}
				result.DataPoints = append(result.DataPoints, TrendPointFact{AssessmentID: row.AssessmentID, RawScore: factor.RawScore, RiskLevel: factor.RiskLevel})
			}
		}
	}
	return result, nil
}

func (r *scoreFactReader) loadScale(ctx context.Context, assessmentID uint64) *scalesnapshot.ScaleSnapshot {
	if r.scales == nil || r.assessments == nil {
		return nil
	}
	row, err := r.assessments.GetAssessment(ctx, assessmentID)
	if err != nil || row == nil || row.EvaluationModelKind == nil || *row.EvaluationModelKind != "scale" || row.EvaluationModelCode == nil {
		return nil
	}
	scale, err := r.scales.GetScale(ctx, *row.EvaluationModelCode)
	if err != nil {
		return nil
	}
	return scale
}

func scoreFactFromProjection(projection *domainassessment.ScaleScoreProjection, scale *scalesnapshot.ScaleSnapshot) *ScoreFact {
	result := &ScoreFact{AssessmentID: projection.AssessmentID().Uint64(), TotalScore: projection.TotalScore(), RiskLevel: projection.RiskLevel().String(), FactorScores: make([]FactorScoreFact, 0, len(projection.FactorScores()))}
	maxScores := factorMaxScores(scale)
	for _, factor := range projection.FactorScores() {
		factorCode := string(factor.FactorCode())
		result.FactorScores = append(result.FactorScores, FactorScoreFact{FactorCode: factorCode, FactorName: factor.FactorName(), RawScore: factor.RawScore(), MaxScore: maxScores[factorCode], RiskLevel: factor.RiskLevel().String(), IsTotalScore: factor.IsTotalScore()})
	}
	return result
}

func factorMaxScores(scale *scalesnapshot.ScaleSnapshot) map[string]*float64 {
	result := map[string]*float64{}
	if scale == nil {
		return result
	}
	for _, factor := range scale.Factors {
		if factor.MaxScore == nil {
			continue
		}
		value := *factor.MaxScore
		result[factor.Code] = &value
	}
	return result
}
