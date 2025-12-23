package assessment

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// scoreQueryService 得分查询服务实现
// 行为者：报告查询者、数据分析系统
type scoreQueryService struct {
	scoreRepo      assessment.ScoreRepository
	assessmentRepo assessment.Repository
	scaleRepo      scale.Repository
}

// NewScoreQueryService 创建得分查询服务
func NewScoreQueryService(
	scoreRepo assessment.ScoreRepository,
	assessmentRepo assessment.Repository,
	scaleRepo scale.Repository,
) ScoreQueryService {
	return &scoreQueryService{
		scoreRepo:      scoreRepo,
		assessmentRepo: assessmentRepo,
		scaleRepo:      scaleRepo,
	}
}

// GetByAssessmentID 获取测评的所有因子得分
func (s *scoreQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ScoreResult, error) {
	id := meta.FromUint64(assessmentID)

	// 获取得分
	scores, err := s.scoreRepo.FindByAssessmentID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentScoreNotFound, "得分不存在")
	}

	if len(scores) == 0 {
		return nil, errors.WithCode(errorCode.ErrAssessmentScoreNotFound, "得分不存在")
	}

	// 获取测评信息以获取量表引用
	assessmentDomain, err := s.assessmentRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 获取量表信息（如果存在）
	var medicalScale *scale.MedicalScale
	if scaleRef := assessmentDomain.MedicalScaleRef(); scaleRef != nil {
		scaleCode := scaleRef.Code().String()
		medicalScale, err = s.scaleRepo.FindByCode(ctx, scaleCode)
		if err != nil {
			// 量表不存在时，不返回错误，只是没有 max_score 信息
			medicalScale = nil
		}
	}

	// 使用第一个得分（假设一个测评只有一个 AssessmentScore）
	return toScoreResult(scores[0], medicalScale), nil
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

	testeeID := testee.NewID(dto.TesteeID)
	factorCode := assessment.NewFactorCode(dto.FactorCode)

	scores, err := s.scoreRepo.FindByTesteeIDAndFactorCode(ctx, testeeID, factorCode, limit)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询得分趋势失败")
	}

	// 构造趋势数据点
	dataPoints := make([]TrendDataPoint, 0, len(scores))
	factorName := ""

	for _, score := range scores {
		// 从 AssessmentScore 中找到对应因子
		fs := score.GetFactorScore(factorCode)
		if fs != nil {
			if factorName == "" {
				factorName = fs.FactorName()
			}
			dataPoints = append(dataPoints, TrendDataPoint{
				AssessmentID: score.AssessmentID().Uint64(),
				RawScore:     fs.RawScore(),
				RiskLevel:    string(fs.RiskLevel()),
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
	id := meta.FromUint64(assessmentID)

	scores, err := s.scoreRepo.FindByAssessmentID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentScoreNotFound, "得分不存在")
	}

	if len(scores) == 0 {
		return &HighRiskFactorsResult{
			AssessmentID:    assessmentID,
			HasHighRisk:     false,
			HighRiskFactors: nil,
			NeedsUrgentCare: false,
		}, nil
	}

	// 获取测评信息以获取量表引用
	assessmentDomain, err := s.assessmentRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 获取量表信息（如果存在）
	var medicalScale *scale.MedicalScale
	if scaleRef := assessmentDomain.MedicalScaleRef(); scaleRef != nil {
		scaleCode := scaleRef.Code().String()
		medicalScale, err = s.scaleRepo.FindByCode(ctx, scaleCode)
		if err != nil {
			// 量表不存在时，不返回错误，只是没有 max_score 信息
			medicalScale = nil
		}
	}

	return toHighRiskFactorsResult(assessmentID, scores[0], medicalScale), nil
}
