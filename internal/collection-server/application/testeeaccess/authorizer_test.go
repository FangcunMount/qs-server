package testeeaccess

import (
	"context"
	"errors"
	"testing"

	testeeapp "github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testeeReaderStub struct {
	testee *testeeapp.TesteeResponse
	err    error
	calls  int
}

func (s *testeeReaderStub) GetTestee(context.Context, uint64) (*testeeapp.TesteeResponse, error) {
	s.calls++
	return s.testee, s.err
}

type profileLinkStub struct {
	enabled bool
	allowed bool
	err     error
	calls   int
}

func (s *profileLinkStub) IsEnabled() bool { return s != nil && s.enabled }
func (s *profileLinkStub) HasActiveProfileLink(context.Context, string, string) (bool, error) {
	s.calls++
	return s.allowed, s.err
}

func TestAuthorizer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		userID    string
		testeeID  uint64
		reader    *testeeReaderStub
		links     *profileLinkStub
		wantError error
	}{
		{name: "missing user", testeeID: 7, reader: &testeeReaderStub{}, links: &profileLinkStub{enabled: true}, wantError: ErrAccessDenied},
		{name: "missing testee id", userID: "9", reader: &testeeReaderStub{}, links: &profileLinkStub{enabled: true}, wantError: ErrAccessDenied},
		{name: "reader missing", userID: "9", testeeID: 7, links: &profileLinkStub{enabled: true}, wantError: ErrAccessUnavailable},
		{name: "links missing", userID: "9", testeeID: 7, reader: &testeeReaderStub{}, wantError: ErrAccessUnavailable},
		{name: "links disabled", userID: "9", testeeID: 7, reader: &testeeReaderStub{}, links: &profileLinkStub{}, wantError: ErrAccessUnavailable},
		{name: "testee not found", userID: "9", testeeID: 7, reader: &testeeReaderStub{err: status.Error(codes.NotFound, "secret missing detail")}, links: &profileLinkStub{enabled: true}, wantError: ErrAccessDenied},
		{name: "testee lookup unavailable", userID: "9", testeeID: 7, reader: &testeeReaderStub{err: status.Error(codes.Unavailable, "actor down")}, links: &profileLinkStub{enabled: true}, wantError: ErrAccessUnavailable},
		{name: "nil testee", userID: "9", testeeID: 7, reader: &testeeReaderStub{}, links: &profileLinkStub{enabled: true}, wantError: ErrAccessDenied},
		{name: "profile missing", userID: "9", testeeID: 7, reader: &testeeReaderStub{testee: &testeeapp.TesteeResponse{}}, links: &profileLinkStub{enabled: true}, wantError: ErrAccessDenied},
		{name: "link denied", userID: "9", testeeID: 7, reader: &testeeReaderStub{testee: &testeeapp.TesteeResponse{IAMProfileID: "p-7"}}, links: &profileLinkStub{enabled: true}, wantError: ErrAccessDenied},
		{name: "link unavailable", userID: "9", testeeID: 7, reader: &testeeReaderStub{testee: &testeeapp.TesteeResponse{IAMProfileID: "p-7"}}, links: &profileLinkStub{enabled: true, err: errors.New("iam down")}, wantError: ErrAccessUnavailable},
		{name: "allowed", userID: "9", testeeID: 7, reader: &testeeReaderStub{testee: &testeeapp.TesteeResponse{IAMProfileID: "p-7"}}, links: &profileLinkStub{enabled: true, allowed: true}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := NewAuthorizer(tt.reader, tt.links).Authorize(context.Background(), tt.userID, tt.testeeID)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Authorize() error = %v, want %v", err, tt.wantError)
			}
			if tt.userID == "" || tt.testeeID == 0 || tt.reader == nil || tt.links == nil || !tt.links.enabled {
				if tt.reader != nil && tt.reader.calls != 0 {
					t.Fatalf("reader calls = %d, want 0", tt.reader.calls)
				}
				if tt.links != nil && tt.links.calls != 0 {
					t.Fatalf("link calls = %d, want 0", tt.links.calls)
				}
			}
		})
	}
}
