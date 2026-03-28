package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"iac-platform/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// assessmentTask represents a task to assess a skill usage log
type assessmentTask struct {
	UsageLogID    string
	TaskSkillName string
}

// AssessmentWorker processes pending skill usage logs through Layer 1 Schema validation
// and optionally Layer 2 (rule) and Layer 3 (semantic) LLM evaluations.
type AssessmentWorker struct {
	db        *gorm.DB
	validator *SkillSchemaValidator
	sampler   *AssessmentSampler
	evaluator *SkillLLMEvaluator
	taskCh    chan assessmentTask
	poolSize  int
	interval  time.Duration
	running   bool
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewAssessmentWorker creates a new assessment worker instance
// - Channel size: 100
// - Pool size: 3 concurrent workers
// - Scanner interval: 30 seconds
func NewAssessmentWorker(db *gorm.DB, validator *SkillSchemaValidator, sampler *AssessmentSampler, evaluator *SkillLLMEvaluator) *AssessmentWorker {
	return &AssessmentWorker{
		db:        db,
		validator: validator,
		sampler:   sampler,
		evaluator: evaluator,
		taskCh:    make(chan assessmentTask, 100),
		poolSize:  3,
		interval:  30 * time.Second,
		running:   false,
	}
}

// Submit submits a usage log for assessment (non-blocking)
// If the channel is full, logs a warning and relies on the scanner to pick it up
func (w *AssessmentWorker) Submit(usageLogID, taskSkillName string) {
	select {
	case w.taskCh <- assessmentTask{UsageLogID: usageLogID, TaskSkillName: taskSkillName}:
		// Successfully submitted
	default:
		// Channel is full, log warning
		log.Printf("[AssessmentWorker] Queue full, task for log %s will be picked up by scanner", usageLogID)
	}
}

// Start starts the worker goroutines and scanner
// Follows the EmbeddingWorker pattern
func (w *AssessmentWorker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		log.Println("[AssessmentWorker] Already running")
		return
	}
	w.running = true
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.mu.Unlock()

	log.Println("[AssessmentWorker] ========== Starting worker pool ==========")

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < w.poolSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			w.worker(workerID)
		}(i)
	}

	// Start scanner goroutine
	go w.scanner()

	// Wait for context cancellation
	<-w.ctx.Done()

	// Close task channel to signal workers to stop
	close(w.taskCh)

	// Wait for all workers to finish
	wg.Wait()

	log.Println("[AssessmentWorker] ========== Stopped ==========")
	w.mu.Lock()
	w.running = false
	w.mu.Unlock()
}

// Stop stops the worker
func (w *AssessmentWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
	}
}

// IsRunning checks if the worker is currently running
func (w *AssessmentWorker) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// worker processes tasks from the channel
func (w *AssessmentWorker) worker(workerID int) {
	for task := range w.taskCh {
		_, err := w.ProcessOne(task.UsageLogID, task.TaskSkillName)
		if err != nil {
			log.Printf("[AssessmentWorker-%d] Error processing log %s: %v", workerID, task.UsageLogID, err)
		}
	}
}

// scanner periodically scans for pending records and submits them to the channel
func (w *AssessmentWorker) scanner() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Run immediately on start
	w.scanPending()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.scanPending()
		}
	}
}

// scanPending scans for pending records and submits them
func (w *AssessmentWorker) scanPending() {
	var logs []models.SkillUsageLog
	err := w.db.Where("assessment_status = ?", models.AssessmentStatusPending).
		Order("created_at ASC").
		Limit(50).
		Find(&logs).Error

	if err != nil {
		log.Printf("[AssessmentWorker] Error scanning pending logs: %v", err)
		return
	}

	if len(logs) > 0 {
		log.Printf("[AssessmentWorker] Found %d pending logs to assess", len(logs))
		for _, logItem := range logs {
			// Extract task skill name from SkillIDs if available
			// For now, we'll leave it empty and rely on content hash fallback
			w.Submit(logItem.ID, "")
		}
	}
}

