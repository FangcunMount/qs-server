//go:build integration && reliable_messaging_m4

package qrcode

import (
	"context"
	"encoding/json"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type concurrentQR struct {
	app.QRCodeService
	calls atomic.Int64
}

func (q *concurrentQR) GenerateQuestionnaireQRCode(context.Context, string, string) (string, error) {
	q.calls.Add(1)
	return "qr", nil
}
func (q *concurrentQR) GenerateScaleQRCode(context.Context, string) (string, error) {
	q.calls.Add(1)
	return "qr", nil
}

func TestQRProbeClaimsOnceAcrossInstancesInRealMongo(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	now := time.Now()
	scope := testScope(now)
	claims := db.Collection("qrcode_acceptance_failure_claims")
	raw, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	mongodbtest.RunTestProcess(t, db, "TestQRProbeRestartProcess", "QS_M7_QR_SCOPE="+string(raw), "QS_M7_CLAIM_PHASE=initial")
	for range 2 {
		mongodbtest.RunTestProcess(t, db, "TestQRProbeRestartProcess", "QS_M7_QR_SCOPE="+string(raw))
	}
	if n, err := claims.CountDocuments(t.Context(), bson.M{"external_call_started": false}); err != nil || n != 2 {
		t.Fatalf("claims=%d err=%v", n, err)
	}
}

func TestQRProbeRestartProcess(t *testing.T) {
	raw := os.Getenv("QS_M7_QR_SCOPE")
	if raw == "" {
		return
	}
	var scope FailureScope
	if err := json.Unmarshal([]byte(raw), &scope); err != nil {
		t.Fatal(err)
	}
	db := mongodbtest.ProcessDatabase(t)
	if os.Getenv("QS_M7_CLAIM_PHASE") == "initial" {
		claims := db.Collection("qrcode_acceptance_failure_claims")
		now := time.Now()
		qr := &concurrentQR{}
		ctx := withPublishedPostAction(t.Context())
		const contenders = 32
		for _, kind := range []string{"questionnaire", "scale"} {
			var wg sync.WaitGroup
			failures := make(chan error, contenders)
			for range contenders {
				wg.Add(1)
				go func() {
					defer wg.Done()
					probe := &failureProbe{QRCodeService: qr, claims: claims, scope: scope, now: func() time.Time { return now }}
					var err error
					if kind == "questionnaire" {
						_, err = probe.GenerateQuestionnaireQRCode(ctx, "test-qr", "v1")
					} else {
						_, err = probe.GenerateScaleQRCode(ctx, "test-scale")
					}
					failures <- err
				}()
			}
			wg.Wait()
			close(failures)
			claimed := 0
			for err := range failures {
				if err != nil {
					if !strings.HasPrefix(err.Error(), "controlled QR acceptance failure before external call:") {
						t.Fatalf("unexpected claim failure: %v", err)
					}
					claimed++
				}
			}
			if claimed != 1 {
				t.Fatalf("%s failures=%d want1", kind, claimed)
			}
		}
		if qr.calls.Load() != 2*(contenders-1) {
			t.Fatalf("external calls=%d", qr.calls.Load())
		}
		return
	}
	qr := &concurrentQR{}
	probe := &failureProbe{QRCodeService: qr, claims: db.Collection("qrcode_acceptance_failure_claims"), scope: scope, now: time.Now}
	ctx := withPublishedPostAction(t.Context())
	if _, err := probe.GenerateQuestionnaireQRCode(ctx, "test-qr", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.GenerateScaleQRCode(ctx, "test-scale"); err != nil {
		t.Fatal(err)
	}
	if qr.calls.Load() != 2 {
		t.Fatalf("restarted external calls=%d want2", qr.calls.Load())
	}
	if n, err := db.Collection("qrcode_acceptance_failure_claims").CountDocuments(t.Context(), bson.M{"external_call_started": false}); err != nil || n != 2 {
		t.Fatalf("restarted claims=%d err=%v", n, err)
	}
}
