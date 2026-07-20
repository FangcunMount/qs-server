package evaluation

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DescriptorKey 路由评估执行 按 机制, 不 测评编码。
type DescriptorKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	PayloadFormat   string
}

// ModelRoute 是运行时路由需要的最小模型身份。
type ModelRoute struct {
	Kind            modelcatalog.Kind
	SubKind         modelcatalog.SubKind
	Algorithm       modelcatalog.Algorithm
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	PayloadFormat   string
}

// HasFrozenRuntime reports publish-time complete RuntimeIdentity on the route.
func (r ModelRoute) HasFrozenRuntime() bool {
	return r.AlgorithmFamily != "" && r.DecisionKind != "" && r.PayloadFormat != ""
}

func (k DescriptorKey) IsZero() bool {
	return k.AlgorithmFamily == ""
}

func (k DescriptorKey) String() string {
	parts := []string{k.AlgorithmFamily.String()}
	if k.DecisionKind != "" {
		parts = append(parts, string(k.DecisionKind))
	}
	if k.PayloadFormat != "" {
		parts = append(parts, k.PayloadFormat)
	}
	return strings.Join(parts, "/")
}
