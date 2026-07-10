package query

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
)

type categoryService struct{}

// NewCategoryService 创建量表分类选项服务。
func NewCategoryService() ports.ScaleCategoryService {
	return &categoryService{}
}

func (s *categoryService) GetCategories(_ context.Context) (*shared.ScaleCategoriesResult, error) {
	categories := append([]shared.CategoryOption(nil), allScaleCategoryOptions()...)

	return &shared.ScaleCategoriesResult{
		Categories:     categories,
		Stages:         scaleStageOptions(),
		ApplicableAges: scaleApplicableAgeOptions(),
		Reporters:      scaleReporterOptions(),
		Tags:           []shared.TagOption{},
	}, nil
}

func (s *categoryService) GetOpenCategories(_ context.Context) (*shared.ScaleCategoriesResult, error) {
	categories := []shared.CategoryOption{}
	for _, category := range allScaleCategoryOptions() {
		if isOpenScaleCategory(category.Value) {
			categories = append(categories, category)
		}
	}

	return &shared.ScaleCategoriesResult{
		Categories:     categories,
		Stages:         scaleStageOptions(),
		ApplicableAges: scaleApplicableAgeOptions(),
		Reporters:      scaleReporterOptions(),
		Tags:           []shared.TagOption{},
	}, nil
}

func allScaleCategoryOptions() []shared.CategoryOption {
	return []shared.CategoryOption{
		{Value: "adhd", Label: "多动"},
		{Value: "td", Label: "抽动"},
		{Value: "asd", Label: "自闭"},
		{Value: "pressure", Label: "压力"},
		{Value: "sii", Label: "感觉统合"},
		{Value: "efn", Label: "执行功能"},
		{Value: "emt", Label: "情绪"},
		{Value: "slp", Label: "睡眠"},
		{Value: "personality", Label: "人格"},
	}
}

func isOpenScaleCategory(value string) bool {
	switch value {
	case "adhd", "td", "asd", "pressure", "sii", "efn", "emt", "slp":
		return true
	default:
		return false
	}
}

func scaleStageOptions() []shared.StageOption {
	return []shared.StageOption{
		{Value: "deep_assessment", Label: "深评"},
		{Value: "follow_up", Label: "随访"},
		{Value: "outcome", Label: "结局"},
	}
}

func scaleApplicableAgeOptions() []shared.ApplicableAgeOption {
	return []shared.ApplicableAgeOption{
		{Value: "infant", Label: "婴幼儿（0-3岁）"},
		{Value: "preschool", Label: "学龄前（3-6岁）"},
		{Value: "school_child", Label: "学龄儿童（6-12岁）"},
		{Value: "adolescent", Label: "青少年（12-18岁）"},
		{Value: "adult", Label: "成人（18岁以上）"},
	}
}

func scaleReporterOptions() []shared.ReporterOption {
	return []shared.ReporterOption{
		{Value: "parent", Label: "家长评"},
		{Value: "teacher", Label: "教师评"},
		{Value: "self", Label: "自评"},
		{Value: "clinical", Label: "临床评定"},
	}
}
