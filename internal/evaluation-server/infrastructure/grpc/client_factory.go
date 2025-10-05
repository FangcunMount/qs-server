package grpc

import (
	"fmt"

	answersheet "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	interpretreport "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/interpret-report"
	medicalscale "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/medical-scale"
	questionnaire "github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/fangcun-mount/qs-server/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientFactory gRPC 客户端工厂
type ClientFactory struct {
	conn *grpc.ClientConn
}

// NewClientFactory 创建 gRPC 客户端工厂
func NewClientFactory(target string) (*ClientFactory, error) {
	// 创建 gRPC 连接
	conn, err := grpc.Dial(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 gRPC 连接失败: %v", err)
	}

	log.Infof("成功连接到 gRPC 服务器: %s", target)
	return &ClientFactory{conn: conn}, nil
}

// Close 关闭连接
func (f *ClientFactory) Close() error {
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}

// NewQuestionnaireClient 创建问卷客户端
func (f *ClientFactory) NewQuestionnaireClient() questionnaire.QuestionnaireServiceClient {
	return questionnaire.NewQuestionnaireServiceClient(f.conn)
}

// NewAnswerSheetClient 创建答卷客户端
func (f *ClientFactory) NewAnswerSheetClient() answersheet.AnswerSheetServiceClient {
	return answersheet.NewAnswerSheetServiceClient(f.conn)
}

// NewMedicalScaleClient 创建医学量表客户端
func (f *ClientFactory) NewMedicalScaleClient() medicalscale.MedicalScaleServiceClient {
	return medicalscale.NewMedicalScaleServiceClient(f.conn)
}

// NewInterpretReportClient 创建解读报告客户端
func (f *ClientFactory) NewInterpretReportClient() interpretreport.InterpretReportServiceClient {
	return interpretreport.NewInterpretReportServiceClient(f.conn)
}
