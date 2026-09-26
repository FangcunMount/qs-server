package wechatapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	miniSubscribe "github.com/silenceper/wechat/v2/miniprogram/subscribe"
)

type receiptClientStub struct{ legacyCalls int }

func (s *receiptClientStub) Send(*miniSubscribe.Message) error { s.legacyCalls++; return nil }
func (*receiptClientStub) GetAccessTokenContext(context.Context) (string, error) {
	return "secret-token", nil
}
func (*receiptClientStub) ListTemplates() (*miniSubscribe.TemplateList, error) { return nil, nil }

func receiptResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestSendSubscribeMessageWithReceiptExplicitSuccess(t *testing.T) {
	for _, tc := range []struct{ body, id string }{
		{`{"errcode":0,"errmsg":"ok"}`, ""},
		{`{"errcode":0,"errmsg":"ok","msgid":123456789}`, "123456789"},
	} {
		client := &receiptClientStub{}
		calls := 0
		sender := &SubscribeSender{
			newClient: func(_, _ string) (subscribeClient, error) { return client, nil },
			doRequest: func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != http.MethodPost || req.URL.Query().Get("access_token") != "secret-token" || req.Header.Get("Content-Type") != "application/json;charset=utf-8" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil || !strings.Contains(string(body), `"touser":"recipient"`) || !strings.Contains(string(body), `"thing1":{"value":"task"`) {
					t.Fatalf("unexpected request body: %s, err=%v", body, err)
				}
				return receiptResponse(http.StatusOK, tc.body), nil
			},
		}
		message := wechatmini.SubscribeMessage{ToUser: "recipient", Data: map[string]string{"thing1": "task"}}
		receipt, err := sender.SendSubscribeMessageWithReceipt(context.Background(), "app", "secret", message)
		if err != nil || !receipt.Accepted || receipt.PlatformMessageID != tc.id || calls != 1 || client.legacyCalls != 0 {
			t.Fatalf("receipt=%+v, err=%v, calls=%d, legacyCalls=%d", receipt, err, calls, client.legacyCalls)
		}
	}
}

func TestSendSubscribeMessageWithReceiptRejectsAmbiguousResponsesWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"empty object", http.StatusOK, `{}`},
		{"null", http.StatusOK, `null`},
		{"missing errcode", http.StatusOK, `{"errmsg":"ok"}`},
		{"missing errmsg", http.StatusOK, `{"errcode":0}`},
		{"invalid errmsg", http.StatusOK, `{"errcode":0,"errmsg":"unknown"}`},
		{"invalid msgid", http.StatusOK, `{"errcode":0,"errmsg":"ok","msgid":0}`},
		{"platform rejection", http.StatusOK, `{"errcode":43101,"errmsg":"user refuse"}`},
		{"malformed", http.StatusOK, `{`},
		{"HTTP failure", http.StatusBadGateway, `{"errcode":0,"errmsg":"ok"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			sender := &SubscribeSender{
				newClient: func(_, _ string) (subscribeClient, error) { return &receiptClientStub{}, nil },
				doRequest: func(*http.Request) (*http.Response, error) {
					calls++
					return receiptResponse(tc.status, tc.body), nil
				},
			}
			receipt, err := sender.SendSubscribeMessageWithReceipt(context.Background(), "app", "secret", wechatmini.SubscribeMessage{})
			if err == nil || receipt.Accepted || calls != 1 {
				t.Fatalf("receipt=%+v, err=%v, calls=%d", receipt, err, calls)
			}
		})
	}
}

func TestSendSubscribeMessageWithReceiptHidesTokenInTransportError(t *testing.T) {
	calls := 0
	sender := &SubscribeSender{
		newClient: func(_, _ string) (subscribeClient, error) { return &receiptClientStub{}, nil },
		doRequest: func(req *http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("failed " + req.URL.String())
		},
	}
	receipt, err := sender.SendSubscribeMessageWithReceipt(context.Background(), "app", "secret", wechatmini.SubscribeMessage{})
	if err == nil || strings.Contains(err.Error(), "secret-token") || receipt.Accepted || calls != 1 {
		t.Fatalf("receipt=%+v, err=%v, calls=%d", receipt, err, calls)
	}
}

func TestLegacySubscribeSendKeepsItsOwnCall(t *testing.T) {
	client := &receiptClientStub{}
	sender := &SubscribeSender{newClient: func(_, _ string) (subscribeClient, error) { return client, nil }}
	if err := sender.SendSubscribeMessage(context.Background(), "app", "secret", wechatmini.SubscribeMessage{}); err != nil {
		t.Fatal(err)
	}
	if client.legacyCalls != 1 {
		t.Fatalf("legacyCalls=%d", client.legacyCalls)
	}
}
