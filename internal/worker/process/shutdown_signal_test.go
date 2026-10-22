package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
)

// This is an owned process/signal contract. The held subscriber and dependency
// are test seams; it makes no real Broker, database or whole-host claim.
func TestWorkerOwnedSIGTERMWaitsForActualDrainAndReportsFailure(t *testing.T) {
	for _, mode := range []string{"drain", "failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			directory := t.TempDir()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkerShutdownSignalChild$")
			cmd.Env = append(os.Environ(), "QS_M3_WORKER_SHUTDOWN_CHILD="+mode, "QS_M3_WORKER_SHUTDOWN_DIR="+directory)
			var logs bytes.Buffer
			cmd.Stdout, cmd.Stderr = &logs, &logs
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			defer func() { _ = input.Close(); cancel(); <-ctx.Done() }()
			await := func(name string) {
				t.Helper()
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					if _, err := os.Stat(filepath.Join(directory, name)); err == nil {
						return
					}
					select {
					case err := <-done:
						// The failure child can write the marker and exit between
						// our stat and select. Preserve its exit for final checking.
						if _, statErr := os.Stat(filepath.Join(directory, name)); statErr == nil {
							done <- err
							return
						}
						t.Fatalf("child exited before %s: %v %s", name, err, logs.String())
					case <-ctx.Done():
						t.Fatalf("child did not reach %s", name)
					case <-ticker.C:
					}
				}
			}
			await("ready")
			if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			await("draining")
			if mode == "drain" {
				select {
				case err := <-done:
					t.Fatal("main bypassed actual drain", err, logs.String())
				case <-time.After(50 * time.Millisecond):
				}
				if _, err = os.Stat(filepath.Join(directory, "dependency_closed")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("dependency closed while subscriber blocked", err)
				}
				if _, err = input.Write([]byte{1}); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err = <-done:
			case <-ctx.Done():
				t.Fatal("child failed to terminate")
			}
			if mode == "drain" {
				if err != nil {
					t.Fatal(err, logs.String())
				}
				if _, err = os.Stat(filepath.Join(directory, "dependency_closed")); err != nil {
					t.Fatal("actual drain did not close dependency", err)
				}
			} else {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 {
					t.Fatal("drain failure did not exit1", err, logs.String())
				}
				if _, err = os.Stat(filepath.Join(directory, "dependency_closed")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failure closed downstream dependency", err)
				}
			}
		})
	}
}

func TestWorkerShutdownSignalChild(t *testing.T) {
	mode := os.Getenv("QS_M3_WORKER_SHUTDOWN_CHILD")
	if mode == "" {
		return
	}
	directory := os.Getenv("QS_M3_WORKER_SHUTDOWN_DIR")
	write := func(name string) error {
		return os.WriteFile(filepath.Join(directory, name), []byte("fixture\n"), 0600)
	}
	s := &server{gs: shutdown.New()}
	s.gs.AddShutdownManager(posixsignal.NewPosixSignalManager())
	s.registerShutdownCallback(lifecycleDeps{
		stopSubscriber: func() error {
			if err := write("draining"); err != nil {
				return err
			}
			if mode == "failure" {
				return errors.New("owned fixture drain failure")
			}
			var token [1]byte
			_, err := io.ReadFull(os.Stdin, token[:])
			return err
		},
		closeDatabase: func() error { return write("dependency_closed") },
	})
	prepared := preparedServer{shutdown: s.shutdown, startShutdown: func() error {
		if err := s.gs.Start(); err != nil {
			return err
		}
		// Give the existing POSIX manager its registration turn before exposing
		// fixture readiness. A missing registration makes the parent fail.
		time.Sleep(50 * time.Millisecond)
		return write("ready")
	}}
	if err := prepared.Run(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
