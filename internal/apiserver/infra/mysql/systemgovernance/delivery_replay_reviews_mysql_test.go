package systemgovernance

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDeliveryReplayReviewsKeepStaleAuditsVisibleWithoutCrossOrgRows(t *testing.T) {
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_ACTION_AUDIT_REQUIRE_MYSQL") == "true" {
			t.Fatal("QS_SERVER_TEST_MYSQL_DSN is required")
		}
		t.Skip("requires isolated MySQL")
	}
	db, err := gorm.Open(mysqlDriver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	for _, path := range []string{
		"../../../../../internal/pkg/migration/migrations/mysql/000048_add_system_governance_action_runs.up.sql",
		"../../../../../internal/pkg/migration/migrations/mysql/000049_add_retry_governance.up.sql",
	} {
		ddl, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		source := string(ddl)
		if strings.Contains(path, "000049") {
			source = source[strings.Index(source, "CREATE TABLE `event_delivery_dead_letter`"):]
		}
		if err := db.Exec(strings.Replace(source, "CREATE TABLE", "CREATE TEMPORARY TABLE", 1)).Error; err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-10 * time.Minute)
	newer := time.Now()
	rows := []actionRunPO{
		{RequestID: "unclaimed", ActionID: "events.replay_delivery", OrgID: 7, ActorUserID: 11, InputJSON: `{"targets":[{"id":1}],"reason":"private text"}`, Status: "running", ResultJSON: "null", StartedAt: old},
		{RequestID: "claimed", ActionID: "events.replay_delivery", OrgID: 7, ActorUserID: 12, InputJSON: `{"targets":[{"id":2},{"id":3}],"reason":"private text"}`, Status: "running", ResultJSON: "null", StartedAt: old},
		{RequestID: "malformed", ActionID: "events.replay_delivery", OrgID: 7, ActorUserID: 13, InputJSON: `{"targets":"bad"}`, Status: "running", ResultJSON: "null", StartedAt: old},
		{RequestID: "recent", ActionID: "events.replay_delivery", OrgID: 7, InputJSON: `{"targets":[{"id":1}]}`, Status: "running", ResultJSON: "null", StartedAt: newer},
		{RequestID: "finished", ActionID: "events.replay_delivery", OrgID: 7, InputJSON: `{"targets":[{"id":1}]}`, Status: "failed", ResultJSON: "null", StartedAt: old},
		{RequestID: "other-org", ActionID: "events.replay_delivery", OrgID: 8, InputJSON: `{"targets":[{"id":3}]}`, Status: "running", ResultJSON: "null", StartedAt: old},
		{RequestID: "other-action", ActionID: "events.replay_pending", OrgID: 7, InputJSON: `{"targets":[{"id":1}]}`, Status: "running", ResultJSON: "null", StartedAt: old},
	}
	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		id    uint64
		org   int
		state string
		claim any
	}{
		{1, 7, "manual_required", nil},
		{2, 7, "automatic", "claimed"},
		{3, 8, "automatic", "other-org"},
	} {
		if err := db.Exec(`INSERT INTO event_delivery_dead_letter
            (id,message_id,event_id,org_id,provider,topic_name,channel_name,delivery_attempts,payload_json,retry_disposition,replay_request_id,failed_at)
            VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, row.id, "msg-"+strconv.FormatUint(row.id, 10), nil, row.org, "nsq", "topic", "channel", 1, "{}", row.state, row.claim, old).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := NewActionAuditStore(db)
	first, err := store.ListDeliveryReplayReviews(context.Background(), 7, "", 2)
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	if first.Items[0].RequestID != "malformed" || first.Items[0].TargetsReadable || len(first.Items[0].Targets) != 0 {
		t.Fatalf("invalid input must remain visible without raw input: %+v", first.Items[0])
	}
	if first.Items[1].RequestID != "claimed" || !first.Items[1].TargetsReadable || len(first.Items[1].Targets) != 2 ||
		first.Items[1].Targets[0].Disposition != "automatic" || !first.Items[1].Targets[0].LinkedToRequest ||
		first.Items[1].Targets[1].Disposition != "unavailable" || first.Items[1].Targets[1].LinkedToRequest {
		t.Fatalf("claimed/foreign targets: %+v", first.Items[1])
	}
	second, err := store.ListDeliveryReplayReviews(context.Background(), 7, first.NextCursor, 2)
	if err != nil || len(second.Items) != 1 || second.Items[0].RequestID != "unclaimed" || second.NextCursor != "" ||
		second.Items[0].Targets[0].Disposition != "manual_required" || second.Items[0].Targets[0].LinkedToRequest {
		t.Fatalf("unclaimed target and cursor: %+v, %v", second, err)
	}
	if _, err := store.ListDeliveryReplayReviews(context.Background(), 7, "bad", 2); err == nil {
		t.Fatal("invalid cursor accepted")
	}
}
