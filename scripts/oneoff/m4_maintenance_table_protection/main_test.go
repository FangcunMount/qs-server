package main

import (
	"context"
	"errors"
	"github.com/go-sql-driver/mysql"
	"io"
	"strings"
	"testing"
	"time"
)

func cliArgs(action string) []string {
	b := testBinding()
	return []string{"--action", action, "--mysql-database", b.SQLName, "--mongo-database", b.MongoName, "--mysql-identity-sha256", b.SQLIdentity, "--mongo-identity-sha256", b.MongoIdentity, "--mongo-ledger-uuid-sha256", b.MongoUUID}
}
func TestReadOnlyEntrypointsRejectRestoreBeforeEnvironment(t *testing.T) {
	cases := [][]string{cliArgs("restore"), append(cliArgs("inspect"), "--approval-file", "/private/approval.json"), append(cliArgs("verify-target"), "--material-file", "/private/original.json"), append(cliArgs("capture"), "--capture-file", "relative.json"), append(cliArgs("inspect"), "--timeout", "21m")}
	for i, args := range cases {
		r := run(context.Background(), args, func(string) string { t.Fatal("read ENV before rejection"); return "" }, testTool)
		if r.Complete || !r.ReadOnly || r.MaterialHash != "" || r.SQLConnectionClosed || r.MongoClosed {
			t.Fatalf("case%d %+v", i, r)
		}
	}
}
func TestUncompiledToolAndUnknownMaterialNeverConnect(t *testing.T) {
	r := run(context.Background(), cliArgs("inspect"), func(string) string { t.Fatal("ENV"); return "" }, "unknown")
	if r.Complete || r.Error != "compiled_tool_source_required" {
		t.Fatal(r)
	}
}
func TestDSNTemporalAndFiniteActionBinding(t *testing.T) {
	cfg, e := mysql.ParseDSN("fixture:only@tcp(localhost:3306)/qs?parseTime=true&loc=Asia%2FShanghai&readTimeout=1s&multiStatements=true")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Second)
	defer cancel()
	if !configureMySQLAction(ctx, cfg) || cfg.ParseTime || cfg.MultiStatements || cfg.Timeout != queryLimit || cfg.WriteTimeout != queryLimit || cfg.ReadTimeout <= queryLimit || cfg.ReadTimeout > 11*time.Second {
		t.Fatal(cfg)
	}
	if configureMySQLAction(context.Background(), cfg) {
		t.Fatal("unbounded action")
	}
}

type badOutput struct{}

func (badOutput) Write([]byte) (int, error) { return 0, errors.New("fixture writer closed") }
func TestReceiptDeliveryFailureIsNotSuccess(t *testing.T) {
	if serve(context.Background(), cliArgs("restore"), func(string) string { t.Fatal("ENV"); return "" }, testTool, badOutput{}) != 1 {
		t.Fatal("missing receipt success")
	}
	var out strings.Builder
	if serve(context.Background(), cliArgs("restore"), func(string) string { return "" }, testTool, &out) != 1 || !strings.Contains(out.String(), "restore_not_implemented") {
		t.Fatal(out.String())
	}
	if serve(context.Background(), nil, func(string) string { return "" }, testTool, io.Discard) != 1 {
		t.Fatal("no bindings")
	}
}
