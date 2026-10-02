package aibridge

import (
	"encoding/json"
	"sync"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// QS validates the finite wire contract, not the reference theory or model quality.
// Applicability of reference entries is checked against the frozen input in qs-ai.
var mbtiOutputSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	const name = "mbti-three-topic-output.json"
	var document any
	if err := json.Unmarshal(interpretationschema.AIExplanationOutputV2(), &document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(name, document); err != nil {
		return nil, err
	}
	return compiler.Compile(name)
})

func validateMBTIOutput(content string, validator string) error {
	if validator != "qs-ai-output-mbti-three-topic/v1" {
		return ErrInvalid
	}
	schema, err := mbtiOutputSchema()
	if err != nil {
		return ErrInvalid
	}
	var document any
	if json.Unmarshal([]byte(content), &document) != nil || schema.Validate(document) != nil {
		return ErrInvalid
	}
	return nil
}
