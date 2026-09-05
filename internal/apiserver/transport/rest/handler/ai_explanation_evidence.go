package handler

import (
	"encoding/json"
	"net/http"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	aiexplanationadministration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainai "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

type AIExplanationEvaluationV2ReviewRequest struct {
	SemanticReview *domainevaluation.SemanticContradictionReview `json:"semantic_review,omitempty"`
	CandidateID    string                                        `json:"candidate_id" binding:"required"`
	Role           string                                        `json:"role" binding:"required"`
	Decision       string                                        `json:"decision" binding:"required"`
	Reason         string                                        `json:"reason" binding:"required,max=1000"`
}

type AIExplanationEvaluationV2BatchReviewRequest struct {
	Role    string                                            `json:"role" binding:"required"`
	Reviews []AIExplanationEvaluationV2BatchReviewItemRequest `json:"reviews" binding:"required,min=1,max=35,dive"`
}

type AIExplanationEvaluationV2BatchReviewItemRequest struct {
	SemanticReview *domainevaluation.SemanticContradictionReview `json:"semantic_review,omitempty"`
	CandidateID    string                                        `json:"candidate_id" binding:"required"`
	Decision       string                                        `json:"decision" binding:"required"`
	Reason         string                                        `json:"reason" binding:"required,max=1000"`
}

type AIExplanationResultUnknownV2Request struct {
	ExecutionID                          string `json:"execution_id" binding:"required"`
	Decision                             string `json:"decision" binding:"required"`
	Confirm                              bool   `json:"confirm" binding:"required"`
	AcknowledgedDuplicateCallAndCostRisk bool   `json:"acknowledged_duplicate_call_and_cost_risk" binding:"required"`
	Reason                               string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationEvaluationV2TransitionWire struct {
	From           *domainevaluation.EvidenceStatus `json:"from"`
	To             domainevaluation.EvidenceStatus  `json:"to"`
	CauseCode      string                           `json:"cause_code"`
	Reason         string                           `json:"reason,omitempty"`
	Actor          string                           `json:"actor"`
	TransitionedAt time.Time                        `json:"transitioned_at"`
	EvidenceRefs   []string                         `json:"evidence_refs"`
}

type AIExplanationEvaluationV2Wire struct {
	ReviewReopenings []domainevaluation.ReviewReopening `json:"review_reopenings,omitempty"`
	CanReopenReview  bool                               `json:"can_reopen_review"`
	GatePreview      *AIExplanationEvaluationV2GateWire `json:"gate_preview,omitempty"`

	CanceledAt                   *time.Time                                 `json:"canceled_at,omitempty"`
	CanCancel                    bool                                       `json:"can_cancel"`
	CanDiscard                   bool                                       `json:"can_discard"`
	StateTransitions             []AIExplanationEvaluationV2TransitionWire  `json:"state_transitions"`
	SchemaVersion                string                                     `json:"schema_version"`
	RunID                        string                                     `json:"run_id"`
	Version                      int64                                      `json:"version"`
	Status                       string                                     `json:"status"`
	OrganizationID               int64                                      `json:"organization_id"`
	RequestedBy                  string                                     `json:"requested_by"`
	RequestReason                string                                     `json:"request_reason"`
	CreatedAt                    time.Time                                  `json:"created_at"`
	ClosedAt                     *time.Time                                 `json:"closed_at,omitempty"`
	FinalizedAt                  *time.Time                                 `json:"finalized_at,omitempty"`
	ReleaseFingerprint           string                                     `json:"release_fingerprint"`
	Release                      AIExplanationEvaluationV2ReleaseWire       `json:"release"`
	ExecutionPolicyID            string                                     `json:"execution_policy_id"`
	ExecutionPolicyVersion       string                                     `json:"execution_policy_version"`
	GatePolicyID                 string                                     `json:"gate_policy_id"`
	GatePolicyVersion            string                                     `json:"gate_policy_version"`
	ReservedProviderInvocations  int                                        `json:"reserved_provider_invocations"`
	RequiredCandidates           int                                        `json:"required_candidates"`
	AcceptedCandidates           int                                        `json:"accepted_candidates"`
	ReviewReadyCandidates        int                                        `json:"review_ready_candidates"`
	UnresolvedResultUnknownCount int                                        `json:"unresolved_result_unknown_count"`
	Execution                    *AIExplanationEvaluationV2CheckpointWire   `json:"execution,omitempty"`
	Slots                        []AIExplanationEvaluationV2SlotWire        `json:"slots"`
	GenerationExecutions         []AIExplanationEvaluationV2ExecutionWire   `json:"generation_executions"`
	SemanticExecutions           []AIExplanationEvaluationV2ExecutionWire   `json:"semantic_executions"`
	HumanReviews                 []AIExplanationEvaluationV2HumanReviewWire `json:"human_reviews"`
	ResultUnknownResolutions     []AIExplanationResultUnknownResolutionWire `json:"result_unknown_resolutions"`
	Gate                         *AIExplanationEvaluationV2GateWire         `json:"gate,omitempty"`
}

type AIExplanationEvaluationV2FrozenRefWire struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

type AIExplanationEvaluationV2ReleaseWire struct {
	Fingerprint          string                                 `json:"fingerprint"`
	Suite                AIExplanationEvaluationV2FrozenRefWire `json:"suite"`
	Prompt               AIExplanationEvaluationV2FrozenRefWire `json:"prompt"`
	Profile              AIExplanationEvaluationV2FrozenRefWire `json:"profile"`
	InputSchema          AIExplanationEvaluationV2FrozenRefWire `json:"input_schema"`
	OutputSchema         AIExplanationEvaluationV2FrozenRefWire `json:"output_schema"`
	GenerationRoute      AIExplanationEvaluationV2FrozenRefWire `json:"generation_route"`
	SemanticPrompt       AIExplanationEvaluationV2FrozenRefWire `json:"semantic_prompt"`
	SemanticOutputSchema AIExplanationEvaluationV2FrozenRefWire `json:"semantic_output_schema"`
	SemanticRoute        AIExplanationEvaluationV2FrozenRefWire `json:"semantic_route"`
	ExecutionPolicy      AIExplanationEvaluationV2FrozenRefWire `json:"execution_policy"`
	GatePolicy           AIExplanationEvaluationV2FrozenRefWire `json:"gate_policy"`
}

type AIExplanationEvaluationV2CheckpointWire struct {
	ID                string     `json:"id"`
	Kind              string     `json:"kind"`
	CaseID            string     `json:"case_id"`
	SlotOrdinal       int        `json:"slot_ordinal"`
	CandidateID       string     `json:"candidate_id,omitempty"`
	ExecutionOrdinal  int        `json:"execution_ordinal"`
	Phase             string     `json:"phase"`
	ClaimedAt         time.Time  `json:"claimed_at"`
	LeaseExpiresAt    time.Time  `json:"lease_expires_at"`
	DispatchStartedAt *time.Time `json:"dispatch_started_at,omitempty"`
}

type AIExplanationEvaluationV2SlotWire struct {
	CaseID                 string                                  `json:"case_id"`
	SlotOrdinal            int                                     `json:"slot_ordinal"`
	Status                 string                                  `json:"status"`
	GenerationExecutionIDs []string                                `json:"generation_execution_ids"`
	Candidate              *AIExplanationEvaluationV2CandidateWire `json:"candidate,omitempty"`
}

type AIExplanationEvaluationV2CandidateWire struct {
	CandidateID                 string                                   `json:"candidate_id"`
	GenerationExecutionID       string                                   `json:"generation_execution_id"`
	NormalizedOutputFingerprint string                                   `json:"normalized_output_fingerprint"`
	AcceptedAt                  time.Time                                `json:"accepted_at"`
	SemanticExecutionIDs        []string                                 `json:"semantic_execution_ids"`
	AcceptedSemanticExecutionID string                                   `json:"accepted_semantic_execution_id,omitempty"`
	ReviewReady                 bool                                     `json:"review_ready"`
	Assertions                  []AIExplanationEvaluationV2AssertionWire `json:"assertions"`
}

type AIExplanationEvaluationV2AssertionWire struct {
	Type      string `json:"type"`
	Scope     string `json:"scope"`
	Ordinal   int    `json:"ordinal"`
	Hard      bool   `json:"hard"`
	Evaluator string `json:"evaluator"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}

type AIExplanationEvaluationV2ExecutionWire struct {
	ExecutionID            string                                 `json:"execution_id"`
	Kind                   string                                 `json:"kind"`
	CaseID                 string                                 `json:"case_id,omitempty"`
	SlotOrdinal            int                                    `json:"slot_ordinal,omitempty"`
	CandidateID            string                                 `json:"candidate_id,omitempty"`
	ExecutionOrdinal       int                                    `json:"execution_ordinal"`
	InvocationID           string                                 `json:"invocation_id"`
	Status                 string                                 `json:"status"`
	StartedAt              time.Time                              `json:"started_at"`
	FinishedAt             *time.Time                             `json:"finished_at,omitempty"`
	ProviderCallCount      int                                    `json:"provider_call_count"`
	ProviderReceiptPresent bool                                   `json:"provider_receipt_present"`
	ProviderReceipt        *AIExplanationProviderReceiptWire      `json:"provider_receipt,omitempty"`
	RawOutputBytes         int                                    `json:"raw_output_bytes"`
	NormalizedBytes        int                                    `json:"normalized_output_bytes"`
	Failure                *AIExplanationEvaluationV2FailureWire  `json:"failure,omitempty"`
	SemanticResult         *AIExplanationEvaluationV2SemanticWire `json:"semantic_result,omitempty"`
}

type AIExplanationEvaluationV2FailureWire struct {
	ProviderDiagnostics *domainai.ProviderFailureDiagnostics `json:"provider_diagnostics,omitempty"`
	Stage               string                               `json:"stage"`
	Kind                string                               `json:"kind"`
	Code                string                               `json:"code"`
	Retryable           bool                                 `json:"retryable"`
	ResultUnknown       bool                                 `json:"result_unknown"`
	Disposition         string                               `json:"disposition"`
	SafeMessage         string                               `json:"safe_message"`
	EvidenceRefs        []string                             `json:"evidence_refs"`
}

type AIExplanationEvaluationV2SemanticWire struct {
	EvaluatorVersion  string                                          `json:"evaluator_version"`
	Scores            AIExplanationEvaluationV2SemanticScoresWire     `json:"scores"`
	Rationale         string                                          `json:"rationale"`
	Decisions         []AIExplanationEvaluationV2SemanticDecisionWire `json:"decisions"`
	OutputFingerprint string                                          `json:"output_fingerprint"`
}

type AIExplanationEvaluationV2SemanticScoresWire struct {
	Faithfulness            int `json:"faithfulness"`
	CrossDimensionQuality   int `json:"cross_dimension_quality"`
	SuggestionActionability int `json:"suggestion_actionability"`
	AudienceClarity         int `json:"audience_clarity"`
	Concision               int `json:"concision"`
}

type AIExplanationEvaluationV2SemanticDecisionWire struct {
	Type    string `json:"type"`
	Scope   string `json:"scope"`
	Ordinal int    `json:"ordinal"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

type AIExplanationEvaluationV2HumanReviewWire struct {
	SemanticReview *domainevaluation.SemanticContradictionReview `json:"semantic_review,omitempty"`
	CandidateID    string                                        `json:"candidate_id"`
	Role           string                                        `json:"role"`
	Reviewer       string                                        `json:"reviewer"`
	Decision       string                                        `json:"decision"`
	ReviewedAt     time.Time                                     `json:"reviewed_at"`
	Reason         string                                        `json:"reason"`
}

type AIExplanationResultUnknownResolutionWire struct {
	ExecutionID                          string    `json:"execution_id"`
	Decision                             string    `json:"decision"`
	Actor                                string    `json:"actor"`
	Reason                               string    `json:"reason"`
	AcknowledgedDuplicateCallAndCostRisk bool      `json:"acknowledged_duplicate_call_and_cost_risk"`
	ResolvedAt                           time.Time `json:"resolved_at"`
}

type AIExplanationEvaluationV2GateWire struct {
	SemanticAdjudications []domainevaluation.SemanticAdjudicationRecord `json:"semantic_adjudications,omitempty"`
	EvaluatedAt           time.Time                                     `json:"evaluated_at"`
	Passed                bool                                          `json:"passed"`
	GatePasses            map[string]bool                               `json:"gate_passes"`
	Reasons               []AIExplanationEvaluationV2GateReasonWire     `json:"reasons"`
}

type AIExplanationEvaluationV2GateReasonWire struct {
	Gate         string   `json:"gate"`
	Code         string   `json:"code"`
	Detail       string   `json:"detail"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type AIExplanationEvaluationV2OutputWire struct {
	Failure          *AIExplanationEvaluationV2FailureWire `json:"failure,omitempty"`
	ExecutionID      string                                `json:"execution_id"`
	Kind             string                                `json:"kind"`
	RawOutput        string                                `json:"raw_output"`
	NormalizedOutput string                                `json:"normalized_output"`
	ProviderReceipt  *AIExplanationProviderReceiptWire     `json:"provider_receipt,omitempty"`
}

type AIExplanationEvaluationV2ExecutionEvidenceWire struct {
	AIExplanationEvaluationV2ExecutionWire
	RawOutput        string `json:"raw_output"`
	NormalizedOutput string `json:"normalized_output"`
}

type AIExplanationEvaluationV2CandidateEvidenceWire struct {
	RunID                       string                                          `json:"run_id"`
	CaseID                      string                                          `json:"case_id"`
	SlotOrdinal                 int                                             `json:"slot_ordinal"`
	AssessmentInput             json.RawMessage                                 `json:"assessment_input" swaggertype:"object"`
	Candidate                   AIExplanationEvaluationV2CandidateWire          `json:"candidate"`
	AcceptedGenerationExecution AIExplanationEvaluationV2ExecutionEvidenceWire  `json:"accepted_generation_execution"`
	AcceptedSemanticExecution   *AIExplanationEvaluationV2ExecutionEvidenceWire `json:"accepted_semantic_execution,omitempty"`
	HumanReviews                []AIExplanationEvaluationV2HumanReviewWire      `json:"human_reviews"`
}

// StartEvaluationV2 godoc
// @Summary 启动 v2 AI 解读 Prompt 评测
// @Description 冻结当前 execution/gate policy，确认最坏 140 次 Provider 调用，并在一个事务中预留容量、写 Evidence 和首个 Outbox 事件。
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param request body AIExplanationEvaluationStartRequest true "成本确认与审计理由"
// @Success 202 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 400 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Failure 429 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations [post]
func (h *AIExplanationAdministrationHandler) StartEvaluationV2(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request AIExplanationEvaluationStartRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	value, err := h.service.StartEvaluationV2(c.Request.Context(), actor, aiexplanationadministration.StartEvaluationV2Command{
		Confirm: request.Confirm, ExpectedProviderInvocations: request.ExpectedProviderInvocations, Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, core.Response{Code: 0, Message: "accepted", Data: evaluationV2Wire(value)})
}

// FindEvaluationV2 godoc
// @Summary 查询 v2 AI 解读 Prompt 评测证据摘要
// @Tags AI-Explanation-Administration
// @Produce json
// @Param run_id path string true "评测 Run ID"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 404 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id} [get]
func (h *AIExplanationAdministrationHandler) FindEvaluationV2(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	value, err := h.service.FindEvaluationV2(c.Request.Context(), actor, runID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationV2Wire(value))
}

// FindEvaluationV2Candidate godoc
// @Summary 查询一个 v2 Candidate 的人工审核证据
// @Description 返回冻结用例输入、已接受的生成与语义执行完整输出和 Provider 收据，以及该 Candidate 的人工审核记录。
// @Tags AI-Explanation-Administration
// @Produce json
// @Param run_id path string true "评测 Run ID"
// @Param candidate_id path string true "Candidate ID"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2CandidateEvidenceWire}
// @Failure 404 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/candidates/{candidate_id} [get]
func (h *AIExplanationAdministrationHandler) FindEvaluationV2Candidate(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	value, err := h.service.FindEvaluationV2(c.Request.Context(), actor, runID)
	if err != nil {
		h.Error(c, err)
		return
	}
	wire, err := evaluationV2CandidateEvidenceWire(value, c.Param("candidate_id"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, wire)
}

// FindEvaluationV2Output godoc
// @Summary 查询一条 v2 执行的 Provider 输出证据
// @Description 仅详情接口返回 raw/normalized output；Run 摘要只返回字节数和收据存在性。
// @Tags AI-Explanation-Administration
// @Produce json
// @Param run_id path string true "评测 Run ID"
// @Param execution_id path string true "执行 ID"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2OutputWire}
// @Failure 404 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/executions/{execution_id}/output [get]
func (h *AIExplanationAdministrationHandler) FindEvaluationV2Output(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	value, err := h.service.FindEvaluationV2(c.Request.Context(), actor, runID)
	if err != nil {
		h.Error(c, err)
		return
	}
	executionID := c.Param("execution_id")
	for _, execution := range value.GenerationExecutions {
		if execution.ID == executionID {
			h.Success(c, AIExplanationEvaluationV2OutputWire{ExecutionID: execution.ID, Kind: "generation", RawOutput: string(execution.RawOutput), NormalizedOutput: string(execution.NormalizedOutput), ProviderReceipt: providerReceiptWire(execution.ProviderReceipt), Failure: failureV2Wire(execution.Failure)})
			return
		}
	}
	for _, execution := range value.SemanticExecutions {
		if execution.ID == executionID {
			h.Success(c, AIExplanationEvaluationV2OutputWire{ExecutionID: execution.ID, Kind: "semantic", RawOutput: string(execution.RawOutput), NormalizedOutput: string(execution.NormalizedOutput), ProviderReceipt: providerReceiptWire(execution.ProviderReceipt), Failure: failureV2Wire(execution.Failure)})
			return
		}
	}
	h.Error(c, cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation v2 execution not found"))
}

// RecordReviewV2 godoc
// @Summary 记录一个 v2 Candidate 的人工审核
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run ID"
// @Param request body AIExplanationEvaluationV2ReviewRequest true "Candidate、角色、决定与理由"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 400 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/reviews [post]
func (h *AIExplanationAdministrationHandler) RecordReviewV2(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationEvaluationV2ReviewRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	value, err := h.service.RecordReviewV2(c.Request.Context(), actor, runID, aiexplanationadministration.ReviewV2Command{
		CandidateID: request.CandidateID, Role: domainevaluation.ReviewRole(request.Role),
		Decision: domainevaluation.ReviewDecision(request.Decision), Reason: request.Reason, SemanticReview: request.SemanticReview,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationV2Wire(value))
}

// RecordReviewsV2 godoc
// @Summary 原子记录一批 v2 Candidate 人工审核
// @Description 一批仅允许一个审核角色，最多 35 条；任一条不合法时整批不写入。
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run ID"
// @Param request body AIExplanationEvaluationV2BatchReviewRequest true "单角色 Candidate 审核列表"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 400 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/reviews/batch [post]
func (h *AIExplanationAdministrationHandler) RecordReviewsV2(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationEvaluationV2BatchReviewRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	reviews := make([]aiexplanationadministration.ReviewV2BatchItemCommand, 0, len(request.Reviews))
	for _, item := range request.Reviews {
		reviews = append(reviews, aiexplanationadministration.ReviewV2BatchItemCommand{
			CandidateID: item.CandidateID, Decision: domainevaluation.ReviewDecision(item.Decision), Reason: item.Reason, SemanticReview: item.SemanticReview,
		})
	}
	value, err := h.service.RecordReviewsV2(c.Request.Context(), actor, runID, aiexplanationadministration.ReviewV2BatchCommand{
		Role: domainevaluation.ReviewRole(request.Role), Reviews: reviews,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationV2Wire(value))
}

// FinalizeEvaluationV2 godoc
// @Summary 按冻结 G1-G5 Policy 终审 v2 Evidence
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run ID"
// @Param request body AIExplanationFinalizeV2CheckedRequest true "终审理由和预检查结果"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 400 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/finalize [post]
func (h *AIExplanationAdministrationHandler) FinalizeEvaluationV2(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationFinalizeV2CheckedRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.Error(c, cberrors.WithCode(code.ErrInvalidArgument, "expected_version, expected_passed and reason are required"))
		return
	}
	value, err := h.service.FinalizeEvaluationV2Checked(c.Request.Context(), actor, runID, request.Reason, *request.ExpectedVersion, *request.ExpectedPassed)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationV2Wire(value))
}

// ResolveResultUnknownV2 godoc
// @Summary 人工处理 v2 result_unknown
// @Description 授权替换执行或取消 Run；两种决定都必须确认潜在重复调用与计费风险。授权替换与下一 Outbox 事件原子提交。
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run ID"
// @Param request body AIExplanationResultUnknownV2Request true "未知结果决定与风险确认"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 400 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/result-unknown/resolve [post]
func (h *AIExplanationAdministrationHandler) ResolveResultUnknownV2(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationResultUnknownV2Request
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	value, err := h.service.ResolveResultUnknownV2(c.Request.Context(), actor, runID, aiexplanationadministration.ResolveResultUnknownV2Command{
		ExecutionID: request.ExecutionID, Decision: domainevaluation.ResultUnknownResolutionDecision(request.Decision),
		Confirm: request.Confirm, AcknowledgedDuplicateCallAndCostRisk: request.AcknowledgedDuplicateCallAndCostRisk, Reason: request.Reason,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationV2Wire(value))
}

func evaluationV2Wire(value *domainevaluation.PromptEvaluationEvidenceV2) *AIExplanationEvaluationV2Wire {
	if value == nil {
		return nil
	}
	result := &AIExplanationEvaluationV2Wire{
		SchemaVersion: value.SchemaVersion, RunID: value.RunID.String(), Version: value.Version(), Status: string(value.Status),
		OrganizationID: value.Audit.OrganizationID, RequestedBy: value.Audit.RequestedBy, RequestReason: value.Audit.RequestReason,
		CreatedAt: value.Audit.CreatedAt, ClosedAt: value.Audit.ClosedAt, FinalizedAt: value.Audit.FinalizedAt,
		ReleaseFingerprint: string(value.Release.Fingerprint), Release: evaluationV2ReleaseWire(value.Release), ExecutionPolicyID: value.ExecutionPolicy.PolicyID,
		ExecutionPolicyVersion: value.ExecutionPolicy.Version, GatePolicyID: value.GatePolicy.PolicyID, GatePolicyVersion: value.GatePolicy.Version,
		ReservedProviderInvocations: value.ExecutionPolicy.WorstCaseProviderCalls(), RequiredCandidates: value.ExecutionPolicy.RequiredCandidateCount(),
		UnresolvedResultUnknownCount: value.UnresolvedResultUnknownCount,
		Slots:                        make([]AIExplanationEvaluationV2SlotWire, 0, len(value.Slots)),
		GenerationExecutions:         make([]AIExplanationEvaluationV2ExecutionWire, 0, len(value.GenerationExecutions)),
		SemanticExecutions:           make([]AIExplanationEvaluationV2ExecutionWire, 0, len(value.SemanticExecutions)),
		HumanReviews:                 make([]AIExplanationEvaluationV2HumanReviewWire, 0, len(value.HumanReviews)),
		ResultUnknownResolutions:     make([]AIExplanationResultUnknownResolutionWire, 0, len(value.ResultUnknownResolutions)),
	}
	result.CanceledAt = value.Audit.CanceledAt
	checkpoint := value.Execution()
	eligible := !value.Status.IsTerminal() && value.UnresolvedResultUnknownCount == 0 && (checkpoint == nil || checkpoint.Phase == domainevaluation.AttemptExecutionPrepared)
	result.CanDiscard = eligible && value.Status == domainevaluation.EvidenceStatusAwaitingReview
	result.CanCancel = eligible && !result.CanDiscard
	result.StateTransitions = make([]AIExplanationEvaluationV2TransitionWire, 0, len(value.StateTransitions))
	for _, t := range value.StateTransitions {
		result.StateTransitions = append(result.StateTransitions, AIExplanationEvaluationV2TransitionWire{From: t.From, To: t.To, CauseCode: t.CauseCode, Reason: t.Reason, Actor: t.Actor, TransitionedAt: t.TransitionedAt, EvidenceRefs: append([]string{}, t.EvidenceRefs...)})
	}
	if checkpoint != nil {
		result.Execution = &AIExplanationEvaluationV2CheckpointWire{
			ID: checkpoint.ID, Kind: string(checkpoint.Kind), CaseID: checkpoint.CaseID, SlotOrdinal: checkpoint.SlotOrdinal,
			CandidateID: checkpoint.CandidateID, ExecutionOrdinal: checkpoint.ExecutionOrdinal, Phase: string(checkpoint.Phase),
			ClaimedAt: checkpoint.ClaimedAt, LeaseExpiresAt: checkpoint.LeaseExpiresAt, DispatchStartedAt: checkpoint.DispatchStartedAt,
		}
	}
	for _, slot := range value.Slots {
		wire := AIExplanationEvaluationV2SlotWire{
			CaseID: slot.CaseID, SlotOrdinal: slot.Ordinal, Status: string(slot.Status),
			GenerationExecutionIDs: append([]string(nil), slot.GenerationExecutionIDs...),
		}
		if slot.Candidate != nil {
			result.AcceptedCandidates++
			if slot.Candidate.ReviewReady {
				result.ReviewReadyCandidates++
			}
			candidate := candidateV2Wire(*slot.Candidate)
			wire.Candidate = &candidate
		}
		result.Slots = append(result.Slots, wire)
	}
	for _, execution := range value.GenerationExecutions {
		result.GenerationExecutions = append(result.GenerationExecutions, generationExecutionV2Wire(execution))
	}
	for _, execution := range value.SemanticExecutions {
		result.SemanticExecutions = append(result.SemanticExecutions, semanticExecutionV2Wire(execution))
	}
	for _, review := range value.HumanReviews {
		result.HumanReviews = append(result.HumanReviews, AIExplanationEvaluationV2HumanReviewWire{
			CandidateID: review.CandidateID, Role: string(review.Role), Reviewer: review.Reviewer,
			Decision: string(review.Decision), ReviewedAt: review.ReviewedAt, Reason: review.Reason, SemanticReview: review.SemanticReview,
		})
	}
	for _, resolution := range value.ResultUnknownResolutions {
		result.ResultUnknownResolutions = append(result.ResultUnknownResolutions, AIExplanationResultUnknownResolutionWire{
			ExecutionID: resolution.ExecutionID, Decision: string(resolution.Decision), Actor: resolution.Actor, Reason: resolution.Reason,
			AcknowledgedDuplicateCallAndCostRisk: resolution.AcknowledgedDuplicateCallAndCostRisk, ResolvedAt: resolution.ResolvedAt,
		})
	}
	result.ReviewReopenings = value.ReviewReopenings
	result.CanReopenReview = len(value.ReopenReviewCandidateIDs()) > 0
	if value.Status == domainevaluation.EvidenceStatusAwaitingReview {
		if preview, err := value.EvaluateGate(time.Now().UTC()); err == nil {
			result.GatePreview = evaluationV2GateWire(&preview)
		}
	}
	result.Gate = evaluationV2GateWire(value.GateResult)
	return result
}

func generationExecutionV2Wire(value domainevaluation.CandidateGenerationExecution) AIExplanationEvaluationV2ExecutionWire {
	return AIExplanationEvaluationV2ExecutionWire{
		ExecutionID: value.ID, Kind: "generation", CaseID: value.CaseID, SlotOrdinal: value.SlotOrdinal,
		ExecutionOrdinal: value.ExecutionOrdinal, InvocationID: value.InvocationID, Status: string(value.Status),
		StartedAt: value.StartedAt, FinishedAt: value.FinishedAt, ProviderCallCount: value.ProviderCallCount,
		ProviderReceiptPresent: value.ProviderReceipt != nil, ProviderReceipt: providerReceiptWire(value.ProviderReceipt), RawOutputBytes: len(value.RawOutput), NormalizedBytes: len(value.NormalizedOutput),
		Failure: failureV2Wire(value.Failure),
	}
}

func semanticExecutionV2Wire(value domainevaluation.SemanticEvaluationExecution) AIExplanationEvaluationV2ExecutionWire {
	result := AIExplanationEvaluationV2ExecutionWire{
		ExecutionID: value.ID, Kind: "semantic", CandidateID: value.CandidateID, ExecutionOrdinal: value.ExecutionOrdinal,
		InvocationID: value.InvocationID, Status: string(value.Status), StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		ProviderCallCount: value.ProviderCallCount, ProviderReceiptPresent: value.ProviderReceipt != nil, ProviderReceipt: providerReceiptWire(value.ProviderReceipt),
		RawOutputBytes: len(value.RawOutput), NormalizedBytes: len(value.NormalizedOutput), Failure: failureV2Wire(value.Failure),
	}
	if value.Result != nil {
		result.SemanticResult = &AIExplanationEvaluationV2SemanticWire{
			EvaluatorVersion: value.Result.EvaluatorVersion, Rationale: value.Result.Rationale,
			OutputFingerprint: string(value.Result.OutputFingerprint),
			Scores: AIExplanationEvaluationV2SemanticScoresWire{
				Faithfulness: value.Result.Scores.Faithfulness, CrossDimensionQuality: value.Result.Scores.CrossDimensionQuality,
				SuggestionActionability: value.Result.Scores.SuggestionActionability, AudienceClarity: value.Result.Scores.AudienceClarity,
				Concision: value.Result.Scores.Concision,
			},
			Decisions: make([]AIExplanationEvaluationV2SemanticDecisionWire, 0, len(value.Result.Decisions)),
		}
		for _, decision := range value.Result.Decisions {
			result.SemanticResult.Decisions = append(result.SemanticResult.Decisions, AIExplanationEvaluationV2SemanticDecisionWire{
				Type: decision.Type, Scope: string(decision.Scope), Ordinal: decision.Ordinal, Status: string(decision.Status), Detail: decision.Detail,
			})
		}
	}
	return result
}

func evaluationV2ReleaseWire(value domainevaluation.EvidenceReleaseIdentity) AIExplanationEvaluationV2ReleaseWire {
	return AIExplanationEvaluationV2ReleaseWire{
		Fingerprint: string(value.Fingerprint), Suite: evaluationV2FrozenRefWire(value.Suite), Prompt: evaluationV2FrozenRefWire(value.Prompt),
		Profile: evaluationV2FrozenRefWire(value.Profile), InputSchema: evaluationV2FrozenRefWire(value.InputSchema),
		OutputSchema: evaluationV2FrozenRefWire(value.OutputSchema), GenerationRoute: evaluationV2FrozenRefWire(value.GenerationRoute),
		SemanticPrompt: evaluationV2FrozenRefWire(value.SemanticPrompt), SemanticOutputSchema: evaluationV2FrozenRefWire(value.SemanticOutputSchema),
		SemanticRoute: evaluationV2FrozenRefWire(value.SemanticRoute), ExecutionPolicy: evaluationV2FrozenRefWire(value.ExecutionPolicy),
		GatePolicy: evaluationV2FrozenRefWire(value.GatePolicy),
	}
}

func evaluationV2FrozenRefWire(value domainevaluation.FrozenContractRef) AIExplanationEvaluationV2FrozenRefWire {
	return AIExplanationEvaluationV2FrozenRefWire{ID: value.ID, Version: value.Version, Fingerprint: string(value.Fingerprint)}
}

func evaluationV2CandidateEvidenceWire(value *domainevaluation.PromptEvaluationEvidenceV2, candidateID string) (*AIExplanationEvaluationV2CandidateEvidenceWire, error) {
	if value == nil || candidateID == "" {
		return nil, cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation v2 candidate not found")
	}
	suite, err := appevaluation.LoadFrozen(value.Release.Suite.ID, value.Release.Suite.Version, value.Release.Suite.Fingerprint)
	if err != nil {
		return nil, cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation v2 frozen suite is unavailable")
	}
	var slot *domainevaluation.CandidateSlot
	for index := range value.Slots {
		if value.Slots[index].Candidate != nil && value.Slots[index].Candidate.ID == candidateID {
			slot = &value.Slots[index]
			break
		}
	}
	if slot == nil {
		return nil, cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation v2 candidate not found")
	}
	var assessmentInput json.RawMessage
	for _, testCase := range suite.Cases {
		if testCase.CaseID == slot.CaseID {
			assessmentInput, err = json.Marshal(testCase.ProviderPayload)
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if len(assessmentInput) == 0 {
		return nil, cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation v2 frozen case not found")
	}
	candidate := candidateV2Wire(*slot.Candidate)
	result := &AIExplanationEvaluationV2CandidateEvidenceWire{
		RunID: value.RunID.String(), CaseID: slot.CaseID, SlotOrdinal: slot.Ordinal, AssessmentInput: assessmentInput, Candidate: candidate,
		HumanReviews: make([]AIExplanationEvaluationV2HumanReviewWire, 0, len(value.HumanReviews)),
	}
	foundGeneration := false
	for _, execution := range value.GenerationExecutions {
		if execution.ID == slot.Candidate.GenerationExecutionID {
			result.AcceptedGenerationExecution = generationExecutionV2EvidenceWire(execution)
			foundGeneration = true
			break
		}
	}
	if !foundGeneration {
		return nil, cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation v2 accepted generation execution not found")
	}
	if slot.Candidate.AcceptedSemanticExecutionID != "" {
		for _, execution := range value.SemanticExecutions {
			if execution.ID == slot.Candidate.AcceptedSemanticExecutionID {
				wire := semanticExecutionV2EvidenceWire(execution)
				result.AcceptedSemanticExecution = &wire
				break
			}
		}
		if result.AcceptedSemanticExecution == nil {
			return nil, cberrors.WithCode(code.ErrPageNotFound, "AI explanation evaluation v2 accepted semantic execution not found")
		}
	}
	for _, review := range value.HumanReviews {
		if review.CandidateID == candidateID {
			result.HumanReviews = append(result.HumanReviews, AIExplanationEvaluationV2HumanReviewWire{
				CandidateID: review.CandidateID, Role: string(review.Role), Reviewer: review.Reviewer,
				Decision: string(review.Decision), ReviewedAt: review.ReviewedAt, Reason: review.Reason, SemanticReview: review.SemanticReview,
			})
		}
	}
	return result, nil
}

func candidateV2Wire(value domainevaluation.Candidate) AIExplanationEvaluationV2CandidateWire {
	result := AIExplanationEvaluationV2CandidateWire{
		CandidateID: value.ID, GenerationExecutionID: value.GenerationExecutionID, NormalizedOutputFingerprint: string(value.NormalizedOutputFingerprint),
		AcceptedAt: value.AcceptedAt, SemanticExecutionIDs: append([]string(nil), value.SemanticExecutionIDs...),
		AcceptedSemanticExecutionID: value.AcceptedSemanticExecutionID, ReviewReady: value.ReviewReady,
		Assertions: make([]AIExplanationEvaluationV2AssertionWire, 0, len(value.Assertions)),
	}
	for _, assertion := range value.Assertions {
		result.Assertions = append(result.Assertions, AIExplanationEvaluationV2AssertionWire{
			Type: assertion.Type, Scope: string(assertion.Scope), Ordinal: assertion.Ordinal, Hard: assertion.Hard,
			Evaluator: assertion.Evaluator, Status: string(assertion.Status), Detail: assertion.Detail,
		})
	}
	return result
}

func generationExecutionV2EvidenceWire(value domainevaluation.CandidateGenerationExecution) AIExplanationEvaluationV2ExecutionEvidenceWire {
	return AIExplanationEvaluationV2ExecutionEvidenceWire{
		AIExplanationEvaluationV2ExecutionWire: generationExecutionV2Wire(value),
		RawOutput:                              string(value.RawOutput), NormalizedOutput: string(value.NormalizedOutput),
	}
}

func semanticExecutionV2EvidenceWire(value domainevaluation.SemanticEvaluationExecution) AIExplanationEvaluationV2ExecutionEvidenceWire {
	return AIExplanationEvaluationV2ExecutionEvidenceWire{
		AIExplanationEvaluationV2ExecutionWire: semanticExecutionV2Wire(value),
		RawOutput:                              string(value.RawOutput), NormalizedOutput: string(value.NormalizedOutput),
	}
}

func failureV2Wire(value *domainevaluation.ClassifiedFailure) *AIExplanationEvaluationV2FailureWire {
	if value == nil {
		return nil
	}
	return &AIExplanationEvaluationV2FailureWire{
		Stage: string(value.Stage), Kind: string(value.Kind), Code: value.Code, Retryable: value.Retryable,
		ResultUnknown: value.ResultUnknown, Disposition: string(value.Disposition), SafeMessage: value.SafeMessage,
		EvidenceRefs: append([]string(nil), value.EvidenceRefs...), ProviderDiagnostics: value.Clone().ProviderDiagnostics,
	}
}

func evaluationV2GateWire(value *domainevaluation.EvidenceGateResult) *AIExplanationEvaluationV2GateWire {
	var result *AIExplanationEvaluationV2GateWire
	if value != nil {
		result = &AIExplanationEvaluationV2GateWire{
			SemanticAdjudications: value.SemanticAdjudications,
			EvaluatedAt:           value.EvaluatedAt, Passed: value.Passed,
			GatePasses: make(map[string]bool, len(value.GatePasses)),
			Reasons:    make([]AIExplanationEvaluationV2GateReasonWire, 0, len(value.Reasons)),
		}
		for gate, passed := range value.GatePasses {
			result.GatePasses[gate] = passed
		}
		for _, reason := range value.Reasons {
			result.Reasons = append(result.Reasons, AIExplanationEvaluationV2GateReasonWire{
				Gate: reason.Gate, Code: reason.Code, Detail: reason.Detail, EvidenceRefs: append([]string(nil), reason.EvidenceRefs...),
			})
		}
	}
	return result
}

// ReopenEvaluationReviewV2 godoc
// @Summary 追加审计记录并重开语义矛盾复核，不调用 Provider
// @Tags AI-Explanation-Administration
// @Accept json
// @Produce json
// @Param run_id path string true "评测 Run ID"
// @Param request body AIExplanationFinalizeRequest true "重新复核理由"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 400 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/reopen-review [post]
func (h *AIExplanationAdministrationHandler) ReopenEvaluationReviewV2(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationFinalizeRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	value, err := h.service.ReopenEvaluationReviewV2(c.Request.Context(), actor, runID, request.Reason)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, evaluationV2Wire(value))
}

type AIExplanationFinalizeV2CheckedRequest struct {
	Reason          string `json:"reason" binding:"required"`
	ExpectedVersion *int64 `json:"expected_version" binding:"required"`
	ExpectedPassed  *bool  `json:"expected_passed" binding:"required"`
}
