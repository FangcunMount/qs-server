package questionnaire

import (
	"fmt"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// Converter 问卷文档转换器
type Converter struct{}

// NewConverter 创建转换器
func NewConverter() *Converter {
	return &Converter{}
}

// DomainToDocument 将领域对象转换为MongoDB文档
func (c *Converter) DomainToDocument(q *questionnaire.Questionnaire) *Document {
	if q == nil {
		return nil
	}

	// 转换问题
	questions := make([]QuestionDocument, len(q.Questions()))
	for i, domainQ := range q.Questions() {
		// 转换选项
		options := make([]OptionDocument, len(domainQ.Options()))
		for j, opt := range domainQ.Options() {
			options[j] = OptionDocument{
				ID:    opt.ID(),
				Text:  opt.Text(),
				Value: opt.Value(),
				Order: j,
			}
		}

		questions[i] = QuestionDocument{
			ID:       domainQ.ID(),
			Type:     string(domainQ.Type()),
			Title:    domainQ.Title(),
			Required: domainQ.Required(),
			Options:  options,
			Settings: domainQ.Settings(),
			Order:    i,
		}
	}

	// 转换设置
	settings := SettingsDocument{
		AllowAnonymous: q.Settings().AllowAnonymous(),
		ShowProgress:   q.Settings().ShowProgress(),
		RandomOrder:    q.Settings().RandomOrder(),
	}
	if timeLimit := q.Settings().TimeLimit(); timeLimit != nil {
		seconds := int64(timeLimit.Seconds())
		settings.TimeLimit = &seconds
	}

	doc := &Document{
		ID:        q.ID().Value(),
		Questions: questions,
		Settings:  settings,
		Version:   q.Version(),
		CreatedAt: q.CreatedAt(),
		UpdatedAt: q.UpdatedAt(),
	}

	return doc
}

// DocumentToResult 将MongoDB文档转换为存储结果
func (c *Converter) DocumentToResult(doc *Document) *storage.QuestionnaireDocumentResult {
	if doc == nil {
		return nil
	}

	// 转换问题
	questions := make([]storage.QuestionResult, len(doc.Questions))
	for i, q := range doc.Questions {
		// 转换选项
		options := make([]storage.OptionResult, len(q.Options))
		for j, opt := range q.Options {
			options[j] = storage.OptionResult{
				ID:    opt.ID,
				Text:  opt.Text,
				Value: opt.Value,
				Order: opt.Order,
			}
		}

		questions[i] = storage.QuestionResult{
			ID:       q.ID,
			Type:     q.Type,
			Title:    q.Title,
			Required: q.Required,
			Options:  options,
			Settings: q.Settings,
			Order:    q.Order,
		}
	}

	// 转换设置
	settings := storage.SettingsResult{
		AllowAnonymous: doc.Settings.AllowAnonymous,
		ShowProgress:   doc.Settings.ShowProgress,
		RandomOrder:    doc.Settings.RandomOrder,
	}
	if doc.Settings.TimeLimit != nil {
		duration := time.Duration(*doc.Settings.TimeLimit) * time.Second
		settings.TimeLimit = &duration
	}

	return &storage.QuestionnaireDocumentResult{
		ID:        doc.ID,
		Questions: questions,
		Settings:  settings,
		Version:   doc.Version,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}
}

// DocumentsToResults 批量转换文档为结果
func (c *Converter) DocumentsToResults(docs []*Document) []*storage.QuestionnaireDocumentResult {
	if len(docs) == 0 {
		return nil
	}

	results := make([]*storage.QuestionnaireDocumentResult, len(docs))
	for i, doc := range docs {
		results[i] = c.DocumentToResult(doc)
	}
	return results
}

// DocumentMapToResults 将文档映射转换为结果映射
func (c *Converter) DocumentMapToResults(docMap map[string]*Document) map[string]*storage.QuestionnaireDocumentResult {
	if len(docMap) == 0 {
		return nil
	}

	resultMap := make(map[string]*storage.QuestionnaireDocumentResult, len(docMap))
	for key, doc := range docMap {
		resultMap[key] = c.DocumentToResult(doc)
	}
	return resultMap
}

// ResultToDocument 将存储结果转换为MongoDB文档（用于更新操作）
func (c *Converter) ResultToDocument(result *storage.QuestionnaireDocumentResult) *Document {
	if result == nil {
		return nil
	}

	// 转换问题
	questions := make([]QuestionDocument, len(result.Questions))
	for i, q := range result.Questions {
		// 转换选项
		options := make([]OptionDocument, len(q.Options))
		for j, opt := range q.Options {
			options[j] = OptionDocument{
				ID:    opt.ID,
				Text:  opt.Text,
				Value: opt.Value,
				Order: opt.Order,
			}
		}

		questions[i] = QuestionDocument{
			ID:       q.ID,
			Type:     q.Type,
			Title:    q.Title,
			Required: q.Required,
			Options:  options,
			Settings: q.Settings,
			Order:    q.Order,
		}
	}

	// 转换设置
	settings := SettingsDocument{
		AllowAnonymous: result.Settings.AllowAnonymous,
		ShowProgress:   result.Settings.ShowProgress,
		RandomOrder:    result.Settings.RandomOrder,
	}
	if result.Settings.TimeLimit != nil {
		seconds := int64(result.Settings.TimeLimit.Seconds())
		settings.TimeLimit = &seconds
	}

	return &Document{
		ID:        result.ID,
		Questions: questions,
		Settings:  settings,
		Version:   result.Version,
		CreatedAt: result.CreatedAt,
		UpdatedAt: result.UpdatedAt,
	}
}

// PrepareDocumentForSave 为保存操作准备文档
func (c *Converter) PrepareDocumentForSave(doc *Document) *Document {
	if doc == nil {
		return nil
	}

	// 创建副本以避免修改原文档
	savedDoc := *doc

	// 更新时间戳
	savedDoc.UpdateTimestamp()

	// 确保问题和选项的顺序
	savedDoc.SortQuestionsByOrder()
	for i := range savedDoc.Questions {
		savedDoc.Questions[i].SortOptionsByOrder()
	}

	return &savedDoc
}

// PrepareDocumentForUpdate 为更新操作准备文档
func (c *Converter) PrepareDocumentForUpdate(doc *Document) *Document {
	if doc == nil {
		return nil
	}

	// 创建副本以避免修改原文档
	updatedDoc := *doc

	// 更新时间戳
	updatedDoc.SetUpdatedAt(time.Now())

	// 增加版本号
	updatedDoc.SetVersion(updatedDoc.GetVersion() + 1)

	// 确保问题和选项的顺序
	updatedDoc.SortQuestionsByOrder()
	for i := range updatedDoc.Questions {
		updatedDoc.Questions[i].SortOptionsByOrder()
	}

	return &updatedDoc
}

// ValidateAndPrepare 验证并准备文档
func (c *Converter) ValidateAndPrepare(doc *Document, isUpdate bool) (*Document, error) {
	if doc == nil {
		return nil, fmt.Errorf("document cannot be nil")
	}

	// 验证文档
	if err := doc.Validate(); err != nil {
		return nil, fmt.Errorf("document validation failed: %w", err)
	}

	// 根据操作类型准备文档
	var preparedDoc *Document
	if isUpdate {
		preparedDoc = c.PrepareDocumentForUpdate(doc)
	} else {
		preparedDoc = c.PrepareDocumentForSave(doc)
	}

	return preparedDoc, nil
}

// BuildDocumentFilter 构建文档查询过滤器
func (c *Converter) BuildDocumentFilter(ids []string) map[string]interface{} {
	if len(ids) == 0 {
		return make(map[string]interface{})
	}

	if len(ids) == 1 {
		return map[string]interface{}{
			"_id": ids[0],
		}
	}

	return map[string]interface{}{
		"_id": map[string]interface{}{
			"$in": ids,
		},
	}
}

// BuildSearchFilter 构建搜索过滤器
func (c *Converter) BuildSearchFilter(query storage.DocumentSearchQuery) map[string]interface{} {
	filter := make(map[string]interface{})

	// 关键字搜索
	if query.Keyword != "" {
		filter["$or"] = []map[string]interface{}{
			{
				"questions.title": map[string]interface{}{
					"$regex":   query.Keyword,
					"$options": "i",
				},
			},
		}
	}

	return filter
}
