package assessmententry

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/testeestore"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Probe without a clinician lock, then acquire store -> clinician -> entry -> testee.
// resolveEntry rechecks the clinician's store after its lock is acquired.
func (u *intakeUseCase) lockIntakeStore(ctx context.Context, token string) (*store.Store, error) {
	if u.service.ownership == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "store ownership service unavailable")
	}
	entry, err := u.service.repo.FindByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	id, err := u.service.ownership.ReadClinicianStore(ctx, entry.OrgID(), entry.ClinicianID().Uint64())
	if err != nil {
		return nil, err
	}
	if id == nil {
		return nil, errors.WithCode(code.ErrConflict, "医生尚未配置服务门店，请联系机构")
	}
	target, err := u.service.ownership.LockStore(ctx, entry.OrgID(), *id)
	if err != nil {
		return nil, err
	}
	if !target.IsActive() {
		return nil, errors.WithCode(code.ErrConflict, "服务门店已停用，请联系机构")
	}
	return target, nil
}

func (u *intakeUseCase) assignIntakeStore(ctx context.Context, state *intakeState, target *store.Store, historyID uint64) error {
	subject, err := u.service.ownership.LockTestee(ctx, state.entry.OrgID(), state.testee.ID().Uint64())
	if err != nil {
		return err
	}
	// Existing ownership is preserved regardless of which clinician's QR was scanned.
	// This does not confer access to the new clinician's store.
	if subject.StoreID() != nil {
		state.testee = subject
		return nil
	}
	expected := subject.StoreVersion()
	changed, err := subject.AssignInitialStore(target, expected)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	entryID, clinicianID := state.entry.ID().Uint64(), state.clinician.ID().Uint64()
	h := &port.History{ID: historyID, OrgID: subject.OrgID(), TesteeID: subject.ID().Uint64(), ToStoreID: target.ID(), Kind: "scan_initial", EntryID: &entryID, ClinicianID: &clinicianID, ActorID: 0, CreatedAt: state.intakeAt, Reason: "扫码首次建立服务门店归属", RequestID: fmt.Sprintf("scan-initial:%d", subject.ID()), Version: subject.StoreVersion()}
	if err = u.service.ownership.SaveOwnership(ctx, h, expected); err != nil {
		return err
	}
	if err = u.service.ownership.AppendHistory(ctx, h); err != nil {
		return err
	}
	state.testee = subject
	return nil
}
