package wechatapi

import (
	"context"
	"errors"
	"testing"

	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	miniSubscribe "github.com/silenceper/wechat/v2/miniprogram/subscribe"
)

type receiptClientStub struct {
	msgID        int64
	err          error
	receiptCalls int
	legacyCalls  int
	message      *miniSubscribe.Message
}

func (s *receiptClientStub) Send(msg *miniSubscribe.Message) error {
	s.legacyCalls++
	s.message = msg
	return s.err
}

func (s *receiptClientStub) SendGetMsgID(msg *miniSubscribe.Message) (int64, error) {
	s.receiptCalls++
	s.message = msg
	return s.msgID, s.err
}

func (*receiptClientStub) ListTemplates() (*miniSubscribe.TemplateList, error) {
	return nil, nil
}

func TestSendSubscribeMessageWithReceiptPreservesPlatformID(t *testing.T) {
	client := &receiptClientStub{msgID: 123456789}
	sender := &SubscribeSender{newClient: func(_, _ string) (subscribeClient, error) { return client, nil }}
	message := wechatmini.SubscribeMessage{
		ToUser: "recipient", TemplateID: "template", Page: "pages/task/index", MiniProgramState: "formal", Lang: "zh_CN",
		Data: map[string]string{"thing1": "task"},
	}

	receipt, err := sender.SendSubscribeMessageWithReceipt(context.Background(), "app", "secret", message)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PlatformMessageID != "123456789" || client.receiptCalls != 1 || client.legacyCalls != 0 {
		t.Fatalf("receipt=%+v, receiptCalls=%d, legacyCalls=%d", receipt, client.receiptCalls, client.legacyCalls)
	}
	if client.message == nil || client.message.ToUser != message.ToUser || client.message.TemplateID != message.TemplateID ||
		client.message.Page != message.Page || client.message.MiniprogramState != message.MiniProgramState ||
		client.message.Lang != message.Lang || client.message.Data["thing1"].Value != "task" {
		t.Fatal("platform request did not preserve the subscribe message")
	}
}

func TestSendSubscribeMessageWithReceiptWithoutIDIsUnknown(t *testing.T) {
	client := &receiptClientStub{}
	sender := &SubscribeSender{newClient: func(_, _ string) (subscribeClient, error) { return client, nil }}
	receipt, err := sender.SendSubscribeMessageWithReceipt(context.Background(), "app", "secret", wechatmini.SubscribeMessage{})
	if err == nil || receipt.PlatformMessageID != "" || client.receiptCalls != 1 || client.legacyCalls != 0 {
		t.Fatalf("receipt=%+v, err=%v, receiptCalls=%d", receipt, err, client.receiptCalls)
	}
}

func TestSendSubscribeMessageWithReceiptReturnsErrorWithoutRetry(t *testing.T) {
	client := &receiptClientStub{err: errors.New("response unavailable")}
	sender := &SubscribeSender{newClient: func(_, _ string) (subscribeClient, error) { return client, nil }}
	receipt, err := sender.SendSubscribeMessageWithReceipt(context.Background(), "app", "secret", wechatmini.SubscribeMessage{})
	if !errors.Is(err, client.err) || receipt.PlatformMessageID != "" || client.receiptCalls != 1 || client.legacyCalls != 0 {
		t.Fatalf("receipt=%+v, err=%v, receiptCalls=%d, legacyCalls=%d", receipt, err, client.receiptCalls, client.legacyCalls)
	}
}

func TestLegacySubscribeSendKeepsItsOwnCall(t *testing.T) {
	client := &receiptClientStub{}
	sender := &SubscribeSender{newClient: func(_, _ string) (subscribeClient, error) { return client, nil }}
	if err := sender.SendSubscribeMessage(context.Background(), "app", "secret", wechatmini.SubscribeMessage{}); err != nil {
		t.Fatal(err)
	}
	if client.legacyCalls != 1 || client.receiptCalls != 0 {
		t.Fatalf("legacyCalls=%d, receiptCalls=%d", client.legacyCalls, client.receiptCalls)
	}
}
