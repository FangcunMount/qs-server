package evaluation

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AssessmentMapper 测评映射器
type AssessmentMapper struct{}

// NewAssessmentMapper 创建测评映射器
func NewAssessmentMapper() *AssessmentMapper {
	return &AssessmentMapper{}
}

// ToPO 将领域对象转换为持久化对象
func (m *AssessmentMapper) ToPO(domain *assessment.Assessment) *AssessmentPO {
	if domain == nil {
		return nil
	}

	po := &AssessmentPO{
		OrgID:                int64(domain.OrgID()),
		TesteeID:             domain.TesteeID().Uint64(),
		QuestionnaireID:      domain.QuestionnaireRef().ID().Uint64(),
		QuestionnaireCode:    domain.QuestionnaireRef().Code().String(),
		QuestionnaireVersion: domain.QuestionnaireRef().Version(),
		AnswerSheetID:        domain.AnswerSheetRef().ID().Uint64(),
		OriginType:           domain.Origin().Type().String(),
		Status:               domain.Status().String(),
	}

	// 设置ID（如果已存在）
	if !domain.ID().IsZero() {
		po.ID = domain.ID()
	}

	// 量表引用（可选）
	if scaleRef := domain.MedicalScaleRef(); scaleRef != nil {
		scaleID := scaleRef.ID().Uint64()
		scaleCode := scaleRef.Code().String()
		scaleName := scaleRef.Name()
		po.MedicalScaleID = &scaleID
		po.MedicalScaleCode = &scaleCode
		po.MedicalScaleName = &scaleName
	}

	// 来源ID（可选）
	if originID := domain.Origin().ID(); originID != nil {
		po.OriginID = originID
	}

	// 评估结果（可选）
	if totalScore := domain.TotalScore(); totalScore != nil {
		po.TotalScore = totalScore
	}
	if riskLevel := domain.RiskLevel(); riskLevel != nil {
		rl := string(*riskLevel)
		po.RiskLevel = &rl
	}

	// 时间戳
	po.SubmittedAt = domain.SubmittedAt()
	po.InterpretedAt = domain.InterpretedAt()
	po.FailedAt = domain.FailedAt()
	po.FailureReason = domain.FailureReason()

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *AssessmentMapper) ToDomain(po *AssessmentPO) *assessment.Assessment {
	if po == nil {
		return nil
	}

	// 构建问卷引用
	questionnaireRef := assessment.NewQuestionnaireRef(
		meta.ID(po.QuestionnaireID),
		meta.NewCode(po.QuestionnaireCode),
		po.QuestionnaireVersion,
	)

	// 构建答卷引用
	answerSheetRef := assessment.NewAnswerSheetRef(meta.ID(po.AnswerSheetID))

	// 构建来源信息
	origin := assessment.ReconstructOrigin(assessment.OriginType(po.OriginType), po.OriginID)

	// 构建量表引用（可选）
	var scaleRef *assessment.MedicalScaleRef
	if po.MedicalScaleID != nil && po.MedicalScaleCode != nil {
		name := ""
		if po.MedicalScaleName != nil {
			name = *po.MedicalScaleName
		}
		ref := assessment.NewMedicalScaleRef(
			meta.ID(*po.MedicalScaleID),
			meta.NewCode(*po.MedicalScaleCode),
			name,
		)
		scaleRef = &ref
	}

	// 构建风险等级（可选）
	var riskLevel *assessment.RiskLevel
	if po.RiskLevel != nil {
		rl := assessment.RiskLevel(*po.RiskLevel)
		riskLevel = &rl
	}

	// 使用 Reconstruct 重建领域对象
	return assessment.Reconstruct(
		po.ID,
		po.OrgID,
		testee.ID(po.TesteeID),
		questionnaireRef,
		answerSheetRef,
		scaleRef,
		origin,
		assessment.Status(po.Status),
		po.TotalScore,
		riskLevel,
		po.CreatedAt,
		po.SubmittedAt,
		po.InterpretedAt,
		po.FailedAt,
		po.FailureReason,
	)
}

// SyncID 同步ID
func (m *AssessmentMapper) SyncID(po *AssessmentPO, domain *assessment.Assessment) {
	// Assessment 使用 meta.ID，需要反射或暴露方法来设置ID
	// 由于领域对象可能需要设置ID的方法，这里暂时跳过
	// 如果需要，可以在 Assessment 中添加 SetID 方法
}

