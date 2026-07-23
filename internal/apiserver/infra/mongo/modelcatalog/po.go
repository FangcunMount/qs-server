package modelcatalog

import (
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

const statusPublished = "published"

const (
	recordRoleHead              = "head"
	recordRolePublishedSnapshot = "published_snapshot"
)

// PublishedAssessmentModelPO is an immutable runtime snapshot stored beside the
// editable head in assessment_models. Field names intentionally distinguish a
// release version from the head revision during the storage migration.
type PublishedAssessmentModelPO struct {
	mongoBase.BaseDocument `bson:",inline"`

	SchemaVersion           string        `bson:"schema_version,omitempty"`
	RecordRole              string        `bson:"record_role"`
	ReleaseStatus           string        `bson:"release_status,omitempty"`
	Kind                    string        `bson:"kind"`
	Algorithm               string        `bson:"algorithm,omitempty"`
	Code                    string        `bson:"code"`
	ReleaseVersion          string        `bson:"release_version"`
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
	DefinitionSchemaVersion string        `bson:"definition_schema_version,omitempty"`
	DefinitionV2            *DefinitionPO `bson:"definition_v2,omitempty"`
	PublishedAt             *time.Time    `bson:"published_at,omitempty"`
	ReleaseArchivedAt       *time.Time    `bson:"release_archived_at,omitempty"`
}

func (PublishedAssessmentModelPO) CollectionName() string {
	return "assessment_models"
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
		"record_role": recordRolePublishedSnapshot,
		"status":      statusPublished,
		"deleted_at":  nil,
	}
	for key, value := range extra {
		filter[key] = value
	}
	return filter
}

func activePublishedFilter(extra bson.M) bson.M {
	filter := publishedFilter(extra)
	filter["release_status"] = string(domain.ReleaseStatusActive)
	return filter
}

// publishedModelUpsertFilter matches one immutable release. Repeating a publish
// for the same revision is idempotent; a later revision inserts a new snapshot.
func publishedModelUpsertFilter(po *PublishedAssessmentModelPO) bson.M {
	filter := bson.M{
		"record_role":     recordRolePublishedSnapshot,
		"kind":            po.Kind,
		"algorithm":       po.Algorithm,
		"code":            po.Code,
		"release_version": po.ReleaseVersion,
		"deleted_at":      nil,
	}
	return filter
}
