package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	delivery "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventdelivery"
	auditstore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	middleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This composes real HTTP authorization, replay storage and action receipts.
// IAM identity and publishing are controlled fixtures, not broker/business proof.
func TestDeliveryReplayHTTPAuditMySQL(t *testing.T) {
	dsn := os.Getenv("QS_DELIVERY_REPLAY_HTTP_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_DELIVERY_REPLAY_HTTP_MYSQL_REQUIRED") == "1" {
			t.Fatal("isolated MySQL required")
		}
		t.Skip("isolated MySQL not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	var database string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&database).Error)
	require.True(t, strings.HasPrefix(database, "qs_m5_delivery_http_test_"))
	for _, migration := range []string{"000048_add_system_governance_action_runs.up.sql", "000049_add_retry_governance.up.sql"} {
		ddl, err := os.ReadFile("../../../pkg/migration/migrations/mysql/" + migration)
		require.NoError(t, err)
		// Migration 49 also mutates unrelated business tables. Use its exact DDL for
		// the dead-letter table only; connection-local tables protect other data.
		text := string(ddl)
		if migration == "000049_add_retry_governance.up.sql" {
			at := strings.Index(text, "CREATE TABLE `event_delivery_dead_letter`")
			require.GreaterOrEqual(t, at, 0)
			text = text[at:]
			text = strings.Replace(text, "`message_id` varchar(128) NOT NULL,", "`message_id` varchar(128) NOT NULL,\n`transport_message_id` varchar(64) NOT NULL DEFAULT '',", 1)
			text = strings.Replace(text, "(`provider`, `topic_name`, `channel_name`, `message_id`)", "(`provider`, `topic_name`, `channel_name`, `message_id`, `transport_message_id`)", 1)
		}
		require.NoError(t, db.Exec(strings.Replace(text, "CREATE TABLE", "CREATE TEMPORARY TABLE", 1)).Error)
	}
	payloads := map[uint64]string{}
	ids := map[uint64]string{}
	for _, id := range []uint64{1, 2, 3} {
		evt := event.New("evaluation.retry.requested", "Evaluation", "42", map[string]any{"org_id": int64(7)})
		payload, err := json.Marshal(evt)
		require.NoError(t, err)
		payloads[id], ids[id] = string(payload), evt.EventID()
		org := 7
		if id == 2 {
			org = 8
		}
		require.NoError(t, db.Exec(`INSERT INTO event_delivery_dead_letter
   (id,message_id,event_id,org_id,provider,topic_name,channel_name,delivery_attempts,payload_json,retry_disposition,failed_at)
   VALUES (?,?,?,?, 'nsq','evaluation.retry.requested','worker',8,?,'manual_required',NOW(3))`, id, evt.EventID(), evt.EventID(), org, string(payload)).Error)
	}
	publisher := &deliveryHTTPPublisher{}
	actions := systemgov.NewActionExecutorWithResilience(systemgov.NewActionRegistry(), nil, nil, nil, auditstore.NewActionAuditStore(db)).BindDeliveryReplay(delivery.NewStore(db), publisher)
	router := newRouterWithBudgets(Deps{SystemGovernanceFacade: systemgov.NewFacade(systemgov.FacadeDeps{Actions: actions})})
	gin.SetMode(gin.TestMode)
	call := func(allowed bool, actor uint64, request string, target uint64, reason string) *httptest.ResponseRecorder {
		t.Helper()
		engine := gin.New()
		if allowed {
			engine.Use(orgAdminSnapshotMiddleware())
		}
		engine.Use(func(c *gin.Context) {
			c.Set(middleware.OrgIDKey, uint64(7))
			c.Set(middleware.UserIDKey, actor)
			c.Request = c.Request.WithContext(actorctx.WithGrantingUserID(c.Request.Context(), actor))
			c.Next()
		})
		router.registerSystemGovernanceInternalRoutes(engine.Group("/internal/v1"))
		body, err := json.Marshal(map[string]any{"request_id": request, "confirm": true, "org_id": 8, "actor_id": 999,
			"input": map[string]any{"reason": reason, "targets": []map[string]any{{"id": target, "expected_delivery_attempts": 8}}}})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/system-governance/actions/events.replay_delivery/runs", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		engine.ServeHTTP(result, req)
		return result
	}
	countAudits := func() int64 {
		var n int64
		require.NoError(t, db.Table("system_governance_action_runs").Count(&n).Error)
		return n
	}
	type state struct{ RetryDisposition, PayloadJSON, EventID, ReplayRequestID string }
	read := func(id uint64) state {
		var row state
		require.NoError(t, db.Table("event_delivery_dead_letter").Where("id=?", id).Take(&row).Error)
		return row
	}
	require.Equal(t, http.StatusForbidden, call(false, 11, "denied", 1, "repair").Code)
	require.Zero(t, countAudits())
	require.Empty(t, publisher.ids)
	foreign := call(true, 11, "foreign", 2, "repair")
	require.NotEqual(t, http.StatusOK, foreign.Code)
	require.Equal(t, "manual_required", read(2).RetryDisposition)
	require.Empty(t, publisher.ids)
	result := call(true, 11, "accepted", 1, "repair")
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	require.Equal(t, []string{ids[1]}, publisher.ids)
	row := read(1)
	require.Equal(t, "terminal", row.RetryDisposition)
	require.Equal(t, "accepted", row.ReplayRequestID)
	require.Equal(t, payloads[1], row.PayloadJSON)
	require.Equal(t, ids[1], row.EventID)
	var audit struct {
		OrgID       int64
		ActorUserID uint64
		Status      string
	}
	require.NoError(t, db.Table("system_governance_action_runs").Where("request_id='accepted'").Take(&audit).Error)
	require.EqualValues(t, 7, audit.OrgID)
	require.EqualValues(t, 11, audit.ActorUserID)
	require.Equal(t, "ok", audit.Status)
	count := countAudits()
	retry := call(true, 11, "accepted", 1, "repair")
	require.Equal(t, http.StatusOK, retry.Code, retry.Body.String())
	require.JSONEq(t, result.Body.String(), retry.Body.String())
	require.NotEqual(t, http.StatusOK, call(true, 12, "accepted", 1, "repair").Code)
	require.NotEqual(t, http.StatusOK, call(true, 11, "accepted", 1, "changed").Code)
	require.Equal(t, count, countAudits())
	require.Len(t, publisher.ids, 1)
	require.NotEqual(t, http.StatusOK, call(true, 11, "again", 1, "repair").Code)
	require.Len(t, publisher.ids, 1)
	// A controlled unknown publishing result keeps the original claim. Neither
	// the same HTTP request nor a new request can blindly resend that event.
	publisher.fail = true
	unknown := call(true, 11, "unknown", 3, "repair")
	require.NotEqual(t, http.StatusOK, unknown.Code)
	require.Equal(t, "automatic", read(3).RetryDisposition)
	require.Equal(t, "unknown", read(3).ReplayRequestID)
	require.Equal(t, payloads[3], read(3).PayloadJSON)
	require.Len(t, publisher.ids, 2)
	repeated := call(true, 11, "unknown", 3, "repair")
	require.Equal(t, unknown.Code, repeated.Code)
	require.JSONEq(t, unknown.Body.String(), repeated.Body.String())
	require.NotEqual(t, http.StatusOK, call(true, 11, "new-unknown", 3, "repair").Code)
	require.Len(t, publisher.ids, 2)
}

type deliveryHTTPPublisher struct {
	ids  []string
	fail bool
}

func (p *deliveryHTTPPublisher) Publish(_ context.Context, e event.DomainEvent) error {
	p.ids = append(p.ids, e.EventID())
	if p.fail {
		return errors.New("controlled publish outcome unknown")
	}
	return nil
}
func (p *deliveryHTTPPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, e := range events {
		if err := p.Publish(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
