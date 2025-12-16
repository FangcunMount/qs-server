package request

// ApplyCodeRequest 请求参数
type ApplyCodeRequest struct {
	Kind     string                 `json:"kind" valid:"required~kind 不能为空"`
	Count    int                    `json:"count"`
	Prefix   string                 `json:"prefix"`
	Metadata map[string]interface{} `json:"metadata"`
}
