package process

import (
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	resttransport "github.com/FangcunMount/qs-server/internal/collection-server/transport/rest"
	"github.com/gin-gonic/gin"
)

func generalConcurrencyMiddleware(gate *concurrency.Gate, maxWait time.Duration) gin.HandlerFunc {
	waiting := gate.WaitMiddleware(maxWait, func(c *gin.Context) {
		resttransport.WriteServiceUnavailable(c, 1)
	})
	return func(c *gin.Context) {
		if isWaitReportPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		waiting(c)
	}
}

func isWaitReportPath(path string) bool {
	return strings.HasSuffix(path, "/wait-report")
}
