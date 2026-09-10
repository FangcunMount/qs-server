package testee

import (
	"context"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOperatorSubjectListClearsStoredRiskWithoutSummaryReader(t *testing.T) {
	t.Setenv("QS_AUTHZ_ROLE_MODEL", appauthz.IndependentRoleModel)
	row := &actorreadmodel.TesteeRow{ID: 1, OrgID: 1, LastRiskLevel: "high", TotalAssessments: 4}
	require.NoError(t, (&queryService{}).enrichAssessmentSummaries(context.Background(), []*actorreadmodel.TesteeRow{row}))
	require.Empty(t, row.LastRiskLevel)
	require.Zero(t, row.TotalAssessments)
}

func TestSelfServicePreservesExistingSummaryWithoutBackendRole(t *testing.T) {
	t.Setenv("QS_AUTHZ_ROLE_MODEL", appauthz.IndependentRoleModel)
	row := &actorreadmodel.TesteeRow{ID: 1, OrgID: 1, LastRiskLevel: "high", TotalAssessments: 4}
	s := NewSelfServiceQueryServiceWithAssessmentSummary(nil, nil).(*queryService)
	require.NoError(t, s.enrichAssessmentSummaries(context.Background(), []*actorreadmodel.TesteeRow{row}))
	require.Equal(t, "high", row.LastRiskLevel)
	require.Equal(t, 4, row.TotalAssessments)
}
