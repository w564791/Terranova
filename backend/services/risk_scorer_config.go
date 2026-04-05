package services

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// RiskScorerConfig holds all configurable scoring parameters.
// Stored as JSON in system_configs table for runtime tunability.
type RiskScorerConfig struct {
	Deductions struct {
		Magnitude struct {
			From4To10  int `json:"from_4_to_10"`
			From11To30 int `json:"from_11_to_30"`
			Over30     int `json:"over_30"`
		} `json:"magnitude"`
		Dependencies struct {
			From1To3  int `json:"from_1_to_3"`
			From4To10 int `json:"from_4_to_10"`
			Over10    int `json:"over_10"`
		} `json:"dependencies"`
		ModuleImpact struct {
			From5To15  int `json:"from_5_to_15"`
			From15To40 int `json:"from_15_to_40"`
			Over40     int `json:"over_40"`
		} `json:"module_impact"`
		CrossWorkspace struct {
			From1To2 int `json:"from_1_to_2"`
			Over2    int `json:"over_2"`
		} `json:"cross_workspace"`
		RiskFactors     map[string]int `json:"risk_factors"`
		Uncertainty     map[string]int `json:"uncertainty"`
		HighFreqFactors []string       `json:"high_freq_factors"`
		HighFreqSteps   []struct {
			Min    int `json:"min"`
			Max    int `json:"max"`
			Points int `json:"points"`
		} `json:"high_freq_steps"`
	} `json:"deductions"`
	ComboMultiplier      float64            `json:"combo_multiplier"`
	ComboHighRiskFactors []string           `json:"combo_high_risk_factors"`
	ComboMinHits         int                `json:"combo_min_hits"`
	EnvMultipliers       map[string]float64 `json:"env_multipliers"`
	DefaultEnvMultiplier float64            `json:"default_env_multiplier"`
	DecisionThresholds   struct {
		Low    int `json:"low"`
		Medium int `json:"medium"`
		High   int `json:"high"`
	} `json:"decision_thresholds"`
	NearThresholdRange int `json:"near_threshold_range"`
}

// riskConfigCache provides TTL-based caching with fallback to last valid config.
type riskConfigCache struct {
	current   *RiskScorerConfig
	lastValid *RiskScorerConfig
	loadedAt  time.Time
	mu        sync.RWMutex
}

const riskConfigCacheTTL = 60 * time.Second
const riskConfigKey = "risk_scorer_config"

// defaultRiskScorerConfig returns the hardcoded default configuration.
func defaultRiskScorerConfig() *RiskScorerConfig {
	cfg := &RiskScorerConfig{}

	cfg.Deductions.Magnitude.From4To10 = -5
	cfg.Deductions.Magnitude.From11To30 = -15
	cfg.Deductions.Magnitude.Over30 = -25

	cfg.Deductions.Dependencies.From1To3 = -5
	cfg.Deductions.Dependencies.From4To10 = -15
	cfg.Deductions.Dependencies.Over10 = -25

	cfg.Deductions.ModuleImpact.From5To15 = -5
	cfg.Deductions.ModuleImpact.From15To40 = -10
	cfg.Deductions.ModuleImpact.Over40 = -20

	cfg.Deductions.CrossWorkspace.From1To2 = -8
	cfg.Deductions.CrossWorkspace.Over2 = -15

	cfg.Deductions.RiskFactors = map[string]int{
		"service_disruption":        -15,
		"external_exposure_change":  -12,
		"dependency_break":          -10,
		"resource_deletion":         -8,
		"permission_scope_change":   -8,
		"sensitive_resource_change": -8,
		"high_blast_radius":         -5,
		"configuration_drift":       -3,
	}

	cfg.Deductions.Uncertainty = map[string]int{
		"high":   -8,
		"medium": -3,
	}

	cfg.Deductions.HighFreqFactors = []string{"service_disruption", "resource_deletion"}

	cfg.Deductions.HighFreqSteps = []struct {
		Min    int `json:"min"`
		Max    int `json:"max"`
		Points int `json:"points"`
	}{
		{Min: 3, Max: 5, Points: -5},
		{Min: 6, Max: 10, Points: -10},
		{Min: 11, Max: 999, Points: -15},
	}

	cfg.ComboMultiplier = 1.5
	cfg.ComboHighRiskFactors = []string{
		"service_disruption",
		"external_exposure_change",
		"dependency_break",
		"resource_deletion",
		"permission_scope_change",
	}
	cfg.ComboMinHits = 3

	cfg.EnvMultipliers = map[string]float64{
		"critical":    1.5,
		"production":  1.3,
		"staging":     1.0,
		"development": 0.7,
	}
	cfg.DefaultEnvMultiplier = 1.3

	cfg.DecisionThresholds.Low = 80
	cfg.DecisionThresholds.Medium = 60
	cfg.DecisionThresholds.High = 40

	cfg.NearThresholdRange = 3

	return cfg
}

