package handler

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/planentry"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
	middleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type taskEntryReader struct {
	entry *planentry.Entry
	err   error
}

func (r taskEntryReader) ResolveTaskEntry(context.Context, string, string) (*planentry.Entry, error) {
	return r.entry, r.err
}

type taskProfileAccess struct {
	err     error
	profile uint64
}

func (a *taskProfileAccess) Authorize(_ context.Context, _ string, profile uint64) error {
	a.profile = profile
	return a.err
}
func TestTaskIDEntryUsesOnlyServerResolvedProfileAndContent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		user    bool
		denied  error
		invalid error
		want    int
	}{
		{name: "anonymous", want: 401}, {name: "authorized", user: true, want: 200},
		{name: "foreign user", user: true, denied: testeeaccess.ErrAccessDenied, want: 403},
		{name: "invalid task cannot fall back", user: true, invalid: planentry.ErrInvalidEntry, want: 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("GET", "/plan-task-entries/42?q=forged&t=999&testee_id=999", nil)
			c.Params = gin.Params{{Key: "task_id", Value: "42"}}
			if tc.user {
				c.Set("user_claims", &middleware.UserClaims{UserID: "11"})
			}
			access := &taskProfileAccess{err: tc.denied}
			service := planentry.NewService(taskEntryReader{entry: &planentry.Entry{TaskID: "42", TesteeID: "31", QuestionnaireCode: "exact", QuestionnaireVersion: "v1"}, err: tc.invalid}, access)
			NewPlanEntryHandler(service).Resolve(c)
			if recorder.Code != tc.want {
				t.Fatalf("code=%d body=%s", recorder.Code, recorder.Body)
			}
			if tc.want == 200 && (access.profile != 31 || strings.Contains(recorder.Body.String(), "forged") || !strings.Contains(recorder.Body.String(), "exact")) {
				t.Fatal("client parameters changed task scope")
			}
		})
	}
}
