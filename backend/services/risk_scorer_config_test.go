package services

import (
	"encoding/json"
	"testing"

	"iac-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ============================================================================
// Test helpers (config-specific)
// ============================================================================

func setupRiskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SystemConfig{}))
	return db
}

// ============================================================================
// Test 13: validateConfig
// ============================================================================

func TestValidateConfig(t *testing.T) {
	t.Run("valid default passes", func(t *testing.T) {
		cfg := defaultRiskScorerConfig()
		result := validateConfig(cfg)
		assert.Equal(t, cfg, result)
	})

	t.Run("invalid env multiplier returns defaults", func(t *testing.T) {
		cfg := defaultRiskScorerConfig()
		cfg.EnvMultipliers["production"] = 6.0
		result := validateConfig(cfg)
		assert.Equal(t, 1.3, result.EnvMultipliers["production"])
	})

	t.Run("invalid risk factor deduction returns defaults", func(t *testing.T) {
		cfg := defaultRiskScorerConfig()
		cfg.Deductions.RiskFactors["service_disruption"] = 5
		result := validateConfig(cfg)
		assert.Equal(t, -15, result.Deductions.RiskFactors["service_disruption"])
	})

	t.Run("invalid combo multiplier returns defaults", func(t *testing.T) {
		cfg := defaultRiskScorerConfig()
		cfg.ComboMultiplier = 0.5
		result := validateConfig(cfg)
		assert.Equal(t, 1.5, result.ComboMultiplier)
	})

	t.Run("invalid combo min hits returns defaults", func(t *testing.T) {
		cfg := defaultRiskScorerConfig()
		cfg.ComboMinHits = 0
		result := validateConfig(cfg)
		assert.Equal(t, 3, result.ComboMinHits)
	})

	t.Run("invalid thresholds returns defaults", func(t *testing.T) {
		cfg := defaultRiskScorerConfig()
		cfg.DecisionThresholds.Low = 50
		cfg.DecisionThresholds.Medium = 60
		result := validateConfig(cfg)
		assert.Equal(t, 80, result.DecisionThresholds.Low)
	})

	t.Run("invalid default env multiplier returns defaults", func(t *testing.T) {
		cfg := defaultRiskScorerConfig()
		cfg.DefaultEnvMultiplier = 0
		result := validateConfig(cfg)
		assert.Equal(t, 1.3, result.DefaultEnvMultiplier)
	})
}

// ============================================================================
// Test 14: getConfig fallback
// ============================================================================

func TestGetConfig_NilDB(t *testing.T) {
	scorer := NewRiskScorer(nil)
	cfg := scorer.getConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 1.5, cfg.ComboMultiplier)
	assert.Equal(t, -15, cfg.Deductions.RiskFactors["service_disruption"])
}

func TestGetConfig_WithDB(t *testing.T) {
	db := setupRiskTestDB(t)
	scorer := NewRiskScorer(db)

	var record models.SystemConfig
	err := db.Where("key = ?", riskConfigKey).First(&record).Error
	require.NoError(t, err)

	cfg := scorer.getConfig()
	assert.Equal(t, 1.5, cfg.ComboMultiplier)
}

func TestGetConfig_InvalidDBJson(t *testing.T) {
	db := setupRiskTestDB(t)
	db.Create(&models.SystemConfig{
		Key:   riskConfigKey,
		Value: "not-json",
	})

	scorer := &RiskScorer{db: db, cache: &riskConfigCache{}}
	cfg := scorer.getConfig()
	assert.Equal(t, 1.5, cfg.ComboMultiplier)
}

// ============================================================================
// Test 15: Golden cases
// ============================================================================

func TestScore_GoldenCaseA(t *testing.T) {
	scorer := newTestScorer()
	input := RiskScoringInput{
		TotalChanges:          1,
		MaxDirectDependencies: 0,
		WorkspaceTier:         "production",
		UncertaintyLevel:      "low",
	}
	result := scorer.Score(input)
	t.Logf("Case A: score=%.1f level=%s color=%s", result.FinalScore, result.RiskLevel, result.DecisionColor)
	assert.Equal(t, 100.0, result.FinalScore)
	assert.Equal(t, "low", result.RiskLevel)
}

