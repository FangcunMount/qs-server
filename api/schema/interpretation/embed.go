// Package interpretationschema embeds canonical AI explanation machine
// contracts for runtime adapters. The JSON files remain the single source.
package interpretationschema

import _ "embed"

//go:embed ai-explanation-input-v1.schema.json
var aiExplanationInputV1 []byte

//go:embed ai-explanation-output-v1.schema.json
var aiExplanationOutputV1 []byte

//go:embed ai-explanation-profile-v1.schema.json
var aiExplanationProfileV1 []byte

//go:embed ai-explanation-prompt-evaluation-cases-v1.json
var aiExplanationPromptEvaluationCasesV1 []byte

//go:embed ai-explanation-semantic-evaluation-output-v1.schema.json
var aiExplanationSemanticEvaluationOutputV1 []byte

func AIExplanationInputV1() []byte {
	return append([]byte(nil), aiExplanationInputV1...)
}

func AIExplanationOutputV1() []byte {
	return append([]byte(nil), aiExplanationOutputV1...)
}

func AIExplanationProfileV1() []byte {
	return append([]byte(nil), aiExplanationProfileV1...)
}

// AIExplanationPromptEvaluationCasesV1 returns an isolated copy of the
// canonical v1 evaluation suite. Runtime evaluation tooling must consume this
// embedded machine contract rather than resolving a mutable working-directory
// path.
func AIExplanationPromptEvaluationCasesV1() []byte {
	return append([]byte(nil), aiExplanationPromptEvaluationCasesV1...)
}

func AIExplanationSemanticEvaluationOutputV1() []byte {
	return append([]byte(nil), aiExplanationSemanticEvaluationOutputV1...)
}
