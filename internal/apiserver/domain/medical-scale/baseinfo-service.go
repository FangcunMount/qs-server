package medicalscale

import (
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// BaseInfoService 用于更新医学量表的基础信息（仅限领域内部访问字段）
type BaseInfoService struct{}

// UpdateTitle 设置医学量表标题
func (BaseInfoService) UpdateTitle(m *MedicalScale, newTitle string) error {
	newTitle = strings.TrimSpace(newTitle)
	if len(newTitle) == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "标题不能为空")
	}
	if len(newTitle) > 100 {
		return errors.WithCode(code.ErrInvalidArgument, "标题长度不能超过 100 字符")
	}
	m.title = newTitle
	return nil
}

// UpdateDescription 设置医学量表描述
func (BaseInfoService) UpdateDescription(m *MedicalScale, newDescription string) error {
	if len(newDescription) > 500 {
		return errors.WithCode(code.ErrInvalidArgument, "描述长度不能超过 500 字符")
	}
	m.description = newDescription
	return nil
}
