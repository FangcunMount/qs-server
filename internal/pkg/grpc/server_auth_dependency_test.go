package grpc

import "testing"

func TestNewServerRejectsEnabledAuthWithoutTokenVerifier(t *testing.T) {
	config := NewConfig()
	config.Auth.Enabled = true

	server, err := NewServer(config, nil)

	if err == nil {
		t.Fatal("NewServer() error = nil, want missing TokenVerifier error")
	}
	if server != nil {
		t.Fatalf("server = %#v, want nil", server)
	}
}
