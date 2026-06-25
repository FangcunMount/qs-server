package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/mbti"
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/sbti"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// RuleSetSBTICatalog 从统一规则目录解码 SBTI 模型。
type RuleSetSBTICatalog struct {
	reader rulesetport.PublishedRuleSetReader
}

func NewRuleSetSBTICatalog(reader rulesetport.PublishedRuleSetReader) RuleSetSBTICatalog {
	return RuleSetSBTICatalog{reader: reader}
}

func (c RuleSetSBTICatalog) GetSBTIModelByRef(ctx context.Context, ref port.ModelRef) (*rulesetsbti.ModelSnapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	snapshot, err := c.reader.GetPublishedByRef(ctx, rulesetport.RuleSetRef{
		Kind:    domain.RuleSetKindSBTI,
		Code:    ref.Code,
		Version: ref.Version,
	})
	if err != nil {
		return nil, err
	}
	return codec.DecodeSBTI(snapshot)
}

func (c RuleSetSBTICatalog) FindSBTIModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*rulesetsbti.ModelSnapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	snapshot, err := c.reader.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if snapshot.Definition.Kind != domain.RuleSetKindSBTI {
		return nil, domain.ErrNotFound
	}
	return codec.DecodeSBTI(snapshot)
}

// RuleSetMBTICatalog 从统一规则目录解码 MBTI 模型。
type RuleSetMBTICatalog struct {
	reader rulesetport.PublishedRuleSetReader
}

func NewRuleSetMBTICatalog(reader rulesetport.PublishedRuleSetReader) RuleSetMBTICatalog {
	return RuleSetMBTICatalog{reader: reader}
}

func (c RuleSetMBTICatalog) GetMBTIModelByRef(ctx context.Context, ref port.ModelRef) (*rulesetmbti.ModelSnapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	snapshot, err := c.reader.GetPublishedByRef(ctx, rulesetport.RuleSetRef{
		Kind:    domain.RuleSetKindMBTI,
		Code:    ref.Code,
		Version: ref.Version,
	})
	if err != nil {
		return nil, err
	}
	return codec.DecodeMBTI(snapshot)
}

func (c RuleSetMBTICatalog) FindMBTIModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*rulesetmbti.ModelSnapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	snapshot, err := c.reader.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if snapshot.Definition.Kind != domain.RuleSetKindMBTI {
		return nil, domain.ErrNotFound
	}
	return codec.DecodeMBTI(snapshot)
}
