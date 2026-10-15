package main

import (
	"context"
	"errors"
	"net"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
)

// A narrow, internal diagnostic seam for this temporary tool. The real CLI
// always passes nil. No error body, original command, credentials or BSON
// document value reaches an observation or the public production receipt.
type indexObserver struct {
	monitor   *event.CommandMonitor
	completed func(indexObservation)
}
type indexObservation struct {
	Operation string `json:"operation"`
	Class     string `json:"class"`
	Code      *int32 `json:"server_code,omitempty"`
	Name      string `json:"server_code_name,omitempty"`
	OK        *bool  `json:"response_ok"`
}

func responseOK(raw bson.Raw) *bool {
	v := raw.Lookup("ok")
	if n, ok := v.DoubleOK(); ok && (n == 0 || n == 1) {
		return boolean(n == 1)
	}
	if n, ok := v.Int32OK(); ok && (n == 0 || n == 1) {
		return boolean(n == 1)
	}
	if n, ok := v.Int64OK(); ok && (n == 0 || n == 1) {
		return boolean(n == 1)
	}
	return nil
}
func observeIndexResult(e error) indexObservation {
	out := indexObservation{Operation: "createIndexes", Class: "success", OK: boolean(true)}
	if e == nil {
		return out
	}
	out.OK = nil
	var server mongo.CommandError
	if errors.As(e, &server) {
		code := server.Code
		out.Class = "server_error"
		out.Code = &code
		out.OK = responseOK(server.Raw)
		// Closed names only. An unexpected server-provided string is omitted,
		// while the actual numeric code remains available to the isolated test.
		names := map[int32]string{9: "FailedToParse", 13: "Unauthorized", 50: "MaxTimeMSExpired", 72: "InvalidOptions", 85: "IndexOptionsConflict", 86: "IndexKeySpecsConflict", 197: "InvalidIndexSpecificationOption", 11000: "DuplicateKey", 40415: "IDLUnknownField"}
		if want := names[code]; want != "" && want == server.Name {
			out.Name = want
		}
		return out
	}
	switch {
	case errors.Is(e, context.DeadlineExceeded):
		out.Class = "deadline"
	case errors.Is(e, context.Canceled):
		out.Class = "canceled"
	default:
		var network net.Error
		if errors.As(e, &network) {
			out.Class = "network_error"
			if network.Timeout() {
				out.Class = "network_timeout"
			}
		} else {
			out.Class = "unknown_error"
		}
	}
	return out
}
