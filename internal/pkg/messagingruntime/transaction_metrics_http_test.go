package messagingruntime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	hostserver "github.com/FangcunMount/qs-server/internal/pkg/server"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestExistingHTTPMetricsIncludesCachedOwnerObservationWithoutDatabaseScrapes(t *testing.T) {
	var reads atomic.Int64
	limits := transaction.DefaultLimits()
	limits.ReconcileInterval = time.Hour
	monitor, err := newProtocolMonitor("qs.mysql", func(context.Context) (protocolSample, error) {
		reads.Add(1)
		return protocolSample{gaps: 2, confirmationAge: 30}, nil
	}, func() rocketmq.TransactionStats {
		return rocketmq.TransactionStats{Ready: true, HalfPreparedResults: 2, LocalCommittedResults: 1, FinalUnknownResults: 2}
	},
		func() bool { return false }, limits, prometheus.DefaultRegisterer)
	require.NoError(t, err)
	require.NoError(t, monitor.start())
	t.Cleanup(func() { require.NoError(t, monitor.close(context.Background())) })
	require.NoError(t, monitor.waitInitial(t.Context()))
	config := hostserver.NewConfig()
	config.EnableMetrics = true
	config.EnableProfiling = false
	server, err := config.Complete().New()
	require.NoError(t, err)
	before := reads.Load()
	for i := 0; i < 2; i++ {
		result := httptest.NewRecorder()
		server.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		require.Equal(t, http.StatusOK, result.Code)
		require.Contains(t, result.Body.String(), `reliable_messaging_transaction_confirmation_gap_branches{owner="qs.mysql"} 2`)
		require.Contains(t, result.Body.String(), `reliable_messaging_transaction_observation_available{owner="qs.mysql"} 1`)
		require.Contains(t, result.Body.String(), `reliable_messaging_transaction_sampler_running{owner="qs.mysql"} 1`)
	}
	require.Equal(t, before, reads.Load(), "HTTP scraping started database work")
}
