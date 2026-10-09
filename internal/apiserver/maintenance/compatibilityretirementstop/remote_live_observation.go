package compatibilityretirementstop

import "context"

// Both tokens are produced only by this same native original pipe's verified
// challenge/reply exchange. A saved DTO, another controller or later run cannot
// reconstruct them. They cover only this fixed D scope, never whole acceptance.
type RemoteRuntimeObservation struct {
	self       *RemoteRuntimeObservation
	controller *RemoteController
	sequence   uint64
	snapshot   DependentRuntimeSnapshot
	seal       string
}
type RemoteMaterialZero struct {
	self       *RemoteMaterialZero
	controller *RemoteController
	sequence   uint64
	snapshot   RemoteMaterialSnapshot
	seal       string
}

func (*RemoteRuntimeObservation) MarshalJSON() ([]byte, error) { return nil, ErrRemoteBudget }
func (*RemoteMaterialZero) MarshalJSON() ([]byte, error)       { return nil, ErrRemoteMaterials }
func (o *RemoteRuntimeObservation) Snapshot() (DependentRuntimeSnapshot, error) {
	if o == nil || o.self != o || o.controller == nil || o.sequence == 0 || o.seal != runtimeDigest(o.snapshot) {
		return DependentRuntimeSnapshot{}, ErrRemoteBudget
	}
	return DependentRuntimeSnapshot{Instances: append([]DependentRuntimeInstance(nil), o.snapshot.Instances...)}, nil
}
func (z *RemoteMaterialZero) Snapshot() (RemoteMaterialSnapshot, error) {
	if z == nil || z.self != z || z.controller == nil || z.sequence == 0 || z.seal != runtimeDigest(z.snapshot) {
		return RemoteMaterialSnapshot{}, ErrRemoteMaterials
	}
	return z.snapshot, nil
}

// ValidateOriginalController consumes only the actual terminal proof issued by
// this exact original controller. A saved snapshot, another controller or an
// earlier sequence cannot establish the terminal D phase.
func (z *RemoteMaterialZero) ValidateOriginalController(ctx context.Context, c *RemoteController) error {
	if ctx == nil || ctx.Err() != nil || z == nil || z.self != z || c == nil || c.self != c {
		return ErrRemoteMaterials
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed || c.materialsZero != z || z.controller != c || z.sequence != c.seq || z.sequence == 0 || z.seal != runtimeDigest(z.snapshot) || z.snapshot.RemainingTemporaryFiles != 0 {
		return ErrRemoteMaterials
	}
	return ctx.Err()
}
func (c *RemoteController) ObserveRunningDependents(ctx context.Context) (*RemoteRuntimeObservation, error) {
	if _, e := c.Do(ctx, "check_running"); e != nil {
		return nil, e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.runtimeObservation == nil || c.runtimeObservation.sequence != c.seq {
		return nil, ErrRemoteBudget
	}
	return c.runtimeObservation, nil
}
func (c *RemoteController) PurgeOwnedMaterials(ctx context.Context, o *RemoteRuntimeObservation) (*RemoteMaterialZero, error) {
	if c == nil || c.self != c || o == nil || o.self != o || o.controller != c {
		return nil, ErrRemoteMaterials
	}
	c.mu.Lock()
	same := !c.closed && c.runtimeObservation == o && c.seq == o.sequence && o.seal == runtimeDigest(o.snapshot)
	c.mu.Unlock()
	if !same {
		return nil, ErrRemoteMaterials
	}
	if _, e := c.Do(ctx, "purge_materials"); e != nil {
		return nil, e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.materialsZero == nil || c.materialsZero.sequence != c.seq {
		return nil, ErrRemoteMaterials
	}
	return c.materialsZero, nil
}
func remoteMaterialsDiagnosticValid(v SessionDiagnostic) bool {
	if v.Action != "purge_materials" || v.Outcome != "observed" {
		return v.Materials == nil
	}
	return v.Materials != nil && hash64.MatchString(v.Materials.ScopeSHA256) && v.Materials.FilesRemoved >= 4 && v.Materials.FilesRemoved <= 8192 && v.Materials.DirectoriesRemoved >= 2 && v.Materials.DirectoriesRemoved <= 4 && v.Materials.RemainingTemporaryFiles == 0
}
