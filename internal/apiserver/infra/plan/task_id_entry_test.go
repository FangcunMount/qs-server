package plan

import (
	"encoding/json"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewTaskLinkAndOpenedEventContainOnlyTaskID(t *testing.T) {
	now := time.Now()
	task := domain.NewAssessmentTaskAt(domain.NewAssessmentPlanID(), 1, 7, testee.NewID(31), "scale", now, now)
	token, link, err := NewEntryGenerator("https://collect.example/entry?token=legacy&q=forged#legacy").GenerateEntry(t.Context(), task)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(link)
	if token != "" || len(parsed.Query()) != 1 || parsed.Query().Get("task_id") != task.GetID().String() {
		t.Fatalf("token or unrelated locator retained: %s", link)
	}
	if err := domain.NewTaskLifecycle().OpenAt(t.Context(), task, token, link, now); err != nil {
		t.Fatal(err)
	}
	for _, event := range task.Events() {
		data, _ := json.Marshal(event)
		if strings.Contains(string(data), "token") {
			t.Fatal("opening event leaked token")
		}
	}
}
