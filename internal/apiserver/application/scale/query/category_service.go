package query

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
)

type categoryService struct{}

// NewCategoryService 创建量表分类选项服务。
func NewCategoryService() ports.ScaleCategoryService {
	return &categoryService{}
}

func (s *categoryService) GetCategories(_ context.Context) (*shared.ScaleCategoriesResult, error) {
	categories := []shared.CategoryOption{}
	for _, category := range scaledefinition.AllCategories {
		if !category.IsEmpty() {
			categories = append(categories, shared.CategoryOption{
				Value: category.String(),
				Label: category.Label(),
			})
		}
	}

	stages := []shared.StageOption{
		{Value: string(scaledefinition.StageDeepAssessment), Label: "深评"},
		{Value: string(scaledefinition.StageFollowUp), Label: "随访"},
		{Value: string(scaledefinition.StageOutcome), Label: "结局"},
	}

	applicableAges := []shared.ApplicableAgeOption{
		{Value: string(scaledefinition.ApplicableAgeInfant), Label: "婴幼儿（0-3岁）"},
		{Value: string(scaledefinition.ApplicableAgePreschool), Label: "学龄前（3-6岁）"},
		{Value: string(scaledefinition.ApplicableAgeSchoolChild), Label: "学龄儿童（6-12岁）"},
		{Value: string(scaledefinition.ApplicableAgeAdolescent), Label: "青少年（12-18岁）"},
		{Value: string(scaledefinition.ApplicableAgeAdult), Label: "成人（18岁以上）"},
	}

	reporters := []shared.ReporterOption{
		{Value: string(scaledefinition.ReporterParent), Label: "家长评"},
		{Value: string(scaledefinition.ReporterTeacher), Label: "教师评"},
		{Value: string(scaledefinition.ReporterSelf), Label: "自评"},
		{Value: string(scaledefinition.ReporterClinical), Label: "临床评定"},
	}

	return &shared.ScaleCategoriesResult{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           []shared.TagOption{},
	}, nil
}

func (s *categoryService) GetOpenCategories(_ context.Context) (*shared.ScaleCategoriesResult, error) {
	categories := []shared.CategoryOption{}
	for _, category := range scaledefinition.AllCategories {
		if category.IsOpen() {
			categories = append(categories, shared.CategoryOption{
				Value: category.String(),
				Label: category.Label(),
			})
		}
	}

	stages := []shared.StageOption{
		{Value: string(scaledefinition.StageDeepAssessment), Label: "深评"},
		{Value: string(scaledefinition.StageFollowUp), Label: "随访"},
		{Value: string(scaledefinition.StageOutcome), Label: "结局"},
	}

	applicableAges := []shared.ApplicableAgeOption{
		{Value: string(scaledefinition.ApplicableAgeInfant), Label: "婴幼儿（0-3岁）"},
		{Value: string(scaledefinition.ApplicableAgePreschool), Label: "学龄前（3-6岁）"},
		{Value: string(scaledefinition.ApplicableAgeSchoolChild), Label: "学龄儿童（6-12岁）"},
		{Value: string(scaledefinition.ApplicableAgeAdolescent), Label: "青少年（12-18岁）"},
		{Value: string(scaledefinition.ApplicableAgeAdult), Label: "成人（18岁以上）"},
	}

	reporters := []shared.ReporterOption{
		{Value: string(scaledefinition.ReporterParent), Label: "家长评"},
		{Value: string(scaledefinition.ReporterTeacher), Label: "教师评"},
		{Value: string(scaledefinition.ReporterSelf), Label: "自评"},
		{Value: string(scaledefinition.ReporterClinical), Label: "临床评定"},
	}

	return &shared.ScaleCategoriesResult{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           []shared.TagOption{},
	}, nil
}
