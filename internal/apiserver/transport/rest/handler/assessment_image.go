package handler

import (
	"errors"
	"net/http"
	"path"
	"regexp"
	"strings"

	objectstorage "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	"github.com/gin-gonic/gin"
)

var (
	assetPathSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
	assetFilename    = regexp.MustCompile(`^[a-f0-9]{64}\.(?:png|jpg|webp)$`)
)

// AssessmentImageHandler proxies immutable model assets from the private OSS
// bucket. The report API may safely expose this stable qs-server URL.
type AssessmentImageHandler struct {
	BaseHandler
	store     objectstorage.ObjectStore
	keyPrefix string
}

func NewAssessmentImageHandler(store objectstorage.ObjectStore, keyPrefix string) *AssessmentImageHandler {
	return &AssessmentImageHandler{store: store, keyPrefix: strings.Trim(keyPrefix, "/")}
}

// GetMBTIOutcomeImage streams one immutable MBTI portrait.
// @Summary 获取 MBTI 人物图片
// @Tags AssessmentAssets
// @Produce image/png,image/jpeg,image/webp
// @Param model path string true "模型编码"
// @Param outcome path string true "MBTI 结果编码"
// @Param filename path string true "内容哈希文件名"
// @Success 200 {file} binary
// @Router /api/v1/assessment-assets/typology/{model}/{outcome}/{filename} [get]
func (h *AssessmentImageHandler) GetMBTIOutcomeImage(c *gin.Context) {
	modelCode, outcomeCode, filename := c.Param("model"), c.Param("outcome"), c.Param("filename")
	if !assetPathSegment.MatchString(modelCode) || !assetPathSegment.MatchString(outcomeCode) || !assetFilename.MatchString(filename) {
		h.NotFoundResponse(c, "assessment image not found", nil)
		return
	}
	if h.store == nil {
		h.NotFoundResponse(c, "assessment image not found", nil)
		return
	}
	reader, err := h.store.Get(c.Request.Context(), path.Join(h.keyPrefix, modelCode, outcomeCode, filename))
	if err != nil {
		if errors.Is(err, objectstorage.ErrObjectNotFound) {
			h.NotFoundResponse(c, "assessment image not found", nil)
			return
		}
		h.InternalErrorResponse(c, "get assessment image failed", err)
		return
	}
	defer func() { _ = reader.Body.Close() }()
	contentType := reader.ContentType
	if contentType == "" {
		contentType = contentTypeForFilename(filename)
	}
	cacheControl := reader.CacheControl
	if cacheControl == "" {
		cacheControl = "public, max-age=31536000, immutable"
	}
	c.Header("Cache-Control", cacheControl)
	c.Header("Content-Disposition", `inline; filename="`+filename+`"`)
	c.DataFromReader(http.StatusOK, reader.ContentLength, contentType, reader.Body, nil)
}

func contentTypeForFilename(filename string) string {
	switch path.Ext(filename) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
