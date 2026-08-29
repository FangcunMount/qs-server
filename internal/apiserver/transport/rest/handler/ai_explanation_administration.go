package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	aiexplanationadministration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appgovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/governance"
	apprecovery "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/recovery"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

type AIExplanationAdministrationHandler struct {
	*BaseHandler
	service aiexplanationadministration.Service
}

func NewAIExplanationAdministrationHandler(service aiexplanationadministration.Service) *AIExplanationAdministrationHandler {
	return &AIExplanationAdministrationHandler{BaseHandler: &BaseHandler{}, service: service}
}

type AIExplanationReviewRequest struct {
	CaseID   string `json:"case_id" binding:"required"`
	Attempt  int    `json:"attempt" binding:"required"`
	Role     string `json:"role" binding:"required"`
	Decision string `json:"decision" binding:"required"`
	Reason   string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationFinalizeRequest struct {
	Reason string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationEvaluationStartRequest struct {
	Confirm                     bool   `json:"confirm" binding:"required"`
	ExpectedProviderInvocations int    `json:"expected_provider_invocations" binding:"required"`
	Reason                      string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationEvaluationRecoverRequest struct {
	Confirm                     bool   `json:"confirm" binding:"required"`
	ExpectedProviderInvocations int    `json:"expected_provider_invocations" binding:"required"`
	Reason                      string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationAttemptRecheckRequest struct {
	Confirm                     bool   `json:"confirm" binding:"required"`
	ExpectedProviderInvocations int    `json:"expected_provider_invocations" binding:"required"`
	Reason                      string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationParticipantRetryRequest struct {
	ExpectedAttempt             int    `json:"expected_attempt" binding:"required"`
	RequestID                   string `json:"request_id" binding:"required,max=256"`
	Confirm                     bool   `json:"confirm" binding:"required"`
	ExpectedProviderInvocations int    `json:"expected_provider_invocations" binding:"required"`
	AcceptResultUnknownRisk     bool   `json:"accept_result_unknown_risk"`
	Reason                      string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationEvaluationCancelRequest struct {
	Reason string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationProfileDraftRequest struct {
	Definition  domainprofile.Definition `json:"definition" binding:"required"`
	Fingerprint string                   `json:"fingerprint" binding:"required"`
	Reason      string                   `json:"reason" binding:"required,max=1000"`
}

type AIExplanationProfilePublishRequest struct {
	EvaluationRunID string `json:"evaluation_run_id" binding:"required"`
	Reason          string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationProfileDisableRequest struct {
	Reason string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationEvaluationRunWire struct {
	RunID                          string                                `json:"run_id"`
	Version                        int64                                 `json:"version"`
	Status                         string                                `json:"status"`
	RequestedOrgID                 int64                                 `json:"requested_org_id,omitempty"`
	RequestedBy                    string                                `json:"requested_by,omitempty"`
	RequestReason                  string                                `json:"request_reason,omitempty"`
	CreatedAt                      time.Time                             `json:"created_at"`
	Execution                      *AIExplanationEvaluationExecutionWire `json:"execution,omitempty"`
	Recoveries                     []AIExplanationEvaluationRecoveryWire `json:"recoveries"`
	Release                        AIExplanationEvaluationReleaseWire    `json:"release"`
	Progress                       AIExplanationReviewProgressWire       `json:"progress"`
	Attempts                       []AIExplanationReviewAttemptSummary   `json:"attempts"`
	Finalized                      *AIExplanationFinalizationWire        `json:"finalized,omitempty"`
	Canceled                       *AIExplanationCancellationWire        `json:"canceled,omitempty"`
	Gate                           *AIExplanationGateWire                `json:"gate,omitempty"`
	CanReview                      bool                                  `json:"can_review"`
	CanFinalize                    bool                                  `json:"can_finalize"`
	CanCancel                      bool                                  `json:"can_cancel"`
	RecoveryMaxProviderInvocations int                                   `json:"recovery_max_provider_invocations"`
}

type AIExplanationEvaluationPageWire struct {
	Items      []AIExplanationEvaluationSummaryWire `json:"items"`
	NextCursor string                               `json:"next_cursor,omitempty"`
}

// AIExplanationEvaluationSummaryWire is intentionally bounded for queue
// views. Attempt inputs and Provider outputs are available only from the
// explicit attempt evidence endpoint.
type AIExplanationEvaluationSummaryWire struct {
	RunID                          string                             `json:"run_id"`
	Version                        int64                              `json:"version"`
	Status                         string                             `json:"status"`
	RequestedOrgID                 int64                              `json:"requested_org_id"`
	RequestedBy                    string                             `json:"requested_by"`
	RequestReason                  string                             `json:"request_reason"`
	CreatedAt                      time.Time                          `json:"created_at"`
	Release                        AIExplanationEvaluationReleaseWire `json:"release"`
	Progress                       AIExplanationReviewProgressWire    `json:"progress"`
	Gate                           *AIExplanationGateWire             `json:"gate,omitempty"`
	CanReview                      bool                               `json:"can_review"`
	CanFinalize                    bool                               `json:"can_finalize"`
	CanCancel                      bool                               `json:"can_cancel"`
	RecoveryMaxProviderInvocations int                                `json:"recovery_max_provider_invocations"`
}

type AIExplanationProfilePageWire struct {
	Items      []AIExplanationProfileWire `json:"items"`
	NextCursor string                     `json:"next_cursor,omitempty"`
}

type AIExplanationEvaluationCapacityWire struct {
	OrganizationID               int64                                  `json:"organization_id"`
	BudgetDay                    time.Time                              `json:"budget_day"`
	MaxActiveRunsPerOrg          int                                    `json:"max_active_runs_per_org"`
	ProviderInvocationsPerStart  int                                    `json:"provider_invocations_per_start"`
	DailyProviderInvocationLimit int                                    `json:"daily_provider_invocation_limit"`
	ReservedProviderInvocations  int                                    `json:"reserved_provider_invocations"`
	RemainingProviderInvocations int                                    `json:"remaining_provider_invocations"`
	AvailableFullRunStarts       int                                    `json:"available_full_run_starts"`
	OverLimit                    bool                                   `json:"over_limit"`
	Reservations                 []AIExplanationCapacityReservationWire `json:"reservations"`
}

type AIExplanationCapacityReservationWire struct {
	RunID               string    `json:"run_id"`
	RequestedBy         string    `json:"requested_by"`
	ProviderInvocations int       `json:"provider_invocations"`
	ReservedAt          time.Time `json:"reserved_at"`
}

type AIExplanationParticipantCapacityWire struct {
	OrganizationID                            int64                                                   `json:"organization_id"`
	BudgetDay                                 time.Time                                               `json:"budget_day"`
	ProviderInvocationsPerGeneration          int                                                     `json:"provider_invocations_per_generation"`
	DailyProviderInvocationLimitPerOrg        int                                                     `json:"daily_provider_invocation_limit_per_org"`
	DailyProviderInvocationLimitPerUser       int                                                     `json:"daily_provider_invocation_limit_per_user"`
	DailyProviderInvocationLimitPerAssessment int                                                     `json:"daily_provider_invocation_limit_per_assessment"`
	MaxActiveProviderExecutionsPerOrg         int                                                     `json:"max_active_provider_executions_per_org"`
	MaxActiveProviderExecutionsPerUser        int                                                     `json:"max_active_provider_executions_per_user"`
	MaxActiveProviderExecutionsPerAssessment  int                                                     `json:"max_active_provider_executions_per_assessment"`
	ReservedProviderInvocations               int                                                     `json:"reserved_provider_invocations"`
	RedactedProviderInvocations               int                                                     `json:"redacted_provider_invocations"`
	RemainingOrgProviderInvocations           int                                                     `json:"remaining_org_provider_invocations"`
	OverOrgLimit                              bool                                                    `json:"over_org_limit"`
	Reservations                              []AIExplanationParticipantCapacityReservationWire       `json:"reservations"`
	ActiveProviderExecutions                  int                                                     `json:"active_provider_executions"`
	RemainingOrgActiveProviderExecutions      int                                                     `json:"remaining_org_active_provider_executions"`
	OverOrgActiveLimit                        bool                                                    `json:"over_org_active_limit"`
	ActiveReservations                        []AIExplanationParticipantActiveCapacityReservationWire `json:"active_reservations"`
}

type AIExplanationParticipantCapacityReservationWire struct {
	ReservationID       string    `json:"reservation_id"`
	GenerationID        string    `json:"generation_id"`
	Attempt             int       `json:"attempt"`
	Origin              string    `json:"origin"`
	UserID              string    `json:"user_id"`
	AssessmentID        string    `json:"assessment_id"`
	ProviderInvocations int       `json:"provider_invocations"`
	ReservedAt          time.Time `json:"reserved_at"`
}

type AIExplanationParticipantRetryWire struct {
	GenerationID              string    `json:"generation_id"`
	FailedRunID               string    `json:"failed_run_id"`
	ExpectedAttempt           int       `json:"expected_attempt"`
	NextAttempt               int       `json:"next_attempt"`
	Origin                    string    `json:"origin"`
	RequestID                 string    `json:"request_id"`
	AuthorizedAt              time.Time `json:"authorized_at"`
	AcceptedResultUnknownRisk bool      `json:"accepted_result_unknown_risk"`
	Created                   bool      `json:"created"`
}

type AIExplanationParticipantActiveCapacityReservationWire struct {
	GenerationID string    `json:"generation_id"`
	RunID        string    `json:"run_id"`
	UserID       string    `json:"user_id"`
	AssessmentID string    `json:"assessment_id"`
	AcquiredAt   time.Time `json:"acquired_at"`
}

type AIExplanationEvaluationRecoveryWire struct {
	ID          string    `json:"id"`
	CaseID      string    `json:"case_id"`
	Attempt     int       `json:"attempt"`
	Actor       string    `json:"actor"`
	Reason      string    `json:"reason"`
	RequestedAt time.Time `json:"requested_at"`
}

type AIExplanationAttemptRecheckWire struct {
	RecheckID      string                                `json:"recheck_id"`
	SourceRunID    string                                `json:"source_run_id"`
	SourceCaseID   string                                `json:"source_case_id"`
	SourceAttempt  int                                   `json:"source_attempt"`
	Status         string                                `json:"status"`
	Version        int64                                 `json:"version"`
	RequestedOrgID int64                                 `json:"requested_org_id"`
	RequestedBy    string                                `json:"requested_by"`
	Reason         string                                `json:"reason"`
	CreatedAt      time.Time                             `json:"created_at"`
	FinishedAt     *time.Time                            `json:"finished_at,omitempty"`
	Release        AIExplanationEvaluationReleaseWire    `json:"release"`
	Execution      *AIExplanationEvaluationExecutionWire `json:"execution,omitempty"`
	Result         *AIExplanationReviewAttemptWire       `json:"result,omitempty"`
}

type AIExplanationCancellationWire struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Reason string    `json:"reason"`
}

type AIExplanationEvaluationExecutionWire struct {
	CaseID            string     `json:"case_id"`
	Attempt           int        `json:"attempt"`
	Phase             string     `json:"phase"`
	ClaimedAt         time.Time  `json:"claimed_at"`
	LeaseExpiresAt    time.Time  `json:"lease_expires_at"`
	DispatchStartedAt *time.Time `json:"dispatch_started_at,omitempty"`
}

type AIExplanationEvaluationReleaseWire struct {
	Suite                    AIExplanationSuiteRefWire          `json:"suite"`
	Prompt                   AIExplanationPromptRefWire         `json:"prompt"`
	Profile                  AIExplanationProfileRefWire        `json:"profile"`
	InputSchema              AIExplanationSchemaRefWire         `json:"input_schema"`
	OutputSchema             AIExplanationSchemaRefWire         `json:"output_schema"`
	Provider                 AIExplanationProviderSpecWire      `json:"provider"`
	Decoding                 AIExplanationDecodingWire          `json:"decoding"`
	SemanticEvaluator        AIExplanationSemanticEvaluatorWire `json:"semantic_evaluator"`
	GenerationCaseIDs        []string                           `json:"generation_case_ids"`
	PreflightCaseID          string                             `json:"preflight_case_id"`
	PreflightRejectionReason string                             `json:"preflight_rejection_reason"`
	RepetitionsPerCase       int                                `json:"repetitions_per_case"`
}

type AIExplanationSuiteRefWire struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
	GitBlobSHA  string `json:"git_blob_sha"`
}

type AIExplanationPromptRefWire struct {
	TemplateID  string `json:"template_id"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
	GitBlobSHA  string `json:"git_blob_sha"`
}

type AIExplanationProfileRefWire struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

type AIExplanationSchemaRefWire struct {
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

type AIExplanationProviderSpecWire struct {
	Route            string `json:"route"`
	RouteRevision    string `json:"route_revision"`
	ResolvedProvider string `json:"resolved_provider"`
	ResolvedModel    string `json:"resolved_model"`
	Fingerprint      string `json:"fingerprint"`
}

type AIExplanationDecodingWire struct {
	MaxOutputTokens int      `json:"max_output_tokens"`
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`
	Seed            *int64   `json:"seed,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
}

type AIExplanationSemanticEvaluatorWire struct {
	Version      string                        `json:"version"`
	Prompt       AIExplanationPromptRefWire    `json:"prompt"`
	OutputSchema AIExplanationSchemaRefWire    `json:"output_schema"`
	Provider     AIExplanationProviderSpecWire `json:"provider"`
	Decoding     AIExplanationDecodingWire     `json:"decoding"`
}

type AIExplanationReviewProgressWire struct {
	PlannedGenerationAttempts  int  `json:"planned_generation_attempts"`
	GenerationAttempts         int  `json:"generation_attempts"`
	FailedAttempts             int  `json:"failed_attempts"`
	PendingGenerationAttempts  int  `json:"pending_generation_attempts"`
	RequiredReviews            int  `json:"required_reviews"`
	RecordedReviews            int  `json:"recorded_reviews"`
	MissingReviews             int  `json:"missing_reviews"`
	FullyReviewedAttempts      int  `json:"fully_reviewed_attempts"`
	RejectedReviews            int  `json:"rejected_reviews"`
	AllRequiredReviewsRecorded bool `json:"all_required_reviews_recorded"`
}

type AIExplanationReviewAttemptSummary struct {
	CaseID            string                           `json:"case_id"`
	Attempt           int                              `json:"attempt"`
	OutputFingerprint string                           `json:"output_fingerprint,omitempty"`
	Failure           *AIExplanationAttemptFailureWire `json:"failure,omitempty"`
	SemanticScores    *AIExplanationSemanticScoresWire `json:"semantic_scores,omitempty"`
	Reviews           []AIExplanationHumanReviewWire   `json:"reviews"`
	MissingRoles      []string                         `json:"missing_roles"`
}

type AIExplanationReviewAttemptWire struct {
	AIExplanationReviewAttemptSummary
	AssessmentInput   json.RawMessage                     `json:"assessment_input" swaggertype:"object"`
	RawProviderOutput string                              `json:"raw_provider_output"`
	NormalizedOutput  json.RawMessage                     `json:"normalized_output,omitempty" swaggertype:"object"`
	ProviderReceipt   *AIExplanationProviderReceiptWire   `json:"provider_receipt,omitempty"`
	Assertions        []AIExplanationAssertionReceiptWire `json:"assertions"`
	Semantic          *AIExplanationSemanticReceiptWire   `json:"semantic,omitempty"`
}

type AIExplanationProviderReceiptWire struct {
	InvocationID string `json:"invocation_id"`
	RequestID    string `json:"request_id,omitempty"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	LatencyMS    int64  `json:"latency_ms"`
}

type AIExplanationAttemptFailureWire struct {
	Stage         string `json:"stage"`
	Code          string `json:"code"`
	SafeMessage   string `json:"safe_message"`
	Retryable     bool   `json:"retryable"`
	ResultUnknown bool   `json:"result_unknown"`
}

type AIExplanationAssertionReceiptWire struct {
	Type      string `json:"type"`
	Scope     string `json:"scope"`
	Ordinal   int    `json:"ordinal"`
	Hard      bool   `json:"hard"`
	Evaluator string `json:"evaluator"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}

type AIExplanationSemanticScoresWire struct {
	Faithfulness            int `json:"faithfulness"`
	CrossDimensionQuality   int `json:"cross_dimension_quality"`
	SuggestionActionability int `json:"suggestion_actionability"`
	AudienceClarity         int `json:"audience_clarity"`
	Concision               int `json:"concision"`
}

type AIExplanationSemanticReceiptWire struct {
	EvaluatorVersion string                           `json:"evaluator_version"`
	ProviderReceipt  AIExplanationProviderReceiptWire `json:"provider_receipt"`
	Scores           AIExplanationSemanticScoresWire  `json:"scores"`
	Rationale        string                           `json:"rationale"`
}

type AIExplanationHumanReviewWire struct {
	Role       string    `json:"role"`
	Reviewer   string    `json:"reviewer"`
	Decision   string    `json:"decision"`
	ReviewedAt time.Time `json:"reviewed_at"`
	Reason     string    `json:"reason"`
}

type AIExplanationFinalizationWire struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Reason string    `json:"reason"`
}

type AIExplanationGateWire struct {
	Passed  bool                            `json:"passed"`
	Reasons []AIExplanationGateReasonWire   `json:"reasons"`
	Metrics AIExplanationQualityMetricsWire `json:"metrics"`
}

type AIExplanationGateReasonWire struct {
	Code    string `json:"code"`
	CaseID  string `json:"case_id,omitempty"`
	Attempt int    `json:"attempt,omitempty"`
	Detail  string `json:"detail"`
}

type AIExplanationQualityMetricsWire struct {
	GenerationAttempts     int     `json:"generation_attempts"`
	CaseAssertionPasses    int     `json:"case_assertion_passes"`
	FaithfulnessAverage    float64 `json:"faithfulness_average"`
	CrossDimensionAverage  float64 `json:"cross_dimension_average"`
	ActionabilityAverage   float64 `json:"actionability_average"`
	AudienceClarityAverage float64 `json:"audience_clarity_average"`
	ConcisionAverage       float64 `json:"concision_average"`
	HumanReviews           int     `json:"human_reviews"`
}

type AIExplanationProfileWire struct {
	ID                     string                   `json:"id"`
	Definition             domainprofile.Definition `json:"definition"`
	Fingerprint            string                   `json:"fingerprint"`
	Status                 string                   `json:"status"`
	CreatedAt              time.Time                `json:"created_at"`
	CreatedBy              string                   `json:"created_by,omitempty"`
	CreatedReason          string                   `json:"created_reason,omitempty"`
	UpdatedAt              time.Time                `json:"updated_at"`
	PublishedAt            *time.Time               `json:"published_at,omitempty"`
	PublishedBy            string                   `json:"published_by,omitempty"`
	PublishedReason        string                   `json:"published_reason,omitempty"`
	PublishedEvidenceRunID string                   `json:"published_evidence_run_id,omitempty"`
	DisabledAt             *time.Time               `json:"disabled_at,omitempty"`
	DisabledBy             string                   `json:"disabled_by,omitempty"`
	DisabledReason         string                   `json:"disabled_reason,omitempty"`
}

// FindEvaluationCapacity godoc
// @Summary 查询当前机构 AI 解读 Prompt 评测容量
// @Description 按可信身份中的机构读取当前 UTC 日预算账本、剩余完整评测次数与逐 Run 预留审计；不接受请求体中的机构或日期。
// @Tags AI-Explanation-Administration
// @Produce json
// @Success 200 {object} core.Response{data=AIExplanationEvaluationCapacityWire}
// @Failure 501 {object} core.ErrResponse
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluation-capacity [get]
func (h *AIExplanationAdministrationHandler) FindEvaluationCapacity(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.FindEvaluationCapacity(c.Request.Context(), actor)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationCapacityWire(result))
}

// FindParticipantCapacity godoc
// @Summary 查询当前机构 Participant AI 解读容量
// @Description 按可信身份中的机构读取当前 UTC 日机构/用户/Assessment 上限与逐 Generation 预留审计；不接受请求体中的机构或日期。
// @Tags AI-Explanation-Administration
// @Produce json
// @Success 200 {object} core.Response{data=AIExplanationParticipantCapacityWire}
// @Failure 501 {object} core.ErrResponse
// @Router /internal/v1/interpretation/ai-explanation/participant-capacity [get]
func (h *AIExplanationAdministrationHandler) FindParticipantCapacity(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.FindParticipantCapacity(c.Request.Context(), actor)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, participantCapacityWire(result))
}

// RetryParticipantGeneration godoc
// @Summary 人工恢复失败的 Participant AI 解读
// @Description 为指定 failed attempt 原子写入一次人工重试授权、追加 1 次 UTC 日调用预算并投递下一 attempt；provider_result_unknown 必须显式接受潜在重复调用与计费风险。相同 request_id 幂等复用。
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param generation_id path string true "AI 解读 Generation ID"
// @Param request body AIExplanationParticipantRetryRequest true "attempt、幂等键、成本与未知结果风险确认"
// @Success 202 {object} core.Response{data=AIExplanationParticipantRetryWire}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/interpretation/ai-explanation/generations/{generation_id}/retry [post]
func (h *AIExplanationAdministrationHandler) RetryParticipantGeneration(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	generationID, err := meta.ParseID(c.Param("generation_id"))
	if err != nil || generationID.IsZero() {
		h.Error(c, cberrors.WithCode(code.ErrInvalidArgument, "generation_id is invalid"))
		return
	}
	var request AIExplanationParticipantRetryRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.RetryParticipantGeneration(c.Request.Context(), actor, aiexplanationadministration.RetryParticipantGenerationCommand{
		GenerationID: generationID, ExpectedAttempt: request.ExpectedAttempt, RequestID: request.RequestID,
		Confirm: request.Confirm, ExpectedProviderInvocations: request.ExpectedProviderInvocations,
		AcceptResultUnknownRisk: request.AcceptResultUnknownRisk, Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, core.Response{Code: 0, Message: "accepted", Data: participantRetryWire(result)})
}

// StartEvaluation godoc
// @Summary 手动启动 AI 解读 Prompt 在线评测
// @Description 请求体必须确认 v1 最多执行 70 次 Provider 调用；start 同时执行机构 collecting 并发和 UTC 日调用预算准入，仅持久化预算预留、评测与首个事件，不同步调用模型。
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param request body AIExplanationEvaluationStartRequest true "成本确认与审计理由"
// @Success 202 {object} core.Response{data=AIExplanationEvaluationRunWire}
// @Failure 400 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations [post]
func (h *AIExplanationAdministrationHandler) StartEvaluation(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request AIExplanationEvaluationStartRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.StartEvaluation(c.Request.Context(), actor, aiexplanationadministration.StartEvaluationCommand{
		Confirm: request.Confirm, ExpectedProviderInvocations: request.ExpectedProviderInvocations, Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, core.Response{Code: 0, Message: "accepted", Data: evaluationRunWire(result)})
}

// RecoverEvaluation godoc
// @Summary 人工恢复 AI 解读 Prompt 在线评测
// @Description 仅重新投递当前待执行或租约已过期的 attempt；dispatch 结果未知时不会重放原 Provider 调用。
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测运行 ID"
// @Param request body AIExplanationEvaluationRecoverRequest true "剩余链路成本确认与恢复理由"
// @Success 202 {object} core.Response{data=AIExplanationEvaluationRunWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/recover [post]
func (h *AIExplanationAdministrationHandler) RecoverEvaluation(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationEvaluationRecoverRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.RecoverEvaluation(c.Request.Context(), actor, runID, aiexplanationadministration.RecoverEvaluationCommand{
		Confirm: request.Confirm, ExpectedProviderInvocations: request.ExpectedProviderInvocations, Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, core.Response{Code: 0, Message: "accepted", Data: evaluationRunWire(result)})
}

// CancelEvaluation godoc
// @Summary 取消尚未 dispatch 的 AI 解读 Prompt 在线评测
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测运行 ID"
// @Param request body AIExplanationEvaluationCancelRequest true "取消理由"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationRunWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/cancel [post]
func (h *AIExplanationAdministrationHandler) CancelEvaluation(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationEvaluationCancelRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.CancelEvaluation(c.Request.Context(), actor, runID, request.Reason)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationRunWire(result))
}

// ListEvaluations godoc
// @Summary 分页查询 AI 解读 Prompt 评测审核目录
// @Description 只返回当前可信机构的队列摘要；原始测评输入和 Provider 输出必须通过单份 attempt 证据接口读取。
// @Tags AI-Explanation-Administration
// @Produce json
// @Param status query string false "状态" Enums(collecting,awaiting_review,approved,rejected,canceled)
// @Param cursor query string false "稳定分页游标"
// @Param limit query int false "页大小，默认 20，最大 100"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationPageWire}
// @Failure 400 {object} core.ErrResponse
// @Failure 501 {object} core.ErrResponse
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations [get]
func (h *AIExplanationAdministrationHandler) ListEvaluations(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	status, err := optionalEvaluationStatus(c.Query("status"))
	if err != nil {
		h.Error(c, err)
		return
	}
	limit, err := administrationCatalogLimit(c, appevaluation.DefaultReviewRunPageSize, appevaluation.MaxReviewRunPageSize)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.ListEvaluations(c.Request.Context(), actor, aiexplanationadministration.EvaluationListQuery{
		Status: status, Cursor: c.Query("cursor"), Limit: limit,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationPageWire(result))
}

// FindEvaluation godoc
// @Summary 查询 AI 解读 Prompt 评测摘要
// @Tags AI-Explanation-Administration
// @Produce json
// @Param run_id path string true "评测运行 ID"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationRunWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id} [get]
func (h *AIExplanationAdministrationHandler) FindEvaluation(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	result, err := h.service.FindEvaluation(c.Request.Context(), actor, runID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationRunWire(result))
}

// FindAttempt godoc
// @Summary 查询一份 AI 解读 Prompt 评测证据
// @Tags AI-Explanation-Administration
// @Produce json
// @Param run_id path string true "评测运行 ID"
// @Param case_id path string true "合成 case ID"
// @Param attempt path int true "重复序号"
// @Success 200 {object} core.Response{data=AIExplanationReviewAttemptWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/attempts/{case_id}/{attempt} [get]
func (h *AIExplanationAdministrationHandler) FindAttempt(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	attempt, err := parsePositiveInt(c.Param("attempt"))
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.FindEvaluation(c.Request.Context(), actor, runID)
	if err != nil {
		h.Error(c, err)
		return
	}
	for _, item := range result.Attempts {
		if item.CaseID == c.Param("case_id") && item.Attempt == attempt {
			h.Success(c, reviewAttemptWire(item))
			return
		}
	}
	h.Error(c, cberrors.WithCode(code.ErrPageNotFound, "AI explanation review attempt not found"))
}

// StartAttemptRecheck godoc
// @Summary 重新测评一条 AI 解读 Prompt 评测记录
// @Description 创建独立诊断证据；不覆盖源记录，不参与 35+35 发布门禁，最多调用两次 Provider。
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "源评测运行 ID"
// @Param case_id path string true "合成 case ID"
// @Param attempt path int true "源重复序号"
// @Param request body AIExplanationAttemptRecheckRequest true "复测成本确认与审计理由"
// @Success 202 {object} core.Response{data=AIExplanationAttemptRecheckWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/attempts/{case_id}/{attempt}/rechecks [post]
func (h *AIExplanationAdministrationHandler) StartAttemptRecheck(c *gin.Context) {
	actor, runID, attempt, ok := h.actorAndAttemptAddress(c)
	if !ok {
		return
	}
	var request AIExplanationAttemptRecheckRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.StartEvaluationRecheck(c.Request.Context(), actor, runID, c.Param("case_id"), attempt, aiexplanationadministration.StartEvaluationRecheckCommand{
		Confirm: request.Confirm, ExpectedProviderInvocations: request.ExpectedProviderInvocations, Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, core.Response{Code: 0, Message: "accepted", Data: attemptRecheckWire(result, true)})
}

// ListAttemptRechecks godoc
// @Summary 查询一条 AI 解读评测记录的复测历史
// @Tags AI-Explanation-Administration
// @Produce json
// @Param run_id path string true "源评测运行 ID"
// @Param case_id path string true "合成 case ID"
// @Param attempt path int true "源重复序号"
// @Param limit query int false "页大小，默认 20，最大 100"
// @Success 200 {object} core.Response{data=[]AIExplanationAttemptRecheckWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/attempts/{case_id}/{attempt}/rechecks [get]
func (h *AIExplanationAdministrationHandler) ListAttemptRechecks(c *gin.Context) {
	actor, runID, attempt, ok := h.actorAndAttemptAddress(c)
	if !ok {
		return
	}
	limit, err := administrationCatalogLimit(c, 20, 100)
	if err != nil {
		h.Error(c, err)
		return
	}
	values, err := h.service.ListEvaluationRechecks(c.Request.Context(), actor, runID, c.Param("case_id"), attempt, limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	result := make([]AIExplanationAttemptRecheckWire, 0, len(values))
	for _, value := range values {
		result = append(result, attemptRecheckWire(value, false))
	}
	h.Success(c, result)
}

// FindAttemptRecheck godoc
// @Summary 查询一份 AI 解读单条复测证据
// @Tags AI-Explanation-Administration
// @Produce json
// @Param run_id path string true "源评测运行 ID"
// @Param case_id path string true "合成 case ID"
// @Param attempt path int true "源重复序号"
// @Param recheck_id path string true "复测 ID"
// @Success 200 {object} core.Response{data=AIExplanationAttemptRecheckWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/attempts/{case_id}/{attempt}/rechecks/{recheck_id} [get]
func (h *AIExplanationAdministrationHandler) FindAttemptRecheck(c *gin.Context) {
	actor, runID, attempt, ok := h.actorAndAttemptAddress(c)
	if !ok {
		return
	}
	recheckID, err := meta.ParseID(c.Param("recheck_id"))
	if err != nil || recheckID.IsZero() {
		h.Error(c, cberrors.WithCode(code.ErrInvalidArgument, "recheck_id is invalid"))
		return
	}
	value, err := h.service.FindEvaluationRecheck(c.Request.Context(), actor, runID, c.Param("case_id"), attempt, recheckID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, attemptRecheckWire(value, true))
}

// RecordReview godoc
// @Summary 记录 AI 解读 Prompt 人工复核
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测运行 ID"
// @Param request body AIExplanationReviewRequest true "复核命令"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationRunWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/reviews [post]
func (h *AIExplanationAdministrationHandler) RecordReview(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationReviewRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.RecordReview(c.Request.Context(), actor, runID, aiexplanationadministration.ReviewCommand{
		CaseID: request.CaseID, Attempt: request.Attempt, Role: domainevaluation.ReviewRole(request.Role),
		Decision: domainevaluation.ReviewDecision(request.Decision), Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationRunWire(result))
}

// FinalizeEvaluation godoc
// @Summary 终审 AI 解读 Prompt 评测
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测运行 ID"
// @Param request body AIExplanationFinalizeRequest true "终审理由"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationRunWire}
// @Router /internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/finalize [post]
func (h *AIExplanationAdministrationHandler) FinalizeEvaluation(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationFinalizeRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.FinalizeEvaluation(c.Request.Context(), actor, runID, request.Reason)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationRunWire(result))
}

// ListProfiles godoc
// @Summary 分页查询 AI 解读 Profile 治理目录
// @Tags AI-Explanation-Administration
// @Produce json
// @Param status query string false "状态" Enums(draft,published,disabled)
// @Param cursor query string false "稳定分页游标"
// @Param limit query int false "页大小，默认 20，最大 100"
// @Success 200 {object} core.Response{data=AIExplanationProfilePageWire}
// @Failure 400 {object} core.ErrResponse
// @Failure 501 {object} core.ErrResponse
// @Router /internal/v1/interpretation/ai-explanation/profiles [get]
func (h *AIExplanationAdministrationHandler) ListProfiles(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	status, err := optionalProfileStatus(c.Query("status"))
	if err != nil {
		h.Error(c, err)
		return
	}
	limit, err := administrationCatalogLimit(c, appgovernance.DefaultProfilePageSize, appgovernance.MaxProfilePageSize)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.ListProfiles(c.Request.Context(), actor, aiexplanationadministration.ProfileListQuery{
		Status: status, Cursor: c.Query("cursor"), Limit: limit,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, profilePageWire(result))
}

// FindProfile godoc
// @Summary 查询一个 AI 解读 Profile 版本
// @Tags AI-Explanation-Administration
// @Produce json
// @Param profile_id path string true "Profile ID"
// @Param version path string true "Profile 版本"
// @Success 200 {object} core.Response{data=AIExplanationProfileWire}
// @Failure 400 {object} core.ErrResponse
// @Failure 404 {object} core.ErrResponse
// @Failure 501 {object} core.ErrResponse
// @Router /internal/v1/interpretation/ai-explanation/profiles/{profile_id}/versions/{version} [get]
func (h *AIExplanationAdministrationHandler) FindProfile(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.FindProfile(c.Request.Context(), actor, c.Param("profile_id"), c.Param("version"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, profileWire(result))
}

// CreateProfileDraft godoc
// @Summary 创建 AI 解读 Profile draft
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param request body AIExplanationProfileDraftRequest true "Profile draft"
// @Success 200 {object} core.Response{data=AIExplanationProfileWire}
// @Router /internal/v1/interpretation/ai-explanation/profiles [post]
func (h *AIExplanationAdministrationHandler) CreateProfileDraft(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request AIExplanationProfileDraftRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	fingerprint, err := aiexplanation.ParseFingerprint(request.Fingerprint)
	if err != nil {
		h.Error(c, cberrors.WithCode(code.ErrInvalidArgument, "%s", err.Error()))
		return
	}
	result, err := h.service.CreateProfileDraft(c.Request.Context(), actor, aiexplanationadministration.CreateProfileDraftCommand{
		Definition: request.Definition, ExpectedFingerprint: fingerprint, Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, profileWire(result))
}

// PublishProfile godoc
// @Summary 发布 AI 解读 Profile
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param profile_id path string true "Profile ID"
// @Param version path string true "Profile 版本"
// @Param request body AIExplanationProfilePublishRequest true "发布命令"
// @Success 200 {object} core.Response{data=AIExplanationProfileWire}
// @Router /internal/v1/interpretation/ai-explanation/profiles/{profile_id}/versions/{version}/publish [post]
func (h *AIExplanationAdministrationHandler) PublishProfile(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request AIExplanationProfilePublishRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	runID, err := meta.ParseID(request.EvaluationRunID)
	if err != nil {
		h.Error(c, cberrors.WithCode(code.ErrInvalidArgument, "evaluation_run_id is invalid"))
		return
	}
	result, err := h.service.PublishProfile(c.Request.Context(), actor, aiexplanationadministration.PublishProfileCommand{
		ProfileID: c.Param("profile_id"), ProfileVersion: c.Param("version"), EvaluationRunID: runID, Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, profileWire(result))
}

// DisableProfile godoc
// @Summary 禁用 AI 解读 Profile
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param profile_id path string true "Profile ID"
// @Param version path string true "Profile 版本"
// @Param request body AIExplanationProfileDisableRequest true "禁用命令"
// @Success 200 {object} core.Response{data=AIExplanationProfileWire}
// @Router /internal/v1/interpretation/ai-explanation/profiles/{profile_id}/versions/{version}/disable [post]
func (h *AIExplanationAdministrationHandler) DisableProfile(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request AIExplanationProfileDisableRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	result, err := h.service.DisableProfile(c.Request.Context(), actor, aiexplanationadministration.DisableProfileCommand{
		ProfileID: c.Param("profile_id"), ProfileVersion: c.Param("version"), Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, profileWire(result))
}

func (h *AIExplanationAdministrationHandler) actor(c *gin.Context) (aiexplanationadministration.Actor, bool) {
	orgID, userID, err := h.RequireProtectedScope(c)
	if err != nil {
		h.Error(c, err)
		return aiexplanationadministration.Actor{}, false
	}
	return aiexplanationadministration.Actor{OrgID: orgID, OperatorUserID: userID}, true
}

func (h *AIExplanationAdministrationHandler) actorAndRunID(c *gin.Context) (aiexplanationadministration.Actor, meta.ID, bool) {
	actor, ok := h.actor(c)
	if !ok {
		return aiexplanationadministration.Actor{}, 0, false
	}
	runID, err := meta.ParseID(c.Param("run_id"))
	if err != nil {
		h.Error(c, cberrors.WithCode(code.ErrInvalidArgument, "run_id is invalid"))
		return aiexplanationadministration.Actor{}, 0, false
	}
	return actor, runID, true
}

func (h *AIExplanationAdministrationHandler) actorAndAttemptAddress(c *gin.Context) (aiexplanationadministration.Actor, meta.ID, int, bool) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return actor, runID, 0, false
	}
	attempt, err := parsePositiveInt(c.Param("attempt"))
	if err != nil {
		h.Error(c, err)
		return actor, runID, 0, false
	}
	if strings.TrimSpace(c.Param("case_id")) == "" {
		h.Error(c, cberrors.WithCode(code.ErrInvalidArgument, "case_id is invalid"))
		return actor, runID, 0, false
	}
	return actor, runID, attempt, true
}

func evaluationRunWire(value *appevaluation.ReviewRun) AIExplanationEvaluationRunWire {
	attempts := make([]AIExplanationReviewAttemptSummary, 0, len(value.Attempts))
	for _, item := range value.Attempts {
		attempts = append(attempts, reviewAttemptSummary(item))
	}
	result := AIExplanationEvaluationRunWire{
		RunID: value.RunID.String(), Version: value.Version, Status: string(value.Status),
		RequestedOrgID: value.RequestedOrgID, RequestedBy: value.RequestedBy,
		RequestReason: value.RequestReason, CreatedAt: value.CreatedAt,
		Release: releaseWire(value.Release), Progress: reviewProgressWire(value.Progress), Attempts: attempts,
		Gate: gateWire(value.Gate), CanReview: value.CanReview, CanFinalize: value.CanFinalize, CanCancel: value.CanCancel,
		RecoveryMaxProviderInvocations: value.RecoveryMaxProviderInvocations,
	}
	if value.Execution != nil {
		result.Execution = &AIExplanationEvaluationExecutionWire{
			CaseID: value.Execution.CaseID, Attempt: value.Execution.Attempt, Phase: string(value.Execution.Phase),
			ClaimedAt: value.Execution.ClaimedAt, LeaseExpiresAt: value.Execution.LeaseExpiresAt,
			DispatchStartedAt: value.Execution.DispatchStartedAt,
		}
	}
	result.Recoveries = make([]AIExplanationEvaluationRecoveryWire, 0, len(value.Recoveries))
	for _, recovery := range value.Recoveries {
		result.Recoveries = append(result.Recoveries, AIExplanationEvaluationRecoveryWire{
			ID: recovery.ID, CaseID: recovery.CaseID, Attempt: recovery.Attempt, Actor: recovery.Actor,
			Reason: recovery.Reason, RequestedAt: recovery.RequestedAt,
		})
	}
	if value.Finalized != nil {
		result.Finalized = &AIExplanationFinalizationWire{At: value.Finalized.At, Actor: value.Finalized.Actor, Reason: value.Finalized.Reason}
	}
	if value.Canceled != nil {
		result.Canceled = &AIExplanationCancellationWire{At: value.Canceled.At, Actor: value.Canceled.Actor, Reason: value.Canceled.Reason}
	}
	return result
}

func attemptRecheckWire(value *domainevaluation.PromptEvaluationRecheck, includeResult bool) AIExplanationAttemptRecheckWire {
	if value == nil {
		return AIExplanationAttemptRecheckWire{}
	}
	result := AIExplanationAttemptRecheckWire{
		RecheckID: value.ID().String(), SourceRunID: value.SourceRunID().String(), SourceCaseID: value.SourceCaseID(),
		SourceAttempt: value.SourceAttempt(), Status: string(value.Status()), Version: value.Version(),
		RequestedOrgID: value.RequestedOrgID(), RequestedBy: value.RequestedBy(), Reason: value.Reason(),
		CreatedAt: value.CreatedAt(), FinishedAt: value.FinishedAt(), Release: releaseWire(value.Release()),
	}
	if execution := value.Execution(); execution != nil {
		result.Execution = &AIExplanationEvaluationExecutionWire{
			CaseID: execution.CaseID, Attempt: execution.Attempt, Phase: string(execution.Phase),
			ClaimedAt: execution.ClaimedAt, LeaseExpiresAt: execution.LeaseExpiresAt, DispatchStartedAt: execution.DispatchStartedAt,
		}
	}
	if record := value.Result(); includeResult && record != nil {
		wire := reviewAttemptWire(appevaluation.ReviewAttempt{
			CaseID: record.CaseID, Attempt: record.Attempt, RawProviderOutput: record.RawOutput,
			NormalizedOutput: record.NormalizedOutput, ProviderReceipt: record.ProviderReceipt, Failure: record.Failure,
			Assertions: record.Assertions, Semantic: record.Semantic,
		})
		result.Result = &wire
	}
	return result
}

func evaluationPageWire(value *appevaluation.ReviewRunPage) AIExplanationEvaluationPageWire {
	result := AIExplanationEvaluationPageWire{Items: []AIExplanationEvaluationSummaryWire{}}
	if value == nil {
		return result
	}
	result.NextCursor = value.NextCursor
	result.Items = make([]AIExplanationEvaluationSummaryWire, 0, len(value.Items))
	for _, item := range value.Items {
		if item != nil {
			result.Items = append(result.Items, evaluationSummaryWire(item))
		}
	}
	return result
}

func evaluationSummaryWire(value *appevaluation.ReviewRun) AIExplanationEvaluationSummaryWire {
	return AIExplanationEvaluationSummaryWire{
		RunID: value.RunID.String(), Version: value.Version, Status: string(value.Status),
		RequestedOrgID: value.RequestedOrgID, RequestedBy: value.RequestedBy,
		RequestReason: value.RequestReason, CreatedAt: value.CreatedAt,
		Release: releaseWire(value.Release), Progress: reviewProgressWire(value.Progress), Gate: gateWire(value.Gate),
		CanReview: value.CanReview, CanFinalize: value.CanFinalize, CanCancel: value.CanCancel,
		RecoveryMaxProviderInvocations: value.RecoveryMaxProviderInvocations,
	}
}

func evaluationCapacityWire(value *aiexplanationadministration.EvaluationCapacity) AIExplanationEvaluationCapacityWire {
	result := AIExplanationEvaluationCapacityWire{
		OrganizationID: value.OrgID, BudgetDay: value.BudgetDay, MaxActiveRunsPerOrg: value.MaxActiveRunsPerOrg,
		ProviderInvocationsPerStart:  value.ProviderInvocationsPerStart,
		DailyProviderInvocationLimit: value.DailyProviderInvocationLimit,
		ReservedProviderInvocations:  value.ReservedProviderInvocations,
		RemainingProviderInvocations: value.RemainingProviderInvocations,
		AvailableFullRunStarts:       value.AvailableFullRunStarts, OverLimit: value.OverLimit,
		Reservations: make([]AIExplanationCapacityReservationWire, 0, len(value.Reservations)),
	}
	for _, reservation := range value.Reservations {
		result.Reservations = append(result.Reservations, AIExplanationCapacityReservationWire{
			RunID: reservation.RunID.String(), RequestedBy: reservation.RequestedBy,
			ProviderInvocations: reservation.ProviderInvocations, ReservedAt: reservation.ReservedAt,
		})
	}
	return result
}

func participantCapacityWire(value *aiexplanationadministration.ParticipantCapacity) AIExplanationParticipantCapacityWire {
	result := AIExplanationParticipantCapacityWire{
		OrganizationID: value.OrgID, BudgetDay: value.BudgetDay,
		ProviderInvocationsPerGeneration:          value.ProviderInvocationsPerGeneration,
		DailyProviderInvocationLimitPerOrg:        value.DailyProviderInvocationLimitPerOrg,
		DailyProviderInvocationLimitPerUser:       value.DailyProviderInvocationLimitPerUser,
		DailyProviderInvocationLimitPerAssessment: value.DailyProviderInvocationLimitPerAssessment,
		MaxActiveProviderExecutionsPerOrg:         value.MaxActiveProviderExecutionsPerOrg,
		MaxActiveProviderExecutionsPerUser:        value.MaxActiveProviderExecutionsPerUser,
		MaxActiveProviderExecutionsPerAssessment:  value.MaxActiveProviderExecutionsPerAssessment,
		ReservedProviderInvocations:               value.ReservedProviderInvocations,
		RedactedProviderInvocations:               value.RedactedProviderInvocations,
		RemainingOrgProviderInvocations:           value.RemainingOrgProviderInvocations,
		OverOrgLimit:                              value.OverOrgLimit,
		Reservations:                              make([]AIExplanationParticipantCapacityReservationWire, 0, len(value.Reservations)),
		ActiveProviderExecutions:                  value.ActiveProviderExecutions,
		RemainingOrgActiveProviderExecutions:      value.RemainingOrgActiveProviderExecutions,
		OverOrgActiveLimit:                        value.OverOrgActiveLimit,
		ActiveReservations:                        make([]AIExplanationParticipantActiveCapacityReservationWire, 0, len(value.ActiveReservations)),
	}
	for _, reservation := range value.Reservations {
		result.Reservations = append(result.Reservations, AIExplanationParticipantCapacityReservationWire{
			ReservationID: reservation.ReservationID, GenerationID: reservation.GenerationID.String(),
			Attempt: reservation.Attempt, Origin: string(reservation.Origin),
			UserID: reservation.UserID, AssessmentID: reservation.AssessmentID.String(),
			ProviderInvocations: reservation.ProviderInvocations, ReservedAt: reservation.ReservedAt,
		})
	}
	for _, reservation := range value.ActiveReservations {
		result.ActiveReservations = append(result.ActiveReservations, AIExplanationParticipantActiveCapacityReservationWire{
			GenerationID: reservation.GenerationID.String(), RunID: reservation.RunID.String(), UserID: reservation.UserID,
			AssessmentID: reservation.AssessmentID.String(), AcquiredAt: reservation.AcquiredAt,
		})
	}
	return result
}

func participantRetryWire(value *apprecovery.Result) AIExplanationParticipantRetryWire {
	if value == nil || value.Generation == nil || value.FailedRun == nil {
		return AIExplanationParticipantRetryWire{}
	}
	authorization := value.Authorization
	return AIExplanationParticipantRetryWire{
		GenerationID: value.Generation.ID().String(), FailedRunID: value.FailedRun.ID().String(),
		ExpectedAttempt: authorization.ExpectedAttempt, NextAttempt: authorization.NextAttempt,
		Origin: string(authorization.Origin), RequestID: authorization.RequestID, AuthorizedAt: authorization.AuthorizedAt,
		AcceptedResultUnknownRisk: authorization.AcceptedResultUnknownRisk, Created: value.Created,
	}
}

func reviewAttemptSummary(value appevaluation.ReviewAttempt) AIExplanationReviewAttemptSummary {
	reviews := make([]AIExplanationHumanReviewWire, 0, len(value.Reviews))
	for _, review := range value.Reviews {
		reviews = append(reviews, AIExplanationHumanReviewWire{Role: string(review.Role), Reviewer: review.Reviewer, Decision: string(review.Decision), ReviewedAt: review.ReviewedAt, Reason: review.Reason})
	}
	missing := make([]string, 0, len(value.MissingRoles))
	for _, role := range value.MissingRoles {
		missing = append(missing, string(role))
	}
	result := AIExplanationReviewAttemptSummary{CaseID: value.CaseID, Attempt: value.Attempt, Reviews: reviews, MissingRoles: missing}
	if len(value.NormalizedOutput) > 0 {
		result.OutputFingerprint = aiexplanation.NewFingerprint(value.NormalizedOutput).String()
	}
	result.Failure = attemptFailureWire(value.Failure)
	if value.Semantic != nil {
		scores := semanticScoresWire(value.Semantic.Scores)
		result.SemanticScores = &scores
	}
	return result
}

func reviewAttemptWire(value appevaluation.ReviewAttempt) AIExplanationReviewAttemptWire {
	result := AIExplanationReviewAttemptWire{
		AIExplanationReviewAttemptSummary: reviewAttemptSummary(value),
		AssessmentInput:                   append(json.RawMessage(nil), value.AssessmentInput...), RawProviderOutput: string(value.RawProviderOutput),
		NormalizedOutput: append(json.RawMessage(nil), value.NormalizedOutput...),
		ProviderReceipt:  providerReceiptWire(value.ProviderReceipt),
		Assertions:       make([]AIExplanationAssertionReceiptWire, 0, len(value.Assertions)),
	}
	for _, assertion := range value.Assertions {
		result.Assertions = append(result.Assertions, AIExplanationAssertionReceiptWire{Type: assertion.Type, Scope: string(assertion.Scope), Ordinal: assertion.Ordinal, Hard: assertion.Hard, Evaluator: assertion.Evaluator, Status: string(assertion.Status), Detail: assertion.Detail})
	}
	if value.Semantic != nil {
		result.Semantic = &AIExplanationSemanticReceiptWire{
			EvaluatorVersion: value.Semantic.EvaluatorVersion, ProviderReceipt: *providerReceiptWire(&value.Semantic.ProviderReceipt),
			Scores: semanticScoresWire(value.Semantic.Scores), Rationale: value.Semantic.Rationale,
		}
	}
	return result
}

func releaseWire(value domainevaluation.ReleaseIdentity) AIExplanationEvaluationReleaseWire {
	return AIExplanationEvaluationReleaseWire{
		Suite:  AIExplanationSuiteRefWire{ID: value.Suite.ID, Version: value.Suite.Version, Fingerprint: value.Suite.Fingerprint.String(), GitBlobSHA: value.Suite.GitBlobSHA},
		Prompt: promptRefWire(value.Prompt), Profile: AIExplanationProfileRefWire{ID: value.Profile.ID, Version: value.Profile.Version, Fingerprint: value.Profile.Fingerprint.String()},
		InputSchema: schemaRefWire(value.InputSchema), OutputSchema: schemaRefWire(value.OutputSchema), Provider: providerSpecWire(value.Provider), Decoding: decodingWire(value.Decoding),
		SemanticEvaluator: AIExplanationSemanticEvaluatorWire{Version: value.SemanticEvaluator.Version, Prompt: promptRefWire(value.SemanticEvaluator.Prompt), OutputSchema: schemaRefWire(value.SemanticEvaluator.OutputSchema), Provider: providerSpecWire(value.SemanticEvaluator.Provider), Decoding: decodingWire(value.SemanticEvaluator.Decoding)},
		GenerationCaseIDs: append([]string(nil), value.GenerationCaseIDs...), PreflightCaseID: value.PreflightCaseID,
		PreflightRejectionReason: value.PreflightRejectionReason, RepetitionsPerCase: value.RepetitionsPerCase,
	}
}

func promptRefWire(value aiexplanation.PromptRef) AIExplanationPromptRefWire {
	return AIExplanationPromptRefWire{TemplateID: value.TemplateID, Version: value.Version, Fingerprint: value.Fingerprint.String(), GitBlobSHA: value.GitBlobSHA}
}

func schemaRefWire(value domainevaluation.SchemaRef) AIExplanationSchemaRefWire {
	return AIExplanationSchemaRefWire{Version: value.Version, Fingerprint: value.Fingerprint.String()}
}

func providerSpecWire(value aiexplanation.ProviderExecutionSpec) AIExplanationProviderSpecWire {
	return AIExplanationProviderSpecWire{Route: value.Route, RouteRevision: value.RouteRevision, ResolvedProvider: value.ResolvedProvider, ResolvedModel: value.ResolvedModel, Fingerprint: value.Fingerprint.String()}
}

func decodingWire(value domainevaluation.DecodingParameters) AIExplanationDecodingWire {
	return AIExplanationDecodingWire{
		MaxOutputTokens: value.MaxOutputTokens, Temperature: value.Temperature,
		TopP: value.TopP, Seed: value.Seed, ReasoningEffort: value.ReasoningEffort,
	}
}

func reviewProgressWire(value appevaluation.ReviewProgress) AIExplanationReviewProgressWire {
	pending := domainevaluation.RequiredGenerationAttempts - value.GenerationAttempts
	if pending < 0 {
		pending = 0
	}
	return AIExplanationReviewProgressWire{PlannedGenerationAttempts: domainevaluation.RequiredGenerationAttempts, GenerationAttempts: value.GenerationAttempts, FailedAttempts: value.FailedAttempts, PendingGenerationAttempts: pending, RequiredReviews: value.RequiredReviews, RecordedReviews: value.RecordedReviews, MissingReviews: value.MissingReviews, FullyReviewedAttempts: value.FullyReviewedAttempts, RejectedReviews: value.RejectedReviews, AllRequiredReviewsRecorded: value.AllRequiredReviewsRecorded}
}

func providerReceiptWire(value *aiexplanation.ProviderReceipt) *AIExplanationProviderReceiptWire {
	if value == nil {
		return nil
	}
	return &AIExplanationProviderReceiptWire{InvocationID: value.InvocationID, RequestID: value.RequestID, Provider: value.Provider, Model: value.Model, InputTokens: value.InputTokens, OutputTokens: value.OutputTokens, LatencyMS: value.Latency.Milliseconds()}
}

func attemptFailureWire(value *domainevaluation.AttemptFailure) *AIExplanationAttemptFailureWire {
	if value == nil {
		return nil
	}
	return &AIExplanationAttemptFailureWire{Stage: value.Stage, Code: value.Code, SafeMessage: value.SafeMessage, Retryable: value.Retryable, ResultUnknown: value.ResultUnknown}
}

func semanticScoresWire(value domainevaluation.SemanticScores) AIExplanationSemanticScoresWire {
	return AIExplanationSemanticScoresWire{Faithfulness: value.Faithfulness, CrossDimensionQuality: value.CrossDimensionQuality, SuggestionActionability: value.SuggestionActionability, AudienceClarity: value.AudienceClarity, Concision: value.Concision}
}

func gateWire(value *domainevaluation.GateResult) *AIExplanationGateWire {
	if value == nil {
		return nil
	}
	reasons := make([]AIExplanationGateReasonWire, 0, len(value.Reasons))
	for _, reason := range value.Reasons {
		reasons = append(reasons, AIExplanationGateReasonWire{Code: reason.Code, CaseID: reason.CaseID, Attempt: reason.Attempt, Detail: reason.Detail})
	}
	m := value.Metrics
	return &AIExplanationGateWire{Passed: value.Passed, Reasons: reasons, Metrics: AIExplanationQualityMetricsWire{GenerationAttempts: m.GenerationAttempts, CaseAssertionPasses: m.CaseAssertionPasses, FaithfulnessAverage: m.FaithfulnessAverage, CrossDimensionAverage: m.CrossDimensionAverage, ActionabilityAverage: m.ActionabilityAverage, AudienceClarityAverage: m.AudienceClarityAverage, ConcisionAverage: m.ConcisionAverage, HumanReviews: m.HumanReviews}}
}

func profileWire(value *domainprofile.AIExplanationProfile) AIExplanationProfileWire {
	evidenceID := ""
	if !value.PublishedEvidenceRunID().IsZero() {
		evidenceID = value.PublishedEvidenceRunID().String()
	}
	return AIExplanationProfileWire{
		ID: value.ID().String(), Definition: value.Definition(), Fingerprint: value.Fingerprint().String(), Status: string(value.Status()),
		CreatedAt: value.CreatedAt(), CreatedBy: value.CreatedBy(), CreatedReason: value.CreatedReason(), UpdatedAt: value.UpdatedAt(),
		PublishedAt: value.PublishedAt(), PublishedBy: value.PublishedBy(), PublishedReason: value.PublishedReason(), PublishedEvidenceRunID: evidenceID,
		DisabledAt: value.DisabledAt(), DisabledBy: value.DisabledBy(), DisabledReason: value.DisabledReason(),
	}
}

func profilePageWire(value *appgovernance.ProfilePage) AIExplanationProfilePageWire {
	result := AIExplanationProfilePageWire{Items: []AIExplanationProfileWire{}}
	if value == nil {
		return result
	}
	result.NextCursor = value.NextCursor
	result.Items = make([]AIExplanationProfileWire, 0, len(value.Items))
	for _, item := range value.Items {
		if item != nil {
			result.Items = append(result.Items, profileWire(item))
		}
	}
	return result
}

func optionalEvaluationStatus(raw string) (*domainevaluation.Status, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value := domainevaluation.Status(strings.TrimSpace(raw))
	if !value.IsValid() {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation evaluation status is invalid")
	}
	return &value, nil
}

func optionalProfileStatus(raw string) (*domainprofile.Status, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value := domainprofile.Status(strings.TrimSpace(raw))
	if !value.IsValid() {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation Profile status is invalid")
	}
	return &value, nil
}

func administrationCatalogLimit(c *gin.Context, fallback, maximum int) (int, error) {
	raw := strings.TrimSpace(c.DefaultQuery("limit", fmt.Sprint(fallback)))
	var result int
	if _, err := fmt.Sscan(raw, &result); err != nil || result < 1 || result > maximum || fmt.Sprint(result) != raw {
		return 0, cberrors.WithCode(code.ErrInvalidArgument, "AI explanation administration page limit is invalid")
	}
	return result, nil
}

func parsePositiveInt(value string) (int, error) {
	var result int
	if _, err := fmt.Sscan(strings.TrimSpace(value), &result); err != nil || result < 1 || fmt.Sprint(result) != strings.TrimSpace(value) {
		return 0, cberrors.WithCode(code.ErrInvalidArgument, "attempt is invalid")
	}
	return result, nil
}
