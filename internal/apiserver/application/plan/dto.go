package plan

// ============= DTO 定义 =============
// DTOs 用于应用服务层的输入参数

// CreatePlanDTO 创建计划 DTO
type CreatePlanDTO struct {
	OrgID         int64    // 机构ID
	ScaleCode     string   // 量表编码
	ScheduleType  string   // 周期类型：by_week, by_day, custom, fixed_date
	Interval      int      // 间隔（用于 by_week/by_day）
	TotalTimes    int      // 总次数
	FixedDates    []string // 固定日期列表（用于 fixed_date，格式：YYYY-MM-DD）
	RelativeWeeks []int    // 相对周次列表（用于 custom，如 [2,4,8,12,18]）
}

// EnrollTesteeDTO 受试者加入计划 DTO
type EnrollTesteeDTO struct {
	PlanID    string // 计划ID
	TesteeID  string // 受试者ID
	StartDate string // 开始日期（格式：YYYY-MM-DD）
}

// OpenTaskDTO 开放任务 DTO
type OpenTaskDTO struct {
	EntryToken string // 入口令牌
	EntryURL   string // 入口URL
	ExpireAt   string // 过期时间（格式：YYYY-MM-DD HH:mm:ss）
}

// ListPlansDTO 查询计划列表 DTO
type ListPlansDTO struct {
	OrgID     int64  // 机构ID（可选）
	ScaleCode string // 量表编码（可选）
	Status    string // 状态（可选）
	Page      int    // 页码（从1开始）
	PageSize  int    // 每页数量
}

// ListTasksDTO 查询任务列表 DTO
type ListTasksDTO struct {
	PlanID   string // 计划ID（可选）
	TesteeID string // 受试者ID（可选）
	Status   string // 状态（可选）
	Page     int    // 页码（从1开始）
	PageSize int    // 每页数量
}
