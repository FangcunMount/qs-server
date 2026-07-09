package modelcatalog

import (
	"context"
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ScaleReadModel lists scale assessment models from assessment_models.
type ScaleReadModel struct {
	repo *DraftRepository
}

var _ scalereadmodel.ScaleReader = (*ScaleReadModel)(nil)

// NewScaleReadModel adapts assessment_models to the scale read-model port.
func NewScaleReadModel(repo *DraftRepository) *ScaleReadModel {
	return &ScaleReadModel{repo: repo}
}

func (r *ScaleReadModel) ListScales(ctx context.Context, filter scalereadmodel.ScaleFilter, page scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	if r == nil || r.repo == nil {
		return nil, nil
	}
	opts := scaleReadModelFindOptions(page)
	cursor, err := r.repo.Collection().Find(ctx, scaleReadModelFilter(filter), opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var poList []AssessmentModelPO
	if err := cursor.All(ctx, &poList); err != nil {
		return nil, err
	}
	return scaleSummaryRowsFromAssessmentPO(poList), nil
}

func (r *ScaleReadModel) CountScales(ctx context.Context, filter scalereadmodel.ScaleFilter) (int64, error) {
	if r == nil || r.repo == nil {
		return 0, nil
	}
	return r.repo.Collection().CountDocuments(ctx, scaleReadModelFilter(filter))
}

func scaleReadModelFindOptions(page scalereadmodel.PageRequest) *options.FindOptions {
	return options.Find().
		SetSkip(scaleReadModelPaginationSkip(page.Page, page.PageSize)).
		SetLimit(scaleReadModelPaginationLimit(page.Page, page.PageSize)).
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetProjection(scaleReadModelProjection())
}

func scaleReadModelProjection() bson.M {
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
		"definition_payload": 1,
		"created_by":         1,
		"created_at":         1,
		"updated_by":         1,
		"updated_at":         1,
	}
}

func scaleReadModelFilter(filter scalereadmodel.ScaleFilter) bson.M {
	query := draftFilter(bson.M{
		"kind": string(domain.KindScale),
	})
	if filter.PublishedOnly {
		query["status"] = string(domain.ModelStatusPublished)
	} else if filter.Status != "" {
		query["status"] = filter.Status
	}
	if filter.Title != "" {
		query["title"] = bson.M{"$regex": filter.Title, "$options": "i"}
	}
	if filter.Category != "" {
		query["category"] = filter.Category
	}
	return query
}

func scaleReadModelPaginationLimit(page, pageSize int) int64 {
	if page <= 0 || pageSize <= 0 {
		return 0
	}
	return int64(pageSize)
}

func scaleReadModelPaginationSkip(page, pageSize int) int64 {
	if page <= 1 || pageSize <= 0 {
		return 0
	}
	return int64((page - 1) * pageSize)
}

func scaleSummaryRowsFromAssessmentPO(items []AssessmentModelPO) []scalereadmodel.ScaleSummaryRow {
	rows := make([]scalereadmodel.ScaleSummaryRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, scalereadmodel.ScaleSummaryRow{
			Code:              item.Code,
			ScaleVersion:      scaleVersionFromDefinitionPayload(item.DefinitionPayload),
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

func scaleVersionFromDefinitionPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var envelope struct {
		ScaleVersion string `json:"scaleVersion"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	return envelope.ScaleVersion
}
