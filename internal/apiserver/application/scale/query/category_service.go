package query

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

type categoryService struct{}

// NewCategoryService 创建量表分类选项服务。
func NewCategoryService() ports.ScaleCategoryService {
	return &categoryService{}
}

func (s *categoryService) GetCategories(_ context.Context) (*shared.ScaleCategoriesResult, error) {
	categories := []shared.CategoryOption{}
	for _, category := range domainScale.AllCategories {
		if !category.IsEmpty() {
			categories = append(categories, shared.CategoryOption{
				Value: category.String(),
				Label: category.Label(),
			})
		}
	}

	stages := []shared.StageOption{
		{Value: string(domainScale.StageDeepAssessment), Label: "深评"},
		{Value: string(domainScale.StageFollowUp), Label: "随访"},
		{Value: string(domainScale.StageOutcome), Label: "结局"},
	}

	applicableAges := []shared.ApplicableAgeOption{
		{Value: string(domainScale.ApplicableAgeInfant), Label: "婴幼儿（0-3岁）"},
		{Value: string(domainScale.ApplicableAgePreschool), Label: "学龄前（3-6岁）"},
		{Value: string(domainScale.ApplicableAgeSchoolChild), Label: "学龄儿童（6-12岁）"},
		{Value: string(domainScale.ApplicableAgeAdolescent), Label: "青少年（12-18岁）"},
		{Value: string(domainScale.ApplicableAgeAdult), Label: "成人（18岁以上）"},
	}

	reporters := []shared.ReporterOption{
		{Value: string(domainScale.ReporterParent), Label: "家长评"},
		{Value: string(domainScale.ReporterTeacher), Label: "教师评"},
		{Value: string(domainScale.ReporterSelf), Label: "自评"},
		{Value: string(domainScale.ReporterClinical), Label: "临床评定"},
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
	for _, category := range domainScale.AllCategories {
		if category.IsOpen() {
			categories = append(categories, shared.CategoryOption{
				Value: category.String(),
				Label: category.Label(),
			})
		}
	}

	stages := []shared.StageOption{
		{Value: string(domainScale.StageDeepAssessment), Label: "深评"},
		{Value: string(domainScale.StageFollowUp), Label: "随访"},
		{Value: string(domainScale.StageOutcome), Label: "结局"},
	}

	applicableAges := []shared.ApplicableAgeOption{
		{Value: string(domainScale.ApplicableAgeInfant), Label: "婴幼儿（0-3岁）"},
		{Value: string(domainScale.ApplicableAgePreschool), Label: "学龄前（3-6岁）"},
		{Value: string(domainScale.ApplicableAgeSchoolChild), Label: "学龄儿童（6-12岁）"},
		{Value: string(domainScale.ApplicableAgeAdolescent), Label: "青少年（12-18岁）"},
		{Value: string(domainScale.ApplicableAgeAdult), Label: "成人（18岁以上）"},
	}

	reporters := []shared.ReporterOption{
		{Value: string(domainScale.ReporterParent), Label: "家长评"},
		{Value: string(domainScale.ReporterTeacher), Label: "教师评"},
		{Value: string(domainScale.ReporterSelf), Label: "自评"},
		{Value: string(domainScale.ReporterClinical), Label: "临床评定"},
	}

	return &shared.ScaleCategoriesResult{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           []shared.TagOption{},
	}, nil
}
