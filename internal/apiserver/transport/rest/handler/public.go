package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// PublicInfo 返回公开服务信息。
// @Summary 获取公开服务信息
// @Tags Public
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/public/info [get]
func PublicInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":     "questionnaire-scale",
		"version":     "1.0.0",
		"description": "问卷量表管理系统",
	})
}
