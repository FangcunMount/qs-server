package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/scale"
)

// ==================== Output Types ====================

// ScaleOutput 量表输出
type ScaleOutput struct {
	Code                 string
	Title                string
	Description          string
	Category             string
	Stages               []string
	ApplicableAges       []string
	Reporters            []string
	Tags                 []string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorOutput
	QuestionCount        int32
}

// FactorOutput 因子输出
type FactorOutput struct {
	Code            string
	Title           string
	FactorType      string
	IsTotalScore    bool
	QuestionCodes   []string
	ScoringStrategy string
	ScoringParams   map[string]string
	RiskLevel       string
	InterpretRules  []InterpretRuleOutput
}

// InterpretRuleOutput 解读规则输出
type InterpretRuleOutput struct {
	MinScore   float64
	MaxScore   float64
	RiskLevel  string
	Conclusion string
	Suggestion string
}

// ScaleSummaryOutput 量表摘要输出（不含因子详情）
type ScaleSummaryOutput struct {
	Code                 string
	Title                string
	Description          string
	Category             string
	Stages               []string
	ApplicableAges       []string
	Reporters            []string
	Tags                 []string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	QuestionCount        int32
}

// ListScalesOutput 量表列表输出
type ListScalesOutput struct {
	Scales   []ScaleSummaryOutput
	Total    int64
	Page     int32
	PageSize int32
}

// ScaleCategoriesOutput 量表分类输出
type ScaleCategoriesOutput struct {
	Categories     []CategoryOutput
	Stages         []StageOutput
	ApplicableAges []ApplicableAgeOutput
	Reporters      []ReporterOutput
	Tags           []TagOutput
}

// CategoryOutput 类别输出
type CategoryOutput struct {
	Value string
	Label string
}

// StageOutput 阶段输出
type StageOutput struct {
	Value string
	Label string
}

// ApplicableAgeOutput 使用年龄输出
type ApplicableAgeOutput struct {
	Value string
	Label string
}

// ReporterOutput 填报人输出
type ReporterOutput struct {
	Value string
	Label string
}

// TagOutput 标签输出
type TagOutput struct {
	Value    string
	Label    string
	Category string
}

// ==================== Client ====================

// ScaleClient 量表服务 gRPC 客户端封装
type ScaleClient struct {
	client     *Client
	grpcClient pb.ScaleServiceClient
}

// NewScaleClient 创建量表服务客户端
func NewScaleClient(client *Client) *ScaleClient {
	return &ScaleClient{
		client:     client,
		grpcClient: pb.NewScaleServiceClient(client.Conn()),
	}
}

// GetScale 获取量表详情
func (c *ScaleClient) GetScale(ctx context.Context, code string) (*ScaleOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetScaleRequest{Code: code}
	resp, err := c.grpcClient.GetScale(ctx, req)
	if err != nil {
		return nil, err
	}

	scale := resp.GetScale()
	if scale == nil {
		return nil, nil
	}

	return c.convertScale(scale), nil
}

// ListScales 获取量表列表（摘要）
func (c *ScaleClient) ListScales(ctx context.Context, page, pageSize int32, status, title, category string, stages, applicableAges, reporters, tags []string) (*ListScalesOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.ListScalesRequest{
		Page:           page,
		PageSize:       pageSize,
		Status:         status,
		Title:          title,
		Category:       category,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}

	resp, err := c.grpcClient.ListScales(ctx, req)
	if err != nil {
		return nil, err
	}

	// 转换摘要列表
	scales := make([]ScaleSummaryOutput, len(resp.GetScales()))
	for i, s := range resp.GetScales() {
		scales[i] = ScaleSummaryOutput{
			Code:                 s.GetCode(),
			Title:                s.GetTitle(),
			Description:          s.GetDescription(),
			Category:             s.GetCategory(),
			Stages:               s.GetStages(),
			ApplicableAges:       s.GetApplicableAges(),
			Reporters:            s.GetReporters(),
			Tags:                 s.GetTags(),
			QuestionnaireCode:    s.GetQuestionnaireCode(),
			QuestionnaireVersion: s.GetQuestionnaireVersion(),
			Status:               s.GetStatus(),
			QuestionCount:        s.GetQuestionCount(),
		}
	}

	return &ListScalesOutput{
		Scales:   scales,
		Total:    resp.GetTotal(),
		Page:     resp.GetPage(),
		PageSize: resp.GetPageSize(),
	}, nil
}

