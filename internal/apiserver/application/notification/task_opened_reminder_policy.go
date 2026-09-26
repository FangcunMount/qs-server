package notification

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

// TaskOpenedReminderWindow is the product deadline for starting an external
// notification call. Delayed events must not create stale reminders.
const TaskOpenedReminderWindow = time.Hour
const maxSelfRecipientUsers = 20

// TaskOpenedReminderIntent contains the immutable facts needed to compare an
// Outbox reminder with the current Task. It deliberately excludes the entry URL.
type TaskOpenedReminderIntent struct {
	OrgID            int64
	TaskID           string
	TesteeID         string
	ScheduleRevision uint32
	OpenAt           time.Time
}

// ReminderTaskDecision is checked before each recipient's external call.
// Retryable errors represent missing or inconsistent facts, not permission to
// send; a nonempty suppression code is a final decision for an unsent row.
type ReminderTaskDecision struct {
	SuppressCode string
}

func EvaluateTaskOpenedReminder(
	intent TaskOpenedReminderIntent, task *planApp.TaskReminderState, now time.Time,
) (ReminderTaskDecision, error) {
	if intent.OrgID <= 0 || intent.TaskID == "" || intent.TesteeID == "" || intent.OpenAt.IsZero() || now.IsZero() {
		return ReminderTaskDecision{}, fmt.Errorf("complete reminder intent and evaluation time are required")
	}
	if task == nil {
		return ReminderTaskDecision{}, fmt.Errorf("current task state is unavailable")
	}
	if task.OrgID != intent.OrgID || task.TaskID != intent.TaskID || task.TesteeID != intent.TesteeID {
		return ReminderTaskDecision{}, fmt.Errorf("current task does not match reminder scope")
	}
	if task.ScheduleRevision != intent.ScheduleRevision || task.OpenAt == nil || !task.OpenAt.Equal(intent.OpenAt) {
		return ReminderTaskDecision{SuppressCode: "opening_changed"}, nil
	}
	if task.Status != planDomain.TaskStatusOpened {
		return ReminderTaskDecision{SuppressCode: "task_not_opened"}, nil
	}
	if task.ExpireAt == nil || task.ExpireAt.IsZero() {
		return ReminderTaskDecision{}, fmt.Errorf("opened task has no expiration time")
	}
	if !now.Before(*task.ExpireAt) {
		return ReminderTaskDecision{SuppressCode: "task_expired"}, nil
	}
	if !now.Before(intent.OpenAt.Add(TaskOpenedReminderWindow)) {
		return ReminderTaskDecision{SuppressCode: "reminder_window_elapsed"}, nil
	}
	if now.Before(intent.OpenAt) {
		return ReminderTaskDecision{}, fmt.Errorf("task opening time is in the future")
	}
	if strings.TrimSpace(task.EntryURL) == "" {
		return ReminderTaskDecision{}, fmt.Errorf("opened task has no entry URL")
	}
	return ReminderTaskDecision{}, nil
}

// SelectSelfLinkedUsers applies the approved recipient rule before resolving
// AppID-scoped login identities. Multiple active SELF links are retained;
// family and OTHER links never grant reminder eligibility.
func SelectSelfLinkedUsers(linked []iambridge.MiniProgramLinkedUser) []string {
	selected := make(map[string]struct{})
	for _, user := range linked {
		id := strings.TrimSpace(user.UserID)
		if id != "" && slices.Contains(user.Relations, "self") {
			selected[id] = struct{}{}
		}
	}
	users := make([]string, 0, len(selected))
	for id := range selected {
		users = append(users, id)
	}
	slices.Sort(users)
	return users
}

// SelectSelfCandidates rechecks the relation after IAM resolves identities.
// A link revoked or changed between the two reads cannot be used to send.
func SelectSelfCandidates(candidates []iambridge.MiniProgramRecipientCandidate) []iambridge.MiniProgramRecipientCandidate {
	selected := make([]iambridge.MiniProgramRecipientCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if slices.Contains(candidate.Relations, "self") {
			selected = append(selected, candidate)
		}
	}
	return selected
}

// ResolveSelfRecipientCandidates reads active relationship facts first, then
// only the selected users' AppID-scoped identities. No family fallback is
// permitted. Missing identities are an empty result; IAM failures are errors.
func ResolveSelfRecipientCandidates(
	ctx context.Context, reader iambridge.MiniProgramRecipientCandidateReader, profileID, appID string,
) ([]iambridge.MiniProgramRecipientCandidate, error) {
	if reader == nil || strings.TrimSpace(profileID) == "" || strings.TrimSpace(appID) == "" {
		return nil, fmt.Errorf("recipient reader, profile ID, and AppID are required")
	}
	linked, err := reader.ListLinkedUsers(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("read linked users for reminder: %w", err)
	}
	users := SelectSelfLinkedUsers(linked)
	if len(users) == 0 {
		return []iambridge.MiniProgramRecipientCandidate{}, nil
	}
	if len(users) > maxSelfRecipientUsers {
		return nil, fmt.Errorf("SELF recipient count exceeds IAM lookup limit")
	}
	candidates, err := reader.ReadCandidates(ctx, profileID, appID, users)
	if err != nil {
		return nil, fmt.Errorf("read SELF recipient identities: %w", err)
	}
	allowed := make(map[string]struct{}, len(users))
	for _, userID := range users {
		allowed[userID] = struct{}{}
	}
	selected := SelectSelfCandidates(candidates)
	for _, candidate := range selected {
		if _, ok := allowed[candidate.UserID]; !ok || candidate.AppID != appID || candidate.LoginIdentityID == "" || candidate.OpenID == "" {
			return nil, fmt.Errorf("IAM returned a recipient outside selected SELF identities and AppID")
		}
	}
	return selected, nil
}
