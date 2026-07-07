package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	aminfrac "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
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
	return aminfrac.DecodeBehavioralRatingFromPublished(snapshot)
}

func (c PublishedBehavioralRatingCatalog) FindBehavioralRatingByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*behavioralsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published behavioral_rating reader is not configured")
	}
	snapshot, err := c.reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return aminfrac.DecodeBehavioralRatingFromPublished(snapshot)
}

func behavioralRatingLookupRef(ref port.ModelRef) rulesetport.Ref {
	algorithm := domain.Algorithm(ref.Algorithm)
	if algorithm == "" {
		algorithm = domain.AlgorithmBrief2
	}
	return rulesetport.Ref{
		Kind:      domain.KindBehavioralRating,
		Algorithm: algorithm,
		Code:      ref.Code,
		Version:   ref.Version,
		Title:     ref.Title,
	}
}
