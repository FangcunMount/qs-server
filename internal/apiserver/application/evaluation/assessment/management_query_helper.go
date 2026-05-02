package assessment

import (
	"context"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

type assessmentListConditions struct {
	testeeID      *testee.ID
	rawTesteeID   string
	status        *assessment.Status
	rawStatus     string
	invalidStatus bool
}

type assessmentAdminQuery struct {
	reader evaluationreadmodel.AssessmentReader
}

func parseAssessmentListConditions(dto ListAssessmentsDTO) (*assessmentListConditions, error) {
	conditions := &assessmentListConditions{}
	if dto.TesteeID != nil {
		testeeID := testee.NewID(*dto.TesteeID)
		conditions.rawTesteeID = strconv.FormatUint(*dto.TesteeID, 10)
		conditions.testeeID = &testeeID
	}
	if dto.Status != "" {
		status := assessment.Status(dto.Status)
		conditions.rawStatus = dto.Status
		if status.IsValid() {
			conditions.status = &status
		} else {
			conditions.invalidStatus = true
		}
	}

	if dto.Conditions == nil {
		return conditions, nil
	}

	if conditions.testeeID == nil {
		testeeIDStr := dto.Conditions["testee_id"]
		if testeeIDStr != "" {
			testeeIDUint, err := strconv.ParseUint(testeeIDStr, 10, 64)
			if err != nil {
				return nil, err
			}
			testeeID := testee.NewID(testeeIDUint)
			conditions.rawTesteeID = testeeIDStr
			conditions.testeeID = &testeeID
		}
	}

	if conditions.rawStatus == "" {
		statusStr := dto.Conditions["status"]
		if statusStr != "" {
			status := assessment.Status(statusStr)
			conditions.rawStatus = statusStr
			if status.IsValid() {
				conditions.status = &status
			} else {
				conditions.invalidStatus = true
			}
		}
	}

	return conditions, nil
}

func (q assessmentAdminQuery) List(
	ctx context.Context,
	dto ListAssessmentsDTO,
	orgID int64,
	page int,
	pageSize int,
	conditions *assessmentListConditions,
) ([]*AssessmentResult, int64, error) {
	if q.reader == nil {
		return nil, 0, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	rows, total, err := q.queryRows(ctx, dto, orgID, page, pageSize, conditions)
	if err != nil {
		return nil, 0, err
	}
	results, err := assessmentRowsToResults(rows)
	return results, total, err
}

func (q assessmentAdminQuery) queryRows(
	ctx context.Context,
	dto ListAssessmentsDTO,
	orgID int64,
	page int,
	pageSize int,
	conditions *assessmentListConditions,
) ([]evaluationreadmodel.AssessmentRow, int64, error) {
	l := logger.L(ctx)
	if conditions.invalidStatus {
		return []evaluationreadmodel.AssessmentRow{}, 0, nil
	}

	filter := evaluationreadmodel.AssessmentFilter{
		OrgID:                 orgID,
		RestrictToAccessScope: dto.RestrictToAccessScope,
		AccessibleTesteeIDs:   dto.AccessibleTesteeIDs,
	}
	if conditions.testeeID != nil {
		id := conditions.testeeID.Uint64()
		filter.TesteeID = &id
	}
	if conditions.status != nil {
		filter.Statuses = []string{conditions.status.String()}
	}
	if dto.OrgID == 0 && conditions.testeeID == nil {
		l.Warnw("未提供 testee_id 和 org_id，无法查询",
			"org_id", dto.OrgID,
		)
		return []evaluationreadmodel.AssessmentRow{}, 0, nil
	}

	rows, total, err := q.reader.ListAssessments(
		ctx,
		filter,
		evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize},
	)
	if err != nil {
		l.Errorw("通过 read model 查询测评列表失败",
			"org_id", dto.OrgID,
			"testee_id", conditions.rawTesteeID,
			"error", err.Error(),
		)
		return nil, 0, evalerrors.Database(err, "查询测评列表失败")
	}
	return rows, total, nil
}
