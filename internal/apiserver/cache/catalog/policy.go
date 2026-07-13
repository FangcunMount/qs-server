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

const (
	CapabilitySurveyQuestionnaire        sharedcache.Capability = "survey.questionnaire"
	CapabilityModelCatalogPublished      sharedcache.Capability = "modelcatalog.published_model"
	CapabilityEvaluationAssessmentDetail sharedcache.Capability = "evaluation.assessment_detail"
	CapabilityEvaluationAssessmentList   sharedcache.Capability = "evaluation.assessment_list"
	CapabilityActorTestee                sharedcache.Capability = "actor.testee"
	CapabilityPlanDetail                 sharedcache.Capability = "plan.detail"
	CapabilityStatisticsQuery            sharedcache.Capability = "statistics.query"
	CapabilityReportStatus               sharedcache.Capability = "report_status"
)