// ensureDefaultConfig seeds the default config into system_configs if absent.
func ensureDefaultConfig(db *gorm.DB) {
	if db == nil {
		return
	}

	var existing models.SystemConfig
	err := db.Where("key = ? AND deleted_at IS NULL", riskConfigKey).First(&existing).Error
	if err == nil {
		return // already exists
	}

	cfg := defaultRiskScorerConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		log.Printf("[risk-scorer] failed to marshal default config: %v", err)
		return
	}

	record := models.SystemConfig{
		Key:         riskConfigKey,
		Value:       string(data),
		Description: "Risk scorer configuration - deduction points, multipliers, thresholds",
	}
	if err := db.Create(&record).Error; err != nil {
		log.Printf("[risk-scorer] failed to seed default config: %v", err)
	}
}

// getConfig returns the current config. The returned pointer is shared — callers must not mutate it.
func (s *RiskScorer) getConfig() *RiskScorerConfig {
	if s.db == nil {
		return defaultRiskScorerConfig()
	}

	s.cache.mu.RLock()
	if s.cache.current != nil && time.Since(s.cache.loadedAt) < riskConfigCacheTTL {
		cfg := s.cache.current
		s.cache.mu.RUnlock()
		return cfg
	}
	s.cache.mu.RUnlock()

	// Cache expired or empty - reload from DB
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()

	// Double-check after acquiring write lock
	if s.cache.current != nil && time.Since(s.cache.loadedAt) < riskConfigCacheTTL {
		return s.cache.current
	}

	var record models.SystemConfig
	err := s.db.Where("key = ? AND deleted_at IS NULL", riskConfigKey).First(&record).Error
	if err != nil {
		if s.cache.lastValid != nil {
			log.Printf("[risk-scorer] DB read failed, using last valid config: %v", err)
			return s.cache.lastValid
		}
		log.Printf("[risk-scorer] DB read failed, using hardcoded defaults: %v", err)
		return defaultRiskScorerConfig()
	}

	var cfg RiskScorerConfig
	if err := json.Unmarshal([]byte(record.Value), &cfg); err != nil {
		if s.cache.lastValid != nil {
			log.Printf("[risk-scorer] config unmarshal failed, using last valid: %v", err)
			return s.cache.lastValid
		}
		log.Printf("[risk-scorer] config unmarshal failed, using defaults: %v", err)
		return defaultRiskScorerConfig()
	}

	validated := validateConfig(&cfg)
	s.cache.current = validated
	s.cache.lastValid = validated
	s.cache.loadedAt = time.Now()

	return validated
}

