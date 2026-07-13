package cachepolicy

import sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"

// Transitional aliases keep the object policy catalog stable while adapters
// move to the shared cache kernel. They are removed with the legacy package.
type PolicySwitch = sharedcache.PolicySwitch

const (
	PolicySwitchInherit  = sharedcache.PolicySwitchInherit
	PolicySwitchEnabled  = sharedcache.PolicySwitchEnabled
	PolicySwitchDisabled = sharedcache.PolicySwitchDisabled
)

var (
	PolicySwitchFromBool    = sharedcache.PolicySwitchFromBool
	PolicySwitchFromBoolPtr = sharedcache.PolicySwitchFromBoolPtr
)

type CachePolicy = sharedcache.Policy

// CachePolicyKey identifies an apiserver object-level cache policy.
type CachePolicyKey string

const (
	PolicyScale            CachePolicyKey = "scale"
	PolicyQuestionnaire    CachePolicyKey = "questionnaire"
	PolicyPublishedModel   CachePolicyKey = "published_model"
	PolicyAssessmentDetail CachePolicyKey = "assessment_detail"
	PolicyAssessmentList   CachePolicyKey = "assessment_list"
	PolicyTestee           CachePolicyKey = "testee"
	PolicyPlan             CachePolicyKey = "plan"
	PolicyStatsQuery       CachePolicyKey = "stats_query"
)
