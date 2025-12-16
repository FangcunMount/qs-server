package response

// ApplyCodeResponse 响应结构
type ApplyCodeResponse struct {
	Codes []string `json:"codes"`
	Count int      `json:"count"`
}
