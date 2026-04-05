package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// Test helpers
// ============================================================================

func newTestScorer() *RiskScorer {
	return NewRiskScorer(nil) // nil DB => hardcoded defaults
}

func baseInput() RiskScoringInput {
	return RiskScoringInput{
		WorkspaceTier: "production",
	}
}

// ============================================================================
// Test 1: Magnitude boundaries
// ============================================================================

func TestScore_Magnitude(t *testing.T) {
	scorer := newTestScorer()

	tests := []struct {
		name    string
		changes int
		wantDed int
	}{
		{"0 resources", 0, 0},
		{"3 resources", 3, 0},
		{"4 resources", 4, -5},
		{"10 resources", 10, -5},
		{"11 resources", 11, -15},
		{"30 resources", 30, -15},
		{"31 resources", 31, -25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := baseInput()
			input.TotalChanges = tt.changes
			result := scorer.Score(input)

			magDed := 0
			for _, d := range result.Deductions {
				if d.Category == "magnitude" {
					magDed += d.Points
				}
			}
			assert.Equal(t, tt.wantDed, magDed)
		})
	}
}

// ============================================================================
// Test 2: Dependency boundaries
// ============================================================================

func TestScore_Dependencies(t *testing.T) {
	scorer := newTestScorer()

	tests := []struct {
		name    string
		deps    int
		wantDed int
	}{
		{"0 deps", 0, 0},
		{"1 dep", 1, -5},
		{"3 deps", 3, -5},
		{"4 deps", 4, -15},
		{"10 deps", 10, -15},
		{"11 deps", 11, -25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := baseInput()
			input.MaxDirectDependencies = tt.deps
			result := scorer.Score(input)

			depDed := 0
			for _, d := range result.Deductions {
				if d.Category == "blast_radius" && d.Item == "dependencies" {
					depDed += d.Points
				}
			}
			assert.Equal(t, tt.wantDed, depDed)
		})
	}
}

// ============================================================================
// Test 3: Module impact
// ============================================================================

func TestScore_ModuleImpact(t *testing.T) {
	scorer := newTestScorer()

	tests := []struct {
		name     string
		affected int
		total    int
		wantDed  int
	}{
		{"0%", 0, 100, 0},
		{"4.9%", 49, 1000, 0},
		{"5%", 5, 100, -5},
		{"15%", 15, 100, -10},
		{"40%", 40, 100, -10},
		{"41%", 41, 100, -20},
		{"total=0 skip", 10, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := baseInput()
			input.AffectedModuleResourceCount = tt.affected
			input.WorkspaceResourceCount = tt.total
			result := scorer.Score(input)

			modDed := 0
			for _, d := range result.Deductions {
				if d.Item == "module_impact" {
					modDed += d.Points
				}
			}
			assert.Equal(t, tt.wantDed, modDed)
		})
	}
}

// ============================================================================
// Test 4: Cross workspace
// ============================================================================

func TestScore_CrossWorkspace(t *testing.T) {
	scorer := newTestScorer()

	tests := []struct {
		name    string
		count   int
		wantDed int
	}{
		{"0", 0, 0},
		{"1", 1, -8},
		{"2", 2, -8},
		{"3", 3, -15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := baseInput()
			input.CrossWorkspaceCount = tt.count
			result := scorer.Score(input)

			xwsDed := 0
			for _, d := range result.Deductions {
				if d.Item == "cross_workspace" {
					xwsDed += d.Points
				}
			}
			assert.Equal(t, tt.wantDed, xwsDed)
		})
	}
}

// ============================================================================
// Test 5: Risk signals basic
// ============================================================================

func TestScore_RiskSignals(t *testing.T) {
	scorer := newTestScorer()

	t.Run("single factor", func(t *testing.T) {
		input := baseInput()
		input.RiskFactors = []string{"service_disruption"}
		result := scorer.Score(input)

		signalDed := 0
		for _, d := range result.Deductions {
			if d.Category == "risk_signal" && d.Item == "service_disruption" {
				signalDed += d.Points
			}
		}
		assert.Equal(t, -15, signalDed)
	})

	t.Run("multiple factors", func(t *testing.T) {
		input := baseInput()
		input.RiskFactors = []string{"service_disruption", "configuration_drift"}
		result := scorer.Score(input)

		signalDed := 0
		for _, d := range result.Deductions {
			if d.Category == "risk_signal" &&
				(d.Item == "service_disruption" || d.Item == "configuration_drift") {
				signalDed += d.Points
			}
		}
		assert.Equal(t, -18, signalDed)
	})
}

// ============================================================================
// Test 6: Uncertainty
// ============================================================================

