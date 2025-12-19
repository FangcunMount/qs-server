package scale

import "context"

// ============= 按行为者组织的应用服务接口（Driving Ports）=============
//
// 设计原则：单一职责原则 (SRP)
// 每个服务只对一个行为者负责，避免不同行为者的需求变更影响同一个类
//
// 行为者识别：
// 1. 量表设计者/管理员 (Designer/Admin) - 负责量表创建、编辑、发布、归档
// 2. 量表因子编辑者 (Factor Editor) - 负责因子的增删改、解读规则配置
// 3. 通用查询服务 (Query Service) - 为所有行为者提供只读查询

// ScaleLifecycleService 量表生命周期服务
// 行为者：量表设计者/管理员 (Designer/Admin)
// 职责：量表创建、发布、下架、归档、基本信息维护
// 变更来源：管理员的业务流程需求变化
type ScaleLifecycleService interface {
	// Create 创建量表
	// 场景：管理员创建新量表，初始状态为草稿
	Create(ctx context.Context, dto CreateScaleDTO) (*ScaleResult, error)

	// UpdateBasicInfo 更新基本信息
	// 场景：管理员修改量表标题、描述
	UpdateBasicInfo(ctx context.Context, dto UpdateScaleBasicInfoDTO) (*ScaleResult, error)

	// UpdateQuestionnaire 更新关联的问卷
	// 场景：管理员更新量表关联的问卷及版本
	UpdateQuestionnaire(ctx context.Context, dto UpdateScaleQuestionnaireDTO) (*ScaleResult, error)

	// Publish 发布量表
	// 场景：管理员审核通过后发布量表，使其可用
	Publish(ctx context.Context, code string) (*ScaleResult, error)

	// Unpublish 下架量表
	// 场景：管理员主动下架量表，暂停使用
	Unpublish(ctx context.Context, code string) (*ScaleResult, error)

	// Archive 归档量表
	// 场景：管理员归档不再使用的量表，保留历史记录
	Archive(ctx context.Context, code string) (*ScaleResult, error)

	// Delete 删除量表
	// 场景：管理员彻底删除量表（只能删除草稿状态的量表）
	Delete(ctx context.Context, code string) error
}

// ScaleFactorService 量表因子编辑服务
// 行为者：量表因子编辑者 (Factor Editor)
// 职责：因子的批量管理、解读规则配置
// 变更来源：因子编辑的业务需求变化
// 设计说明：API 层仅提供批量操作接口，内部服务保留细粒度方法供内部调用
type ScaleFactorService interface {
	// AddFactor 添加因子（内部使用）
	// 场景：编辑者为量表添加新因子
	AddFactor(ctx context.Context, dto AddFactorDTO) (*ScaleResult, error)

	// UpdateFactor 更新因子（内部使用）
	// 场景：编辑者修改因子标题、关联题目、计分策略
	UpdateFactor(ctx context.Context, dto UpdateFactorDTO) (*ScaleResult, error)

	// RemoveFactor 删除因子（内部使用）
	// 场景：编辑者从量表中移除因子
	RemoveFactor(ctx context.Context, scaleCode, factorCode string) (*ScaleResult, error)

	// ReplaceFactors 批量替换所有因子
	// 场景：编辑者一次性替换量表的所有因子（批量导入）
	ReplaceFactors(ctx context.Context, scaleCode string, factors []FactorDTO) (*ScaleResult, error)

	// UpdateFactorInterpretRules 更新单个因子解读规则（内部使用）
	// 场景：编辑者为因子配置分数区间和解读文案
	UpdateFactorInterpretRules(ctx context.Context, dto UpdateFactorInterpretRulesDTO) (*ScaleResult, error)

	// ReplaceInterpretRules 批量设置所有因子的解读规则
	// 场景：编辑者一次性设置量表所有因子的解读规则
	ReplaceInterpretRules(ctx context.Context, scaleCode string, rules []UpdateFactorInterpretRulesDTO) (*ScaleResult, error)
}

// ScaleQueryService 量表查询服务
// 行为者：所有用户（管理员、编辑者、评估服务）
// 职责：提供只读查询功能
// 变更来源：查询需求变化
type ScaleQueryService interface {
	// GetByCode 根据编码获取量表
	// 场景：查询指定量表的完整信息
	GetByCode(ctx context.Context, code string) (*ScaleResult, error)

	// GetByQuestionnaireCode 根据问卷编码获取量表
	// 场景：查询关联到指定问卷的量表
	GetByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*ScaleResult, error)

	// List 查询量表摘要列表（轻量级，不包含因子详情）
	// 场景：分页查询量表列表，支持条件筛选
	List(ctx context.Context, dto ListScalesDTO) (*ScaleSummaryListResult, error)

	// GetPublishedByCode 获取已发布的量表
	// 场景：评估服务查询可用的量表（只返回已发布状态）
	GetPublishedByCode(ctx context.Context, code string) (*ScaleResult, error)

	// ListPublished 查询已发布量表摘要列表（轻量级，不包含因子详情）
	// 场景：浏览可用的量表列表
	ListPublished(ctx context.Context, dto ListScalesDTO) (*ScaleSummaryListResult, error)

	// GetFactors 获取量表的因子列表
	// 场景：查询指定量表的所有因子
	GetFactors(ctx context.Context, scaleCode string) ([]FactorResult, error)
}

// ScaleCategoryService 量表分类服务
// 行为者：所有用户（管理员、编辑者、前端）
// 职责：提供量表分类选项的统一数据源
// 变更来源：分类选项的定义变化
type ScaleCategoryService interface {
	// GetCategories 获取量表分类列表
	// 场景：前端需要渲染下拉框、多选框等组件时，获取所有可用的分类选项
	GetCategories(ctx context.Context) (*ScaleCategoriesResult, error)
}
