//go:build integration

package transport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
)

// This test uses two independent OS processes against a disposable MySQL/NSQ
// pair. The first process dies after receiving the message but before its
// durable unknown-event record; the second must persist and confirm it.
func TestWorkerUnknownEventRecoversAfterProcessKill(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start disposable NSQ/MySQL services")
	}
	config, err := drivermysql.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil || config.Addr != "mysql:3306" ||
		integrationEnv("NSQD_ADDR", "") != "nsqd:4150" ||
		integrationEnv("NSQD_HTTP_ADDR", "") != "nsqd:4151" ||
		integrationEnv("NSQ_LOOKUPD_ADDR", "") != "nsqlookupd:4161" {
		t.Fatal("Worker process proof requires the disposable Compose MySQL and NSQ endpoints")
	}
	db := openIsolatedDeadLetterDatabase(t)
	var databaseName string
	if err := db.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	config.DBName = databaseName
	childDSN := config.FormatDSN()
	topic := fmt.Sprintf("qs-worker-process-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)
	markerDir := t.TempDir()
	readyMarker := filepath.Join(markerDir, "ready")
	blockedMarker := filepath.Join(markerDir, "before-record")
	childEnv := append(os.Environ(),
		"RM_QS_WORKER_PROCESS_DSN="+childDSN,
		"RM_QS_WORKER_PROCESS_TOPIC="+topic,
		"RM_QS_WORKER_PROCESS_CHANNEL="+channel,
		"RM_QS_WORKER_PROCESS_READY="+readyMarker,
		"RM_QS_WORKER_PROCESS_BLOCKED="+blockedMarker,
	)

	crash := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkerUnknownEventProcessChild$", "-test.v")
	crash.Env = append(childEnv, "RM_QS_WORKER_PROCESS_MODE=crash")
	var crashOutput bytes.Buffer
	crash.Stdout, crash.Stderr = &crashOutput, &crashOutput
	if err := crash.Start(); err != nil {
		t.Fatal(err)
	}
	crashDone := make(chan error, 1)
	go func() { crashDone <- crash.Wait() }()
	t.Cleanup(func() { _ = crash.Process.Kill() })
	waitForWorkerProcessMarker(t, readyMarker, crashDone)

	publisher, err := cbnsq.NewPublisher(integrationEnv("NSQD_ADDR", "127.0.0.1:4150"), nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	message := basemessaging.NewMessage("worker-process-unknown-1", []byte(`{"id":"process-unknown-1","data":{"org_id":501}}`))
	message.Metadata["event_type"] = "future.event"
	if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
		t.Fatal(err)
	}
	waitForWorkerProcessMarker(t, blockedMarker, crashDone)
	var prematureRows int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_delivery_dead_letter WHERE message_id=?", message.UUID).Scan(&prematureRows); err != nil {
		t.Fatal(err)
	}
	if prematureRows != 0 {
		t.Fatalf("unknown event was recorded before the crash barrier: %d rows", prematureRows)
	}
	if err := crash.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := <-crashDone; err == nil {
		t.Fatalf("first Worker process exited normally: %s", crashOutput.String())
	}

	recoverContext, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	recovery := exec.CommandContext(recoverContext, os.Args[0], "-test.run=^TestWorkerUnknownEventProcessChild$", "-test.v")
	recovery.Env = append(childEnv, "RM_QS_WORKER_PROCESS_MODE=recover")
	if output, err := recovery.CombinedOutput(); err != nil {
		t.Fatalf("replacement Worker process failed: %v\n%s", err, output)
	}
	var rows, attempts int
	var eventID, disposition string
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*),COALESCE(MAX(delivery_attempts),0),
		COALESCE(MAX(event_id),''),COALESCE(MAX(retry_disposition),'')
		FROM event_delivery_dead_letter WHERE message_id=?`, message.UUID).
		Scan(&rows, &attempts, &eventID, &disposition); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || attempts < 2 || attempts >= 5 || eventID != "process-unknown-1" || disposition != "manual_required" {
		t.Fatalf("recovered record: rows=%d attempts=%d event=%q disposition=%q", rows, attempts, eventID, disposition)
	}
	waitForWorkerProcessNSQDrain(t, topic, channel)
}

// Invoked only by the parent test; a separate test process owns each delivery.
func TestWorkerUnknownEventProcessChild(t *testing.T) {
	mode := os.Getenv("RM_QS_WORKER_PROCESS_MODE")
	if mode == "" {
		t.Skip("invoked only by the disposable Worker process test")
	}
	if mode != "crash" && mode != "recover" {
		t.Fatalf("invalid Worker process test mode %q", mode)
	}
	config, err := drivermysql.ParseDSN(os.Getenv("RM_QS_WORKER_PROCESS_DSN"))
	if err != nil || config.Addr != "mysql:3306" || !strings.HasPrefix(config.DBName, "qs_transport_") ||
		integrationEnv("NSQD_ADDR", "") != "nsqd:4150" || integrationEnv("NSQ_LOOKUPD_ADDR", "") != "nsqlookupd:4161" {
		t.Fatal("Worker process child requires invocation-owned MySQL and NSQ")
	}
	topic := os.Getenv("RM_QS_WORKER_PROCESS_TOPIC")
	channel := os.Getenv("RM_QS_WORKER_PROCESS_CHANNEL")
	ready := os.Getenv("RM_QS_WORKER_PROCESS_READY")
	blocked := os.Getenv("RM_QS_WORKER_PROCESS_BLOCKED")
	if !strings.HasPrefix(topic, "qs-worker-process-") || channel != topic+"-worker" ||
		!strings.HasPrefix(ready, "/tmp/") || !strings.HasPrefix(blocked, "/tmp/") {
		t.Fatal("Worker process child requires invocation-owned topic and markers")
	}
	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}
	var unknownRecorder DeadLetterRecorder = recorder
	if mode == "crash" {
		unknownRecorder = deadLetterRecorderFunc(func(_ context.Context, _ DeadLetterRecord) error {
			if err := os.WriteFile(blocked, []byte("received before durable record"), 0o600); err != nil {
				return err
			}
			select {} // The parent kills this OS process while its NSQ delivery is in flight.
		})
	}
	observer := &workerSettlementObserver{}
	subscriber, err := NewSubscriber(SubscriberConfig{
		Provider: "nsq", NSQLookupdAddr: "nsqlookupd:4161", NSQMessageTimeout: 5 * time.Second,
	}, basemessaging.SubscriberOptions{
		MaxInFlight: 1, MaxAttempts: 5,
		RetryBackoff:         basemessaging.RetryBackoffOptions{BaseDelay: 500 * time.Millisecond, MaxDelay: 500 * time.Millisecond},
		FailedMessageHandler: FailedMessageHandler(recorder),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer subscriber.Close()
	if err := workermessaging.SubscribeHandlersWithOptions(workermessaging.SubscribeHandlersOptions{
		ServiceName: channel, Logger: slog.Default(), Runtime: &workerSettlementRuntime{topic: topic},
		Subscriber: subscriber, Observer: observer,
		UnknownRecorder: NewUnknownEventRecorder("nsq", unknownRecorder),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ready, []byte(mode), 0o600); err != nil {
		t.Fatal(err)
	}
	if mode == "crash" {
		select {}
	}
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for observer.unknownAcked.Load() != 1 {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("replacement Worker did not confirm the recovered unknown event")
		}
	}
	waitForWorkerProcessNSQDrain(t, topic, channel)
}

func waitForWorkerProcessMarker(t *testing.T, path string, childDone <-chan error) {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case err := <-childDone:
			t.Fatalf("Worker process exited before marker %q: %v", filepath.Base(path), err)
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("Worker process did not reach marker %q", filepath.Base(path))
		}
	}
}

// NSQ does not expose a channel FIN count in /stats. Keep the channel empty
// longer than this test's five-second message timeout after the replacement
// observer reports confirmation, so an unconfirmed delivery would reappear.
func waitForWorkerProcessNSQDrain(t *testing.T, topic, channel string) {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	var emptySince time.Time
	for {
		response, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://" + integrationEnv("NSQD_HTTP_ADDR", "127.0.0.1:4151") + "/stats?format=json")
		if err != nil {
			t.Fatal(err)
		}
		var stats struct {
			Topics []struct {
				TopicName string `json:"topic_name"`
				Channels  []struct {
					ChannelName   string `json:"channel_name"`
					Depth         int64  `json:"depth"`
					InFlightCount int64  `json:"in_flight_count"`
					DeferredCount int64  `json:"deferred_count"`
				} `json:"channels"`
			} `json:"topics"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&stats)
		_ = response.Body.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		empty := false
		for _, item := range stats.Topics {
			if item.TopicName != topic {
				continue
			}
			for _, consumer := range item.Channels {
				if consumer.ChannelName == channel {
					empty = consumer.Depth == 0 && consumer.InFlightCount == 0 && consumer.DeferredCount == 0
				}
			}
		}
		if empty {
			if emptySince.IsZero() {
				emptySince = time.Now()
			}
			if time.Since(emptySince) >= 6*time.Second {
				return
			}
		} else {
			emptySince = time.Time{}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("recovered delivery did not remain drained beyond its NSQ message timeout")
		}
	}
}
