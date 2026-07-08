package shared

import (
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func SummaryFromSnapshot(snapshot *domain.Snapshot, payload *modeltypology.Payload) PersonalityModelSummaryResult {
	result := PersonalityModelSummaryResult{
		Code:                 snapshot.Definition.Code,
		Version:              snapshot.Definition.Version,
		Title:                snapshot.Definition.Title,
		Status:               snapshot.Definition.Status,
		QuestionnaireCode:    snapshot.Binding.QuestionnaireCode,
		QuestionnaireVersion: snapshot.Binding.QuestionnaireVersion,
	}
	if payload != nil {
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
		result.QuestionCount = len(payload.QuestionMappings)
	}
	if result.Status == "" {
		result.Status = "published"
	}
	return result
}

func SummaryFromSnapshotOnly(snapshot *domain.Snapshot) (PersonalityModelSummaryResult, error) {
	payload, err := modeltypology.DecodeFromSnapshot(snapshot)
	if err != nil {
		return PersonalityModelSummaryResult{}, err
	}
	return SummaryFromSnapshot(snapshot, payload), nil
}

func SummaryFromPublishedModel(snapshot *domain.PublishedModelSnapshot) (PersonalityModelSummaryResult, error) {
	payload, err := payloadFromPublishedModel(snapshot)
	if err != nil {
		return PersonalityModelSummaryResult{}, err
	}
	result := PersonalityModelSummaryResult{
		Code:                 snapshot.Model.Code,
		Version:              snapshot.Model.Version,
		Title:                snapshot.Model.Title,
		Status:               snapshot.Model.Status,
		QuestionnaireCode:    snapshot.Binding.QuestionnaireCode,
		QuestionnaireVersion: snapshot.Binding.QuestionnaireVersion,
	}
	applyPayloadSummary(&result, payload)
	return result, nil
}

func DetailFromSnapshot(snapshot *domain.Snapshot) (*PersonalityModelResult, error) {
	payload, err := modeltypology.DecodeFromSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	summary := SummaryFromSnapshot(snapshot, payload)
	dimensions, order := dimensionsFromPayload(payload)
	outcomes := outcomesFromPayload(payload)
	return &PersonalityModelResult{
		PersonalityModelSummaryResult: summary,
		DimensionOrder:                order,
		Dimensions:                    dimensions,
		Outcomes:                      outcomes,
	}, nil
}

func DetailFromPublishedModel(snapshot *domain.PublishedModelSnapshot) (*PersonalityModelResult, error) {
	payload, err := payloadFromPublishedModel(snapshot)
	if err != nil {
		return nil, err
	}
	summary, err := SummaryFromPublishedModel(snapshot)
	if err != nil {
		return nil, err
	}
	dimensions, order := dimensionsFromPayload(payload)
	return &PersonalityModelResult{
		PersonalityModelSummaryResult: summary,
		DimensionOrder:                order,
		Dimensions:                    dimensions,
		Outcomes:                      outcomesFromPayload(payload),
	}, nil
}

func payloadFromPublishedModel(snapshot *domain.PublishedModelSnapshot) (*modeltypology.Payload, error) {
	if snapshot == nil {
		return nil, domain.ErrNotFound
	}
	if snapshot.PayloadFormat != domain.PayloadFormatPersonalityTypologyV1 {
		return modeltypology.DecodeFromSnapshot(domain.LegacyFromPublished(snapshot))
	}
	var payload modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func applyPayloadSummary(result *PersonalityModelSummaryResult, payload *modeltypology.Payload) {
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

func dimensionsFromPayload(payload *modeltypology.Payload) ([]PersonalityDimensionResult, []string) {
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
	items := make([]PersonalityDimensionResult, 0, len(order))
	for _, code := range order {
		if dim, ok := dimensions[code]; ok {
			items = append(items, PersonalityDimensionResult{
				Code:      dim.Code,
				Name:      dim.Name,
				LeftPole:  dim.LeftPole,
				RightPole: dim.RightPole,
			})
			continue
		}
		if payload.Runtime != nil {
			if factor, ok := payload.Runtime.FactorGraph.Factors[code]; ok {
				items = append(items, PersonalityDimensionResult{
					Code: factor.Code,
					Name: factor.Name,
				})
			}
		}
	}
	return items, order
}

func outcomesFromPayload(payload *modeltypology.Payload) []PersonalityOutcomeSummaryResult {
	if payload == nil {
		return nil
	}
	outcomes := make([]PersonalityOutcomeSummaryResult, 0, len(payload.Outcomes))
	for _, outcome := range payload.Outcomes {
		imageURL := outcome.ImageURL
		if imageURL == "" {
			imageURL = outcome.Image
		}
		outcomes = append(outcomes, PersonalityOutcomeSummaryResult{
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
