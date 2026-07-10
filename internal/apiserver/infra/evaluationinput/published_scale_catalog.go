package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	aminfrac "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// PublishedScaleCatalog reads published scale snapshots from the assessment model catalog.
type PublishedScaleCatalog struct {
	reader rulesetport.PublishedModelReader
}

func NewPublishedScaleCatalog(reader rulesetport.PublishedModelReader, fallback port.ScaleModelCatalog) PublishedScaleCatalog {
	return PublishedScaleCatalog{reader: reader}
}

func (c PublishedScaleCatalog) GetScale(ctx context.Context, code string) (*scalesnapshot.ScaleSnapshot, error) {
	if c.reader != nil {
		if lister, ok := c.reader.(rulesetport.PublishedModelLister); ok {
			snapshot, err := lister.FindPublishedModelByCode(ctx, domain.KindScale, code)
			if err == nil {
				decoded, decodeErr := aminfrac.DecodeScaleFromPublished(snapshot)
				if decodeErr != nil {
					return nil, port.NewResolveError(port.FailureKindScaleNotFound, decodeErr, "量表不存在", "加载量表失败")
				}
				return decoded, nil
			}
			if !domain.IsNotFound(err) {
				return nil, port.NewResolveError(port.FailureKindScaleNotFound, err, "量表不存在", "加载量表失败")
			}
		}
	}
	return nil, port.NewResolveError(port.FailureKindScaleNotFound, fmt.Errorf("published scale model is not found"), "量表不存在", "加载量表失败")
}

func (c PublishedScaleCatalog) GetScaleByRef(ctx context.Context, ref port.ModelRef) (*scalesnapshot.ScaleSnapshot, error) {
	if ref.Version != "" && c.reader != nil {
		snapshot, err := c.reader.GetPublishedModelByRef(ctx, rulesetport.Ref{
			Kind:    domain.KindScale,
			Code:    ref.Code,
			Version: ref.Version,
		})
		if err == nil {
			if snapshot.Kind != domain.KindScale {
				return nil, domain.ErrNotFound
			}
			decoded, decodeErr := aminfrac.DecodeScaleFromPublished(snapshot)
			if decodeErr != nil {
				return nil, port.NewResolveError(port.FailureKindModelNotFound, decodeErr, "解释模型不存在", "加载解释模型失败")
			}
			return decoded, nil
		}
		if !domain.IsNotFound(err) {
			return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "解释模型不存在", "加载解释模型失败")
		}
	}
	return nil, port.NewResolveError(port.FailureKindModelNotFound, domain.ErrNotFound, "解释模型不存在", "加载解释模型失败")
}
