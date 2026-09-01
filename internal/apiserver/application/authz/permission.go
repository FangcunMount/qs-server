package authz

// IAM AuthZ v3 resource identifiers consumed by qs-server.
const (
	QuestionnaireResource      = "qs:questionnaire:collection:questionnaires"
	AssessmentModelResource    = "qs:scale:collection:scales"
	NormTableResource          = "qs:modelcatalog:collection:norm_tables"
	AnswerSheetResource        = "qs:answersheet:collection:answersheets"
	AssessmentResource         = "qs:evaluation:collection:assessments"
	EvaluationPlanResource     = "qs:plan:collection:evaluation_plans"
	EvaluationPlanTaskResource = "qs:plan_task:collection:evaluation_plan_tasks"
)
