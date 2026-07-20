package evaluationinput

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func attachPublishedCanonical(
	ctx context.Context,
	reader rulesetport.PublishedModelReader,
	ref rulesetport.Ref,
	snapshot *port.InputSnapshot,
) {
	if reader == nil || snapshot == nil || ref.Code == "" || ref.Version == "" {
		return
	}
	published, err := reader.GetPublishedModelByRef(ctx, ref)
	if err != nil || published == nil || published.DefinitionV2 == nil {
		return
	}
	port.AttachCanonicalDefinition(snapshot, published.DefinitionV2)
}

func attachScaleCanonical(ctx context.Context, reader rulesetport.PublishedModelReader, ref port.InputRef, snapshot *port.InputSnapshot) {
	attachPublishedCanonical(ctx, reader, rulesetport.Ref{
		Kind: domain.KindScale, Code: ref.ModelRef.Code, Version: ref.ModelRef.Version,
	}, snapshot)
}

func attachTypologyCanonical(ctx context.Context, reader rulesetport.PublishedModelReader, ref port.InputRef, algorithm domain.Algorithm, snapshot *port.InputSnapshot) {
	lookup := rulesetport.Ref{Kind: domain.KindTypology, SubKind: domain.SubKindTypology, Code: ref.ModelRef.Code, Version: ref.ModelRef.Version}
	if algorithm != "" {
		lookup.Algorithm = algorithm
	}
	attachPublishedCanonical(ctx, reader, lookup, snapshot)
}

func attachBehavioralCanonical(ctx context.Context, reader rulesetport.PublishedModelReader, ref port.InputRef, snapshot *port.InputSnapshot) {
	if reader == nil || snapshot == nil {
		return
	}
	for _, lookup := range behavioralRatingLookupRefs(ref.ModelRef) {
		published, err := reader.GetPublishedModelByRef(ctx, lookup)
		if err != nil || published == nil || published.DefinitionV2 == nil {
			continue
		}
		requested := domain.Algorithm(ref.ModelRef.Algorithm)
		if requested != "" && !domain.BehavioralAlgorithmsEquivalent(published.Algorithm, requested) {
			continue
		}
		port.AttachCanonicalDefinition(snapshot, published.DefinitionV2)
		return
	}
}

func attachCognitiveCanonical(ctx context.Context, reader rulesetport.PublishedModelReader, ref port.InputRef, snapshot *port.InputSnapshot) {
	algorithm := domain.Algorithm(ref.ModelRef.Algorithm)
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
		domain.ObserveAlgorithmFallback(
			domain.KindCognitive, "", algorithm, "infra.cognitive_canonical_attach",
		)
	}
	attachPublishedCanonical(ctx, reader, rulesetport.Ref{
		Kind: domain.KindCognitive, Algorithm: algorithm, Code: ref.ModelRef.Code, Version: ref.ModelRef.Version,
	}, snapshot)
}
