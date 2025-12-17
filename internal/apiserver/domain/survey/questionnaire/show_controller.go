package questionnaire

import (
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ShowController 显示控制器
// 用于控制问题的显示条件，基于其他问题的答案
type ShowController struct {
	// Rule 逻辑规则：and（所有条件满足）或 or（任一条件满足）
	Rule string `json:"rule"`

	// Questions 条件问题列表
	Questions []ShowControllerCondition `json:"questions"`
}

// ShowControllerCondition 显示控制条件
// 定义某个问题的答案条件
type ShowControllerCondition struct {
	// Code 问题编码
	Code meta.Code `json:"code"`

	// SelectOptionCodes 选中的选项编码列表
	// 对于单选题，数组长度为1；对于多选题，数组长度>=1
	SelectOptionCodes []meta.Code `json:"select_option_codes"`
}

// IsEmpty 判断显示控制器是否为空
func (sc *ShowController) IsEmpty() bool {
	return sc == nil || sc.Rule == "" || len(sc.Questions) == 0
}

// GetRule 获取逻辑规则
func (sc *ShowController) GetRule() string {
	if sc == nil {
		return ""
	}
	return sc.Rule
}

// GetQuestions 获取条件问题列表
func (sc *ShowController) GetQuestions() []ShowControllerCondition {
	if sc == nil {
		return nil
	}
	return sc.Questions
}

// NewShowController 创建显示控制器
func NewShowController(rule string, questions []ShowControllerCondition) *ShowController {
	return &ShowController{
		Rule:      rule,
		Questions: questions,
	}
}

// NewShowControllerCondition 创建显示控制条件
func NewShowControllerCondition(code meta.Code, selectOptionCodes []meta.Code) ShowControllerCondition {
	return ShowControllerCondition{
		Code:              code,
		SelectOptionCodes: selectOptionCodes,
	}
}
