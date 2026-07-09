package modelcatalog

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

const statusPublished = "published"

// PublishedAssessmentModelPO v2 已发布测评模型 Mongo 文档。
type PublishedAssessmentModelPO struct {
	mongoBase.BaseDocument `bson:",inline"`

	SchemaVersion           string        `bson:"schema_version,omitempty"`
	PayloadFormat           string        `bson:"payload_format,omitempty"`
	ModelProductChannel     string        `bson:"model_product_channel,omitempty"`
	ModelKind               string        `bson:"model_kind"`
	ModelSubKind            string        `bson:"model_sub_kind,omitempty"`
	ModelAlgorithm          string        `bson:"model_algorithm,omitempty"`
	ModelCode               string        `bson:"model_code"`
	ModelVersion            string        `bson:"model_version"`
	Title                   string        `bson:"title"`
	Description             string        `bson:"description,omitempty"`
	Category                string        `bson:"category,omitempty"`
	Stages                  []string      `bson:"stages,omitempty"`
	ApplicableAges          []string      `bson:"applicable_ages,omitempty"`
	Reporters               []string      `bson:"reporters,omitempty"`
	Tags                    []string      `bson:"tags,omitempty"`
	Status                  string        `bson:"status"`
	DecisionKind            string        `bson:"decision_kind"`
	QuestionnaireCode       string        `bson:"questionnaire_code"`
	QuestionnaireVersion    string        `bson:"questionnaire_version"`
	Source                  bson.M        `bson:"source,omitempty"`
	Payload                 []byte        `bson:"payload"`
	DefinitionSchemaVersion string        `bson:"definition_schema_version,omitempty"`
	DefinitionV2            *DefinitionPO `bson:"definition_v2,omitempty"`
	PublishedAt             *time.Time    `bson:"published_at,omitempty"`
}

func (PublishedAssessmentModelPO) CollectionName() string {
	return "published_assessment_models"
}

func (p *PublishedAssessmentModelPO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
	p.DeletedBy = 0
}

func (p *PublishedAssessmentModelPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
}

func (p *PublishedAssessmentModelPO) ToBsonM() (bson.M, error) {
	data, err := bson.Marshal(p)
	if err != nil {
		return nil, err
	}
	var result bson.M
	if err := bson.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func publishedFilter(extra bson.M) bson.M {
	filter := bson.M{
		"status":     statusPublished,
		"deleted_at": nil,
	}
	for key, value := range extra {
		filter[key] = value
	}
	return filter
}

// publishedModelUpsertFilter matches the active published row for a model code.
// model_version is intentionally excluded: draft edits bump optimistic version and
// republication must replace the existing snapshot instead of inserting a new row.
func publishedModelUpsertFilter(po *PublishedAssessmentModelPO) bson.M {
	filter := bson.M{
		"model_kind":      po.ModelKind,
		"model_algorithm": po.ModelAlgorithm,
		"model_code":      po.ModelCode,
		"deleted_at":      nil,
	}
	// Empty sub_kind must not be written as "" — legacy rows store null/absent.
	if po.ModelSubKind != "" {
		filter["model_sub_kind"] = po.ModelSubKind
	}
	return filter
}
