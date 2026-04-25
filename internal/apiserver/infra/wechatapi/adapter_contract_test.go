package wechatapi

import (
	"context"
	"testing"
)

func TestQRCodeGeneratorValidatesRequiredInputsBeforeSDKCall(t *testing.T) {
	generator := NewQRCodeGenerator(nil)

	if _, err := generator.GenerateQRCode(context.Background(), "", "secret", "pages/index", 430); err == nil {
		t.Fatal("GenerateQRCode should reject empty appID before SDK call")
	}
	if _, err := generator.GenerateQRCode(context.Background(), "app", "secret", "", 430); err == nil {
		t.Fatal("GenerateQRCode should reject empty path before SDK call")
	}
	if _, err := generator.GenerateUnlimitedQRCode(context.Background(), "app", "secret", "", "pages/index", 430, false, nil, false); err == nil {
		t.Fatal("GenerateUnlimitedQRCode should reject empty scene before SDK call")
	}
	if _, err := generator.GenerateUnlimitedQRCode(context.Background(), "app", "secret", "scene", "", 430, false, nil, false); err == nil {
		t.Fatal("GenerateUnlimitedQRCode should reject empty page before SDK call")
	}
}

func TestTokenProviderValidatesRequiredInputsBeforeSDKCall(t *testing.T) {
	provider := NewTokenProvider(nil)

	if _, err := provider.FetchMiniProgramToken(context.Background(), "", "secret"); err == nil {
		t.Fatal("FetchMiniProgramToken should reject empty appID before SDK call")
	}
	if _, err := provider.FetchOfficialAccountToken(context.Background(), "app", ""); err == nil {
		t.Fatal("FetchOfficialAccountToken should reject empty appSecret before SDK call")
	}
}

// func TestSubscribeSenderValidatesRequiredInputsBeforeSDKCall(t *testing.T) {
// 	sender := NewSubscribeSender(nil)

// 	err := sender.SendSubscribeMessage(context.Background(), "", "secret", wechatPort.SubscribeMessage{
// 		ToUser:     "openid",
// 		TemplateID: "tmpl",
// 	})
// 	if err == nil || !strings.Contains(err.Error(), "appID and appSecret cannot be empty") {
// 		t.Fatalf("expected app config validation error, got %v", err)
// 	}
// }
