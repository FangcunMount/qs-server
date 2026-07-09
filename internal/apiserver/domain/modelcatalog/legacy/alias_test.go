package legacy_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy/behavioral"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy/cognitive"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy/typology"
)

func TestLegacyAdaptersExposeCompatibilityTypes(t *testing.T) {
	t.Parallel()

	if (&scale.Snapshot{Status: "published"}).IsPublished() != true {
		t.Fatal("scale legacy snapshot should preserve IsPublished behavior")
	}
	if (&typology.Payload{Status: "published"}).IsPublished() != true {
		t.Fatal("typology legacy payload should preserve IsPublished behavior")
	}
	if (&behavioral.Snapshot{Status: "published"}).IsPublished() != true {
		t.Fatal("behavioral legacy snapshot should preserve IsPublished behavior")
	}
	if (&cognitive.Snapshot{Status: "published"}).IsPublished() != true {
		t.Fatal("cognitive legacy snapshot should preserve IsPublished behavior")
	}
}
