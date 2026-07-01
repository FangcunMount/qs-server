package process

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/gin-gonic/gin"
)

func generalConcurrencyMiddleware(gate *concurrency.Gate) gin.HandlerFunc {
	blocking := gate.BlockingMiddleware()
	return func(c *gin.Context) {
		if isWaitReportPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		blocking(c)
	}
}

func isWaitReportPath(path string) bool {
	return strings.HasSuffix(path, "/wait-report")
}
