package qrcode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const AcceptanceFailureEnv = "QS_M5_ACCEPTANCE_QRCODE_FAILURE"

type FailureTarget struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Version string `json:"version"`
}

type FailureScope struct {
	Token     string          `json:"token"`
	StartsAt  time.Time       `json:"starts_at"`
	ExpiresAt time.Time       `json:"expires_at"`
	Targets   []FailureTarget `json:"targets"`
}

type claimInserter interface {
	InsertOne(context.Context, interface{}, ...*options.InsertOneOptions) (*mongo.InsertOneResult, error)
}

type failureProbe struct {
	app.QRCodeService
	claims claimInserter
	scope  FailureScope
	now    func() time.Time
}

// WrapAcceptanceFailureFromEnv preserves the original service unless explicitly
// configured. A durable unique claim prevents another failure after restart.
func WrapAcceptanceFailureFromEnv(service app.QRCodeService, db *mongo.Database) (app.QRCodeService, error) {
	raw, configured := os.LookupEnv(AcceptanceFailureEnv)
	if !configured {
		return service, nil
	}
	scope, err := parseFailureScope(raw, time.Now())
	if err != nil {
		return nil, err
	}
	if service == nil || db == nil {
		return nil, fmt.Errorf("QR acceptance probe requires QR service and Mongo database")
	}
	return &failureProbe{QRCodeService: service, claims: db.Collection("qrcode_acceptance_failure_claims"), scope: scope, now: time.Now}, nil
}

func parseFailureScope(raw string, now time.Time) (FailureScope, error) {
	var s FailureScope
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, fmt.Errorf("decode QR acceptance scope: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return s, fmt.Errorf("QR acceptance scope must be one JSON object")
	}
	if len(s.Token) < 16 || len(s.Token) > 80 || strings.Trim(s.Token, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") != "" {
		return s, fmt.Errorf("QR acceptance token must be 16-80 alphanumeric/hyphen characters")
	}
	if s.StartsAt.IsZero() || s.ExpiresAt.IsZero() || !s.StartsAt.Before(s.ExpiresAt) || s.ExpiresAt.Sub(s.StartsAt) > 10*time.Minute || !s.ExpiresAt.After(now) || s.ExpiresAt.After(now.Add(10*time.Minute)) {
		return s, fmt.Errorf("QR acceptance window must be active/upcoming and at most ten minutes")
	}
	if len(s.Targets) < 1 || len(s.Targets) > 2 {
		return s, fmt.Errorf("QR acceptance requires one or two exact targets")
	}
	kinds := map[string]bool{}
	for _, t := range s.Targets {
		if (t.Kind != "questionnaire" && t.Kind != "scale") || kinds[t.Kind] || len(t.Code) == 0 || len(t.Code) > 128 || strings.Trim(t.Code, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
			return s, fmt.Errorf("QR acceptance requires distinct kinds and exact non-wildcard codes")
		}
		if (t.Kind == "questionnaire" && (strings.TrimSpace(t.Version) == "" || strings.TrimSpace(t.Version) != t.Version || len(t.Version) > 128)) || (t.Kind == "scale" && t.Version != "") {
			return s, fmt.Errorf("questionnaire requires exact version; scale has a code-only contract")
		}
		kinds[t.Kind] = true
	}
	return s, nil
}

func (p *failureProbe) before(ctx context.Context, kind, code, version string) error {
	now := p.now()
	if !app.IsPublishedPostAction(ctx) || now.Before(p.scope.StartsAt) || !now.Before(p.scope.ExpiresAt) {
		return nil
	}
	for _, target := range p.scope.Targets {
		if target.Kind != kind || target.Code != code || target.Version != version {
			continue
		}
		_, err := p.claims.InsertOne(ctx, bson.M{"_id": p.scope.Token + ":" + kind, "kind": kind, "code": code, "version": version, "claimed_at": now.UTC(), "expires_at": p.scope.ExpiresAt.UTC(), "external_call_started": false})
		if mongo.IsDuplicateKeyError(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("persist QR acceptance claim: %w", err)
		}
		return fmt.Errorf("controlled QR acceptance failure before external call: %s/%s/%s", kind, code, version)
	}
	return nil
}

func (p *failureProbe) GenerateQuestionnaireQRCode(ctx context.Context, code, version string) (string, error) {
	if err := p.before(ctx, "questionnaire", code, version); err != nil {
		return "", err
	}
	return p.QRCodeService.GenerateQuestionnaireQRCode(ctx, code, version)
}
func (p *failureProbe) GenerateScaleQRCode(ctx context.Context, code string) (string, error) {
	if err := p.before(ctx, "scale", code, ""); err != nil {
		return "", err
	}
	return p.QRCodeService.GenerateScaleQRCode(ctx, code)
}
