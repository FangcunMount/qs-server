package scale

import (
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"go.mongodb.org/mongo-driver/bson"
)

func TestScaleFilterToBSONMapsTypedFilter(t *testing.T) {
	t.Parallel()

	query := scaleFilterToBSON(scalereadmodel.ScaleFilter{
		Status:   "published",
		Title:    "PHQ",
		Category: "mental",
	})

	if got := query["deleted_at"]; got != nil {
		t.Fatalf("deleted_at = %#v, want nil", got)
	}
	if got := query["status"]; got != "published" {
		t.Fatalf("status = %#v, want published", got)
	}
	if got := query["category"]; got != "mental" {
		t.Fatalf("category = %#v, want mental", got)
	}
	titleQuery, ok := query["title"].(bson.M)
	if !ok {
		t.Fatalf("title query = %#v, want bson.M", query["title"])
	}
	if got := titleQuery["$regex"]; got != "PHQ" {
		t.Fatalf("title regex = %#v, want PHQ", got)
	}
	if got := titleQuery["$options"]; got != "i" {
		t.Fatalf("title options = %#v, want i", got)
	}
}

func TestScaleFilterToBSONIgnoresUnknownStatus(t *testing.T) {
	t.Parallel()

	query := scaleFilterToBSON(scalereadmodel.ScaleFilter{Status: "unknown"})
	if _, ok := query["status"]; ok {
		t.Fatalf("status should be omitted for unknown status, got %#v", query["status"])
	}
}

func TestScaleReadModelFindOptionsAppliesPaginationSortAndProjection(t *testing.T) {
	t.Parallel()

	opts := scaleReadModelFindOptions(scalereadmodel.PageRequest{Page: 3, PageSize: 20})
	if opts.Skip == nil || *opts.Skip != 40 {
		t.Fatalf("skip = %#v, want 40", opts.Skip)
	}
	if opts.Limit == nil || *opts.Limit != 20 {
		t.Fatalf("limit = %#v, want 20", opts.Limit)
	}
	if !reflect.DeepEqual(opts.Sort, bson.D{{Key: "created_at", Value: -1}}) {
		t.Fatalf("sort = %#v, want created_at desc", opts.Sort)
	}
	projection, ok := opts.Projection.(bson.M)
	if !ok {
		t.Fatalf("projection = %#v, want bson.M", opts.Projection)
	}
	for _, field := range []string{"code", "title", "description", "category", "stages", "applicable_ages", "reporters", "tags", "questionnaire_code", "status", "created_by", "created_at", "updated_by", "updated_at"} {
		if projection[field] != 1 {
			t.Fatalf("projection[%s] = %#v, want 1", field, projection[field])
		}
	}
}

func TestScaleRowsFromPOCopiesSlicesAndAuditFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	items := []ScalePO{
		{
			Code:              "SCALE_A",
			Title:             "Scale A",
			Description:       "desc",
			Category:          "mental",
			Stages:            []string{"screening"},
			ApplicableAges:    []string{"adult"},
			Reporters:         []string{"self"},
			Tags:              []string{"phq"},
			QuestionnaireCode: "Q_A",
			Status:            "published",
		},
	}
	items[0].CreatedBy = 1001
	items[0].CreatedAt = now
	items[0].UpdatedBy = 1002
	items[0].UpdatedAt = now.Add(time.Minute)

	rows := scaleRowsFromPO(items)
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Code != "SCALE_A" || row.Title != "Scale A" || row.QuestionnaireCode != "Q_A" {
		t.Fatalf("unexpected row identity fields: %#v", row)
	}
	if row.CreatedBy.Uint64() != 1001 || row.UpdatedBy.Uint64() != 1002 {
		t.Fatalf("audit ids = (%d,%d), want (1001,1002)", row.CreatedBy.Uint64(), row.UpdatedBy.Uint64())
	}
	if !row.CreatedAt.Equal(now) || !row.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("audit times = (%s,%s), want (%s,%s)", row.CreatedAt, row.UpdatedAt, now, now.Add(time.Minute))
	}

	items[0].Stages[0] = "mutated"
	items[0].ApplicableAges[0] = "mutated"
	items[0].Reporters[0] = "mutated"
	items[0].Tags[0] = "mutated"
	if row.Stages[0] != "screening" || row.ApplicableAges[0] != "adult" || row.Reporters[0] != "self" || row.Tags[0] != "phq" {
		t.Fatalf("row slices should be copied, got %#v", row)
	}
}
