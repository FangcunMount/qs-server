package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DescriptorKey 路由评估执行 按 机制, 不 测评编码。
type DescriptorKey struct {
	DecisionKind modelcatalog.DecisionKind
}

// ModelRoute 是运行时路由需要的最小模型身份。
type ModelRoute struct {
	DecisionKind modelcatalog.DecisionKind
}

// HasFrozenRuntime reports publish-time complete RuntimeIdentity on the route.
func (r ModelRoute) HasFrozenRuntime() bool {
	return r.DecisionKind != ""
}

func (k DescriptorKey) IsZero() bool {
	return k.DecisionKind == ""
}

func (k DescriptorKey) String() string {
	return string(k.DecisionKind)
}
