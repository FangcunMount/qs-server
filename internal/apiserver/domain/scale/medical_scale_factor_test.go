package scale

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestMedicalScaleAddUpdateRemoveFactor(t *testing.T) {
	t.Parallel()

	m := newTestMedicalScale(t)
	factor := newTestFactor(t, "F1")

	if err := m.AddFactor(factor); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}
	if err := m.AddFactor(factor); err == nil {
		t.Fatal("expected duplicate factor error")
	}

	updated := newTestFactor(t, "F1", WithFactorType(FactorTypeMultilevel))
	if err := m.UpdateFactor(updated); err != nil {
		t.Fatalf("UpdateFactor() error = %v", err)
	}
	got, ok := m.FindFactorByCode(NewFactorCode("F1"))
	if !ok {
		t.Fatal("expected updated factor")
	}
	if got.GetFactorType() != FactorTypeMultilevel {
		t.Fatalf("factor type = %q, want %q", got.GetFactorType(), FactorTypeMultilevel)
	}

	if err := m.RemoveFactor(NewFactorCode("F1")); err != nil {
		t.Fatalf("RemoveFactor() error = %v", err)
	}
	if m.FactorCount() != 0 {
		t.Fatalf("factor count = %d, want 0", m.FactorCount())
	}
}

func TestMedicalScaleReplaceFactorsRejectsDuplicateAndMultipleTotalScore(t *testing.T) {
	t.Parallel()

	t.Run("duplicate factor code", func(t *testing.T) {
		t.Parallel()

		m := newTestMedicalScale(t)
		if err := m.ReplaceFactors([]*Factor{
			newTestFactor(t, "F1"),
			newTestFactor(t, "F1"),
		}); err == nil {
			t.Fatal("expected duplicate factor code error")
		}
	})

	t.Run("multiple total score factors", func(t *testing.T) {
		t.Parallel()

		m := newTestMedicalScale(t)
		if err := m.ReplaceFactors([]*Factor{
			newTestFactor(t, "TOTAL_1", WithIsTotalScore(true)),
			newTestFactor(t, "TOTAL_2", WithIsTotalScore(true)),
		}); err == nil {
			t.Fatal("expected multiple total score factors error")
		}
	})

	t.Run("valid replacement", func(t *testing.T) {
		t.Parallel()

		m := newTestMedicalScale(t)
		if err := m.ReplaceFactors([]*Factor{
			newTestFactor(t, "TOTAL", WithIsTotalScore(true)),
			newTestFactor(t, "F1"),
		}); err != nil {
			t.Fatalf("ReplaceFactors() error = %v", err)
		}
		if m.FactorCount() != 2 {
			t.Fatalf("factor count = %d, want 2", m.FactorCount())
		}
	})
}

func TestMedicalScaleUpdateFactorInterpretRulesValidatesRules(t *testing.T) {
	t.Parallel()

	m := newTestMedicalScale(t)
	if err := m.AddFactor(newTestFactor(t, "F1")); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}

	validRule := NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", "watch")
	if err := m.UpdateFactorInterpretRules(NewFactorCode("F1"), []InterpretationRule{validRule}); err != nil {
		t.Fatalf("UpdateFactorInterpretRules() error = %v", err)
	}
	factor, ok := m.FindFactorByCode(NewFactorCode("F1"))
	if !ok {
		t.Fatal("expected factor")
	}
	if len(factor.GetInterpretRules()) != 1 {
		t.Fatalf("interpret rule count = %d, want 1", len(factor.GetInterpretRules()))
	}
	if factor.GetInterpretRules()[0].GetRiskLevel() != RiskLevelLow {
		t.Fatalf("risk level = %q, want %q", factor.GetInterpretRules()[0].GetRiskLevel(), RiskLevelLow)
	}

	invalidRule := NewInterpretationRule(NewScoreRange(10, 0), RiskLevelLow, "invalid", "")
	if err := m.UpdateFactorInterpretRules(NewFactorCode("F1"), []InterpretationRule{invalidRule}); err == nil {
		t.Fatal("expected invalid interpretation rule error")
	}
}

func newTestMedicalScale(t *testing.T) *MedicalScale {
	t.Helper()

	m, err := NewMedicalScale(meta.NewCode("SCALE_A"), "Scale A")
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return m
}

func newTestFactor(t *testing.T, code string, opts ...FactorOption) *Factor {
	t.Helper()

	factor, err := NewFactor(NewFactorCode(code), "Factor "+code, opts...)
	if err != nil {
		t.Fatalf("NewFactor() error = %v", err)
	}
	return factor
}
