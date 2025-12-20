package request

// ============= Plan Lifecycle Requests =============

// CreatePlanRequest 创建计划请求
type CreatePlanRequest struct {
	ScaleID       string   `json:"scale_id" valid:"required~量表ID不能为空"`
	ScheduleType  string   `json:"schedule_type" valid:"required~周期类型不能为空"`
	Interval      int      `json:"interval" valid:"required~间隔不能为空"`
	TotalTimes    int      `json:"total_times" valid:"required~总次数不能为空"`
	FixedDates    []string `json:"fixed_dates,omitempty"`    // 固定日期列表（格式：YYYY-MM-DD）
	RelativeWeeks []int    `json:"relative_weeks,omitempty"` // 相对周次列表
}

// PausePlanRequest 暂停计划请求（无请求体，使用路径参数）
// ResumePlanRequest 恢复计划请求
type ResumePlanRequest struct {
	TesteeStartDates map[string]string `json:"testee_start_dates,omitempty"` // 受试者ID -> 开始日期（格式：YYYY-MM-DD）
}

// ============= Plan Enrollment Requests =============

// EnrollTesteeRequest 受试者加入计划请求
type EnrollTesteeRequest struct {
	PlanID    string `json:"plan_id" valid:"required~计划ID不能为空"`
	TesteeID  string `json:"testee_id" valid:"required~受试者ID不能为空"`
	StartDate string `json:"start_date" valid:"required~开始日期不能为空"` // 格式：YYYY-MM-DD
}

// ============= Task Management Requests =============

// OpenTaskRequest 开放任务请求
type OpenTaskRequest struct {
	EntryToken string `json:"entry_token" valid:"required~入口令牌不能为空"`
	EntryURL   string `json:"entry_url" valid:"required~入口URL不能为空"`
	ExpireAt   string `json:"expire_at" valid:"required~过期时间不能为空"` // 格式：YYYY-MM-DD HH:mm:ss
}

// ============= Query Requests =============

// ListPlansRequest 查询计划列表请求
type ListPlansRequest struct {
	OrgID    int64  `form:"org_id"`
	ScaleID  string `form:"scale_id"`
	Status   string `form:"status"`
	Page     int    `form:"page" valid:"required~页码不能为空"`
	PageSize int    `form:"page_size" valid:"required~每页数量不能为空"`
}

// ListTasksRequest 查询任务列表请求
type ListTasksRequest struct {
	PlanID   string `form:"plan_id"`
	TesteeID string `form:"testee_id"`
	Status   string `form:"status"`
	Page     int    `form:"page" valid:"required~页码不能为空"`
	PageSize int    `form:"page_size" valid:"required~每页数量不能为空"`
}
