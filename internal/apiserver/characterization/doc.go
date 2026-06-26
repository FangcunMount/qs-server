// Package characterization hosts v1 baseline tests for Assessment Model v2 migration.
//
// These tests lock observable behavior before refactoring:
//   - scale total score / risk / dimensions / suggestions
//   - MBTI type code / match percent / profile suggestions
//   - SBTI outcome / similarity / rarity projection
//   - ruleset codec payload formats and round-trip
//   - InterpretReport Mongo mapper preservation
//   - execute service EvaluatorKey dispatch for scale/MBTI/SBTI
//   - typology executor legacy payload scoring parity
//   - legacy kind -> EvaluatorKey mapping
//   - report v2 model/primary_score/level projection and Mongo dual-write
//
// Worker high-risk handling baseline lives in internal/worker/handlers/report_risk_v1_characterization_test.go.
package characterization
