package compatibilityretirementstop

import (
	"context"
	"path/filepath"
)

// CheckStoppedDependents is a read-only post-inline-deployment observation.
// It requires the original successful A stop lease, original window and actual
// full relevant-container catalog. The host's original native API owner must
// separately verify the supplied actual CID/image/config; this method grants
// neither API acceptance nor whole-writer fencing. Check and all recovery
// methods continue to enforce their existing scopes without this exception.
func (l *Lease) CheckStoppedDependents(ctx context.Context, nativeInlineAPIID string) error {
	if l == nil || l.self != l {
		return ErrBinding
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.failed || ctx == nil || ctx.Err() != nil || l.approval == nil || l.approval.descriptor.HostRole != "server-a" || checkWindow(ctx, l.approval, l.window) != nil {
		return ErrBinding
	}
	bounded, cancel, e := l.window.ForwardContext(ctx)
	if e != nil {
		return ErrBinding
	}
	defer cancel()
	actual, e := l.approval.catalog(bounded)
	if e != nil {
		return e
	}
	dependents, e := stoppedDependents(l.baseline, actual, l.stopped, nativeInlineAPIID)
	if e != nil {
		return e
	}
	for _, v := range dependents {
		if !l.stopped[v.ID] {
			// An originally stopped instance has no issued stop requirement;
			// any actual orphan or pending journal is still rejected.
			if e = l.noPendingStop(v); e != nil {
				return e
			}
			continue
		}
		intent, ie := readProtected(filepath.Join(l.dir, v.ID+".stop-intent.json"))
		result, re := readProtected(filepath.Join(l.dir, v.ID+".stop-result.json"))
		if ie != nil || re != nil || validateStopCompletion(intent, result, false, v) != nil {
			return ErrJournal
		}
	}
	return bounded.Err()
}

// Pure comparison only: no imported catalog can construct a Lease or authorize
// an effect. Filtering permits exactly the owner's single actual replacement
// API; it rejects a residual original API, additional related containers and
// every dependent change instead of hiding them in a JOIN/filtered count.
func stoppedDependents(baseline []Container, actual []actualContainer, stopped map[string]bool, nativeInlineAPIID string) ([]actualContainer, error) {
	if !hash64.MatchString(nativeInlineAPIID) || len(baseline) < 2 || len(actual) != len(baseline) {
		return nil, ErrState
	}
	originals := make([]Container, 0, len(baseline)-1)
	observed := make([]actualContainer, 0, len(actual)-1)
	oldAPIs, newAPIs := 0, 0
	for _, v := range baseline {
		if v.Component == "qs-apiserver" {
			oldAPIs++
			if v.ID == nativeInlineAPIID {
				return nil, ErrState
			}
		} else {
			if v.Component != "qs-collection-server" {
				return nil, ErrState
			}
			originals = append(originals, v)
		}
	}
	for _, v := range actual {
		if v.Component == "qs-apiserver" {
			newAPIs++
			if v.ID != nativeInlineAPIID || !validDependentScopeAPI(v.Container) {
				return nil, ErrState
			}
		} else {
			observed = append(observed, v)
		}
	}
	if oldAPIs != 1 || newAPIs != 1 || len(observed) != len(originals) {
		return nil, ErrState
	}
	for i, v := range observed {
		original := originals[i]
		if !sameIdentity(v, original) || v.StartedAt != original.StartedAt || v.Running || v.PID != 0 || v.Paused || v.Restarting || v.Dead || v.OOMKilled || original.Running && (!stopped[v.ID] || v.ExitCode != 0) {
			return nil, ErrState
		}
	}
	return observed, nil
}
