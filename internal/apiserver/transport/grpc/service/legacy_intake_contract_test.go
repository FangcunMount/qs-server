package service

// This file keeps decoder-level characterization for the retired Internal RPC
// messages while production callers migrate stored payloads. No server method
// or application orchestration depends on these helpers.

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func validateCreateAssessmentFromAnswerSheetRequest(req *pb.CreateAssessmentFromAnswerSheetRequest) error {
	switch {
	case req == nil:
		return status.Error(codes.InvalidArgument, "request 不能为空")
	case req.AnswersheetId == 0:
		return status.Error(codes.InvalidArgument, "answersheet_id 不能为空")
	case req.QuestionnaireCode == "":
		return status.Error(codes.InvalidArgument, "questionnaire_code 不能为空")
	case req.QuestionnaireVersion == "":
		return status.Error(codes.InvalidArgument, "questionnaire_version 不能为空")
	case req.TesteeId == 0:
		return status.Error(codes.InvalidArgument, "testee_id 不能为空")
	case req.FillerId == 0:
		return status.Error(codes.InvalidArgument, "filler_id 不能为空")
	default:
		return nil
	}
}

func buildCreateAssessmentDTO(ctx context.Context, req *pb.CreateAssessmentFromAnswerSheetRequest, resolver rulesetport.AssessmentBindingResolver) (assessmentApp.CreateAssessmentDTO, error) {
	dto := assessmentApp.CreateAssessmentDTO{OrgID: req.OrgId, TesteeID: req.TesteeId, QuestionnaireCode: req.QuestionnaireCode, QuestionnaireVersion: req.QuestionnaireVersion, AnswerSheetID: req.AnswersheetId, OriginType: req.OriginType}
	if dto.OriginType == "" {
		dto.OriginType = "adhoc"
	}
	if req.OriginId != "" {
		dto.OriginID = &req.OriginId
	}
	if resolver == nil {
		return dto, nil
	}
	binding, ok, err := resolver.ResolveAssessmentBinding(ctx, req.QuestionnaireCode, req.QuestionnaireVersion)
	if err != nil || !ok {
		return dto, err
	}
	kind, subKind, algorithm, mapped := modelcatalog.LegacyKindMapping(binding.Ref.Kind)
	if !mapped {
		kind = binding.Ref.Kind
	}
	if binding.Ref.SubKind != "" {
		subKind = binding.Ref.SubKind
	}
	if binding.Ref.Algorithm != "" {
		algorithm = binding.Ref.Algorithm
	}
	k := kind.String()
	dto.ModelKind = &k
	if subKind != "" {
		v := subKind.String()
		dto.ModelSubKind = &v
	}
	if algorithm != "" {
		v := algorithm.String()
		dto.ModelAlgorithm = &v
	}
	dto.ModelCode = &binding.Ref.Code
	dto.ModelVersion = &binding.Ref.Version
	dto.ModelTitle = &binding.Ref.Title
	return dto, nil
}

func shouldAutoSubmitAssessment(dto assessmentApp.CreateAssessmentDTO) bool {
	return dto.ModelCode != nil
}
func existingAssessmentResponse(id uint64) *pb.CreateAssessmentFromAnswerSheetResponse {
	return &pb.CreateAssessmentFromAnswerSheetResponse{Success: true, AssessmentId: id, Message: "测评已存在"}
}
func createdAssessmentResponse(id uint64, submitted bool) *pb.CreateAssessmentFromAnswerSheetResponse {
	return &pb.CreateAssessmentFromAnswerSheetResponse{Success: true, AssessmentId: id, Created: true, AutoSubmitted: submitted, Message: "测评创建成功"}
}
