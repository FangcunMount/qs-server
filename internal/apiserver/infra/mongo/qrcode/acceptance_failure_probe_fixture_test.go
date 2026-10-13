package qrcode

import (
	"context"
	"fmt"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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

// failureProbe is a test-only durable fault fixture. Production QR wiring uses the ordinary optional service.
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
