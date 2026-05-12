// Package result owns the post-execution write phase for Evaluation outcomes.
//
// The writer deliberately preserves the legacy durable side-effect order:
// score projection, Assessment interpreted save, report durable save with
// outbox staging, then waiter notification. Callers must treat this package as
// an application consistency boundary, not as model-specific scoring logic.
package result
