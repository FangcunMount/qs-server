package reportevents

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalityassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
)

const (
	KindMedical     = "medical"
	KindPersonality = "personality"
)

var (
	ErrInvalidKind      = errors.New("invalid assessment kind")
	ErrAssessmentAccess = errors.New("assessment access denied")
)

// StatusPayload 是 WebSocket 推送与 HTTP report-status 对齐的公共状态载荷。
type StatusPayload = reportstatus.View

type medicalReader interface {
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentDetailResponse, error)
}

type medicalStatusReader interface {
	GetStatus(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentStatusResponse, error)
}

type personalityStatusReader interface {
	Get(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentDetailResponse, error)
	GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentStatusResponse, error)
}

// Service 负责 WS 订阅鉴权与状态读取。
type Service struct {
	waitReport  medicalStatusReader
	medical     medicalReader
	personality personalityStatusReader
}

func NewService(
	waitReport medicalStatusReader,
	medical medicalReader,
	personality personalityStatusReader,
) *Service {
	return &Service{
		waitReport:  waitReport,
		medical:     medical,
		personality: personality,
	}
}

func (s *Service) Authorize(ctx context.Context, kind string, testeeID, assessmentID uint64) error {
	if s == nil {
		return fmt.Errorf("report events service is not configured")
	}
	switch kind {
	case KindMedical:
		if s.medical == nil {
			return fmt.Errorf("medical query service is not configured")
		}
		result, err := s.medical.GetMyAssessment(ctx, testeeID, assessmentID)
		if err != nil {
			return err
		}
		if result == nil {
			return ErrAssessmentAccess
		}
		return nil
	case KindPersonality:
		if s.personality == nil {
			return fmt.Errorf("personality query service is not configured")
		}
		result, err := s.personality.Get(ctx, testeeID, assessmentID)
		if err != nil {
			return err
		}
		if result == nil {
			return ErrAssessmentAccess
		}
		return nil
	default:
		return ErrInvalidKind
	}
}

func (s *Service) CurrentStatus(ctx context.Context, kind string, testeeID, assessmentID uint64) (*StatusPayload, error) {
	if err := s.Authorize(ctx, kind, testeeID, assessmentID); err != nil {
		return nil, err
	}
	switch kind {
	case KindMedical:
		status, err := s.waitReport.GetStatus(ctx, testeeID, assessmentID)
		if err != nil {
			return nil, err
		}
		return reportstatus.MedicalView(reportwait.ToPublicAssessmentStatus(status)), nil
	case KindPersonality:
		status, err := s.personality.GetReportStatus(ctx, testeeID, assessmentID)
		if err != nil {
			return nil, err
		}
		return personalityStatusView(status), nil
	default:
		return nil, ErrInvalidKind
	}
}

func ParseUintID(raw string) (uint64, error) {
	if raw == "" {
		return 0, fmt.Errorf("id is required")
	}
	return strconv.ParseUint(raw, 10, 64)
}

func IsTerminalStatus(status string) bool {
	return reportstatus.IsTerminalStatus(status)
}

func personalityStatusView(status *personalityassessment.AssessmentStatusResponse) *StatusPayload {
	if status == nil {
		return nil
	}
	return &StatusPayload{
		Status:          status.Status,
		Stage:           status.Stage,
		Message:         status.Message,
		Reason:          status.Reason,
		NextPollAfterMs: status.NextPollAfterMs,
		UpdatedAt:       status.UpdatedAt,
	}
}
