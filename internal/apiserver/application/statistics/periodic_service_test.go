package statistics

import (
	"context"
	"testing"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type periodicReaderStub struct {
	orgID    int64
	testeeID uint64
	result   *domainStatistics.TesteePeriodicStatisticsResponse
}

func (s *periodicReaderStub) GetPeriodicStats(_ context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteePeriodicStatisticsResponse, error) {
	s.orgID = orgID
	s.testeeID = testeeID
	return s.result, nil
}

func TestPeriodicStatsServiceDelegatesToReader(t *testing.T) {
	t.Parallel()

	reader := &periodicReaderStub{result: &domainStatistics.TesteePeriodicStatisticsResponse{TotalProjects: 2}}
	service := NewPeriodicStatsService(reader)

	got, err := service.GetPeriodicStats(context.Background(), 11, 22)
	if err != nil {
		t.Fatalf("GetPeriodicStats returned error: %v", err)
	}
	if got.TotalProjects != 2 {
		t.Fatalf("TotalProjects = %d, want 2", got.TotalProjects)
	}
	if reader.orgID != 11 || reader.testeeID != 22 {
		t.Fatalf("reader args = (%d, %d), want (11, 22)", reader.orgID, reader.testeeID)
	}
}
