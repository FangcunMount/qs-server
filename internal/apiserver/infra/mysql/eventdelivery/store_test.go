package eventdelivery

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FangcunMount/component-base/pkg/event"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
)

func TestValidateReplayRowBlocksLegacyTaskOpenedBeforeClaim(t *testing.T) {
	evt := event.New("task.opened", "AssessmentTask", "42", map[string]any{"org_id": int64(9)})
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	orgID := int64(9)
	eventID := evt.EventID()
	row := deadLetterPO{ID: 7, EventID: &eventID, OrgID: &orgID, DeliveryAttempts: 8, PayloadJSON: string(payload), RetryDisposition: "manual_required"}
	target := app.DeliveryReplayTarget{ID: 7, ExpectedDeliveryAttempts: 8}
	if err := validateReplayRow(row, orgID, target); err == nil || !strings.Contains(err.Error(), "manual reconciliation") {
		t.Fatalf("legacy task.opened must be rejected before claim: %v", err)
	}
}
