//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The parent invokes this helper in another OS process, then kills it after
// the original standard Mongo intent is claimed but before any NSQ publish.
type m5MongoClaimCrashBarrier struct {
	outbox.Store
	eventID string
}

func (s m5MongoClaimCrashBarrier) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]outbox.Claim, error) {
	claims, err := s.Store.ClaimDue(ctx, limit, lease)
	if err != nil || len(claims) == 0 {
		return claims, err
	}
	if len(claims) != 1 || claims[0].Message.Input().ID != s.eventID {
		return nil, fmt.Errorf("crash barrier claimed an unexpected event")
	}
	fmt.Println("M5_MONGO_CLAIM_READY")
	time.Sleep(time.Hour)
	return claims, nil
}

func TestM5MongoClaimCrashChild(t *testing.T) {
	if os.Getenv("RM_QS_M5_MONGO_CLAIM_CHILD") != "1" {
		t.Skip("invoked by the disposable process recovery test")
	}
	uri := os.Getenv("RM_QS_ATTENTION_MONGO_URI")
	dbName := os.Getenv("RM_QS_M5_MONGO_CLAIM_DB")
	eventID := os.Getenv("RM_QS_M5_MONGO_CLAIM_EVENT")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") ||
		!strings.HasPrefix(dbName, "m5_qs_relay_crash_") || eventID == "" {
		t.Fatal("invocation-owned Mongo replica set, database, and event ID required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer client.Disconnect(context.Background())
	store, err := sdkmongo.New(client.Database(dbName).Collection("rm_outbox"))
	require.NoError(t, err)
	configNSQ := nsq.NewConfig()
	producer, err := nsq.NewProducer("nsqd:4150", configNSQ)
	require.NoError(t, err)
	defer producer.Stop()
	const topic = "qs.evaluation.lifecycle"
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	require.NoError(t, err)
	forwarder, err := relay.New(m5MongoClaimCrashBarrier{Store: store, eventID: eventID}, publisher, relay.Config{
		Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 4 * time.Second,
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	require.NoError(t, err)
	if err := forwarder.Run(ctx); err != nil {
		t.Fatalf("crash-barrier Relay returned before process kill: %v", err)
	}
	t.Fatal("crash-barrier Relay exited before process kill")
}

func TestM5StandardMongoAnswerSheetRecoversAfterRelayProcessKill(t *testing.T) {
	uri, nsqAddress := os.Getenv("RM_QS_ATTENTION_MONGO_URI"), os.Getenv("RM_QS_NSQ_TCP")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") || nsqAddress != "nsqd:4150" {
		t.Fatal("disposable Mongo replica set and nsqd:4150 required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer client.Disconnect(context.Background())
	dbName := "m5_qs_relay_crash_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	db := client.Database(dbName)
	defer db.Drop(context.Background())
	require.NoError(t, db.CreateCollection(ctx, "rm_outbox"))
	outbox := db.Collection("rm_outbox")
	_, err = outbox.Indexes().CreateMany(ctx, sdkmongo.Indexes())
	require.NoError(t, err)
	repo, err := mongoanswersheet.NewRepository(db)
	require.NoError(t, err)
	config, err := eventcatalog.Load("/configs/events.yaml")
	require.NoError(t, err)
	stager, err := mongostandard.NewStager(outbox, eventcatalog.NewCatalog(config), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	runner := NewMongoRunner(db, MongoRunnerOptions{Boundary: "m5_answersheet_process_recovery", Limiter: &transactionLimiterSpy{}})
	durable := appanswersheet.NewTransactionalSubmissionDurableStore(runner, repo, stager, nil)
	sheet := standardSubmissionSheet(t, 90030001, "process recovery")
	eventID := sheet.Events()[0].EventID()
	fingerprint, err := submitport.Fingerprint(sheet)
	require.NoError(t, err)
	_, existed, err := durable.CreateDurably(ctx, sheet, appanswersheet.DurableSubmitMeta{
		WriterID: 301, IdempotencyKey: "m5-process-recovery", Fingerprint: fingerprint,
	})
	require.NoError(t, err)
	require.False(t, existed)
	require.EqualValues(t, 1, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90030001)}))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{"message_id": eventID, "state": "pending"}))

	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestM5MongoClaimCrashChild$", "-test.v")
	child.Env = append(os.Environ(), "RM_QS_M5_MONGO_CLAIM_CHILD=1", "RM_QS_M5_MONGO_CLAIM_DB="+dbName, "RM_QS_M5_MONGO_CLAIM_EVENT="+eventID)
	stdout, err := child.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	child.Stderr = &stderr
	require.NoError(t, child.Start())
	ready := make(chan struct{})
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Text() == "M5_MONGO_CLAIM_READY" {
				close(ready)
				return
			}
		}
	}()
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()
	reaped := false
	defer func() {
		if !reaped {
			_ = child.Process.Kill()
			<-exited
		}
		stdout.Close()
		<-scanDone
	}()
	select {
	case <-ready:
	case err := <-exited:
		reaped = true
		t.Fatalf("claiming process exited early: %v %s", err, stderr.String())
	case <-ctx.Done():
		t.Fatal("claiming process did not reach crash barrier")
	}
	var claimed struct {
		State string `bson:"state"`
	}
	require.NoError(t, outbox.FindOne(ctx, bson.M{"message_id": eventID}).Decode(&claimed))
	require.Equal(t, "publishing", claimed.State)
	require.NoError(t, child.Process.Kill())
	err = <-exited
	reaped = true
	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr))
	status, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.Equal(t, syscall.SIGKILL, status.Signal())

	const topic = "qs.evaluation.lifecycle"
	channel := "m5-process-recovery-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	configNSQ := nsq.NewConfig()
	consumer, err := nsq.NewConsumer(topic, channel, configNSQ)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	delivered := make(chan string, 2)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		msg, recognized, decodeErr := messaging.DecodeMessagePayload(raw.Body)
		if decodeErr != nil {
			return decodeErr
		}
		if !recognized || msg.UUID != eventID || msg.Metadata["event_type"] != eventcatalog.AnswerSheetSubmitted {
			return fmt.Errorf("recovered NSQ message changed identity")
		}
		select {
		case delivered <- msg.UUID:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}))
	require.NoError(t, consumer.ConnectToNSQD(nsqAddress))
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("NSQ consumer drain timeout")
		}
	}()
	producer, err := nsq.NewProducer(nsqAddress, configNSQ)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	require.NoError(t, err)
	defer publisher.Drain(ctx)
	store, err := sdkmongo.New(outbox)
	require.NoError(t, err)
	forwarder, err := relay.New(store, publisher, relay.Config{
		Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 4 * time.Second,
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	require.NoError(t, err)
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- forwarder.Run(runCtx) }()
	select {
	case got := <-delivered:
		require.Equal(t, eventID, got)
	case <-ctx.Done():
		t.Fatal("replacement process did not publish original event")
	}
	require.Eventually(t, func() bool {
		var row struct {
			State string `bson:"state"`
		}
		return outbox.FindOne(ctx, bson.M{"message_id": eventID}).Decode(&row) == nil && row.State == "published"
	}, 10*time.Second, 50*time.Millisecond)
	stop()
	require.NoError(t, <-done)
	var final struct {
		State        string `bson:"state"`
		AttemptCount int    `bson:"attempt_count"`
	}
	require.NoError(t, outbox.FindOne(ctx, bson.M{"message_id": eventID}).Decode(&final))
	require.Equal(t, "published", final.State)
	require.Equal(t, 2, final.AttemptCount)
	require.EqualValues(t, 1, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90030001)}))
	t.Logf("SIGKILL after Mongo claim: original event=%s claim attempts=%d NSQ deliveries=1 answer sheets=1", eventID, final.AttemptCount)
}
