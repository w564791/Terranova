package services

import (
	"math/rand"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// SummarySamplingDecision 摘要评估抽样决策
type SummarySamplingDecision struct {
	ShouldEvalRule     bool
	ShouldEvalSemantic bool
	Reason             string
}

// SummaryAssessmentSampler 摘要评估抽样策略
type SummaryAssessmentSampler struct {
	db *gorm.DB
}

func NewSummaryAssessmentSampler(db *gorm.DB) *SummaryAssessmentSampler {
	return &SummaryAssessmentSampler{db: db}
}

// Decide 根据信号决定是否需要 L2/L3 评估
func (s *SummaryAssessmentSampler) Decide(resourceType string, l1Passed bool, configChanged bool) SummarySamplingDecision {
	// Priority 1: L1 fail → full eval
	if !l1Passed {
		return SummarySamplingDecision{
			ShouldEvalRule:     true,
			ShouldEvalSemantic: true,
			Reason:             "l1_fail",
		}
	}

	// Priority 2: new resource type → full eval
	if s.isNewResourceType(resourceType) {
		return SummarySamplingDecision{
			ShouldEvalRule:     true,
			ShouldEvalSemantic: true,
			Reason:             "new_resource_type",
		}
	}

	// Priority 3: config changed → higher rate
	if configChanged {
		return SummarySamplingDecision{
			ShouldEvalRule:     rand.Float64() < 0.50,
			ShouldEvalSemantic: rand.Float64() < 0.30,
			Reason:             "config_changed",
		}
	}

	// Default: low sampling
	return SummarySamplingDecision{
		ShouldEvalRule:     rand.Float64() < 0.10,
		ShouldEvalSemantic: rand.Float64() < 0.05,
		Reason:             "default_sampling",
	}
}

// isNewResourceType checks if this resource_type has ever been assessed before
func (s *SummaryAssessmentSampler) isNewResourceType(resourceType string) bool {
	var count int64
	s.db.Model(&models.SkillAssessmentResult{}).
		Where("source_type = ? AND skill_name = ?", "summary", resourceType).
		Count(&count)
	return count == 0
}
