package apiserver

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
)

func TestRunDelegatesToProcessRun(t *testing.T) {
	t.Parallel()

	orig := runProcess
	defer func() { runProcess = orig }()

	wantCfg := &config.Config{}
	wantErr := errors.New("boom")
	called := false
	runProcess = func(cfg *config.Config) error {
		called = true
		if cfg != wantCfg {
			t.Fatalf("Run() passed cfg %p, want %p", cfg, wantCfg)
		}
		return wantErr
	}

	if err := Run(wantCfg); !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
	if !called {
		t.Fatal("Run() did not delegate to process runner")
	}
}
