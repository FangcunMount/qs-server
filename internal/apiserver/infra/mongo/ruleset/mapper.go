package ruleset

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	v1envelope "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/v1envelope"
	"go.mongodb.org/mongo-driver/bson"
)

type Mapper struct{}

func NewMapper() *Mapper {
	return &Mapper{}
}

func (Mapper) ToPO(snapshot *v1envelope.V1Snapshot) *EvaluationRuleSetPO {
	if snapshot == nil {
		return nil
	}
	status := snapshot.Definition.Status
	if status == "" {
		status = statusPublished
	}
	source := bson.M{}
	for key, value := range snapshot.Source {
		source[key] = value
	}
	return &EvaluationRuleSetPO{
		SchemaVersion:        snapshot.SchemaVersion,
		PayloadFormat:        snapshot.PayloadFormat,
		RuleSetKind:          string(snapshot.Definition.Kind),
		RuleSetCode:          snapshot.Definition.Code,
		RuleSetVersion:       snapshot.Definition.Version,
		Title:                snapshot.Definition.Title,
		Status:               status,
		DecisionKind:         string(snapshot.DecisionKind),
		QuestionnaireCode:    snapshot.Binding.QuestionnaireCode,
		QuestionnaireVersion: snapshot.Binding.QuestionnaireVersion,
		Source:               source,
		Payload:              append([]byte(nil), snapshot.Payload...),
	}
}

func (Mapper) ToDomain(po *EvaluationRuleSetPO) *v1envelope.V1Snapshot {
	if po == nil {
		return nil
	}
	source := make(map[string]any, len(po.Source))
	for key, value := range po.Source {
		source[key] = value
	}
	return &v1envelope.V1Snapshot{
		SchemaVersion: po.SchemaVersion,
		PayloadFormat: po.PayloadFormat,
		Definition: v1envelope.V1Definition{
			Kind:    v1envelope.RuleSetKind(po.RuleSetKind),
			Code:    po.RuleSetCode,
			Version: po.RuleSetVersion,
			Title:   po.Title,
			Status:  po.Status,
		},
		Binding: binding.QuestionnaireBinding{
			QuestionnaireCode:    po.QuestionnaireCode,
			QuestionnaireVersion: po.QuestionnaireVersion,
		},
		DecisionKind: binding.DecisionKind(po.DecisionKind),
		Source:       source,
		Payload:      append([]byte(nil), po.Payload...),
	}
}
