package service

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
)

type AssessmentIntakeService struct {
	pb.UnimplementedAssessmentIntakeServiceServer
	journey journey.Service
	intake  evaluationintake.Service
}

func NewAssessmentIntakeService(journey journey.Service, intake evaluationintake.Service) *AssessmentIntakeService {
	return &AssessmentIntakeService{journey: journey, intake: intake}
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
	result, err := s.journey.Ensure(ctx, journey.Command{OrgID: orgID, AnswerSheetID: req.AnswerSheetId, QuestionnaireCode: req.QuestionnaireCode, QuestionnaireVersion: req.QuestionnaireVersion, TesteeID: req.TesteeId, FillerID: req.FillerId, TaskID: req.TaskId, OriginType: req.OriginType, OriginID: req.OriginId})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
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
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	return &pb.ResolveAssessmentByAnswerSheetIDResponse{TesteeId: result.TesteeID, AssessmentId: result.ID}, nil
}
