package modelcatalog_test

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestNewAssessmentModelRejectsMigrationKinds(t *testing.T) {
	t.Parallel()

	for _, kind := range []modelcatalog.Kind{
		modelcatalog.KindMBTIMigration,
		modelcatalog.KindSBTIMigration,
	} {
		kind := kind
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			_, err := modelcatalog.NewAssessmentModel(modelcatalog.NewAssessmentModelInput{
				Code:  "legacy-model",
				Kind:  kind,
				Title: "legacy",
				Now:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			})
			if err == nil {
				t.Fatalf("NewAssessmentModel(%s) should reject legacy flat kind", kind)
			}
		})
	}
}