// validateConfig checks config bounds; returns hardcoded defaults on failure.
func validateConfig(cfg *RiskScorerConfig) *RiskScorerConfig {
	// env_multipliers: must be >0 and <=5.0
	for env, mult := range cfg.EnvMultipliers {
		if mult <= 0 || mult > 5.0 {
			log.Printf("[risk-scorer] invalid env multiplier for %s: %.2f, using defaults", env, mult)
			return defaultRiskScorerConfig()
		}
	}

	// default_env_multiplier: must be >0 and <=5.0
	if cfg.DefaultEnvMultiplier <= 0 || cfg.DefaultEnvMultiplier > 5.0 {
		log.Printf("[risk-scorer] invalid default env multiplier: %.2f, using defaults", cfg.DefaultEnvMultiplier)
		return defaultRiskScorerConfig()
	}

	// structured deductions: must be <=0 and >=-50
	magnitudeFields := []int{
		cfg.Deductions.Magnitude.From4To10,
		cfg.Deductions.Magnitude.From11To30,
		cfg.Deductions.Magnitude.Over30,
	}
	for _, pts := range magnitudeFields {
		if pts > 0 || pts < -50 {
			log.Printf("[risk-scorer] invalid magnitude deduction: %d, using defaults", pts)
			return defaultRiskScorerConfig()
		}
	}

	depFields := []int{
		cfg.Deductions.Dependencies.From1To3,
		cfg.Deductions.Dependencies.From4To10,
		cfg.Deductions.Dependencies.Over10,
	}
	for _, pts := range depFields {
		if pts > 0 || pts < -50 {
			log.Printf("[risk-scorer] invalid dependency deduction: %d, using defaults", pts)
			return defaultRiskScorerConfig()
		}
	}

	moduleFields := []int{
		cfg.Deductions.ModuleImpact.From5To15,
		cfg.Deductions.ModuleImpact.From15To40,
		cfg.Deductions.ModuleImpact.Over40,
	}
	for _, pts := range moduleFields {
		if pts > 0 || pts < -50 {
			log.Printf("[risk-scorer] invalid module impact deduction: %d, using defaults", pts)
			return defaultRiskScorerConfig()
		}
	}

	xwsFields := []int{
		cfg.Deductions.CrossWorkspace.From1To2,
		cfg.Deductions.CrossWorkspace.Over2,
	}
	for _, pts := range xwsFields {
		if pts > 0 || pts < -50 {
			log.Printf("[risk-scorer] invalid cross-workspace deduction: %d, using defaults", pts)
			return defaultRiskScorerConfig()
		}
	}

	// uncertainty deductions: must be <=0 and >=-50
	for level, pts := range cfg.Deductions.Uncertainty {
		if pts > 0 || pts < -50 {
			log.Printf("[risk-scorer] invalid uncertainty deduction for %s: %d, using defaults", level, pts)
			return defaultRiskScorerConfig()
		}
	}

	// risk_factor deductions: must be <=0 and >=-50
	for factor, pts := range cfg.Deductions.RiskFactors {
		if pts > 0 || pts < -50 {
			log.Printf("[risk-scorer] invalid risk factor deduction for %s: %d, using defaults", factor, pts)
			return defaultRiskScorerConfig()
		}
	}

	// combo_multiplier: must be >=1.0 and <=5.0
	if cfg.ComboMultiplier < 1.0 || cfg.ComboMultiplier > 5.0 {
		log.Printf("[risk-scorer] invalid combo multiplier: %.2f, using defaults", cfg.ComboMultiplier)
		return defaultRiskScorerConfig()
	}

	// combo_min_hits: must be >=1 and <=10
	if cfg.ComboMinHits < 1 || cfg.ComboMinHits > 10 {
		log.Printf("[risk-scorer] invalid combo min hits: %d, using defaults", cfg.ComboMinHits)
		return defaultRiskScorerConfig()
	}

	// decision_thresholds: low > medium > high, high >= 0, low <= 100
	t := cfg.DecisionThresholds
	if t.Low <= t.Medium || t.Medium <= t.High || t.High < 0 || t.Low > 100 {
		log.Printf("[risk-scorer] invalid decision thresholds: low=%d medium=%d high=%d, using defaults",
			t.Low, t.Medium, t.High)
		return defaultRiskScorerConfig()
	}

	return cfg
}