// ProcessOne processes a single usage log assessment
// Returns (success, error) where success indicates if the record was processed
func (w *AssessmentWorker) ProcessOne(usageLogID, taskSkillName string) (bool, error) {
	startTime := time.Now()

	// 1. Load SkillUsageLog by ID
	var usageLog models.SkillUsageLog
	if err := w.db.First(&usageLog, "id = ?", usageLogID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, fmt.Errorf("usage log not found: %s", usageLogID)
		}
		return false, fmt.Errorf("failed to load usage log: %w", err)
	}

	// 2. If already assessed, return early
	if usageLog.AssessmentStatus == string(models.AssessmentStatusAssessed) {
		return false, nil
	}

	// 3. Idempotency: CAS update pending → assessing to prevent duplicate processing
	result := w.db.Model(&models.SkillUsageLog{}).
		Where("id = ? AND assessment_status = ?", usageLogID, string(models.AssessmentStatusPending)).
		Update("assessment_status", "assessing")
	if result.RowsAffected == 0 {
		// Another worker already picked this up
		return false, nil
	}

	// 3. Get output from OutputSnapshot (default to null if nil)
	var output json.RawMessage
	if usageLog.OutputSnapshot != nil {
		output = *usageLog.OutputSnapshot
	} else {
		output = json.RawMessage(`null`)
	}

	// 4. Determine schema lookup key (priority: taskSkillName > capability > content_hash)
	skillName := taskSkillName
	if skillName == "" {
		skillName = usageLog.Capability // 用 capability 作为 schema key（plan_summary, apply_summary 等）
	}
	if skillName == "" {
		skillName = usageLog.SkillContentHash
	}

	// 5. Call validator.Validate(skillName, output)
	validationResult := w.validator.Validate(skillName, output)

	// 6. Create SkillAssessmentResult record
	schemaValid := validationResult.Valid
	assessmentLatencyMs := int(time.Since(startTime).Milliseconds())

	schemaResult := models.SkillAssessmentResult{
		ID:                  uuid.New().String(),
		UsageLogID:          usageLogID,
		SkillName:           skillName,
		SkillContentHash:    usageLog.SkillContentHash,
		AssessedAt:          time.Now(),
		AssessmentLayer:     models.AssessmentLayerSchema,
		Verdict:             models.AssessmentVerdict(validationResult.Verdict),
		Score:               int16(validationResult.Score),
		AssessmentLatencyMs: &assessmentLatencyMs,
		SchemaValid:         &schemaValid,
		MissingFields:       models.TextArray(validationResult.MissingFields),
		InvalidEnumFields:   models.TextArray(validationResult.InvalidEnums),
	}

	// 7. Insert assessment result
	if err := w.db.Create(&schemaResult).Error; err != nil {
		return false, fmt.Errorf("failed to insert assessment result: %w", err)
	}

	log.Printf("[AssessmentWorker] Layer 1 assessed %s: verdict=%s, score=%d, latency=%dms",
		usageLogID, schemaResult.Verdict, schemaResult.Score, assessmentLatencyMs)

	// --- Layer 2/3: LLM evaluation (optional, based on sampling) ---
	if w.sampler != nil && w.evaluator != nil {
		decision := w.sampler.Decide(&usageLog, validationResult.Valid)

		// Layer 2: Rule consistency
		if decision.ShouldEvalRule {
			evalCtx, evalCancel := context.WithTimeout(context.Background(), 60*time.Second)
			ruleResult, err := w.evaluator.EvaluateRule(evalCtx, &usageLog)
			evalCancel()
			if err != nil {
				log.Printf("[AssessmentWorker] Layer 2 eval error for %s: %v", usageLogID, err)
			} else {
				latency := ruleResult.LatencyMs
				ruleViolations := ruleResult.RuleViolations
				ruleAssessment := models.SkillAssessmentResult{
					ID:                   uuid.New().String(),
					UsageLogID:           usageLogID,
					SkillName:            skillName,
					SkillContentHash:     usageLog.SkillContentHash,
					AssessedAt:           time.Now(),
					AssessmentLayer:      models.AssessmentLayerRule,
					Verdict:              models.AssessmentVerdict(ruleResult.Verdict),
					Score:                int16(ruleResult.Score),
					AssessmentLatencyMs:  &latency,
					RuleViolations:       &ruleViolations,
					AssessmentConfidence: assessmentStrPtr(ruleResult.Confidence),
					AssessmentModel:      assessmentStrPtr(ruleResult.Model),
					AssessmentRawOutput:  assessmentStrPtr(ruleResult.RawOutput),
				}
				w.db.Create(&ruleAssessment)
				log.Printf("[AssessmentWorker] Layer 2 assessed %s: verdict=%s, score=%d", usageLogID, ruleResult.Verdict, ruleResult.Score)
			}
		}

		// Layer 3: Semantic quality
		if decision.ShouldEvalSemantic {
			evalCtx, evalCancel := context.WithTimeout(context.Background(), 60*time.Second)
			semanticResult, err := w.evaluator.EvaluateSemantic(evalCtx, &usageLog)
			evalCancel()
			if err != nil {
				log.Printf("[AssessmentWorker] Layer 3 eval error for %s: %v", usageLogID, err)
			} else {
				latency := semanticResult.LatencyMs
				qualityIssues := semanticResult.QualityIssues
				semanticAssessment := models.SkillAssessmentResult{
					ID:                   uuid.New().String(),
					UsageLogID:           usageLogID,
					SkillName:            skillName,
					SkillContentHash:     usageLog.SkillContentHash,
					AssessedAt:           time.Now(),
					AssessmentLayer:      models.AssessmentLayerSemantic,
					Verdict:              models.AssessmentVerdict(semanticResult.Verdict),
					Score:                int16(semanticResult.Score),
					AssessmentLatencyMs:  &latency,
					QualityIssues:        &qualityIssues,
					AssessmentConfidence: assessmentStrPtr(semanticResult.Confidence),
					AssessmentModel:      assessmentStrPtr(semanticResult.Model),
					AssessmentRawOutput:  assessmentStrPtr(semanticResult.RawOutput),
				}
				w.db.Create(&semanticAssessment)
				log.Printf("[AssessmentWorker] Layer 3 assessed %s: verdict=%s, score=%d", usageLogID, semanticResult.Verdict, semanticResult.Score)
			}
		}
	}

	// 8. Update usage log assessment_status to "assessed" (after all layers complete)
	if err := w.db.Model(&models.SkillUsageLog{}).
		Where("id = ?", usageLogID).
		Update("assessment_status", models.AssessmentStatusAssessed).Error; err != nil {
		return false, fmt.Errorf("failed to update assessment status: %w", err)
	}

	return true, nil
}

// assessmentStrPtr returns a pointer to the string, or nil if empty.
func assessmentStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
