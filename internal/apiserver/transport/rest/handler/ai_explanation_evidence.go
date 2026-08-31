package handler

import (
	"net/http"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	aiexplanationadministration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

type AIExplanationEvaluationV2ReviewRequest struct {
	CandidateID string `json:"candidate_id" binding:"required"`
	Role        string `json:"role" binding:"required"`
	Decision    string `json:"decision" binding:"required"`
	Reason      string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationResultUnknownV2Request struct {
	ExecutionID                          string `json:"execution_id" binding:"required"`
	Decision                             string `json:"decision" binding:"required"`
	Confirm                              bool   `json:"confirm" binding:"required"`
	AcknowledgedDuplicateCallAndCostRisk bool   `json:"acknowledged_duplicate_call_and_cost_risk" binding:"required"`
	Reason                               string `json:"reason" binding:"required,max=1000"`
}

type AIExplanationEvaluationV2Wire struct {
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
	ExecutionID       string                                 `json:"execution_id"`
	Kind              string                                 `json:"kind"`
	CaseID            string                                 `json:"case_id,omitempty"`
	SlotOrdinal       int                                    `json:"slot_ordinal,omitempty"`
	CandidateID       string                                 `json:"candidate_id,omitempty"`
	ExecutionOrdinal  int                                    `json:"execution_ordinal"`
	InvocationID      string                                 `json:"invocation_id"`
	Status            string                                 `json:"status"`
	StartedAt         time.Time                              `json:"started_at"`
	FinishedAt        *time.Time                             `json:"finished_at,omitempty"`
	ProviderCallCount int                                    `json:"provider_call_count"`
	ProviderReceipt   bool                                   `json:"provider_receipt_present"`
	RawOutputBytes    int                                    `json:"raw_output_bytes"`
	NormalizedBytes   int                                    `json:"normalized_output_bytes"`
	Failure           *AIExplanationEvaluationV2FailureWire  `json:"failure,omitempty"`
	SemanticResult    *AIExplanationEvaluationV2SemanticWire `json:"semantic_result,omitempty"`
}

type AIExplanationEvaluationV2FailureWire struct {
	Stage         string   `json:"stage"`
	Kind          string   `json:"kind"`
	Code          string   `json:"code"`
	Retryable     bool     `json:"retryable"`
	ResultUnknown bool     `json:"result_unknown"`
	Disposition   string   `json:"disposition"`
	SafeMessage   string   `json:"safe_message"`
	EvidenceRefs  []string `json:"evidence_refs"`
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
	CandidateID string    `json:"candidate_id"`
	Role        string    `json:"role"`
	Reviewer    string    `json:"reviewer"`
	Decision    string    `json:"decision"`
	ReviewedAt  time.Time `json:"reviewed_at"`
	Reason      string    `json:"reason"`
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
	EvaluatedAt time.Time                                 `json:"evaluated_at"`
	Passed      bool                                      `json:"passed"`
	GatePasses  map[string]bool                           `json:"gate_passes"`
	Reasons     []AIExplanationEvaluationV2GateReasonWire `json:"reasons"`
}

type AIExplanationEvaluationV2GateReasonWire struct {
	Gate         string   `json:"gate"`
	Code         string   `json:"code"`
	Detail       string   `json:"detail"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type AIExplanationEvaluationV2OutputWire struct {
	ExecutionID      string `json:"execution_id"`
	Kind             string `json:"kind"`
	RawOutput        string `json:"raw_output"`
	NormalizedOutput string `json:"normalized_output"`
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
			h.Success(c, AIExplanationEvaluationV2OutputWire{ExecutionID: execution.ID, Kind: "generation", RawOutput: string(execution.RawOutput), NormalizedOutput: string(execution.NormalizedOutput)})
			return
		}
	}
	for _, execution := range value.SemanticExecutions {
		if execution.ID == executionID {
			h.Success(c, AIExplanationEvaluationV2OutputWire{ExecutionID: execution.ID, Kind: "semantic", RawOutput: string(execution.RawOutput), NormalizedOutput: string(execution.NormalizedOutput)})
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
		Decision: domainevaluation.ReviewDecision(request.Decision), Reason: request.Reason,
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
// @Param request body AIExplanationFinalizeRequest true "终审理由"
// @Success 200 {object} core.Response{data=AIExplanationEvaluationV2Wire}
// @Failure 400 {object} core.ErrResponse
// @Failure 409 {object} core.ErrResponse
// @Router /internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/finalize [post]
func (h *AIExplanationAdministrationHandler) FinalizeEvaluationV2(c *gin.Context) {
	actor, runID, ok := h.actorAndRunID(c)
	if !ok {
		return
	}
	var request AIExplanationFinalizeRequest
	if err := h.BindJSON(c, &request); err != nil {
		return
	}
	value, err := h.service.FinalizeEvaluationV2(c.Request.Context(), actor, runID, request.Reason)
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
		ReleaseFingerprint: string(value.Release.Fingerprint), ExecutionPolicyID: value.ExecutionPolicy.PolicyID,
		ExecutionPolicyVersion: value.ExecutionPolicy.Version, GatePolicyID: value.GatePolicy.PolicyID, GatePolicyVersion: value.GatePolicy.Version,
		ReservedProviderInvocations: value.ExecutionPolicy.WorstCaseProviderCalls(), RequiredCandidates: value.ExecutionPolicy.RequiredCandidateCount(),
		UnresolvedResultUnknownCount: value.UnresolvedResultUnknownCount,
		Slots:                        make([]AIExplanationEvaluationV2SlotWire, 0, len(value.Slots)),
		GenerationExecutions:         make([]AIExplanationEvaluationV2ExecutionWire, 0, len(value.GenerationExecutions)),
		SemanticExecutions:           make([]AIExplanationEvaluationV2ExecutionWire, 0, len(value.SemanticExecutions)),
		HumanReviews:                 make([]AIExplanationEvaluationV2HumanReviewWire, 0, len(value.HumanReviews)),
		ResultUnknownResolutions:     make([]AIExplanationResultUnknownResolutionWire, 0, len(value.ResultUnknownResolutions)),
	}
	if checkpoint := value.Execution(); checkpoint != nil {
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
			candidate := slot.Candidate
			wire.Candidate = &AIExplanationEvaluationV2CandidateWire{
				CandidateID: candidate.ID, GenerationExecutionID: candidate.GenerationExecutionID,
				NormalizedOutputFingerprint: string(candidate.NormalizedOutputFingerprint), AcceptedAt: candidate.AcceptedAt,
				SemanticExecutionIDs:        append([]string(nil), candidate.SemanticExecutionIDs...),
				AcceptedSemanticExecutionID: candidate.AcceptedSemanticExecutionID, ReviewReady: candidate.ReviewReady,
				Assertions: make([]AIExplanationEvaluationV2AssertionWire, 0, len(candidate.Assertions)),
			}
			for _, assertion := range candidate.Assertions {
				wire.Candidate.Assertions = append(wire.Candidate.Assertions, AIExplanationEvaluationV2AssertionWire{
					Type: assertion.Type, Scope: string(assertion.Scope), Ordinal: assertion.Ordinal, Hard: assertion.Hard,
					Evaluator: assertion.Evaluator, Status: string(assertion.Status), Detail: assertion.Detail,
				})
			}
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
			Decision: string(review.Decision), ReviewedAt: review.ReviewedAt, Reason: review.Reason,
		})
	}
	for _, resolution := range value.ResultUnknownResolutions {
		result.ResultUnknownResolutions = append(result.ResultUnknownResolutions, AIExplanationResultUnknownResolutionWire{
			ExecutionID: resolution.ExecutionID, Decision: string(resolution.Decision), Actor: resolution.Actor, Reason: resolution.Reason,
			AcknowledgedDuplicateCallAndCostRisk: resolution.AcknowledgedDuplicateCallAndCostRisk, ResolvedAt: resolution.ResolvedAt,
		})
	}
	if value.GateResult != nil {
		result.Gate = &AIExplanationEvaluationV2GateWire{
			EvaluatedAt: value.GateResult.EvaluatedAt, Passed: value.GateResult.Passed,
			GatePasses: make(map[string]bool, len(value.GateResult.GatePasses)),
			Reasons:    make([]AIExplanationEvaluationV2GateReasonWire, 0, len(value.GateResult.Reasons)),
		}
		for gate, passed := range value.GateResult.GatePasses {
			result.Gate.GatePasses[gate] = passed
		}
		for _, reason := range value.GateResult.Reasons {
			result.Gate.Reasons = append(result.Gate.Reasons, AIExplanationEvaluationV2GateReasonWire{
				Gate: reason.Gate, Code: reason.Code, Detail: reason.Detail, EvidenceRefs: append([]string(nil), reason.EvidenceRefs...),
			})
		}
	}
	return result
}

func generationExecutionV2Wire(value domainevaluation.CandidateGenerationExecution) AIExplanationEvaluationV2ExecutionWire {
	return AIExplanationEvaluationV2ExecutionWire{
		ExecutionID: value.ID, Kind: "generation", CaseID: value.CaseID, SlotOrdinal: value.SlotOrdinal,
		ExecutionOrdinal: value.ExecutionOrdinal, InvocationID: value.InvocationID, Status: string(value.Status),
		StartedAt: value.StartedAt, FinishedAt: value.FinishedAt, ProviderCallCount: value.ProviderCallCount,
		ProviderReceipt: value.ProviderReceipt != nil, RawOutputBytes: len(value.RawOutput), NormalizedBytes: len(value.NormalizedOutput),
		Failure: failureV2Wire(value.Failure),
	}
}

func semanticExecutionV2Wire(value domainevaluation.SemanticEvaluationExecution) AIExplanationEvaluationV2ExecutionWire {
	result := AIExplanationEvaluationV2ExecutionWire{
		ExecutionID: value.ID, Kind: "semantic", CandidateID: value.CandidateID, ExecutionOrdinal: value.ExecutionOrdinal,
		InvocationID: value.InvocationID, Status: string(value.Status), StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		ProviderCallCount: value.ProviderCallCount, ProviderReceipt: value.ProviderReceipt != nil,
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

func failureV2Wire(value *domainevaluation.ClassifiedFailure) *AIExplanationEvaluationV2FailureWire {
	if value == nil {
		return nil
	}
	return &AIExplanationEvaluationV2FailureWire{
		Stage: string(value.Stage), Kind: string(value.Kind), Code: value.Code, Retryable: value.Retryable,
		ResultUnknown: value.ResultUnknown, Disposition: string(value.Disposition), SafeMessage: value.SafeMessage,
		EvidenceRefs: append([]string(nil), value.EvidenceRefs...),
	}
}
