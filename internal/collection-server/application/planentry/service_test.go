package planentry

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
)

type entryReader struct {
	entry *Entry
	err   error
}

func (r entryReader) ResolveTaskEntry(context.Context, string, string) (*Entry, error) {
	return r.entry, r.err
}

type accessProbe struct {
	called   bool
	userID   string
	testeeID uint64
	err      error
}

func (a *accessProbe) Authorize(_ context.Context, userID string, testeeID uint64) error {
	a.called, a.userID, a.testeeID = true, userID, testeeID
	return a.err
}

func TestResolveRequiresActiveUserTesteeRelationship(t *testing.T) {
	entry := &Entry{TaskID: "task-1", TesteeID: "42", ScaleCode: "scale-1"}
	for _, tc := range []struct {
		name       string
		userID     string
		reader     entryReader
		accessErr  error
		wantErr    error
		wantAccess bool
	}{
		{name: "allowed", userID: "user-1", reader: entryReader{entry: entry}, wantAccess: true},
		{name: "unrelated user", userID: "user-2", reader: entryReader{entry: entry}, accessErr: testeeaccess.ErrAccessDenied, wantErr: testeeaccess.ErrAccessDenied, wantAccess: true},
		{name: "iam unavailable", userID: "user-1", reader: entryReader{entry: entry}, accessErr: testeeaccess.ErrAccessUnavailable, wantErr: testeeaccess.ErrAccessUnavailable, wantAccess: true},
		{name: "anonymous", reader: entryReader{entry: entry}, wantErr: testeeaccess.ErrAccessDenied},
		{name: "invalid testee", userID: "user-1", reader: entryReader{entry: &Entry{TesteeID: "bad"}}, wantErr: ErrInvalidEntry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			access := &accessProbe{err: tc.accessErr}
			got, err := NewService(tc.reader, access).Resolve(t.Context(), tc.userID, "task-1", "token")
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("Resolve error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && got != nil {
				t.Fatalf("denied request leaked entry: %+v", got)
			}
			if tc.wantErr == nil && got != entry {
				t.Fatalf("authorized entry = %+v", got)
			}
			if access.called != tc.wantAccess {
				t.Fatalf("access called = %v, want %v", access.called, tc.wantAccess)
			}
			if tc.wantAccess && (access.userID != tc.userID || access.testeeID != 42) {
				t.Fatalf("access argument user=%q testee=%d", access.userID, access.testeeID)
			}
		})
	}
}
