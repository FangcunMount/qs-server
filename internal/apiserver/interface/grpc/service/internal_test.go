package service

import (
	"context"
	"testing"

	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestValidateCreateAssessmentFromAnswerSheetRequest(t *testing.T) {
	tests := []struct {
		name string
		req  *pb.CreateAssessmentFromAnswerSheetRequest
		want codes.Code
	}{
		{
			name: "missing answersheet",
			req:  &pb.CreateAssessmentFromAnswerSheetRequest{},
			want: codes.InvalidArgument,
		},
		{
			name: "valid request",
			req: &pb.CreateAssessmentFromAnswerSheetRequest{
				AnswersheetId:        1,
				QuestionnaireCode:    "QNR-001",
				QuestionnaireVersion: "1.0.0",
				TesteeId:             2,
				FillerId:             3,
			},
			want: codes.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateAssessmentFromAnswerSheetRequest(tt.req)
			if status.Code(err) != tt.want {
				t.Fatalf("validateCreateAssessmentFromAnswerSheetRequest() = %s, want %s", status.Code(err), tt.want)
			}
		})
	}
}

func TestBuildCreateAssessmentDTODefaultsOriginType(t *testing.T) {
	req := &pb.CreateAssessmentFromAnswerSheetRequest{
		OrgId:                9,
		TesteeId:             101,
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		AnswersheetId:        202,
	}
	scaleCtx := assessmentScaleContext{
		medicalScaleID:   uint64Ptr(8),
		medicalScaleCode: stringPtr("SCL-001"),
		medicalScaleName: stringPtr("Scale"),
	}

	dto := buildCreateAssessmentDTO(req, scaleCtx)
	if dto.OriginType != "adhoc" {
		t.Fatalf("expected adhoc origin type, got %q", dto.OriginType)
	}
	if dto.MedicalScaleID == nil || *dto.MedicalScaleID != 8 {
		t.Fatalf("expected medical scale id 8, got %#v", dto.MedicalScaleID)
	}
	if dto.MedicalScaleCode == nil || *dto.MedicalScaleCode != "SCL-001" {
		t.Fatalf("expected medical scale code SCL-001, got %#v", dto.MedicalScaleCode)
	}
}

func TestValidateBootstrapOperatorRequest(t *testing.T) {
	svc := &InternalService{
		operatorLifecycleService: bootstrapLifecycleServiceStub{},
		operatorQueryService:     bootstrapQueryServiceStub{},
	}

	err := validateBootstrapOperatorRequest(svc, &pb.BootstrapOperatorRequest{
		OrgId:  9,
		UserId: 101,
		Name:   "Alice",
	})
	if err != nil {
		t.Fatalf("validateBootstrapOperatorRequest() error = %v", err)
	}

	err = validateBootstrapOperatorRequest(&InternalService{}, &pb.BootstrapOperatorRequest{
		OrgId:  9,
		UserId: 101,
		Name:   "Alice",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %s", status.Code(err))
	}
}

func TestBootstrapOperatorMessage(t *testing.T) {
	if got := bootstrapOperatorMessage(true); got != "operator bootstrapped" {
		t.Fatalf("expected created bootstrap message, got %q", got)
	}
	if got := bootstrapOperatorMessage(false); got != "operator already exists" {
		t.Fatalf("expected existing bootstrap message, got %q", got)
	}
}

type bootstrapLifecycleServiceStub struct{}

func (bootstrapLifecycleServiceStub) Register(_ context.Context, _ operatorApp.RegisterOperatorDTO) (*operatorApp.OperatorResult, error) {
	return nil, nil
}
func (bootstrapLifecycleServiceStub) EnsureByUser(_ context.Context, _, _ int64, _ string) (*operatorApp.OperatorResult, error) {
	return nil, nil
}
func (bootstrapLifecycleServiceStub) Delete(_ context.Context, _ uint64) error {
	return nil
}
func (bootstrapLifecycleServiceStub) UpdateProfile(_ context.Context, _ operatorApp.UpdateOperatorProfileDTO) (*operatorApp.OperatorResult, error) {
	return nil, nil
}
func (bootstrapLifecycleServiceStub) UpdateContactInfo(_ context.Context, _ operatorApp.UpdateOperatorContactDTO) error {
	return nil
}
func (bootstrapLifecycleServiceStub) UpdateFromExternalSource(_ context.Context, _ uint64, _, _, _ string) error {
	return nil
}

type bootstrapQueryServiceStub struct{}

func (bootstrapQueryServiceStub) GetByID(_ context.Context, _ uint64) (*operatorApp.OperatorResult, error) {
	return nil, nil
}
func (bootstrapQueryServiceStub) GetByUser(_ context.Context, _ int64, _ int64) (*operatorApp.OperatorResult, error) {
	return nil, nil
}
func (bootstrapQueryServiceStub) ListOperators(_ context.Context, _ operatorApp.ListOperatorDTO) (*operatorApp.OperatorListResult, error) {
	return nil, nil
}

func uint64Ptr(v uint64) *uint64 { return &v }
func stringPtr(v string) *string { return &v }
