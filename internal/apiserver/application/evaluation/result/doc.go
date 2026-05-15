// Package result owns the post-execution write phase for Evaluation outcomes.
//
// The writer persists report durably (with outbox staging) before score
// projection and Assessment interpreted save, then waiter notification.
// Cross-store compensation is not handled here. Callers must treat this
// package as an application consistency boundary, not model-specific scoring logic.
package result
