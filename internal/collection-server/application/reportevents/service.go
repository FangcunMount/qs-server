package reportevents

import (
	"context"
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
)

// StatusPayload 是 WebSocket 推送与 HTTP report-status 对齐的公共状态载荷。
type StatusPayload = reportstatus.View

// Service 负责 WS 订阅鉴权与状态读取。
type Service struct {
	resolver *reportstatus.Resolver
}

func NewService(resolver *reportstatus.Resolver) *Service {
	return &Service{resolver: resolver}
}

func (s *Service) Authorize(ctx context.Context, kind string, testeeID, assessmentID uint64) error {
	if s == nil || s.resolver == nil {
		return fmt.Errorf("report events service is not configured")
	}
	return s.resolver.Authorize(ctx, kind, testeeID, assessmentID)
}

func (s *Service) CurrentStatus(ctx context.Context, kind string, testeeID, assessmentID uint64) (*StatusPayload, error) {
	if s == nil || s.resolver == nil {
		return nil, fmt.Errorf("report events service is not configured")
	}
	return s.resolver.CurrentStatus(ctx, kind, testeeID, assessmentID)
}

func ParseUintID(raw string) (uint64, error) {
	if raw == "" {
		return 0, fmt.Errorf("id is required")
	}
	return strconv.ParseUint(raw, 10, 64)
}
