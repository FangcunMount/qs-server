package handler

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/response"
	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
)

// CodesHandler 处理 code 申请
type CodesHandler struct {
	BaseHandler
	svc codes.CodesService
}

// NewCodesHandler 创建 CodesHandler
func NewCodesHandler(svc codes.CodesService) *CodesHandler {
	return &CodesHandler{svc: svc}
}

// Apply 申请 code
// @Summary 申请唯一 code
// @Tags 系统
// @Accept json
// @Produce json
// @Param Authorization header string false "Bearer 用户令牌"
// @Param request body request.ApplyCodeRequest true "申请请求"
// @Success 200 {object} core.Response{data=response.ApplyCodeResponse}
// @Router /api/v1/codes/apply [post]
func (h *CodesHandler) Apply(c *gin.Context) {
	var req request.ApplyCodeRequest
	if err := h.BindJSON(c, &req); err != nil {
		h.Error(c, err)
		return
	}
	if ok, err := govalidator.ValidateStruct(req); !ok {
		h.Error(c, err)
		return
	}

	codes, err := h.svc.Apply(c.Request.Context(), req.Kind, req.Count, req.Prefix, req.Metadata)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, response.ApplyCodeResponse{Codes: codes, Count: len(codes)})
}
