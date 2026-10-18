// qs-operator groups explicit personnel maintenance operations.
package main

import (
	"errors"
	"fmt"
	"os"
)

const operatorUsage = `usage:
  qs-operator prepare preflight|apply|status [QS config flags]
  qs-operator recover preflight|apply|status [QS config flags]
  qs-operator retire selected preflight|apply|verify [QS config flags]
  qs-operator retire orphan preflight|apply|verify [QS config flags]

Each operation retains its OPERATOR_PREPARE_*, OPERATOR_RECOVERY_* or
OPERATOR_RETIRE_* private input, report, fingerprint and maintenance requirements.
Configuration retains the QS_APISERVER_ environment prefix.`

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println(operatorUsage)
		return
	}
	family, action, flags, err := parseOperatorArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Resolve the operation before constructing Options or reading configuration.
	os.Args = append([]string{os.Args[0]}, flags...)
	switch family {
	case "prepare":
		runPrepare(action)
	case "recover":
		runRecover(action)
	case "retire":
		runRetire(action)
	}
}

func parseOperatorArgs(args []string) (family, action string, flags []string, err error) {
	if len(args) < 2 {
		return "", "", nil, errors.New("operator maintenance command and action are required; use qs-operator --help")
	}
	family, action = args[0], args[1]
	switch family {
	case "prepare", "recover":
		if action == "preflight" || action == "apply" || action == "status" {
			return family, action, args[2:], nil
		}
	case "retire":
		if action == "preflight" || action == "apply" || action == "status" || action == "verify" {
			return "", "", nil, errors.New("historical batch retirement actions are retired; archived tools are restricted to isolated historical databases")
		}
		if len(args) >= 3 && (action == "selected" || action == "orphan") {
			operation := args[2]
			if operation == "preflight" || operation == "apply" || operation == "verify" {
				return family, action + "-" + operation, args[3:], nil
			}
		}
	}
	return "", "", nil, errors.New("unsupported operator maintenance command; use qs-operator --help")
}
