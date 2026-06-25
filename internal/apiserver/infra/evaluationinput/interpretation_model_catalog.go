package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/interpretationmodel/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	interpretationmodelport "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

// InterpretationSBTICatalog 从统一规则目录解码 SBTI 模型。
type InterpretationSBTICatalog struct {
	reader interpretationmodelport.PublishedModelReader
}

func NewInterpretationSBTICatalog(reader interpretationmodelport.PublishedModelReader) InterpretationSBTICatalog {
	return InterpretationSBTICatalog{reader: reader}
}

func (c InterpretationSBTICatalog) GetSBTIModelByRef(ctx context.Context, ref port.ModelRef) (*port.SBTIModelSnapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("interpretation model catalog is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	snapshot, err := c.reader.GetPublishedByRef(ctx, interpretationmodelport.ModelRef{
		Kind:    domain.ModelKindSBTI,
		Code:    ref.Code,
		Version: ref.Version,
	})
	if err != nil {
		return nil, err
	}
	return codec.DecodeSBTI(snapshot)
}

func (c InterpretationSBTICatalog) FindSBTIModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*port.SBTIModelSnapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("interpretation model catalog is not configured")
	}
	snapshot, err := c.reader.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if snapshot.Definition.Kind != domain.ModelKindSBTI {
		return nil, domain.ErrNotFound
	}
	return codec.DecodeSBTI(snapshot)
}

// InterpretationMBTICatalog 从统一规则目录解码 MBTI 模型。
type InterpretationMBTICatalog struct {
	reader interpretationmodelport.PublishedModelReader
}

func NewInterpretationMBTICatalog(reader interpretationmodelport.PublishedModelReader) InterpretationMBTICatalog {
	return InterpretationMBTICatalog{reader: reader}
}

func (c InterpretationMBTICatalog) GetMBTIModelByRef(ctx context.Context, ref port.ModelRef) (*port.MBTIModelSnapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("interpretation model catalog is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	snapshot, err := c.reader.GetPublishedByRef(ctx, interpretationmodelport.ModelRef{
		Kind:    domain.ModelKindMBTI,
		Code:    ref.Code,
		Version: ref.Version,
	})
	if err != nil {
		return nil, err
	}
	return codec.DecodeMBTI(snapshot)
}

func (c InterpretationMBTICatalog) FindMBTIModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*port.MBTIModelSnapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("interpretation model catalog is not configured")
	}
	snapshot, err := c.reader.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if snapshot.Definition.Kind != domain.ModelKindMBTI {
		return nil, domain.ErrNotFound
	}
	return codec.DecodeMBTI(snapshot)
}
