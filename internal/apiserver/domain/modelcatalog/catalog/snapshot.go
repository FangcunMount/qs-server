package catalog

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

const (
	SchemaVersionV1 = "1"
	SchemaVersionV2 = "2"
)

// QuestionnaireBinding binds a published model to a questionnaire version.
type QuestionnaireBinding struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
}

// ModelDefinition is canonical published-model metadata.
type ModelDefinition struct {
	ProductChannel identity.ProductChannel
	Kind           identity.Kind
	SubKind        identity.SubKind
	Algorithm      identity.Algorithm
	Code           string
	Version        string
	Title          string
	Status         string
}

// DecisionSpec captures the outcome decision strategy for a published model.
type DecisionSpec struct {
	Kind identity.DecisionKind
}

// SourceRef carries optional provenance metadata for a published snapshot.
type SourceRef map[string]any

// PublishedModelSnapshot is the v2 published-model envelope.
type PublishedModelSnapshot struct {
	SchemaVersion string
	Model         ModelDefinition
	Binding       QuestionnaireBinding
	Decision      DecisionSpec
	Source        SourceRef
	PayloadFormat string
	Payload       []byte
}
