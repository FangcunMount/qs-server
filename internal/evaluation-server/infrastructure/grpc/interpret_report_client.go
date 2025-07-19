package grpc

import (
	"context"
	"fmt"

	interpret_report "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/interpret-report"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// InterpretReportClient 解读报告客户端
type InterpretReportClient struct {
	client interpret_report.InterpretReportServiceClient
}

// NewInterpretReportClient 创建解读报告客户端
func NewInterpretReportClient(factory *ClientFactory) *InterpretReportClient {
	return &InterpretReportClient{
		client: factory.NewInterpretReportClient(),
	}
}

// SaveInterpretReport 保存解读报告
func (c *InterpretReportClient) SaveInterpretReport(ctx context.Context, answerSheetId uint64, medicalScaleCode, title, description string, interpretItems []*interpret_report.InterpretItem) (uint64, error) {
	log.Infof("保存解读报告，答卷ID: %d", answerSheetId)

	// 调用 gRPC 服务
	resp, err := c.client.SaveInterpretReport(ctx, &interpret_report.SaveInterpretReportRequest{
		AnswerSheetId:    answerSheetId,
		MedicalScaleCode: medicalScaleCode,
		Title:            title,
		Description:      description,
		InterpretItems:   interpretItems,
	})
	if err != nil {
		return 0, fmt.Errorf("保存解读报告失败: %v", err)
	}

	return resp.Id, nil
}

// GetInterpretReportByAnswerSheetID 根据答卷ID获取解读报告
func (c *InterpretReportClient) GetInterpretReportByAnswerSheetID(ctx context.Context, answerSheetId uint64) (*interpret_report.InterpretReport, error) {
	log.Infof("获取解读报告，答卷ID: %d", answerSheetId)

	// 调用 gRPC 服务
	resp, err := c.client.GetInterpretReportByAnswerSheetID(ctx, &interpret_report.GetInterpretReportByAnswerSheetIDRequest{
		AnswerSheetId: answerSheetId,
	})
	if err != nil {
		return nil, fmt.Errorf("获取解读报告失败: %v", err)
	}

	return resp.InterpretReport, nil
}
