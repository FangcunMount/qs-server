// Package redislock provides the shared Redis lease primitive used by qs-server.
//
// This package is intentionally not a business-level idempotency or scheduling
// framework. It only owns lock specifications, lock key construction through a
// redisplane handle, token-based acquire/release, and lock/family observability.
//
// Callers keep their own domain semantics:
//   - apiserver schedulers use leader locks and skip work on contention.
//   - collection-server submit guard combines an in-flight lock with a done
//     marker to protect idempotent submission.
//   - worker event handlers use best-effort duplicate suppression and continue
//     processing when the lock path is degraded.
//
// Lease TTL is not renewed by this package. Long-running callers must choose a
// TTL that covers their critical section or introduce a separate renewal design.
// Releasing a lease is token-based; releasing with the wrong token must not
// unlock another owner.
package redisadapter
