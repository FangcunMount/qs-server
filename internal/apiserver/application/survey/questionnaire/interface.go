package questionnaire

import "context"

// ============= 按行为者组织的应用服务接口（Driving Ports）=============
//
// 设计原则：单一职责原则 (SRP)
// 每个服务只对一个行为者负责，避免不同行为者的需求变更影响同一个类
//
// 行为者识别：
// 1. 问卷设计者/管理员 (Designer/Admin) - 负责问卷创建、编辑、发布、归档
// 2. 问卷内容编辑者 (Content Editor) - 负责问题的增删改、排序、分组
// 3. 通用查询服务 (Query Service) - 为所有行为者提供只读查询

// QuestionnaireLifecycleService 问卷生命周期服务
// 行为者：问卷设计者/管理员 (Designer/Admin)
// 职责：问卷创建、发布、下架、归档、基本信息维护
// 变更来源：管理员的业务流程需求变化
type QuestionnaireLifecycleService interface {
	// Create 创建问卷
	// 场景：管理员创建新问卷，初始状态为草稿
	Create(ctx context.Context, dto CreateQuestionnaireDTO) (*QuestionnaireResult, error)

	// SaveDraft 保存草稿并更新版本
	// 场景：管理员编辑问卷后点击「存草稿」，小版本号递增但保持草稿状态
	// 注意：问题内容应先通过 ContentService.BatchUpdateQuestions 更新
	SaveDraft(ctx context.Context, code string) (*QuestionnaireResult, error)

	// UpdateBasicInfo 更新基本信息
	// 场景：管理员修改问卷标题、描述、封面图
	UpdateBasicInfo(ctx context.Context, dto UpdateQuestionnaireBasicInfoDTO) (*QuestionnaireResult, error)

	// Publish 发布问卷
	// 场景：管理员审核通过后发布问卷，使其可用
	Publish(ctx context.Context, code string) (*QuestionnaireResult, error)

	// Unpublish 下架问卷
	// 场景：管理员主动下架问卷，暂停使用
	Unpublish(ctx context.Context, code string) (*QuestionnaireResult, error)

	// Archive 归档问卷
	// 场景：管理员归档不再使用的问卷，保留历史记录
	Archive(ctx context.Context, code string) (*QuestionnaireResult, error)

	// Delete 删除问卷
	// 场景：管理员彻底删除问卷（只能删除草稿状态的问卷）
	Delete(ctx context.Context, code string) error
}

// QuestionnaireContentService 问卷内容编辑服务
// 行为者：问卷内容编辑者 (Content Editor)
// 职责：问题的增删改、排序、分组
// 变更来源：内容编辑的业务需求变化
type QuestionnaireContentService interface {
	// AddQuestion 添加问题
	// 场景：编辑者为问卷添加新问题
	AddQuestion(ctx context.Context, dto AddQuestionDTO) (*QuestionnaireResult, error)

	// UpdateQuestion 更新问题
	// 场景：编辑者修改问题内容、选项、配置
	UpdateQuestion(ctx context.Context, dto UpdateQuestionDTO) (*QuestionnaireResult, error)

	// RemoveQuestion 删除问题
	// 场景：编辑者从问卷中移除问题
	RemoveQuestion(ctx context.Context, questionnaireCode, questionCode string) (*QuestionnaireResult, error)

	// ReorderQuestions 重排问题顺序
	// 场景：编辑者调整问题的显示顺序
	ReorderQuestions(ctx context.Context, questionnaireCode string, orderedCodes []string) (*QuestionnaireResult, error)

	// BatchUpdateQuestions 批量更新问题
	// 场景：编辑者一次性更新多个问题（批量导入、批量编辑）
	BatchUpdateQuestions(ctx context.Context, questionnaireCode string, questions []QuestionDTO) (*QuestionnaireResult, error)
}

// QuestionnaireQueryService 问卷查询服务
// 行为者：所有用户（管理员、编辑者、答题者）
// 职责：提供只读查询功能
// 变更来源：查询需求变化
type QuestionnaireQueryService interface {
	// GetByCode 根据编码获取问卷
	// 场景：查询指定问卷的完整信息
	GetByCode(ctx context.Context, code string) (*QuestionnaireResult, error)

	// List 查询问卷列表
	// 场景：分页查询问卷列表，支持条件筛选
	List(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireListResult, error)

	// GetPublishedByCode 获取已发布的问卷
	// 场景：答题者查询可用的问卷（只返回已发布状态）
	GetPublishedByCode(ctx context.Context, code string) (*QuestionnaireResult, error)

	// ListPublished 查询已发布问卷列表
	// 场景：答题者浏览可答题的问卷列表
	ListPublished(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireListResult, error)
}
