package processruntime

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRunGroupStartsShutdownBeforeServices(t *testing.T) {
	t.Parallel()

	order := make(chan string, 3)
	httpDone := make(chan struct{})
	grpcDone := make(chan struct{})
	err := RunGroup{
		StartShutdown: func() error {
			order <- "shutdown"
			return nil
		},
		Services: []ServiceRunner{
			{
				Name: "http",
				Run: func() error {
					defer close(httpDone)
					order <- "http"
					return errors.New("http boom")
				},
			},
			{
				Name: "grpc",
				Run: func() error {
					defer close(grpcDone)
					order <- "grpc"
					return nil
				},
			},
		},
	}.Run()
	if err == nil || err.Error() != "http boom" {
		t.Fatalf("Run() error = %v, want http boom", err)
	}

	waitDone := func(name string, done <-chan struct{}) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("%s service did not finish", name)
		}
	}

	waitDone("http", httpDone)
	waitDone("grpc", grpcDone)

	first := <-order
	if first != "shutdown" {
		t.Fatalf("first event = %q, want shutdown", first)
	}
}

func TestRunGroupReturnsNilWhenNoServices(t *testing.T) {
	t.Parallel()

	var order []string
	err := RunGroup{
		StartShutdown: func() error {
			order = append(order, "shutdown")
			return nil
		},
	}.Run()
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if want := []string{"shutdown"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}
