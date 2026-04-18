package cache

import cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"

type PolicySwitch = cachepolicy.PolicySwitch

const (
	PolicySwitchInherit  = cachepolicy.PolicySwitchInherit
	PolicySwitchEnabled  = cachepolicy.PolicySwitchEnabled
	PolicySwitchDisabled = cachepolicy.PolicySwitchDisabled
)

var (
	PolicySwitchFromBool    = cachepolicy.PolicySwitchFromBool
	PolicySwitchFromBoolPtr = cachepolicy.PolicySwitchFromBoolPtr
)

type CachePolicyKey = cachepolicy.CachePolicyKey

const (
	PolicyScale            = cachepolicy.PolicyScale
	PolicyScaleList        = cachepolicy.PolicyScaleList
	PolicyQuestionnaire    = cachepolicy.PolicyQuestionnaire
	PolicyAssessmentDetail = cachepolicy.PolicyAssessmentDetail
	PolicyAssessmentList   = cachepolicy.PolicyAssessmentList
	PolicyTestee           = cachepolicy.PolicyTestee
	PolicyPlan             = cachepolicy.PolicyPlan
	PolicyStatsQuery       = cachepolicy.PolicyStatsQuery
)

type CachePolicy = cachepolicy.CachePolicy
