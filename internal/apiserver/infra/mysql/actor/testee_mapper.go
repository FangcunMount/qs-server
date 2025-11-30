package actor

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// TesteeMapper 受试者映射器
type TesteeMapper struct{}

// NewTesteeMapper 创建受试者映射器
func NewTesteeMapper() *TesteeMapper {
	return &TesteeMapper{}
}

// ToPO 将领域对象转换为持久化对象
func (m *TesteeMapper) ToPO(domain *testee.Testee) *TesteePO {
	if domain == nil {
		return nil
	}

	po := &TesteePO{
		OrgID:      domain.OrgID(),
		Name:       domain.Name(),
		Gender:     int8(domain.Gender()),
		Birthday:   domain.Birthday(),
		Tags:       domain.TagsAsStrings(),
		Source:     domain.Source(),
		IsKeyFocus: domain.IsKeyFocus(),
	}

	// 处理 ProfileID
	po.ProfileID = domain.ProfileID()

	// 设置ID（如果已存在）
	if domain.ID() > 0 {
		po.ID = meta.ID(domain.ID())
	}

	// 映射测评统计
	if stats := domain.AssessmentStats(); stats != nil {
		po.TotalAssessments = stats.TotalCount()
		lastAt := stats.LastAssessmentAt()
		po.LastAssessmentAt = &lastAt
		level := stats.LastRiskLevel()
		if level != "" {
			po.LastRiskLevel = &level
		}
	}

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *TesteeMapper) ToDomain(po *TesteePO) *testee.Testee {
	if po == nil {
		return nil
	}

	// 创建基础受试者
	domain := testee.NewTestee(po.OrgID, po.Name, testee.Gender(po.Gender), po.Birthday)

	// 设置ID
	domain.SetID(testee.ID(po.ID))

	// 设置来源
	domain.SetSource(po.Source)

	// 构建测评统计
	var stats *testee.AssessmentStats
	if po.TotalAssessments > 0 || po.LastAssessmentAt != nil {
		lastRiskLevel := ""
		if po.LastRiskLevel != nil {
			lastRiskLevel = *po.LastRiskLevel
		}
		lastAt := time.Time{}
		if po.LastAssessmentAt != nil {
			lastAt = *po.LastAssessmentAt
		}
		// NewAssessmentStats(lastAssessmentAt time.Time, totalCount int, lastRiskLevel string)
		stats = testee.NewAssessmentStats(lastAt, po.TotalAssessments, lastRiskLevel)
	}

	// 从仓储恢复状态
	domain.RestoreFromRepository(
		po.ProfileID,
		po.Tags,
		po.IsKeyFocus,
		stats,
	)

	return domain
}

// ToDomains 批量转换为领域对象
func (m *TesteeMapper) ToDomains(pos []*TesteePO) []*testee.Testee {
	if pos == nil {
		return nil
	}

	domains := make([]*testee.Testee, len(pos))
	for i, po := range pos {
		domains[i] = m.ToDomain(po)
	}
	return domains
}

// SyncID 同步ID到领域对象
func (m *TesteeMapper) SyncID(po *TesteePO, domain *testee.Testee) {
	if po != nil && domain != nil {
		domain.SetID(testee.ID(po.ID))
	}
}
