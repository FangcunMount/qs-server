package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// FromMBTI 转换旧版 MBTI 载荷 到 unified 类型学 form。
func FromMBTI(model *MBTILegacyModel) *Payload {
	if model == nil {
		return nil
	}
	dimensions := make(map[string]Dimension, len(model.Dimensions))
	for code, dim := range model.Dimensions {
		dimensions[code] = Dimension{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
			Constant:  dim.Constant,
			Threshold: dim.Threshold,
		}
	}
	mappings := make([]QuestionMapping, 0, len(model.QuestionMappings))
	for _, mapping := range model.QuestionMappings {
		mappings = append(mappings, QuestionMapping{
			QuestionCode: mapping.QuestionCode,
			Dimension:    mapping.Dimension,
			Sign:         mapping.Sign,
		})
	}
	outcomes := make([]Outcome, 0, len(model.TypeProfiles))
	for _, profile := range model.TypeProfiles {
		outcomes = append(outcomes, Outcome{
			Code:        profile.TypeCode,
			Name:        profile.TypeName,
			OneLiner:    profile.OneLiner,
			Summary:     profile.Summary,
			Traits:      append([]string(nil), profile.Traits...),
			Strengths:   append([]string(nil), profile.Strengths...),
			Weaknesses:  append([]string(nil), profile.Weaknesses...),
			Suggestions: append([]string(nil), profile.Suggestions...),
			ImageURL:    profile.ImageURL,
		})
	}
	return &Payload{
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Status:               model.Status,
		Source: Source{
			QuestionsRepo: model.Source.QuestionsRepo,
			SourceSite:    model.Source.SourceSite,
			License:       model.Source.License,
			Attribution:   model.Source.Attribution,
			NonCommercial: model.Source.NonCommercial,
		},
		Algorithm:        binding.AlgorithmMBTI,
		DimensionOrder:   append([]string(nil), model.DimensionOrder...),
		Dimensions:       dimensions,
		QuestionMappings: mappings,
		Outcomes:         outcomes,
		MatchingSpec: MatchingSpec{
			Kind: binding.DecisionKindPoleComposition,
		},
	}
}

// ToMBTI 转换类型学载荷 back 到 旧版 MBTI form。
func ToMBTI(payload *Payload) (*MBTILegacyModel, error) {
	if payload == nil {
		return nil, fmt.Errorf("typology payload is nil")
	}
	if payload.Algorithm != binding.AlgorithmMBTI {
		return nil, fmt.Errorf("typology algorithm %s is not mbti", payload.Algorithm)
	}
	dimensions := make(map[string]MBTILegacyDimension, len(payload.Dimensions))
	for code, dim := range payload.Dimensions {
		dimensions[code] = MBTILegacyDimension{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
			Constant:  dim.Constant,
			Threshold: dim.Threshold,
		}
	}
	mappings := make([]MBTILegacyQuestionMapping, 0, len(payload.QuestionMappings))
	for _, mapping := range payload.QuestionMappings {
		mappings = append(mappings, MBTILegacyQuestionMapping{
			QuestionCode: mapping.QuestionCode,
			Dimension:    mapping.Dimension,
			Sign:         mapping.Sign,
		})
	}
	profiles := make([]MBTILegacyTypeProfile, 0, len(payload.Outcomes))
	for _, outcome := range payload.Outcomes {
		profiles = append(profiles, MBTILegacyTypeProfile{
			TypeCode:    outcome.Code,
			TypeName:    outcome.Name,
			OneLiner:    outcome.OneLiner,
			Summary:     outcome.Summary,
			Traits:      append([]string(nil), outcome.Traits...),
			Strengths:   append([]string(nil), outcome.Strengths...),
			Weaknesses:  append([]string(nil), outcome.Weaknesses...),
			Suggestions: append([]string(nil), outcome.Suggestions...),
			ImageURL:    outcome.ImageURL,
		})
	}
	return &MBTILegacyModel{
		Code:                 payload.Code,
		Version:              payload.Version,
		Title:                payload.Title,
		QuestionnaireCode:    payload.QuestionnaireCode,
		QuestionnaireVersion: payload.QuestionnaireVersion,
		Status:               payload.Status,
		Source: MBTILegacySource{
			QuestionsRepo: payload.Source.QuestionsRepo,
			SourceSite:    payload.Source.SourceSite,
			License:       payload.Source.License,
			Attribution:   payload.Source.Attribution,
			NonCommercial: payload.Source.NonCommercial,
		},
		DimensionOrder:   append([]string(nil), payload.DimensionOrder...),
		Dimensions:       dimensions,
		QuestionMappings: mappings,
		TypeProfiles:     profiles,
	}, nil
}
