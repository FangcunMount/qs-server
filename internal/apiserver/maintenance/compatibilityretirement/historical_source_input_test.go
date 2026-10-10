package retirement

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestHistoricalSourceInputFrozenRecipeDoesNotRenewOldCapability(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	copies, err := VerifySourceCopies(t.Context(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BindOriginCopies(t.Context(), copies, f.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := FreezeHistoricalSourceInputRecipe(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}
	started := binding.started
	binding.started = started.Add(-2 * time.Hour)
	if binding.alive(t.Context()) == nil || !recipe.valid() || recipe.binding.started != started {
		t.Fatal("input freeze renewed or requires old authority")
	}
	if _, err = FreezeHistoricalSourceInputRecipe(t.Context(), binding); err == nil {
		t.Fatal("expired old binding minted new input")
	}
	for _, v := range []any{recipe, &HistoricalSourceInputEpoch{}, &HistoricalSourceInputPair{}} {
		if _, err = json.Marshal(v); err == nil {
			t.Fatal("opaque input serialized")
		}
	}
	altered := *recipe
	if altered.valid() {
		t.Fatal("copied recipe pointer accepted")
	}
	recipe.binding.expected[0].Records++
	if recipe.valid() {
		t.Fatal("modified baseline accepted")
	}
	if _, err = CompareIndependentHistoricalSourceInputs(context.Background(), nil, nil); err == nil {
		t.Fatal("absent full inputs accepted")
	}
}
