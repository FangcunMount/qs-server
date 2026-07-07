package modelcatalog

// RuleSetKind is kept as a compatibility name while callers migrate to Kind.
type RuleSetKind = Kind

const (
	RuleSetKindScale = KindScale
	RuleSetKindMBTI  = KindMBTIMigration
	RuleSetKindSBTI  = KindSBTIMigration
)

// Definition is kept as a compatibility name while callers migrate to ModelDefinition.
type Definition struct {
	Kind    Kind
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
	Binding       QuestionnaireBinding
	DecisionKind  DecisionKind
	Source        map[string]any
	Payload       []byte
}

// RuleSetSnapshot is kept as a compatibility name while callers migrate to Snapshot.
type RuleSetSnapshot = Snapshot

const RuleSetSchemaVersionV1 = SchemaVersionV1
