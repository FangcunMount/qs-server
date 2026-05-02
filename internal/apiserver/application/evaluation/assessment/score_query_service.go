package assessment

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// scoreQueryService 得分查询服务实现
// 行为者：报告查询者、数据分析系统
type scoreQueryService struct {
	scoreReader      evaluationreadmodel.ScoreReader
	assessmentReader evaluationreadmodel.AssessmentReader
	scaleCatalog     evaluationinput.ScaleCatalog
}

func NewScoreQueryServiceWithReadModel(
	scoreReader evaluationreadmodel.ScoreReader,
	assessmentReader evaluationreadmodel.AssessmentReader,
	scaleCatalog evaluationinput.ScaleCatalog,
) ScoreQueryService {
	return &scoreQueryService{
		scoreReader:      scoreReader,
		assessmentReader: assessmentReader,
		scaleCatalog:     scaleCatalog,
	}
}

// GetByAssessmentID 获取测评的所有因子得分
func (s *scoreQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ScoreResult, error) {
	if s.scoreReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "assessment score read model is not configured")
	}
	scoreRow, err := s.scoreReader.GetScoreByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentScoreNotFound, "得分不存在")
	}
	medicalScale := s.loadScaleForAssessmentRow(ctx, assessmentID)
	return scoreRowToResult(scoreRow, medicalScale), nil
}

// GetFactorTrend 获取因子得分趋势
func (s *scoreQueryService) GetFactorTrend(ctx context.Context, dto GetFactorTrendDTO) (*FactorTrendResult, error) {
	if dto.TesteeID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}
	if dto.FactorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	limit := dto.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	if s.scoreReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "assessment score read model is not configured")
	}
	rows, err := s.scoreReader.ListFactorTrend(ctx, evaluationreadmodel.FactorTrendFilter{
		TesteeID:   dto.TesteeID,
		FactorCode: dto.FactorCode,
		Limit:      limit,
	})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询得分趋势失败")
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
	if s.scoreReader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "assessment score read model is not configured")
	}
	scoreRow, err := s.scoreReader.GetScoreByAssessmentID(ctx, assessmentID)
	if err != nil {
		if errors.ParseCoder(err).Code() == errorCode.ErrAssessmentScoreNotFound {
			return &HighRiskFactorsResult{
				AssessmentID:    assessmentID,
				HasHighRisk:     false,
				HighRiskFactors: nil,
				NeedsUrgentCare: false,
			}, nil
		}
		return nil, errors.WrapC(err, errorCode.ErrAssessmentScoreNotFound, "得分不存在")
	}
	medicalScale := s.loadScaleForAssessmentRow(ctx, assessmentID)
	return highRiskFactorsResultFromScoreRow(assessmentID, scoreRow, medicalScale), nil
}

func (s *scoreQueryService) loadScaleForAssessmentRow(ctx context.Context, assessmentID uint64) *evaluationinput.ScaleSnapshot {
	if s.scaleCatalog == nil || s.assessmentReader == nil {
		return nil
	}
	row, err := s.assessmentReader.GetAssessment(ctx, assessmentID)
	if err != nil || row == nil || row.MedicalScaleCode == nil {
		return nil
	}
	medicalScale, err := s.scaleCatalog.GetScale(ctx, *row.MedicalScaleCode)
	if err != nil {
		return nil
	}
	return medicalScale
}
