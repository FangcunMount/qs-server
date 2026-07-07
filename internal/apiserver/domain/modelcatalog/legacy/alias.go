package legacy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// RuleSetKind 是kept 作为 兼容性 name while callers migrate 到 类型。
type RuleSetKind = identity.Kind

const (
	RuleSetKindScale = identity.KindScale
	RuleSetKindMBTI  = RuleSetKind(KindMBTIMigration)
	RuleSetKindSBTI  = RuleSetKind(KindSBTIMigration)
)

// Definition 是kept 作为 兼容性 name while callers migrate 到 ModelDefinition。
type Definition struct {
	Kind    identity.Kind
	Code    string
	Version string
	Title   string
	Status  string
}

// RuleSetDefinition 是kept 作为 兼容性 name while callers migrate 到 Definition。
type RuleSetDefinition = Definition

// Snapshot 是v1 envelope kept 用于 迁移 读取器。
type Snapshot struct {
	SchemaVersion string
	PayloadFormat string
	Definition    Definition
	Binding       catalog.QuestionnaireBinding
	DecisionKind  identity.DecisionKind
	Source        map[string]any
	Payload       []byte
}

// RuleSetSnapshot 是kept 作为 兼容性 name while callers migrate 到 快照。
type RuleSetSnapshot = Snapshot

const RuleSetSchemaVersionV1 = catalog.SchemaVersionV1
