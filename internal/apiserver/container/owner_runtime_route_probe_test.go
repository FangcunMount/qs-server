//go:build integration && m3_host_runtime

package container

// This owned-fixture probe follows the pinned5.1.4 signing contract. It only
// queries routes; no Producer, Checker, consumer poller or DB session is made.
import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"time"

	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	mqmeta "github.com/apache/rocketmq-clients/golang/v5/metadata"
	pb "github.com/apache/rocketmq-clients/golang/v5/protocol/v2"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func ownedRouteSignature(ctx context.Context, c rocketmq.Config, client string, at time.Time) context.Context {
	stamp := at.UTC().Format("20060102T150405Z")
	mac := hmac.New(sha1.New, []byte(c.SecretKey))
	_, _ = mac.Write([]byte(stamp))
	authorization := fmt.Sprintf("%s %s=%s//%s, %s=%s, %s=%s", mqmeta.EncryptHeader, mqmeta.Credential, c.AccessKey, mqmeta.Rocketmq, mqmeta.SignedHeaders, mqmeta.DateTime, mqmeta.Signature, hex.EncodeToString(mac.Sum(nil)))
	return metadata.AppendToOutgoingContext(ctx, mqmeta.LanguageKey, mqmeta.LanguageValue, mqmeta.ProtocolKey, mqmeta.ProtocolValue, mqmeta.RequestID, uuid.NewString(), mqmeta.VersionKey, mqmeta.VersionValue, mqmeta.ClintID, client, mqmeta.NameSpace, "", mqmeta.DateTime, stamp, mqmeta.Authorization, authorization)
}

type ownedRouteStatus struct{ code int32 }

func (e ownedRouteStatus) Error() string  { return "owned authenticated Proxy route unavailable" }
func (e ownedRouteStatus) GetCode() int32 { return e.code }

type ownedRouteQuery struct {
	config        rocketmq.Config
	topic, client string
	ownerIndex    int
}

func ownedProxyRouteQueries(base, observer *generic.RocketMQOptions) ([]ownedRouteQuery, error) {
	var queries []ownedRouteQuery
	for _, role := range []struct {
		owner string
		index int
	}{{"", -1}, {"qs.mysql", 0}, {"qs.mongo", 1}} {
		options := *base
		var topics []string
		if role.owner == "" {
			options = *observer
			topics = []string{"qs.evaluation.lifecycle"}
		} else {
			options.CredentialsFile = base.Transactions[role.owner].CredentialsFile
		}
		c, err := options.Load(topics, 1, 5*time.Second)
		if err != nil {
			return nil, err
		}
		if c.Endpoint != base.Endpoint || options.CAFile != base.CAFile {
			return nil, ownedRouteStatus{}
		}
		for _, route := range c.Routes {
			if route.MessageType != transport.TransactionMessageType || (role.owner != "" && route.Owner != role.owner) {
				continue
			}
			queries = append(queries, ownedRouteQuery{c, route.PhysicalTopic, "m3-readonly-route-probe-" + strconv.Itoa(role.index), role.index})
		}
	}
	if len(queries) != 7 {
		return nil, ownedRouteStatus{}
	}
	return queries, nil
}

func waitOwnedProxyRoutes(ctx context.Context, base, observer *generic.RocketMQOptions) (int, error) {
	queries, err := ownedProxyRouteQueries(base, observer)
	if err != nil {
		return -1, err
	}
	host, port, err := net.SplitHostPort(base.Endpoint)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return -1, ownedRouteStatus{}
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return -1, ownedRouteStatus{}
	}
	conn, err := grpc.NewClient(base.Endpoint, grpc.WithTransportCredentials(grpccreds.NewTLS(queries[0].config.TLS)))
	if err != nil {
		return -1, err
	}
	defer func() { _ = conn.Close() }()
	stub := pb.NewMessagingServiceClient(conn)
	// Proxy5.5.1 caches a not-found route and refreshes after20s. A ten-try
	// two-second loop could expire before its first async refresh. This owned
	// restart readiness window covers registration/cache refresh, not sends.
	deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	endpoints := &pb.Endpoints{Scheme: pb.AddressScheme_IPv4, Addresses: []*pb.Address{{Host: host, Port: int32(number)}}}
	var last error
	lastOwner := -1
	for attempt := 0; attempt < 46; attempt++ {
		all := true
		for _, query := range queries {
			call, done := context.WithTimeout(deadline, 3*time.Second)
			response, e := stub.QueryRoute(ownedRouteSignature(call, query.config, query.client, time.Now()), &pb.QueryRouteRequest{Topic: &pb.Resource{Name: query.topic}, Endpoints: endpoints})
			done()
			lastOwner = query.ownerIndex
			if e != nil {
				last = e
				all = false
				if code := status.Code(e); code == codes.Unauthenticated || code == codes.PermissionDenied || code == codes.InvalidArgument {
					return lastOwner, last
				}
				break
			}
			if response.GetStatus().GetCode() != pb.Code_OK || len(response.GetMessageQueues()) == 0 {
				code := response.GetStatus().GetCode()
				last = ownedRouteStatus{int32(code)}
				all = false
				switch code {
				case pb.Code_OK, pb.Code_TOPIC_NOT_FOUND, pb.Code_NOT_FOUND, pb.Code_INTERNAL_ERROR, pb.Code_INTERNAL_SERVER_ERROR, pb.Code_REQUEST_TIMEOUT, pb.Code_PROXY_TIMEOUT:
				default:
					return lastOwner, last
				}
				break
			}
		}
		if all {
			return -1, nil
		}
		if attempt == 45 {
			break
		}
		select {
		case <-deadline.Done():
			return lastOwner, last
		case <-time.After(2 * time.Second):
		}
	}
	return lastOwner, last
}
