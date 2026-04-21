package cachepolicy

import "github.com/FangcunMount/qs-server/internal/pkg/redisplane"

// PolicyCatalog 统一管理对象级缓存策略。
// 它只负责对象策略与逻辑 family 的映射，不再承担 Redis profile/namespace 路由职责。
type PolicyCatalog struct {
	familyDefaults map[redisplane.Family]CachePolicy
	policies       map[CachePolicyKey]CachePolicy
}

// NewPolicyCatalog 创建对象级缓存策略目录。
func NewPolicyCatalog(familyDefaults map[redisplane.Family]CachePolicy, policies map[CachePolicyKey]CachePolicy) *PolicyCatalog {
	catalog := &PolicyCatalog{
		familyDefaults: make(map[redisplane.Family]CachePolicy),
		policies:       make(map[CachePolicyKey]CachePolicy),
	}
	for family, policy := range familyDefaults {
		catalog.familyDefaults[family] = policy
	}
	for key, policy := range policies {
		catalog.policies[key] = policy
	}
	return catalog
}

// Policy 返回对象级缓存策略。
// 若对象自身未显式配置，则自动继承所属 family 的默认策略。
func (c *PolicyCatalog) Policy(key CachePolicyKey) CachePolicy {
	if c == nil {
		return CachePolicy{}
	}
	return c.policies[key].MergeWith(c.familyDefaults[FamilyFor(key)])
}

// FamilyFor 返回对象策略所属的逻辑 Redis family。
func FamilyFor(key CachePolicyKey) redisplane.Family {
	switch key {
	case PolicyScale, PolicyScaleList, PolicyQuestionnaire:
		return redisplane.FamilyStatic
	case PolicyAssessmentDetail, PolicyTestee, PolicyPlan:
		return redisplane.FamilyObject
	case PolicyAssessmentList, PolicyStatsQuery:
		return redisplane.FamilyQuery
	default:
		return redisplane.FamilyDefault
	}
}
