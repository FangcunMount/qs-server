// qs-ai-messaging operates the host database only. Offline key/configuration
// preflight remains a separate binary and never opens these database bindings.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

var sourceSHA = "development"

const messagingUsage = `usage:
  qs-ai-messaging audit [-limit 100]                 read-only database snapshot
  qs-ai-messaging admission inspect                 read-only admission state
  qs-ai-messaging admission close|open -expected-revision N

Database bindings remain QS_AI_MESSAGING_AUDIT_DSN and QS_AI_MESSAGING_CONTROL_DSN.
Offline companion: qs-ai-messaging-preflight --source-sha|--binding ...
The offline companion never loads either database binding.`

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println(messagingUsage)
		return
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(messagingUsage)
	}
	switch args[0] {
	case "audit":
		limit, err := auditArguments(args[1:])
		if err != nil {
			return err
		}
		if err := runAudit(limit); err != nil {
			return errors.New("read-only MQ database audit failed")
		}
	case "admission":
		action, revision, err := admissionArguments(args[1:])
		if err != nil {
			return err
		}
		if err := runAdmission(action, revision); err != nil {
			return errors.New("MQ runtime admission control failed; no closure or submission decision can be inferred")
		}
	default:
		return errors.New("unsupported MQ maintenance command; use qs-ai-messaging --help")
	}
	return nil
}

func auditArguments(args []string) (int, error) {
	flags := flag.NewFlagSet("qs-ai-messaging audit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	limit := flags.Int("limit", 100, "bounded message sample limit, 1..1000")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *limit < 1 || *limit > 1000 {
		return 0, errors.New("audit requires a sample limit from 1 to 1000 and no positional arguments")
	}
	return *limit, nil
}

func admissionArguments(args []string) (string, string, error) {
	if len(args) == 0 || (args[0] != "inspect" && args[0] != "close" && args[0] != "open") {
		return "", "", errors.New("admission requires explicit inspect, close or open")
	}
	flags := flag.NewFlagSet("qs-ai-messaging admission", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	revision := flags.String("expected-revision", "", "reviewed revision required for close/open")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return "", "", errors.New("invalid admission arguments")
	}
	if _, err := admissionRevision(args[0], *revision); err != nil {
		return "", "", err
	}
	return args[0], *revision, nil
}
