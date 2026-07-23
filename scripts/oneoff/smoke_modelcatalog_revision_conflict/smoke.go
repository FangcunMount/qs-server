package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type roundEvidence struct {
	Round             int      `json:"round"`
	Successes         int      `json:"successes"`
	RevisionConflicts int      `json:"revision_conflicts"`
	Unexpected        int      `json:"unexpected"`
	ConflictMessages  []string `json:"conflict_messages,omitempty"`
	Errors            []string `json:"errors,omitempty"`
}

type targetEvidence struct {
	Kind              targetKind      `json:"kind"`
	Code              string          `json:"code"`
	Status            string          `json:"status"`
	WorkingVersion    string          `json:"working_version,omitempty"`
	ActiveVersion     string          `json:"active_version,omitempty"`
	Rounds            []roundEvidence `json:"rounds,omitempty"`
	Successes         int             `json:"successes"`
	RevisionConflicts int             `json:"revision_conflicts"`
	Unexpected        int             `json:"unexpected"`
	ConflictObserved  bool            `json:"conflict_observed"`
	RestoreAttempted  bool            `json:"restore_attempted"`
	RestorePassed     bool            `json:"restore_passed"`
	Passed            bool            `json:"passed"`
	Error             string          `json:"error,omitempty"`
}

func newPreflightEvidence(snapshot targetSnapshot) targetEvidence {
	return targetEvidence{
		Kind: snapshot.Spec.Kind, Code: snapshot.Spec.Code, Status: snapshot.Status,
		WorkingVersion: snapshot.ReleaseState.WorkingVersion,
		ActiveVersion:  snapshot.ReleaseState.ActiveVersion,
	}
}

func validateDedicatedDraft(snapshot targetSnapshot, requiredPrefix string) error {
	if !strings.HasPrefix(snapshot.Spec.Code, requiredPrefix) {
		return fmt.Errorf("refusing target without required dedicated prefix %q", requiredPrefix)
	}
	if strings.ToLower(strings.TrimSpace(snapshot.Status)) != "draft" {
		return fmt.Errorf("refusing non-draft target with status %q", snapshot.Status)
	}
	if working := strings.ToLower(strings.TrimSpace(snapshot.ReleaseState.WorkingStatus)); working != "" && working != "draft" {
		return fmt.Errorf("refusing target with working_status %q", snapshot.ReleaseState.WorkingStatus)
	}
	if strings.TrimSpace(snapshot.ReleaseState.ActiveVersion) != "" {
		return fmt.Errorf("refusing target with active_version %q", snapshot.ReleaseState.ActiveVersion)
	}
	if online := strings.ToLower(strings.TrimSpace(snapshot.ReleaseState.OnlineStatus)); online == "online" || online == "published" || online == "active" {
		return fmt.Errorf("refusing online target with online_status %q", snapshot.ReleaseState.OnlineStatus)
	}
	if strings.TrimSpace(snapshot.title()) == "" {
		return fmt.Errorf("refusing target with empty title")
	}
	return nil
}

func exerciseTarget(ctx context.Context, client *restClient, original targetSnapshot, concurrency, maxRounds int) targetEvidence {
	evidence := newPreflightEvidence(original)
	evidence.Rounds = make([]roundEvidence, 0, maxRounds)

	for round := 1; round <= maxRounds; round++ {
		result := runConcurrentRound(ctx, client, original, concurrency, round)
		evidence.Rounds = append(evidence.Rounds, result)
		evidence.Successes += result.Successes
		evidence.RevisionConflicts += result.RevisionConflicts
		evidence.Unexpected += result.Unexpected
		if result.RevisionConflicts > 0 || result.Unexpected > 0 || ctx.Err() != nil {
			break
		}
	}
	evidence.ConflictObserved = evidence.Successes > 0 && evidence.RevisionConflicts > 0
	if !evidence.ConflictObserved {
		evidence.Error = "no deployed revision conflict observed; REST concurrency is probabilistic, so rerun or increase --concurrency/--rounds"
	}
	if evidence.Unexpected > 0 {
		evidence.Error = "unexpected HTTP result observed during concurrent writes"
	}
	if ctx.Err() != nil {
		evidence.Error = fmt.Sprintf("smoke context ended: %v", ctx.Err())
	}

	evidence.RestoreAttempted = true
	restoreCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	restore := client.update(restoreCtx, original, original.originalRequest())
	if !isSuccess(restore) {
		evidence.Error = appendError(evidence.Error, fmt.Sprintf("restore PUT returned %s", describeHTTPResult(restore)))
		return evidence
	}
	restored, err := client.getSnapshot(restoreCtx, original.Spec)
	if err != nil {
		evidence.Error = appendError(evidence.Error, fmt.Sprintf("restore verification GET failed: %v", err))
		return evidence
	}
	if !original.basicInfoEquals(restored) {
		evidence.Error = appendError(evidence.Error, "restore verification found changed basic-info fields")
		return evidence
	}
	evidence.RestorePassed = true
	evidence.Passed = evidence.ConflictObserved && evidence.Unexpected == 0
	return evidence
}

func runConcurrentRound(ctx context.Context, client *restClient, original targetSnapshot, concurrency, round int) roundEvidence {
	evidence := roundEvidence{Round: round}
	results := make(chan httpResult, concurrency)
	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(concurrency)
	done.Add(concurrency)

	for index := 0; index < concurrency; index++ {
		go func(writer int) {
			defer done.Done()
			ready.Done()
			<-start
			title := fmt.Sprintf("revision-smoke-%d-%d-%d-%s", time.Now().UTC().UnixMilli(), round, writer, randomHex(3))
			results <- client.update(ctx, original, original.requestWithTitle(title))
		}(index)
	}
	ready.Wait()
	close(start)
	done.Wait()
	close(results)

	seenMessages := make(map[string]struct{})
	for result := range results {
		switch {
		case isSuccess(result):
			evidence.Successes++
		case isRevisionConflict(result):
			evidence.RevisionConflicts++
			message := truncate(result.Message, 500)
			if _, exists := seenMessages[message]; !exists {
				seenMessages[message] = struct{}{}
				evidence.ConflictMessages = append(evidence.ConflictMessages, message)
			}
		default:
			evidence.Unexpected++
			evidence.Errors = append(evidence.Errors, describeHTTPResult(result))
		}
	}
	return evidence
}

func isSuccess(result httpResult) bool {
	return result.Err == nil && result.StatusCode == http.StatusOK && result.Code == 0
}

func isRevisionConflict(result httpResult) bool {
	message := strings.ToLower(result.Message)
	return result.Err == nil && result.StatusCode == http.StatusConflict &&
		strings.Contains(message, "revision conflict") && strings.Contains(message, "refresh and retry")
}

func describeHTTPResult(result httpResult) string {
	if result.Err != nil {
		return result.Err.Error()
	}
	return fmt.Sprintf("HTTP %d code=%d message=%q", result.StatusCode, result.Code, truncate(result.Message, 500))
}

func appendError(current, addition string) string {
	if current == "" {
		return addition
	}
	return current + "; " + addition
}
