package services

import (
	"math/rand"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// SamplingDecision represents the decision on whether a usage log needs Layer 2 (rule)
// and/or Layer 3 (semantic) LLM evaluation.
type SamplingDecision struct {
	ShouldEvalRule     bool   `json:"should_eval_rule"`
	ShouldEvalSemantic bool   `json:"should_eval_semantic"`
	Reason             string `json:"reason"`
}

// AssessmentSampler decides whether a usage log needs L2/L3 evaluation
// based on content hash novelty, schema validity, user action, and random sampling.
type AssessmentSampler struct {
	db *gorm.DB
}

// NewAssessmentSampler creates a new AssessmentSampler.
func NewAssessmentSampler(db *gorm.DB) *AssessmentSampler {
	return &AssessmentSampler{db: db}
}

// Decide determines which evaluation layers should be applied for a given usage log.
//
// Priority order:
//  1. New version (content_hash < 20 rule evals) → L2=true, L3=true
//  2. Schema failed → L2=true, L3=sampling
//  3. User aborted → L2=true, L3=true
//  4. User modified → L2=sampling, L3=true
//  5. Default random sampling → L2=20%, L3=5%
func (s *AssessmentSampler) Decide(usageLog *models.SkillUsageLog, schemaValid bool) SamplingDecision {
	// 1. Check content_hash first appearance (< 20 rule evals)
	if s.isNewVersion(usageLog.SkillContentHash) {
		return SamplingDecision{
			ShouldEvalRule:     true,
			ShouldEvalSemantic: true,
			Reason:             "new_version",
		}
	}

	// 2. Schema failed
	if !schemaValid {
		return SamplingDecision{
			ShouldEvalRule:     true,
			ShouldEvalSemantic: rand.Float64() < 0.05,
			Reason:             "schema_failed",
		}
	}

	// 3. User aborted
	if usageLog.UserAction != nil && *usageLog.UserAction == "aborted" {
		return SamplingDecision{
			ShouldEvalRule:     true,
			ShouldEvalSemantic: true,
			Reason:             "user_aborted",
		}
	}

	// 4. User modified
	if usageLog.UserAction != nil && *usageLog.UserAction == "modified" {
		return SamplingDecision{
			ShouldEvalRule:     rand.Float64() < 0.20,
			ShouldEvalSemantic: true,
			Reason:             "user_modified",
		}
	}

	// 5. Default random sampling
	return SamplingDecision{
		ShouldEvalRule:     rand.Float64() < 0.20,
		ShouldEvalSemantic: rand.Float64() < 0.05,
		Reason:             "random_sample",
	}
}

// isNewVersion checks if the content hash has fewer than 20 rule-layer assessment results.
func (s *AssessmentSampler) isNewVersion(contentHash string) bool {
	var count int64
	s.db.Model(&models.SkillAssessmentResult{}).
		Where("skill_content_hash = ? AND assessment_layer = ?", contentHash, string(models.AssessmentLayerRule)).
		Count(&count)
	return count < 20
}
