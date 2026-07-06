// Package result is a deprecated compatibility facade for evaluation write paths.
//
// Deprecated: new code should import:
//   - github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting for report writers/builders
//   - github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome for Outcome
//
// This package keeps thin aliases and legacy projection helpers during migration.
// Evaluation-only concerns that remain here:
//   - deprecated aliases to evaluation/scoring snapshot store
//   - deprecated aliases delegating legacy projection to evaluation/outcome
package result
