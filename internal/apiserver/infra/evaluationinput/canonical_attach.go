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
) error {
	if reader == nil || snapshot == nil || ref.Code == "" || ref.Version == "" {
		return nil
	}
	published, err := reader.GetPublishedModelByRef(ctx, ref)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil
		}
		return port.NewDependencyResolveError(
			port.DependencyCategoryModelCatalog,
			err,
			"加载解释模型依赖失败",
			"加载解释模型失败",
		)
	}
	if published == nil || published.DefinitionV2 == nil {
		return nil
	}
	port.AttachCanonicalDefinition(snapshot, published.DefinitionV2)
	return nil
}

func attachScaleCanonical(ctx context.Context, reader rulesetport.PublishedModelReader, ref port.InputRef, snapshot *port.InputSnapshot) error {
	return attachPublishedCanonical(ctx, reader, rulesetport.Ref{
		Kind: domain.KindScale, Code: ref.ModelRef.Code, Version: ref.ModelRef.Version,
	}, snapshot)
}

func attachTypologyCanonical(ctx context.Context, reader rulesetport.PublishedModelReader, ref port.InputRef, algorithm domain.Algorithm, snapshot *port.InputSnapshot) error {
	lookup := rulesetport.Ref{Kind: domain.KindTypology, SubKind: domain.SubKindTypology, Code: ref.ModelRef.Code, Version: ref.ModelRef.Version}
	if algorithm != "" {
		lookup.Algorithm = algorithm
	}
	return attachPublishedCanonical(ctx, reader, lookup, snapshot)
}

func attachBehavioralCanonical(ctx context.Context, reader rulesetport.PublishedModelReader, ref port.InputRef, snapshot *port.InputSnapshot) error {
	if reader == nil || snapshot == nil {
		return nil
	}
	lookups, err := behavioralRatingLookupRefs(ref.ModelRef)
	if err != nil {
		return port.NewResolveError(port.FailureKindUnsupportedModel, err, "不支持的解释模型", "加载解释模型失败")
	}
	for _, lookup := range lookups {
		published, err := reader.GetPublishedModelByRef(ctx, lookup)
		if err != nil {
			if domain.IsNotFound(err) {
				continue
			}
			return port.NewDependencyResolveError(
				port.DependencyCategoryModelCatalog,
				err,
				"加载解释模型依赖失败",
				"加载解释模型失败",
			)
		}
		if published == nil || published.DefinitionV2 == nil {
			continue
		}
		requested := domain.Algorithm(ref.ModelRef.Algorithm)
		if requested != "" && published.Algorithm != requested {
			continue
		}
		port.AttachCanonicalDefinition(snapshot, published.DefinitionV2)
		return nil
	}
	return nil
}

func attachCognitiveCanonical(ctx context.Context, reader rulesetport.PublishedModelReader, ref port.InputRef, snapshot *port.InputSnapshot) error {
	algorithm := domain.Algorithm(ref.ModelRef.Algorithm)
	if algorithm == "" {
		return nil
	}
	return attachPublishedCanonical(ctx, reader, rulesetport.Ref{
		Kind: domain.KindCognitive, Algorithm: algorithm, Code: ref.ModelRef.Code, Version: ref.ModelRef.Version,
	}, snapshot)
}
