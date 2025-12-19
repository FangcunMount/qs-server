package scale

import (
	"context"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

// ScaleCategoriesResult 量表分类结果
type ScaleCategoriesResult struct {
	Categories     []CategoryOption     `json:"categories"`
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
	categories := []CategoryOption{
		{Value: string(domainScale.CategoryADHD), Label: "ADHD"},
		{Value: string(domainScale.CategoryTicDisorder), Label: "抽动障碍"},
		{Value: string(domainScale.CategorySensoryIntegration), Label: "感统"},
		{Value: string(domainScale.CategoryExecutiveFunction), Label: "执行功能"},
		{Value: string(domainScale.CategoryMentalHealth), Label: "心理健康"},
		{Value: string(domainScale.CategoryNeurodevelopmentalScreening), Label: "神经发育"},
		{Value: string(domainScale.CategoryChronicDiseaseManagement), Label: "慢性病管理"},
		{Value: string(domainScale.CategoryQualityOfLife), Label: "生活质量"},
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
		{Value: string(domainScale.ApplicableAgeInfant), Label: "婴幼儿"},
		{Value: string(domainScale.ApplicableAgePreschool), Label: "学龄前"},
		{Value: string(domainScale.ApplicableAgeSchoolChild), Label: "学龄儿童"},
		{Value: string(domainScale.ApplicableAgeAdolescent), Label: "青少年"},
		{Value: string(domainScale.ApplicableAgeAdult), Label: "成人"},
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

