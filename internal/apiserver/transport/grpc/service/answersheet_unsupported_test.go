package service

import (
	"context"
	"testing"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAnswerSheetServiceSaveAnswerSheetScoresReturnsUnimplemented(t *testing.T) {
	svc := NewAnswerSheetService(nil, nil)

	_, err := svc.SaveAnswerSheetScores(context.Background(), &pb.SaveAnswerSheetScoresRequest{})
	if err == nil {
		t.Fatalf("expected unimplemented error")
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("expected codes.Unimplemented, got %v", status.Code(err))
	}
}
