//go:build reliable_messaging

package transaction

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	intake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	persistence "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	oldoutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Real broker redelivery -> original Worker handler -> loopback gRPC -> original
// transport service/Journey/intake -> real MySQL Assessment and Outbox.
// AnswerSheet loading and model validation are explicit fixtures. No model runs.
func TestReliableMessagingAnswerSheetFINLoss(t *testing.T) {
	dsn, address := os.Getenv("RM_QS_REDELIVERY_DSN"), os.Getenv("RM_QS_NSQ_TCP")
	httpAddress := os.Getenv("RM_QS_NSQ_HTTP")
	if dsn == "" || address == "" || httpAddress == "" {
		t.Fatal("isolated MySQL and NSQ required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&persistence.AssessmentPO{}, &oldoutbox.OutboxPO{}))
	config, err := catalog.Parse([]byte(`version: "1"
topics:
  proof:
    name: qs.rm.evaluation
events:
  evaluation.requested:
    topic: proof
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: proof
`))
	require.NoError(t, err)
	service := intake.NewService(persistence.NewAssessmentRepository(db), proofModelValidator{}, NewMySQLRunner(db), oldoutbox.NewStoreWithTopicResolver(db, catalog.NewCatalog(config)))
	submission, err := sheet.NewSubmissionContext(actor.NewFillerRef(4, actor.FillerTypeSelf), actor.NewTesteeRef(meta.FromUint64(2)), meta.FromUint64(1), "")
	require.NoError(t, err)
	questionnaire, err := sheet.NewQuestionnaireRef("Q-001", "v1", "proof")
	require.NoError(t, err)
	persisted := sheet.ReconstructWithSubmissionContext(meta.FromUint64(3), questionnaire, submission, nil, time.Now(), 0)
	ensure := journey.NewService(nil, nil, nil, nil, service, nil, proofSubmissionReader{value: persisted})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var rpcCalls atomic.Int32
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if strings.HasSuffix(info.FullMethod, "/EnsureAssessment") {
			rpcCalls.Add(1)
		}
		return next(ctx, req)
	}))
	grpcservice.NewAssessmentIntakeService(ensure, service, nil).RegisterService(server)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	defer func() { server.Stop(); require.NoError(t, <-serverDone) }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	handler, ok := handlers.NewRegistry().Create("answersheet_submitted_handler", &handlers.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AssessmentIntakeClient: proofIntakeGRPCClient{pb.NewAssessmentIntakeServiceClient(conn)}})
	require.True(t, ok)
	nsqConfig := nsq.NewConfig()
	nsqConfig.HeartbeatInterval = time.Second
	nsqConfig.ReadTimeout = 3 * time.Second
	nsqConfig.WriteTimeout = time.Second
	nsqConfig.MsgTimeout = time.Second
	httpClient := &http.Client{Timeout: 3 * time.Second}
	for _, path := range []string{"/topic/create?topic=qs-rm-answersheet", "/channel/create?topic=qs-rm-answersheet&channel=rm-proof"} {
		request, e := http.NewRequestWithContext(ctx, "POST", httpAddress+path, nil)
		require.NoError(t, e)
		response, e := httpClient.Do(request)
		require.NoError(t, e)
		response.Body.Close()
		require.Equal(t, 200, response.StatusCode)
	}
	consumer, err := nsq.NewConsumer("qs-rm-answersheet", "rm-proof", nsqConfig)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	type delivery struct {
		id                  nsq.MessageID
		attempt             uint16
		assessments, events int64
		err                 error
	}
	delivered := make(chan delivery, 8)
	consumer.AddHandler(nsq.HandlerFunc(func(msg *nsq.Message) error {
		e := handler(ctx, "answersheet.submitted", msg.Body)
		var assessments, events int64
		if e == nil {
			e = db.Model(&persistence.AssessmentPO{}).Count(&assessments).Error
		}
		if e == nil {
			e = db.Model(&oldoutbox.OutboxPO{}).Where("event_type=?", "evaluation.requested").Count(&events).Error
		}
		select {
		case delivered <- delivery{msg.ID, msg.Attempts, assessments, events, e}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return e
	}))
	proxy, stopProxy := dropFirstFINProxy(t, ctx, address)
	defer stopProxy()
	require.NoError(t, consumer.ConnectToNSQD(proxy))
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("consumer did not drain")
		}
	}()
	producer, err := nsq.NewProducer(address, nsqConfig)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{"submitted": "qs-rm-answersheet"}, 1)
	require.NoError(t, err)
	defer func() { require.NoError(t, publisher.Drain(ctx)) }()
	body := []byte(`{"id":"stable-answer-event","eventType":"answersheet.submitted","occurredAt":"2026-09-22T00:00:00Z","aggregateType":"AnswerSheet","aggregateID":"3","data":{"answersheet_id":"3","questionnaire_code":"Q-001","questionnaire_version":"v1","org_id":1,"testee_id":2,"filler_id":4,"admission":{"purpose":"assessment","model_kind":"scale","model_code":"MODEL-1","model_version":"1.0.0"}}}`)
	intent, err := message.New(message.Input{Producer: "qs-server", ID: "stable-answer-event", Destination: "submitted", EventType: "answersheet.submitted", SchemaVersion: "v1", Scope: "1", ContentType: "application/json", OccurredAt: "2026-09-22T00:00:00Z", Payload: body})
	require.NoError(t, err)
	require.Equal(t, transport.Confirmed, publisher.Publish(ctx, intent).Outcome)
	var first nsq.MessageID
	for i := 0; i < 2; i++ {
		select {
		case got := <-delivered:
			require.NoError(t, got.err)
			require.EqualValues(t, 1, got.assessments)
			require.EqualValues(t, 1, got.events)
			if i == 0 {
				first = got.id
				require.EqualValues(t, 1, got.attempt)
			} else {
				require.Equal(t, first, got.id)
				require.Greater(t, got.attempt, uint16(1))
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	require.EqualValues(t, 2, rpcCalls.Load(), "both broker deliveries must reach the original gRPC service")
	var stored persistence.AssessmentPO
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, "submitted", stored.Status)
	require.NotNil(t, stored.EvaluationModelCode)
	require.Equal(t, "MODEL-1", *stored.EvaluationModelCode)
	require.NotNil(t, stored.EvaluationModelVersion)
	require.Equal(t, "1.0.0", *stored.EvaluationModelVersion)
	t.Log("same broker ID redelivered after dropped FIN; original handler/gRPC/Journey leave one Assessment and one evaluation event")
}

type proofIntakeGRPCClient struct {
	client pb.AssessmentIntakeServiceClient
}

func (c proofIntakeGRPCClient) EnsureAssessment(ctx context.Context, req *pb.EnsureAssessmentRequest) (*pb.EnsureAssessmentResponse, error) {
	return c.client.EnsureAssessment(ctx, req)
}

func dropFirstFINProxy(t *testing.T, ctx context.Context, target string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxyCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		downstream, e := listener.Accept()
		if e != nil {
			done <- e
			return
		}
		defer downstream.Close()
		upstream, e := net.DialTimeout("tcp", target, time.Second)
		if e != nil {
			done <- e
			return
		}
		defer upstream.Close()
		interrupt := context.AfterFunc(proxyCtx, func() { downstream.Close(); upstream.Close() })
		defer interrupt()
		copied := make(chan struct{})
		go func() { defer close(copied); _, _ = io.Copy(downstream, upstream); downstream.Close() }()
		defer func() { downstream.Close(); upstream.Close(); <-copied }()
		var magic [4]byte
		if _, e = io.ReadFull(downstream, magic[:]); e != nil {
			done <- e
			return
		}
		if _, e = upstream.Write(magic[:]); e != nil {
			done <- e
			return
		}
		reader := bufio.NewReader(downstream)
		dropped := false
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				if e == io.EOF || proxyCtx.Err() != nil {
					e = nil
				}
				done <- e
				return
			}
			if strings.HasPrefix(line, "FIN ") && !dropped {
				dropped = true
				continue
			}
			if _, e = io.WriteString(upstream, line); e != nil {
				done <- e
				return
			}
			if line == "IDENTIFY\n" || line == "AUTH\n" {
				var size [4]byte
				if _, e = io.ReadFull(reader, size[:]); e != nil {
					done <- e
					return
				}
				n := binary.BigEndian.Uint32(size[:])
				if n > 1024*1024 {
					done <- fmt.Errorf("oversized command")
					return
				}
				body := make([]byte, n)
				if _, e = io.ReadFull(reader, body); e != nil {
					done <- e
					return
				}
				if _, e = upstream.Write(append(size[:], body...)); e != nil {
					done <- e
					return
				}
			}
		}
	}()
	return listener.Addr().String(), func() {
		cancel()
		listener.Close()
		select {
		case e := <-done:
			if e != nil && !errors.Is(e, net.ErrClosed) {
				t.Errorf("FIN proxy: %v", e)
			}
		case <-time.After(5 * time.Second):
			t.Error("FIN proxy failed to drain")
		}
	}
}
