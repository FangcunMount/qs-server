package compatibilityretirementstop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"

	reader "github.com/FangcunMount/qs-server/internal/pkg/runtimefactsreader"
)

// This bounded diagnostic contains no message/configuration body or endpoint.
// It reports successful local observation, always with incomplete broker scope.
// It cannot authorize acceptance, a writer fence, replay or material deletion.
type LoadedMQDiagnostic struct {
	ObservationSHA256        string `json:"observation_sha256"`
	Workers                  int    `json:"workers"`
	Nodes                    int    `json:"nodes"`
	Clients                  int    `json:"clients"`
	PublisherHistoryUnproven bool   `json:"publisher_history_unproven"`
	SharedHandoffUnproven    bool   `json:"shared_handoff_unproven"`
	ExternalAIUnproven       bool   `json:"external_ai_unproven"`
	BrokerScopeComplete      bool   `json:"broker_scope_complete"`
}

var ErrLoadedMQ = errors.New("retirement_loaded_mq_observation_failed")

type loadedMQProcess struct{ process reader.BorrowedProcess }

func (p *loadedMQProcess) close() error {
	if p == nil {
		return ErrLoadedMQ
	}
	var e error
	if p.process.Root != nil {
		e = p.process.Root.Close()
		p.process.Root = nil
	}
	if p.process.Proc != nil {
		e = errors.Join(e, p.process.Proc.Close())
		p.process.Proc = nil
	}
	return e
}