func TestScore_Uncertainty(t *testing.T) {
	scorer := newTestScorer()

	tests := []struct {
		name    string
		level   string
		wantDed int
	}{
		{"low", "low", 0},
		{"medium", "medium", -3},
		{"high", "high", -8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := baseInput()
			input.UncertaintyLevel = tt.level
			result := scorer.Score(input)

			uncDed := 0
			for _, d := range result.Deductions {
				if d.Item == "uncertainty" {
					uncDed += d.Points
				}
			}
			assert.Equal(t, tt.wantDed, uncDed)
		})
	}
}

// ============================================================================
// Test 7: High-freq bonus
// ============================================================================

func TestScore_HighFreqBonus(t *testing.T) {
	scorer := newTestScorer()

	tests := []struct {
		name    string
		count   int
		wantDed int
	}{
		{"2 resources - no bonus", 2, 0},
		{"3 resources", 3, -5},
		{"5 resources", 5, -5},
		{"6 resources", 6, -10},
		{"10 resources", 10, -10},
		{"11 resources", 11, -15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := baseInput()
			input.RiskFactors = []string{"service_disruption"}
			input.RiskFactorResourceCounts = map[string]int{
				"service_disruption": tt.count,
			}
			result := scorer.Score(input)

			hfDed := 0
			for _, d := range result.Deductions {
				if d.Item == "service_disruption_high_freq" {
					hfDed += d.Points
				}
			}
			assert.Equal(t, tt.wantDed, hfDed)
		})
	}
}

// ============================================================================
// Test 8: Combo multiplier
// ============================================================================

func TestScore_ComboMultiplier(t *testing.T) {
	scorer := newTestScorer()

	t.Run("3 high-risk factors triggers combo", func(t *testing.T) {
		input := baseInput()
		input.RiskFactors = []string{
			"service_disruption",
			"external_exposure_change",
			"dependency_break",
		}
		result := scorer.Score(input)
		assert.True(t, result.ComboMultiplierApplied)

		// base risk signal: -15 + -12 + -10 = -37
		// combo extra: round(-37 * 0.5) = round(-18.5) = -19
		comboDed := 0
		for _, d := range result.Deductions {
			if d.Item == "combo_multiplier" {
				comboDed += d.Points
			}
		}
		assert.Equal(t, -19, comboDed)
	})

	t.Run("2 high-risk factors no combo", func(t *testing.T) {
		input := baseInput()
		input.RiskFactors = []string{
			"service_disruption",
			"external_exposure_change",
		}
		result := scorer.Score(input)
		assert.False(t, result.ComboMultiplierApplied)
	})
}

// ============================================================================
// Test 9: Env multiplier
// ============================================================================

func TestScore_EnvMultiplier(t *testing.T) {
	scorer := newTestScorer()

	tests := []struct {
		name     string
		tier     string
		wantMult float64
	}{
		{"critical", "critical", 1.5},
		{"production", "production", 1.3},
		{"staging", "staging", 1.0},
		{"development", "development", 0.7},
		{"unknown", "sandbox", 1.3},
		{"empty", "", 1.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := baseInput()
			input.WorkspaceTier = tt.tier
			input.TotalChanges = 5
			result := scorer.Score(input)
			assert.Equal(t, tt.wantMult, result.EnvMultiplier)
		})
	}
}

// ============================================================================
// Test 10: Near threshold
// ============================================================================

func TestScore_NearThreshold(t *testing.T) {
	scorer := newTestScorer()

	t.Run("score 100 not near", func(t *testing.T) {
		input := RiskScoringInput{WorkspaceTier: "staging"}
		result := scorer.Score(input)
		assert.Equal(t, 100.0, result.FinalScore)
		assert.False(t, result.NearThreshold)
	})

	t.Run("near 80 boundary", func(t *testing.T) {
		input := RiskScoringInput{
			WorkspaceTier:         "staging",
			TotalChanges:          11,
			MaxDirectDependencies: 2,
			RiskFactors:           []string{"configuration_drift"},
		}
		// -15 + -5 + -3 = -23, score = 77, within 3 of 80
		result := scorer.Score(input)
		assert.True(t, result.NearThreshold)
	})
}

// ============================================================================
// Test 11: severityGap
// ============================================================================

func TestSeverityGap(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"low", "low", 0},
		{"low", "medium", 1},
		{"low", "critical", 3},
		{"critical", "low", 3},
		{"medium", "high", 1},
		{"high", "critical", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, severityGap(tt.a, tt.b))
		})
	}
}

// ============================================================================
// Test 12: maxSeverity
// ============================================================================

func TestMaxSeverity(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{"low", "high", "high"},
		{"critical", "low", "critical"},
		{"medium", "medium", "medium"},
		{"high", "critical", "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, maxSeverity(tt.a, tt.b))
		})
	}
}
