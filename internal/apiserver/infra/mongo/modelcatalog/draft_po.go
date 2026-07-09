package modelcatalog

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// AssessmentModelPO persists draft assessment models for admin configuration.
type AssessmentModelPO struct {
	mongoBase.BaseDocument `bson:",inline"`

	Code                    string        `bson:"code"`
	ProductChannel          string        `bson:"product_channel,omitempty"`
	Kind                    string        `bson:"kind"`
	SubKind                 string        `bson:"sub_kind,omitempty"`
	Algorithm               string        `bson:"algorithm,omitempty"`
	Title                   string        `bson:"title"`
	Description             string        `bson:"description,omitempty"`
	Category                string        `bson:"category,omitempty"`
	Stages                  []string      `bson:"stages,omitempty"`
	ApplicableAges          []string      `bson:"applicable_ages,omitempty"`
	Reporters               []string      `bson:"reporters,omitempty"`
	Tags                    []string      `bson:"tags,omitempty"`
	Status                  string        `bson:"status"`
	QuestionnaireCode       string        `bson:"questionnaire_code,omitempty"`
	QuestionnaireVersion    string        `bson:"questionnaire_version,omitempty"`
	DefinitionPayloadFormat string        `bson:"definition_payload_format,omitempty"`
	DefinitionPayload       []byte        `bson:"definition_payload,omitempty"`
	DefinitionSchemaVersion string        `bson:"definition_schema_version,omitempty"`
	DefinitionV2            *DefinitionPO `bson:"definition_v2,omitempty"`
	Version                 int64         `bson:"version"`
	PublishedAt             *time.Time    `bson:"published_at,omitempty"`
	ArchivedAt              *time.Time    `bson:"archived_at,omitempty"`
}

func (AssessmentModelPO) CollectionName() string {
	return "assessment_models"
}

func (p *AssessmentModelPO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
	p.DeletedBy = 0
}

func (p *AssessmentModelPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
}

func (p *AssessmentModelPO) ToBsonM() (bson.M, error) {
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

func draftFilter(extra bson.M) bson.M {
	filter := bson.M{"deleted_at": nil}
	for key, value := range extra {
		filter[key] = value
	}
	return filter
}
