package assessment

import (
	"context"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

type assessmentListFilter struct {
	testeeID      *uint64
	rawTesteeID   string
	status        *assessment.Status
	rawStatus     string
	invalidStatus bool
}

type assessmentAdminQuery struct {
	reader evaluationreadmodel.AssessmentReader
}

func parseAssessmentListFilter(dto ListAssessmentsDTO) (*assessmentListFilter, error) {
	filter := &assessmentListFilter{}
	if dto.TesteeID != nil {
		testeeID := *dto.TesteeID
		filter.rawTesteeID = strconv.FormatUint(testeeID, 10)
		filter.testeeID = &testeeID
	}
	if dto.Status != "" {
		status := assessment.Status(dto.Status)
		filter.rawStatus = dto.Status
		if status.IsValid() {
			filter.status = &status
		} else {
			filter.invalidStatus = true
		}
	}

	return filter, nil
}

func (q assessmentAdminQuery) List(
	ctx context.Context,
	dto ListAssessmentsDTO,
	orgID int64,
	page int,
	pageSize int,
	filter *assessmentListFilter,
) ([]*AssessmentResult, int64, error) {
	if q.reader == nil {
		return nil, 0, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	rows, total, err := q.queryRows(ctx, dto, orgID, page, pageSize, filter)
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
	listFilter *assessmentListFilter,
) ([]evaluationreadmodel.AssessmentRow, int64, error) {
	l := logger.L(ctx)
	if listFilter.invalidStatus {
		return []evaluationreadmodel.AssessmentRow{}, 0, nil
	}

	filter := evaluationreadmodel.AssessmentFilter{
		OrgID:                 orgID,
		RestrictToAccessScope: dto.RestrictToAccessScope,
		AccessibleTesteeIDs:   dto.AccessibleTesteeIDs,
	}
	if listFilter.testeeID != nil {
		filter.TesteeID = listFilter.testeeID
	}
	if listFilter.status != nil {
		filter.Statuses = []string{listFilter.status.String()}
	}
	if dto.OrgID == 0 && listFilter.testeeID == nil {
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
			"testee_id", listFilter.rawTesteeID,
			"error", err.Error(),
		)
		return nil, 0, evalerrors.Database(err, "查询测评列表失败")
	}
	return rows, total, nil
}
