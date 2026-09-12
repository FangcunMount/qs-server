package aibridge

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func validCreate() EvaluationCreate {
	command := EvaluationCreate{Reason: "创建评测", Confirm: true}
	release := reflect.ValueOf(&command.Release).Elem()
	for i := 0; i < release.NumField(); i++ {
		release.Field(i).Set(reflect.ValueOf(FrozenEvaluationRef{ID: release.Type().Field(i).Name, Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64)}))
	}
	return command
}

func TestCreateChecksEveryReferenceAndCurrentAuthorization(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	for i := 0; i < reflect.TypeOf(EvaluationRelease{}).NumField(); i++ {
		command := validCreate()
		reflect.ValueOf(&command.Release).Elem().Field(i).Set(reflect.ValueOf(FrozenEvaluationRef{}))
		if _, err := service.Create(adminContext(), managementScope(), command); !errors.Is(err, ErrInvalid) {
			t.Fatalf("field %d: %v", i, err)
		}
	}
	for _, change := range []func(*EvaluationCreate){func(c *EvaluationCreate) { c.Confirm = false }, func(c *EvaluationCreate) { c.Reason = " " }, func(c *EvaluationCreate) { c.Reason = strings.Repeat("中", 334) }, func(c *EvaluationCreate) { c.Release.Suite.Fingerprint = "invalid" }} {
		command := validCreate()
		change(&command)
		if _, err := service.Create(adminContext(), managementScope(), command); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if gateway.calls != 0 {
		t.Fatal("invalid creation forwarded")
	}
	if _, err := service.Create(adminContext(), managementScope(), validCreate()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), managementScope(), validCreate()); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if gateway.calls != 1 || gateway.scope != managementScope() {
		t.Fatal("authorization or identity drift")
	}
}
