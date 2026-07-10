package definition_test

import (
	"testing"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
)

func TestValidateSharedFactorPayloadForPublishPreservesInvalidJSONIssue(t *testing.T) {
	t.Parallel()

	issues := appdefinition.ValidateSharedFactorPayloadForPublish([]byte(`{"dimensions":`))
	if len(issues) != 1 || issues[0].Code != "definition.payload.invalid" || issues[0].Field != "definition.payload" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateSharedFactorPayloadForPublishMapsHierarchyIssues(t *testing.T) {
	t.Parallel()

	issues := appdefinition.ValidateSharedFactorPayloadForPublish([]byte(`{"dimensions":[]}`))
	if len(issues) != 1 || issues[0].Code != "dimensions.required" || issues[0].Field != "dimensions" {
		t.Fatalf("issues = %#v", issues)
	}
}
