package historicalseed

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GinMiddleware verifies the optional signed history envelope and projects it
// into the request context without changing any process-wide clock.
func GinMiddleware(verifier *Verifier) gin.HandlerFunc {
	if verifier == nil {
		verifier = &Verifier{}
	}
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			abortGin(c, fmt.Errorf("read request body: %w", err))
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		historical, present, err := verifier.Verify(c.Request.Method, c.Request.URL.RequestURI(), body, Headers{
			EncodedContext: c.GetHeader(HeaderContext), RequestedAt: c.GetHeader(HeaderRequestedAt), Signature: c.GetHeader(HeaderSignature),
		})
		if err != nil {
			abortGin(c, err)
			return
		}
		if present {
			c.Request = c.Request.WithContext(WithContext(c.Request.Context(), historical))
		}
		c.Next()
	}
}

func abortGin(c *gin.Context, err error) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "historical context rejected", "error": err.Error()})
}
