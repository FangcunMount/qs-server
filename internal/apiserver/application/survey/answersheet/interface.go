package answersheet

import "context"

// ============= 按行为者组织的应用服务接口（Driving Ports）=============
//
// 设计原则：单一职责原则 (SRP)
// 每个服务只对一个行为者负责，避免不同行为者的需求变更影响同一个类
//
// 行为者识别：
// 1. 答题者 (Testee/Filler) - C端用户，填写和提交答卷
// 2. 管理员 (Staff/Admin) - B端用户，查看、管理、统计答卷
// 3. 评分系统 (Scoring System) - 自动计算并保存答卷分数

// AnswerSheetSubmissionService 答卷提交服务
// 行为者：答题者 (Testee/Filler)
// 职责：答卷的提交、查看自己的答卷
// 变更来源：答题者的使用需求变化
type AnswerSheetSubmissionService interface {
	// Submit 提交答卷
	// 场景：答题者填写完问卷后提交答案
	Submit(ctx context.Context, dto SubmitAnswerSheetDTO) (*AnswerSheetResult, error)

	// GetMyAnswerSheet 获取我的答卷
	// 场景：答题者查看自己提交的答卷详情
	GetMyAnswerSheet(ctx context.Context, fillerID uint64, answerSheetID uint64) (*AnswerSheetResult, error)

	// ListMyAnswerSheets 查询我的答卷摘要列表
	// 场景：答题者查看自己提交的所有答卷（摘要信息，不含答案详情）
	ListMyAnswerSheets(ctx context.Context, dto ListMyAnswerSheetsDTO) (*AnswerSheetSummaryListResult, error)
}

// AnswerSheetManagementService 答卷管理服务
// 行为者：管理员 (Staff/Admin)
// 职责：答卷的查看、管理、删除
// 变更来源：管理后台的管理需求变化
type AnswerSheetManagementService interface {
	// GetByID 根据ID获取答卷详情
	// 场景：管理员查看答卷的完整信息
	GetByID(ctx context.Context, id uint64) (*AnswerSheetResult, error)

	// List 查询答卷摘要列表
	// 场景：管理员查询答卷列表（摘要信息，不含答案详情），支持按问卷、填写人、时间等条件筛选
	List(ctx context.Context, dto ListAnswerSheetsDTO) (*AnswerSheetSummaryListResult, error)

	// Delete 删除答卷
	// 场景：管理员删除无效或测试的答卷
	Delete(ctx context.Context, id uint64) error

	// GetStatistics 获取答卷统计
	// 场景：管理员查看某问卷的答卷统计数据（提交数、平均分等）
	GetStatistics(ctx context.Context, questionnaireCode string) (*AnswerSheetStatistics, error)
}
