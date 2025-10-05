package core

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/marmotedu/errors"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// ErrResponse 定义了当发生错误时返回的消息
// 如果 Reference 不存在，则省略
// swagger:model
type ErrResponse struct {
	// Code 定义了业务错误代码
	Code int `json:"code"`

	// Message 包含此消息的详细信息
	// 此消息适合暴露给外部
	Message string `json:"message"`

	// Reference 返回参考文档，可能有助于解决此错误
	Reference string `json:"reference,omitempty"`
}

// WriteResponse 将错误或响应数据写入 HTTP 响应体
// 它使用 errors.ParseCoder 将任何错误解析为 errors.Coder
// 如果 err 不为 nil，则将错误写入响应体
// 如果 err 为 nil，则将响应数据写入响应体
func WriteResponse(c *gin.Context, err error, data interface{}) {
	if err != nil {
		log.Errorf("%#+v", err)
		coder := errors.ParseCoder(err)
		c.JSON(coder.HTTPStatus(), ErrResponse{
			Code:      coder.Code(),
			Message:   coder.String(),
			Reference: coder.Reference(),
		})

		return
	}

	c.JSON(http.StatusOK, data)
}
