package services

import (
	"fmt"
	"math"
	"strconv"

	"gorm.io/gorm"
)

// RiskScoringInput contains all data needed for deterministic scoring.
type RiskScoringInput struct {
	TotalChanges                int
	MaxDirectDependencies       int
	MaxDependencyResource       string
	AffectedModuleResourceCount int
	WorkspaceResourceCount      int
	CrossWorkspaceCount         int
	RiskFactors                 []string
	RiskFactorResourceCounts    map[string]int
	UncertaintyLevel            string
	WorkspaceTier               string
}

// RiskScoreResult contains the scoring output with full audit trail.
type RiskScoreResult struct {
	FinalScore             float64     `json:"final_score"`
	RiskLevel              string      `json:"risk_level"`
	DecisionColor          string      `json:"decision_color"`
	Deductions             []Deduction `json:"deductions"`
	BaseDeduction          float64     `json:"base_deduction"`
	EnvMultiplier          float64     `json:"env_multiplier"`
	BaseScore              int         `json:"base_score"`
	ComboMultiplierApplied bool        `json:"combo_multiplier_applied,omitempty"`
	ComboDetail            string      `json:"combo_detail,omitempty"`
	NearThreshold          bool        `json:"near_threshold,omitempty"`
	DivergenceAlert        bool        `json:"divergence_alert,omitempty"`
	AIRiskLevel            string      `json:"ai_risk_level,omitempty"`
}

// Deduction records a single scoring deduction for audit purposes.
type Deduction struct {
	Category string `json:"category"` // magnitude / blast_radius / risk_signal
	Item     string `json:"item"`
	Points   int    `json:"points"` // negative
	Reason   string `json:"reason"`
}

// RiskScorer provides deterministic risk scoring with configurable parameters.
type RiskScorer struct {
	db    *gorm.DB
	cache *riskConfigCache
}

// NewRiskScorer creates a scorer instance, seeding config if db is provided.
func NewRiskScorer(db *gorm.DB) *RiskScorer {
	s := &RiskScorer{
		db:    db,
		cache: &riskConfigCache{},
	}
	if db != nil {
		ensureDefaultConfig(db)
	}
	return s
}

