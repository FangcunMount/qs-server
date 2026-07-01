package ratelimit

import (
	"net/http"
	"strconv"
)

// ApplyRetryAfterHeader sets Retry-After from a transport-neutral rate limit decision.
func ApplyRetryAfterHeader(header http.Header, decision RateLimitDecision) {
	seconds := decision.RetryAfterSeconds
	if seconds < 1 {
		seconds = 1
	}
	header.Set("Retry-After", strconv.Itoa(seconds))
}

// ApplyRetryAfterSeconds sets Retry-After when only seconds are known (e.g. poll backoff).
func ApplyRetryAfterSeconds(header http.Header, seconds int) {
	if seconds < 1 {
		seconds = 1
	}
	header.Set("Retry-After", strconv.Itoa(seconds))
}
