package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// managementService 答卷管理服务实现
// 行为者：管理员
type managementService struct {
	repo             answersheet.Repository
	reader           surveyreadmodel.AnswerSheetReader
	identityResolver iambridge.IdentityResolver
}

// NewManagementService 创建答卷管理服务
func NewManagementService(
	repo answersheet.Repository,
	reader surveyreadmodel.AnswerSheetReader,
	identityResolvers ...iambridge.IdentityResolver,
) AnswerSheetManagementService {
	var identityResolver iambridge.IdentityResolver
	if len(identityResolvers) > 0 {
		identityResolver = identityResolvers[0]
	}
	return &managementService{
		repo:             repo,
		reader:           reader,
		identityResolver: identityResolver,
	}
}

// GetByID 根据ID获取答卷详情
func (s *managementService) GetByID(ctx context.Context, id uint64) (*AnswerSheetResult, error) {
	// 1. 验证输入参数
	if id == 0 {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答卷ID不能为空")
	}
	sheetID, err := answerSheetIDFromUint64("answersheet_id", id)
	if err != nil {
		return nil, err
	}

	// 2. 获取答卷
	sheet, err := s.repo.FindByID(ctx, sheetID)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "获取答卷失败")
	}

	result := toAnswerSheetResult(sheet)
	s.resolveFillerName(ctx, result)
	return result, nil
}

func (s *managementService) resolveFillerName(ctx context.Context, result *AnswerSheetResult) {
	if result == nil || result.FillerID == 0 || s.identityResolver == nil || !s.identityResolver.IsEnabled() {
		return
	}

	id := meta.FromUint64(result.FillerID)
	if name := s.identityResolver.ResolveUserNames(ctx, []meta.ID{id})[id.String()]; name != "" {
		result.FillerName = name
	}
}

// GetByIDInOrg enforces tenant ownership for protected management reads.
// A mismatch is reported as not found so callers cannot probe another org's IDs.
func (s *managementService) GetByIDInOrg(ctx context.Context, orgID, id uint64) (*AnswerSheetResult, error) {
	if orgID == 0 {
		return nil, errors.WithCode(errorCode.ErrPermissionDenied, "机构范围不能为空")
	}
	result, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if result == nil || result.OrgID != orgID {
		return nil, errors.WithCode(errorCode.ErrAnswerSheetNotFound, "答卷不存在")
	}
	return result, nil
}

// List 查询答卷列表
func (s *managementService) List(ctx context.Context, dto ListAnswerSheetsDTO) (*AnswerSheetSummaryListResult, error) {
	if err := validateManagementListDTO(dto); err != nil {
		return nil, err
	}
	filter := buildListFilter(dto)

	// 3. 查询答卷摘要列表
	sheets, err := s.reader.ListAnswerSheets(ctx, filter, surveyreadmodel.PageRequest{Page: dto.Page, PageSize: dto.PageSize})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询答卷列表失败")
	}

	// 4. 获取总数
	total, err := s.reader.CountAnswerSheets(ctx, filter)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取答卷总数失败")
	}

	return toSummaryRowsResult(sheets, total), nil
}

func validateManagementListDTO(dto ListAnswerSheetsDTO) error {
	if dto.OrgID == 0 {
		return errors.WithCode(errorCode.ErrPermissionDenied, "机构范围不能为空")
	}
	if dto.Page <= 0 {
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量不能超过100")
	}
	return nil
}

func buildListFilter(dto ListAnswerSheetsDTO) surveyreadmodel.AnswerSheetFilter {
	return surveyreadmodel.AnswerSheetFilter{
		OrgID:             dto.OrgID,
		QuestionnaireCode: dto.QuestionnaireCode,
		FillerID:          dto.FillerID,
		StartTime:         dto.StartTime,
		EndTime:           dto.EndTime,
	}
}

// Delete 删除答卷
func (s *managementService) Delete(ctx context.Context, id uint64) error {
	// 1. 验证输入参数
	if id == 0 {
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "答卷ID不能为空")
	}
	sheetID, err := answerSheetIDFromUint64("answersheet_id", id)
	if err != nil {
		return err
	}

	// 2. 检查答卷是否存在
	_, err = s.repo.FindByID(ctx, sheetID)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	// 3. 删除答卷
	if err := s.repo.Delete(ctx, sheetID); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "删除答卷失败")
	}

	return nil
}
