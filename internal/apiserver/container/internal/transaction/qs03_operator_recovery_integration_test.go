//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	assessmentcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/answersheetgap"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/recoveryjournal"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	servergrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	workereventing "github.com/FangcunMount/qs-server/internal/worker/integration/eventing"
	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

type qs03NormalSubscriber struct {
	subscriber *sdknsq.Subscriber
	ctx        context.Context
	original   legacy.Envelope
	deliveries atomic.Int32
	failed     atomic.Int32
}

func (s *qs03NormalSubscriber) Subscribe(topic, channel string, next transport.Handler) error {
	return s.subscriber.Subscribe(s.ctx, topic, channel, func(ctx context.Context, d transport.Delivery) error {
		got := d.Message()
		if got.ID != s.original.UUID || !bytes.Equal(got.Payload, s.original.Payload) {
			return errors.New("late original wire changed")
		}
		s.deliveries.Add(1)
		return next(ctx, d)
	}, func(context.Context, legacy.FailedHandoff) error {
		s.failed.Add(1)
		return errors.New("unexpected failure handoff")
	})
}

// The runner proves PUB OK -> memory-only channel -> SIGKILL -> same-volume
// loss first. This test then executes the real operator binary, over production
// mTLS/ACL and the normal Worker handler -> Journey -> original SQL transaction.
// No recovery PUB may reach another topic consumer. Late originals subsequently
// traverse the normal SDK subscriber and Worker dispatch/FIN path.
func TestQS03OperatorRecoveryAfterBrokerLoss(t *testing.T) {
	mongoDB, mysqlDB, nsqTCP := qs03ProofConnections(t)
	binary := os.Getenv("RM_QS03_RECOVERY_BINARY")
	require.NotEmpty(t, binary, "real recovery binary required; no skip or direct substitute")
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	var stored struct {
		MessageID string `bson:"message_id"`
		Payload   []byte `bson:"payload"`
		Version   uint64 `bson:"version"`
		Attempts  uint64 `bson:"attempt_count"`
	}
	require.NoError(t, mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"state": "published"}).Decode(&stored))
	original, recognized, err := legacy.Decode(stored.Payload)
	require.NoError(t, err)
	require.True(t, recognized)
	sheetRepo, err := mongoanswersheet.NewRepository(mongoDB)
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(qs03Catalog(t, "evaluation.requested"), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	intake := appintake.NewService(assessmentcache.NewInvalidatingAssessmentRepository(assessmentmysql.NewAssessmentRepository(mysqlDB), nil), proofModelValidator{}, NewMySQLRunner(mysqlDB), stager)
	ensure := journey.NewService(nil, nil, nil, nil, intake, nil, sheetRepo)
	ca := tlsfixture.New(t)
	pair := ca.Issue(t, "server.test", false)
	worker := ca.Issue(t, "qs-worker.svc", false)
	server, err := servergrpc.NewServer(&servergrpc.Config{TLSCertFile: pair.CertFile, TLSKeyFile: pair.KeyFile, MTLS: servergrpc.MTLSConfig{Enabled: true, CAFile: ca.CAFile, RequireClientCert: true}, ACL: servergrpc.ACLConfig{Enabled: true, ConfigFile: "../../../../../configs/grpc-acl.prod.yaml", DefaultPolicy: "deny"}}, nil)
	require.NoError(t, err)
	defer server.Stop()
	grpcservice.NewAssessmentIntakeService(ensure, intake, nil).RegisterService(server.Server)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(lis) }()
	journal := t.TempDir()
	require.NoError(t, os.Chmod(journal, 0700))
	cutoff := time.Now().Format(time.RFC3339Nano)
	environment := append(os.Environ(), "MONGO_URI="+os.Getenv("RM_QS03_MONGO_URI"), "MONGO_DB="+qs03ProofDB, "MYSQL_DSN="+os.Getenv("RM_QS03_MYSQL_DSN"), "M6_QS03_GRPC_ENDPOINT="+lis.Addr().String(), "M6_QS03_CA_FILE="+ca.CAFile, "M6_QS03_CERT_FILE="+worker.CertFile, "M6_QS03_KEY_FILE="+worker.KeyFile, "M6_QS03_SERVER_NAME=server.test")
	run := func(want int, args ...string) []byte {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = environment
		var output, stderr bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, stderr.String())
			code = exit.ExitCode()
		}
		require.Equal(t, want, code, stderr.String())
		return output.Bytes()
	}
	scope := []string{"--answersheet-id=" + strconv.FormatUint(qs03ProofSheetID, 10), "--org-id=501", "--accepted-before=" + cutoff}
	// Real TCP accepts but never supplies the MySQL handshake. The command
	// must reject unknown source within its deadline, before reservation/RPC.
	blockedListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = blockedListener.Close() }()
	acceptedConnection := make(chan struct{})
	go func() {
		connection, err := blockedListener.Accept()
		if err != nil {
			return
		}
		close(acceptedConnection)
		defer func() { _ = connection.Close() }()
		_, _ = io.Copy(io.Discard, connection)
	}()
	blockedDSN, err := mysqldriver.ParseDSN(os.Getenv("RM_QS03_MYSQL_DSN"))
	require.NoError(t, err)
	blockedDSN.Addr = blockedListener.Addr().String()
	normalEnvironment := environment
	environment = append(append([]string{}, environment...), "MYSQL_DSN="+blockedDSN.FormatDSN())
	started := time.Now()
	run(1, append(append([]string{}, scope...), "--timeout=1s")...)
	elapsed := time.Since(started)
	environment = normalEnvironment
	select {
	case <-acceptedConnection:
	default:
		t.Fatal("bounded check never reached actual MySQL handshake")
	}
	require.Less(t, elapsed, 4*time.Second, "initial connection escaped original operation deadline")
	entries, err := os.ReadDir(journal)
	require.NoError(t, err)
	require.Empty(t, entries)
	t.Logf("blocked_mysql_handshake=%s source=unknown journal=empty effect_calls=0", elapsed)
	var plan answersheetgap.RecoveryPlan
	require.NoError(t, json.Unmarshal(run(0, scope...), &plan))
	require.Equal(t, stored.MessageID, plan.EventID)
	require.NotContains(t, string(run(0, scope...)), "broker lost after confirm")
	id := uuid.NewString()
	apply := append(append([]string{}, scope...), "--mode=apply", "--source-fingerprint="+plan.SourceFingerprint, "--audit-dir="+journal, "--request-id="+id, "--operator=isolated-proof", "--reason=same-broker confirmed loss reviewed", "--external-result-reviewed")
	wrong := append([]string{}, apply...)
	wrong = append(wrong, "--source-fingerprint="+string(bytes.Repeat([]byte("0"), 64)))
	run(1, wrong...)
	var receipt recoveryjournal.Receipt
	require.NoError(t, json.Unmarshal(run(0, apply...), &receipt))
	require.Equal(t, "not_sent", receipt.TransportOutcome)
	require.Equal(t, "accepted", receipt.EffectOutcome)
	require.NotZero(t, receipt.AssessmentID)
	require.False(t, receipt.BusinessCompletionProven)
	run(0, "--mode=reconcile", "--audit-dir="+journal, "--request-id="+id)
	run(1, apply...)
	run(1, append(apply, "--request-id="+uuid.NewString())...)
	run(1, scope...) // Any accepted Assessment blocks another initial repair.
	var count, intents int64
	require.NoError(t, mysqlDB.Table("assessment").Where("answer_sheet_id=?", qs03ProofSheetID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, mysqlDB.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&intents).Error)
	require.EqualValues(t, 1, intents)
	var assessment assessmentmysql.AssessmentPO
	require.NoError(t, mysqlDB.Where("id=?", receipt.AssessmentID).First(&assessment).Error)
	require.Equal(t, "submitted", assessment.Status)
	require.Equal(t, "MODEL-1", *assessment.EvaluationModelCode)
	require.Equal(t, "1.0.0", *assessment.EvaluationModelVersion)
	qs03AssertBroker(t, 0, 0) // The actual recovery binary did not broadcast a PUB.
	// A late original goes through the real SDK and normal Worker dispatcher.
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{Endpoint: lis.Addr().String(), Timeout: 10 * time.Second, TLS: grpcclient.TLSConfig{CAFile: ca.CAFile, CertFile: worker.CertFile, KeyFile: worker.KeyFile, ServerName: "server.test"}})
	require.NoError(t, err)
	defer manager.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dispatcher := workereventing.NewDispatcher(logger, &workereventing.HandlerDependencies{Logger: logger, AssessmentIntakeClient: grpcclient.NewAssessmentIntakeClient(manager)}, handlers.NewRegistry())
	catalogConfig, err := eventcatalog.Parse([]byte("version: '1'\ntopics:\n  assessment:\n    name: qs.evaluation.lifecycle\nevents:\n  answersheet.submitted:\n    topic: assessment\n    delivery: durable_outbox\n    aggregate: AnswerSheet\n    domain: survey/answersheet\n    handler: answersheet_submitted_handler\n"))
	require.NoError(t, err)
	require.NoError(t, dispatcher.Initialize(eventcatalog.NewCatalog(catalogConfig)))
	driver := nsq.NewConfig()
	driver.HeartbeatInterval = time.Second
	driver.ReadTimeout = 3 * time.Second
	driver.WriteTimeout = time.Second
	sub, err := sdknsq.NewSubscriber(sdknsq.SubscriberConfig{NSQDAddresses: []string{nsqTCP}, Driver: driver, MaxInFlight: 1, MaxAttempts: 3, DeliveryContext: ctx})
	require.NoError(t, err)
	defer func() {
		shutdown, stop := context.WithTimeout(context.Background(), 6*time.Second)
		defer stop()
		require.NoError(t, sub.Close(shutdown))
	}()
	adapter := &qs03NormalSubscriber{subscriber: sub, ctx: ctx, original: original}
	require.NoError(t, workermessaging.SubscribeSDKHandlersWithOptions(workermessaging.SubscribeSDKHandlersOptions{ServiceName: "rm-qs03-postconfirm", Logger: logger, Runtime: dispatcher, Subscriber: adapter, UnknownRecorder: func(context.Context, transport.Received, string) error { return errors.New("unexpected unknown event") }}))
	producer, err := nsq.NewProducer(nsqTCP, driver)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	for range 2 {
		require.NoError(t, producer.Publish(qs03ProofTopic, stored.Payload))
	}
	require.Eventually(t, func() bool { return adapter.deliveries.Load() == 2 }, 10*time.Second, 25*time.Millisecond)
	qs03AssertBroker(t, 2, 2)
	require.Zero(t, adapter.failed.Load())
	require.NoError(t, mysqlDB.Table("assessment").Where("answer_sheet_id=?", qs03ProofSheetID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, mysqlDB.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&intents).Error)
	require.EqualValues(t, 1, intents)
	// Soft deletion remains proof of an accepted effect; no resurrection/re-PUB.
	require.NoError(t, mysqlDB.Table("assessment").Where("id=?", receipt.AssessmentID).Update("deleted_at", time.Now()).Error)
	run(1, scope...)
	var after struct {
		Payload  []byte `bson:"payload"`
		Version  uint64 `bson:"version"`
		Attempts uint64 `bson:"attempt_count"`
	}
	require.NoError(t, mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": stored.MessageID, "state": "published"}).Decode(&after))
	require.Equal(t, stored.Payload, after.Payload)
	require.Equal(t, stored.Version, after.Version)
	require.Equal(t, stored.Attempts, after.Attempts)
	t.Logf("original_event=%s assessment=%d intent=1 recovery_pub=0 late_originals=2 frozen_model=MODEL-1@1.0.0", stored.MessageID, receipt.AssessmentID)
}

