//go:build integration

package transport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
)

func TestNSQDeliveryExhaustionPersistsMySQLDeadLetter(t *testing.T) {
	if os.Getenv("MESSAGING_INTEGRATION") != "1" {
		t.Skip("set MESSAGING_INTEGRATION=1 and start NSQ/MySQL integration services")
	}
	db := openIsolatedDeadLetterDatabase(t)
	recorder, err := NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}

	lookupd := integrationEnv("NSQ_LOOKUPD_ADDR", "127.0.0.1:4161")
	nsqdAddress := integrationEnv("NSQD_ADDR", "127.0.0.1:4150")
	topic := fmt.Sprintf("qs-server-dead-letter-%d", time.Now().UnixNano())
	channel := topic + "-worker"
	cleanupNSQTopics(t, topic, nsqFailedHandoffTopic(topic, channel))
	createNSQTopicAndChannel(t, topic, channel)

	var handlerCalls atomic.Int32
	options := basemessaging.SubscriberOptions{
		MaxInFlight: 1,
		MaxAttempts: 2,
		RetryBackoff: basemessaging.RetryBackoffOptions{
			BaseDelay: 10 * time.Millisecond,
			MaxDelay:  20 * time.Millisecond,
		},
		FailedMessageHandler: FailedMessageHandler(recorder),
	}
	subscriber, err := NewSubscriber(SubscriberConfig{Provider: "nsq", NSQLookupdAddr: lookupd}, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscriber.Close() })
	wantCause := errors.New("qs-server transport integration handler failed")
	if err := subscriber.Subscribe(topic, channel, func(context.Context, *basemessaging.Message) error {
		handlerCalls.Add(1)
		return wantCause
	}); err != nil {
		t.Fatal(err)
	}

	publisher, err := cbnsq.NewPublisher(nsqdAddress, nsq.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	payload := []byte(`{"id":"transport-event-1","data":{"org_id":7}}`)
	message := basemessaging.NewMessage("transport-message-1", payload)
	if err := publisher.PublishMessage(t.Context(), topic, message); err != nil {
		t.Fatal(err)
	}

	type deadLetterRow struct {
		MessageID        string
		EventID          string
		OrgID            int64
		Provider         string
		Topic            string
		Channel          string
		DeliveryAttempts int
		Payload          string
		LastError        string
		Disposition      string
	}
	var row deadLetterRow
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		err = db.QueryRowContext(t.Context(), `SELECT message_id,event_id,org_id,provider,topic_name,channel_name,
delivery_attempts,payload_json,last_error,retry_disposition FROM event_delivery_dead_letter WHERE message_id=?`, message.UUID).
			Scan(&row.MessageID, &row.EventID, &row.OrgID, &row.Provider, &row.Topic, &row.Channel, &row.DeliveryAttempts, &row.Payload, &row.LastError, &row.Disposition)
		if err == nil {
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("timed out waiting for NSQ terminal handoff to persist in MySQL")
		}
	}

	if got := handlerCalls.Load(); got != 2 {
		t.Fatalf("handler calls = %d, want 2", got)
	}
	if row.MessageID != message.UUID || row.EventID != "transport-event-1" || row.OrgID != 7 || row.Provider != "nsq" || row.Topic != topic || row.Channel != channel || row.DeliveryAttempts != 2 || row.Payload != string(payload) || row.LastError != wantCause.Error() || row.Disposition != "manual_required" {
		t.Fatalf("dead-letter row = %#v", row)
	}
}

func openIsolatedDeadLetterDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is required for transport integration tests")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("qs_transport_%d", time.Now().UnixNano())
	cfg.DBName = ""
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`") })
	cfg.DBName = databaseName
	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE event_delivery_dead_letter (
 id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
 message_id varchar(128) NOT NULL,
 event_id varchar(128) NULL,
 org_id bigint NULL,
 provider varchar(32) NOT NULL,
 topic_name varchar(255) NOT NULL,
 channel_name varchar(255) NOT NULL,
 delivery_attempts int NOT NULL,
 payload_json longtext NOT NULL,
 last_error text NULL,
 retry_disposition varchar(32) NOT NULL,
 failed_at datetime(3) NOT NULL,
 created_at datetime(3) NOT NULL,
 updated_at datetime(3) NOT NULL,
 UNIQUE KEY uq_delivery_identity (provider,topic_name,channel_name,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	return db
}

func nsqFailedHandoffTopic(topic, channel string) string {
	digest := sha256.Sum256([]byte(topic + "\x00" + channel))
	return fmt.Sprintf("cb.failed.%x", digest[:12])
}

func cleanupNSQTopics(t *testing.T, topics ...string) {
	t.Helper()
	httpAddress := integrationEnv("NSQD_HTTP_ADDR", "127.0.0.1:4151")
	t.Cleanup(func() {
		for _, topic := range topics {
			response, err := postNSQAdmin(httpAddress, "/topic/delete?topic="+url.QueryEscape(topic))
			if err != nil {
				t.Errorf("delete NSQ topic %q: %v", topic, err)
				continue
			}
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNotFound && (response.StatusCode < 200 || response.StatusCode >= 300) {
				t.Errorf("delete NSQ topic %q: status %s", topic, response.Status)
			}
		}
	})
}

func createNSQTopicAndChannel(t *testing.T, topic, channel string) {
	t.Helper()
	httpAddress := integrationEnv("NSQD_HTTP_ADDR", "127.0.0.1:4151")
	for _, endpoint := range []string{
		"/topic/create?topic=" + url.QueryEscape(topic),
		"/channel/create?topic=" + url.QueryEscape(topic) + "&channel=" + url.QueryEscape(channel),
	} {
		response, err := postNSQAdmin(httpAddress, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("prepare NSQ topic/channel: status %s", response.Status)
		}
	}
}

func postNSQAdmin(address, path string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+address+path, nil)
	if err != nil {
		return nil, err
	}
	return (&http.Client{Timeout: 5 * time.Second}).Do(request)
}

func integrationEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
