package grpc

import (
	"context"
	"fmt"

	medical_scale "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/medical-scale"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// MedicalScaleClient 医学量表客户端
type MedicalScaleClient struct {
	client medical_scale.MedicalScaleServiceClient
}

// NewMedicalScaleClient 创建医学量表客户端
func NewMedicalScaleClient(factory *ClientFactory) *MedicalScaleClient {
	return &MedicalScaleClient{
		client: factory.NewMedicalScaleClient(),
	}
}

// GetMedicalScaleByQuestionnaireCode 根据问卷代码获取医学量表详情
func (c *MedicalScaleClient) GetMedicalScaleByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*medical_scale.MedicalScale, error) {
	log.Infof("获取医学量表详情，问卷代码: %s", questionnaireCode)

	// 调用 gRPC 服务
	resp, err := c.client.GetMedicalScaleByQuestionnaireCode(ctx, &medical_scale.GetMedicalScaleByQuestionnaireCodeRequest{
		QuestionnaireCode: questionnaireCode,
	})
	if err != nil {
		return nil, fmt.Errorf("获取医学量表详情失败: %v", err)
	}

	return resp.MedicalScale, nil
}
