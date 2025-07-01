package questionnaire

import (
	"strconv"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

// QuestionnaireMapper 问卷映射器
type QuestionnaireMapper struct{}

// NewQuestionnaireMapper 创建问卷映射器
func NewQuestionnaireMapper() *QuestionnaireMapper {
	return &QuestionnaireMapper{}
}

// ToPO 将领域模型转换为MongoDB持久化对象
func (m *QuestionnaireMapper) ToPO(domainQuestionnaire *questionnaire.Questionnaire) *QuestionnairePO {
	po := &QuestionnairePO{
		DomainID:    domainQuestionnaire.ID.Value(),
		Code:        domainQuestionnaire.Code,
		Title:       domainQuestionnaire.Title,
		Description: domainQuestionnaire.Description,
		ImgUrl:      domainQuestionnaire.ImgUrl,
		Version:     domainQuestionnaire.Version,
		Status:      domainQuestionnaire.Status,
	}

	// 处理MongoDB ObjectID
	if domainQuestionnaire.ID.Value() != 0 {
		po.ID = m.ObjectIDFromUint64(domainQuestionnaire.ID.Value())
	}

	// 设置审计字段
	if !domainQuestionnaire.CreatedAt.IsZero() {
		po.CreatedAt = domainQuestionnaire.CreatedAt
	}
	if !domainQuestionnaire.UpdatedAt.IsZero() {
		po.UpdatedAt = domainQuestionnaire.UpdatedAt
	}
	if !domainQuestionnaire.DeletedAt.IsZero() {
		po.DeletedAt = &domainQuestionnaire.DeletedAt
	}

	po.CreatedBy = domainQuestionnaire.CreatedBy
	po.UpdatedBy = domainQuestionnaire.UpdatedBy
	if domainQuestionnaire.DeletedBy != 0 {
		po.DeletedBy = domainQuestionnaire.DeletedBy
	}

	return po
}

// ToBO 将MongoDB持久化对象转换为业务对象
func (m *QuestionnaireMapper) ToBO(po *QuestionnairePO) *questionnaire.Questionnaire {
	// 直接使用存储的 DomainID
	domain := &questionnaire.Questionnaire{
		ID:          questionnaire.NewQuestionnaireID(po.DomainID),
		Code:        po.Code,
		Title:       po.Title,
		Description: po.Description,
		ImgUrl:      po.ImgUrl,
		Version:     po.Version,
		Status:      po.Status,
	}

	// 设置审计字段
	domain.CreatedAt = po.CreatedAt
	domain.UpdatedAt = po.UpdatedAt
	if po.DeletedAt != nil {
		domain.DeletedAt = *po.DeletedAt
	}

	domain.CreatedBy = po.CreatedBy
	domain.UpdatedBy = po.UpdatedBy
	domain.DeletedBy = po.DeletedBy

	return domain
}

// ObjectIDFromUint64 将uint64转换为ObjectID
// 这是一个辅助方法，可以根据实际需要自定义转换逻辑
func (m *QuestionnaireMapper) ObjectIDFromUint64(id uint64) primitive.ObjectID {
	if id == 0 {
		return primitive.NewObjectID()
	}

	// 将uint64转换为12字节的ObjectID
	// 这里使用简单的转换方式，实际项目中可能需要更复杂的转换逻辑
	idStr := strconv.FormatUint(id, 16)
	if len(idStr) < 24 {
		// 补齐到24位十六进制字符串
		idStr = "000000000000000000000000" + idStr
		idStr = idStr[len(idStr)-24:]
	}

	objectID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return primitive.NewObjectID()
	}

	return objectID
}

// Uint64FromObjectID 将ObjectID转换为uint64
func (m *QuestionnaireMapper) Uint64FromObjectID(objectID primitive.ObjectID) uint64 {
	if objectID.IsZero() {
		return 0
	}

	// 使用ObjectID的时间戳部分作为uint64
	return uint64(objectID.Timestamp().Unix())
}
