package service

import (
	"context"
	"fmt"

	"github.com/fangcun-mount/qs-server/internal/apiserver/application/dto"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/medical-scale/port"
	pb "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/medical-scale"
	"github.com/fangcun-mount/qs-server/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MedicalScaleService 医学量表 gRPC 服务
type MedicalScaleService struct {
	pb.UnimplementedMedicalScaleServiceServer
	medicalScaleQueryer port.MedicalScaleQueryer
}

// NewMedicalScaleService 创建医学量表服务
func NewMedicalScaleService(queryer port.MedicalScaleQueryer) *MedicalScaleService {
	return &MedicalScaleService{
		medicalScaleQueryer: queryer,
	}
}

// RegisterService 注册 GRPC 服务
func (s *MedicalScaleService) RegisterService(server *grpc.Server) {
	pb.RegisterMedicalScaleServiceServer(server, s)
}

// GetMedicalScaleByCode 根据医学量表代码获取医学量表详情
func (s *MedicalScaleService) GetMedicalScaleByCode(ctx context.Context, req *pb.GetMedicalScaleByCodeRequest) (*pb.GetMedicalScaleByCodeResponse, error) {
	if req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "医学量表代码不能为空")
	}

	log.Infof("获取医学量表详情，代码: %s", req.Code)

	// 查询医学量表
	medicalScale, err := s.medicalScaleQueryer.GetMedicalScaleByCode(ctx, req.Code)
	if err != nil {
		log.Errorf("获取医学量表失败: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("获取医学量表失败: %v", err))
	}

	if medicalScale == nil {
		return nil, status.Error(codes.NotFound, "医学量表不存在")
	}

	// 转换为 gRPC 响应
	response := &pb.GetMedicalScaleByCodeResponse{
		MedicalScale: convertMedicalScaleToProto(medicalScale),
	}

	return response, nil
}

// GetMedicalScaleByQuestionnaireCode 根据问卷代码获取医学量表详情
func (s *MedicalScaleService) GetMedicalScaleByQuestionnaireCode(ctx context.Context, req *pb.GetMedicalScaleByQuestionnaireCodeRequest) (*pb.GetMedicalScaleByQuestionnaireCodeResponse, error) {
	if req.QuestionnaireCode == "" {
		return nil, status.Error(codes.InvalidArgument, "问卷代码不能为空")
	}

	log.Infof("根据问卷代码获取医学量表详情，问卷代码: %s", req.QuestionnaireCode)

	// 查询医学量表
	medicalScale, err := s.medicalScaleQueryer.GetMedicalScaleByQuestionnaireCode(ctx, req.QuestionnaireCode)
	if err != nil {
		log.Errorf("获取医学量表失败: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("获取医学量表失败: %v", err))
	}

	if medicalScale == nil {
		return nil, status.Error(codes.NotFound, "医学量表不存在")
	}

	// 转换为 gRPC 响应
	response := &pb.GetMedicalScaleByQuestionnaireCodeResponse{
		MedicalScale: convertMedicalScaleToProto(medicalScale),
	}

	return response, nil
}

// convertMedicalScaleToProto 将 DTO 转换为 Proto 消息
func convertMedicalScaleToProto(medicalScale *dto.MedicalScaleDTO) *pb.MedicalScale {
	if medicalScale == nil {
		return nil
	}

	// 转换因子列表
	factors := make([]*pb.Factor, 0, len(medicalScale.Factors))
	for _, factor := range medicalScale.Factors {
		factors = append(factors, convertFactorToProto(factor))
	}

	return &pb.MedicalScale{
		Id:                medicalScale.ID,
		Code:              medicalScale.Code,
		QuestionnaireCode: medicalScale.QuestionnaireCode,
		Title:             medicalScale.Title,
		Description:       medicalScale.Description,
		Factors:           factors,
		CreatedAt:         "", // DTO 中没有时间字段，暂时为空
		UpdatedAt:         "", // DTO 中没有时间字段，暂时为空
	}
}

// convertFactorToProto 将因子 DTO 转换为 Proto 消息
func convertFactorToProto(factor dto.FactorDTO) *pb.Factor {
	// 转换计算规则
	var calculationRule *pb.CalculationRule
	if factor.CalculationRule != nil {
		calculationRule = &pb.CalculationRule{
			FormulaType: factor.CalculationRule.FormulaType,
			SourceCodes: factor.CalculationRule.SourceCodes,
		}
	}

	// 转换解读规则列表
	interpretationRules := make([]*pb.InterpretationRule, 0, len(factor.InterpretRules))
	for _, rule := range factor.InterpretRules {
		interpretationRules = append(interpretationRules, &pb.InterpretationRule{
			ScoreRange: &pb.ScoreRange{
				MinScore: rule.ScoreRange.MinScore,
				MaxScore: rule.ScoreRange.MaxScore,
			},
			Content: rule.Content,
		})
	}

	return &pb.Factor{
		Code:                factor.Code,
		Title:               factor.Title,
		FactorType:          factor.FactorType,
		IsTotalScore:        factor.IsTotalScore,
		CalculationRule:     calculationRule,
		InterpretationRules: interpretationRules,
	}
}
