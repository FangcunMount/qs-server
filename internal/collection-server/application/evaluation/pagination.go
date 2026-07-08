package evaluation

// ListPageDefaults 列表分页默认值。
type ListPageDefaults struct {
	Page     int32
	PageSize int32
	MaxSize  int32
}

var AssessmentListPageDefault = ListPageDefaults{Page: 1, PageSize: 10, MaxSize: 100}

// NormalizeListPage 规范化分页参数并返回页码与页大小。
func NormalizeListPage(page, pageSize int32, defaults ListPageDefaults) (int32, int32) {
	if defaults.Page <= 0 {
		defaults.Page = 1
	}
	if defaults.PageSize <= 0 {
		defaults.PageSize = 50
	}
	if defaults.MaxSize <= 0 {
		defaults.MaxSize = 100
	}
	if page <= 0 {
		page = defaults.Page
	}
	if pageSize <= 0 {
		pageSize = defaults.PageSize
	}
	if pageSize > defaults.MaxSize {
		pageSize = defaults.MaxSize
	}
	return page, pageSize
}

// NormalizeAssessmentListRequest 就地规范化测评列表请求分页。
func NormalizeAssessmentListRequest(req *ListAssessmentsRequest, defaults ListPageDefaults) {
	if req == nil {
		return
	}
	req.Page, req.PageSize = NormalizeListPage(req.Page, req.PageSize, defaults)
}
