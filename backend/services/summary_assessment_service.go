package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"iac-platform/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SummaryAssessmentService orchestrates L1/L2/L3 assessment for resource summaries
type SummaryAssessmentService struct {
	db        *gorm.DB
	validator *SummaryTextValidator
	sampler   *SummaryAssessmentSampler
	evaluator *SummaryLLMEvaluator
}

func NewSummaryAssessmentService(db *gorm.DB) *SummaryAssessmentService {
	return &SummaryAssessmentService{
		db:        db,
		validator: NewSummaryTextValidator(),
		sampler:   NewSummaryAssessmentSampler(db),
		evaluator: NewSummaryLLMEvaluator(db),
	}
}

// AssessSource runs assessment for all pending resources of a given source
func (s *SummaryAssessmentService) AssessSource(ctx context.Context, sourceID string) error {
	var resources []models.ResourceIndex
	s.db.Where("external_source_id = ? AND summary_assessment_status = ?", sourceID, "pending").
		Find(&resources)

	if len(resources) == 0 {
		log.Printf("[SummaryAssessment] source %s: no pending resources", sourceID)
		return nil
	}

	log.Printf("[SummaryAssessment] source %s: assessing %d resources", sourceID, len(resources))

	configChanged := s.detectConfigChange()
	assessed := 0

	for _, resource := range resources {
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled, assessed %d/%d", assessed, len(resources))
		}
		s.assessOne(ctx, &resource, configChanged)
		assessed++
	}

	log.Printf("[SummaryAssessment] source %s: completed %d assessments", sourceID, assessed)
	return nil
}

