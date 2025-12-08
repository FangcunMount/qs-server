package grpc

import (
	"github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/util/idutil"
)

// NewComponentBaseLogger 创建适配 component-base 日志的拦截器日志
func NewComponentBaseLogger() interceptors.InterceptorLogger {
	return &componentBaseLogger{}
}

// RequestIDGenerator 返回请求 ID 生成器
func RequestIDGenerator() func() string {
	return idutil.NewRequestID
}

// componentBaseLogger 适配 component-base 日志到 InterceptorLogger 接口
type componentBaseLogger struct{}

func (l *componentBaseLogger) LogInfo(msg string, fields map[string]interface{}) {
	log.Infow(msg, mapToLogFields(fields)...)
}

func (l *componentBaseLogger) LogError(msg string, fields map[string]interface{}) {
	log.Errorw(msg, mapToLogFields(fields)...)
}

// mapToLogFields 将 map 转换为 log.Field
func mapToLogFields(m map[string]interface{}) []interface{} {
	fields := make([]interface{}, 0, len(m)*2)
	for k, v := range m {
		fields = append(fields, k, v)
	}
	return fields
}