func qs03AssertBroker(t *testing.T, published, finished int64) {
	t.Helper()
	endpoint := os.Getenv("RM_QS03_NSQ_HTTP")
	require.True(t, strings.HasPrefix(endpoint, "http://127.0.0.1:"))
	client := &http.Client{Timeout: time.Second}
	require.Eventually(t, func() bool {
		response, err := client.Get(endpoint + "/stats?format=json&topic=" + qs03ProofTopic)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		var data struct {
			Topics []struct {
				MessageCount int64 `json:"message_count"`
				Channels     []struct {
					Name     string `json:"channel_name"`
					Depth    int64  `json:"depth"`
					InFlight int64  `json:"in_flight_count"`
					Deferred int64  `json:"deferred_count"`
					Requeues int64  `json:"requeue_count"`
					Clients  []struct {
						Finish int64 `json:"finish_count"`
					} `json:"clients"`
				} `json:"channels"`
			} `json:"topics"`
		}
		if json.NewDecoder(response.Body).Decode(&data) != nil || len(data.Topics) != 1 || data.Topics[0].MessageCount != published {
			return false
		}
		for _, channel := range data.Topics[0].Channels {
			if channel.Name != "rm-qs03-postconfirm" {
				continue
			}
			var total int64
			for _, c := range channel.Clients {
				total += c.Finish
			}
			return channel.Depth == 0 && channel.InFlight == 0 && channel.Deferred == 0 && channel.Requeues == 0 && total == finished
		}
		return false
	}, 5*time.Second, 25*time.Millisecond, "real original channel did not FIN/settle or recovery broadcast a PUB")
}
