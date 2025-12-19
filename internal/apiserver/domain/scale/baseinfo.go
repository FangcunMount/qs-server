package scale

import (
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// BaseInfo 基础信息领域服务
// 负责更新量表的基础信息（标题、描述、关联问卷）
// 通过调用聚合根的私有方法来修改状态，保证领域完整性
type BaseInfo struct{}

// UpdateTitle 更新量表标题
func (BaseInfo) UpdateTitle(m *MedicalScale, newTitle string) error {
	newTitle = strings.TrimSpace(newTitle)
	if len(newTitle) == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "标题不能为空")
	}
	if len(newTitle) > 100 {
		return errors.WithCode(code.ErrInvalidArgument, "标题长度不能超过 100 字符")
	}

	return m.updateBasicInfo(newTitle, m.description)
}

// UpdateDescription 更新量表描述
func (BaseInfo) UpdateDescription(m *MedicalScale, newDescription string) error {
	if len(newDescription) > 500 {
		return errors.WithCode(code.ErrInvalidArgument, "描述长度不能超过 500 字符")
	}

	return m.updateBasicInfo(m.title, newDescription)
}

// UpdateAll 批量更新基础信息
func (BaseInfo) UpdateAll(m *MedicalScale, title, description string) error {
	title = strings.TrimSpace(title)
	if len(title) == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "标题不能为空")
	}
	if len(title) > 100 {
		return errors.WithCode(code.ErrInvalidArgument, "标题长度不能超过 100 字符")
	}
	if len(description) > 500 {
		return errors.WithCode(code.ErrInvalidArgument, "描述长度不能超过 500 字符")
	}

	return m.updateBasicInfo(title, description)
}

// UpdateClassificationInfo 更新分类信息
func (BaseInfo) UpdateClassificationInfo(m *MedicalScale, category Category, stage Stage, applicableAge ApplicableAge, reporters []Reporter, tags []Tag) error {
	// 验证类型值
	if !category.IsValid() {
		return errors.WithCode(code.ErrInvalidArgument, "类别值无效")
	}
	if !stage.IsValid() {
		return errors.WithCode(code.ErrInvalidArgument, "阶段值无效")
	}
	if !applicableAge.IsValid() {
		return errors.WithCode(code.ErrInvalidArgument, "使用年龄值无效")
	}

	// 验证填报人列表
	for _, reporter := range reporters {
		if !reporter.IsValid() {
			return errors.WithCode(code.ErrInvalidArgument, "填报人值无效: %s", reporter.String())
		}
	}

	// 验证标签（最多5个）
	if len(tags) > 5 {
		return errors.WithCode(code.ErrInvalidArgument, "标签数量不能超过5个")
	}
	for _, tag := range tags {
		if err := tag.Validate(); err != nil {
			return err
		}
	}

	return m.updateClassificationInfo(category, stage, applicableAge, reporters, tags)
}

// UpdateAllWithClassification 批量更新基础信息和分类信息
func (BaseInfo) UpdateAllWithClassification(m *MedicalScale, title, description string, category Category, stage Stage, applicableAge ApplicableAge, reporters []Reporter, tags []Tag) error {
	title = strings.TrimSpace(title)
	if len(title) == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "标题不能为空")
	}
	if len(title) > 100 {
		return errors.WithCode(code.ErrInvalidArgument, "标题长度不能超过 100 字符")
	}
	if len(description) > 500 {
		return errors.WithCode(code.ErrInvalidArgument, "描述长度不能超过 500 字符")
	}

	// 验证类型值
	if !category.IsValid() {
		return errors.WithCode(code.ErrInvalidArgument, "类别值无效")
	}
	if !stage.IsValid() {
		return errors.WithCode(code.ErrInvalidArgument, "阶段值无效")
	}
	if !applicableAge.IsValid() {
		return errors.WithCode(code.ErrInvalidArgument, "使用年龄值无效")
	}

	// 验证填报人列表
	for _, reporter := range reporters {
		if !reporter.IsValid() {
			return errors.WithCode(code.ErrInvalidArgument, "填报人值无效: %s", reporter.String())
		}
	}

	// 验证标签（最多5个）
	if len(tags) > 5 {
		return errors.WithCode(code.ErrInvalidArgument, "标签数量不能超过5个")
	}
	for _, tag := range tags {
		if err := tag.Validate(); err != nil {
			return err
		}
	}

	if err := m.updateBasicInfo(title, description); err != nil {
		return err
	}

	return m.updateClassificationInfo(category, stage, applicableAge, reporters, tags)
}

// UpdateQuestionnaire 更新关联的问卷
// 当问卷版本更新时，需要重新关联问卷版本
func (BaseInfo) UpdateQuestionnaire(m *MedicalScale, questionnaireCode meta.Code, questionnaireVersion string) error {
	if questionnaireCode.IsEmpty() {
		return errors.WithCode(code.ErrInvalidArgument, "问卷编码不能为空")
	}
	if questionnaireVersion == "" {
		return errors.WithCode(code.ErrInvalidArgument, "问卷版本不能为空")
	}

	return m.updateQuestionnaire(questionnaireCode, questionnaireVersion)
}
