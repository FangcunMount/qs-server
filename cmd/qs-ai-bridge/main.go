// qs-ai-bridge is an internal integration entry, not a participant API.
// Stage input must come from an authorized business caller. No user-auth bypass route is registered.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	sender "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	receiver "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/aibridge"
	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "AI bridge operation failed")
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "", "stage-start, stage-change, relay, receive, projection")
	address := flag.String("address", "", "bind address or AI target")
	input := flag.String("input", "", "JSON command file")
	id := flag.String("request-id", "", "business request ID")
	ca := flag.String("ca", "", "CA file")
	cert := flag.String("cert", "", "certificate")
	key := flag.String("key", "", "private key")
	flag.Parse()
	ctx := context.Background()
	db, err := sql.Open("mysql", os.Getenv("QS_AI_BRIDGE_DSN"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(5)
	s := &app.Service{Store: &store.Store{DB: db}}
	switch *mode {
	case "stage-start":
		var r app.Start
		if err = read(*input, &r); err != nil {
			return err
		}
		return s.Start(ctx, r)
	case "stage-change":
		var r app.Change
		if err = read(*input, &r); err != nil {
			return err
		}
		return s.Change(ctx, *id, r)
	case "projection":
		v, e := s.Store.Projection(ctx, *id)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "relay", "receive":
		pair, e := tls.LoadX509KeyPair(*cert, *key)
		if e != nil {
			return e
		}
		roots, e := os.ReadFile(*ca)
		if e != nil {
			return e
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(roots) {
			return fmt.Errorf("invalid CA")
		}
		config := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, RootCAs: pool}
		if *mode == "relay" {
			conn, e := grpc.NewClient(*address, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDisableRetry())
			if e != nil {
				return e
			}
			defer func() { _ = conn.Close() }()
			s.Sender = sender.New(conn)
			n, e := s.Relay(ctx)
			if e != nil {
				return e
			}
			fmt.Println(n)
			return nil
		}
		config.ClientCAs = pool
		config.ClientAuth = tls.RequireAndVerifyClientCert
		server := grpc.NewServer(grpc.Creds(credentials.NewTLS(config)), grpc.MaxRecvMsgSize(65536))
		defer server.Stop()
		pb.RegisterResultsServer(server, &receiver.Receiver{Service: s})
		listener, e := net.Listen("tcp", *address)
		if e != nil {
			return e
		}
		fmt.Println("LISTENING", listener.Addr().String())
		return server.Serve(listener)
	default:
		return fmt.Errorf("mode required")
	}
}
func read(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}
