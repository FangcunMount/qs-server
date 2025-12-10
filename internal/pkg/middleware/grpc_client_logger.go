package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/FangcunMount/component-base/pkg/log"
)

// GRPCLoggerConfig gRPC日志配置
type GRPCLoggerConfig struct {
	// LogRequestPayload 是否记录请求载荷
	LogRequestPayload bool
	// LogResponsePayload 是否记录响应载荷
	LogResponsePayload bool
	// LogLevel 日志级别，0=INFO, 1=DEBUG
	LogLevel int
	// MaxPayloadSize 最大记录的载荷大小
	MaxPayloadSize int
}

// DefaultGRPCLoggerConfig 默认gRPC日志配置
func DefaultGRPCLoggerConfig() GRPCLoggerConfig {
	return GRPCLoggerConfig{
		LogRequestPayload:  true,
		LogResponsePayload: true,
		LogLevel:           0,    // INFO level
		MaxPayloadSize:     2048, // 2KB
	}
}

// UnaryClientLoggingInterceptor gRPC一元客户端日志拦截器
func UnaryClientLoggingInterceptor() grpc.UnaryClientInterceptor {
	return UnaryClientLoggingInterceptorWithConfig(DefaultGRPCLoggerConfig())
}

// UnaryClientLoggingInterceptorWithConfig 带配置的gRPC一元客户端日志拦截器
func UnaryClientLoggingInterceptorWithConfig(config GRPCLoggerConfig) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		startTime := time.Now()

		// 记录请求开始
		grpcInfo(ctx, "gRPC client request started",
			log.String("method", method),
			log.String("target", cc.Target()),
		)

		// 记录请求metadata
		if config.LogLevel >= 1 { // DEBUG level
			if md, ok := metadata.FromOutgoingContext(ctx); ok {
				grpcDebug(ctx, "Outgoing metadata", log.Any("metadata", metadataToStringMap(md)))
			}
		}

		// 记录请求载荷
		if config.LogRequestPayload && req != nil {
			if payload, err := formatPayload(req, config.MaxPayloadSize); err == nil {
				grpcDebug(ctx, "gRPC request payload", log.String("payload", payload))
			} else {
				grpcWarn(ctx, "gRPC request payload formatting failed", log.String("error", err.Error()))
			}
		}

		// 执行gRPC调用
		err := invoker(ctx, method, req, reply, cc, opts...)

		// 计算耗时
		latency := time.Since(startTime)

		// 记录响应信息
		if err != nil {
			st := status.Convert(err)
			grpcError(ctx, "gRPC client request failed",
				log.String("method", method),
				log.String("code", st.Code().String()),
				log.String("message", st.Message()),
				log.Duration("latency", latency),
			)
		} else {
			grpcInfo(ctx, "gRPC client request succeeded",
				log.String("method", method),
				log.Duration("latency", latency),
			)

			// 记录响应载荷
			if config.LogResponsePayload && reply != nil {
				if payload, err := formatPayload(reply, config.MaxPayloadSize); err == nil {
					grpcDebug(ctx, "gRPC response payload", log.String("payload", payload))
				} else {
					grpcWarn(ctx, "gRPC response payload formatting failed", log.String("error", err.Error()))
				}
			}
		}

		// 记录响应metadata
		if config.LogLevel >= 1 {
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				grpcDebug(ctx, "Incoming metadata", log.Any("metadata", metadataToStringMap(md)))
			}
		}

		grpcInfo(ctx, "gRPC client request completed",
			log.String("method", method),
			log.Duration("latency", latency),
		)
		return err
	}
}

// StreamClientLoggingInterceptor gRPC流式客户端日志拦截器
func StreamClientLoggingInterceptor() grpc.StreamClientInterceptor {
	return StreamClientLoggingInterceptorWithConfig(DefaultGRPCLoggerConfig())
}

// StreamClientLoggingInterceptorWithConfig 带配置的gRPC流式客户端日志拦截器
func StreamClientLoggingInterceptorWithConfig(config GRPCLoggerConfig) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		grpcInfo(ctx, "gRPC stream started",
			log.String("method", method),
			log.String("target", cc.Target()),
			log.Bool("server_streams", desc.ServerStreams),
			log.Bool("client_streams", desc.ClientStreams),
		)

		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			st := status.Convert(err)
			grpcError(ctx, "gRPC stream error",
				log.String("method", method),
				log.String("code", st.Code().String()),
				log.String("message", st.Message()),
			)
			return nil, err
		}

		grpcInfo(ctx, "gRPC stream established successfully", log.String("method", method))
		return &loggingClientStream{ClientStream: stream, ctx: ctx, method: method}, nil
	}
}

// loggingClientStream 包装gRPC流以添加日志功能
type loggingClientStream struct {
	grpc.ClientStream
	ctx    context.Context
	method string
}

func (s *loggingClientStream) SendMsg(m interface{}) error {
	grpcDebug(s.ctx, "gRPC stream send message", log.String("method", s.method))
	return s.ClientStream.SendMsg(m)
}

func (s *loggingClientStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		grpcWarn(s.ctx, "gRPC stream receive error",
			log.String("method", s.method),
			log.String("error", err.Error()),
		)
	} else {
		grpcDebug(s.ctx, "gRPC stream receive success", log.String("method", s.method))
	}
	return err
}

func (s *loggingClientStream) CloseSend() error {
	grpcDebug(s.ctx, "gRPC stream close send", log.String("method", s.method))
	return s.ClientStream.CloseSend()
}

// formatPayload 格式化载荷数据
func formatPayload(payload interface{}, maxSize int) (string, error) {
	// 尝试JSON序列化
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("[Non-JSON: %T]", payload), nil
	}

	// 格式化JSON
	var compact strings.Builder
	compact.Write(data)

	result := compact.String()
	if len(result) > maxSize {
		return result[:maxSize] + "...", nil
	}

	return result, nil
}

func grpcInfo(ctx context.Context, msg string, fields ...log.Field) {
	log.GRPC(msg, append(fields, log.TraceFields(ctx)...)...)
}

func grpcDebug(ctx context.Context, msg string, fields ...log.Field) {
	log.GRPCDebug(msg, append(fields, log.TraceFields(ctx)...)...)
}

func grpcWarn(ctx context.Context, msg string, fields ...log.Field) {
	log.GRPCWarn(msg, append(fields, log.TraceFields(ctx)...)...)
}

func grpcError(ctx context.Context, msg string, fields ...log.Field) {
	log.GRPCError(msg, append(fields, log.TraceFields(ctx)...)...)
}

func metadataToStringMap(md metadata.MD) map[string]string {
	result := make(map[string]string, len(md))
	for key, values := range md {
		result[key] = strings.Join(values, ", ")
	}
	return result
}
