package processruntime

import (
	"errors"
	"reflect"
	"testing"
)

type testState struct {
	order []string
}

type testStage struct {
	name string
	run  func(*testState) error
}

func (s testStage) Name() string            { return s.name }
func (s testStage) Run(st *testState) error { return s.run(st) }

func TestRunnerExecutesStagesInOrder(t *testing.T) {
	t.Parallel()

	state := &testState{}
	got, failedStage, err := Runner[testState, []string]{
		State: state,
		Stages: []Stage[testState]{
			testStage{name: "one", run: func(st *testState) error {
				st.order = append(st.order, "one")
				return nil
			}},
			testStage{name: "two", run: func(st *testState) error {
				st.order = append(st.order, "two")
				return nil
			}},
		},
		BuildPrepared: func(st *testState) []string {
			return append([]string(nil), st.order...)
		},
	}.Run()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if failedStage != "" {
		t.Fatalf("failedStage = %q, want empty", failedStage)
	}
	if want := []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("prepared = %#v, want %#v", got, want)
	}
}

func TestRunnerStopsOnFirstError(t *testing.T) {
	t.Parallel()

	state := &testState{}
	_, failedStage, err := Runner[testState, struct{}]{
		State: state,
		Stages: []Stage[testState]{
			testStage{name: "one", run: func(st *testState) error {
				st.order = append(st.order, "one")
				return nil
			}},
			testStage{name: "two", run: func(st *testState) error {
				st.order = append(st.order, "two")
				return errors.New("boom")
			}},
			testStage{name: "three", run: func(st *testState) error {
				st.order = append(st.order, "three")
				return nil
			}},
		},
	}.Run()
	if err == nil || err.Error() != "boom" {
		t.Fatalf("Run() error = %v, want boom", err)
	}
	if failedStage != "two" {
		t.Fatalf("failedStage = %q, want two", failedStage)
	}
	if want := []string{"one", "two"}; !reflect.DeepEqual(state.order, want) {
		t.Fatalf("state order = %#v, want %#v", state.order, want)
	}
}
