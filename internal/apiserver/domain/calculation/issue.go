package calculation

// Issue 记录校验发现 on 计算输入s 或 结果s。
type Issue struct {
	Code    string
	Message string
}

// NewIssue 构造校验问题 使用 编码 和 message。
func NewIssue(code, message string) Issue {
	return Issue{Code: code, Message: message}
}
