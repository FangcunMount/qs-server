package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"time"

	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	subject  = "user:636933038441247278"
	orgID    = "1"
	storeID  = "636841287823143470"
	reviewer = "qs:result_reviewer"
	operator = "qs:assessment_operator"
)

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func main() {
	if len(os.Args) != 6 {
		fail(errors.New("usage: endpoint ca cert key inspect|plan-revoke|plan-restore|revoke|restore"))
	}
	mode := os.Args[5]
	if mode != "inspect" && mode != "plan-revoke" && mode != "plan-restore" && mode != "revoke" && mode != "restore" {
		fail(errors.New("unknown mode"))
	}
	cert, err := tls.LoadX509KeyPair(os.Args[3], os.Args[4])
	if err != nil {
		fail(err)
	}
	caPEM, err := os.ReadFile(os.Args[2])
	if err != nil {
		fail(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		fail(errors.New("CA PEM invalid"))
	}
	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(connectCtx, os.Args[1], grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}, RootCAs: roots, MinVersion: tls.VersionTLS12})), grpc.WithBlock())
	if err != nil {
		fail(err)
	}
	defer conn.Close()
	client := authzv4.NewAuthorizationServiceClient(conn)
	read := func() *authzv4.GetAuthorizationSnapshotResponse {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		snap, err := client.GetAuthorizationSnapshot(ctx, &authzv4.GetAuthorizationSnapshotRequest{Subject: subject, AppName: "qs", IncludeAssignmentFacts: true})
		if err != nil {
			fail(err)
		}
		if err := validate(snap, mode); err != nil {
			fail(err)
		}
		return snap
	}
	snap := read()
	if mode == "inspect" {
		emit(map[string]any{"time": time.Now().Format(time.RFC3339Nano), "mode": mode, "policy_version": snap.GetPolicyVersion(), "roles": snap.GetDirectRoles(), "assignment_scopes": snap.GetAssignmentScopes()})
		return
	}
	ro := []*authzv4.ScopedRoleAssignment{{RoleName: reviewer, Scope: scope()}}
	if mode == "plan-restore" || mode == "restore" {
		ro = append(ro, &authzv4.ScopedRoleAssignment{RoleName: operator, Scope: scope()})
	}
	req := &authzv4.ReplaceScopedAssignmentsRequest{Subject: subject, OrgId: orgID, Roles: ro, ChangedBy: "service:qs-apiserver.svc", Reason: "reliable-messaging M5-06 production authorization verification", ExpectedPolicyVersion: snap.GetPolicyVersion()}
	if mode == "plan-revoke" || mode == "plan-restore" {
		emit(map[string]any{"time": time.Now().Format(time.RFC3339Nano), "mode": mode, "request": req})
		return
	}
	ctx, cancelWrite := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := client.ReplaceScopedAssignments(ctx, req)
	cancelWrite()
	if err != nil {
		fail(fmt.Errorf("write outcome unknown; inspect before any follow-up: %w", err))
	}
	emit(map[string]any{"time": time.Now().Format(time.RFC3339Nano), "mode": mode, "previous_policy_version": snap.GetPolicyVersion(), "committed_policy_version": resp.GetPolicyVersion(), "changed": resp.GetChanged(), "direct_roles": resp.GetDirectRoles()})
}

func scope() *authzv4.DataScope {
	return &authzv4.DataScope{OrgId: orgID, Kind: authzv4.DataScopeKind_STORES, StoreIds: []string{storeID}}
}

func validate(snap *authzv4.GetAuthorizationSnapshotResponse, mode string) error {
	if snap == nil || snap.GetPolicyVersion() <= 0 || !snap.GetAssignmentFactsComplete() || snap.GetScopeContractVersion() != 1 {
		return errors.New("incomplete authorization snapshot")
	}
	want := []string{reviewer}
	if mode == "inspect" || mode == "plan-revoke" || mode == "revoke" {
		want = append(want, operator)
	}
	got := append([]string(nil), snap.GetDirectRoles()...)
	sort.Strings(got)
	sort.Strings(want)
	if mode != "inspect" && !reflect.DeepEqual(got, want) {
		return fmt.Errorf("roles changed: got %v want %v", got, want)
	}
	if len(snap.GetAssignmentScopes()) != len(got) || len(snap.GetAssignmentFacts()) != len(got) {
		return errors.New("assignment facts do not match direct roles")
	}
	seen := map[string]bool{}
	for _, fact := range snap.GetAssignmentScopes() {
		if fact == nil || fact.Role == nil || fact.Scope == nil || fact.GetAssignmentId() == "" {
			return errors.New("incomplete assignment scope")
		}
		role := fact.Role.GetRoleName()
		if role != reviewer && role != operator || seen[role] || fact.Role.GetManagementProtection() != "standard" {
			return errors.New("unexpected role fact")
		}
		seen[role] = true
		if !reflect.DeepEqual(fact.Scope, scope()) {
			return fmt.Errorf("scope changed for %s", role)
		}
	}
	if mode != "inspect" && len(seen) != len(want) {
		return errors.New("role fact count mismatch")
	}
	return nil
}

func emit(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fail(err)
	}
	fmt.Println(string(b))
}
