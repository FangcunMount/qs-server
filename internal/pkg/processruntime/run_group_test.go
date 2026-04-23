package processruntime

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRunGroupStartsShutdownBeforeServices(t *testing.T) {
	t.Parallel()

	var order []string
	err := RunGroup{
		StartShutdown: func() error {
			order = append(order, "shutdown")
			return nil
		},
		Services: []ServiceRunner{
			{
				Name: "http",
				Run: func() error {
					order = append(order, "http")
					return errors.New("http boom")
				},
			},
			{
				Name: "grpc",
				Run: func() error {
					order = append(order, "grpc")
					time.Sleep(10 * time.Millisecond)
					return nil
				},
			},
		},
	}.Run()
	if err == nil || err.Error() != "http boom" {
		t.Fatalf("Run() error = %v, want http boom", err)
	}
	if len(order) == 0 || order[0] != "shutdown" {
		t.Fatalf("order = %#v, want shutdown first", order)
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
