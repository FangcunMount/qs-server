package evaluation

import (
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

	if modelRef := domain.EvaluationModelRef(); modelRef != nil && !modelRef.IsEmpty() {
		modelKind := modelRef.Kind().String()
		modelCode := modelRef.Code().String()
		modelVersion := modelRef.Version()
		modelTitle := modelRef.Title()
		po.EvaluationModelKind = &modelKind
		po.EvaluationModelCode = &modelCode
		po.EvaluationModelVersion = &modelVersion
		po.EvaluationModelTitle = &modelTitle
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
	po.EvaluatedAt = domain.EvaluatedAt()
	po.FailedAt = domain.FailedAt()
	po.FailureReason = domain.FailureReason()

	applyAssessmentOutcomeV2Fields(po, domain)

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *AssessmentMapper) ToDomain(po *AssessmentPO) *assessment.Assessment {
	if po == nil {
		return nil
	}

	// 构建问卷引用（使用 Code 作为唯一标识）
	questionnaireRef := assessment.NewQuestionnaireRefByCode(
		meta.NewCode(po.QuestionnaireCode),
		po.QuestionnaireVersion,
	)

	// 构建答卷引用
	answerSheetRef := assessment.NewAnswerSheetRef(mustMetaIDFromUint64("assessment.answer_sheet_id", po.AnswerSheetID))

	// 构建来源信息
	origin := assessment.ReconstructOrigin(assessment.OriginType(po.OriginType), po.OriginID)

	var modelRef *assessment.EvaluationModelRef
	if po.EvaluationModelKind != nil && po.EvaluationModelCode != nil {
		version := ""
		if po.EvaluationModelVersion != nil {
			version = *po.EvaluationModelVersion
		}
		title := ""
		if po.EvaluationModelTitle != nil {
			title = *po.EvaluationModelTitle
		}
		ref := assessment.NewEvaluationModelRefWithIdentity(
			assessment.EvaluationModelKind(*po.EvaluationModelKind),
			subKindFromPO(po),
			algorithmFromPO(po),
			meta.ID(0),
			meta.NewCode(*po.EvaluationModelCode),
			version,
			title,
		)
		modelRef = &ref
	}

	// 构建风险等级（可选）
	var riskLevel *assessment.RiskLevel
	if po.RiskLevel != nil {
		rl := assessment.RiskLevel(*po.RiskLevel)
		riskLevel = &rl
	}

	// 使用 Reconstruct 重建领域对象
	a := assessment.Reconstruct(
		po.ID,
		po.OrgID,
		mustTesteeIDFromUint64("assessment.testee_id", po.TesteeID),
		questionnaireRef,
		answerSheetRef,
		origin,
		assessment.Status(po.Status),
		po.TotalScore,
		riskLevel,
		po.SubmittedAt,
		po.EvaluatedAt,
		po.FailedAt,
		po.FailureReason,
		modelRef,
	)
	return a
}

// SyncID 同步ID
func (m *AssessmentMapper) SyncID(po *AssessmentPO, domain *assessment.Assessment) {
	domain.AssignID(assessment.NewID(mustUint64FromMetaID("assessment.id", po.ID)))
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

// ScoreMapper writes the mutable assessment_score projection. Outcome is the
// only source from which this projection is built; persisted rows are never
// mapped back into a domain score fact.
type ScoreMapper struct{}

// NewScoreMapper 创建得分映射器
func NewScoreMapper() *ScoreMapper {
	return &ScoreMapper{}
}

// ToPOs 将领域对象转换为持久化对象列表（一个 AssessmentScore 对应多个 PO）
func (m *ScoreMapper) ToPOs(domain *assessment.ScaleScoreProjection, testeeID uint64, outcomeID meta.ID) []*AssessmentScorePO {
	if domain == nil {
		return nil
	}

	factorScores := domain.FactorScores()
	pos := make([]*AssessmentScorePO, 0, len(factorScores))

	for _, fs := range factorScores {
		po := &AssessmentScorePO{
			AssessmentID: domain.AssessmentID().Uint64(),
			TesteeID:     testeeID,
			FactorCode:   fs.FactorCode().String(),
			FactorName:   fs.FactorName(),
			IsTotalScore: fs.IsTotalScore(),
			RawScore:     fs.RawScore(),
			RiskLevel:    string(fs.RiskLevel()),
		}
		if !outcomeID.IsZero() {
			id := outcomeID.Uint64()
			po.EvaluationOutcomeID = &id
		}
		pos = append(pos, po)
	}

	return pos
}
