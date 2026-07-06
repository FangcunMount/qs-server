// Package characterization hosts v1 baseline tests for Assessment Model v2 migration.
//
// These tests lock observable behavior before refactoring:
//
//   - scale total score / risk / dimensions / suggestions
//
//   - MBTI type code / match percent / profile suggestions
//
//   - SBTI outcome / similarity / rarity projection
//
//   - ruleset codec payload formats and round-trip
//
//   - InterpretReport Mongo mapper preservation
//
//   - cross-module answersheet.submitted → worker create/submit → evaluate/report (scale sync + async)
//
//   - cross-module survey submit → worker handler → split-phase execute/report (scale sync + async, MBTI sync)
//
//   - typology executor legacy payload scoring parity
//
//   - legacy kind -> EvaluatorKey mapping
//
//   - report v2 model/primary_score/level projection and Mongo dual-write
//
// Worker high-risk handling baseline lives in internal/worker/handlers/report_risk_v1_characterization_test.go.
package characterization
