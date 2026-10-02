// Package interpretationschema preserves the shared AI output contract.
package interpretationschema

import _ "embed"

//go:embed ai-explanation-output-v1.schema.json
var aiExplanationOutputV1 []byte

//go:embed ai-explanation-output-v2.schema.json
var aiExplanationOutputV2 []byte

func AIExplanationOutputV1() []byte {
	return append([]byte(nil), aiExplanationOutputV1...)
}

func AIExplanationOutputV2() []byte {
	return append([]byte(nil), aiExplanationOutputV2...)
}
