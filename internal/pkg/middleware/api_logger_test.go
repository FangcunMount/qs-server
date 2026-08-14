package middleware

import (
	"path/filepath"
	"testing"

	"github.com/FangcunMount/component-base/pkg/log"
)

func TestRequestLoggingSkipsDisabledHotPathLevels(t *testing.T) {
	t.Cleanup(func() { log.Init(log.NewOptions()) })

	initTestLogger(t, "warn")
	if log.V(log.InfoLevel).Enabled() {
		t.Fatal("request-start info logging enabled at warn level")
	}
	for _, tc := range []struct {
		name       string
		statusCode int
		want       bool
	}{
		{name: "success", statusCode: 200, want: false},
		{name: "client error", statusCode: 400, want: true},
		{name: "server error", statusCode: 500, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := requestCompletionLoggingEnabled(tc.statusCode); got != tc.want {
				t.Fatalf("requestCompletionLoggingEnabled(%d) = %t, want %t", tc.statusCode, got, tc.want)
			}
		})
	}

	initTestLogger(t, "debug")
	if !log.V(log.InfoLevel).Enabled() {
		t.Fatal("request-start info logging disabled at debug level")
	}
	if !requestCompletionLoggingEnabled(200) {
		t.Fatal("successful request logging disabled at debug level")
	}
}

func initTestLogger(t *testing.T, level string) {
	t.Helper()
	opts := log.NewOptions()
	opts.Level = level
	opts.OutputPaths = []string{filepath.Join(t.TempDir(), "application.log")}
	opts.ErrorOutputPaths = []string{filepath.Join(t.TempDir(), "error.log")}
	log.Init(opts)
}
