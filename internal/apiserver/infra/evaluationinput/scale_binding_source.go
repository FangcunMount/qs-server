package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/authoring/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleset"
)

// RepositoryScaleBindingSource 从量表 command repo 提供 scale 规则绑定回退。
type RepositoryScaleBindingSource struct {
	repo ScaleSnapshotRepository
}

func NewRepositoryScaleBindingSource(repo ScaleSnapshotRepository) RepositoryScaleBindingSource {
	return RepositoryScaleBindingSource{repo: repo}
}

func (s RepositoryScaleBindingSource) FindScaleByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*port.ScaleSnapshot, error) {
	if s.repo == nil {
		return nil, scale.ErrNotFound
	}
	medicalScale, err := s.repo.FindByQuestionnaireRef(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return MedicalScaleToSnapshot(medicalScale), nil
}

func (s RepositoryScaleBindingSource) GetScaleByRef(ctx context.Context, code, version string) (*port.ScaleSnapshot, error) {
	catalog := NewRepositoryScaleSnapshotCatalog(s.repo)
	return catalog.GetScaleByRef(ctx, port.ModelRef{
		Kind:    port.EvaluationModelKindScale,
		Code:    code,
		Version: version,
	})
}

// RuleSetScaleCatalog 优先从统一规则目录解码 scale payload，未命中时回退量表 repo。
type RuleSetScaleCatalog struct {
	reader   rulesetport.PublishedRuleSetReader
	fallback port.ScaleModelCatalog
}

func NewRuleSetScaleCatalog(reader rulesetport.PublishedRuleSetReader, fallback port.ScaleModelCatalog) RuleSetScaleCatalog {
	return RuleSetScaleCatalog{reader: reader, fallback: fallback}
}

func (c RuleSetScaleCatalog) GetScale(ctx context.Context, code string) (*port.ScaleSnapshot, error) {
	if c.fallback == nil {
		return nil, port.NewResolveError(port.FailureKindScaleNotFound, fmt.Errorf("scale catalog is not configured"), "量表不存在", "加载量表失败")
	}
	return c.fallback.GetScale(ctx, code)
}

func (c RuleSetScaleCatalog) GetScaleByRef(ctx context.Context, ref port.ModelRef) (*port.ScaleSnapshot, error) {
	if ref.Version != "" && c.reader != nil {
		snapshot, err := c.reader.GetPublishedByRef(ctx, rulesetport.RuleSetRef{
			Kind:    domain.RuleSetKindScale,
			Code:    ref.Code,
			Version: ref.Version,
		})
		if err == nil {
			if snapshot.Definition.Kind != domain.RuleSetKindScale {
				return nil, domain.ErrNotFound
			}
			decoded, decodeErr := codec.DecodeScale(snapshot)
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
