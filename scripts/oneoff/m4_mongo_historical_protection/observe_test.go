package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestIndexObservationKeepsTypedCodeWithoutErrorBody(t *testing.T) {
	secret := "never-emit-command-credentials-or-business-values"
	cases := []struct {
		name      string
		err       error
		class     string
		code      *int32
		knownName string
	}{
		{name: "server11000", err: mongo.CommandError{Code: 11000, Name: "DuplicateKey", Message: secret, Raw: rawBSON(t, bson.D{{Key: "ok", Value: float64(0)}, {Key: "errmsg", Value: secret}})}, class: "server_error", code: func() *int32 { n := int32(11000); return &n }(), knownName: "DuplicateKey"},
		{name: "unknown_name", err: mongo.CommandError{Code: 123456, Name: secret, Message: secret}, class: "server_error", code: func() *int32 { n := int32(123456); return &n }()},
		{name: "canceled", err: context.Canceled, class: "canceled"},
		{name: "deadline", err: context.DeadlineExceeded, class: "deadline"},
		{name: "other", err: errors.New(secret), class: "unknown_error"},
		{name: "success", class: "success"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := observeIndexResult(tc.err)
			raw, e := json.Marshal(o)
			if e != nil || strings.Contains(string(raw), secret) || o.Operation != "createIndexes" || o.Class != tc.class || o.Name != tc.knownName || (tc.code == nil) != (o.Code == nil) || tc.code != nil && *tc.code != *o.Code {
				t.Fatal("typed operation diagnostic leaked or changed category")
			}
			if tc.err == nil && (o.OK == nil || !*o.OK) {
				t.Fatal("success acknowledgment lost")
			}
			if tc.name == "server11000" && (o.OK == nil || *o.OK) {
				t.Fatal("actual typed server failure ok0 lost")
			}
		})
	}
}
