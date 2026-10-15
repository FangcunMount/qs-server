package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
)

type arguments struct {
	action, sqlName, mongoName, capture, material, materialHash, approval, approvalHash string
	timeout                                                                             time.Duration
}
type receipt struct {
	Action              string `json:"action"`
	ToolSource          string `json:"compiled_tool_source"`
	BSource             string `json:"b_execution_source"`
	BResources          string `json:"b_execution_resources_sha256"`
	BResourceFiles      int    `json:"b_execution_resource_files"`
	Complete            bool   `json:"complete"`
	ImplementationReady bool   `json:"database_actions_implemented"`
	FreezeCertified     bool   `json:"production_freeze_certified"`
	Error               string `json:"error_category"`
}

func absolutePath(s string) bool { return filepath.IsAbs(s) && filepath.Clean(s) == s }
func parseArgs(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("m4-mongo-historical-protection", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&a.action, "action", "inspect", "inspect, capture, verify-target, restore")
	f.StringVar(&a.sqlName, "mysql-database", "", "explicit selected database")
	f.StringVar(&a.mongoName, "mongo-database", "", "explicit selected database")
	f.StringVar(&a.capture, "capture-file", "", "first private capture path")
	f.StringVar(&a.material, "material-file", "", "independently frozen private material")
	f.StringVar(&a.materialHash, "material-sha256", "", "externally frozen raw bytes hash")
	f.StringVar(&a.approval, "approval-file", "", "independent pre-start restore approval")
	f.StringVar(&a.approvalHash, "approval-sha256", "", "independent approval raw bytes hash")
	f.DurationVar(&a.timeout, "timeout", 15*time.Minute, "finite total deadline, at most20m")
	if f.Parse(args) != nil || f.NArg() != 0 || !nameRE.MatchString(a.sqlName) || !nameRE.MatchString(a.mongoName) || a.timeout <= 0 || a.timeout > 20*time.Minute {
		return a, errors.New("arguments_invalid")
	}
	switch a.action {
	case "inspect":
		if a.capture != "" || a.material != "" || a.materialHash != "" || a.approval != "" || a.approvalHash != "" {
			return a, errors.New("inspect_rejects_write_inputs")
		}
	case "capture":
		if !absolutePath(a.capture) || a.material != "" || a.materialHash != "" || a.approval != "" || a.approvalHash != "" {
			return a, errors.New("capture_arguments_invalid")
		}
	case "verify-target", "restore":
		if a.capture != "" || !absolutePath(a.material) || !nonzeroHash(a.materialHash) || a.action == "restore" && (!absolutePath(a.approval) || !nonzeroHash(a.approvalHash)) || a.action == "verify-target" && (a.approval != "" || a.approvalHash != "") {
			return a, errors.New("bound_material_and_action_required")
		}
	default:
		return a, errors.New("action_invalid")
	}
	return a, nil
}

// First review batch: no connection, file creation, approval consumption or
// database operation is implemented. A valid CLI never implies completion.
func serve(args []string, tool string, out io.Writer) int {
	a, e := parseArgs(args)
	r := receipt{BSource: bSource, BResources: bResources, BResourceFiles: bResourceFiles, Error: "arguments_invalid"}
	if e == nil && sourceRE.MatchString(tool) {
		r.Action, r.ToolSource = a.action, tool
		r.Error = "database_actions_not_implemented"
	}
	_ = json.NewEncoder(out).Encode(r)
	return 1
}
func main() { os.Exit(serve(os.Args[1:], version.GitCommit, os.Stdout)) }