// ToDomainList 批量转换持久化对象为领域对象
func (m *AssessmentMapper) ToDomainList(pos []*AssessmentPO) []*assessment.Assessment {
	if len(pos) == 0 {
		return nil
	}

	result := make([]*assessment.Assessment, 0, len(pos))
	for _, po := range pos {
		if domain := m.ToDomain(po); domain != nil {
			result = append(result, domain)
		}
	}
	return result
}

// ==================== Score Mapper ====================

// ScoreMapper 得分映射器
// 注意：PO 是按因子扁平化存储的（每行一个 Factor），
// 而领域对象 AssessmentScore 是聚合的（包含多个 FactorScore）。
// 因此 ToDomain 需要处理聚合逻辑，ToPO 需要打散为多行。
type ScoreMapper struct{}

// NewScoreMapper 创建得分映射器
func NewScoreMapper() *ScoreMapper {
	return &ScoreMapper{}
}

// ToPOs 将领域对象转换为持久化对象列表（一个 AssessmentScore 对应多个 PO）
func (m *ScoreMapper) ToPOs(domain *assessment.AssessmentScore, testeeID uint64, scaleID uint64, scaleCode string) []*AssessmentScorePO {
	if domain == nil {
		return nil
	}

	factorScores := domain.FactorScores()
	pos := make([]*AssessmentScorePO, 0, len(factorScores))

	for _, fs := range factorScores {
		po := &AssessmentScorePO{
			AssessmentID:     domain.AssessmentID().Uint64(),
			TesteeID:         testeeID,
			MedicalScaleID:   scaleID,
			MedicalScaleCode: scaleCode,
			FactorCode:       fs.FactorCode().String(),
			FactorName:       fs.FactorName(),
			IsTotalScore:     fs.IsTotalScore(),
			RawScore:         fs.RawScore(),
			RiskLevel:        string(fs.RiskLevel()),
			Conclusion:       "", // 解读内容由 InterpretReport 管理
			Suggestion:       "", // 建议内容由 InterpretReport 管理
		}
		pos = append(pos, po)
	}

	return pos
}

// ToDomain 将持久化对象列表转换为领域对象（多个 PO 聚合为一个 AssessmentScore）
// 要求输入的 POs 必须属于同一个 Assessment
func (m *ScoreMapper) ToDomain(pos []*AssessmentScorePO) *assessment.AssessmentScore {
	if len(pos) == 0 {
		return nil
	}

	// 取第一个 PO 作为参考（假设同一 Assessment 的 PO 共享这些字段）
	firstPO := pos[0]
	assessmentID := meta.ID(firstPO.AssessmentID)

	// 计算总分和风险等级（从总分因子获取）
	var totalScore float64
	var riskLevel assessment.RiskLevel = assessment.RiskLevelNone

	// 构建因子得分列表
	factorScores := make([]assessment.FactorScore, 0, len(pos))
	for _, po := range pos {
		fs := assessment.NewFactorScore(
			assessment.FactorCode(po.FactorCode),
			po.FactorName,
			po.RawScore,
			assessment.RiskLevel(po.RiskLevel),
			po.IsTotalScore,
		)
		factorScores = append(factorScores, fs)

		// 如果是总分因子，提取总分和风险等级
		if po.IsTotalScore {
			totalScore = po.RawScore
			riskLevel = assessment.RiskLevel(po.RiskLevel)
		}
	}

	return assessment.ReconstructAssessmentScore(
		assessmentID,
		totalScore,
		riskLevel,
		factorScores,
		firstPO.CreatedAt,
	)
}

// GroupByAssessmentID 按 AssessmentID 分组
func (m *ScoreMapper) GroupByAssessmentID(pos []*AssessmentScorePO) map[uint64][]*AssessmentScorePO {
	grouped := make(map[uint64][]*AssessmentScorePO)
	for _, po := range pos {
		grouped[po.AssessmentID] = append(grouped[po.AssessmentID], po)
	}
	return grouped
}

// ToDomainList 将 PO 列表转换为领域对象列表（按 AssessmentID 聚合）
func (m *ScoreMapper) ToDomainList(pos []*AssessmentScorePO) []*assessment.AssessmentScore {
	grouped := m.GroupByAssessmentID(pos)
	result := make([]*assessment.AssessmentScore, 0, len(grouped))
	for _, group := range grouped {
		if score := m.ToDomain(group); score != nil {
			result = append(result, score)
		}
	}
	return result
}

// ==================== 辅助函数 ====================

// timePtr 获取时间指针
func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
