package assessment

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// submissionService 测评提交服务实现
// 行为者：答题者 (Testee)
type submissionService struct {
	repo    assessment.Repository
	creator assessment.AssessmentCreator
}

// NewSubmissionService 创建测评提交服务
func NewSubmissionService(
	repo assessment.Repository,
	creator assessment.AssessmentCreator,
) AssessmentSubmissionService {
	return &submissionService{
		repo:    repo,
		creator: creator,
	}
}

// Create 创建测评
func (s *submissionService) Create(ctx context.Context, dto CreateAssessmentDTO) (*AssessmentResult, error) {
	// 1. 验证必要参数
	if dto.TesteeID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}
	if dto.QuestionnaireID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷ID不能为空")
	}
	if dto.AnswerSheetID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "答卷ID不能为空")
	}

	// 2. 构造创建请求
	req := s.buildCreateRequest(dto)

	// 3. 调用领域服务创建测评
	a, err := s.creator.Create(ctx, req)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentCreateFailed, "创建测评失败")
	}

	// 4. 持久化
	if err := s.repo.Save(ctx, a); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}

	return toAssessmentResult(a), nil
}

// Submit 提交测评
func (s *submissionService) Submit(ctx context.Context, assessmentID uint64) (*AssessmentResult, error) {
	// 1. 查询测评
	id := meta.FromUint64(assessmentID)
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 2. 提交测评
	if err := a.Submit(); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentSubmitFailed, "提交测评失败")
	}

	// 3. 持久化
	if err := s.repo.Save(ctx, a); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}

	// 4. 发布领域事件（由事件发布器处理）
	// 说明：领域事件已在 Submit() 内部添加到聚合根，
	// 由基础设施层的事件发布器在事务提交后发布

	return toAssessmentResult(a), nil
}

// GetMyAssessment 获取我的测评详情
func (s *submissionService) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentResult, error) {
	// 1. 查询测评
	id := meta.FromUint64(assessmentID)
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 2. 验证归属
	if a.TesteeID().Uint64() != testeeID {
		return nil, errors.WithCode(errorCode.ErrForbidden, "无权访问此测评")
	}

	return toAssessmentResult(a), nil
}

// ListMyAssessments 查询我的测评列表
func (s *submissionService) ListMyAssessments(ctx context.Context, dto ListMyAssessmentsDTO) (*AssessmentListResult, error) {
	// 1. 验证参数
	if dto.TesteeID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}

	// 2. 设置默认分页
	page, pageSize := normalizePagination(dto.Page, dto.PageSize)

	// 3. 构造查询参数
	testeeID := testee.NewID(dto.TesteeID)
	pagination := assessment.NewPagination(page, pageSize)

	// 4. 查询（暂不支持状态筛选，后续可扩展 Repository 接口）
	list, total, err := s.repo.FindByTesteeID(ctx, testeeID, pagination)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询测评列表失败")
	}

	// 5. 转换结果
	items := make([]*AssessmentResult, len(list))
	for i, a := range list {
		items[i] = toAssessmentResult(a)
	}

	totalInt := int(total)
	return &AssessmentListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}, nil
}

// buildCreateRequest 构造创建请求
func (s *submissionService) buildCreateRequest(dto CreateAssessmentDTO) assessment.CreateAssessmentRequest {
	req := assessment.CreateAssessmentRequest{
		OrgID:    int64(dto.OrgID),
		TesteeID: meta.FromUint64(dto.TesteeID),
		QuestionnaireRef: assessment.NewQuestionnaireRef(
			meta.FromUint64(dto.QuestionnaireID),
			meta.NewCode(dto.QuestionnaireCode),
			dto.QuestionnaireVersion,
		),
		AnswerSheetRef: assessment.NewAnswerSheetRef(
			meta.FromUint64(dto.AnswerSheetID),
		),
	}

	// 设置量表引用（可选）
	if dto.MedicalScaleID != nil {
		scaleCode := ""
		if dto.MedicalScaleCode != nil {
			scaleCode = *dto.MedicalScaleCode
		}
		scaleName := ""
		if dto.MedicalScaleName != nil {
			scaleName = *dto.MedicalScaleName
		}
		scaleRef := assessment.NewMedicalScaleRef(
			meta.FromUint64(*dto.MedicalScaleID),
			meta.NewCode(scaleCode),
			scaleName,
		)
		req.MedicalScaleRef = &scaleRef
	}

	// 设置来源
	switch dto.OriginType {
	case "plan":
		if dto.OriginID != nil {
			req.Origin = assessment.NewPlanOrigin(*dto.OriginID)
		}
	case "screening":
		if dto.OriginID != nil {
			req.Origin = assessment.NewScreeningOrigin(*dto.OriginID)
		}
	default:
		req.Origin = assessment.NewAdhocOrigin()
	}

	return req
}

// normalizePagination 规范化分页参数
func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
