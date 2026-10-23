package retryschedule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func reviewFixture(t *testing.T, kind, id string) Frozen {
	t.Helper()
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	a, in := fixture(t, kind, retrygovernance.AttemptOriginAutomatic, "", id, at)
	f, err := New(a, in, at)
	require.NoError(t, err)
	return f
}

func rawDigest(raw []byte) string {
	d := sha256.Sum256(raw)
	return hex.EncodeToString(d[:])
}

func TestHandoffReviewPreservesExactFrozenEvidenceAndPrimary(t *testing.T) {
	for _, kind := range []string{Evaluation, Interpretation} {
		t.Run(kind, func(t *testing.T) {
			f := reviewFixture(t, kind, "original-event")
			binding := transaction.Binding{Owner: f.Authority().Owner, StoreID: "original-primary", WriterEpoch: "m3-1"}
			r, err := ReviewHandoff(binding, strings.Repeat("a", 40), "operator:9", "approved maintenance", []Frozen{f})
			require.NoError(t, err)
			raw := r.Bytes()
			decoded, err := DecodeHandoffReview(raw, rawDigest(raw))
			require.NoError(t, err)
			require.Equal(t, binding, decoded.Binding())
			require.Equal(t, r.SourceSHA(), decoded.SourceSHA())
			require.Equal(t, r.Actor(), decoded.Actor())
			require.Equal(t, r.Reason(), decoded.Reason())
			require.Equal(t, f.Bytes(), decoded.Plans()[0].Bytes())
			require.Equal(t, f.DueAt(), decoded.Plans()[0].DueAt())
			raw[0] ^= 1
			copiedPlans := r.Plans()
			copiedPlans[0] = Frozen{}
			require.True(t, r.Plans()[0].Valid())
			require.NotEqual(t, raw, r.Bytes())
		})
	}
}

func TestHandoffReviewRejectsAmbiguousOrUnreviewedFiles(t *testing.T) {
	f := reviewFixture(t, Evaluation, "original-event")
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: "original-primary", WriterEpoch: "m3-1"}
	r, err := ReviewHandoff(binding, strings.Repeat("a", 40), "operator:9", "approved maintenance", []Frozen{f})
	require.NoError(t, err)
	for name, raw := range map[string][]byte{
		"duplicate-field": append([]byte(`{"version":1,`), r.Bytes()[1:]...),
		"unknown-field":   append([]byte(`{"extra":true,`), r.Bytes()[1:]...),
		"leading-space":   append([]byte(" "), r.Bytes()...),
		"trailing-value":  append(r.Bytes(), []byte("{}")...),
		"changed-version": bytes.Replace(r.Bytes(), []byte(`"version":1`), []byte(`"version":2`), 1),
		"oversized":       bytes.Repeat([]byte(" "), handoffReviewLimit+1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeHandoffReview(raw, rawDigest(raw))
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
	for _, digest := range []string{"", strings.Repeat("0", 64), strings.ToUpper(rawDigest(r.Bytes()))} {
		_, err := DecodeHandoffReview(r.Bytes(), digest)
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, err = ReviewHandoff(binding, r.SourceSHA(), r.Actor(), r.Reason(), []Frozen{f, f})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = ReviewHandoff(binding, r.SourceSHA(), r.Actor(), r.Reason(), []Frozen{reviewFixture(t, Interpretation, "different-owner")})
	require.ErrorIs(t, err, ErrInvalid)
	for _, bad := range []string{"", "  ", "operator\n9", string([]byte{0xff}), strings.Repeat("a", 129)} {
		_, err = ReviewHandoff(binding, r.SourceSHA(), bad, r.Reason(), []Frozen{f})
		require.ErrorIs(t, err, ErrInvalid)
	}
	for _, plans := range [][]Frozen{nil, {{}}, make([]Frozen, 101)} {
		_, err = ReviewHandoff(binding, r.SourceSHA(), r.Actor(), r.Reason(), plans)
		require.ErrorIs(t, err, ErrInvalid)
	}
	for _, source := range []string{"main", strings.Repeat("A", 40), strings.Repeat("a", 39)} {
		_, err = ReviewHandoff(binding, source, r.Actor(), r.Reason(), []Frozen{f})
		require.ErrorIs(t, err, ErrInvalid)
	}
	require.False(t, (HandoffReview{}).Valid())
}
