package evaluation

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
)

var (
	outputSchemaOnce sync.Once
	outputSchemaV1   *jsonschema.Schema
	outputSchemaErr  error
)

func compileContractSchema(name string, raw []byte) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode %s JSON Schema: %w", name, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(name, document); err != nil {
		return nil, fmt.Errorf("register %s JSON Schema: %w", name, err)
	}
	compiled, err := compiler.Compile(name)
	if err != nil {
		return nil, fmt.Errorf("compile %s JSON Schema: %w", name, err)
	}
	return compiled, nil
}

func validateContractInstance(schema *jsonschema.Schema, name string, raw []byte) error {
	if schema == nil {
		return fmt.Errorf("%s JSON Schema is unavailable", name)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("decode %s instance: %w", name, err)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("validate %s instance: %w", name, err)
	}
	return nil
}

func validateOutputContractV1(raw []byte) error {
	outputSchemaOnce.Do(func() {
		outputSchemaV1, outputSchemaErr = compileContractSchema("ai-explanation-output-v1.schema.json", interpretationschema.AIExplanationOutputV1())
	})
	if outputSchemaErr != nil {
		return outputSchemaErr
	}
	return validateContractInstance(outputSchemaV1, "AIExplanationOutput v1", raw)
}
