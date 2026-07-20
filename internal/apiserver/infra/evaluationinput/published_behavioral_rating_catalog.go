package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	aminfrac "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

// PublishedBehavioralRatingCatalog loads behavioral_rating payloads from v2 published-model snapshots.
type PublishedBehavioralRatingCatalog struct {
	reader rulesetport.PublishedModelReader
	norms  rulesetport.NormRepository
}

func NewPublishedBehavioralRatingCatalog(reader rulesetport.PublishedModelReader, norms ...rulesetport.NormRepository) PublishedBehavioralRatingCatalog {
	var normRepo rulesetport.NormRepository
	if len(norms) > 0 {
		normRepo = norms[0]
	}
	return PublishedBehavioralRatingCatalog{reader: reader, norms: normRepo}
}

func (c PublishedBehavioralRatingCatalog) GetBehavioralRatingByRef(ctx context.Context, ref port.ModelRef) (*behavioralsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published behavioral_rating reader is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	requested := domain.Algorithm(ref.Algorithm)
	for _, lookup := range behavioralRatingLookupRefs(ref) {
		snapshot, err := c.reader.GetPublishedModelByRef(ctx, lookup)
		if err != nil {
			if domain.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if requested != "" && snapshot != nil &&
			!domain.BehavioralAlgorithmsEquivalent(snapshot.Algorithm, requested) {
			continue
		}
		return c.decodePublished(ctx, snapshot)
	}
	return nil, domain.ErrNotFound
}

func (c PublishedBehavioralRatingCatalog) FindBehavioralRatingByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*behavioralsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published behavioral_rating reader is not configured")
	}
	snapshot, err := c.reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return c.decodePublished(ctx, snapshot)
}

func (c PublishedBehavioralRatingCatalog) decodePublished(ctx context.Context, snapshot *rulesetport.PublishedModel) (*behavioralsnapshot.Snapshot, error) {
	if snapshot == nil || snapshot.DefinitionV2 == nil {
		return nil, fmt.Errorf("behavioral_rating definition_v2 is required for runtime")
	}
	tables := make(map[string]*norm.Norm)
	for _, ref := range snapshot.DefinitionV2.Calibration.NormRefs {
		if ref.NormTableVersion == "" {
			continue
		}
		if _, ok := tables[ref.NormTableVersion]; ok {
			continue
		}
		if c.norms == nil {
			return nil, fmt.Errorf("behavioral_rating norm repository is not configured")
		}
		table, err := c.norms.FindNorm(ctx, ref.NormTableVersion)
		if err != nil {
			return nil, err
		}
		tables[ref.NormTableVersion] = table
	}
	return aminfrac.DecodeBehavioralRatingFromDefinition(snapshot, tables)
}

func behavioralRatingLookupRefs(ref port.ModelRef) []rulesetport.Ref {
	algorithm := domain.Algorithm(ref.Algorithm)
	if algorithm == "" {
		algorithm = domain.AlgorithmBrief2
		domain.ObserveAlgorithmFallback(
			domain.KindBehavioralRating, "", algorithm, "infra.behavioral_lookup_ref",
		)
	}
	algorithms := []domain.Algorithm{algorithm}
	algorithms = append(algorithms, domain.BehavioralAlgorithmLookupAlternates(algorithm)...)
	seen := make(map[domain.Algorithm]struct{}, len(algorithms))
	refs := make([]rulesetport.Ref, 0, len(algorithms))
	for _, alg := range algorithms {
		if _, ok := seen[alg]; ok {
			continue
		}
		seen[alg] = struct{}{}
		refs = append(refs, rulesetport.Ref{
			Kind:      domain.KindBehavioralRating,
			Algorithm: alg,
			Code:      ref.Code,
			Version:   ref.Version,
			Title:     ref.Title,
		})
	}
	return refs
}
