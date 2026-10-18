package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
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
	return attachCanonicalWithProjection(snapshot, published)
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
		return attachCanonicalWithProjection(snapshot, published)
	}
	return nil
}

func attachCanonicalWithProjection(snapshot *port.InputSnapshot, published *rulesetport.PublishedModel) error {
	assets, err := modeldefinition.ResolveReportAssetsProjection(published.DefinitionV2, published.ReportAssetsProjectionVersion)
	if err != nil {
		return port.NewResolveError(port.FailureKindUnsupportedModel,
			fmt.Errorf("model %s/%s projection version %d: %w", published.Code, published.Version, published.ReportAssetsProjectionVersion, err),
			"解释模型报告资产无效", "加载解释模型失败")
	}
	snapshot.DefinitionV2 = published.DefinitionV2
	// A previously frozen ReportInput owns its presentation assets. Fresh inputs
	// select the explicit pure projection, never an unversioned stored-first read.
	if (snapshot.InterpretationAssets == nil || !snapshot.InterpretationAssets.IsMaterialized()) && assets.IsMaterialized() {
		snapshot.InterpretationAssets = &assets
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
