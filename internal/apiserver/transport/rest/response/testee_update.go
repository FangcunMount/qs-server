package response

// TesteeUpdateResponse acknowledges the committed mutation without implying read access.
type TesteeUpdateResponse struct {
	ID      string `json:"id"`
	Updated bool   `json:"updated"`
}
