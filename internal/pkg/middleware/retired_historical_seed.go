package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	HistoricalContextHeader     = "X-QS-Historical-Context"
	HistoricalRequestedAtHeader = "X-QS-Historical-Requested-At"
	HistoricalSignatureHeader   = "X-QS-Historical-Signature"
)

var retiredHistoricalSeedHeaders = [...]string{
	HistoricalContextHeader,
	HistoricalRequestedAtHeader,
	HistoricalSignatureHeader,
}

// RejectRetiredHistoricalSeedHeaders permanently rejects the retired
// historical seed protocol. Header presence is sufficient, including an empty
// value; the request body is deliberately left unread.
func RejectRetiredHistoricalSeedHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, header := range retiredHistoricalSeedHeaders {
			if _, present := c.Request.Header[http.CanonicalHeaderKey(header)]; present {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"code":    http.StatusForbidden,
					"message": "historical context rejected",
					"error":   "historical seed context is disabled",
				})
				return
			}
		}
		c.Next()
	}
}
