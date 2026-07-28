package handler

import (
	"strconv"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	stageport "github.com/FangcunMount/qs-server/internal/apiserver/port/historicalseedstage"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type HistoricalSeedStageHandler struct {
	BaseHandler
	reader stageport.Reader
}

func NewHistoricalSeedStageHandler(reader stageport.Reader) *HistoricalSeedStageHandler {
	return &HistoricalSeedStageHandler{BaseHandler: *NewBaseHandler(), reader: reader}
}

// Scenario returns the immutable stage ledger for one batch scenario.
// @Summary 查询历史回填场景阶段账本
// @ID getHistoricalSeedScenarioStages
// @Description 按当前机构、批次和场景返回只读阶段终态。
// @Tags Historical-Seed-Internal
// @Produce json
// @Param batch_id path string true "批次 ID"
// @Param scenario_id path string true "场景 ID"
// @Success 200 {object} core.Response
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v1/historical-seed/batches/{batch_id}/scenarios/{scenario_id} [get]
func (h *HistoricalSeedStageHandler) Scenario(c *gin.Context) {
	h.scenario(c, strings.TrimSpace(c.Param("scenario_id")))
}

// ScenarioQuery is the slash-safe scenario lookup used by the runner. Scenario
// IDs are business identities and intentionally contain path separators.
// @Summary 查询历史回填场景阶段账本（查询参数）
// @ID getHistoricalSeedScenarioStagesByQuery
// @Description 使用查询参数传递包含斜杠的场景 ID，按当前机构和批次返回只读阶段终态。
// @Tags Historical-Seed-Internal
// @Produce json
// @Param batch_id path string true "批次 ID"
// @Param scenario_id query string true "场景 ID"
// @Success 200 {object} core.Response
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v1/historical-seed/batches/{batch_id}/scenarios [get]
func (h *HistoricalSeedStageHandler) ScenarioQuery(c *gin.Context) {
	h.scenario(c, strings.TrimSpace(c.Query("scenario_id")))
}

func (h *HistoricalSeedStageHandler) scenario(c *gin.Context, scenarioID string) {
	batchID := strings.TrimSpace(c.Param("batch_id"))
	if h.reader == nil || batchID == "" || scenarioID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "batch_id and scenario_id are required"))
		return
	}
	orgID := h.GetOrgID(c)
	if orgID == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "org scope is required"))
		return
	}
	records, err := h.reader.ListScenario(c.Request.Context(), orgID, batchID, scenarioID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, gin.H{"batch_id": batchID, "scenario_id": scenarioID, "stages": records})
}

// Batch returns one page of immutable stage-ledger records for a batch.
// @Summary 查询历史回填批次阶段账本
// @ID getHistoricalSeedBatchStages
// @Description 按当前机构和批次分页返回只读阶段终态，仅用于受控回填验证。
// @Tags Historical-Seed-Internal
// @Produce json
// @Param batch_id path string true "批次 ID"
// @Param offset query int false "分页偏移" minimum(0) default(0)
// @Param limit query int false "返回条数" minimum(1) maximum(10000) default(1000)
// @Success 200 {object} core.Response
// @Failure 400 {object} core.ErrResponse
// @Failure 401 {object} core.ErrResponse
// @Failure 403 {object} core.ErrResponse
// @Failure 500 {object} core.ErrResponse
// @Router /internal/v1/historical-seed/batches/{batch_id} [get]
func (h *HistoricalSeedStageHandler) Batch(c *gin.Context) {
	batchID := strings.TrimSpace(c.Param("batch_id"))
	if h.reader == nil || batchID == "" {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "batch_id is required"))
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))
	orgID := h.GetOrgID(c)
	if orgID == 0 {
		h.Error(c, errors.WithCode(code.ErrInvalidArgument, "org scope is required"))
		return
	}
	records, err := h.reader.ListBatch(c.Request.Context(), orgID, batchID, offset, limit)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, gin.H{"batch_id": batchID, "offset": offset, "limit": limit, "stages": records})
}
