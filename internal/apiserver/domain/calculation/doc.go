// Package calculation executes scoring and projection rules against model inputs.
//
// Boundaries:
//   - ModelCatalog owns model structure and rule configuration (factors, norms, policies).
//   - Calculation executes those rules and produces calculation.Result.
//   - Evaluation orchestrates one assessment run and maps calculation results into
//     assessment.AssessmentOutcome via application-layer adapters.
//
// Calculation is a stateless computation kernel: it must not import modelcatalog, factor,
// question, or other domain assets. Callers translate their domain assets into calculation's
// neutral inputs (ScoreNode, ScoreValue) and consume calculation.Result.
package calculation
