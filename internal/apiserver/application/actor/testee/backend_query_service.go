package testee

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
)

// backendQueryService 受试者后台查询服务实现
// 行为者：B端员工(Staff) - 后台管理系统
// 职责：提供受试者详细信息查询能力（包含家长信息等后台管理所需数据）
type backendQueryService struct {
	queryService    TesteeQueryService       // 复用基础查询服务
	guardianshipSvc *iam.GuardianshipService // IAM 监护关系服务
}

// NewBackendQueryService 创建受试者后台查询服务
func NewBackendQueryService(
	queryService TesteeQueryService,
	guardianshipSvc *iam.GuardianshipService,
) TesteeBackendQueryService {
	return &backendQueryService{
		queryService:    queryService,
		guardianshipSvc: guardianshipSvc,
	}
}

// GetByIDWithGuardians 根据ID查询受试者详情（包含家长信息）
func (s *backendQueryService) GetByIDWithGuardians(ctx context.Context, testeeID uint64) (*TesteeBackendResult, error) {
	// 1. 使用基础查询服务获取受试者信息
	testeeResult, err := s.queryService.GetByID(ctx, testeeID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get testee")
	}

	// 2. 构建后台结果（嵌入基础结果）
	backendResult := &TesteeBackendResult{
		TesteeResult: testeeResult,
		Guardians:    []GuardianInfo{},
	}

	// 3. 如果受试者有 profileID 且监护关系服务可用，则获取家长信息
	if testeeResult.ProfileID == nil {
		logger.L(ctx).Debugw("Testee has no profileID, skipping guardian fetch",
			"action", "get_testee_with_guardians",
			"testee_id", testeeID,
		)
	} else if s.guardianshipSvc == nil {
		logger.L(ctx).Debugw("Guardianship service is nil, skipping guardian fetch",
			"action", "get_testee_with_guardians",
			"testee_id", testeeID,
			"profile_id", *testeeResult.ProfileID,
		)
	} else if !s.guardianshipSvc.IsEnabled() {
		logger.L(ctx).Debugw("Guardianship service is not enabled, skipping guardian fetch",
			"action", "get_testee_with_guardians",
			"testee_id", testeeID,
			"profile_id", *testeeResult.ProfileID,
		)
	} else {
		// 所有条件满足，获取家长信息
		logger.L(ctx).Debugw("Fetching guardians from IAM",
			"action", "get_testee_with_guardians",
			"testee_id", testeeID,
			"profile_id", *testeeResult.ProfileID,
		)
		guardians, err := s.fetchGuardians(ctx, *testeeResult.ProfileID)
		if err != nil {
			// 家长信息获取失败不影响主流程，记录日志即可
			logger.L(ctx).Warnw("Failed to fetch guardians for testee",
				"action", "get_testee_with_guardians",
				"testee_id", testeeID,
				"profile_id", *testeeResult.ProfileID,
				"error", err.Error(),
			)
		} else {
			logger.L(ctx).Debugw("Successfully fetched guardians",
				"action", "get_testee_with_guardians",
				"testee_id", testeeID,
				"profile_id", *testeeResult.ProfileID,
				"guardians_count", len(guardians),
			)
			backendResult.Guardians = guardians
		}
	}

	return backendResult, nil
}

// ListTesteesWithGuardians 列出受试者（包含家长信息）
func (s *backendQueryService) ListTesteesWithGuardians(ctx context.Context, dto ListTesteeDTO) (*TesteeBackendListResult, error) {
	// 1. 使用基础查询服务获取受试者列表
	listResult, err := s.queryService.ListTestees(ctx, dto)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list testees")
	}

	// 2. 转换为后台结果列表
	backendItems := make([]*TesteeBackendResult, len(listResult.Items))
	for i, testeeResult := range listResult.Items {
		backendItem := &TesteeBackendResult{
			TesteeResult: testeeResult,
			Guardians:    []GuardianInfo{},
		}

		// 3. 为每个受试者获取家长信息（如果可用）
		if testeeResult.ProfileID != nil && s.guardianshipSvc != nil && s.guardianshipSvc.IsEnabled() {
			guardians, err := s.fetchGuardians(ctx, *testeeResult.ProfileID)
			if err != nil {
				// 家长信息获取失败不影响主流程，记录日志即可
				logger.L(ctx).Warnw("Failed to fetch guardians for testee in list",
					"action", "list_testees_with_guardians",
					"testee_id", testeeResult.ID,
					"profile_id", *testeeResult.ProfileID,
					"error", err.Error(),
				)
			} else {
				backendItem.Guardians = guardians
			}
		}

		backendItems[i] = backendItem
	}

	return &TesteeBackendListResult{
		Items:      backendItems,
		TotalCount: listResult.TotalCount,
		Offset:     listResult.Offset,
		Limit:      listResult.Limit,
	}, nil
}

// fetchGuardians 从 IAM 服务获取监护人信息
func (s *backendQueryService) fetchGuardians(ctx context.Context, profileID uint64) ([]GuardianInfo, error) {
	childID := fmt.Sprintf("%d", profileID)

	resp, err := s.guardianshipSvc.ListGuardians(ctx, childID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list guardians from IAM")
	}

	if resp == nil || len(resp.Items) == 0 {
		return []GuardianInfo{}, nil
	}

	// 转换 IAM 响应为 GuardianInfo
	guardians := make([]GuardianInfo, 0, len(resp.Items))
	for _, edge := range resp.Items {
		if edge.Guardianship == nil || edge.Guardian == nil {
			continue
		}

		// 获取监护关系
		relation := edge.Guardianship.GetRelation().String()

		// 从 Guardian 的 Contacts 中获取电话号码
		phone := ""
		if len(edge.Guardian.Contacts) > 0 {
			// 优先获取手机号
			for _, contact := range edge.Guardian.Contacts {
				if contact.GetType().String() == "CONTACT_TYPE_PHONE" {
					phone = contact.GetValue()
					break
				}
			}
		}

		guardians = append(guardians, GuardianInfo{
			Name:     edge.Guardian.GetNickname(),
			Relation: relation,
			Phone:    phone,
		})
	}

	return guardians, nil
}
