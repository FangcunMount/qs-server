package shared

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
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

func DetailFromSnapshot(snapshot *domain.Snapshot) (*PersonalityModelResult, error) {
	payload, err := modeltypology.DecodeFromSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	summary := SummaryFromSnapshot(snapshot, payload)
	dimensions := make([]PersonalityDimensionResult, 0, len(payload.DimensionOrder))
	for _, code := range payload.DimensionOrder {
		dim, ok := payload.Dimensions[code]
		if !ok {
			continue
		}
		dimensions = append(dimensions, PersonalityDimensionResult{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
		})
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
	return &PersonalityModelResult{
		PersonalityModelSummaryResult: summary,
		DimensionOrder:                append([]string(nil), payload.DimensionOrder...),
		Dimensions:                    dimensions,
		Outcomes:                      outcomes,
	}, nil
}
