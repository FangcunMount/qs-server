package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
)

// The CLI is a privileged maintenance client authenticated by the service's
// mTLS credentials. ActorID is audited and checked remotely, not trusted as a role.
func maintenanceIAMOptions(getenv func(string) string) (*iam.IAMOptions, error) {
	address, ca, cert, key := getenv("TESTEE_STORE_IAM_ADDRESS"), getenv("TESTEE_STORE_IAM_CA"), getenv("TESTEE_STORE_IAM_CERT"), getenv("TESTEE_STORE_IAM_KEY")
	if address == "" || ca == "" || cert == "" || key == "" {
		return nil, fmt.Errorf("IAM address and mTLS CA/certificate/key paths required for writes")
	}
	return &iam.IAMOptions{Enabled: true, GRPCEnabled: true, AuthzAppName: "qs", GRPC: &iam.GRPCOptions{Address: address, Timeout: 10 * time.Second, TLS: &iam.TLSOptions{Enabled: true, CAFile: ca, CertFile: cert, KeyFile: key}}}, nil
}

type freshSnapshotLoader interface {
	LoadFresh(context.Context, string) (*authz.Snapshot, error)
}

func authorizeMaintenance(ctx context.Context, loader freshSnapshotLoader, actor int64) (context.Context, error) {
	if actor <= 0 || loader == nil {
		return nil, fmt.Errorf("maintenance actor and IAM loader required")
	}
	snapshot, err := loader.LoadFresh(ctx, strconv.FormatInt(actor, 10))
	if err != nil {
		return nil, fmt.Errorf("cannot read current IAM administrator authorization")
	}
	if snapshot == nil || snapshot.AuthzVersion <= 0 || !snapshot.IsQSAdmin() {
		return nil, fmt.Errorf("actor lacks current headquarters management permission")
	}
	return authz.WithSnapshot(ctx, snapshot), nil
}
