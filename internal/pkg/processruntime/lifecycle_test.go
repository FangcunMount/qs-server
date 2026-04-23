package processruntime

import (
	"errors"
	"reflect"
	"testing"
)

func TestLifecycleRunsHooksInOrder(t *testing.T) {
	t.Parallel()

	var order []string
	lifecycle := Lifecycle{}
	lifecycle.AddShutdownHook("one", func() error {
		order = append(order, "one")
		return nil
	})
	lifecycle.AddShutdownHook("two", func() error {
		order = append(order, "two")
		return nil
	})

	lifecycle.Run(nil)

	if want := []string{"one", "two"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("hook order = %#v, want %#v", order, want)
	}
}

func TestLifecycleContinuesAfterError(t *testing.T) {
	t.Parallel()

	var order []string
	var reported []string
	lifecycle := Lifecycle{}
	lifecycle.AddShutdownHook("one", func() error {
		order = append(order, "one")
		return errors.New("boom")
	})
	lifecycle.AddShutdownHook("two", func() error {
		order = append(order, "two")
		return nil
	})

	lifecycle.Run(func(name string, err error) {
		reported = append(reported, name+":"+err.Error())
	})

	if want := []string{"one", "two"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("hook order = %#v, want %#v", order, want)
	}
	if want := []string{"one:boom"}; !reflect.DeepEqual(reported, want) {
		t.Fatalf("reported = %#v, want %#v", reported, want)
	}
}
