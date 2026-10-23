package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type routes struct{}

func (routes) GetTopicForEvent(string) (string, bool) { return model.LifecycleTopic, true }

func fixture(t *testing.T) (model.HandoffReview, []string, string) {
	t.Helper()
	oldSource := version.GitCommit
	version.GitCommit = strings.Repeat("a", 40)
	t.Cleanup(func() { version.GitCommit = oldSource })
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	e := event.Event[payload.EvaluationRequestedData]{
		BaseEvent: event.BaseEvent{ID: "private-event-id", EventTypeValue: "evaluation.retry.requested", OccurredAtValue: at, AggregateTypeValue: "Evaluation", AggregateIDValue: "41"},
		Data:      payload.EvaluationRequestedData{OrgID: 7, AssessmentID: 41, TesteeID: 9, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: at},
	}
	prepared, err := standard.PrepareIntents([]event.DomainEvent{e}, routes{}, "api-server")
	require.NoError(t, err)
	plan, err := model.New(model.Authority{Owner: "qs.mysql", OrgID: 7, Kind: model.Evaluation, SubjectID: "41", RunID: "41:1", AssessmentID: 41, TesteeID: 9, ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginAutomatic, DecisionHash: [32]byte{1}}, prepared[0].Message.Input(), at)
	require.NoError(t, err)
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: "original-primary", WriterEpoch: "m3-1"}
	review, err := model.ReviewHandoff(binding, version.GitCommit, "private-operator", "private-review-reason", []model.Frozen{plan})
	require.NoError(t, err)
	dir := t.TempDir()
	path := filepath.Join(dir, "review.json")
	require.NoError(t, os.WriteFile(path, review.Bytes(), 0600))
	digest := review.Digest()
	args := []string{"--review-file", path, "--review-sha256", hex.EncodeToString(digest[:]), "--owner", binding.Owner, "--store-id", binding.StoreID, "--writer-epoch", binding.WriterEpoch}
	return review, args, filepath.Join(dir, "receipt.jsonl")
}

func noEnvironment(string) string { panic("offline verification must not load database bindings") }

func noHost(context.Context, arguments, func(string) string) (executor, func() error, error) {
	panic("offline verification must not construct a database host")
}

func TestDefaultVerificationNeverLoadsDatabaseOrExportsPrivateSelection(t *testing.T) {
	_, args, _ := fixture(t)
	var output bytes.Buffer
	require.NoError(t, run(t.Context(), args, noEnvironment, &output, noHost))
	var result map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	require.Equal(t, "offline_review_valid", result["status"])
	require.Equal(t, false, result["cutover_authorized"])
	for _, private := range []string{"private-event-id", "private-operator", "private-review-reason", "original-primary"} {
		require.NotContains(t, output.String(), private)
	}
}

func TestPrivateReviewRejectsLinksPermissionsFifoAndOversize(t *testing.T) {
	review, args, _ := fixture(t)
	digest := args[3]
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, review.Bytes(), 0600))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))
	_, err := readReview(link, digest)
	require.Error(t, err)
	require.NoError(t, os.Chmod(target, 0644))
	_, err = readReview(target, digest)
	require.Error(t, err)
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, unix.Mkfifo(fifo, 0600))
	_, err = readReview(fifo, digest)
	require.Error(t, err)
	_, err = readReview(dir, digest)
	require.Error(t, err)
	require.NoError(t, os.Chmod(target, 0600))
	require.NoError(t, os.WriteFile(target, bytes.Repeat([]byte(" "), (1<<20)+1), 0600))
	_, err = readReview(target, digest)
	require.Error(t, err)
}

type hostFixture struct {
	review model.HandoffReview
	err    error
	calls  int
}

func (h *hostFixture) Apply(_ context.Context, r model.HandoffReview, p app.HandoffPreconditions) ([]app.HandoffOutcome, error) {
	h.calls++
	if r.Digest() != h.review.Digest() || p.ReviewedDigest != r.Digest() || !p.AllLegacyWritersStopped || !p.AdmissionClosed {
		return nil, errors.New("incorrect reviewed host invocation")
	}
	status := "transferred"
	if h.err != nil {
		status = "unknown"
	}
	return []app.HandoffOutcome{{ScheduleID: r.Plans()[0].ID(), Status: status}}, h.err
}

