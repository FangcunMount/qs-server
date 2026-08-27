package schema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

var ErrNotFound = errors.New("AI explanation output schema not found")

type Catalog struct{}

func NewCatalog() *Catalog { return &Catalog{} }

func (*Catalog) ResolveOutputSchema(_ context.Context, version string) (appport.StructuredOutputSchema, error) {
	if version != aiexplanation.OutputSchemaVersionV1 {
		return appport.StructuredOutputSchema{}, ErrNotFound
	}
	raw := interpretationschema.AIExplanationOutputV1()
	var header struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return appport.StructuredOutputSchema{}, fmt.Errorf("decode embedded AI explanation output schema: %w", err)
	}
	value := appport.StructuredOutputSchema{
		Version: version, Name: header.Title, JSON: raw, Fingerprint: aiexplanation.NewFingerprint(raw),
	}
	if err := value.Validate(); err != nil {
		return appport.StructuredOutputSchema{}, err
	}
	return value, nil
}
