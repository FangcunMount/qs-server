package aibridge

import (
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"testing"
)

func TestSemanticDraftPayloadIsScopedAndTyped(t *testing.T) {
	scope := app.DraftScope{OrganizationID: 12, OperatorUserID: 34}
	raw := &pb.SemanticDraftResponse{SchemaVersion: "qs-ai-semantic-draft/v1", DataJson: `{"draft":{"draft_id":"00000000-0000-4000-8000-000000000003","organization_id":12,"revision":1,"state":"editing"},"asset":null}`}
	if _, err := semanticDraftPayload(raw, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := semanticDraftPayload(raw, app.DraftScope{OrganizationID: 13, OperatorUserID: 34}); err == nil {
		t.Fatal("cross-organization response accepted")
	}
	for _, value := range []string{`null`, `{}`, `{"draft":{"organization_id":12,"revision":1,"state":"editing"}}`} {
		copy := &pb.SemanticDraftResponse{SchemaVersion: raw.SchemaVersion, DataJson: value}
		if _, err := semanticDraftPayload(copy, scope); err == nil {
			t.Fatal(value)
		}
	}
}
