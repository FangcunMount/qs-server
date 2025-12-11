package questionnaire

import (
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// BaseInfo 基础信息领域服务
// 负责更新问卷的基础信息（标题、描述、封面图）
// 通过调用聚合根的私有方法来修改状态，保证领域完整性
type BaseInfo struct{}

// UpdateTitle 更新问卷标题
func (BaseInfo) UpdateTitle(q *Questionnaire, newTitle string) error {
	newTitle = strings.TrimSpace(newTitle)
	if len(newTitle) == 0 {
		return errors.WithCode(code.ErrQuestionnaireInvalidTitle, "标题不能为空")
	}
	if len(newTitle) > 100 {
		return errors.WithCode(code.ErrQuestionnaireInvalidTitle, "标题长度不能超过 100 字符")
	}

	return q.updateBasicInfo(newTitle, q.desc, q.imgUrl)
}

// UpdateDescription 更新问卷描述
func (BaseInfo) UpdateDescription(q *Questionnaire, newDescription string) error {
	if len(newDescription) > 500 {
		return errors.WithCode(code.ErrQuestionnaireInvalidInput, "描述长度不能超过 500 字符")
	}

	return q.updateBasicInfo(q.title, newDescription, q.imgUrl)
}

// UpdateCoverImage 更新问卷封面图
func (BaseInfo) UpdateCoverImage(q *Questionnaire, newImgUrl string) error {
	// 封面图URL可以为空，不做验证
	return q.updateBasicInfo(q.title, q.desc, newImgUrl)
}

// UpdateAll 批量更新基础信息
func (BaseInfo) UpdateAll(q *Questionnaire, title, description, imgUrl string, typ QuestionnaireType) error {
	title = strings.TrimSpace(title)
	if len(title) == 0 {
		return errors.WithCode(code.ErrQuestionnaireInvalidTitle, "标题不能为空")
	}
	if len(title) > 100 {
		return errors.WithCode(code.ErrQuestionnaireInvalidTitle, "标题长度不能超过 100 字符")
	}
	if len(description) > 500 {
		return errors.WithCode(code.ErrQuestionnaireInvalidInput, "描述长度不能超过 500 字符")
	}

	if err := q.updateBasicInfo(title, description, imgUrl); err != nil {
		return err
	}

	if typ != "" {
		if err := q.updateType(typ); err != nil {
			return err
		}
	}

	return nil
}
