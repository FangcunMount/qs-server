package acl

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// TesteeActorAdapter 将 infra ActorWriter 适配为 testee.ActorPort。
type TesteeActorAdapter struct {
	inner  grpcbridge.ActorWriter
	reader actorReaderBridge
}

// NewTesteeActorAdapter 构造受试者 BFF ACL 适配器。
func NewTesteeActorAdapter(inner grpcbridge.ActorWriter) *TesteeActorAdapter {
	return &TesteeActorAdapter{
		inner:  inner,
		reader: newActorReaderBridge(inner),
	}
}

func (a *TesteeActorAdapter) GetTestee(ctx context.Context, testeeID uint64) (*testee.TesteeResponse, error) {
	if a == nil {
		return nil, nil
	}
	out, err := a.reader.getTestee(ctx, testeeID)
	if err != nil {
		return nil, err
	}
	return toTesteeResponse(out), nil
}

func (a *TesteeActorAdapter) TesteeExists(ctx context.Context, orgID, iamProfileID uint64) (bool, uint64, error) {
	if a == nil {
		return false, 0, nil
	}
	return a.reader.testeeExists(ctx, orgID, iamProfileID)
}

func (a *TesteeActorAdapter) CreateTestee(ctx context.Context, input testee.CreateTesteeInput) (*testee.TesteeResponse, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	out, err := a.inner.CreateTestee(ctx, &grpcbridge.CreateTesteeRequest{
		OrgID:        input.OrgID,
		IAMUserID:    input.IAMUserID,
		IAMProfileID: input.IAMProfileID,
		Name:         input.Name,
		Gender:       input.Gender,
		Birthday:     birthdayToTimePtr(input.Birthday),
		Tags:         input.Tags,
		Source:       input.Source,
		IsKeyFocus:   input.IsKeyFocus,
	})
	if err != nil {
		return nil, err
	}
	resp := toTesteeResponse(out)
	if resp != nil {
		resp.IAMUserID = input.IAMUserID
		resp.IAMProfileID = input.IAMProfileID
	}
	return resp, nil
}

func (a *TesteeActorAdapter) GetTesteeCareContext(ctx context.Context, testeeID uint64) (*testee.TesteeCareContextResponse, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	out, err := a.inner.GetTesteeCareContext(ctx, testeeID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &testee.TesteeCareContextResponse{}, nil
	}
	return &testee.TesteeCareContextResponse{
		ClinicianName:   out.ClinicianName,
		ClinicianRole:   out.ClinicianRole,
		RelationType:    out.RelationType,
		EntryTitle:      out.EntryTitle,
		EntrySourceType: out.EntrySourceType,
	}, nil
}

func (a *TesteeActorAdapter) UpdateTestee(ctx context.Context, testeeID uint64, req *testee.UpdateTesteeRequest) (*testee.TesteeResponse, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	out, err := a.inner.UpdateTestee(ctx, &grpcbridge.UpdateTesteeRequest{
		ID:         testeeID,
		Name:       req.Name,
		Gender:     req.Gender,
		Birthday:   birthdayToTimePtr(req.Birthday),
		Tags:       req.Tags,
		IsKeyFocus: req.IsKeyFocus,
	})
	if err != nil {
		return nil, err
	}
	return toTesteeResponse(out), nil
}

func (a *TesteeActorAdapter) ListTesteesByUser(ctx context.Context, profileIDs []uint64, offset, limit int32) ([]*testee.TesteeResponse, int64, error) {
	if a == nil || a.inner == nil {
		return nil, 0, nil
	}
	items, total, err := a.inner.ListTesteesByUser(ctx, profileIDs, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	result := make([]*testee.TesteeResponse, len(items))
	for i, item := range items {
		result[i] = toTesteeResponse(item)
	}
	return result, total, nil
}

func toTesteeResponse(from *grpcbridge.TesteeResponse) *testee.TesteeResponse {
	if from == nil {
		return nil
	}
	resp := &testee.TesteeResponse{
		ID:           strconv.FormatUint(from.ID, 10),
		OrgID:        strconv.FormatUint(from.OrgID, 10),
		IAMUserID:    from.IAMUserID,
		IAMProfileID: from.IAMProfileID,
		Name:         from.Name,
		Gender:       from.Gender,
		Birthday:     meta.NewBirthday(from.Birthday.Format("2006-01-02")),
		Tags:         from.Tags,
		Source:       from.Source,
		IsKeyFocus:   from.IsKeyFocus,
		CreatedAt:    from.CreatedAt,
		UpdatedAt:    from.UpdatedAt,
	}
	if from.AssessmentStats != nil {
		resp.AssessmentStats = &testee.AssessmentStatsDTO{
			TotalCount:       from.AssessmentStats.TotalCount,
			LastAssessmentAt: from.AssessmentStats.LastAssessmentAt,
			LastRiskLevel:    from.AssessmentStats.LastRiskLevel,
		}
	}
	return resp
}

func birthdayToTimePtr(birthday *meta.Birthday) *time.Time {
	if birthday == nil {
		return nil
	}
	return birthday.ToTimePtr()
}
