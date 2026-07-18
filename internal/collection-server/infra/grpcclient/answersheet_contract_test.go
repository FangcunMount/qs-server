package grpcclient_test

import (
	"context"
	"net"
	"testing"
	"time"

	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	collectionanswersheet "github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	client "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/acl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type contractSubmissionService struct{}

func (contractSubmissionService) Submit(context.Context, appanswersheet.SubmitAnswerSheetDTO) (*appanswersheet.AnswerSheetResult, error) {
	return nil, nil
}
func (contractSubmissionService) GetMyAnswerSheet(context.Context, uint64, uint64) (*appanswersheet.AnswerSheetResult, error) {
	return nil, nil
}
func (contractSubmissionService) ListMyAnswerSheets(context.Context, appanswersheet.ListMyAnswerSheetsDTO) (*appanswersheet.AnswerSheetSummaryListResult, error) {
	return nil, nil
}

type contractManagementService struct {
	result *appanswersheet.AnswerSheetResult
}

func (s contractManagementService) GetByID(context.Context, uint64) (*appanswersheet.AnswerSheetResult, error) {
	return s.result, nil
}
func (contractManagementService) List(context.Context, appanswersheet.ListAnswerSheetsDTO) (*appanswersheet.AnswerSheetSummaryListResult, error) {
	return nil, nil
}
func (contractManagementService) Delete(context.Context, uint64) error { return nil }

type contractActorLookup struct{}

func (contractActorLookup) GetTestee(context.Context, uint64) (*collectionanswersheet.ActorTestee, error) {
	return &collectionanswersheet.ActorTestee{OrgID: 18, IAMProfileID: "profile-77"}, nil
}
func (contractActorLookup) TesteeExists(context.Context, uint64, uint64) (bool, uint64, error) {
	return false, 0, nil
}

type contractProfileLink struct{}

func (contractProfileLink) IsEnabled() bool         { return true }
func (contractProfileLink) GetDefaultOrgID() uint64 { return 18 }
func (contractProfileLink) HasActiveProfileLink(context.Context, string, string) (bool, error) {
	return true, nil
}

type contractAssessmentResolver struct {
	testeeID     uint64
	assessmentID uint64
	err          error
}

func (r contractAssessmentResolver) ResolveAssessmentByAnswerSheetID(context.Context, uint64) (uint64, uint64, error) {
	return r.testeeID, r.assessmentID, r.err
}

func TestAnswerSheetOwnershipSurvivesRealGRPCContract(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	grpcservice.NewAnswerSheetService(contractSubmissionService{}, contractManagementService{result: &appanswersheet.AnswerSheetResult{
		ID: 42, QuestionnaireCode: "Q", QuestionnaireVer: "1.2.3", TesteeID: 77, FilledAt: time.Date(2026, 7, 18, 12, 0, 0, 0, time.Local),
	}}).RegisterService(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	baseClient, err := client.NewClient(&client.ClientConfig{Endpoint: "passthrough:///bufnet", Timeout: time.Second, Insecure: true},
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = baseClient.Close() })

	grpcClient := client.NewAnswerSheetClient(baseClient)
	got, err := grpcClient.GetAnswerSheet(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.TesteeID != 77 || got.QuestionnaireVersion != "1.2.3" {
		t.Fatalf("GetAnswerSheet() = %#v", got)
	}

	reader := acl.NewAnswerSheetBFFReader(grpcClient)
	pendingService := collectionanswersheet.NewSubmissionService(nil, reader, contractActorLookup{}, contractProfileLink{}, nil,
		contractAssessmentResolver{err: status.Error(codes.NotFound, "not ready")}, nil, time.Second)
	pending, err := pendingService.GetAssessmentReadiness(t.Context(), 11, 42, 77)
	if err != nil || pending.Status != "pending" || pending.AnswerSheetID != "42" {
		t.Fatalf("pending readiness = %#v, error = %v", pending, err)
	}

	readyService := collectionanswersheet.NewSubmissionService(nil, reader, contractActorLookup{}, contractProfileLink{}, nil,
		contractAssessmentResolver{testeeID: 77, assessmentID: 99}, nil, time.Second)
	ready, err := readyService.GetAssessmentReadiness(t.Context(), 11, 42, 77)
	if err != nil || ready.Status != "ready" || ready.AssessmentID != "99" {
		t.Fatalf("ready readiness = %#v, error = %v", ready, err)
	}
}
