package grpcclient

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"google.golang.org/grpc/metadata"
)

func TestParticipantReportClientAttachDelegatedSubject(t *testing.T) {
	signer, err := delegatedsubject.NewSignerFromOptions(&delegatedsubject.Options{
		Enabled:    true,
		CurrentKey: "client-test-key",
		TTL:        time.Minute,
	})
	if err != nil {
		t.Fatalf("NewSignerFromOptions() error = %v", err)
	}
	client := &ParticipantReportClient{signer: signer}
	ctx := context.WithValue(context.Background(), pkgmiddleware.UserClaimsContextKey{}, &pkgmiddleware.UserClaims{UserID: "42", OrgID: "9"})
	outgoing, err := client.attachDelegatedSubject(ctx, 7, delegatedsubject.PurposeGetAssessmentReport)
	if err != nil {
		t.Fatalf("attachDelegatedSubject() error = %v", err)
	}
	md, ok := metadata.FromOutgoingContext(outgoing)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	values := md.Get(delegatedsubject.MetadataKey)
	if len(values) == 0 || values[0] == "" {
		t.Fatalf("metadata %q missing, got %#v", delegatedsubject.MetadataKey, md)
	}
}
