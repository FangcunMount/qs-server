package testee

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// toTesteeResult 将领域对象转换为应用层 DTO
func toTesteeResult(testee *domain.Testee) *TesteeResult {
	result := &TesteeResult{
		ID:         testee.ID().Uint64(),
		OrgID:      testee.OrgID(),
		Name:       testee.Name(),
		Gender:     int8(testee.Gender()),
		CreatedAt:  testee.CreatedAt(),
		UpdatedAt:  testee.UpdatedAt(),
		Age:        testee.GetAge(),
		Tags:       testee.TagsAsStrings(),
		Source:     testee.Source(),
		IsKeyFocus: testee.IsKeyFocus(),
	}

	// 可选字段
	if testee.ProfileID() != nil {
		profileID := *testee.ProfileID()
		result.ProfileID = &profileID
	}

	if testee.Birthday() != nil {
		result.Birthday = testee.Birthday()
	}

	// 统计信息
	if stats := testee.AssessmentStats(); stats != nil {
		result.TotalAssessments = stats.TotalCount()
		lastAt := stats.LastAssessmentAt()
		result.LastAssessmentAt = &lastAt
		result.LastRiskLevel = stats.LastRiskLevel()
	}

	return result
}
