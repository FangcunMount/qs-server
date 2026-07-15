package container

import (
	"context"
	"fmt"

	behaviorassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/behaviorassessment"
	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologyassessment"
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
	return reportstatus.MedicalView(status), nil
}

type typologyKindReader struct {
	typology interface {
		Get(ctx context.Context, testeeID, assessmentID uint64) (*typologyassessment.AssessmentDetailResponse, error)
		GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*typologyassessment.AssessmentStatusResponse, error)
	}
}

type behaviorKindReader struct {
	behavior interface {
		Get(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentDetailResponse, error)
		GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentStatusResponse, error)
	}
}

func (p behaviorKindReader) Authorize(ctx context.Context, testeeID, assessmentID uint64) error {
	if p.behavior == nil {
		return fmt.Errorf("behavior assessment query service is not configured")
	}
	result, err := p.behavior.Get(ctx, testeeID, assessmentID)
	if err != nil {
		return err
	}
	if result == nil {
		return reportstatus.ErrAssessmentAccess
	}
	return nil
}

func (p behaviorKindReader) CurrentStatus(ctx context.Context, testeeID, assessmentID uint64) (*reportstatus.View, error) {
	if p.behavior == nil {
		return nil, fmt.Errorf("behavior assessment query service is not configured")
	}
	status, err := p.behavior.GetReportStatus(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	if status == nil {
		return nil, nil
	}
	return reportstatus.ViewFromFields(reportstatus.StatusFields{Status: status.Status, Stage: status.Stage, Message: status.Message, Reason: status.Reason, NextPollAfterMs: status.NextPollAfterMs, UpdatedAt: status.UpdatedAt}), nil
}

func (p typologyKindReader) Authorize(ctx context.Context, testeeID, assessmentID uint64) error {
	if p.typology == nil {
		return fmt.Errorf("typology query service is not configured")
	}
	result, err := p.typology.Get(ctx, testeeID, assessmentID)
	if err != nil {
		return err
	}
	if result == nil {
		return reportstatus.ErrAssessmentAccess
	}
	return nil
}

func (p typologyKindReader) CurrentStatus(ctx context.Context, testeeID, assessmentID uint64) (*reportstatus.View, error) {
	if p.typology == nil {
		return nil, fmt.Errorf("typology query service is not configured")
	}
	status, err := p.typology.GetReportStatus(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	if status == nil {
		return nil, nil
	}
	return reportstatus.PersonalityView(reportstatus.StatusFields{
		Status:          status.Status,
		Stage:           status.Stage,
		Message:         status.Message,
		Reason:          status.Reason,
		NextPollAfterMs: status.NextPollAfterMs,
		UpdatedAt:       status.UpdatedAt,
	}), nil
}

func newReportStatusResolver(
	medical interface {
		GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentDetailResponse, error)
	},
	waitReport interface {
		GetStatus(ctx context.Context, testeeID, assessmentID uint64) (*evaluationapp.AssessmentStatusResponse, error)
	},
	typology interface {
		Get(ctx context.Context, testeeID, assessmentID uint64) (*typologyassessment.AssessmentDetailResponse, error)
		GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*typologyassessment.AssessmentStatusResponse, error)
	},
	behavior interface {
		Get(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentDetailResponse, error)
		GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentStatusResponse, error)
	},
) *reportstatus.Resolver {
	return reportstatus.NewResolver(map[string]reportstatus.KindReader{
		reportstatus.KindMedical: medicalKindReader{
			medical:    medical,
			waitReport: waitReport,
		},
		reportstatus.KindPersonality: typologyKindReader{typology: typology},
		reportstatus.KindBehavior:    behaviorKindReader{behavior: behavior},
	})
}