// GetScaleCategories 获取量表分类列表
func (c *ScaleClient) GetScaleCategories(ctx context.Context) (*ScaleCategoriesOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetScaleCategoriesRequest{}
	resp, err := c.grpcClient.GetScaleCategories(ctx, req)
	if err != nil {
		return nil, err
	}

	// 转换分类列表
	categories := make([]CategoryOutput, len(resp.GetCategories()))
	for i, cat := range resp.GetCategories() {
		categories[i] = CategoryOutput{
			Value: cat.GetValue(),
			Label: cat.GetLabel(),
		}
	}

	stages := make([]StageOutput, len(resp.GetStages()))
	for i, stage := range resp.GetStages() {
		stages[i] = StageOutput{
			Value: stage.GetValue(),
			Label: stage.GetLabel(),
		}
	}

	applicableAges := make([]ApplicableAgeOutput, len(resp.GetApplicableAges()))
	for i, age := range resp.GetApplicableAges() {
		applicableAges[i] = ApplicableAgeOutput{
			Value: age.GetValue(),
			Label: age.GetLabel(),
		}
	}

	reporters := make([]ReporterOutput, len(resp.GetReporters()))
	for i, rep := range resp.GetReporters() {
		reporters[i] = ReporterOutput{
			Value: rep.GetValue(),
			Label: rep.GetLabel(),
		}
	}

	tags := make([]TagOutput, len(resp.GetTags()))
	for i, tag := range resp.GetTags() {
		tags[i] = TagOutput{
			Value:    tag.GetValue(),
			Label:    tag.GetLabel(),
			Category: tag.GetCategory(),
		}
	}

	return &ScaleCategoriesOutput{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}, nil
}

// convertScale 转换 protobuf 量表到输出类型
func (c *ScaleClient) convertScale(s *pb.Scale) *ScaleOutput {
	// 转换因子列表
	factors := make([]FactorOutput, len(s.GetFactors()))
	for i, f := range s.GetFactors() {
		factors[i] = c.convertFactor(f)
	}

	return &ScaleOutput{
		Code:                 s.GetCode(),
		Title:                s.GetTitle(),
		Description:          s.GetDescription(),
		Category:             s.GetCategory(),
		Stages:               s.GetStages(),
		ApplicableAges:       s.GetApplicableAges(),
		Reporters:            s.GetReporters(),
		Tags:                 s.GetTags(),
		QuestionnaireCode:    s.GetQuestionnaireCode(),
		QuestionnaireVersion: s.GetQuestionnaireVersion(),
		Status:               s.GetStatus(),
		Factors:              factors,
		QuestionCount:        s.GetQuestionCount(),
	}
}

// convertFactor 转换 protobuf 因子到输出类型
func (c *ScaleClient) convertFactor(f *pb.Factor) FactorOutput {
	// 转换解读规则
	rules := make([]InterpretRuleOutput, len(f.GetInterpretRules()))
	for i, rule := range f.GetInterpretRules() {
		rules[i] = InterpretRuleOutput{
			MinScore:   rule.GetMinScore(),
			MaxScore:   rule.GetMaxScore(),
			RiskLevel:  rule.GetRiskLevel(),
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		}
	}

	return FactorOutput{
		Code:            f.GetCode(),
		Title:           f.GetTitle(),
		FactorType:      f.GetFactorType(),
		IsTotalScore:    f.GetIsTotalScore(),
		QuestionCodes:   f.GetQuestionCodes(),
		ScoringStrategy: f.GetScoringStrategy(),
		ScoringParams:   f.GetScoringParams(),
		RiskLevel:       f.GetRiskLevel(),
		InterpretRules:  rules,
	}
}
