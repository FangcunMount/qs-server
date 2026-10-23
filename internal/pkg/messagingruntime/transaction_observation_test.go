package messagingruntime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestProtocolMonitorRetainsActualDatabaseWorkAndScrapesNeverRead(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int64
	registry := prometheus.NewRegistry()
	m, err := newProtocolMonitor("iam.mysql", func(context.Context) (protocolSample, error) {
		reads.Add(1)
		close(entered)
		<-release
		return protocolSample{}, nil
	}, func() rocketmq.TransactionStats { return rocketmq.TransactionStats{Ready: true} }, func() bool { return false }, transaction.DefaultLimits(), registry)
	require.NoError(t, err)
	require.Zero(t, reads.Load(), "constructor ran DB work")
	require.NoError(t, m.start())
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("sampler did not enter")
	}
	for i := 0; i < 3; i++ {
		metrics, err := registry.Gather()
		require.NoError(t, err)
		require.NotEmpty(t, metrics)
	}
	require.EqualValues(t, 1, reads.Load(), "scrapes started database work")
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, m.close(expired), transaction.ErrShutdownIncomplete)
	select {
	case <-m.done:
		t.Fatal("cancellation discarded still-running database work")
	default:
	}
	require.False(t, m.allowed())
	require.Error(t, m.start(), "stopped sampler restarted")
	close(release)
	finish, done := context.WithTimeout(t.Context(), time.Second)
	defer done()
	require.NoError(t, m.close(finish))
	require.NoError(t, m.close(finish))
	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.Empty(t, metrics, "closed runtime retained collector ownership")
}

func TestProtocolCollectorsShareNamesAcrossOwnersWithoutMultiplyingChecks(t *testing.T) {
	registry := prometheus.NewRegistry()
	var reads atomic.Int64
	monitors := make([]*protocolMonitor, 0, 2)
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		m, err := newProtocolMonitor(owner, func(context.Context) (protocolSample, error) {
			reads.Add(1)
			return protocolSample{untrackedAborted: 2}, nil
		},
			func() rocketmq.TransactionStats {
				return rocketmq.TransactionStats{Ready: true, RunningChecks: 1, QueuedChecks: 2}
			}, func() bool { return false }, transaction.DefaultLimits(), registry)
		require.NoError(t, err)
		require.NoError(t, m.start())
		monitors = append(monitors, m)
	}
	require.Eventually(t, func() bool { return monitors[0].allowed() && monitors[1].allowed() }, time.Second, time.Millisecond)
	before := reads.Load()
	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.Equal(t, before, reads.Load())
	found := false
	for _, family := range metrics {
		if family.GetName() == protocolMetricPrefix+"untracked_aborted_branches" {
			found = true
			require.Len(t, family.Metric, 2)
			for _, metric := range family.Metric {
				require.Equal(t, float64(2), metric.GetGauge().GetValue())
				require.Len(t, metric.Label, 1)
				require.Equal(t, "owner", metric.Label[0].GetName())
			}
		}
	}
	require.True(t, found)
	for _, m := range monitors {
		require.NoError(t, m.close(t.Context()))
	}
}

func TestRuntimeProtectionKeepsCheckerAndCannotBeBypassedByOpeningAdmission(t *testing.T) {
	// No goroutine is needed to prove the host gate: sampling state is injected
	// under its actual mutex, independently of transport/native dependency proofs.
	driver := &transactionDriverStub{ready: true}
	runtime := &TransactionRuntime{sender: driver, admission: true}
	monitor, err := newProtocolMonitor("iam.mysql", func(context.Context) (protocolSample, error) { return protocolSample{}, nil }, driver.Stats, runtime.admitted, transaction.DefaultLimits(), prometheus.NewRegistry())
	require.NoError(t, err)
	runtime.monitor = monitor
	monitor.mu.Lock()
	monitor.policy.observe(time.Now(), protocolSample{gaps: 1, confirmationAge: 300}, nil, false, false, monitor.freshness)
	monitor.mu.Unlock()
	require.False(t, runtime.Stats().Ready)
	require.ErrorIs(t, runtime.SetAdmission(true), ErrTransactionAdmissionClosed)
	_, err = runtime.Reserve(t.Context(), transaction.Request{}, transaction.FrozenPlan{})
	require.ErrorIs(t, err, ErrTransactionAdmissionClosed)
	_, err = runtime.Execute(t.Context(), transaction.Execution{}, nil)
	require.ErrorIs(t, err, ErrTransactionAdmissionClosed)
	require.Zero(t, driver.reserveCalls+driver.executeCalls+driver.stopCalls+driver.closeCalls)
	require.True(t, driver.ready, "protection stopped Checker")
	status := runtime.ProtectionStats()
	require.True(t, status.Protected)
	monitor.mu.Lock()
	monitor.policy.observe(time.Now(), protocolSample{}, nil, false, false, monitor.freshness)
	monitor.mu.Unlock()
	require.ErrorIs(t, runtime.SetAdmission(true), ErrTransactionAdmissionClosed, "healthy observation silently reopened protection")
	require.ErrorIs(t, runtime.ReviewProtection(status.Generation+1, "review-1"), ErrProtocolProtection)
	require.NoError(t, runtime.ReviewProtection(status.Generation, "review-1"))
	require.True(t, runtime.Stats().Ready)
	require.NoError(t, runtime.SetAdmission(false))
	require.False(t, runtime.Stats().Ready, "review bypassed operator's configured pause")
}
