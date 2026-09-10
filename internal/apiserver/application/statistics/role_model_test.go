package statistics

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProfessionalStatisticsDenyBeforeCacheOrStorage(t *testing.T) {
	s := &ReadService{}
	ctx := context.Background()
	_, err := s.Overview(ctx, 1, QueryFilter{})
	require.Error(t, err)
	_, err = s.Clinicians(ctx, 1, nil, nil, QueryFilter{}, 1, 10)
	require.Error(t, err)
	_, err = s.Entries(ctx, 1, nil, nil, nil, QueryFilter{}, 1, 10)
	require.Error(t, err)
	_, err = s.CurrentClinicianTesteeSummary(ctx, 1, 1, QueryFilter{})
	require.Error(t, err)
}
