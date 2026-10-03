// Candidate interop probe: disposable keys on stdin, protected bytes on stdout.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	"google.golang.org/protobuf/proto"
)

func run() error {
	var input struct {
		Mode, ID, Aggregate, Correlation, Organization, OriginalTime, Topic string
		Kind                                                                pb.MessagingKind
		Body, Wire                                                          []byte
		Signing, Encryption                                                 jose.JSONWebKey
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return err
	}
	var output []byte
	switch input.Mode {
	case "protect":
		body := new(pb.MessagingBody)
		if err := proto.Unmarshal(input.Body, body); err != nil {
			return err
		}
		message, err := app.ProtectMessaging(input.Kind, input.ID, input.Aggregate, input.Correlation, input.Organization, input.OriginalTime, body, input.Signing, input.Encryption)
		if err != nil {
			return err
		}
		output = message.Wire
		return json.NewEncoder(os.Stdout).Encode(struct{ Wire, Body []byte }{message.Wire, message.Body})
	case "authenticate":
		producer := "qs-server"
		if input.Topic == app.EventsTopic {
			producer = "qs-ai"
		}
		header, err := app.AuthenticateMessaging(input.Wire, input.Topic, protected.Keyring{
			Decrypt: map[string]jose.JSONWebKey{input.Encryption.KeyID: input.Encryption},
			Signers: map[string]protected.TrustedSigner{input.Signing.KeyID: {Producer: producer, Key: input.Signing.Public()}},
		})
		if err != nil {
			return err
		}
		if _, err := app.ParseMessagingBody(header, input.Body); err != nil {
			return err
		}
		output, err = proto.MarshalOptions{Deterministic: true}.Marshal(header)
		if err != nil {
			return err
		}
	default:
		return app.ErrMessagingContract
	}
	fmt.Print(base64.StdEncoding.EncodeToString(output))
	return nil
}

func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "messaging probe rejected")
		os.Exit(1)
	}
}
