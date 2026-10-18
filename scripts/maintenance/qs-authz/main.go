// qs-authz separates read-only verification from explicit IAM provisioning.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/authzmatrix"
)

const authzUsage = `usage:
  qs-authz verify [QS config flags]       read-only IAM action matrix
  qs-authz provision [QS config flags]    explicit isolated IAM test subjects

Provision requires QS_AUTHZ_MATRIX_PROVISION_CONFIRM with the reviewed confirmation.
Both commands retain the QS_APISERVER_ configuration prefix.`

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println(authzUsage)
		return
	}
	action, flags, err := parseAuthzArgs(os.Args[1:])
	if err == nil && action == "provision" && os.Getenv("QS_AUTHZ_MATRIX_PROVISION_CONFIRM") != authzmatrix.ProvisionConfirmation {
		err = errors.New("explicit provisioning confirmation is required")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Args = append([]string{os.Args[0]}, flags...)
	if action == "verify" {
		runVerifyCommand()
	} else {
		runProvisionCommand()
	}
}

func parseAuthzArgs(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, errors.New("an AuthZ command is required; use qs-authz --help")
	}
	if args[0] != "verify" && args[0] != "provision" {
		return "", nil, errors.New("unsupported AuthZ command; use qs-authz --help")
	}
	return args[0], args[1:], nil
}