// Score computes a deterministic risk score from the given input.
func (s *RiskScorer) Score(input RiskScoringInput) RiskScoreResult {
	cfg := s.getConfig()
	result := RiskScoreResult{
		BaseScore:  100,
		Deductions: []Deduction{},
	}

	// Step 1: Magnitude deductions
	magPts := magnitudeDeduction(input.TotalChanges, cfg)
	if magPts != 0 {
		result.Deductions = append(result.Deductions, Deduction{
			Category: "magnitude",
			Item:     "total_changes",
			Points:   magPts,
			Reason:   reasonForMagnitude(input.TotalChanges),
		})
	}

	// Step 2: Blast radius deductions
	depPts := dependencyDeduction(input.MaxDirectDependencies, cfg)
	if depPts != 0 {
		result.Deductions = append(result.Deductions, Deduction{
			Category: "blast_radius",
			Item:     "dependencies",
			Points:   depPts,
			Reason:   reasonForDependencies(input.MaxDirectDependencies, input.MaxDependencyResource),
		})
	}

	modPts := moduleImpactDeduction(input.AffectedModuleResourceCount, input.WorkspaceResourceCount, cfg)
	if modPts != 0 {
		pct := float64(input.AffectedModuleResourceCount) / float64(input.WorkspaceResourceCount) * 100
		result.Deductions = append(result.Deductions, Deduction{
			Category: "blast_radius",
			Item:     "module_impact",
			Points:   modPts,
			Reason:   reasonForModuleImpact(pct),
		})
	}

	xwsPts := crossWorkspaceDeduction(input.CrossWorkspaceCount, cfg)
	if xwsPts != 0 {
		result.Deductions = append(result.Deductions, Deduction{
			Category: "blast_radius",
			Item:     "cross_workspace",
			Points:   xwsPts,
			Reason:   reasonForCrossWorkspace(input.CrossWorkspaceCount),
		})
	}

	// Step 3: Risk signal deductions
	baseRiskSignalPts := 0
	for _, factor := range input.RiskFactors {
		if pts, ok := cfg.Deductions.RiskFactors[factor]; ok {
			baseRiskSignalPts += pts
			result.Deductions = append(result.Deductions, Deduction{
				Category: "risk_signal",
				Item:     factor,
				Points:   pts,
				Reason:   "risk factor: " + factor,
			})
		}
	}

	// High-frequency bonus
	highFreqPts := 0
	highFreqSet := toSet(cfg.Deductions.HighFreqFactors)
	for _, factor := range input.RiskFactors {
		if !highFreqSet[factor] {
			continue
		}
		count := 0
		if input.RiskFactorResourceCounts != nil {
			count = input.RiskFactorResourceCounts[factor]
		}
		bonus := highFreqBonus(count, cfg)
		if bonus != 0 {
			highFreqPts += bonus
			result.Deductions = append(result.Deductions, Deduction{
				Category: "risk_signal",
				Item:     factor + "_high_freq",
				Points:   bonus,
				Reason:   reasonForHighFreq(factor, count),
			})
		}
	}

	// Combo multiplier check
	comboHighRiskSet := toSet(cfg.ComboHighRiskFactors)
	hitCount := 0
	for _, factor := range input.RiskFactors {
		if comboHighRiskSet[factor] {
			hitCount++
		}
	}

	riskSignalBeforeCombo := baseRiskSignalPts + highFreqPts
	riskSignalTotal := riskSignalBeforeCombo

	if hitCount >= cfg.ComboMinHits {
		extraPortion := float64(riskSignalBeforeCombo) * (cfg.ComboMultiplier - 1.0)
		extraPts := int(math.Round(extraPortion))
		if extraPts != 0 {
			result.ComboMultiplierApplied = true
			result.ComboDetail = reasonForCombo(hitCount, cfg.ComboMultiplier)
			result.Deductions = append(result.Deductions, Deduction{
				Category: "risk_signal",
				Item:     "combo_multiplier",
				Points:   extraPts,
				Reason:   result.ComboDetail,
			})
			riskSignalTotal += extraPts
		}
	}

	// Uncertainty deduction
	uncertaintyPts := 0
	if pts, ok := cfg.Deductions.Uncertainty[input.UncertaintyLevel]; ok {
		uncertaintyPts = pts
		result.Deductions = append(result.Deductions, Deduction{
			Category: "risk_signal",
			Item:     "uncertainty",
			Points:   pts,
			Reason:   "uncertainty level: " + input.UncertaintyLevel,
		})
	}

	// Step 4: Calculate final
	blastRadiusPts := depPts + modPts + xwsPts
	baseDeductions := float64(magPts + blastRadiusPts + riskSignalTotal + uncertaintyPts)

	envMult := cfg.DefaultEnvMultiplier
	if m, ok := cfg.EnvMultipliers[input.WorkspaceTier]; ok {
		envMult = m
	}

	result.BaseDeduction = baseDeductions
	result.EnvMultiplier = envMult

	raw := 100.0 + baseDeductions*envMult // baseDeductions is negative
	final := math.Round(math.Max(0, raw)*10) / 10
	result.FinalScore = final

	// Decision mapping
	result.RiskLevel, result.DecisionColor = decisionMapping(final, cfg)

	// Near threshold check
	thresholds := []int{cfg.DecisionThresholds.Low, cfg.DecisionThresholds.Medium, cfg.DecisionThresholds.High}
	for _, t := range thresholds {
		if math.Abs(final-float64(t)) <= float64(cfg.NearThresholdRange) {
			result.NearThreshold = true
			break
		}
	}

	return result
}

// --- Deduction calculation helpers ---

func magnitudeDeduction(total int, cfg *RiskScorerConfig) int {
	switch {
	case total > 30:
		return cfg.Deductions.Magnitude.Over30
	case total >= 11:
		return cfg.Deductions.Magnitude.From11To30
	case total >= 4:
		return cfg.Deductions.Magnitude.From4To10
	default:
		return 0
	}
}

