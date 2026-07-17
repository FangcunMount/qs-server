package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
)

func TestActivePublishedFilterSupportsCanonicalAndLegacyRows(t *testing.T) {
	t.Parallel()
	filter := activePublishedFilter(bson.M{"code": "MODEL-1"})
	or, ok := filter["$or"].(bson.A)
	if !ok || len(or) != 2 {
		t.Fatalf("active filter = %#v, want canonical and legacy branches", filter)
	}
	canonical := or[0].(bson.M)
	if canonical["release_status"] != string(domain.ReleaseStatusActive) {
		t.Fatalf("canonical release status = %#v", canonical["release_status"])
	}
}