func TestScore_GoldenCaseB(t *testing.T) {
	scorer := newTestScorer()
	input := RiskScoringInput{
		TotalChanges:             1,
		MaxDirectDependencies:    0,
		WorkspaceTier:            "production",
		RiskFactors:              []string{"resource_deletion"},
		RiskFactorResourceCounts: map[string]int{"resource_deletion": 1},
		UncertaintyLevel:         "low",
	}
	result := scorer.Score(input)
	t.Logf("Case B: score=%.1f level=%s", result.FinalScore, result.RiskLevel)
	// resource_deletion: -8, mult 1.3 => 100 - 8*1.3 = 89.6
	assert.Equal(t, 89.6, result.FinalScore)
	assert.Equal(t, "low", result.RiskLevel)
}

func TestScore_GoldenCaseD(t *testing.T) {
	scorer := newTestScorer()
	input := RiskScoringInput{
		TotalChanges:          1,
		MaxDirectDependencies: 0,
		WorkspaceTier:         "production",
		RiskFactors:           []string{"external_exposure_change"},
		UncertaintyLevel:      "low",
	}
	result := scorer.Score(input)
	t.Logf("Case D: score=%.1f level=%s", result.FinalScore, result.RiskLevel)
	// external_exposure_change: -12, mult 1.3 => 100 - 15.6 = 84.4
	assert.Equal(t, 84.4, result.FinalScore)
	assert.Equal(t, "low", result.RiskLevel)
}

func TestScore_GoldenCaseE(t *testing.T) {
	scorer := newTestScorer()
	input := RiskScoringInput{
		TotalChanges:          1,
		MaxDirectDependencies: 0,
		WorkspaceTier:         "production",
		RiskFactors: []string{
			"external_exposure_change",
			"sensitive_resource_change",
		},
		RiskFactorResourceCounts: map[string]int{
			"external_exposure_change": 1,
			"sensitive_resource_change": 1,
		},
		UncertaintyLevel: "medium",
	}
	result := scorer.Score(input)
	// risk: -12+-8=-20, uncertainty: -3, total: -23, *1.3=29.9 => 70.1
	t.Logf("Case E: score=%.1f level=%s", result.FinalScore, result.RiskLevel)
	assert.Equal(t, 70.1, result.FinalScore)
	assert.Equal(t, "medium", result.RiskLevel)
}

func TestScore_GoldenCaseG(t *testing.T) {
	scorer := newTestScorer()
	input := RiskScoringInput{
		TotalChanges:                1,
		MaxDirectDependencies:       50,
		MaxDependencyResource:       "aws_vpc.main",
		AffectedModuleResourceCount: 80,
		WorkspaceResourceCount:      100,
		CrossWorkspaceCount:         5,
		WorkspaceTier:               "production",
		RiskFactors: []string{
			"service_disruption",
			"resource_deletion",
			"dependency_break",
			"external_exposure_change",
			"permission_scope_change",
		},
		RiskFactorResourceCounts: map[string]int{
			"service_disruption": 50,
			"resource_deletion":  1,
		},
		UncertaintyLevel: "high",
	}
	result := scorer.Score(input)
	t.Logf("Case G: score=%.1f level=%s combo=%v", result.FinalScore, result.RiskLevel, result.ComboMultiplierApplied)
	assert.Equal(t, 0.0, result.FinalScore)
	assert.Equal(t, "critical", result.RiskLevel)
	assert.Equal(t, "red", result.DecisionColor)
	assert.True(t, result.ComboMultiplierApplied)
}

// ============================================================================
// Test: DB config round-trip
// ============================================================================

func TestConfigDBRoundTrip(t *testing.T) {
	db := setupRiskTestDB(t)
	scorer := NewRiskScorer(db)

	cfg := defaultRiskScorerConfig()
	cfg.ComboMultiplier = 2.0

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	db.Model(&models.SystemConfig{}).Where("key = ?", riskConfigKey).
		Update("value", string(data))

	// Force cache expiry
	scorer.cache.mu.Lock()
	scorer.cache.loadedAt = scorer.cache.loadedAt.Add(-2 * riskConfigCacheTTL)
	scorer.cache.mu.Unlock()

	newCfg := scorer.getConfig()
	assert.Equal(t, 2.0, newCfg.ComboMultiplier)
}
