package middleware

import (
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"github.com/gin-gonic/gin"
)

// HistoricalSeedMiddleware verifies the optional signed history envelope and
// projects it into the standard request context. It never changes a clock.
func HistoricalSeedMiddleware(verifier *historicalseed.Verifier) gin.HandlerFunc {
	return historicalseed.GinMiddleware(verifier)
}
