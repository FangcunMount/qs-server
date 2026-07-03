package systemgovernance

import (
	"context"
	"testing"
	"time"

	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
)

func TestGetOverviewProbesMetricsOnce(t *testing.T) {
	metrics := &countingMetricsClient{}
	_, err := NewFacade(FacadeDeps{Metrics: metrics}).GetOverview(context.Background(), "5m")
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if metrics.probes != 1 {
		t.Fatalf("Probe calls = %d, want 1", metrics.probes)
	}
}

type countingMetricsClient struct {
	probes int
}

func (c *countingMetricsClient) Probe(context.Context, time.Time) govprom.Summary {
	c.probes++
	return govprom.Summary{Available: true}
}

func (c *countingMetricsClient) Query(_ context.Context, spec govprom.QuerySpec, _ time.Time) govprom.MetricResult {
	return govprom.MetricResult{Name: spec.Name, Window: spec.Window, Unit: spec.Unit, Available: true}
}
