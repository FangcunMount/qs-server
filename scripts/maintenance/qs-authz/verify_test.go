package main

import (
	"path/filepath"
	"strings"
	"testing"

	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestOpenReadOnlySourceHonorsConfiguredCA(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	opts.MySQLOptions.TLSMode = "VERIFY_IDENTITY"
	opts.MySQLOptions.TLSCAFile = filepath.Join(t.TempDir(), "missing-ca.pem")

	conn, _, err := openReadOnlySource(opts)
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "missing-ca.pem") {
		t.Fatalf("configured CA must be validated before dialing: %v", err)
	}
}
