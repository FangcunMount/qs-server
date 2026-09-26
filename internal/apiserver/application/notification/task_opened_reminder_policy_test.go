package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/stretchr/testify/require"
)

type reminderCandidateReaderStub struct {
	linked     []iambridge.MiniProgramLinkedUser
	candidates []iambridge.MiniProgramRecipientCandidate
	readErr    error
	selected   []string
}

func (s *reminderCandidateReaderStub) ListLinkedUsers(context.Context, string) ([]iambridge.MiniProgramLinkedUser, error) {
	return s.linked, nil
}

func (s *reminderCandidateReaderStub) ReadCandidates(_ context.Context, _, _ string, selected []string) ([]iambridge.MiniProgramRecipientCandidate, error) {
	s.selected = selected
	return s.candidates, s.readErr
}

func TestTaskOpenedReminderStopsWhenCurrentTaskOrOneHourWindowChanges(t *testing.T) {
	opened := time.Date(2026, 9, 26, 9, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	expires := opened.Add(24 * time.Hour)
	intent := TaskOpenedReminderIntent{OrgID: 7, TaskID: "task-1", TesteeID: "testee-1", ScheduleRevision: 3, OpenAt: opened}
	base := planApp.TaskReminderState{OrgID: 7, TaskID: "task-1", TesteeID: "testee-1", Status: planDomain.TaskStatusOpened,
		ScheduleRevision: 3, OpenAt: &opened, ExpireAt: &expires, EntryURL: "https://example.invalid/?token=secret"}
	for _, tc := range []struct {
		name string
		at   time.Time
		edit func(*planApp.TaskReminderState)
		want string
	}{
		{name: "valid", at: opened.Add(59*time.Minute + 59*time.Second)},
		{name: "one hour elapsed", at: opened.Add(time.Hour), want: "reminder_window_elapsed"},
		{name: "completed", at: opened.Add(time.Minute), edit: func(s *planApp.TaskReminderState) { s.Status = planDomain.TaskStatusCompleted }, want: "task_not_opened"},
		{name: "canceled", at: opened.Add(time.Minute), edit: func(s *planApp.TaskReminderState) { s.Status = planDomain.TaskStatusCanceled }, want: "task_not_opened"},
		{name: "expired state", at: opened.Add(time.Minute), edit: func(s *planApp.TaskReminderState) { s.Status = planDomain.TaskStatusExpired }, want: "task_not_opened"},
		{name: "expiration reached", at: opened.Add(time.Minute), edit: func(s *planApp.TaskReminderState) { cutoff := opened.Add(time.Minute); s.ExpireAt = &cutoff }, want: "task_expired"},
		{name: "new schedule", at: opened.Add(time.Minute), edit: func(s *planApp.TaskReminderState) { s.ScheduleRevision++ }, want: "opening_changed"},
		{name: "different opening", at: opened.Add(time.Minute), edit: func(s *planApp.TaskReminderState) { changed := opened.Add(time.Second); s.OpenAt = &changed }, want: "opening_changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := base
			if tc.edit != nil {
				tc.edit(&state)
			}
			decision, err := EvaluateTaskOpenedReminder(intent, &state, tc.at)
			require.NoError(t, err)
			require.Equal(t, tc.want, decision.SuppressCode)
		})
	}
	wrongOrg := base
	wrongOrg.OrgID = 8
	_, err := EvaluateTaskOpenedReminder(intent, &wrongOrg, opened.Add(time.Minute))
	require.Error(t, err, "a cross-organization read must never authorize sending")
	missingEntry := base
	missingEntry.EntryURL = ""
	_, err = EvaluateTaskOpenedReminder(intent, &missingEntry, opened.Add(time.Minute))
	require.Error(t, err, "missing entry must not produce an unusable reminder")
}

func TestTaskOpenedReminderDateUsesUTCPlusEight(t *testing.T) {
	openedUTC := time.Date(2026, 9, 25, 16, 30, 0, 0, time.UTC)
	require.Equal(t, "2026.09.26", formatTaskOpenedDate(openedUTC))
}

func TestTaskOpenedReminderOnlySelectsActiveSelfRelation(t *testing.T) {
	linked := []iambridge.MiniProgramLinkedUser{
		{UserID: "parent", Relations: []string{"parent"}},
		{UserID: "other", Relations: []string{"other"}},
		{UserID: "self-b", Relations: []string{"self"}},
		{UserID: "self-a", Relations: []string{"other", "self"}},
		{UserID: "self-b", Relations: []string{"self"}},
	}
	require.Equal(t, []string{"self-a", "self-b"}, SelectSelfLinkedUsers(linked))
	resolved := []iambridge.MiniProgramRecipientCandidate{
		{UserID: "self-a", LoginIdentityID: "identity-a", Relations: []string{"self"}},
		{UserID: "self-b", LoginIdentityID: "identity-b", Relations: []string{"parent"}},
	}
	require.Equal(t, resolved[:1], SelectSelfCandidates(resolved), "relationship changes during identity lookup must be excluded")
	reader := &reminderCandidateReaderStub{linked: linked, candidates: []iambridge.MiniProgramRecipientCandidate{
		{UserID: "self-a", LoginIdentityID: "identity-a", AppID: "app-1", OpenID: "openid-a", Relations: []string{"self"}},
		{UserID: "self-b", LoginIdentityID: "identity-b", AppID: "app-1", OpenID: "openid-b", Relations: []string{"parent"}},
	}}
	candidates, err := ResolveSelfRecipientCandidates(context.Background(), reader, "profile-1", "app-1")
	require.NoError(t, err)
	require.Equal(t, []string{"self-a", "self-b"}, reader.selected)
	require.Len(t, candidates, 1)
	require.Equal(t, "identity-a", candidates[0].LoginIdentityID)
	reader.readErr = errors.New("IAM unavailable")
	_, err = ResolveSelfRecipientCandidates(context.Background(), reader, "profile-1", "app-1")
	require.ErrorContains(t, err, "IAM unavailable")
	reader.readErr = nil
	reader.candidates[0].AppID = "another-app"
	_, err = ResolveSelfRecipientCandidates(context.Background(), reader, "profile-1", "app-1")
	require.Error(t, err, "wrong AppID must fail closed")
}
