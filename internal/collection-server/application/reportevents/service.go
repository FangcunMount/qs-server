package reportevents

import (
	"context"
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
)

// StatusPayload 是 WebSocket 推送与 HTTP report-status 对齐的公共状态载荷。
type StatusPayload = reportstatus.View

type TesteeAccessAuthorizer interface {
	Authorize(ctx context.Context, userID string, testeeID uint64) error
}

// Service 负责 WS 订阅鉴权与状态读取。
type Service struct {
	testeeAccess TesteeAccessAuthorizer
	resolver     *reportstatus.Resolver
}

func NewService(testeeAccess TesteeAccessAuthorizer, resolver *reportstatus.Resolver) *Service {
	return &Service{testeeAccess: testeeAccess, resolver: resolver}
}

func (s *Service) Authorize(ctx context.Context, userID, kind string, testeeID, assessmentID uint64) error {
	if s == nil || s.testeeAccess == nil || s.resolver == nil {
		return testeeaccess.ErrAccessUnavailable
	}
	if err := s.testeeAccess.Authorize(ctx, userID, testeeID); err != nil {
		return err
	}
	return s.resolver.Authorize(ctx, kind, testeeID, assessmentID)
}

func (s *Service) CurrentStatus(ctx context.Context, userID, kind string, testeeID, assessmentID uint64) (*StatusPayload, error) {
	if err := s.Authorize(ctx, userID, kind, testeeID, assessmentID); err != nil {
		return nil, err
	}
	return s.resolver.CurrentStatusAuthorized(ctx, kind, testeeID, assessmentID)
}

func ParseUintID(raw string) (uint64, error) {
	if raw == "" {
		return 0, fmt.Errorf("id is required")
	}
	return strconv.ParseUint(raw, 10, 64)
}
