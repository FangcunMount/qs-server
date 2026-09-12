package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/protobuf/encoding/protojson"
	"reflect"
	"strings"
	"testing"
)

func TestCreatePreservesAllRefsAndNeverRetriesUnknownReceipt(t *testing.T) {
	rpc := &evaluationRPCStub{t: t}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	command := app.EvaluationCreate{Reason: "创建评测", Confirm: true}
	value := reflect.ValueOf(&command.Release).Elem()
	for i := 0; i < value.NumField(); i++ {
		value.Field(i).Set(reflect.ValueOf(app.FrozenEvaluationRef{ID: value.Type().Field(i).Name, Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64)}))
	}
	if _, err := client.CreateEvaluation(context.Background(), scope, command); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(command.Release)
	got, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(rpc.create.Release)
	if err != nil {
		t.Fatal(err)
	}
	var wantFields, gotFields any
	if json.Unmarshal(want, &wantFields) != nil || json.Unmarshal(got, &gotFields) != nil || !reflect.DeepEqual(wantFields, gotFields) {
		t.Fatal("frozen references changed in transit")
	}
	if rpc.calls != 1 || rpc.create.Scope.OrganizationId != 7 || rpc.create.Scope.OperatorUserId != 42 || !rpc.create.Confirm || rpc.create.Reason != command.Reason {
		t.Fatal("creation scope drift")
	}
	rpc.fail = context.DeadlineExceeded
	if _, err := client.CreateEvaluation(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if rpc.calls != 2 {
		t.Fatal("uncertain creation automatically retried")
	}
}
