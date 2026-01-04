package scale

import (
	"context"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

// ScaleCategoriesResult 量表分类结果
type ScaleCategoriesResult struct {
	Categories     []CategoryOption      `json:"categories"`
	Stages         []StageOption         `json:"stages"`
	ApplicableAges []ApplicableAgeOption `json:"applicable_ages"`
	Reporters      []ReporterOption      `json:"reporters"`
	Tags           []TagOption           `json:"tags"` // 标签已改为动态输入，返回空列表
}

// CategoryOption 类别选项
type CategoryOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// StageOption 阶段选项
type StageOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ApplicableAgeOption 使用年龄选项
type ApplicableAgeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ReporterOption 填报人选项
type ReporterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// TagOption 标签选项（已废弃，标签改为动态输入）
type TagOption struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

// categoryService 量表分类服务实现
type categoryService struct{}

// NewCategoryService 创建量表分类服务
func NewCategoryService() ScaleCategoryService {
	return &categoryService{}
}

// GetCategories 获取量表分类列表
func (s *categoryService) GetCategories(ctx context.Context) (*ScaleCategoriesResult, error) {
	// 构建类别列表
	// 根据 Category.isOpen() 判断
	categories := []CategoryOption{}
	for _, category := range domainScale.AllCategories {
		if !category.IsEmpty() {
			categories = append(categories, CategoryOption{
				Value: category.String(),
				Label: category.Label(),
			})
		}
	}

	// 构建阶段列表
	stages := []StageOption{
		{Value: string(domainScale.StageScreening), Label: "筛查"},
		{Value: string(domainScale.StageDeepAssessment), Label: "深评"},
		{Value: string(domainScale.StageFollowUp), Label: "随访"},
		{Value: string(domainScale.StageOutcome), Label: "结局"},
	}

	// 构建使用年龄列表
	applicableAges := []ApplicableAgeOption{
		{Value: string(domainScale.ApplicableAgeInfant), Label: "婴幼儿（0-3岁）"},
		{Value: string(domainScale.ApplicableAgePreschool), Label: "学龄前（3-6岁）"},
		{Value: string(domainScale.ApplicableAgeSchoolChild), Label: "学龄儿童（6-12岁）"},
		{Value: string(domainScale.ApplicableAgeAdolescent), Label: "青少年（12-18岁）"},
		{Value: string(domainScale.ApplicableAgeAdult), Label: "成人（18岁以上）"},
	}

	// 构建填报人列表
	reporters := []ReporterOption{
		{Value: string(domainScale.ReporterParent), Label: "家长评"},
		{Value: string(domainScale.ReporterTeacher), Label: "教师评"},
		{Value: string(domainScale.ReporterSelf), Label: "自评"},
		{Value: string(domainScale.ReporterClinical), Label: "临床评定"},
	}

	// 标签已改为动态输入，不再返回固定列表
	tags := []TagOption{}

	return &ScaleCategoriesResult{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}, nil
}

// GetOpenCategories 获取开放的量表分类列表
func (s *categoryService) GetOpenCategories(ctx context.Context) (*ScaleCategoriesResult, error) {
	// 构建类别列表
	// 根据 Category.isOpen() 判断
	categories := []CategoryOption{}
	for _, category := range domainScale.AllCategories {
		if category.IsOpen() {
			categories = append(categories, CategoryOption{
				Value: category.String(),
				Label: category.Label(),
			})
		}
	}

	// 构建阶段列表
	stages := []StageOption{
		{Value: string(domainScale.StageScreening), Label: "筛查"},
		{Value: string(domainScale.StageDeepAssessment), Label: "深评"},
		{Value: string(domainScale.StageFollowUp), Label: "随访"},
		{Value: string(domainScale.StageOutcome), Label: "结局"},
	}

	// 构建使用年龄列表
	applicableAges := []ApplicableAgeOption{
		{Value: string(domainScale.ApplicableAgeInfant), Label: "婴幼儿（0-3岁）"},
		{Value: string(domainScale.ApplicableAgePreschool), Label: "学龄前（3-6岁）"},
		{Value: string(domainScale.ApplicableAgeSchoolChild), Label: "学龄儿童（6-12岁）"},
		{Value: string(domainScale.ApplicableAgeAdolescent), Label: "青少年（12-18岁）"},
		{Value: string(domainScale.ApplicableAgeAdult), Label: "成人（18岁以上）"},
	}

	// 构建填报人列表
	reporters := []ReporterOption{
		{Value: string(domainScale.ReporterParent), Label: "家长评"},
		{Value: string(domainScale.ReporterTeacher), Label: "教师评"},
		{Value: string(domainScale.ReporterSelf), Label: "自评"},
		{Value: string(domainScale.ReporterClinical), Label: "临床评定"},
	}

	// 标签已改为动态输入，不再返回固定列表
	tags := []TagOption{}

	return &ScaleCategoriesResult{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}, nil
}
