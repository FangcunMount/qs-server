package handler

import (
	"bytes"
	goerrors "errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

// QRCodeHandler 二维码图片处理器
type QRCodeHandler struct {
	BaseHandler
	storageDir      string
	objectStore     objectstorageport.PublicObjectStore
	objectKeyPrefix string
}

// NewQRCodeHandler 创建二维码图片处理器
func NewQRCodeHandler(objectStore objectstorageport.PublicObjectStore, objectKeyPrefix string) *QRCodeHandler {
	return &QRCodeHandler{
		storageDir:      qrcode.QRCodeStorageDir,
		objectStore:     objectStore,
		objectKeyPrefix: strings.Trim(objectKeyPrefix, "/"),
	}
}

// GetQRCodeImage 获取二维码图片
// @Summary 获取二维码图片
// @Description 根据文件名获取二维码图片
// @Tags QRCode
// @Produce image/png
// @Param filename path string true "文件名，例如 questionnaire_3adyDE_v1.png 或 scale_3adyDE.png"
// @Success 200 {file} image/png
// @Failure 400 {object} core.Response
// @Failure 404 {object} core.Response
// @Router /api/v1/qrcodes/{filename} [get]
func (h *QRCodeHandler) GetQRCodeImage(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" {
		h.BadRequestResponse(c, "文件名不能为空", errors.New("filename cannot be empty"))
		return
	}

	// 验证文件名格式（防止路径遍历攻击）
	if !isValidQRCodeFilename(filename) {
		h.BadRequestResponse(c, "无效的文件名格式", errors.New("invalid filename format"))
		return
	}

	// 构建完整文件路径
	filePath := filepath.Join(h.storageDir, filename)

	if fileData, err := h.readLocalQRCode(filePath); err == nil {
		h.writePNG(c, filename, int64(len(fileData)), bytes.NewReader(fileData), "public, max-age=3600")
		return
	} else if !goerrors.Is(err, os.ErrNotExist) {
		logger.L(c.Request.Context()).Errorw("读取本地二维码文件失败",
			"action", "get_qrcode_image",
			"filename", filename,
			"error", err.Error(),
		)
		h.Error(c, errors.WithCode(code.ErrInternalServerError, "读取二维码文件失败"))
		return
	}

	if h.objectStore == nil {
		h.NotFoundResponse(c, "二维码文件不存在", errors.New("qrcode file not found"))
		return
	}

	reader, err := h.objectStore.Get(c.Request.Context(), h.objectKey(filename))
	if err != nil {
		if goerrors.Is(err, objectstorageport.ErrObjectNotFound) {
			h.NotFoundResponse(c, "二维码文件不存在", errors.New("qrcode object not found"))
			return
		}
		logger.L(c.Request.Context()).Errorw("读取 OSS 二维码对象失败",
			"action", "get_qrcode_image",
			"filename", filename,
			"object_key", h.objectKey(filename),
			"error", err.Error(),
		)
		h.Error(c, errors.WithCode(code.ErrInternalServerError, "读取二维码文件失败"))
		return
	}
	defer func() {
		if closeErr := reader.Body.Close(); closeErr != nil {
			logger.L(c.Request.Context()).Warnw("关闭二维码对象流失败",
				"action", "close_qrcode_stream",
				"filename", filename,
				"error", closeErr.Error(),
			)
		}
	}()

	cacheControl := reader.CacheControl
	if cacheControl == "" {
		cacheControl = "public, max-age=3600"
	}
	contentType := reader.ContentType
	if contentType == "" {
		contentType = "image/png"
	}
	h.writeImage(c, filename, reader.ContentLength, contentType, reader.Body, cacheControl)
}

func (h *QRCodeHandler) objectKey(filename string) string {
	if h.objectKeyPrefix == "" {
		return filename
	}
	return h.objectKeyPrefix + "/" + filename
}

func (h *QRCodeHandler) readLocalQRCode(filePath string) ([]byte, error) {
	// #nosec G304 -- filename is validated and constrained under the QR code storage directory.
	return os.ReadFile(filePath)
}

func (h *QRCodeHandler) writePNG(c *gin.Context, filename string, contentLength int64, body io.Reader, cacheControl string) {
	h.writeImage(c, filename, contentLength, "image/png", body, cacheControl)
}

func (h *QRCodeHandler) writeImage(c *gin.Context, filename string, contentLength int64, contentType string, body io.Reader, cacheControl string) {
	// 设置响应头
	c.Header("Content-Disposition", `inline; filename="`+filename+`"`)
	c.Header("Cache-Control", cacheControl)
	c.DataFromReader(http.StatusOK, contentLength, contentType, body, nil)
}

// isValidQRCodeFilename 验证文件名格式
// 只允许：questionnaire_{code}_{version}.png / scale_{code}.png / assessment_entry_{token}.png
func isValidQRCodeFilename(filename string) bool {
	// 检查基本格式
	if !strings.HasSuffix(filename, ".png") {
		return false
	}

	// 移除 .png 后缀
	name := strings.TrimSuffix(filename, ".png")

	// 检查是否包含路径分隔符（防止路径遍历）
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return false
	}

	// 检查文件名格式
	// 问卷格式：questionnaire_{code}_{version}
	// 量表格式：scale_{code}
	if strings.HasPrefix(name, "questionnaire_") {
		// questionnaire_xxx_xxx 格式
		parts := strings.Split(name, "_")
		if len(parts) >= 3 && parts[0] == "questionnaire" {
			return true
		}
	} else if strings.HasPrefix(name, "scale_") {
		// scale_xxx 格式
		parts := strings.Split(name, "_")
		if len(parts) >= 2 && parts[0] == "scale" {
			return true
		}
	} else if strings.HasPrefix(name, "assessment_entry_") {
		parts := strings.Split(name, "_")
		if len(parts) >= 3 && parts[0] == "assessment" && parts[1] == "entry" {
			return true
		}
	}

	return false
}
