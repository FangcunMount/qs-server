package schema

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

func TestCatalogReturnsCanonicalEmbeddedOutputSchema(t *testing.T) {
	value, err := NewCatalog().ResolveOutputSchema(context.Background(), aiexplanation.OutputSchemaVersionV1)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(value.JSON, &document); err != nil {
		t.Fatal(err)
	}
	if document["additionalProperties"] != false || value.Name == "" {
		t.Fatalf("schema header = %#v", document)
	}

	value.JSON[0] = 'x'
	again, err := NewCatalog().ResolveOutputSchema(context.Background(), aiexplanation.OutputSchemaVersionV1)
	if err != nil || again.JSON[0] == 'x' {
		t.Fatal("embedded schema bytes were mutable across resolutions")
	}
}

func TestCatalogRejectsUnknownOutputSchema(t *testing.T) {
	_, err := NewCatalog().ResolveOutputSchema(context.Background(), "ai-explanation-output/v2")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}
