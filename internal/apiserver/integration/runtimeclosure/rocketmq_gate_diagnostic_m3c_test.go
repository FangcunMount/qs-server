//go:build integration && m3_host_runtime

package runtimeclosure

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type m3cGateDiagnostic struct {
	Worker    string         `json:"worker"`
	Counts    map[string]int `json:"counts"`
	Truncated bool           `json:"truncated"`
}

var m3cGateTextMessage = regexp.MustCompile(`^(?:time=[^ ]+ )?level=(?:DEBUG|INFO|WARN|ERROR) msg="([^"]*)"`)

// Only fixed messages become counters. IDs, payloads, keys and error text stay
// in the private child log, including when a field contains a known phrase.
func m3cProcessingGateDiagnostics(input io.Reader, replacement bool) m3cGateDiagnostic {
	result := m3cGateDiagnostic{Worker: "first", Counts: map[string]int{}}
	if replacement {
		result.Worker = "replacement"
	}
	classes := map[string]string{
		"answersheet processing deferred for retry":   "contention_retry",
		"answersheet processing waiting for lease":    "wait_started",
		"answersheet processing lease wait complete":  "wait_completed",
		"answersheet processing lease wait cancelled": "wait_cancelled",
		"answersheet processing lease wait exhausted": "wait_exhausted",
		"answersheet assessment ensure failed":        "ensure_failed",
		"answersheet submitted event processed":       "processed",
	}
	for _, class := range classes {
		result.Counts[class] = 0
	}
	const limit = 4 * 1024 * 1024
	scanner := bufio.NewScanner(io.LimitReader(input, limit+1))
	scanner.Buffer(make([]byte, 4096), 65536)
	examined := 0
	for scanner.Scan() {
		line := scanner.Text()
		examined += len(line) + 1
		if examined > limit {
			result.Truncated = true
			break
		}
		var record struct {
			Msg     string `json:"msg"`
			Message string `json:"message"`
		}
		message := ""
		if json.Unmarshal([]byte(line), &record) == nil {
			message = record.Msg
			if message == "" {
				message = record.Message
			}
		} else if match := m3cGateTextMessage.FindStringSubmatch(line); len(match) == 2 {
			message = match[1]
		}
		if class, ok := classes[message]; ok {
			if result.Counts[class] >= 10000 {
				result.Truncated = true
				break
			}
			result.Counts[class]++
		}
	}
	result.Truncated = result.Truncated || scanner.Err() != nil
	return result
}

func TestM3CGateDiagnosticUsesOnlyFixedMessages(t *testing.T) {
	raw := `{"msg":"answersheet processing deferred for retry","event_id":"PRIVATE"}
{"msg":"other","payload":"answersheet processing deferred for retry"}
time=2026-10-30T00:00:00Z level=INFO msg="answersheet assessment ensure failed" error="PRIVATE"
{"message":"answersheet submitted event processed"}
`
	value := m3cProcessingGateDiagnostics(strings.NewReader(raw), true)
	require.Equal(t, 1, value.Counts["contention_retry"])
	require.Equal(t, 1, value.Counts["ensure_failed"])
	require.Equal(t, 1, value.Counts["processed"])
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE")
	require.False(t, value.Truncated)
	require.Equal(t, "replacement", value.Worker)
}
func TestM3CGateDiagnosticIsBounded(t *testing.T) {
	for _, raw := range []string{strings.Repeat("x", 65537), strings.Repeat("{}\n", 1500000), strings.Repeat("{\"msg\":\"answersheet processing deferred for retry\"}\n", 10001)} {
		require.True(t, m3cProcessingGateDiagnostics(strings.NewReader(raw), false).Truncated)
	}
}
