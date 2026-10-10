//go:build integration

package retirement

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestAIHistoricalInputNativeTwoSourceBoundFullDiskEpochs(t *testing.T) {
	sqlDB, client, db, cfg := originNativeDBs(t, false)
	firstSession := snapshotInputNativeSession(t, client)
	var fixture authFixture
	var recipe *HistoricalSourceInputRecipe
	var source1, source2 *HistoricalSourceInputEpoch
	var ai1, ai2 *AIHistoricalInputEpoch
	capture := func(first bool) func(context.Context, *SQLResponsibilitySnapshot, *MongoSnapshotInputEpoch, *gorm.DB) error {
		return func(ctx context.Context, sqlInput *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, tx *gorm.DB) error {
			if first {
				fixture = originNativeCopies(t, ctx, tx, &MongoResponsibilitySnapshot{db: db, metadata: mongoInput.metadata})
			}
			c, err := PrepareHistoricalCoordinator(ctx, coordinatorBinding(), fixture.inputs(), DefaultHistoricalCoordinatorLimits())
			if err != nil {
				return err
			}
			if first {
				binding, e := c.BindOriginCopies(ctx, fixture.inputs(), DefaultSourceOriginLimits())
				if e != nil {
					return e
				}
				binding.limits.PageRows = 2
				recipe, e = FreezeHistoricalSourceInputRecipe(ctx, binding)
				if e != nil {
					return e
				}
			}
			source, err := PrepareHistoricalSourceInputEpoch(ctx, recipe, sqlInput, mongoInput, snapshotInputNativeFile(t), time.Minute)
			if err != nil {
				return err
			}
			limits := DefaultAIReverseLimits()
			limits.PageRows = 2
			limits.MaxRetainedBytes = 64 << 20
			input, err := PrepareAIHistoricalInputEpoch(ctx, c, source, fixture.inputs(), snapshotInputNativeFile(t), limits)
			if err != nil {
				return err
			}
			r := input.Summary()
			if !r.CompleteInput || len(r.Ledgers) != 14 || r.Rows == 0 || r.CASAuthority || r.DropReady || !r.FreshComponentQualificationRequired {
				return errors.New("actual full AI input or authority mismatch")
			}
			// The current synthetic old START rows are still pending. Two matching
			// inputs must retain that blocker instead of silently blessing history.
			if r.Blocking == 0 {
				return errors.New("actual pending source graph was hidden")
			}
			if first {
				source1, ai1 = source, input
			} else {
				source2, ai2 = source, input
			}
			over := limits
			over.MaxRetainedBytes = aiHistoricalInputMaxRetained + 1
			if v, e := PrepareAIHistoricalInputEpoch(ctx, c, source, fixture.inputs(), snapshotInputNativeFile(t), over); e == nil || v != nil {
				return errors.New("unbounded compact memory reservation accepted")
			}
			tiny := limits
			tiny.MaxRetainedBytes = 1
			if v, e := PrepareAIHistoricalInputEpoch(ctx, c, source, fixture.inputs(), snapshotInputNativeFile(t), tiny); !errors.Is(e, ErrAIReverseBounds) || v != nil {
				return fmt.Errorf("actual compact reservation was not enforced: %w", e)
			}
			return nil
		}
	}
	if err := historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, firstSession, capture(true)); err != nil {
		t.Fatal(err)
	}
	// Unrelated real writes create a later server snapshot time without changing
	// the source or current responsibility input selected for the next cycle.
	mongoCycleNativeSheet(t, db, 29999, "", "")
	if _, err := db.Collection("answersheets").DeleteMany(t.Context(), map[string]any{}); err != nil {
		t.Fatal(err)
	}
	secondSession := snapshotInputNativeSession(t, client)
	if err := historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, secondSession, capture(false)); err != nil {
		t.Fatal(err)
	}
	firstSession.EndSession(t.Context())
	secondSession.EndSession(t.Context())
	sources, err := CompareIndependentHistoricalSourceInputs(t.Context(), source1, source2)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := CompareIndependentAIHistoricalInputs(t.Context(), ai1, ai2, sources)
	if err != nil || pair.ValidateFrozen(t.Context()) != nil || !pair.Summary().TwoIndependentInputsMatched || pair.Summary().CASAuthority || pair.Summary().DropReady || pair.Summary().Blocking == 0 {
		t.Fatal("two actual native pure inputs accepted invalid authority", err)
	}
	if _, err = CompareIndependentAIHistoricalInputs(t.Context(), ai1, ai1, sources); err == nil {
		t.Fatal("same AI epoch accepted")
	}
	raw := make([]byte, 1)
	if _, err = ai1.file.ReadAt(raw, ai1.pages[0].Offset); err != nil {
		t.Fatal(err)
	}
	raw[0] ^= 1
	if _, err = ai1.file.WriteAt(raw, ai1.pages[0].Offset); err != nil {
		t.Fatal(err)
	}
	if pair.ValidateFrozen(t.Context()) == nil {
		t.Fatal("same inode and size tamper accepted")
	}
	raw[0] ^= 1
	if _, err = ai1.file.WriteAt(raw, ai1.pages[0].Offset); err != nil {
		t.Fatal(err)
	}
	// A new unknown wire has no credible command/org selector. Preserve the
	// actual new orphan and refuse equal whole input, without claiming it is
	// unrelated or making old source identities disappear.
	wire := []byte("synthetic-unbound-wire")
	if err = sqlDB.Exec("INSERT INTO ai_messaging_quarantine(wire_sha256,wire,code,attempts,first_seen_at,last_seen_at) VALUES(?,?,?,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", sourceSHA(wire), wire, "synthetic_unknown").Error; err != nil {
		t.Fatal(err)
	}
	mongoCycleNativeSheet(t, db, 29999, "", "")
	if _, err = db.Collection("answersheets").DeleteMany(t.Context(), map[string]any{}); err != nil {
		t.Fatal(err)
	}
	thirdSession := snapshotInputNativeSession(t, client)
	if err = historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, thirdSession, capture(false)); err != nil {
		t.Fatal(err)
	}
	thirdSession.EndSession(t.Context())
	changedSources, err := CompareIndependentHistoricalSourceInputs(t.Context(), source1, source2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CompareIndependentAIHistoricalInputs(t.Context(), ai1, ai2, changedSources); !errors.Is(err, ErrAIReverseChanged) {
		t.Fatal("new unknown wire was hidden", err)
	}
	// Pure disk matching still has no effect API and does not renew s.alive,
	// coordinator lifetime, source scope, or any SQL/Mongo write qualification.
}
