package ruleset

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"go.mongodb.org/mongo-driver/bson"
)

type Mapper struct{}

func NewMapper() *Mapper {
	return &Mapper{}
}

func (Mapper) ToPO(snapshot *domain.RuleSetSnapshot) *EvaluationRuleSetPO {
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

func (Mapper) ToDomain(po *EvaluationRuleSetPO) *domain.RuleSetSnapshot {
	if po == nil {
		return nil
	}
	source := make(map[string]any, len(po.Source))
	for key, value := range po.Source {
		source[key] = value
	}
	return &domain.RuleSetSnapshot{
		SchemaVersion: po.SchemaVersion,
		PayloadFormat: po.PayloadFormat,
		Definition: domain.RuleSetDefinition{
			Kind:    domain.RuleSetKind(po.RuleSetKind),
			Code:    po.RuleSetCode,
			Version: po.RuleSetVersion,
			Title:   po.Title,
			Status:  po.Status,
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    po.QuestionnaireCode,
			QuestionnaireVersion: po.QuestionnaireVersion,
		},
		DecisionKind: domain.DecisionKind(po.DecisionKind),
		Source:       source,
		Payload:      append([]byte(nil), po.Payload...),
	}
}
