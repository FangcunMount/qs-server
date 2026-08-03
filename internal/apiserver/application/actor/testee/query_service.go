package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// 查询Service 受试者查询服务实现
// 行为者：所有需要查询受试者信息的用户
type queryService struct {
	reader        actorreadmodel.TesteeReader
	summaryReader actorreadmodel.AssessmentSummaryReader
}

func NewQueryServiceWithAssessmentSummary(reader actorreadmodel.TesteeReader, summaryReader actorreadmodel.AssessmentSummaryReader) TesteeQueryService {
	return &queryService{reader: reader, summaryReader: summaryReader}
}

// NewQueryService 创建受试者查询服务
func NewQueryService(reader actorreadmodel.TesteeReader) TesteeQueryService {
	return &queryService{
		reader: reader,
	}
}

// GetByID 根据ID查询受试者
func (s *queryService) GetByID(ctx context.Context, testeeID uint64) (*TesteeResult, error) {
	resolvedID, err := testeeIDFromUint64("testee_id", testeeID)
	if err != nil {
		return nil, err
	}

	testee, err := s.reader.GetTestee(ctx, resolvedID.Uint64())
	if err != nil {
		return nil, errors.Wrap(err, "failed to find testee")
	}

	if err := s.enrichAssessmentSummaries(ctx, []*actorreadmodel.TesteeRow{testee}); err != nil {
		return nil, errors.Wrap(err, "failed to read testee assessment summary")
	}
	return toTesteeResultFromRow(testee), nil
}

// FindByProfile 根据用户档案ID查询受试者
func (s *queryService) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeResult, error) {
	testee, err := s.reader.FindTesteeByProfile(ctx, orgID, profileID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, errors.Wrap(err, "failed to find testee by profile")
	}

	if err := s.enrichAssessmentSummaries(ctx, []*actorreadmodel.TesteeRow{testee}); err != nil {
		return nil, errors.Wrap(err, "failed to read testee assessment summary")
	}
	return toTesteeResultFromRow(testee), nil
}

// ListTestees 列出受试者
func (s *queryService) ListTestees(ctx context.Context, dto ListTesteeDTO) (*TesteeListResult, error) {
	if dto.RestrictToAccessScope {
		for _, id := range dto.AccessibleTesteeIDs {
			if _, err := testeeIDFromUint64("accessible_testee_id", id); err != nil {
				return nil, err
			}
		}
	}

	filter := actorreadmodel.TesteeFilter{
		OrgID:                 dto.OrgID,
		Name:                  dto.Name,
		KeyFocus:              dto.KeyFocus,
		CreatedAtStart:        dto.CreatedAtStart,
		CreatedAtEnd:          dto.CreatedAtEnd,
		AccessibleTesteeIDs:   append([]uint64(nil), dto.AccessibleTesteeIDs...),
		RestrictToAccessScope: dto.RestrictToAccessScope,
		Offset:                dto.Offset,
		Limit:                 dto.Limit,
	}

	testees, err := s.reader.ListTestees(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list testees")
	}
	if err := s.enrichAssessmentSummaryRows(ctx, testees); err != nil {
		return nil, errors.Wrap(err, "failed to read testee assessment summaries")
	}
	totalCount, err := s.reader.CountTestees(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees")
	}

	// 转换为 DTO
	items := make([]*TesteeResult, len(testees))
	for i := range testees {
		items[i] = toTesteeResultFromRow(&testees[i])
	}

	return &TesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

// ListKeyFocus 列出重点关注的受试者
func (s *queryService) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) (*TesteeListResult, error) {
	keyFocus := true
	return s.ListTestees(ctx, ListTesteeDTO{
		OrgID:    orgID,
		KeyFocus: &keyFocus,
		Offset:   offset,
		Limit:    limit,
	})
}

// ListByProfileIDs 根据多个用户档案ID查询受试者列表
func (s *queryService) ListByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) (*TesteeListResult, error) {
	if len(profileIDs) == 0 {
		return &TesteeListResult{
			Items:      []*TesteeResult{},
			TotalCount: 0,
			Offset:     offset,
			Limit:      limit,
		}, nil
	}

	testees, err := s.reader.ListTesteesByProfileIDs(ctx, profileIDs, offset, limit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list testees by profile IDs")
	}
	if err := s.enrichAssessmentSummaryRows(ctx, testees); err != nil {
		return nil, errors.Wrap(err, "failed to read testee assessment summaries")
	}

	totalCount, err := s.reader.CountTesteesByProfileIDs(ctx, profileIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count testees by profile IDs")
	}

	// 转换为 DTO
	items := make([]*TesteeResult, len(testees))
	for i := range testees {
		items[i] = toTesteeResultFromRow(&testees[i])
	}

	return &TesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     offset,
		Limit:      limit,
	}, nil
}

func (s *queryService) enrichAssessmentSummaryRows(ctx context.Context, rows []actorreadmodel.TesteeRow) error {
	pointers := make([]*actorreadmodel.TesteeRow, 0, len(rows))
	for i := range rows {
		pointers = append(pointers, &rows[i])
	}
	return s.enrichAssessmentSummaries(ctx, pointers)
}

func (s *queryService) enrichAssessmentSummaries(ctx context.Context, rows []*actorreadmodel.TesteeRow) error {
	if s.summaryReader == nil || len(rows) == 0 {
		return nil
	}
	var orgID int64
	for _, row := range rows {
		if row != nil {
			orgID = row.OrgID
			break
		}
	}
	if orgID == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.OrgID != orgID {
			return errors.WithCode(code.ErrInternalServerError, "testee page contains multiple organizations")
		}
		ids = append(ids, row.ID)
		row.TotalAssessments = 0
		row.LastAssessmentAt = nil
		row.LastRiskLevel = ""
	}
	summaries, err := s.summaryReader.ReadAssessmentSummaries(ctx, orgID, ids)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		summary, ok := summaries[row.ID]
		if !ok {
			continue
		}
		row.TotalAssessments = summary.TotalEvaluated
		row.LastAssessmentAt = summary.LastEvaluatedAt
		row.LastRiskLevel = summary.RiskLevel
	}
	return nil
}
