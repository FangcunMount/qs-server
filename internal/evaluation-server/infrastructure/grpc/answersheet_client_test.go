package grpc

import (
	"context"
	"testing"

	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
)

func TestAnswerSheetClient_SaveAnswerSheetScores(t *testing.T) {
	// 创建客户端（这里只是测试方法签名，实际调用需要真实的GRPC服务）
	client := &AnswerSheetClient{}

	// 创建测试数据
	answerSheetID := uint64(12345)
	totalScore := float64(100)
	answers := []*answersheet.Answer{
		{
			QuestionCode: "Q1",
			QuestionType: "Radio",
			Score:        5,
			Value:        "\"option1\"",
		},
		{
			QuestionCode: "Q2",
			QuestionType: "Radio",
			Score:        3,
			Value:        "\"option2\"",
		},
	}

	// 测试方法调用（这里会失败，因为没有真实的GRPC服务）
	ctx := context.Background()

	// 由于client.client是nil，我们期望会panic
	defer func() {
		if r := recover(); r != nil {
			t.Logf("预期的panic（因为client.client是nil）: %v", r)
		} else {
			t.Error("期望panic，但没有发生")
		}
	}()

	client.SaveAnswerSheetScores(ctx, answerSheetID, totalScore, answers)
}
