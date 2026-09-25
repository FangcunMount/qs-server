//go:build integration

package eventdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type exitAfterReplayPublish struct{ next event.EventPublisher }

func (p exitAfterReplayPublish) Publish(ctx context.Context, evt event.DomainEvent) error {
	if err := p.next.Publish(ctx, evt); err != nil {
		return err
	}
	// NSQ acknowledged PUB, but the process cannot settle the MySQL claim.
	os.Exit(0)
	return nil
}

func (p exitAfterReplayPublish) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func TestDeliveryReplayPublishedEventSurvivesProcessExitWithoutResend(t *testing.T) {
	if dsn := os.Getenv("QS_REPLAY_PUBLISHED_CHILD_DSN"); dsn != "" {
		db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		publisher := newReplayNSQPublisher(t, os.Getenv("QS_REPLAY_NSQ_ADDR"), os.Getenv("QS_REPLAY_NSQ_TOPIC"))
		executor := app.NewActionExecutor(app.NewActionRegistry(), nil).
			BindDeliveryReplay(NewStore(db), exitAfterReplayPublish{next: publisher})
		_, err = executor.Run(t.Context(), 7, "events.replay_delivery", replayNSQAction("published-then-exited"))
		t.Fatalf("child must exit after the broker confirms publish; run returned %v", err)
	}

	nsqAddr, nsqHTTP := os.Getenv("QS_REPLAY_NSQ_ADDR"), os.Getenv("QS_REPLAY_NSQ_HTTP_ADDR")
	if nsqAddr == "" || nsqHTTP == "" {
		t.Skip("disposable QS_REPLAY_NSQ_ADDR and QS_REPLAY_NSQ_HTTP_ADDR are required")
	}
	db := openDeliveryReplayMySQL(t)
	evt := event.New("evaluation.retry.requested", "Evaluation", "42", map[string]any{"org_id": int64(7)})
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO event_delivery_dead_letter
		(id,message_id,event_id,org_id,delivery_attempts,payload_json,retry_disposition,updated_at)
		VALUES (1,'m1',?,7,8,?,'manual_required',?)`, evt.EventID(), string(payload), time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	topic := fmt.Sprintf("qs-m5-replay-%d", time.Now().UnixNano())
	channel := "replay-proof"
	replayNSQPost(t, nsqHTTP, "/topic/create?topic="+url.QueryEscape(topic))
	replayNSQPost(t, nsqHTTP, "/channel/create?topic="+url.QueryEscape(topic)+"&channel="+url.QueryEscape(channel))
	t.Cleanup(func() {
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Post("http://"+nsqHTTP+"/topic/delete?topic="+url.QueryEscape(topic), "", nil)
		if err != nil {
			t.Errorf("delete isolated NSQ topic: %v", err)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("delete isolated NSQ topic returned %s", response.Status)
		}
	})
	var databaseName string
	if err := db.Raw("SELECT DATABASE()").Scan(&databaseName).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err := drivermysql.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = databaseName
	cfg.ParseTime = true
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDeliveryReplayPublishedEventSurvivesProcessExitWithoutResend$")
	child.Env = append(os.Environ(),
		"QS_REPLAY_PUBLISHED_CHILD_DSN="+cfg.FormatDSN(),
		"QS_REPLAY_NSQ_ADDR="+nsqAddr,
		"QS_REPLAY_NSQ_TOPIC="+topic,
	)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("publishing process failed: %v\n%s", err, output)
	}
	checkDisposition(t, db, 1, "automatic", "published-then-exited", "")
	if count := replayNSQMessageCount(t, nsqHTTP, topic, channel); count != 1 {
		t.Fatalf("broker message count after acknowledged publish = %d, want 1", count)
	}

	received := make(chan []byte, 1)
	consumer, err := nsq.NewConsumer(topic, channel, nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	consumer.AddHandler(nsq.HandlerFunc(func(message *nsq.Message) error {
		received <- append([]byte(nil), message.Body...)
		return nil
	}))
	if err := consumer.ConnectToNSQD(nsqAddr); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { consumer.Stop(); <-consumer.StopChan })
	select {
	case body := <-received:
		message, ok, err := basemessaging.DecodeMessagePayload(body)
		if err != nil || !ok || message.UUID != evt.EventID() {
			t.Fatalf("broker event identity = %#v, envelope=%t, error=%v; want %s", message, ok, err, evt.EventID())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("acknowledged event was not delivered from NSQ")
	}
	publisher := newReplayNSQPublisher(t, nsqAddr, topic)
	executor := app.NewActionExecutor(app.NewActionRegistry(), nil).BindDeliveryReplay(NewStore(db), publisher)
	if _, err := executor.Run(t.Context(), 7, "events.replay_delivery", replayNSQAction("new-request")); err == nil {
		t.Fatal("a new request must not publish the claimed event again")
	}
	if count := replayNSQMessageCount(t, nsqHTTP, topic, channel); count != 1 {
		t.Fatalf("broker message count after denied replay = %d, want 1", count)
	}
}

func replayNSQAction(requestID string) app.ActionRunRequest {
	return app.ActionRunRequest{
		RequestID: requestID,
		Confirm:   true,
		Input: map[string]interface{}{
			"reason":  "isolated crash recovery proof",
			"targets": []interface{}{map[string]interface{}{"id": 1, "expected_delivery_attempts": 8}},
		},
	}
}

func newReplayNSQPublisher(t *testing.T, address, topic string) event.EventPublisher {
	t.Helper()
	mq, err := cbnsq.NewPublisher(address, nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mq.Close() })
	catalog := eventcatalog.NewCatalog(&eventcatalog.Config{
		Topics: map[string]eventcatalog.TopicConfig{"replay": {Name: topic}},
		Events: map[string]eventcatalog.EventConfig{"evaluation.retry.requested": {Topic: "replay"}},
	})
	return eventruntime.NewRoutingPublisher(eventruntime.RoutingPublisherOptions{
		Catalog: catalog, MQPublisher: mq, Mode: eventruntime.PublishModeMQ,
	})
}

func replayNSQPost(t *testing.T, address, path string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Post("http://"+address+path, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("NSQ admin %s returned %s", path, response.Status)
	}
}

func replayNSQMessageCount(t *testing.T, address, topic, channel string) uint64 {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + address + "/stats?format=json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("NSQ stats returned %s", response.Status)
	}
	var stats struct {
		Topics []struct {
			Name     string `json:"topic_name"`
			Channels []struct {
				Name         string `json:"channel_name"`
				MessageCount uint64 `json:"message_count"`
			} `json:"channels"`
		} `json:"topics"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	for _, foundTopic := range stats.Topics {
		if foundTopic.Name == topic {
			for _, foundChannel := range foundTopic.Channels {
				if foundChannel.Name == channel {
					return foundChannel.MessageCount
				}
			}
		}
	}
	t.Fatalf("NSQ channel %s/%s missing", topic, channel)
	return 0
}
