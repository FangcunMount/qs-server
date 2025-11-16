package questionnaire

import (
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// BaseInfoService 用于更新问卷的基础信息（仅限领域内部访问字段）
type BaseInfoService struct{}

// UpdateTitle 设置问卷标题（带有合法性校验）
func (BaseInfoService) UpdateTitle(q *Questionnaire, newTitle string) error {
	newTitle = strings.TrimSpace(newTitle)
	if len(newTitle) == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "标题不能为空")
	}
	if len(newTitle) > 100 {
		return errors.WithCode(code.ErrInvalidArgument, "标题长度不能超过 100 字符")
	}
	q.title = newTitle
	return nil
}

// UpdateDescription 设置问卷描述
func (BaseInfoService) UpdateDescription(q *Questionnaire, newDescription string) error {
	if len(newDescription) > 500 {
		return errors.WithCode(code.ErrInvalidArgument, "描述长度不能超过 500 字符")
	}
	q.description = newDescription
	return nil
}

// UpdateCoverImage 设置封面图（必须为 http(s) 地址）
func (BaseInfoService) UpdateCoverImage(q *Questionnaire, imageURL string) error {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return errors.WithCode(code.ErrInvalidArgument, "封面图 URL 不能为空")
	}
	if !(strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://")) {
		return errors.WithCode(code.ErrInvalidArgument, "封面图 URL 必须以 http:// 或 https:// 开头")
	}
	q.imgUrl = imageURL
	return nil
}
