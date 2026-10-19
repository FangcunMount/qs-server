//go:build integration && m3_answersheet_integration

package transaction_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
	survey "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	questionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	answerrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	collection "github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/handler"
	"github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type m3UnreadQuestionnaire struct{ questionnaire.Repository }
type m3UnreadQuestionnaireView struct {
	surveyreadmodel.QuestionnaireReader
}

// Only the Collection port-to-protobuf mapping is supplied by the fixture.
// The actual gRPC service, both applications and Mongo authority remain real.
// It supplies no IAM session or mutable questionnaire/profile fallback.
type m3AcceptedReadback struct {
	client pb.AnswerSheetServiceClient
}

func (b m3AcceptedReadback) LookupAcceptedSubmission(ctx context.Context, in *collection.LookupAcceptedSubmissionInput) (*collection.LookupAcceptedSubmissionOutput, error) {
	req := &pb.LookupAnswerSheetSubmissionRequest{AnsweringStartId: in.AnsweringStartID, QuestionnaireCode: in.QuestionnaireCode,
		QuestionnaireVersion: in.QuestionnaireVersion, IdempotencyKey: in.IdempotencyKey, WriterId: in.WriterID, TesteeId: in.TesteeID, TaskId: in.TaskID}
	for _, answer := range in.Answers {
		req.Answers = append(req.Answers, &pb.SubmissionIntentAnswer{QuestionCode: answer.QuestionCode, QuestionType: answer.QuestionType, Value: answer.Value})
	}
	if in.OriginRef != nil {
		req.OriginRef = &pb.OriginRef{Type: in.OriginRef.Type, Id: in.OriginRef.ID}
	}
	result, err := b.client.LookupAnswerSheetSubmission(ctx, req)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &collection.LookupAcceptedSubmissionOutput{}, nil
	}
	return &collection.LookupAcceptedSubmissionOutput{Found: result.Found, ID: result.Id}, nil
}

func newM3CollectionReadback(t *testing.T, db *mongo.Database, repo *answerrepo.Repository, store *answerrepo.RocketMQSubmissionStore, limiter backpressure.Acquirer) func(string, string) int {
	t.Helper()
	module, err := survey.Wire(survey.WireInput{MongoDB: db, MongoLimiter: limiter, SurveyRuntimeInfra: &survey.SurveyRuntimeInfra{
		QuestionnaireRepo: m3UnreadQuestionnaire{}, QuestionnaireReader: m3UnreadQuestionnaireView{},
		AnswerSheetRepo: repo, AnswerSheetReader: answerrepo.NewAnswerSheetReadModel(repo), SubmissionStore: store,
	}})
	require.NoError(t, err)
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	grpcservice.NewAnswerSheetService(module.AnswerSheet.SubmissionService).RegisterService(server)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///qs-m3-owned-fixture", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.DialContext(ctx)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
		server.Stop()
		_ = listener.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
				t.Error("owned gRPC server did not drain")
			}
		case <-time.After(5 * time.Second):
			t.Error("owned gRPC server still running")
		}
	})
	bridge := m3AcceptedReadback{client: pb.NewAnswerSheetServiceClient(conn)}
	service := collection.NewSubmissionService(nil, bridge, nil, nil, nil, nil, nil, nil, 2*time.Second)
	h := handler.NewAnswerSheetHandler(service)
	return func(key, answer string) int {
		payload, err := json.Marshal(map[string]any{"questionnaire_code": "M3-NATIVE", "questionnaire_version": "1.0.0",
			"idempotency_key": key, "testee_id": 401, "answers": []map[string]string{{"question_code": "Q1", "question_type": "Text", "value": answer}}})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/answersheets", strings.NewReader(string(payload)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(middleware.UserIDKey, uint64(301))
		h.Submit(c)
		if w.Code == http.StatusAccepted {
			var response struct {
				Data collection.SubmitAcceptedResponse `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Equal(t, "accepted", response.Data.Status)
			require.Equal(t, "101", response.Data.AnswerSheetID)
		}
		return w.Code
	}
}
