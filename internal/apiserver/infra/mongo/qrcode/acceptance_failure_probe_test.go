package qrcode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type recordingQR struct {
	app.QRCodeService
	calls int
}

func (q *recordingQR) GenerateQuestionnaireQRCode(context.Context, string, string) (string, error) {
	q.calls++
	return "qr", nil
}
func (q *recordingQR) GenerateScaleQRCode(context.Context, string) (string, error) {
	q.calls++
	return "qr", nil
}

type failingClaimStore struct {
	calls int
	err   error
}

func (s *failingClaimStore) InsertOne(context.Context, interface{}, ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
	s.calls++
	return &mongo.InsertOneResult{}, s.err
}
func testScope(now time.Time) FailureScope {
	return FailureScope{Token: "m5-qr-acceptance-test", StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute), Targets: []FailureTarget{{Kind: "questionnaire", Code: "test-qr", Version: "v1"}, {Kind: "scale", Code: "test-scale"}}}
}

func TestProbeNeverTouchesDirectRecoveryOtherIdentityOrExpiredWindow(t *testing.T) {
	now := time.Now()
	qr := &recordingQR{}
	store := &failingClaimStore{}
	p := &failureProbe{QRCodeService: qr, claims: store, scope: testScope(now), now: func() time.Time { return now }}
	ctx := context.Background()
	marked := app.WithPublishedPostAction(ctx)
	for _, c := range []struct {
		ctx           context.Context
		code, version string
	}{{ctx, "test-qr", "v1"}, {marked, "other", "v1"}, {marked, "test-qr", "v2"}} {
		if _, err := p.GenerateQuestionnaireQRCode(c.ctx, c.code, c.version); err != nil {
			t.Fatal(err)
		}
	}
	p.now = func() time.Time { return p.scope.ExpiresAt }
	if _, err := p.GenerateScaleQRCode(marked, "test-scale"); err != nil {
		t.Fatal(err)
	}
	if store.calls != 0 || qr.calls != 4 {
		t.Fatalf("claim calls=%d QR calls=%d", store.calls, qr.calls)
	}
}

func TestProbeClaimsBeforeExternalCallAndDoesNotSwallowStoreFailure(t *testing.T) {
	now := time.Now()
	qr := &recordingQR{}
	store := &failingClaimStore{}
	p := &failureProbe{QRCodeService: qr, claims: store, scope: testScope(now), now: func() time.Time { return now }}
	ctx := app.WithPublishedPostAction(context.Background())
	if _, err := p.GenerateQuestionnaireQRCode(ctx, "test-qr", "v1"); err == nil || qr.calls != 0 {
		t.Fatal("injected failure must occur before external call")
	}
	store.err = errors.New("claim database unavailable")
	if _, err := p.GenerateScaleQRCode(ctx, "test-scale"); !errors.Is(err, store.err) || qr.calls != 0 {
		t.Fatal("uncertain claim must not start external call")
	}
	store.err = mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 11000}}}
	if _, err := p.GenerateQuestionnaireQRCode(ctx, "test-qr", "v1"); err != nil || qr.calls != 1 {
		t.Fatal("existing durable claim must allow original service")
	}
}

func TestProbeConfigurationIsExplicitAndStrict(t *testing.T) {
	t.Setenv(AcceptanceFailureEnv, "")
	if _, err := WrapAcceptanceFailureFromEnv(nil, nil); err == nil {
		t.Fatal("empty explicit setting must fail")
	}
	now := time.Now()
	scope := testScope(now)
	raw, _ := json.Marshal(scope)
	if _, err := parseFailureScope(string(raw), now); err != nil {
		t.Fatal(err)
	}
	cases := []string{"", string(raw) + " {}", string(raw[:len(raw)-1]) + `,"unknown":true}`}
	for _, r := range cases {
		if _, err := parseFailureScope(r, now); err == nil {
			t.Fatalf("accepted invalid config %q", r)
		}
	}
	for _, mutate := range []func(*FailureScope){func(s *FailureScope) { s.Targets[0].Code = "*" }, func(s *FailureScope) { s.Targets[0].Version = "" }, func(s *FailureScope) { s.ExpiresAt = now.Add(11 * time.Minute) }, func(s *FailureScope) { s.Targets[1].Version = "v1" }} {
		s := testScope(now)
		mutate(&s)
		raw, _ := json.Marshal(s)
		if _, err := parseFailureScope(string(raw), now); err == nil {
			t.Fatal("accepted unbounded or ambiguous scope")
		}
	}
}

func TestUnconfiguredProbePreservesOriginalService(t *testing.T) {
	t.Setenv(AcceptanceFailureEnv, "")
	if err := os.Unsetenv(AcceptanceFailureEnv); err != nil {
		t.Fatal(err)
	}
	qr := &recordingQR{}
	got, err := WrapAcceptanceFailureFromEnv(qr, nil)
	if err != nil || got != qr {
		t.Fatal("absent configuration must retain original service without database")
	}
}
