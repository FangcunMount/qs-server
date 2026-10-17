package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestMockCohortSafeFailureDoesNotExposeDriverOrNetworkInputs(t *testing.T) {
	secret := "not-a-real-password.private.invalid"
	sqlErr := &mysqldriver.MySQLError{Number: 1045, SQLState: [5]byte{'2', '8', '0', '0', '0'}, Message: secret}
	mongoErr := mongo.CommandError{Code: 13, Name: "Unauthorized", Message: secret}
	netErr := &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 3), Port: 12345}, Err: errors.New(secret)}
	r := safeFailure(safePhase("initial_identity", errors.Join(sqlErr, mongoErr, netErr, errors.New(secret), errors.New("mongo identity changed"), context.DeadlineExceeded)))
	raw := string(encode(r))
	for _, bad := range []string{secret, "192.0.2.3", "12345", "dial"} {
		if strings.Contains(raw, bad) {
			t.Fatalf("private driver/network input escaped: %s", bad)
		}
	}
	if len(r.Drivers) != 2 || r.Drivers[0].Code != 1045 || r.Drivers[0].SQLState != "28000" || r.Drivers[1].Code != 13 || r.Drivers[1].Name != "Unauthorized" || !r.NetworkFailure || !r.Deadline || len(r.Codes) != 1 || r.Codes[0] != "mongo_identity_changed" {
		t.Fatalf("safe actionable categories missing: %+v", r)
	}
	if len(r.ErrorTypes) == 0 {
		t.Fatal("safe concrete error types missing")
	}
	if len(r.Phases) != 1 || r.Phases[0] != "initial_identity" {
		t.Fatal("phase missing")
	}
}
func TestMockCohortSafeFailureRejectsUntrustedDriverName(t *testing.T) {
	r := safeFailure(mongo.CommandError{Code: 13, Name: "URI mongodb://private.invalid/password", Message: "private"})
	if r.Drivers[0].Name != "" || strings.Contains(string(encode(r)), "private") {
		t.Fatal("untrusted driver name escaped")
	}
}
