package handler

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/gin-gonic/gin"
)

// AssessmentReleaseHandler exposes the only public publish/archive boundary
// for questionnaire-backed assessment models.
type AssessmentReleaseHandler struct {
	BaseHandler
	service modelcatalog.AssessmentReleaseService
	query   modelcatalog.CatalogQueryService
}

func NewAssessmentReleaseHandler(service modelcatalog.AssessmentReleaseService, queries ...modelcatalog.CatalogQueryService) *AssessmentReleaseHandler {
	var query modelcatalog.CatalogQueryService
	if len(queries) > 0 {
		query = queries[0]
	}
	return &AssessmentReleaseHandler{service: service, query: query}
}

// Publish atomically publishes the model and its bound questionnaire.
// @Summary 发布测评版本
// @Tags AssessmentRelease
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=modelcatalog.AssessmentRelease}
// @Router /api/v1/assessment-releases/{code}/publish [post]
func (h *AssessmentReleaseHandler) Publish(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.PublishRelease(c.Request.Context(), actor, c.Param("code"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// Unpublish atomically archives the active model/questionnaire snapshots.
// @Summary 下架测评版本
// @Tags AssessmentRelease
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=modelcatalog.AssessmentRelease}
// @Router /api/v1/assessment-releases/{code}/unpublish [post]
func (h *AssessmentReleaseHandler) Unpublish(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.UnpublishRelease(c.Request.Context(), actor, c.Param("code"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// Versions lists every retained immutable release pair.
// @Summary 查询测评发布版本历史
// @Tags AssessmentRelease
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=[]modelcatalog.AssessmentReleaseVersion}
// @Router /api/v1/assessment-releases/{code}/versions [get]
func (h *AssessmentReleaseHandler) Versions(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.query.ListReleaseVersions(c.Request.Context(), actor, c.Param("code"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// Archive atomically archives the active model/questionnaire pair.
// @Summary 归档测评版本
// @Tags AssessmentRelease
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param code path string true "模型编码"
// @Success 200 {object} core.Response{data=modelcatalog.AssessmentRelease}
// @Router /api/v1/assessment-releases/{code}/archive [post]
func (h *AssessmentReleaseHandler) Archive(c *gin.Context) {
	actor, err := assessmentModelActorContext(c)
	if err != nil {
		h.Error(c, err)
		return
	}
	result, err := h.service.ArchiveRelease(c.Request.Context(), actor, c.Param("code"))
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}
