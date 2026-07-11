// Package run models one durable Evaluation execution attempt.
//
// Assessment owns the business lifecycle. EvaluationRun owns attempt state,
// exclusive worker claim/lease, failure classification and audit references.
package run
