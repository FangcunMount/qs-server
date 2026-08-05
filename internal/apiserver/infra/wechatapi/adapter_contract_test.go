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
