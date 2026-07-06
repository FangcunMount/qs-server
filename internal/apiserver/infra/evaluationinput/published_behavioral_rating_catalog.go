package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedBehavioralRatingCatalog loads behavioral_rating payloads from v2 published-model snapshots.
type PublishedBehavioralRatingCatalog struct {
	reader rulesetport.PublishedModelReader
}

func NewPublishedBehavioralRatingCatalog(reader rulesetport.PublishedModelReader) PublishedBehavioralRatingCatalog {
	return PublishedBehavioralRatingCatalog{reader: reader}
}

func (c PublishedBehavioralRatingCatalog) GetBehavioralRatingByRef(ctx context.Context, ref port.ModelRef) (*behavioralsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published behavioral_rating reader is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	snapshot, err := c.reader.GetPublishedModelByRef(ctx, behavioralRatingLookupRef(ref))
	if err != nil {
		return nil, err
	}
	return decodePublishedBehavioralRatingModel(snapshot)
}

func (c PublishedBehavioralRatingCatalog) FindBehavioralRatingByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*behavioralsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published behavioral_rating reader is not configured")
	}
	snapshot, err := c.reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return decodePublishedBehavioralRatingModel(snapshot)
}

func behavioralRatingLookupRef(ref port.ModelRef) rulesetport.Ref {
	return rulesetport.Ref{
		Kind:      domain.KindBehavioralRating,
		Algorithm: domain.AlgorithmBehavioralRatingDefault,
		Code:      ref.Code,
		Version:   ref.Version,
		Title:     ref.Title,
	}
}

func decodePublishedBehavioralRatingModel(snapshot *domain.PublishedModelSnapshot) (*behavioralsnapshot.Snapshot, error) {
	if snapshot == nil {
		return nil, domain.ErrNotFound
	}
	if snapshot.Model.Kind != domain.KindBehavioralRating {
		return nil, fmt.Errorf("published model kind = %q, want behavioral_rating", snapshot.Model.Kind)
	}
	payload, err := behavioralsnapshot.ParseDefinitionPayload(
		snapshot.Model.Code,
		snapshot.Model.Version,
		snapshot.Model.Title,
		snapshot.Model.Status,
		snapshot.Payload,
	)
	if err != nil {
		return nil, err
	}
	payload.QuestionnaireCode = snapshot.Binding.QuestionnaireCode
	payload.QuestionnaireVersion = snapshot.Binding.QuestionnaireVersion
	if !payload.IsPublished() {
		return nil, fmt.Errorf("behavioral_rating model is not published: %s", payload.Code)
	}
	return payload, nil
}
