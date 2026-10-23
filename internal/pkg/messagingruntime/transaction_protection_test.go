package messagingruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProtocolProtectionUsesCurrentAuditAndRequiresMatchingReview(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	freshness := 12 * time.Second
	p := &protocolProtection{}
	require.False(t, p.allowed(now, freshness), "no sampling is not zero pending")
	// An age with no matching current gap must not become historical backlog.
	p.observe(now, protocolSample{confirmationAge: 10000, untrackedAborted: 8}, nil, false, false, freshness)
	require.True(t, p.allowed(now, freshness))
	p.observe(now, protocolSample{gaps: 1, confirmationAge: 300}, nil, false, false, freshness)
	require.Equal(t, "confirmation_gap", p.reason)
	require.False(t, p.allowed(now, freshness))
	generation := p.generation
	p.observe(now, protocolSample{}, nil, false, false, freshness)
	require.False(t, p.allowed(now, freshness), "healthy sampling cannot silently clear protection")
	for _, review := range []struct {
		generation  uint64
		name        string
		ready, full bool
	}{
		{generation + 1, "review-1", true, false}, {generation, "", true, false}, {generation, "review-1", false, false}, {generation, "review-1", true, true},
	} {
		require.ErrorIs(t, p.review(now, freshness, review.generation, review.name, review.ready, review.full), ErrProtocolProtection)
	}
	require.ErrorIs(t, p.review(now.Add(13*time.Second), freshness, generation, "review-1", true, false), ErrProtocolProtection)
	require.NoError(t, p.review(now, freshness, generation, "review-1", true, false))
	require.True(t, p.allowed(now, freshness))
	require.False(t, p.allowed(now.Add(13*time.Second), freshness), "blocked sampler must not leave stale admission open")
}

func TestProtocolProtectionSeparatesUntrackedAndUnagedEvidence(t *testing.T) {
	now := time.Now()
	freshness := 12 * time.Second
	for _, test := range []struct {
		sample protocolSample
		reason string
	}{
		{protocolSample{untrackedAborted: 3}, ""}, {protocolSample{gaps: 1, unaged: 1}, "confirmation_age_unavailable"},
		{protocolSample{unobservedCommitted: 1}, "committed_half_unobserved"}, {protocolSample{conflicts: 1}, "metadata_conflict"},
		{protocolSample{preparing: 1, expiryAge: 300}, "preparation_expired"}, {protocolSample{gaps: -1}, "invalid_observation"},
	} {
		p := &protocolProtection{}
		p.observe(now, test.sample, nil, false, false, freshness)
		require.Equal(t, test.reason, p.reason)
	}
}

func TestProtocolProtectionRequiresObservedSaturationAndPreservesUnavailable(t *testing.T) {
	now := time.Now()
	freshness := 12 * time.Second
	p := &protocolProtection{}
	for step := 0; step <= 24; step++ {
		p.observe(now.Add(time.Duration(step)*5*time.Second), protocolSample{}, nil, false, true, freshness)
		if step < 24 {
			require.Empty(t, p.reason)
		}
	}
	require.Equal(t, "checker_saturated", p.reason)
	gap := &protocolProtection{}
	gap.observe(now, protocolSample{}, nil, false, true, freshness)
	gap.observe(now.Add(121*time.Second), protocolSample{}, nil, false, true, freshness)
	require.Empty(t, gap.reason, "missing observations cannot prove continuous queue saturation")
	unavailable := &protocolProtection{}
	unavailable.observe(now, protocolSample{gaps: 1, confirmationAge: 10}, nil, false, false, freshness)
	for step := 1; step <= 25; step++ {
		unavailable.observe(now.Add(time.Duration(step)*5*time.Second), protocolSample{}, errors.New("unavailable"), false, false, freshness)
	}
	require.Equal(t, "observation_unavailable", unavailable.reason)
	require.EqualValues(t, 1, unavailable.sample.gaps, "failed read must not replace old counts with zero")
	require.False(t, unavailable.available)
}