// ObserveLoadedMQ borrows the original caller's direct GET client. The actual
// original Lease owns each newly opened proc/root FD and closes it here, before
// returning. Catalog PID and current restored timestamp select the process;
// no request supplies a PID, FD, endpoint, UID or source claim.
func (l *Lease) ObserveLoadedMQ(ctx context.Context, client *http.Client) (result LoadedMQDiagnostic, err error) {
	if l == nil || l.self != l || client == nil {
		return result, ErrLoadedMQ
	}
	observed, e := l.ObserveRunningDependents(ctx)
	if e != nil {
		return result, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.failed || !l.controlledResumed || l.approval == nil || l.approval.descriptor.HostRole != "server-d" || l.runtimeObservation != observed || observed.self != observed || observed.lease != l || observed.seal != runtimeDigest(observed.snapshot) || checkWindow(ctx, l.approval, l.window) != nil {
		return result, ErrLoadedMQ
	}
	q, cancel, e := l.window.ForwardContext(ctx)
	if e != nil {
		return result, ErrLoadedMQ
	}
	defer cancel()
	before, e := l.approval.catalog(q)
	if e != nil {
		return result, e
	}
	selected, e := runningDependents(l.baseline, before, l.restored, "server-d")
	if e != nil {
		return result, e
	}
	programs := map[string]DependentRuntimeInstance{}
	for _, v := range observed.snapshot.Instances {
		programs[v.ContainerID] = v
	}
	type ownedObservation struct {
		ContainerID, ImageID, ProgramSHA256, StateSHA256 string
		Observation                                      reader.Observation
	}
	actual := []ownedObservation{}
	owners := []*loadedMQProcess{}
	defer func() {
		for _, owner := range owners {
			if e := owner.close(); e != nil {
				result = LoadedMQDiagnostic{}
				err = ErrLoadedMQ
			}
		}
	}()
	for _, v := range selected {
		if !v.Running {
			continue
		}
		p, ok := programs[v.ID]
		if !ok || v.Component != "qs-worker" || p.ImageID != v.Image || p.StateSHA256 != runtimeDigest(v) || l.noPendingStop(v) != nil {
			return result, ErrLoadedMQ
		}
		owner, e := openLoadedMQProcess(q, v, l.approval.descriptor.SourceSHA, p.ProgramSHA256)
		if e != nil {
			return result, e
		}
		owners = append(owners, owner)
		observation, readErr := reader.Observe(q, owner.process, client)
		exeErr := verifyLoadedMQExecutable(q, owner, p.ProgramSHA256)
		if !errors.Is(readErr, reader.ErrScopeUnproven) || exeErr != nil {
			return result, ErrLoadedMQ
		}
		if e = accumulateLoadedMQ(&result, observation); e != nil {
			return LoadedMQDiagnostic{}, e
		}
		actual = append(actual, ownedObservation{v.ID, v.Image, p.ProgramSHA256, p.StateSHA256, observation})
	}
	after, e := l.approval.catalog(q)
	if e != nil || !reflect.DeepEqual(before, after) || q.Err() != nil || result.Workers == 0 || len(actual) != len(programs) {
		return LoadedMQDiagnostic{}, ErrLoadedMQ
	}
	// Every original process/root FD remains held through catalog-after and this
	// final live native challenge. A process/namespace/source change cannot be
	// hidden by closing its descriptors before the daemon identity is rechecked.
	for i, owner := range owners {
		snapshot, native, e := reader.QuerySnapshot(q, owner.process)
		expected := actual[i].Observation.Snapshot
		snapshot.ObservedAt, expected.ObservedAt = "", ""
		if e != nil || native != actual[i].Observation.Native || !reflect.DeepEqual(snapshot, expected) || verifyLoadedMQExecutable(q, owner, actual[i].ProgramSHA256) != nil {
			return LoadedMQDiagnostic{}, ErrLoadedMQ
		}
	}
	raw, e := json.Marshal(actual)
	if e != nil || len(raw) > maxBytes {
		return LoadedMQDiagnostic{}, ErrLoadedMQ
	}
	result.ObservationSHA256 = digest(raw)
	if !loadedMQDiagnosticValid(&result) {
		return LoadedMQDiagnostic{}, ErrLoadedMQ
	}
	return result, reader.ErrScopeUnproven
}

func accumulateLoadedMQ(d *LoadedMQDiagnostic, o reader.Observation) error {
	if d == nil || o.Snapshot.Component != "worker" || !o.Snapshot.ObservationComplete || o.Snapshot.BrokerConnectionsVerified || len(o.Nodes) == 0 || len(o.Gaps) == 0 {
		return ErrLoadedMQ
	}
	external := false
	for _, gap := range o.Gaps {
		switch {
		case gap == "external_qs_ai_not_locally_proven":
			external = true
			d.ExternalAIUnproven = true
		case strings.HasPrefix(gap, "publisher_topic_history_not_exhaustive:"):
			d.PublisherHistoryUnproven = true
		case strings.HasPrefix(gap, "shared_handoff_connection_unbound:"):
			d.SharedHandoffUnproven = true
		default:
			return ErrLoadedMQ
		}
	}
	if !external {
		return ErrLoadedMQ
	}
	d.Workers++
	d.Nodes += len(o.Nodes)
	for _, node := range o.Nodes {
		d.Clients += len(node.Clients)
	}
	return nil
}
func loadedMQDiagnosticValid(d *LoadedMQDiagnostic) bool {
	return d != nil && hash64.MatchString(d.ObservationSHA256) && d.Workers > 0 && d.Workers <= 32 && d.Nodes > 0 && d.Nodes <= 2048 && d.Clients >= 0 && d.Clients <= 65536 && d.ExternalAIUnproven && !d.BrokerScopeComplete
}

// Keep the observation transport distinct from a complete runtime proof. The
// returned ScopeUnproven does not replay an action or fabricate a new window.
func (c *RemoteController) ObserveLoadedMQ(ctx context.Context) (LoadedMQDiagnostic, error) {
	d, e := c.Do(ctx, "observe_loaded_mq")
	if e != nil {
		return LoadedMQDiagnostic{}, e
	}
	if !loadedMQDiagnosticValid(d.LoadedMQ) {
		return LoadedMQDiagnostic{}, ErrLoadedMQ
	}
	return *d.LoadedMQ, reader.ErrScopeUnproven
}
