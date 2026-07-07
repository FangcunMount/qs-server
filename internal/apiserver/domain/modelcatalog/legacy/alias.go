package legacy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// RuleSetKind is kept as a compatibility name while callers migrate to Kind.
type RuleSetKind = identity.Kind

const (
	RuleSetKindScale = identity.KindScale
	RuleSetKindMBTI  = identity.KindMBTIMigration
	RuleSetKindSBTI  = identity.KindSBTIMigration
)

// Definition is kept as a compatibility name while callers migrate to ModelDefinition.
type Definition struct {
	Kind    identity.Kind
	Code    string
	Version string
	Title   string
	Status  string
}

// RuleSetDefinition is kept as a compatibility name while callers migrate to Definition.
type RuleSetDefinition = Definition

// Snapshot is the v1 envelope kept for migration readers.
type Snapshot struct {
	SchemaVersion string
	PayloadFormat string
	Definition    Definition
	Binding       catalog.QuestionnaireBinding
	DecisionKind  identity.DecisionKind
	Source        map[string]any
	Payload       []byte
}

// RuleSetSnapshot is kept as a compatibility name while callers migrate to Snapshot.
type RuleSetSnapshot = Snapshot

const RuleSetSchemaVersionV1 = catalog.SchemaVersionV1
