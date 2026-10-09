package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

// A diagnostic must not call Error(), even to classify a wrapped failure.
type mongoIndexDiagnosticWrapped struct{ cause error }

func (e mongoIndexDiagnosticWrapped) Error() string { panic("error text must stay private") }
func (e mongoIndexDiagnosticWrapped) Unwrap() error { return e.cause }

type mongoIndexDiagnosticTimeout struct{}

func (mongoIndexDiagnosticTimeout) Error() string   { panic("network text must stay private") }
func (mongoIndexDiagnosticTimeout) Timeout() bool   { return true }
func (mongoIndexDiagnosticTimeout) Temporary() bool { return false }

func TestMongoIndexDiagnosticClassifiesActualTypedErrorsWithoutErrorText(t *testing.T) {
	var typedNil *mongo.CommandError
	cases := []struct {
		name string
		err  error
		kind string
		code int32
	}{
		{"server-value", mongo.CommandError{Code: 13, Message: "PRIVATE_URI", Name: "PRIVATE_USER"}, "server", 13},
		{"server-pointer", &mongo.CommandError{Code: 26, Message: "PRIVATE_BODY"}, "server", 26},
		{"server-wrapped", mongoIndexDiagnosticWrapped{mongo.CommandError{Code: 50}}, "server", 50},
		{"server-deadline", mongo.CommandError{Code: 50, Wrapped: context.DeadlineExceeded}, "context_deadline", 50},
		{"deadline", mongoIndexDiagnosticWrapped{context.DeadlineExceeded}, "context_deadline", 0},
		{"cancelled", mongoIndexDiagnosticWrapped{context.Canceled}, "context_cancelled", 0},
		{"network-timeout", mongoIndexDiagnosticWrapped{mongoIndexDiagnosticTimeout{}}, "network_timeout", 0},
		{"other", errors.New("PRIVATE_URI_AND_USER"), "other", 0},
		{"typed-nil", typedNil, "other", 0},
		{"wrapped-typed-nil", mongoIndexDiagnosticWrapped{typedNil}, "other", 0},
		{"int32-min", mongo.CommandError{Code: -2147483648}, "server", -2147483648},
		{"int32-max", mongo.CommandError{Code: 2147483647}, "server", 2147483647},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			kind, code := mongoIndexErrorKind(item.err)
			if kind != item.kind || code != item.code {
				t.Fatalf("wrong closed diagnostic classification: %s/%d", kind, code)
			}
			line := mongoIndexDiagnosticLine("list", "private-db.private-collection", 25*time.Millisecond, item.err)
			if strings.Contains(line, "PRIVATE") || strings.Contains(line, "private-db") || strings.Contains(line, "private-collection") {
				t.Fatal("diagnostic disclosed source or error text")
			}
		})
	}
}

func TestMongoIndexDiagnosticHasOneFixedASCIIGoldenLineAndRejectsUnknownPhase(t *testing.T) {
	err := mongo.CommandError{Code: 13, Message: "PRIVATE_URI", Name: "PRIVATE_ACCOUNT"}
	want := "QS_MONGO_INDEX_DIAGNOSTIC phase=list kind=server code=13 namespace_sha256=2fc6912fe0ca9b79e3275613dba206e5c64605f1c663724fcc6a738f0661b86a elapsed_ms=25"
	if actual := mongoIndexDiagnosticLine("list", "private-db.private-collection", 25*time.Millisecond, err); actual != want {
		t.Fatal("diagnostic wire golden changed")
	}
	for _, phase := range []string{"list", "iterate", "close"} {
		line := mongoIndexDiagnosticLine(phase, "private-db.private-collection", 25*time.Millisecond, err)
		if strings.Replace(line, "phase="+phase, "phase=list", 1) != want || strings.ContainsAny(line, "\r\n") {
			t.Fatal("diagnostic is not one fixed ASCII line")
		}
	}
	if mongoIndexDiagnosticLine("unknown\nPRIVATE", "private-db.private-collection", 0, err) != "" || mongoIndexDiagnosticLine("list", "private-db.private-collection", 0, nil) != "" {
		t.Fatal("unknown phase or nil failure generated a diagnostic")
	}
	if line := mongoIndexDiagnosticLine("close", "private-db.private-collection", -time.Millisecond, err); !strings.HasSuffix(line, "elapsed_ms=0") {
		t.Fatal("diagnostic emitted a negative duration")
	}
}
