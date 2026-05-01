package scale

import (
	"context"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type scaleReadModel struct {
	repo *Repository
}

// NewScaleReadModel adapts scale Mongo repositories to the read-model port.
func NewScaleReadModel(repo *Repository) scalereadmodel.ScaleReader {
	return scaleReadModel{repo: repo}
}

func (r scaleReadModel) ListScales(ctx context.Context, filter scalereadmodel.ScaleFilter, page scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	opts := scaleReadModelFindOptions(page)

	cursor, err := r.repo.Collection().Find(ctx, scaleFilterToBSON(filter), opts)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	var poList []ScalePO
	if err := cursor.All(ctx, &poList); err != nil {
		return nil, err
	}
	return scaleRowsFromPO(poList), nil
}

func scaleReadModelFindOptions(page scalereadmodel.PageRequest) *options.FindOptions {
	return options.Find().
		SetSkip(scalePaginationSkip(page.Page, page.PageSize)).
		SetLimit(scalePaginationLimit(page.Page, page.PageSize)).
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetProjection(scaleSummaryProjection())
}

func scaleSummaryProjection() bson.M {
	return bson.M{
		"code":               1,
		"title":              1,
		"description":        1,
		"category":           1,
		"stages":             1,
		"applicable_ages":    1,
		"reporters":          1,
		"tags":               1,
		"questionnaire_code": 1,
		"status":             1,
		"created_by":         1,
		"created_at":         1,
		"updated_by":         1,
		"updated_at":         1,
	}
}

func (r scaleReadModel) CountScales(ctx context.Context, filter scalereadmodel.ScaleFilter) (int64, error) {
	return r.repo.Collection().CountDocuments(ctx, scaleFilterToBSON(filter))
}

func scaleFilterToBSON(filter scalereadmodel.ScaleFilter) bson.M {
	query := bson.M{
		"deleted_at": nil,
	}
	if filter.Status != "" {
		if parsed, ok := domainScale.ParseStatus(filter.Status); ok {
			query["status"] = parsed.String()
		}
	}
	if filter.Title != "" {
		query["title"] = bson.M{"$regex": filter.Title, "$options": "i"}
	}
	if filter.Category != "" {
		query["category"] = filter.Category
	}
	return query
}

func scalePaginationLimit(page, pageSize int) int64 {
	if page <= 0 || pageSize <= 0 {
		return 0
	}
	return int64(pageSize)
}

func scalePaginationSkip(page, pageSize int) int64 {
	if page <= 1 || pageSize <= 0 {
		return 0
	}
	return int64((page - 1) * pageSize)
}

func scaleRowsFromPO(items []ScalePO) []scalereadmodel.ScaleSummaryRow {
	rows := make([]scalereadmodel.ScaleSummaryRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, scalereadmodel.ScaleSummaryRow{
			Code:              item.Code,
			Title:             item.Title,
			Description:       item.Description,
			Category:          item.Category,
			Stages:            append([]string(nil), item.Stages...),
			ApplicableAges:    append([]string(nil), item.ApplicableAges...),
			Reporters:         append([]string(nil), item.Reporters...),
			Tags:              append([]string(nil), item.Tags...),
			QuestionnaireCode: item.QuestionnaireCode,
			Status:            item.Status,
			CreatedBy:         meta.FromUint64(item.CreatedBy),
			CreatedAt:         item.CreatedAt,
			UpdatedBy:         meta.FromUint64(item.UpdatedBy),
			UpdatedAt:         item.UpdatedAt,
		})
	}
	return rows
}
