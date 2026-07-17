// Package ratelimit models HTTP entry rate limit decisions.
//
// The package is transport-neutral: it owns policy, decision, and limiter
// adapters, while Gin middleware translates decisions into HTTP behavior.
package ratelimit
