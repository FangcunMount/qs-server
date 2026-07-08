package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance/snapshot"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedCognitiveCatalog loads cognitive payloads from v2 published-model snapshots.
type PublishedCognitiveCatalog struct {
	reader rulesetport.PublishedModelReader
}

func NewPublishedCognitiveCatalog(reader rulesetport.PublishedModelReader) PublishedCognitiveCatalog {
	return PublishedCognitiveCatalog{reader: reader}
}

func (c PublishedCognitiveCatalog) GetCognitiveByRef(ctx context.Context, ref port.ModelRef) (*taskperfsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published cognitive reader is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	snapshot, err := c.reader.GetPublishedModelByRef(ctx, cognitiveLookupRef(ref))
	if err != nil {
		return nil, err
	}
	return decodePublishedCognitiveModel(snapshot)
}

func (c PublishedCognitiveCatalog) FindCognitiveByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*taskperfsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published cognitive reader is not configured")
	}
	snapshot, err := c.reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return decodePublishedCognitiveModel(snapshot)
}

func cognitiveLookupRef(ref port.ModelRef) rulesetport.Ref {
	algorithm := domain.Algorithm(ref.Algorithm)
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	return rulesetport.Ref{
		Kind:      domain.KindCognitive,
		Algorithm: algorithm,
		Code:      ref.Code,
		Version:   ref.Version,
		Title:     ref.Title,
	}
}

func decodePublishedCognitiveModel(snapshot *domain.PublishedModelSnapshot) (*taskperfsnapshot.Snapshot, error) {
	if snapshot == nil {
		return nil, domain.ErrNotFound
	}
	if snapshot.Model.Kind != domain.KindCognitive {
		return nil, fmt.Errorf("published model kind = %q, want cognitive", snapshot.Model.Kind)
	}
	if !domain.IsCognitivePayloadFormat(snapshot.PayloadFormat) {
		return nil, fmt.Errorf("unsupported cognitive payload format: %s", snapshot.PayloadFormat)
	}
	payload, err := taskperfsnapshot.ParsePublishedPayload(
		snapshot.PayloadFormat,
		snapshot.Model.Code,
		snapshot.Model.Version,
		snapshot.Model.Title,
		snapshot.Model.Status,
		snapshot.Payload,
	)
	if err != nil {
		return nil, err
	}
	payload.QuestionnaireCode = snapshot.Binding.QuestionnaireCode
	payload.QuestionnaireVersion = snapshot.Binding.QuestionnaireVersion
	if !payload.IsPublished() {
		return nil, fmt.Errorf("cognitive model is not published: %s", payload.Code)
	}
	return payload, nil
}
