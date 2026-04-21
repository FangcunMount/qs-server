package redisplane

import (
	"sort"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

// Family identifies a logical Redis workload inside qs-server.
type Family string

const (
	FamilyDefault Family = "default"
	FamilyStatic  Family = "static_meta"
	FamilyObject  Family = "object_view"
	FamilyQuery   Family = "query_result"
	FamilyMeta    Family = "meta_hotset"
	FamilySDK     Family = "sdk_token"
	FamilyLock    Family = "lock_lease"
	FamilyOps     Family = "ops_runtime"
)

// Route defines runtime routing for one logical family.
type Route struct {
	RedisProfile         string
	NamespaceSuffix      string
	AllowFallbackDefault bool
	AllowWarmup          bool
}

// Catalog stores runtime family routing.
type Catalog struct {
	root   string
	routes map[Family]Route
}

// NewCatalog creates a catalog with default fallback route plus explicit family routes.
func NewCatalog(rootNamespace string, routes map[Family]Route) *Catalog {
	catalog := &Catalog{
		root:   rootNamespace,
		routes: map[Family]Route{},
	}
	catalog.routes[FamilyDefault] = Route{
		AllowFallbackDefault: true,
	}
	for family, route := range routes {
		catalog.routes[family] = route
	}
	return catalog
}

// Route returns the configured route, falling back to the default route for unknown families.
func (c *Catalog) Route(family Family) Route {
	if c == nil {
		return Route{AllowFallbackDefault: true}
	}
	if route, ok := c.routes[family]; ok {
		return route
	}
	return c.routes[FamilyDefault]
}

// Namespace returns the namespaced keyspace root for one family.
func (c *Catalog) Namespace(family Family) string {
	if c == nil {
		return ""
	}
	return rediskey.ComposeNamespace(c.root, c.Route(family).NamespaceSuffix)
}

// Builder returns a family-scoped key builder.
func (c *Catalog) Builder(family Family) *rediskey.Builder {
	return rediskey.NewBuilderWithNamespace(c.Namespace(family))
}

// Families returns all explicitly configured families in a stable order.
func (c *Catalog) Families() []Family {
	if c == nil {
		return nil
	}
	families := make([]Family, 0, len(c.routes))
	for family := range c.routes {
		if family == FamilyDefault {
			continue
		}
		families = append(families, family)
	}
	sort.Slice(families, func(i, j int) bool {
		return families[i] < families[j]
	})
	return families
}
