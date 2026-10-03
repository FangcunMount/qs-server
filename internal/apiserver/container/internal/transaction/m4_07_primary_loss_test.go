//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration && m4_07_new_chain

package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type m407MemberState struct {
	Member   string `json:"member"`
	Writable bool   `json:"writable"`
	Error    bool   `json:"error"`
}

type m407PrimarySample struct {
	At      time.Time         `json:"at"`
	Members []m407MemberState `json:"members"`
}

type m407PrimaryLoss struct {
	clients    map[string]*mongo.Client
	oldPrimary string
	started    time.Time
	mu         sync.Mutex
	samples    []m407PrimarySample
	electedAt  time.Time
	elected    string
	done       chan struct{}
	cancel     context.CancelFunc
}

func (p *m407PrimaryLoss) sample(ctx context.Context) m407PrimarySample {
	s := m407PrimarySample{At: time.Now()}
	for _, host := range []string{"mongo:27017", "mongo2:27017", "mongo3:27017"} {
		var hello struct {
			Writable bool `bson:"isWritablePrimary"`
		}
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := p.clients[host].Database("admin").RunCommand(c, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
		cancel()
		s.Members = append(s.Members, m407MemberState{Member: host, Writable: hello.Writable, Error: err != nil})
	}
	return s
}

// Election freezing changes only the three disposable servers. The business
// client retains its original selection, transaction and pool policies.
func beginM407PrimaryLoss(ctx context.Context) (*m407PrimaryLoss, error) {
	p := &m407PrimaryLoss{clients: make(map[string]*mongo.Client), done: make(chan struct{})}
	for _, host := range []string{"mongo:27017", "mongo2:27017", "mongo3:27017"} {
		client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://"+host+"/?directConnection=true").SetMaxPoolSize(2).SetServerSelectionTimeout(2*time.Second))
		if err != nil {
			p.close()
			return nil, err
		}
		p.clients[host] = client
	}
	initial := p.sample(ctx)
	for _, member := range initial.Members {
		if member.Error {
			p.close()
			return nil, fmt.Errorf("initial direct hello failed")
		}
		if member.Writable {
			if p.oldPrimary != "" {
				p.close()
				return nil, fmt.Errorf("multiple initial primaries")
			}
			p.oldPrimary = member.Member
		}
	}
	if p.oldPrimary == "" {
		p.close()
		return nil, fmt.Errorf("initial primary missing")
	}
	for host, client := range p.clients {
		if host == p.oldPrimary {
			continue
		}
		if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetFreeze", Value: 20}}).Err(); err != nil {
			p.close()
			return nil, err
		}
	}
	stepCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	// Disconnect is permissible only if all three actual hello replies below
	// independently prove that there is no writable primary.
	_ = p.clients[p.oldPrimary].Database("admin").RunCommand(stepCtx, bson.D{{Key: "replSetStepDown", Value: 90}, {Key: "force", Value: true}}).Err()
	cancel()
	first := p.sample(ctx)
	for _, member := range first.Members {
		if member.Error || member.Writable {
			p.close()
			return nil, fmt.Errorf("three actual non-writable replies required")
		}
	}
	p.started = first.At
	p.samples = append(p.samples, first)
	watchCtx, stop := context.WithCancel(ctx)
	p.cancel = stop
	go func() {
		defer close(p.done)
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-time.After(time.Second):
			}
			s := p.sample(watchCtx)
			p.mu.Lock()
			p.samples = append(p.samples, s)
			for _, member := range s.Members {
				if !member.Error && member.Writable && p.electedAt.IsZero() {
					p.electedAt = s.At
					p.elected = member.Member
				}
			}
			elected := !p.electedAt.IsZero()
			p.mu.Unlock()
			if elected {
				return
			}
		}
	}()
	return p, nil
}

func (p *m407PrimaryLoss) election() (time.Time, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.electedAt, p.elected
}
func (p *m407PrimaryLoss) timeline() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, _ := json.Marshal(p.samples)
	return string(b)
}
func (p *m407PrimaryLoss) close() {
	if p.cancel != nil {
		p.cancel()
		<-p.done
	}
	for _, c := range p.clients {
		_ = c.Disconnect(context.Background())
	}
}