// assessOne runs L1 + optional L2/L3 for a single resource
func (s *SummaryAssessmentService) assessOne(ctx context.Context, resource *models.ResourceIndex, configChanged bool) {
	startTime := time.Now()

	// L1: Text rule validation (always)
	l1Result := s.validator.Validate(resource.ResourceSummary, resource.ResourceType, resource.Attributes)
	l1Latency := int(time.Since(startTime).Milliseconds())

	// Marshal security tag misses
	var securityMissesJSON *json.RawMessage
	if len(l1Result.SecurityTagMisses) > 0 {
		data, _ := json.Marshal(l1Result.SecurityTagMisses)
		raw := json.RawMessage(data)
		securityMissesJSON = &raw
	}

	resourceID := resource.ID
	l1Assessment := models.SkillAssessmentResult{
		ID:                    uuid.New().String(),
		SkillName:             resource.ResourceType,
		SkillContentHash:      resource.SummaryHash,
		AssessedAt:            time.Now(),
		AssessmentLayer:       models.AssessmentLayerSchema,
		Verdict:               models.AssessmentVerdict(l1Result.Verdict),
		Score:                 int16(l1Result.Score),
		AssessmentLatencyMs:   &l1Latency,
		SourceType:            "summary",
		ResourceID:            &resourceID,
		FormatViolations:      models.TextArray(l1Result.FormatViolations),
		SecurityTagMisses:     securityMissesJSON,
		HallucinationSuspects: models.TextArray(l1Result.HallucinationSuspects),
	}

	if err := s.db.Create(&l1Assessment).Error; err != nil {
		log.Printf("[SummaryAssessment] L1 insert failed for resource %d: %v", resource.ID, err)
		return
	}

	log.Printf("[SummaryAssessment] L1 resource %d (%s): verdict=%s, score=%d",
		resource.ID, resource.ResourceType, l1Result.Verdict, l1Result.Score)

	// Sampling decision for L2/L3
	l1Passed := l1Result.Verdict == "pass"
	decision := s.sampler.Decide(resource.ResourceType, l1Passed, configChanged)

	// L2: Prompt compliance (LLM)
	if decision.ShouldEvalRule {
		evalCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		ruleResult, err := s.evaluator.EvaluateRule(evalCtx, resource.ResourceSummary, resource.ResourceType, resource.Attributes)
		cancel()

		if err != nil {
			log.Printf("[SummaryAssessment] L2 error for resource %d: %v", resource.ID, err)
		} else {
			latency := ruleResult.LatencyMs
			ruleViolations := ruleResult.RuleViolations
			l2Assessment := models.SkillAssessmentResult{
				ID:                   uuid.New().String(),
				SkillName:            resource.ResourceType,
				SkillContentHash:     resource.SummaryHash,
				AssessedAt:           time.Now(),
				AssessmentLayer:      models.AssessmentLayerRule,
				Verdict:              models.AssessmentVerdict(ruleResult.Verdict),
				Score:                int16(ruleResult.Score),
				AssessmentLatencyMs:  &latency,
				RuleViolations:       &ruleViolations,
				AssessmentConfidence: assessmentStrPtr(ruleResult.Confidence),
				AssessmentModel:      assessmentStrPtr(ruleResult.Model),
				AssessmentRawOutput:  assessmentStrPtr(ruleResult.RawOutput),
				SourceType:           "summary",
				ResourceID:           &resourceID,
			}
			s.db.Create(&l2Assessment)
			log.Printf("[SummaryAssessment] L2 resource %d: verdict=%s, score=%d", resource.ID, ruleResult.Verdict, ruleResult.Score)
		}
	}

	// L3: Content quality (LLM)
	if decision.ShouldEvalSemantic {
		evalCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		semanticResult, err := s.evaluator.EvaluateSemantic(evalCtx, resource.ResourceSummary, resource.ResourceType, resource.Attributes)
		cancel()

		if err != nil {
			log.Printf("[SummaryAssessment] L3 error for resource %d: %v", resource.ID, err)
		} else {
			latency := semanticResult.LatencyMs
			qualityIssues := semanticResult.QualityIssues
			l3Assessment := models.SkillAssessmentResult{
				ID:                   uuid.New().String(),
				SkillName:            resource.ResourceType,
				SkillContentHash:     resource.SummaryHash,
				AssessedAt:           time.Now(),
				AssessmentLayer:      models.AssessmentLayerSemantic,
				Verdict:              models.AssessmentVerdict(semanticResult.Verdict),
				Score:                int16(semanticResult.Score),
				AssessmentLatencyMs:  &latency,
				QualityIssues:        &qualityIssues,
				AssessmentConfidence: assessmentStrPtr(semanticResult.Confidence),
				AssessmentModel:      assessmentStrPtr(semanticResult.Model),
				AssessmentRawOutput:  assessmentStrPtr(semanticResult.RawOutput),
				SourceType:           "summary",
				ResourceID:           &resourceID,
			}
			s.db.Create(&l3Assessment)
			log.Printf("[SummaryAssessment] L3 resource %d: verdict=%s, score=%d", resource.ID, semanticResult.Verdict, semanticResult.Score)
		}
	}

	// Mark resource as assessed
	s.db.Model(&models.ResourceIndex{}).Where("id = ?", resource.ID).
		Update("summary_assessment_status", "assessed")
}

// detectConfigChange checks if the AI config for cmdb_resource_summary has changed
func (s *SummaryAssessmentService) detectConfigChange() bool {
	configService := NewAIConfigService(s.db)
	cfg, err := configService.GetConfigForCapability("cmdb_resource_summary")
	if err != nil || cfg == nil {
		return false
	}

	prompt := defaultResourceSummaryPrompt
	if customPrompt, ok := cfg.CapabilityPrompts["cmdb_resource_summary"]; ok && customPrompt != "" {
		prompt = customPrompt
	}
	currentHash := summaryConfigHash(cfg.ModelID, prompt)

	var lastHash string
	s.db.Model(&models.SkillAssessmentResult{}).
		Where("source_type = ?", "summary").
		Order("assessed_at DESC").
		Limit(1).
		Pluck("skill_content_hash", &lastHash)

	return lastHash != "" && lastHash != currentHash
}

// summaryConfigHash creates a hash from model ID and prompt for change detection
func summaryConfigHash(modelID, prompt string) string {
	h := md5.Sum([]byte(modelID + "|" + prompt))
	return hex.EncodeToString(h[:])
}
