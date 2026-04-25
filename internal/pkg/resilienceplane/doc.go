// Package resilienceplane defines the bounded vocabulary and observability
// contract for qs-server resilience controls.
//
// The package intentionally does not implement rate limiting, queues,
// backpressure, Redis locks, or business idempotency. Those primitives stay in
// their existing packages. This package only provides shared decision labels so
// operators can reason about entry protection, dependency protection, duplicate
// suppression, and degraded-open paths with one vocabulary.
package resilienceplane
