package interpretationmodel

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

const statusPublished = "published"

// InterpretationModelPO 解释模型规则文档。
type InterpretationModelPO struct {
	mongoBase.BaseDocument `bson:",inline"`

	ModelKind            string     `bson:"model_kind"`
	ModelCode            string     `bson:"model_code"`
	ModelVersion         string     `bson:"model_version"`
	Title                string     `bson:"title"`
	Status               string     `bson:"status"`
	DecisionKind         string     `bson:"decision_kind"`
	QuestionnaireCode    string     `bson:"questionnaire_code"`
	QuestionnaireVersion string     `bson:"questionnaire_version"`
	Source               bson.M     `bson:"source,omitempty"`
	Payload              []byte     `bson:"payload"`
	PublishedAt          *time.Time `bson:"published_at,omitempty"`
}

func (InterpretationModelPO) CollectionName() string {
	return "interpretation_models"
}

func (p *InterpretationModelPO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
	p.DeletedBy = 0
}

func (p *InterpretationModelPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
}

func (p *InterpretationModelPO) ToBsonM() (bson.M, error) {
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
