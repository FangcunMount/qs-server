package service

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/protobuf/types/known/timestamppb"
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
		operatorLifecycleService: &bootstrapLifecycleServiceStub{},
		operatorQueryService:     &bootstrapQueryServiceStub{},
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

func TestBootstrapOperatorRejectsNilRequest(t *testing.T) {
	svc := &InternalService{
		operatorLifecycleService: &bootstrapLifecycleServiceStub{},
		operatorQueryService:     &bootstrapQueryServiceStub{},
	}

	_, err := svc.BootstrapOperator(context.Background(), nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("BootstrapOperator(nil) = %s, want %s", status.Code(err), codes.InvalidArgument)
	}
}

func TestCreateAssessmentFromAnswerSheetRejectsNilRequest(t *testing.T) {
	svc := &InternalService{}

	_, err := svc.CreateAssessmentFromAnswerSheet(context.Background(), nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateAssessmentFromAnswerSheet(nil) = %s, want %s", status.Code(err), codes.InvalidArgument)
	}
}

func TestCalculateAnswerSheetScoreRejectsNilRequest(t *testing.T) {
	svc := &InternalService{}

	resp, err := svc.CalculateAnswerSheetScore(context.Background(), nil)
	if err != nil {
		t.Fatalf("CalculateAnswerSheetScore(nil) error = %v", err)
	}
	if resp.Success {
		t.Fatalf("CalculateAnswerSheetScore(nil) success = true, want false")
	}
	if resp.Message != "answersheet_id 不能为空" {
		t.Fatalf("message = %q, want answersheet_id 不能为空", resp.Message)
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

func TestBootstrapOperatorRunsLifecycleAndRoleSync(t *testing.T) {
	query := &bootstrapQueryServiceStub{
		getByUserResults: []*operatorApp.OperatorResult{
			nil,
			{
				ID:       77,
				OrgID:    9,
				UserID:   101,
				Name:     "Alice",
				IsActive: true,
				Roles:    []string{"qs:admin"},
			},
		},
		getByUserErrs: []error{
			cberrors.WithCode(errorCode.ErrUserNotFound, "operator not found"),
			nil,
		},
	}
	lifecycle := &bootstrapLifecycleServiceStub{
		ensureByUserResult: &operatorApp.OperatorResult{ID: 77},
	}
	auth := &bootstrapAuthorizationServiceStub{}
	roleSyncer := &bootstrapRoleSyncerStub{}
	svc := &InternalService{
		operatorLifecycleService: lifecycle,
		operatorAuthService:      auth,
		operatorQueryService:     query,
		operatorRoleSyncer:       roleSyncer,
	}

	resp, err := svc.BootstrapOperator(context.Background(), &pb.BootstrapOperatorRequest{
		OrgId:    9,
		UserId:   101,
		Name:     "Alice",
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("BootstrapOperator() error = %v", err)
	}
	if !resp.Created || resp.OperatorId != 77 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if lifecycle.ensureByUserCalls != 1 || lifecycle.updateFromExternalSourceCalls != 1 {
		t.Fatalf("unexpected lifecycle calls: ensure=%d sync=%d", lifecycle.ensureByUserCalls, lifecycle.updateFromExternalSourceCalls)
	}
	if auth.activateCalls != 1 || auth.deactivateCalls != 0 {
		t.Fatalf("unexpected auth calls: activate=%d deactivate=%d", auth.activateCalls, auth.deactivateCalls)
	}
	if roleSyncer.calls != 1 || roleSyncer.lastOrgID != 9 || roleSyncer.lastOperatorID != 77 {
		t.Fatalf("unexpected role sync invocation: %+v", roleSyncer)
	}
}

func TestProjectBehaviorEventRequiresProjectorService(t *testing.T) {
	svc := &InternalService{}

	_, err := svc.ProjectBehaviorEvent(context.Background(), &pb.ProjectBehaviorEventRequest{
		EventId:    "evt-1",
		EventType:  "behavior.opened",
		OrgId:      9,
		OccurredAt: timestamppb.Now(),
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("ProjectBehaviorEvent() = %s, want %s", status.Code(err), codes.FailedPrecondition)
	}
}

func TestSendTaskOpenedMiniProgramNotificationWithoutServiceIsSkipped(t *testing.T) {
	svc := &InternalService{}

	resp, err := svc.SendTaskOpenedMiniProgramNotification(context.Background(), &pb.SendTaskOpenedMiniProgramNotificationRequest{
		TaskId:   "task-1",
		TesteeId: 10,
	})
	if err != nil {
		t.Fatalf("SendTaskOpenedMiniProgramNotification() error = %v", err)
	}
	if resp == nil || !resp.Skipped || resp.Success {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHandleQuestionnairePublishedPostActionsSucceedsWithoutWarmupCoordinator(t *testing.T) {
	svc := &InternalService{
		qrCodeService: &grpcQRCodeServiceStub{questionnaireURL: "https://example.com/qnr.png"},
	}

	resp, err := svc.HandleQuestionnairePublishedPostActions(context.Background(), &pb.GenerateQuestionnaireQRCodeRequest{
		Code:    "QNR-1",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("HandleQuestionnairePublishedPostActions() error = %v", err)
	}
	if resp == nil || !resp.Success || resp.QrcodeUrl != "https://example.com/qnr.png" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHandleScalePublishedPostActionsSucceedsWithoutWarmupCoordinator(t *testing.T) {
	svc := &InternalService{
		qrCodeService: &grpcQRCodeServiceStub{scaleURL: "https://example.com/scale.png"},
	}

	resp, err := svc.HandleScalePublishedPostActions(context.Background(), &pb.GenerateScaleQRCodeRequest{
		Code: "SCL-1",
	})
	if err != nil {
		t.Fatalf("HandleScalePublishedPostActions() error = %v", err)
	}
	if resp == nil || !resp.Success || resp.QrcodeUrl != "https://example.com/scale.png" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestBuildBootstrapOperatorResponseCopiesRoles(t *testing.T) {
	roles := []string{"qs:admin"}
	resp := buildBootstrapOperatorResponse(&operatorApp.OperatorResult{
		ID:    7,
		Roles: roles,
	}, true)

	roles[0] = "changed"

	if resp.OperatorId != 7 || !resp.Created {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.Roles) != 1 || resp.Roles[0] != "qs:admin" {
		t.Fatalf("roles = %v, want copied original roles", resp.Roles)
	}
}

type bootstrapLifecycleServiceStub struct {
	ensureByUserResult            *operatorApp.OperatorResult
	ensureByUserErr               error
	ensureByUserCalls             int
	updateFromExternalSourceCalls int
}

func (*bootstrapLifecycleServiceStub) Register(_ context.Context, _ operatorApp.RegisterOperatorDTO) (*operatorApp.OperatorResult, error) {
	return nil, nil
}
func (s *bootstrapLifecycleServiceStub) EnsureByUser(_ context.Context, _, _ int64, _ string) (*operatorApp.OperatorResult, error) {
	s.ensureByUserCalls++
	return s.ensureByUserResult, s.ensureByUserErr
}
func (*bootstrapLifecycleServiceStub) Delete(_ context.Context, _ uint64) error {
	return nil
}
func (*bootstrapLifecycleServiceStub) UpdateProfile(_ context.Context, _ operatorApp.UpdateOperatorProfileDTO) (*operatorApp.OperatorResult, error) {
	return nil, nil
}
func (*bootstrapLifecycleServiceStub) UpdateContactInfo(_ context.Context, _ operatorApp.UpdateOperatorContactDTO) error {
	return nil
}
func (s *bootstrapLifecycleServiceStub) UpdateFromExternalSource(_ context.Context, _ uint64, _, _, _ string) error {
	s.updateFromExternalSourceCalls++
	return nil
}

type bootstrapAuthorizationServiceStub struct {
	activateCalls   int
	deactivateCalls int
}

func (*bootstrapAuthorizationServiceStub) AssignRole(context.Context, uint64, string) error {
	return nil
}
func (*bootstrapAuthorizationServiceStub) RemoveRole(context.Context, uint64, string) error {
	return nil
}
func (s *bootstrapAuthorizationServiceStub) Activate(context.Context, uint64) error {
	s.activateCalls++
	return nil
}
func (s *bootstrapAuthorizationServiceStub) Deactivate(context.Context, uint64) error {
	s.deactivateCalls++
	return nil
}

type bootstrapRoleSyncerStub struct {
	calls          int
	lastOrgID      int64
	lastOperatorID uint64
}

func (s *bootstrapRoleSyncerStub) SyncRoles(_ context.Context, orgID int64, operatorID uint64) error {
	s.calls++
	s.lastOrgID = orgID
	s.lastOperatorID = operatorID
	return nil
}

type bootstrapQueryServiceStub struct {
	getByUserResults []*operatorApp.OperatorResult
	getByUserErrs    []error
	getByUserCalls   int
}

func (*bootstrapQueryServiceStub) GetByID(_ context.Context, _ uint64) (*operatorApp.OperatorResult, error) {
	return nil, nil
}
func (s *bootstrapQueryServiceStub) GetByUser(_ context.Context, _ int64, _ int64) (*operatorApp.OperatorResult, error) {
	call := s.getByUserCalls
	s.getByUserCalls++
	var result *operatorApp.OperatorResult
	if call < len(s.getByUserResults) {
		result = s.getByUserResults[call]
	}
	var err error
	if call < len(s.getByUserErrs) {
		err = s.getByUserErrs[call]
	}
	return result, err
}
func (*bootstrapQueryServiceStub) ListOperators(_ context.Context, _ operatorApp.ListOperatorDTO) (*operatorApp.OperatorListResult, error) {
	return nil, nil
}

type grpcQRCodeServiceStub struct {
	questionnaireURL string
	scaleURL         string
}

func (s *grpcQRCodeServiceStub) GenerateQuestionnaireQRCode(context.Context, string, string) (string, error) {
	return s.questionnaireURL, nil
}

func (s *grpcQRCodeServiceStub) GenerateScaleQRCode(context.Context, string) (string, error) {
	return s.scaleURL, nil
}

func (*grpcQRCodeServiceStub) GenerateAssessmentEntryQRCode(context.Context, string) (string, error) {
	return "", nil
}

func uint64Ptr(v uint64) *uint64 { return &v }
func stringPtr(v string) *string { return &v }

var _ qrcodeApp.QRCodeService = (*grpcQRCodeServiceStub)(nil)
