package main

import (
	"context"
	"encoding/json"
	"fmt"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/operatorretire"
	"io"
	"os"
	"strings"
)

func readSelectedCommand(path string) (operatorapp.Command, error) {
	var cmd operatorapp.Command
	file, err := os.Open(path)
	if err != nil {
		return cmd, fmt.Errorf("selected retirement input file required")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return cmd, fmt.Errorf("selected retirement input must be a regular file smaller than 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cmd); err != nil {
		return cmd, fmt.Errorf("invalid selected retirement input")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return cmd, fmt.Errorf("selected retirement input must contain one JSON object")
	}
	if cmd.OrgID <= 0 || cmd.ActorID <= 0 || cmd.OperatorID == 0 || cmd.ExpectedVersion == 0 || cmd.RequestID == "" || len(cmd.RequestID) > 64 || strings.TrimSpace(cmd.RequestID) != cmd.RequestID || strings.TrimSpace(cmd.Reason) != cmd.Reason || cmd.Reason == "" || len([]rune(cmd.Reason)) > 500 {
		return cmd, fmt.Errorf("exact selected retirement identity, versions, request and reason required")
	}
	return cmd, nil
}
func runSelected(ctx context.Context, tool *operatorretire.Tool, action string, output *os.File) error {
	cmd, err := readSelectedCommand(os.Getenv("OPERATOR_RETIRE_INPUT"))
	if err != nil {
		return err
	}
	var report *operatorretire.SelectedReport
	switch action {
	case "selected-preflight":
		report, err = tool.SelectedPreflight(ctx, cmd)
	case "selected-apply":
		report, err = tool.SelectedApply(ctx, cmd, os.Getenv("OPERATOR_RETIRE_FINGERPRINT"), os.Getenv("OPERATOR_RETIRE_MAINTENANCE") == "true")
	case "selected-verify":
		report, err = tool.SelectedVerify(ctx, cmd)
	}
	if report == nil {
		report = &operatorretire.SelectedReport{State: "invalid", Command: cmd}
	}
	if err != nil {
		report.LastError = err.Error()
	}
	if e := json.NewEncoder(output).Encode(report); e != nil {
		return e
	}
	if e := output.Sync(); e != nil {
		return e
	}
	if e := output.Close(); e != nil {
		return e
	}
	fmt.Printf("selected retirement state=%s; details saved privately\n", report.State)
	if err != nil {
		return fmt.Errorf("selected retirement stopped; inspect restricted report")
	}
	return nil
}
