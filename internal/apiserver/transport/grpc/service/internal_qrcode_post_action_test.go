package service

import (
	"context"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"reflect"
	"testing"
)

type markedQRRecorder struct{ marked []bool }

func (q *markedQRRecorder) GenerateQuestionnaireQRCode(ctx context.Context, code, version string) (string, error) {
	q.marked = append(q.marked, app.IsPublishedPostAction(ctx))
	return "qr", nil
}
func (q *markedQRRecorder) GenerateScaleQRCode(ctx context.Context, code string) (string, error) {
	q.marked = append(q.marked, app.IsPublishedPostAction(ctx))
	return "qr", nil
}
func TestOnlyPublishedPostActionsCarryAcceptanceMarker(t *testing.T) {
	qr := &markedQRRecorder{}
	s := &InternalService{qrCodeService: qr}
	ctx := context.Background()
	q := &pb.GenerateQuestionnaireQRCodeRequest{Code: "test-qr", Version: "v1"}
	scale := &pb.GenerateScaleQRCodeRequest{Code: "test-scale"}
	if _, err := s.HandleQuestionnairePublishedPostActions(ctx, q); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HandleScalePublishedPostActions(ctx, scale); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GenerateQuestionnaireQRCode(ctx, q); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GenerateScaleQRCode(ctx, scale); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(qr.marked, []bool{true, true, false, false}) {
		t.Fatalf("marked calls=%v", qr.marked)
	}
}
