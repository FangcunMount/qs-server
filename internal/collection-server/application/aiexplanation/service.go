// Package aiexplanation exposes participant-facing BFF use cases for the
// optional AI supplement without coupling standard report reads to it.
package aiexplanation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
)

var (
	ErrInvalidRequest = errors.New("invalid AI explanation request")
	ErrUnavailable    = errors.New("AI explanation service unavailable")
)

type Request struct {
	Locale     string   `json:"locale" binding:"required"`
	FocusAreas []string `json:"focus_areas" validate:"max=3"`
}

type Response = aiport.Output
type ExportPage = aiport.ExportPage
type ExportItem = aiport.ExportItem

type Service struct {
	client aiport.Client
}

func NewService(client aiport.Client) *Service {
	return &Service{client: client}
}

func (s *Service) Capability(ctx context.Context, testeeID, assessmentID uint64, request Request) (*Response, error) {
	if err := validateRequest(testeeID, assessmentID, request); err != nil {
		return nil, err
	}
	if s == nil || s.client == nil {
		return disabledResponse(), nil
	}
	result, err := s.client.GetCapability(ctx, testeeID, assessmentID, request.Locale, request.FocusAreas)
	return normalizeResult(result, err)
}

func (s *Service) Request(ctx context.Context, testeeID, assessmentID uint64, request Request) (*Response, error) {
	if err := validateRequest(testeeID, assessmentID, request); err != nil {
		return nil, err
	}
	if s == nil || s.client == nil {
		return disabledResponse(), nil
	}
	result, err := s.client.Request(ctx, testeeID, assessmentID, request.Locale, request.FocusAreas)
	return normalizeResult(result, err)
}

func (s *Service) Get(ctx context.Context, testeeID, assessmentID uint64, generationID string) (*Response, error) {
	if testeeID == 0 || assessmentID == 0 || strings.TrimSpace(generationID) == "" {
		return nil, fmt.Errorf("%w: testee, assessment and AI explanation generation are required", ErrInvalidRequest)
	}
	if s == nil || s.client == nil {
		return disabledResponse(), nil
	}
	result, err := s.client.Get(ctx, testeeID, assessmentID, generationID)
	return normalizeResult(result, err)
}

// Export returns a privacy projection and therefore fails closed when the
// upstream capability is unavailable; an outage must never look like an empty
// personal-data export.
func (s *Service) Export(ctx context.Context, testeeID uint64, pageSize int, cursor string) (*ExportPage, error) {
	if testeeID == 0 || pageSize < 0 || pageSize > 100 || len(strings.TrimSpace(cursor)) > 8192 {
		return nil, fmt.Errorf("%w: valid Testee, page size and cursor are required", ErrInvalidRequest)
	}
	if s == nil || s.client == nil {
		return nil, ErrUnavailable
	}
	result, err := s.client.Export(ctx, testeeID, pageSize, strings.TrimSpace(cursor))
	if errors.Is(err, aiport.ErrDisabled) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, err
	}
	if result == nil || strings.TrimSpace(result.SchemaVersion) == "" || result.TesteeID != testeeID || result.Items == nil {
		return nil, fmt.Errorf("AI explanation export service returned an invalid result")
	}
	return result, nil
}

func validateRequest(testeeID, assessmentID uint64, request Request) error {
	if testeeID == 0 || assessmentID == 0 || strings.TrimSpace(request.Locale) == "" {
		return fmt.Errorf("%w: testee, assessment and locale are required", ErrInvalidRequest)
	}
	if len(request.FocusAreas) > 3 {
		return fmt.Errorf("%w: AI explanation focus areas exceed limit", ErrInvalidRequest)
	}
	return nil
}

func normalizeResult(result *aiport.Output, err error) (*Response, error) {
	if errors.Is(err, aiport.ErrDisabled) {
		return disabledResponse(), nil
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("AI explanation service returned no result")
	}
	if err := validateResult(result); err != nil {
		return nil, fmt.Errorf("AI explanation service returned an invalid result: %w", err)
	}
	return result, nil
}

func validateResult(result *aiport.Output) error {
	status := strings.TrimSpace(result.Status)
	reasonCode := strings.TrimSpace(result.ReasonCode)
	sourceState := strings.TrimSpace(result.SourceState)
	switch sourceState {
	case "current", "stale", "unavailable", "unknown":
	default:
		return fmt.Errorf("unsupported source state %q", sourceState)
	}

	switch status {
	case "ready":
		if reasonCode != "" {
			return fmt.Errorf("ready result must not contain reason code")
		}
		if strings.TrimSpace(result.SourceReportID) == "" {
			return fmt.Errorf("ready result requires source report")
		}
	case "not_ready":
		if reasonCode != aiport.ReasonCodeStandardReportNotReady {
			return fmt.Errorf("not_ready result requires standard_report_not_ready reason code")
		}
	case "not_applicable":
		if !isNotApplicableReasonCode(reasonCode) {
			return fmt.Errorf("not_applicable result contains unsupported reason code %q", reasonCode)
		}
	case "pending", "generating":
		if reasonCode != "" {
			return fmt.Errorf("%s result must not contain reason code", status)
		}
		if strings.TrimSpace(result.GenerationID) == "" {
			return fmt.Errorf("%s result requires generation", status)
		}
	case "generated":
		if reasonCode != "" {
			return fmt.Errorf("generated result must not contain reason code")
		}
		if strings.TrimSpace(result.GenerationID) == "" || strings.TrimSpace(result.ArtifactID) == "" || result.Content == nil ||
			strings.TrimSpace(result.Content.SchemaVersion) == "" {
			return fmt.Errorf("generated result requires generation, artifact and content")
		}
	case "failed":
		if reasonCode != "" {
			return fmt.Errorf("failed result must not contain reason code")
		}
		if strings.TrimSpace(result.GenerationID) == "" || result.Failure == nil || strings.TrimSpace(result.Failure.Code) == "" {
			return fmt.Errorf("failed result requires generation and failure")
		}
	default:
		return fmt.Errorf("unsupported status %q", status)
	}
	return nil
}

func isNotApplicableReasonCode(reasonCode string) bool {
	switch reasonCode {
	case aiport.ReasonCodeFeatureDisabled,
		aiport.ReasonCodeSourceNotSupported,
		aiport.ReasonCodeProfileUnresolved,
		aiport.ReasonCodeProfileMismatch,
		aiport.ReasonCodeNotApplicable:
		return true
	default:
		return false
	}
}

func disabledResponse() *Response {
	return &Response{Status: "not_applicable", ReasonCode: aiport.ReasonCodeFeatureDisabled, SourceState: "unavailable"}
}
