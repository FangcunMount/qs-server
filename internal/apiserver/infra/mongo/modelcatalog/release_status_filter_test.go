package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
)

func TestActivePublishedFilterRequiresCanonicalReleaseStatus(t *testing.T) {
	t.Parallel()
	filter := activePublishedFilter(bson.M{"code": "MODEL-1"})
	if filter["release_status"] != string(domain.ReleaseStatusActive) {
		t.Fatalf("canonical release status = %#v", filter["release_status"])
	}
	if _, exists := filter["$or"]; exists {
		t.Fatalf("active filter must not contain a compatibility branch: %#v", filter)
	}
}
