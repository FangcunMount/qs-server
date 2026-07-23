package service

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	answersheetapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
)

const (
	readinessPhasePending              = "pending"
	readinessPhaseNoAssessmentRequired = "no_assessment_required"
	readinessPhaseReady                = "ready"
	readinessPhaseFailed               = "failed"
	assessmentStatusPending            = "pending"
	assessmentStatusSubmitted          = "submitted"
	assessmentStatusEvaluated          = "evaluated"
	assessmentStatusFailed             = "failed"
)

type AssessmentIntakeService struct {
	pb.UnimplementedAssessmentIntakeServiceServer
	journey journey.Service
	intake  evaluationintake.Service
	sheets  answersheetapp.AnswerSheetManagementService
}

func NewAssessmentIntakeService(journey journey.Service, intake evaluationintake.Service, sheets answersheetapp.AnswerSheetManagementService) *AssessmentIntakeService {
	return &AssessmentIntakeService{journey: journey, intake: intake, sheets: sheets}
}

func (s *AssessmentIntakeService) RegisterService(server *grpc.Server) {
	pb.RegisterAssessmentIntakeServiceServer(server, s)
}

func (s *AssessmentIntakeService) EnsureAssessment(ctx context.Context, req *pb.EnsureAssessmentRequest) (*pb.EnsureAssessmentResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request 不能为空")
	}
	orgID := req.OrgId
	if orgID == 0 {
		var err error
		orgID, err = requestOrgIDUint64(ctx, 0)
		if err != nil {
			return nil, err
		}
	}
	logger.L(ctx).Infow("gRPC: received ensure assessment request",
		"answersheet_id", req.AnswerSheetId,
		"org_id", orgID,
		"testee_id", req.TesteeId,
		"questionnaire_code", req.QuestionnaireCode,
		"questionnaire_version", req.QuestionnaireVersion,
	)
	result, err := s.journey.Ensure(ctx, journey.Command{
		OrgID: orgID, AnswerSheetID: req.AnswerSheetId,
		QuestionnaireCode: req.QuestionnaireCode, QuestionnaireVersion: req.QuestionnaireVersion,
		TesteeID: req.TesteeId, FillerID: req.FillerId, TaskID: req.TaskId,
		OriginType: req.OriginType, OriginID: req.OriginId,
		Admission: admissionFromProto(req.GetAdmission()),
	})
	if err != nil {
		return nil, toEvaluationGRPCError(err)
	}
	logger.L(ctx).Infow("gRPC: ensure assessment completed",
		"answersheet_id", req.AnswerSheetId,
		"assessment_id", result.AssessmentID,
		"created", result.Created,
		"auto_submitted", result.AutoSubmitted,
	)
	return &pb.EnsureAssessmentResponse{AssessmentId: result.AssessmentID, Created: result.Created, AutoSubmitted: result.AutoSubmitted}, nil
}

func (s *AssessmentIntakeService) ResolveAssessmentByAnswerSheetID(ctx context.Context, req *pb.ResolveAssessmentByAnswerSheetIDRequest) (*pb.ResolveAssessmentByAnswerSheetIDResponse, error) {
	if req == nil || req.AnswerSheetId == 0 {
		return nil, status.Error(codes.InvalidArgument, "answer_sheet_id 不能为空")
	}

	result, err := s.intake.FindByAnswerSheetID(ctx, req.AnswerSheetId)
	if err == nil && result != nil {
		return readinessFromAssessment(result), nil
	}
	if err != nil && !evalerrors.IsAssessmentNotFound(err) {
		return nil, toAssessmentQueryGRPCError(err)
	}

	// No Assessment row: classify via AnswerSheet admission (EV-R007).
	if s.sheets == nil {
		return nil, status.Error(codes.NotFound, "assessment not found")
	}
	sheet, sheetErr := s.sheets.GetByID(ctx, req.AnswerSheetId)
	if sheetErr != nil {
		return nil, toAssessmentQueryGRPCError(sheetErr)
	}
	if sheet == nil {
		return nil, status.Error(codes.NotFound, "answer sheet not found")
	}

	phase := readinessPhasePending
	if sheet.AdmissionPurpose == string(domainanswersheet.AdmissionPurposeIndependentQuestionnaire) {
		phase = readinessPhaseNoAssessmentRequired
	}
	return &pb.ResolveAssessmentByAnswerSheetIDResponse{
		TesteeId:         sheet.TesteeID,
		AssessmentId:     0,
		ReadinessPhase:   phase,
		AssessmentStatus: "",
	}, nil
}

func readinessFromAssessment(result *evaluationintake.Assessment) *pb.ResolveAssessmentByAnswerSheetIDResponse {
	phase := readinessPhasePending
	switch result.Status {
	case assessmentStatusSubmitted, assessmentStatusEvaluated:
		phase = readinessPhaseReady
	case assessmentStatusFailed:
		phase = readinessPhaseFailed
	case assessmentStatusPending:
		phase = readinessPhasePending
	}
	failureReason := ""
	if result.FailureReason != nil {
		failureReason = *result.FailureReason
	}
	return &pb.ResolveAssessmentByAnswerSheetIDResponse{
		TesteeId:         result.TesteeID,
		AssessmentId:     result.ID,
		ReadinessPhase:   phase,
		AssessmentStatus: result.Status,
		FailureReason:    failureReason,
	}
}

func admissionFromProto(in *pb.AssessmentAdmission) *journey.Admission {
	if in == nil || in.GetPurpose() == "" {
		return nil
	}
	return &journey.Admission{
		Purpose:              in.GetPurpose(),
		QuestionnaireCode:    in.GetQuestionnaireCode(),
		QuestionnaireVersion: in.GetQuestionnaireVersion(),
		ModelKind:            in.GetModelKind(),
		ModelAlgorithm:       in.GetModelAlgorithm(),
		ModelCode:            in.GetModelCode(),
		ModelVersion:         in.GetModelVersion(),
		ModelTitle:           in.GetModelTitle(),
	}
}
