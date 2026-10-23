//go:build integration && m3_retry_store

package transaction_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func handoffCLIRequired(t *testing.T) bool {
	t.Helper()
	value := os.Getenv("QS_M3_RETRY_HANDOFF_CLI_REQUIRED")
	require.Contains(t, []string{"", "1"}, value)
	return value == "1"
}

// The CI host builds the exact-source CLI BEFORE creating time-sensitive
// original Run/authorization fixtures. This is never a business runtime probe.
func nativeHandoffCLI(t *testing.T, binding transaction.Binding, f model.Frozen, database, envName, envValue string) map[string]bool {
	t.Helper()
	binary := os.Getenv("QS_M3_RETRY_HANDOFF_CLI_BINARY")
	require.True(t, filepath.IsAbs(binary))
	info, err := os.Lstat(binary)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular() && info.Mode().Perm()&0100 != 0)
	source := os.Getenv("QS_M3_RETRY_HANDOFF_CLI_SOURCE_SHA")
	review, err := model.ReviewHandoff(binding, source, "isolated-ci-owner", "verify original future responsibility transfer", []model.Frozen{f})
	require.NoError(t, err)
	dir := t.TempDir()
	path := filepath.Join(dir, "review.json")
	require.NoError(t, os.WriteFile(path, review.Bytes(), 0600))
	digest := review.Digest()
	base := []string{"--review-file", path, "--review-sha256", hex.EncodeToString(digest[:]), "--owner", binding.Owner, "--store-id", binding.StoreID, "--writer-epoch", binding.WriterEpoch}
	env := append(os.Environ(), "QS_RETRY_HANDOFF_MYSQL_DSN=invalid", "QS_RETRY_HANDOFF_MONGO_URI=invalid")
	command := func(args, environment []string, succeeds bool) []byte {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = environment
		// Never send private stdout/stderr, wire or connection strings to t.Log.
		out, err := cmd.Output()
		if succeeds {
			require.NoError(t, err, "original offline CLI failed; private output withheld")
		} else {
			require.Error(t, err)
			require.Empty(t, out)
		}
		return out
	}
	var verification map[string]any
	require.NoError(t, json.Unmarshal(command(base, env, true), &verification))
	require.Equal(t, "offline_review_valid", verification["status"])
	require.Equal(t, false, verification["cutover_authorized"])
	wrong := append([]string(nil), base...)
	wrong[7] = "wrong-original-store"
	command(wrong, env, false)
	wrong = append([]string(nil), base...)
	wrong[3] = strings.Repeat("0", 64)
	command(wrong, env, false)
	// Only the apply invocation receives the configured original primary.
	env = append(env, envName+"="+envValue)
	for index, status := range []string{"transferred", "already_transferred"} {
		receipt := filepath.Join(dir, []string{"first.jsonl", "repeat.jsonl"}[index])
		args := append(append([]string(nil), base...), "--apply", "--database", database, "--receipt", receipt, "--all-legacy-writers-stopped-and-admission-closed")
		var output map[string]any
		require.NoError(t, json.Unmarshal(command(args, env, true), &output))
		require.Equal(t, "handoff_complete", output["status"])
		require.Equal(t, false, output["cutover_authorized"])
		raw, err := os.ReadFile(receipt)
		require.NoError(t, err)
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		require.Len(t, lines, 2)
		var final struct {
			Status   string `json:"status"`
			Outcomes []struct {
				ScheduleID string `json:"schedule_id"`
				Status     string `json:"status"`
			} `json:"outcomes"`
			Cutover bool `json:"cutover_authorized"`
		}
		require.NoError(t, json.Unmarshal([]byte(lines[1]), &final))
		require.Equal(t, "handoff_complete", final.Status)
		require.False(t, final.Cutover)
		require.Len(t, final.Outcomes, 1)
		require.Equal(t, f.ID(), final.Outcomes[0].ScheduleID)
		require.Equal(t, status, final.Outcomes[0].Status)
		info, err := os.Stat(receipt)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
		// A repeated path cannot overwrite the first positive receipt.
		command(args, env, false)
		after, err := os.ReadFile(receipt)
		require.NoError(t, err)
		require.True(t, string(raw) == string(after), "existing private receipt changed")
	}
	return map[string]bool{
		"offline_cli_exact_source_original_primary":            true,
		"offline_cli_default_without_database_binding":         true,
		"offline_cli_wrong_binding_and_digest_refused":         true,
		"offline_cli_first_transfer_and_same_plan_idempotence": true,
		"offline_cli_private_receipt_preserved":                true,
		"offline_cli_does_not_authorize_cutover":               true,
	}
}
