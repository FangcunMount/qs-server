package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	aminfrac "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// RepositoryScaleBindingSource 从量表 command repo 提供 scale 规则绑定回退。
type RepositoryScaleBindingSource struct {
	repo ScaleSnapshotRepository
}

func NewRepositoryScaleBindingSource(repo ScaleSnapshotRepository) RepositoryScaleBindingSource {
	return RepositoryScaleBindingSource{repo: repo}
}

func (s RepositoryScaleBindingSource) FindScaleByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*scalesnapshot.ScaleSnapshot, error) {
	if s.repo == nil {
		return nil, scaledefinition.ErrNotFound
	}
	medicalScale, err := s.repo.FindByQuestionnaireRef(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return MedicalScaleToSnapshot(medicalScale), nil
}

func (s RepositoryScaleBindingSource) GetScaleByRef(ctx context.Context, code, version string) (*scalesnapshot.ScaleSnapshot, error) {
	catalog := NewRepositoryScaleSnapshotCatalog(s.repo)
	return catalog.GetScaleByRef(ctx, port.ModelRef{
		Kind:    port.EvaluationModelKindScale,
		Code:    code,
		Version: version,
	})
}

// PublishedScaleCatalog prefers published scale snapshots from the model catalog, then falls back to the scale repo.
type PublishedScaleCatalog struct {
	reader   rulesetport.PublishedModelReader
	fallback port.ScaleModelCatalog
}

func NewPublishedScaleCatalog(reader rulesetport.PublishedModelReader, fallback port.ScaleModelCatalog) PublishedScaleCatalog {
	return PublishedScaleCatalog{reader: reader, fallback: fallback}
}

func (c PublishedScaleCatalog) GetScale(ctx context.Context, code string) (*scalesnapshot.ScaleSnapshot, error) {
	if c.fallback == nil {
		return nil, port.NewResolveError(port.FailureKindScaleNotFound, fmt.Errorf("scale catalog is not configured"), "量表不存在", "加载量表失败")
	}
	return c.fallback.GetScale(ctx, code)
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
	if c.fallback == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, domain.ErrNotFound, "解释模型不存在", "加载解释模型失败")
	}
	return c.fallback.GetScaleByRef(ctx, ref)
}
