package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	ledger "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/notification"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Exercise the production router, handler, facade and SQL store together. The
// IAM capability snapshot is a fixture; this is not a production login test.
func TestReminderResolutionHTTPReceiptMySQL(t *testing.T) {
	dsn := os.Getenv("QS_REMINDER_RESOLUTION_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_REMINDER_RESOLUTION_MYSQL_REQUIRED") == "1" {
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
	require.True(t, strings.HasPrefix(database, "qs_m5_reminder_resolution_test_"))
	for _, name := range []string{"000048_add_system_governance_action_runs.up.sql", "000087_task_opened_reminder_delivery.up.sql"} {
		ddl, err := os.ReadFile("../../../pkg/migration/migrations/mysql/" + name)
		require.NoError(t, err)
		require.NoError(t, db.Exec(strings.Replace(string(ddl), "CREATE TABLE", "CREATE TEMPORARY TABLE", 1)).Error)
	}
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	require.NoError(t, db.Exec(`INSERT INTO task_opened_reminder_delivery
 (id,org_id,task_id,opening_event_id,schedule_revision,user_id,login_identity_id,app_id,template_id,reminder_version,state,external_call_started_at,resolution_code,created_at,updated_at)
 VALUES (1,7,'task-http','event-http',1,'user-http','identity-http','app-http','template-http',1,'sending',?,'',?,?)`, now, now, now).Error)
	facade := systemgov.NewFacade(systemgov.FacadeDeps{ReminderResolver: store.NewActionAuditStore(db)})
	router := newRouterWithBudgets(Deps{SystemGovernanceFacade: facade})
	gin.SetMode(gin.TestMode)
	request := map[string]any{
		"request_id": "http-review", "delivery_id": 1, "task_id": "task-http", "opening_event_id": "event-http",
		"expected_updated_at": now, "finding": "unknown_no_resend", "evidence_reference": "isolated-incident",
		"reason": "unknown outcome; no resend", "confirm": true, "acknowledge_original_call_may_complete": true,
		"org_id": 999, "actor_id": 999,
	}
	const endpoint = "/internal/v1/system-governance/actions/reminder-resolutions"
	call := func(allowed bool, org, actor uint64, method, path string, body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		engine := gin.New()
		if allowed {
			engine.Use(orgAdminSnapshotMiddleware())
		}
		engine.Use(func(c *gin.Context) {
			c.Set(restmiddleware.OrgIDKey, org)
			c.Set(restmiddleware.UserIDKey, actor)
			c.Next()
		})
		router.registerSystemGovernanceInternalRoutes(engine.Group("/internal/v1"))
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	countAudits := func() int64 {
		t.Helper()
		var count int64
		require.NoError(t, db.Table("system_governance_action_runs").Count(&count).Error)
		return count
	}
	require.Equal(t, http.StatusForbidden, call(false, 7, 11, http.MethodPost, endpoint, request).Code)
	require.NotEqual(t, http.StatusOK, call(true, 8, 11, http.MethodPost, endpoint, request).Code)
	require.Zero(t, countAudits())
	result := call(true, 7, 11, http.MethodPost, endpoint, request)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	var envelope struct {
		Data systemgov.ActionRunResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &envelope))
	require.Equal(t, "http-review", envelope.Data.RequestID)
	require.Equal(t, false, envelope.Data.Result["automatic_resend"])
	// Simulate a client losing the successful response: recovery reads the same
	// committed receipt, and an identical retry cannot create another action.
	receipt := call(true, 7, 11, http.MethodGet, endpoint+"/http-review", nil)
	require.Equal(t, http.StatusOK, receipt.Code, receipt.Body.String())
	require.JSONEq(t, result.Body.String(), receipt.Body.String())
	retry := call(true, 7, 11, http.MethodPost, endpoint, request)
	require.Equal(t, http.StatusOK, retry.Code, retry.Body.String())
	require.JSONEq(t, result.Body.String(), retry.Body.String())
	require.NotEqual(t, http.StatusOK, call(true, 7, 12, http.MethodGet, endpoint+"/http-review", nil).Code)
	require.NotEqual(t, http.StatusOK, call(true, 8, 11, http.MethodGet, endpoint+"/http-review", nil).Code)
	request["reason"] = "changed input"
	require.NotEqual(t, http.StatusOK, call(true, 7, 11, http.MethodPost, endpoint, request).Code)
	require.EqualValues(t, 1, countAudits())
	var row struct{ State, ResolutionCode string }
	require.NoError(t, db.Table("task_opened_reminder_delivery").Where("id = 1").Take(&row).Error)
	require.Equal(t, "reviewed", row.State)
	require.Equal(t, "manual_unknown_no_resend", row.ResolutionCode)
	key := notification.ReminderDeliveryKey{OrgID: 7, TaskID: "task-http", OpeningEventID: "event-http", ScheduleRevision: 1, LoginIdentityID: "identity-http", AppID: "app-http", TemplateID: "template-http", ReminderVersion: 1}
	_, claimed, err := ledger.NewReminderDeliveryLedger(db).Claim(t.Context(), key, time.Minute, now.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, claimed)
}
