package plan

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ValidationError 验证错误
type ValidationError struct {
	Field   string // 字段名
	Message string // 错误信息
}

// Error 实现 error 接口
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// PlanValidator 计划验证器领域服务
// 负责领域层的验证：创建时验证、加入时验证、周期策略验证等
type PlanValidator struct{}

// NewPlanValidator 创建计划验证器
func NewPlanValidator() *PlanValidator {
	return &PlanValidator{}
}

// ValidateForCreation 验证计划创建参数
func (v *PlanValidator) ValidateForCreation(
	orgID int64,
	scaleCode string,
	scheduleType PlanScheduleType,
	interval int,
	totalTimes int,
	fixedDates []time.Time,
	relativeWeeks []int,
) []ValidationError {
	var errs []ValidationError

	if orgID <= 0 {
		errs = append(errs, ValidationError{Field: "orgID", Message: "机构ID不能为空"})
	}
	if scaleCode == "" {
		errs = append(errs, ValidationError{Field: "scaleCode", Message: "量表编码不能为空"})
	}
	if !scheduleType.IsValid() {
		errs = append(errs, ValidationError{Field: "scheduleType", Message: "无效的周期类型"})
	}

	// 验证周期策略的具体参数
	errs = append(errs, v.ValidateScheduleStrategy(scheduleType, interval, totalTimes, fixedDates, relativeWeeks)...)

	return errs
}

// ValidateForEnrollment 验证计划是否可以加入
func (v *PlanValidator) ValidateForEnrollment(
	plan *AssessmentPlan,
	testeeID testee.ID,
	startDate time.Time,
) []ValidationError {
	var errs []ValidationError

	if plan == nil {
		errs = append(errs, ValidationError{Field: "plan", Message: "计划不能为空"})
		return errs
	}
	if !plan.IsActive() {
		errs = append(errs, ValidationError{Field: "status", Message: "计划未处于活跃状态，无法加入"})
	}
	if testeeID.IsZero() {
		errs = append(errs, ValidationError{Field: "testeeID", Message: "受试者ID不能为空"})
	}
	if startDate.IsZero() {
		errs = append(errs, ValidationError{Field: "startDate", Message: "开始日期不能为空"})
	}

	// TODO: 检查该受试者是否已加入此计划（幂等性检查，可能需要查询 TaskRepository）

	return errs
}

// ValidateScheduleStrategy 验证周期策略的有效性
func (v *PlanValidator) ValidateScheduleStrategy(
	scheduleType PlanScheduleType,
	interval int,
	totalTimes int,
	fixedDates []time.Time,
	relativeWeeks []int,
) []ValidationError {
	var errs []ValidationError

	switch scheduleType {
	case PlanScheduleByWeek, PlanScheduleByDay:
		if interval <= 0 {
			errs = append(errs, ValidationError{Field: "interval", Message: "间隔时间必须大于0"})
		}
		if totalTimes <= 0 {
			errs = append(errs, ValidationError{Field: "totalTimes", Message: "总次数必须大于0"})
		}
		if totalTimes > 100 {
			errs = append(errs, ValidationError{Field: "totalTimes", Message: "总次数不能超过100次"})
		}
	case PlanScheduleCustom:
		if len(relativeWeeks) == 0 {
			errs = append(errs, ValidationError{Field: "relativeWeeks", Message: "相对周次列表不能为空"})
		} else {
			// 验证周次必须递增且大于0
			for i, week := range relativeWeeks {
				if week <= 0 {
					errs = append(errs, ValidationError{Field: fmt.Sprintf("relativeWeeks[%d]", i), Message: "周次必须大于0"})
				}
				if i > 0 && week <= relativeWeeks[i-1] {
					errs = append(errs, ValidationError{Field: fmt.Sprintf("relativeWeeks[%d]", i), Message: "周次必须按顺序递增"})
				}
			}
		}
	case PlanScheduleFixedDate:
		if len(fixedDates) == 0 {
			errs = append(errs, ValidationError{Field: "fixedDates", Message: "固定日期列表不能为空"})
		} else {
			// 验证日期必须递增
			for i := 1; i < len(fixedDates); i++ {
				if fixedDates[i].Before(fixedDates[i-1]) {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("fixedDates[%d]", i),
						Message: "日期必须按时间顺序递增",
					})
				}
			}
		}
	default:
		errs = append(errs, ValidationError{
			Field:   "scheduleType",
			Message: fmt.Sprintf("无效的周期类型: %s", scheduleType),
		})
	}
	return errs
}

// ToError 将验证错误列表转换为单个 error
func ToError(errs []ValidationError) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		firstErr := errs[0]
		return errors.WithCode(code.ErrInvalidArgument, "%s: %s", firstErr.Field, firstErr.Message)
	}
	var messages []string
	for _, err := range errs {
		messages = append(messages, fmt.Sprintf("%s: %s", err.Field, err.Message))
	}
	return errors.WithCode(code.ErrInvalidArgument, "验证失败：%s", strings.Join(messages, "; "))
}
