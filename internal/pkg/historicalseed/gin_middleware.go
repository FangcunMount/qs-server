package historicalseed

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const MaxSignedBodyBytes int64 = 8 << 20

// GinMiddleware verifies the optional signed history envelope and projects it
// into the request context without changing any process-wide clock.
func GinMiddleware(verifier *Verifier) gin.HandlerFunc {
	if verifier == nil {
		verifier = &Verifier{}
	}
	return func(c *gin.Context) {
		headers := Headers{
			EncodedContext: c.GetHeader(HeaderContext),
			RequestedAt:    c.GetHeader(HeaderRequestedAt),
			Signature:      c.GetHeader(HeaderSignature),
		}
		if !hasHistoricalHeaders(headers) {
			c.Next()
			return
		}
		if !hasCompleteHistoricalHeaders(headers) {
			abortGin(c, ErrIncompleteHeaders)
			return
		}
		if c.Request.ContentLength > MaxSignedBodyBytes {
			abortGinBodyTooLarge(c)
			return
		}
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, MaxSignedBodyBytes+1))
		if err != nil {
			abortGin(c, fmt.Errorf("read request body: %w", err))
			return
		}
		if int64(len(body)) > MaxSignedBodyBytes {
			abortGinBodyTooLarge(c)
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		historical, present, err := verifier.Verify(c.Request.Method, c.Request.URL.RequestURI(), body, headers)
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

func hasHistoricalHeaders(headers Headers) bool {
	return strings.TrimSpace(headers.EncodedContext) != "" || strings.TrimSpace(headers.RequestedAt) != "" || strings.TrimSpace(headers.Signature) != ""
}

func hasCompleteHistoricalHeaders(headers Headers) bool {
	return strings.TrimSpace(headers.EncodedContext) != "" && strings.TrimSpace(headers.RequestedAt) != "" && strings.TrimSpace(headers.Signature) != ""
}

func abortGin(c *gin.Context, err error) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "historical context rejected", "error": err.Error()})
}

func abortGinBodyTooLarge(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"code": http.StatusRequestEntityTooLarge, "message": "historical context rejected", "error": "historical signed request body exceeds 8 MiB"})
}
