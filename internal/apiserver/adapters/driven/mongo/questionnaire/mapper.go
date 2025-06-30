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

// ToDocument 将领域模型转换为MongoDB文档
func (m *QuestionnaireMapper) ToDocument(domainQuestionnaire *questionnaire.Questionnaire) *QuestionnaireDocument {
	doc := &QuestionnaireDocument{
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
		doc.ID = m.ObjectIDFromUint64(domainQuestionnaire.ID.Value())
	}

	// 设置审计字段
	if !domainQuestionnaire.CreatedAt.IsZero() {
		doc.CreatedAt = domainQuestionnaire.CreatedAt
	}
	if !domainQuestionnaire.UpdatedAt.IsZero() {
		doc.UpdatedAt = domainQuestionnaire.UpdatedAt
	}
	if !domainQuestionnaire.DeletedAt.IsZero() {
		doc.DeletedAt = &domainQuestionnaire.DeletedAt
	}

	doc.CreatedBy = domainQuestionnaire.CreatedBy
	doc.UpdatedBy = domainQuestionnaire.UpdatedBy
	if domainQuestionnaire.DeletedBy != 0 {
		doc.DeletedBy = domainQuestionnaire.DeletedBy
	}

	return doc
}

// ToDomain 将MongoDB文档转换为领域模型
func (m *QuestionnaireMapper) ToDomain(doc *QuestionnaireDocument) *questionnaire.Questionnaire {
	// 直接使用存储的 DomainID
	domain := &questionnaire.Questionnaire{
		ID:          questionnaire.NewQuestionnaireID(doc.DomainID),
		Code:        doc.Code,
		Title:       doc.Title,
		Description: doc.Description,
		ImgUrl:      doc.ImgUrl,
		Version:     doc.Version,
		Status:      doc.Status,
	}

	// 设置审计字段
	domain.CreatedAt = doc.CreatedAt
	domain.UpdatedAt = doc.UpdatedAt
	if doc.DeletedAt != nil {
		domain.DeletedAt = *doc.DeletedAt
	}

	domain.CreatedBy = doc.CreatedBy
	domain.UpdatedBy = doc.UpdatedBy
	domain.DeletedBy = doc.DeletedBy

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
