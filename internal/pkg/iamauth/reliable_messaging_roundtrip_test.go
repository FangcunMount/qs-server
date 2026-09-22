//go:build reliable_messaging

package iamauth_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/stretchr/testify/require"
)

// Invoked by IAM's isolated business proof; no production endpoint is accepted
// implicitly. The IAM peer serves its actual authorization RPC implementation.
func TestReliableMessagingIAMPolicyRoundtrip(t *testing.T) {
	require.Equal(t, "1", os.Getenv("RM_BUSINESS_REQUIRED"))
	endpoint, control, lookup := os.Getenv("RM_IAM_GRPC"), os.Getenv("RM_IAM_CONTROL"), os.Getenv("RM_NSQ_LOOKUP")
	require.NotEmpty(t, endpoint)
	require.NotEmpty(t, control)
	require.NotEmpty(t, lookup)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	opts := &iaminfra.IAMOptions{Enabled: true, GRPCEnabled: true, GRPC: &iaminfra.GRPCOptions{
		Address: endpoint, Timeout: 2 * time.Second, RetryMax: 1,
		TLS: &iaminfra.TLSOptions{Enabled: true, CAFile: os.Getenv("RM_TLS_CA"), CertFile: os.Getenv("RM_TLS_CERT"), KeyFile: os.Getenv("RM_TLS_KEY")},
	}}
	client, err := iaminfra.NewClientWithRuntimeOptions(ctx, opts, iaminfra.ClientRuntimeOptions{})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	loader := iamauth.NewSnapshotLoader(client, iamauth.SnapshotLoaderOptions{AppName: "qs", CacheTTL: time.Hour})
	checker := iamauth.NewActionChecker(client)
	require.NotNil(t, checker)
	var failures atomic.Int32
	settings, err := eventtransport.NewSubscriberOptions(1, 5, func(context.Context, basemessaging.FailedMessage) error {
		failures.Add(1)
		return fmt.Errorf("unexpected delivery exhaustion in business proof")
	})
	require.NoError(t, err)
	subscriber, err := eventtransport.NewSubscriber(eventtransport.SubscriberConfig{Provider: "nsq", NSQLookupdAddr: lookup}, settings)
	require.NoError(t, err)
	defer func() { require.NoError(t, subscriber.Close()) }()
	channel := iamauth.DefaultVersionSyncChannel("rm-business-qs")
	require.NoError(t, iamauth.SubscribeVersionChanges(ctx, subscriber, iamauth.DefaultVersionTopic, channel, loader))
	const resource = "qs:assessment:collection:probe"
	var observed string
	check := func(version int64, allowed bool) bool {
		snapshot, e := loader.Load(ctx, "2")
		observed = fmt.Sprintf("snapshot=%+v error=%v", snapshot, e)
		if e != nil || snapshot.AuthzVersion != version || snapshot.HasResourceAction(resource, "read") != allowed {
			return false
		}
		decision, e := checker.CheckAction(ctx, appauthz.ActionCheckRequest{Subject: "user:2", Resource: resource, Action: "read"})
		observed += fmt.Sprintf(" decision=%+v error=%v", decision, e)
		return e == nil && decision.PolicyVersion == version && decision.Allowed == allowed
	}
	defer func() { t.Log(observed) }()
	require.Eventually(t, func() bool { return check(1, true) }, 5*time.Second, 20*time.Millisecond)
	// The parent waits for both actual NSQ channels before mutating the grant.
	for index, phase := range []string{"revoke", "restore"} {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, control+"/"+phase, nil)
		require.NoError(t, err)
		response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusNoContent, response.StatusCode)
		started := time.Now()
		require.Eventually(t, func() bool { return check(int64(index+2), phase == "restore") }, 10*time.Second, 20*time.Millisecond)
		t.Logf("%s: IAM RPC decision and QS cached permission agree at version %d after %s", phase, index+2, time.Since(started))
	}
	require.Zero(t, failures.Load(), "this normal-flow proof does not exercise dead-letter recovery")
}
