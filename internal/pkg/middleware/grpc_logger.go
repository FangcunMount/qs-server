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
		log.L(ctx).Infof("=== gRPC Request Started ===")
		log.L(ctx).Infof("Method: %s", method)
		log.L(ctx).Infof("Target: %s", cc.Target())

		// 记录请求metadata
		if config.LogLevel >= 1 { // DEBUG level
			if md, ok := metadata.FromOutgoingContext(ctx); ok {
				log.L(ctx).V(1).Info("Outgoing Metadata:")
				for key, values := range md {
					log.L(ctx).V(1).Infof("  %s: %s", key, strings.Join(values, ", "))
				}
			}
		}

		// 记录请求载荷
		if config.LogRequestPayload && req != nil {
			if payload, err := formatPayload(req, config.MaxPayloadSize); err == nil {
				log.L(ctx).Infof("Request Payload: %s", payload)
			} else {
				log.L(ctx).Infof("Request Payload: [Error formatting: %v]", err)
			}
		}

		// 执行gRPC调用
		err := invoker(ctx, method, req, reply, cc, opts...)

		// 计算耗时
		latency := time.Since(startTime)

		// 记录响应信息
		if err != nil {
			// 记录错误
			st := status.Convert(err)
			log.L(ctx).Errorf("gRPC Error: code=%s, message=%s", st.Code(), st.Message())
			log.L(ctx).Errorf("Latency: %v", latency)
		} else {
			log.L(ctx).Infof("gRPC Success")
			log.L(ctx).Infof("Latency: %v", latency)

			// 记录响应载荷
			if config.LogResponsePayload && reply != nil {
				if payload, err := formatPayload(reply, config.MaxPayloadSize); err == nil {
					log.L(ctx).Infof("Response Payload: %s", payload)
				} else {
					log.L(ctx).Infof("Response Payload: [Error formatting: %v]", err)
				}
			}
		}

		// 记录响应metadata
		if config.LogLevel >= 1 { // DEBUG level
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				log.L(ctx).V(1).Info("Incoming Metadata:")
				for key, values := range md {
					log.L(ctx).V(1).Infof("  %s: %s", key, strings.Join(values, ", "))
				}
			}
		}

		log.L(ctx).Infof("=== gRPC Request Completed ===")
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
		log.L(ctx).Infof("=== gRPC Stream Started ===")
		log.L(ctx).Infof("Method: %s", method)
		log.L(ctx).Infof("Target: %s", cc.Target())
		log.L(ctx).Infof("ServerStreams: %t, ClientStreams: %t", desc.ServerStreams, desc.ClientStreams)

		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			st := status.Convert(err)
			log.L(ctx).Errorf("gRPC Stream Error: code=%s, message=%s", st.Code(), st.Message())
			return nil, err
		}

		log.L(ctx).Info("gRPC Stream established successfully")
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
	log.L(s.ctx).V(1).Infof("gRPC Stream SendMsg: method=%s", s.method)
	return s.ClientStream.SendMsg(m)
}

func (s *loggingClientStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		log.L(s.ctx).V(1).Infof("gRPC Stream RecvMsg error: method=%s, err=%v", s.method, err)
	} else {
		log.L(s.ctx).V(1).Infof("gRPC Stream RecvMsg success: method=%s", s.method)
	}
	return err
}

func (s *loggingClientStream) CloseSend() error {
	log.L(s.ctx).V(1).Infof("gRPC Stream CloseSend: method=%s", s.method)
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
