package catalog

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

const (
	SchemaVersionV1 = "1"
	SchemaVersionV2 = "2"
)

// QuestionnaireBinding binds 已发布模型 到 问卷版本。
type QuestionnaireBinding struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
}

// ModelDefinition 是规范 published-model 元数据。
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

// DecisionSpec 记录结果 判定策略 用于 已发布模型。
type DecisionSpec struct {
	Kind identity.DecisionKind
}

// SourceRef 携带可选 provenance 元数据 用于 已发布快照。
type SourceRef map[string]any

// PublishedModelSnapshot 是v2 published-model envelope。
type PublishedModelSnapshot struct {
	SchemaVersion string
	Model         ModelDefinition
	Binding       QuestionnaireBinding
	Decision      DecisionSpec
	Source        SourceRef
	PayloadFormat string
	Payload       []byte
}
