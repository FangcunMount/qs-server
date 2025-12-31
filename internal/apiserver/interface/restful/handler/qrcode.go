package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

// QRCodeHandler 二维码图片处理器
type QRCodeHandler struct {
	BaseHandler
	storageDir string
}

// NewQRCodeHandler 创建二维码图片处理器
func NewQRCodeHandler() *QRCodeHandler {
	return &QRCodeHandler{
		storageDir: qrcode.QRCodeStorageDir,
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

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		h.NotFoundResponse(c, "二维码文件不存在", errors.New("qrcode file not found"))
		return
	}

	// 读取文件
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		logger.L(c.Request.Context()).Errorw("读取二维码文件失败",
			"action", "get_qrcode_image",
			"filename", filename,
			"error", err.Error(),
		)
		h.Error(c, errors.WithCode(code.ErrInternalServerError, "读取二维码文件失败"))
		return
	}

	// 设置响应头
	c.Header("Content-Type", "image/png")
	c.Header("Content-Disposition", `inline; filename="`+filename+`"`)
	c.Header("Cache-Control", "public, max-age=3600") // 缓存1小时

	// 返回图片数据
	c.Data(http.StatusOK, "image/png", fileData)
}

// isValidQRCodeFilename 验证文件名格式
// 只允许：questionnaire_{code}_{version}.png 或 scale_{code}.png
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
	}

	return false
}
