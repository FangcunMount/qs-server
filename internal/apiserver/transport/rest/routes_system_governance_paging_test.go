package rest

import (
	"net/http/httptest"
	"strings"
	"testing"

	gov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func TestGovernanceNumericPagingPreservesDefaultsAndBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"pending-reconciliations", "delivery-replay-reviews"} {
		for _, page := range []struct {
			query, cursor string
			limit         int
			message       string
		}{
			{"", "", 50, ""},
			{"?cursor=01&limit=1", "01", 1, ""},
			{"?cursor=18446744073709551615&limit=100", "18446744073709551615", 100, ""},
			{"?cursor=0", "", 0, "invalid cursor"},
			{"?cursor=bad", "", 0, "invalid cursor"},
			{"?cursor=18446744073709551616", "", 0, "invalid cursor"},
			{"?limit=0", "", 0, "limit must be between 1 and 100"},
			{"?limit=101", "", 0, "limit must be between 1 and 100"},
			{"?limit=bad&cursor=bad", "", 0, "limit must be between 1 and 100"},
		} {
			t.Run(path+page.query, func(t *testing.T) {
				calls := 0
				check := func(orgID int64, cursor string, limit int) {
					calls++
					if orgID != 88 || cursor != page.cursor || limit != page.limit {
						t.Fatalf("page scope = %d/%q/%d", orgID, cursor, limit)
					}
				}
				facade := stubSystemGovernanceFacade{
					pendingFn: func(orgID int64, cursor string, limit int) (*gov.PendingReplayAuditPage, error) {
						check(orgID, cursor, limit)
						return &gov.PendingReplayAuditPage{}, nil
					},
					deliveryReviewFn: func(orgID int64, cursor string, limit int) (*gov.DeliveryReplayReviewPage, error) {
						check(orgID, cursor, limit)
						return &gov.DeliveryReplayReviewPage{}, nil
					},
				}
				engine := gin.New()
				engine.Use(orgAdminSnapshotMiddleware())
				engine.Use(func(c *gin.Context) { c.Set(restmiddleware.OrgIDKey, uint64(88)); c.Next() })
				newRouterWithBudgets(Deps{SystemGovernanceFacade: facade}).registerSystemGovernanceInternalRoutes(engine.Group("/internal/v1"))
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, httptest.NewRequest("GET", "/internal/v1/system-governance/actions/"+path+page.query, nil))
				if page.message == "" {
					if response.Code != 200 || calls != 1 {
						t.Fatalf("status/calls = %d/%d", response.Code, calls)
					}
				} else if response.Code != 400 || calls != 0 || !strings.Contains(response.Body.String(), page.message) {
					t.Fatalf("status/calls/body = %d/%d/%s", response.Code, calls, response.Body.String())
				}
			})
		}
	}
}
