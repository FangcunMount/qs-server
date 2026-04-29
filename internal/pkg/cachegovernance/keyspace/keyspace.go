package keyspace

import basekeyspace "github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"

// Keyspace builds cache governance keys.
type Keyspace struct {
	base basekeyspace.GovernanceKeyspace
}

func New(ns string) Keyspace {
	return Keyspace{base: basekeyspace.NewGovernanceKeyspace(ns)}
}

func FromBuilder(builder *basekeyspace.Builder) Keyspace {
	if builder == nil {
		return New("")
	}
	return New(builder.Namespace())
}

func (k Keyspace) WarmupHotset(family, kind string) string {
	return k.base.WarmupHotset(family, kind)
}
