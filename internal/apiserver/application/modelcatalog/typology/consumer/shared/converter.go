package shared

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	identitypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func SummaryFromPublishedModel(model *port.PublishedModel) (TypologyModelSummaryResult, error) {
	payload, err := payloadFromPublishedModel(model)
	if err != nil {
		return TypologyModelSummaryResult{}, err
	}
	result := TypologyModelSummaryResult{
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		Status:               model.Status,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
	}
	applyPayloadSummary(&result, payload)
	applyPublishedModelRouting(&result, model)
	return result, nil
}

func DetailFromPublishedModel(model *port.PublishedModel) (*TypologyModelResult, error) {
	payload, err := payloadFromPublishedModel(model)
	if err != nil {
		return nil, err
	}
	summary, err := SummaryFromPublishedModel(model)
	if err != nil {
		return nil, err
	}
	dimensions, order := dimensionsFromPayload(payload)
	return &TypologyModelResult{
		TypologyModelSummaryResult: summary,
		DimensionOrder:             order,
		Dimensions:                 dimensions,
		Outcomes:                   outcomesFromPayload(payload),
	}, nil
}

func payloadFromPublishedModel(model *port.PublishedModel) (*modeltypology.Payload, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.PayloadFormat != domain.PayloadFormatPersonalityTypologyV1 {
		return nil, fmt.Errorf("unsupported typology payload format %q", model.PayloadFormat)
	}
	var payload modeltypology.Payload
	if err := json.Unmarshal(model.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func applyPayloadSummary(result *TypologyModelSummaryResult, payload *modeltypology.Payload) {
	if result == nil || payload == nil {
		return
	}
	if result.Title == "" {
		result.Title = payload.Title
	}
	if result.Version == "" {
		result.Version = payload.Version
	}
	if payload.Status != "" {
		result.Status = payload.Status
	}
	result.Algorithm = string(payload.Algorithm)
	result.QuestionnaireCode = payload.QuestionnaireCode
	result.QuestionnaireVersion = payload.QuestionnaireVersion
	result.QuestionCount = countPayloadQuestions(payload)
	if result.Status == "" {
		result.Status = "published"
	}
}

func dimensionsFromPayload(payload *modeltypology.Payload) ([]TypologyDimensionResult, []string) {
	if payload == nil {
		return nil, nil
	}
	order := append([]string(nil), payload.DimensionOrder...)
	dimensions := payload.Dimensions
	if payload.Runtime != nil {
		if len(order) == 0 {
			order = payload.Runtime.FactorGraph.DecisionFactorOrder()
		}
		if len(dimensions) == 0 {
			dimensions = payload.Runtime.FactorGraph.Dimensions
		}
	}
	items := make([]TypologyDimensionResult, 0, len(order))
	for _, code := range order {
		if dim, ok := dimensions[code]; ok {
			items = append(items, TypologyDimensionResult{
				Code:      dim.Code,
				Name:      dim.Name,
				LeftPole:  dim.LeftPole,
				RightPole: dim.RightPole,
			})
			continue
		}
		if payload.Runtime != nil {
			if factor, ok := payload.Runtime.FactorGraph.Factors[code]; ok {
				items = append(items, TypologyDimensionResult{
					Code: factor.Code,
					Name: factor.Name,
				})
			}
		}
	}
	return items, order
}

func outcomesFromPayload(payload *modeltypology.Payload) []TypologyOutcomeSummaryResult {
	if payload == nil {
		return nil
	}
	outcomes := make([]TypologyOutcomeSummaryResult, 0, len(payload.Outcomes))
	for _, outcome := range payload.Outcomes {
		imageURL := outcome.ImageURL
		if imageURL == "" {
			imageURL = outcome.Image
		}
		outcomes = append(outcomes, TypologyOutcomeSummaryResult{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			ImageURL: imageURL,
		})
	}
	return outcomes
}

func countPayloadQuestions(payload *modeltypology.Payload) int {
	if payload == nil {
		return 0
	}
	seen := make(map[string]struct{})
	for _, mapping := range payload.QuestionMappings {
		if mapping.QuestionCode != "" {
			seen[mapping.QuestionCode] = struct{}{}
		}
	}
	if payload.Runtime != nil {
		for _, factor := range payload.Runtime.FactorGraph.Factors {
			for _, contribution := range factor.Contributions {
				if contribution.QuestionCode != "" {
					seen[contribution.QuestionCode] = struct{}{}
				}
			}
		}
		for _, rule := range payload.Runtime.SpecialRules {
			for _, code := range rule.QuestionCodes {
				if code != "" {
					seen[code] = struct{}{}
				}
			}
			for _, code := range rule.Condition.QuestionCodes {
				if code != "" {
					seen[code] = struct{}{}
				}
			}
		}
	}
	return len(seen)
}

func applyPublishedModelRouting(result *TypologyModelSummaryResult, model *port.PublishedModel) {
	if result == nil || model == nil {
		return
	}
	result.Kind = string(model.Kind)
	result.SubKind = string(model.SubKind)
	result.ProductChannel = identitypkg.ProductChannelForIdentity(model.Kind, string(model.ProductChannel))
	result.PayloadFormat = model.PayloadFormat
	result.DecisionKind = string(model.DecisionKind)
	result.AlgorithmFamily = identitypkg.AlgorithmFamilyStringFromIdentity(
		model.Kind,
		model.SubKind,
		model.Algorithm,
	)
}
