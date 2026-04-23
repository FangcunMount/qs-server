package container

import codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"

// initCodesService 初始化 CodesService。
func (c *Container) initCodesService() {
	// 如果已经有实现则不覆盖。
	if c == nil || c.CodesService != nil {
		return
	}
	c.CodesService = codesapp.NewService()
	c.printf("🔑 CodesService initialized\n")
}
