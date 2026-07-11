package assessment

import (
	"context"
	"fmt"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// scoreQueryService 得分查询服务实现
// 行为者：报告查询者、数据分析系统
type scoreQueryService struct {
	outcomes         domainoutcome.Repository
	projectionReader evaluationreadmodel.ScoreProjectionReader
	assessmentReader evaluationreadmodel.AssessmentReader
	scaleCatalog     evaluationinput.ScaleCatalog
}

// scoreQueryService 实现了 ScoreQueryService 接口
var _ ScoreQueryService = &scoreQueryService{}

// NewScoreQueryService 创建得分查询服务实例
func NewScoreQueryService(
	outcomes domainoutcome.Repository,
	projectionReader evaluationreadmodel.ScoreProjectionReader,
	assessmentReader evaluationreadmodel.AssessmentReader,
	scaleCatalog evaluationinput.ScaleCatalog,
) ScoreQueryService {
	return &scoreQueryService{
		outcomes:         outcomes,
		projectionReader: projectionReader,
		assessmentReader: assessmentReader,
		scaleCatalog:     scaleCatalog,
	}
}

// GetByAssessmentID 获取测评的所有因子得分
func (s *scoreQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ScoreResult, error) {
	result, err := s.scoreFromOutcome(ctx, assessmentID)
	if err != nil {
		return nil, evalerrors.AssessmentScoreNotFound(err, "得分不存在")
	}
	return result, nil
}

// GetFactorTrend 获取因子得分趋势
func (s *scoreQueryService) GetFactorTrend(ctx context.Context, dto GetFactorTrendDTO) (*FactorTrendResult, error) {
	if dto.TesteeID == 0 {
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	if dto.FactorCode == "" {
		return nil, evalerrors.InvalidArgument("因子编码不能为空")
	}

	limit := dto.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	if s.projectionReader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment score projection read model is not configured")
	}
	rows, err := s.projectionReader.ListFactorTrend(ctx, evaluationreadmodel.FactorTrendFilter{
		TesteeID:   dto.TesteeID,
		FactorCode: dto.FactorCode,
		Limit:      limit,
	})
	if err != nil {
		return nil, evalerrors.Database(err, "查询得分趋势失败")
	}
	dataPoints := make([]TrendDataPoint, 0, len(rows))
	factorName := ""
	for _, row := range rows {
		for _, fs := range row.FactorScores {
			if fs.FactorCode != dto.FactorCode {
				continue
			}
			if factorName == "" {
				factorName = fs.FactorName
			}
			dataPoints = append(dataPoints, TrendDataPoint{
				AssessmentID: row.AssessmentID,
				RawScore:     fs.RawScore,
				RiskLevel:    fs.RiskLevel,
			})
		}
	}
	return &FactorTrendResult{
		TesteeID:   dto.TesteeID,
		FactorCode: dto.FactorCode,
		FactorName: factorName,
		DataPoints: dataPoints,
	}, nil
}

// GetHighRiskFactors 获取高风险因子
func (s *scoreQueryService) GetHighRiskFactors(ctx context.Context, assessmentID uint64) (*HighRiskFactorsResult, error) {
	result, err := s.scoreFromOutcome(ctx, assessmentID)
	if err != nil {
		wrapped := evalerrors.AssessmentScoreNotFound(err, "得分不存在")
		if evalerrors.IsAssessmentScoreNotFound(wrapped) {
			return &HighRiskFactorsResult{
				AssessmentID:    assessmentID,
				HasHighRisk:     false,
				HighRiskFactors: nil,
				NeedsUrgentCare: false,
			}, nil
		}
		return nil, wrapped
	}
	return highRiskFactorsResultFromScoreResult(result), nil
}

func (s *scoreQueryService) scoreFromOutcome(ctx context.Context, assessmentID uint64) (*ScoreResult, error) {
	if s.outcomes == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation outcome repository is not configured")
	}
	record, err := s.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("evaluation outcome not found")
	}
	execution, err := evaloutcome.RestoreExecution(record)
	if err != nil {
		return nil, err
	}
	projection := domainassessment.ScaleScoreProjectionFromOutcome(record.AssessmentID(), evaloutcome.AssessmentOutcomeFromExecution(execution))
	if projection == nil {
		return nil, fmt.Errorf("evaluation outcome does not project scale scores")
	}
	return scoreResultFromScaleProjection(projection, s.loadScaleForAssessmentRow(ctx, assessmentID)), nil
}

func (s *scoreQueryService) loadScaleForAssessmentRow(ctx context.Context, assessmentID uint64) *scalesnapshot.ScaleSnapshot {
	if s.scaleCatalog == nil || s.assessmentReader == nil {
		return nil
	}
	row, err := s.assessmentReader.GetAssessment(ctx, assessmentID)
	if err != nil || row == nil || row.EvaluationModelKind == nil || *row.EvaluationModelKind != "scale" || row.EvaluationModelCode == nil {
		return nil
	}
	medicalScale, err := s.scaleCatalog.GetScale(ctx, *row.EvaluationModelCode)
	if err != nil {
		return nil
	}
	return medicalScale
}
