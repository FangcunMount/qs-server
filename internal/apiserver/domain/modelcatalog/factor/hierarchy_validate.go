package factor

// HierarchyIssue 记录一个因子 层级 校验 problem。
type HierarchyIssue struct {
	Field   string
	Code    string
	Message string
}
