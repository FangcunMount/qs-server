package delegatedsubject

import (
	"context"
	"testing"
	"time"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"google.golang.org/grpc/metadata"
)

func testOptions(current, previous string) *Options {
	return &Options{
		Enabled:     true,
		CurrentKey:  current,
		PreviousKey: previous,
		TTL:         time.Minute,
	}
}

func TestSignerVerifierRoundTrip(t *testing.T) {
	signer, err := NewSignerFromOptions(testOptions("current-key", "previous-key"))
	if err != nil || signer == nil {
		t.Fatalf("NewSignerFromOptions() = %v, %v", signer, err)
	}
	verifier, err := NewVerifierFromOptions(testOptions("current-key", "previous-key"))
	if err != nil || verifier == nil {
		t.Fatalf("NewVerifierFromOptions() = %v, %v", verifier, err)
	}

	raw, err := signer.Sign(SignInput{UserID: "42", TesteeID: 7, OrgID: 9, Purpose: PurposeGetAssessmentReport, TTL: time.Minute})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	token, err := verifier.Verify(raw, PurposeGetAssessmentReport, 7)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if token.UserID != "42" || token.OrgID != 9 {
		t.Fatalf("token = %#v", token)
	}
}

func TestVerifierAcceptsPreviousKeyDuringRotation(t *testing.T) {
	oldSigner, err := NewSignerFromOptions(testOptions("old-key", ""))
	if err != nil {
		t.Fatalf("NewSignerFromOptions() error = %v", err)
	}
	raw, err := oldSigner.Sign(SignInput{UserID: "42", TesteeID: 7, Purpose: PurposeGetAssessmentReport, TTL: time.Minute})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	verifier, err := NewVerifierFromOptions(testOptions("new-key", "old-key"))
	if err != nil {
		t.Fatalf("NewVerifierFromOptions() error = %v", err)
	}
	if _, err := verifier.Verify(raw, PurposeGetAssessmentReport, 7); err != nil {
		t.Fatalf("Verify() with previous key error = %v", err)
	}
}

func TestEnabledOptionsRequireCurrentKeyAndPositiveTTL(t *testing.T) {
	tests := []struct {
		name string
		opts *Options
		ok   bool
	}{
		{name: "nil", opts: nil, ok: true},
		{name: "disabled", opts: &Options{}, ok: true},
		{name: "previous only", opts: &Options{Enabled: true, PreviousKey: "old", TTL: time.Minute}},
		{name: "missing ttl", opts: &Options{Enabled: true, CurrentKey: "current"}},
		{name: "valid", opts: &Options{Enabled: true, CurrentKey: "current", TTL: DefaultTTL}, ok: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if tc.ok && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestVerifierRejectsTamperedTestee(t *testing.T) {
	signer, _ := NewSignerFromOptions(testOptions("key-a", ""))
	verifier, _ := NewVerifierFromOptions(testOptions("key-a", ""))
	raw, err := signer.Sign(SignInput{UserID: "42", TesteeID: 7, Purpose: PurposeGetAssessmentReport, TTL: time.Minute})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if _, err := verifier.Verify(raw, PurposeGetAssessmentReport, 8); err == nil {
		t.Fatal("Verify() with mismatched testee error = nil")
	}
}

func TestVerifierRejectsExpiredToken(t *testing.T) {
	verifier, _ := NewVerifierFromOptions(testOptions("key-a", ""))
	nonce, err := newNonce()
	if err != nil {
		t.Fatalf("newNonce() error = %v", err)
	}
	raw, err := encodeToken(tokenPayload{
		UserID:   "42",
		TesteeID: 7,
		Purpose:  PurposeGetAssessmentReport,
		Exp:      time.Now().Add(-time.Minute).Unix(),
		Nonce:    nonce,
	}, []byte("key-a"))
	if err != nil {
		t.Fatalf("encodeToken() error = %v", err)
	}
	if _, err := verifier.Verify(raw, PurposeGetAssessmentReport, 7); err == nil {
		t.Fatal("Verify() expired token error = nil")
	}
}

func TestVerifierRejectsBadSignature(t *testing.T) {
	verifier, _ := NewVerifierFromOptions(testOptions("key-a", ""))
	if _, err := verifier.Verify("payload.deadbeef", PurposeGetAssessmentReport, 7); err == nil {
		t.Fatal("Verify() bad signature error = nil")
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	signer, _ := NewSignerFromOptions(testOptions("key-a", ""))
	verifier, _ := NewVerifierFromOptions(testOptions("key-a", ""))
	base := context.WithValue(context.Background(), pkgmiddleware.UserClaimsContextKey{}, &pkgmiddleware.UserClaims{UserID: "42", OrgID: "9"})
	outgoing, err := AppendToOutgoingContext(base, signer, SignInput{UserID: "42", TesteeID: 7, OrgID: 9, Purpose: PurposeGetAssessmentReport, TTL: time.Minute})
	if err != nil {
		t.Fatalf("AppendToOutgoingContext() error = %v", err)
	}
	md, _ := metadata.FromOutgoingContext(outgoing)
	incoming := metadata.NewIncomingContext(context.Background(), md)
	token, err := FromIncomingContext(incoming, verifier, PurposeGetAssessmentReport, 7)
	if err != nil {
		t.Fatalf("FromIncomingContext() error = %v", err)
	}
	if token.UserID != "42" {
		t.Fatalf("token user = %q", token.UserID)
	}
}

func TestAllowWorkloadRejectsUnknownCaller(t *testing.T) {
	verifier, _ := NewVerifierFromOptions(testOptions("key-a", ""))
	if err := verifier.AllowWorkload("qs-worker.svc"); err == nil {
		t.Fatal("AllowWorkload() error = nil, want untrusted workload")
	}
}
