package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type reminderGapRouteFacade struct {
	stubSystemGovernanceFacade
	write func(int64, uint64, systemgov.ReminderGapRecoveryRequest) (*systemgov.ActionRunResult, error)
	read  func(int64, uint64, systemgov.ReminderGapRecoveryRequest) (*systemgov.ActionRunResult, bool, error)
}

func (s reminderGapRouteFacade) AuthorizeReminderGap(_ context.Context, org int64, actor uint64, req systemgov.ReminderGapRecoveryRequest) (*systemgov.ActionRunResult, error) {
	return s.write(org, actor, req)
}
func (s reminderGapRouteFacade) ResolveReminderGap(_ context.Context, org int64, actor uint64, req systemgov.ReminderGapRecoveryRequest) (*systemgov.ActionRunResult, bool, error) {
	return s.read(org, actor, req)
}

func TestReminderGapRoutesRequireAdminAndTrustedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	calls := 0
	check := func(org int64, actor uint64, req systemgov.ReminderGapRecoveryRequest) {
		calls++
		require.EqualValues(t, 88, org)
		require.EqualValues(t, 701, actor)
		require.Equal(t, "original-task", req.TaskID)
		require.Equal(t, "original-event", req.OpeningEventID)
	}
	facade := reminderGapRouteFacade{
		write: func(org int64, actor uint64, req systemgov.ReminderGapRecoveryRequest) (*systemgov.ActionRunResult, error) {
			check(org, actor, req)
			return &systemgov.ActionRunResult{RequestID: req.RequestID, Status: "succeeded"}, nil
		},
		read: func(org int64, actor uint64, req systemgov.ReminderGapRecoveryRequest) (*systemgov.ActionRunResult, bool, error) {
			check(org, actor, req)
			return nil, false, nil
		},
	}
	router := newRouterWithBudgets(Deps{SystemGovernanceFacade: facade})
	body := `{"request_id":"review","task_id":"original-task","opening_event_id":"original-event","org_id":999,"actor_id":999}`
	for _, admin := range []bool{false, true} {
		engine := gin.New()
		if admin {
			engine.Use(orgAdminSnapshotMiddleware())
		}
		engine.Use(func(c *gin.Context) {
			c.Set(restmiddleware.OrgIDKey, uint64(88))
			c.Set(restmiddleware.UserIDKey, uint64(701))
			c.Next()
		})
		router.registerSystemGovernanceInternalRoutes(engine.Group("/internal/v1"))
		for _, suffix := range []string{"", "/resolve"} {
			before := calls
			request := httptest.NewRequest(http.MethodPost, "/internal/v1/system-governance/actions/reminder-gap-recoveries"+suffix, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if !admin {
				require.Equal(t, http.StatusForbidden, response.Code)
				require.Equal(t, before, calls)
			} else if suffix == "/resolve" {
				require.Equal(t, http.StatusNotFound, response.Code)
			} else {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			}
		}
	}
	require.Equal(t, 2, calls)
}
