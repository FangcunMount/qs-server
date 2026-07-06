package modelcatalog

import (
	"context"
	"fmt"

	personalitymodel "github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
)

func (s *service) listPersonality(ctx context.Context, dto ListModelsDTO) ([]ModelSummary, int64, error) {
	seen := make(map[string]struct{})
	var items []ModelSummary
	var total int64

	if s.personality.cmd != nil && dto.Status != StatusPublished {
		result, err := s.personality.cmd.List(ctx, personalityListInput(dto))
		if err != nil {
			return nil, 0, err
		}
		if result != nil {
			total += result.Total
			for _, item := range summariesFromPersonalityList(result) {
				items = append(items, item)
				seen[item.Code] = struct{}{}
			}
		}
	}

	if dto.Status == "" || dto.Status == StatusPublished {
		if s.deps.PersonalityQuery != nil {
			result, err := s.deps.PersonalityQuery.ListPublished(ctx, personalitymodel.ListPersonalityModelsDTO{
				Page:     dto.Page,
				PageSize: dto.PageSize,
			})
			if err != nil {
				return nil, 0, err
			}
			for _, item := range result.Items {
				if dto.Algorithm != "" && item.Algorithm != dto.Algorithm {
					continue
				}
				if dto.SubKind != "" && dto.SubKind != SubKindTypology {
					continue
				}
				if dto.Keyword != "" && item.Title != "" && !containsFold(item.Title, dto.Keyword) {
					continue
				}
				if _, ok := seen[item.Code]; ok {
					continue
				}
				items = append(items, personalitySummaryFromSummary(item))
				total++
			}
		}
	}
	return items, total, nil
}

func (s *service) getPersonalityQRCode(ctx context.Context, modelCode string) (string, error) {
	if s.deps.RawQRCodeGenerator == nil {
		return fmt.Sprintf("/personality/assessment/%s", modelCode), nil
	}
	return s.deps.RawQRCodeGenerator.GeneratePersonalityAssessmentQRCode(ctx, modelCode)
}

func (s *service) questionnaireBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) (*QuestionnaireBindingResult, error) {
	result := &QuestionnaireBindingResult{
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
	}
	if questionnaireCode == "" || s.deps.QuestionnaireQuery == nil {
		return result, nil
	}
	var q *questionnaireapp.QuestionnaireResult
	var err error
	if questionnaireVersion != "" {
		q, err = s.deps.QuestionnaireQuery.GetPublishedByCodeVersion(ctx, questionnaireCode, questionnaireVersion)
	} else {
		q, err = s.deps.QuestionnaireQuery.GetByCode(ctx, questionnaireCode)
	}
	if err != nil {
		return result, nil
	}
	if q != nil {
		result.Title = q.Title
		result.QuestionCount = len(q.Questions)
	}
	return result, nil
}
