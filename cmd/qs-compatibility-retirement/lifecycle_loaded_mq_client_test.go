package main

import (
	"net/http"
	"testing"
	"time"
)

func TestLifecycleLoadedMQClientIsReadOnlyDirectAndBounded(t *testing.T) {
	c, transport := lifecycleLoadedMQHTTPClient()
	defer transport.CloseIdleConnections()
	if c.Transport != transport || transport.Proxy != nil || transport.DialContext == nil || !transport.DisableKeepAlives || c.Timeout != 5*time.Second || transport.TLSHandshakeTimeout != 5*time.Second || transport.ResponseHeaderTimeout != 5*time.Second || c.Jar != nil || c.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("metadata reader gained indirect transport or unbounded authentication")
	}
}
