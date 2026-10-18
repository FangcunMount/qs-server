package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/operatorretire"
	"io"
	"os"
	"strings"
)

func readOrphanCommand(path string) (operatorretire.OrphanCommand, error) {
	var cmd operatorretire.OrphanCommand
	file, err := os.Open(path)
	if err != nil {
		return cmd, fmt.Errorf("orphan retirement input file required")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return cmd, fmt.Errorf("orphan retirement input must be a regular file smaller than 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cmd); err != nil {
		return cmd, fmt.Errorf("invalid orphan retirement input")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return cmd, fmt.Errorf("orphan retirement input must contain one JSON object")
	}
	if cmd.OrgID <= 0 || cmd.ActorID <= 0 || cmd.UserID <= 0 || cmd.UserID == cmd.ActorID || cmd.RequestID == "" || len(cmd.RequestID) > 64 || strings.TrimSpace(cmd.RequestID) != cmd.RequestID || strings.TrimSpace(cmd.Reason) != cmd.Reason || cmd.Reason == "" || len([]rune(cmd.Reason)) > 500 {
		return cmd, fmt.Errorf("exact orphan retirement identity, request and reason required")
	}
	return cmd, nil
}
func runOrphan(ctx context.Context, tool *operatorretire.Tool, action string, output *os.File) error {
	cmd, err := readOrphanCommand(os.Getenv("OPERATOR_RETIRE_INPUT"))
	if err != nil {
		return err
	}
	var report *operatorretire.OrphanReport
	switch action {
	case "orphan-preflight":
		report, err = tool.OrphanPreflight(ctx, cmd)
	case "orphan-apply":
		report, err = tool.OrphanApply(ctx, cmd, os.Getenv("OPERATOR_RETIRE_FINGERPRINT"), os.Getenv("OPERATOR_RETIRE_MAINTENANCE") == "true")
	case "orphan-verify":
		report, err = tool.OrphanPreflight(ctx, cmd)
		if err == nil && report.State != "already_unprivileged" {
			err = fmt.Errorf("backend roles remain")
		}
	}
	if report == nil {
		report = &operatorretire.OrphanReport{State: "invalid", Command: cmd}
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
	fmt.Printf("orphan retirement state=%s; details saved privately\n", report.State)
	if err != nil {
		return fmt.Errorf("orphan retirement stopped; inspect restricted report")
	}
	return nil
}
