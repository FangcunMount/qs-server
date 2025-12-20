package plan

import (
	"errors"
)

// ==================== 领域错误定义 ====================

var (
	// ErrPlanNotFound 计划不存在
	ErrPlanNotFound = errors.New("plan not found")

	// ErrTaskNotFound 任务不存在
	ErrTaskNotFound = errors.New("task not found")

	// ErrPlanNotActive 计划不是活跃状态
	ErrPlanNotActive = errors.New("plan is not active")

	// ErrPlanNotPaused 计划不是暂停状态
	ErrPlanNotPaused = errors.New("plan is not paused")

	// ErrPlanTerminal 计划已处于终态
	ErrPlanTerminal = errors.New("plan is in terminal state")

	// ErrTaskNotPending 任务不是待推送状态
	ErrTaskNotPending = errors.New("task is not pending")

	// ErrTaskNotOpened 任务不是已推送状态
	ErrTaskNotOpened = errors.New("task is not opened")

	// ErrTaskTerminal 任务已处于终态
	ErrTaskTerminal = errors.New("task is in terminal state")

	// ErrInvalidScheduleType 无效的周期类型
	ErrInvalidScheduleType = errors.New("invalid schedule type")

	// ErrInvalidInterval 无效的间隔值
	ErrInvalidInterval = errors.New("invalid interval")

	// ErrInvalidTotalTimes 无效的总次数
	ErrInvalidTotalTimes = errors.New("invalid total times")

	// ErrInvalidFixedDates 无效的固定日期列表
	ErrInvalidFixedDates = errors.New("invalid fixed dates")

	// ErrInvalidCustomWeeks 无效的自定义周次列表
	ErrInvalidCustomWeeks = errors.New("invalid custom weeks")
)
