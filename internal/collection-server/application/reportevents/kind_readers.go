package reportevents

import (
	"context"
	"fmt"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalityassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
)

type medicalKindReader struct {
	medical interface {
		GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentDetailResponse, error)
	}
	waitReport interface {
		GetStatus(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentStatusResponse, error)
	}
}

func (m medicalKindReader) Authorize(ctx context.Context, testeeID, assessmentID uint64) error {
	if m.medical == nil {
		return fmt.Errorf("medical query service is not configured")
	}
	result, err := m.medical.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		return err
	}
	if result == nil {
		return reportstatus.ErrAssessmentAccess
	}
	return nil
}

func (m medicalKindReader) CurrentStatus(ctx context.Context, testeeID, assessmentID uint64) (*reportstatus.View, error) {
	if m.waitReport == nil {
		return nil, fmt.Errorf("wait-report service is not configured")
	}
	status, err := m.waitReport.GetStatus(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return reportstatus.MedicalView(reportstatus.ToPublicAssessmentStatus(status)), nil
}

type personalityKindReader struct {
	personality interface {
		Get(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentDetailResponse, error)
		GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentStatusResponse, error)
	}
}

func (p personalityKindReader) Authorize(ctx context.Context, testeeID, assessmentID uint64) error {
	if p.personality == nil {
		return fmt.Errorf("personality query service is not configured")
	}
	result, err := p.personality.Get(ctx, testeeID, assessmentID)
	if err != nil {
		return err
	}
	if result == nil {
		return reportstatus.ErrAssessmentAccess
	}
	return nil
}

func (p personalityKindReader) CurrentStatus(ctx context.Context, testeeID, assessmentID uint64) (*reportstatus.View, error) {
	if p.personality == nil {
		return nil, fmt.Errorf("personality query service is not configured")
	}
	status, err := p.personality.GetReportStatus(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return personalityStatusView(status), nil
}

func personalityStatusView(status *personalityassessment.AssessmentStatusResponse) *reportstatus.View {
	if status == nil {
		return nil
	}
	return &reportstatus.View{
		Status:          status.Status,
		Stage:           status.Stage,
		Message:         status.Message,
		Reason:          status.Reason,
		NextPollAfterMs: status.NextPollAfterMs,
		UpdatedAt:       status.UpdatedAt,
	}
}

// NewDefaultResolver 用 collection 应用服务构造默认 Resolver。
func NewDefaultResolver(
	medical interface {
		GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentDetailResponse, error)
	},
	waitReport interface {
		GetStatus(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentStatusResponse, error)
	},
	personality interface {
		Get(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentDetailResponse, error)
		GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentStatusResponse, error)
	},
) *reportstatus.Resolver {
	return reportstatus.NewResolver(map[string]reportstatus.KindReader{
		reportstatus.KindMedical: medicalKindReader{
			medical:    medical,
			waitReport: waitReport,
		},
		reportstatus.KindPersonality: personalityKindReader{personality: personality},
	})
}