func dependencyDeduction(deps int, cfg *RiskScorerConfig) int {
	switch {
	case deps > 10:
		return cfg.Deductions.Dependencies.Over10
	case deps >= 4:
		return cfg.Deductions.Dependencies.From4To10
	case deps >= 1:
		return cfg.Deductions.Dependencies.From1To3
	default:
		return 0
	}
}

func moduleImpactDeduction(affected, total int, cfg *RiskScorerConfig) int {
	if total == 0 {
		return 0
	}
	pct := float64(affected) / float64(total) * 100
	switch {
	case pct > 40:
		return cfg.Deductions.ModuleImpact.Over40
	case pct >= 15:
		return cfg.Deductions.ModuleImpact.From15To40
	case pct >= 5:
		return cfg.Deductions.ModuleImpact.From5To15
	default:
		return 0
	}
}

func crossWorkspaceDeduction(count int, cfg *RiskScorerConfig) int {
	switch {
	case count > 2:
		return cfg.Deductions.CrossWorkspace.Over2
	case count >= 1:
		return cfg.Deductions.CrossWorkspace.From1To2
	default:
		return 0
	}
}

func highFreqBonus(count int, cfg *RiskScorerConfig) int {
	for _, step := range cfg.Deductions.HighFreqSteps {
		if count >= step.Min && count <= step.Max {
			return step.Points
		}
	}
	return 0
}

func decisionMapping(score float64, cfg *RiskScorerConfig) (string, string) {
	switch {
	case score >= float64(cfg.DecisionThresholds.Low):
		return "low", "green"
	case score >= float64(cfg.DecisionThresholds.Medium):
		return "medium", "yellow"
	case score >= float64(cfg.DecisionThresholds.High):
		return "high", "orange"
	default:
		return "critical", "red"
	}
}

// --- Reason string helpers ---

func reasonForMagnitude(total int) string {
	switch {
	case total > 30:
		return "very large change: >30 resources"
	case total >= 11:
		return "large change: 11-30 resources"
	default:
		return "moderate change: 4-10 resources"
	}
}

func reasonForDependencies(deps int, resource string) string {
	suffix := ""
	if resource != "" {
		suffix = " (top: " + resource + ")"
	}
	switch {
	case deps > 10:
		return "very high dependency count: >10" + suffix
	case deps >= 4:
		return "high dependency count: 4-10" + suffix
	default:
		return "moderate dependency count: 1-3" + suffix
	}
}

func reasonForModuleImpact(pct float64) string {
	switch {
	case pct > 40:
		return "critical module impact: >40%"
	case pct >= 15:
		return "significant module impact: 15-40%"
	default:
		return "moderate module impact: 5-15%"
	}
}

func reasonForCrossWorkspace(count int) string {
	if count > 2 {
		return "high cross-workspace impact: >2 workspaces"
	}
	return "cross-workspace impact: 1-2 workspaces"
}

func reasonForHighFreq(factor string, count int) string {
	return factor + " affects " + strconv.Itoa(count) + " resources (high frequency)"
}

func reasonForCombo(hits int, mult float64) string {
	return strconv.Itoa(hits) + " high-risk factors triggered, " +
		fmt.Sprintf("%.1f", mult) + "x combo multiplier applied"
}

// --- Utility helpers ---

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

var severityOrder = map[string]int{
	"low":      0,
	"medium":   1,
	"high":     2,
	"critical": 3,
}

// severityGap returns the absolute difference between two severity levels.
func severityGap(a, b string) int {
	va := severityOrder[a]
	vb := severityOrder[b]
	diff := va - vb
	if diff < 0 {
		return -diff
	}
	return diff
}

// maxSeverity returns the higher of two severity levels.
func maxSeverity(a, b string) string {
	if severityOrder[a] >= severityOrder[b] {
		return a
	}
	return b
}

