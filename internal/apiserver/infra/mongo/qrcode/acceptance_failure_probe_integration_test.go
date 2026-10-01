//go:build integration && reliable_messaging_m4

package qrcode

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"testing"
	"time"
)

func TestQRProbeClaimsOnceAcrossInstancesInRealMongo(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	now := time.Now()
	scope := testScope(now)
	qr := &recordingQR{}
	claims := db.Collection("qrcode_acceptance_failure_claims")
	makeProbe := func() *failureProbe {
		return &failureProbe{QRCodeService: qr, claims: claims, scope: scope, now: func() time.Time { return now }}
	}
	ctx := app.WithPublishedPostAction(t.Context())
	if _, err := makeProbe().GenerateQuestionnaireQRCode(ctx, "test-qr", "v1"); err == nil {
		t.Fatal("first instance must claim and fail")
	}
	if _, err := makeProbe().GenerateQuestionnaireQRCode(ctx, "test-qr", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := makeProbe().GenerateScaleQRCode(ctx, "test-scale"); err == nil {
		t.Fatal("scale must have its own one-shot claim")
	}
	if _, err := makeProbe().GenerateScaleQRCode(ctx, "test-scale"); err != nil {
		t.Fatal(err)
	}
	if qr.calls != 2 {
		t.Fatalf("external calls=%d want 2 recovery calls", qr.calls)
	}
	if n, err := claims.CountDocuments(t.Context(), bson.M{"external_call_started": false}); err != nil || n != 2 {
		t.Fatalf("claims=%d err=%v", n, err)
	}
}
