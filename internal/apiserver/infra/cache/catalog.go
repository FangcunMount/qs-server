package cache

import (
	rediskit "github.com/FangcunMount/component-base/pkg/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

// CacheFamily 定义统一缓存家族。
type CacheFamily string

const (
	CacheFamilyDefault CacheFamily = "default"
	CacheFamilyStatic  CacheFamily = "static_meta"
	CacheFamilyObject  CacheFamily = "object_view"
	CacheFamilyQuery   CacheFamily = "query_result"
	CacheFamilyMeta    CacheFamily = "meta_hotset"
	CacheFamilySDK     CacheFamily = "sdk_token"
	CacheFamilyLock    CacheFamily = "lock_lease"
)

// CatalogRoute 定义缓存家族路由。
type CatalogRoute struct {
	RedisProfile    string
	NamespaceSuffix string
	AllowWarmup     bool
}

// CacheCatalog 统一管理 family -> profile/keyspace/builder 路由。
type CacheCatalog struct {
	root         rediskit.Keyspace
	routes       map[CacheFamily]CatalogRoute
	familyPolicy map[CacheFamily]CachePolicy
	policy       map[CachePolicyKey]CachePolicy
}

// NewCacheCatalog 创建缓存目录。
func NewCacheCatalog(rootNamespace string, routes map[CacheFamily]CatalogRoute) *CacheCatalog {
	return NewCacheCatalogWithPolicies(rootNamespace, routes, nil, nil)
}

// NewCacheCatalogWithPolicies 创建带 family/object 策略的缓存目录。
func NewCacheCatalogWithPolicies(rootNamespace string, routes map[CacheFamily]CatalogRoute, familyPolicies map[CacheFamily]CachePolicy, policies map[CachePolicyKey]CachePolicy) *CacheCatalog {
	catalog := &CacheCatalog{
		root:         rediskit.NewKeyspace(rootNamespace),
		routes:       make(map[CacheFamily]CatalogRoute),
		familyPolicy: make(map[CacheFamily]CachePolicy),
		policy:       make(map[CachePolicyKey]CachePolicy),
	}
	for family, route := range defaultCatalogRoutes() {
		catalog.routes[family] = route
	}
	for family, route := range routes {
		defaultRoute := catalog.routes[family]
		if route.RedisProfile == "" {
			route.RedisProfile = defaultRoute.RedisProfile
		}
		if route.NamespaceSuffix == "" {
			route.NamespaceSuffix = defaultRoute.NamespaceSuffix
		}
		catalog.routes[family] = route
	}
	for family, policy := range familyPolicies {
		catalog.familyPolicy[family] = policy
	}
	for key, policy := range policies {
		catalog.policy[key] = policy
	}
	return catalog
}

func defaultCatalogRoutes() map[CacheFamily]CatalogRoute {
	return map[CacheFamily]CatalogRoute{
		CacheFamilyDefault: {
			RedisProfile:    "",
			NamespaceSuffix: "",
			AllowWarmup:     false,
		},
		CacheFamilyStatic: {
			RedisProfile:    "static_cache",
			NamespaceSuffix: "cache:static",
			AllowWarmup:     true,
		},
		CacheFamilyObject: {
			RedisProfile:    "object_cache",
			NamespaceSuffix: "cache:object",
			AllowWarmup:     false,
		},
		CacheFamilyQuery: {
			RedisProfile:    "query_cache",
			NamespaceSuffix: "cache:query",
			AllowWarmup:     true,
		},
		CacheFamilyMeta: {
			RedisProfile:    "meta_cache",
			NamespaceSuffix: "cache:meta",
			AllowWarmup:     false,
		},
		CacheFamilySDK: {
			RedisProfile:    "sdk_cache",
			NamespaceSuffix: "cache:sdk",
			AllowWarmup:     false,
		},
		CacheFamilyLock: {
			RedisProfile:    "lock_cache",
			NamespaceSuffix: "cache:lock",
			AllowWarmup:     false,
		},
	}
}

// Route 返回 family 配置；未知 family 回退到 default。
func (c *CacheCatalog) Route(family CacheFamily) CatalogRoute {
	if c == nil {
		return defaultCatalogRoutes()[CacheFamilyDefault]
	}
	if route, ok := c.routes[family]; ok {
		return route
	}
	return c.routes[CacheFamilyDefault]
}

// Namespace 返回 family 对应 keyspace namespace。
func (c *CacheCatalog) Namespace(family CacheFamily) string {
	if c == nil {
		return ""
	}
	return rediskey.ComposeNamespace(string(c.root.Namespace()), c.Route(family).NamespaceSuffix)
}

// Builder 返回 family 对应 redis key builder。
func (c *CacheCatalog) Builder(family CacheFamily) *rediskey.Builder {
	return rediskey.NewBuilderWithNamespace(c.Namespace(family))
}

// AllowsWarmup 返回 family 是否允许启动预热。
func (c *CacheCatalog) AllowsWarmup(family CacheFamily) bool {
	return c.Route(family).AllowWarmup
}

// Policy 返回对象级缓存策略；未配置时返回零值策略。
func (c *CacheCatalog) Policy(key CachePolicyKey) CachePolicy {
	if c == nil {
		return CachePolicy{}
	}
	return c.policy[key].MergeWith(c.familyPolicy[PolicyFamily(key)])
}

func PolicyFamily(key CachePolicyKey) CacheFamily {
	switch key {
	case PolicyScale, PolicyScaleList, PolicyQuestionnaire:
		return CacheFamilyStatic
	case PolicyAssessmentDetail, PolicyTestee, PolicyPlan:
		return CacheFamilyObject
	case PolicyAssessmentList, PolicyStatsQuery:
		return CacheFamilyQuery
	default:
		return CacheFamilyDefault
	}
}
