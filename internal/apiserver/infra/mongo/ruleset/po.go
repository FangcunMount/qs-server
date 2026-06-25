package ruleset

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

const statusPublished = "published"

// EvaluationRuleSetPO 测评规则集 Mongo 文档。
type EvaluationRuleSetPO struct {
	mongoBase.BaseDocument `bson:",inline"`

	SchemaVersion        string     `bson:"schema_version,omitempty"`
	PayloadFormat        string     `bson:"payload_format,omitempty"`
	RuleSetKind            string     `bson:"ruleset_kind"`
	RuleSetCode            string     `bson:"ruleset_code"`
	RuleSetVersion         string     `bson:"ruleset_version"`
	Title                string     `bson:"title"`
	Status               string     `bson:"status"`
	DecisionKind         string     `bson:"decision_kind"`
	QuestionnaireCode    string     `bson:"questionnaire_code"`
	QuestionnaireVersion string     `bson:"questionnaire_version"`
	Source               bson.M     `bson:"source,omitempty"`
	Payload              []byte     `bson:"payload"`
	PublishedAt          *time.Time `bson:"published_at,omitempty"`
}

func (EvaluationRuleSetPO) CollectionName() string {
	return "evaluation_rule_sets"
}

func (p *EvaluationRuleSetPO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
	p.DeletedBy = 0
}

func (p *EvaluationRuleSetPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
}

func (p *EvaluationRuleSetPO) ToBsonM() (bson.M, error) {
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