func applyArgs(args []string, receipt string) []string {
	return append(append([]string(nil), args...), "--apply", "--database", "original-db", "--receipt", receipt, "--all-legacy-writers-stopped-and-admission-closed")
}

func TestApplyFsyncsPrivateStartedReceiptBeforeOpeningHostAndJoinsCleanup(t *testing.T) {
	for _, failed := range []string{"", "commit", "cleanup"} {
		t.Run(failed, func(t *testing.T) {
			review, args, receipt := fixture(t)
			h := &hostFixture{review: review}
			if failed == "commit" {
				h.err = app.ErrHandoffIncomplete
			}
			closed := 0
			open := func(_ context.Context, a arguments, _ func(string) string) (executor, func() error, error) {
				raw, err := os.ReadFile(receipt)
				require.NoError(t, err)
				var header map[string]any
				require.NoError(t, json.Unmarshal(raw, &header))
				require.Equal(t, "started", header["status"])
				require.Equal(t, false, header["cutover_authorized"])
				require.Equal(t, "original-db", a.database)
				info, err := os.Stat(receipt)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0600), info.Mode().Perm())
				return h, func() error {
					closed++
					if failed == "cleanup" {
						return errors.New("controlled owned cleanup failure")
					}
					return nil
				}, nil
			}
			var output bytes.Buffer
			err := run(t.Context(), applyArgs(args, receipt), noEnvironment, &output, open)
			if failed == "" {
				require.NoError(t, err)
				require.Contains(t, output.String(), "handoff_complete")
			} else {
				require.Error(t, err)
				require.Empty(t, output.String())
			}
			require.Equal(t, 1, closed)
			require.Equal(t, 1, h.calls)
			raw, err := os.ReadFile(receipt)
			require.NoError(t, err)
			lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
			require.Len(t, lines, 2)
			var final map[string]any
			require.NoError(t, json.Unmarshal(lines[1], &final))
			want := "handoff_complete"
			if failed != "" {
				want = "handoff_incomplete"
			}
			require.Equal(t, want, final["status"])
			require.Equal(t, false, final["cutover_authorized"])
			require.NotContains(t, string(raw), "private-event-id")
			require.NotContains(t, output.String(), "private-operator")
		})
	}
}

func TestApplyRefusesExistingReceiptWrongBindingAndSourceBeforeHost(t *testing.T) {
	_, args, receipt := fixture(t)
	require.NoError(t, os.WriteFile(receipt, []byte("preserve"), 0600))
	require.Error(t, run(t.Context(), applyArgs(args, receipt), noEnvironment, &bytes.Buffer{}, noHost))
	raw, err := os.ReadFile(receipt)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(raw))
	changed := append([]string(nil), args...)
	changed[7] = "different-primary"
	require.Error(t, run(t.Context(), changed, noEnvironment, &bytes.Buffer{}, noHost))
	version.GitCommit = strings.Repeat("b", 40)
	require.Error(t, run(t.Context(), args, noEnvironment, &bytes.Buffer{}, noHost))
}

func TestArgumentFailuresNeverTouchEnvironmentOrHost(t *testing.T) {
	_, args, receipt := fixture(t)
	for _, invalid := range [][]string{
		nil,
		append(append([]string(nil), args...), "extra"),
		append(append([]string(nil), args...), "--apply"),
		append(append([]string(nil), args...), "--receipt", receipt),
		append(append([]string(nil), args...), "--database", "original-db"),
		append(applyArgs(args, receipt), "--timeout", "0s"),
		append(applyArgs(args, receipt), "--timeout", "6m"),
		append(append([]string(nil), args...), "--all-legacy-writers-stopped-and-admission-closed"),
	} {
		require.Error(t, run(t.Context(), invalid, noEnvironment, &bytes.Buffer{}, noHost))
	}
}
