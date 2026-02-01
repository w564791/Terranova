package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// RunTaskExecutor handles execution of run tasks
type RunTaskExecutor struct {
	db                    *gorm.DB
	httpClient            *http.Client
	platformConfigService *PlatformConfigService
	mu                    sync.Mutex
}

// NewRunTaskExecutor creates a new run task executor
func NewRunTaskExecutor(db *gorm.DB, baseURL string) *RunTaskExecutor {
	return &RunTaskExecutor{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		platformConfigService: NewPlatformConfigService(db),
	}
}

// getBaseURL returns the current platform base URL from config
func (e *RunTaskExecutor) getBaseURL() string {
	if e.platformConfigService != nil {
		return e.platformConfigService.GetBaseURL()
	}
	return "http://localhost:8080"
}

// generateResultID generates a unique result ID
func generateResultID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 16
	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))
	for i := range b {
		num, _ := rand.Int(rand.Reader, charsetLen)
		b[i] = charset[num.Int64()]
	}
	return fmt.Sprintf("rtr-%s", string(b))
}

// RunTaskStageResult contains the result of executing run tasks for a stage
type RunTaskStageResult struct {
	AllPassed           bool // All run tasks passed
	HasMandatoryFailure bool // At least one mandatory run task failed
	HasAdvisoryFailure  bool // At least one advisory run task failed (but no mandatory failures)
}

// ExecuteRunTasksForStage executes all run tasks for a specific stage
// This function will wait for all run tasks to complete (via callback) before returning
// Returns (passed, error) where passed is true only if all mandatory tasks passed
// Advisory failures will still return passed=true but the result can be checked via GetResultsForTask
func (e *RunTaskExecutor) ExecuteRunTasksForStage(
	ctx context.Context,
	task *models.WorkspaceTask,
	stage models.RunTaskStage,
) (bool, error) {
	// Get workspace run tasks for this stage
	var workspaceRunTasks []models.WorkspaceRunTask
	err := e.db.Preload("RunTask").
		Where("workspace_id = ? AND stage = ? AND enabled = true", task.WorkspaceID, stage).
		Find(&workspaceRunTasks).Error
	if err != nil {
		return false, fmt.Errorf("failed to get workspace run tasks: %w", err)
	}

	// 【新增】获取全局 Run Task（is_global = true 且 global_stages 包含当前阶段）
	var globalRunTasks []models.RunTask
	err = e.db.Where("is_global = ? AND enabled = ? AND global_stages LIKE ?", true, true, "%"+string(stage)+"%").
		Find(&globalRunTasks).Error
	if err != nil {
		log.Printf("[RunTask] Warning: failed to get global run tasks: %v", err)
		// 不返回错误，继续执行 workspace 级别的 run tasks
	}

	// 将全局 Run Task 转换为 WorkspaceRunTask 格式
	for _, grt := range globalRunTasks {
		// 检查是否已经在 workspace 级别配置了这个 run task（避免重复执行）
		alreadyConfigured := false
		for _, wrt := range workspaceRunTasks {
			if wrt.RunTaskID == grt.RunTaskID {
				alreadyConfigured = true
				break
			}
		}
		if alreadyConfigured {
			continue
		}

		// 创建一个虚拟的 WorkspaceRunTask 用于执行
		virtualWRT := models.WorkspaceRunTask{
			WorkspaceRunTaskID: fmt.Sprintf("global-%s-%s", grt.RunTaskID, stage),
			WorkspaceID:        task.WorkspaceID,
			RunTaskID:          grt.RunTaskID,
			Stage:              stage,
			EnforcementLevel:   grt.GlobalEnforcementLevel,
			Enabled:            true,
			RunTask:            &grt,
		}
		workspaceRunTasks = append(workspaceRunTasks, virtualWRT)
		log.Printf("[RunTask] Added global run task %s (%s) for stage %s", grt.Name, grt.RunTaskID, stage)
	}

	if len(workspaceRunTasks) == 0 {
		log.Printf("[RunTask] No run tasks configured for workspace %s stage %s", task.WorkspaceID, stage)
		return true, nil
	}

	log.Printf("[RunTask] Executing %d run tasks for workspace %s stage %s", len(workspaceRunTasks), task.WorkspaceID, stage)

	// Execute all run tasks in parallel and collect result IDs
	var wg sync.WaitGroup
	resultIDs := make(chan string, len(workspaceRunTasks))
	execErrors := make(chan error, len(workspaceRunTasks))

	for _, wrt := range workspaceRunTasks {
		if wrt.RunTask == nil || !wrt.RunTask.Enabled {
			continue
		}

		wg.Add(1)
		go func(wrt models.WorkspaceRunTask) {
			defer wg.Done()
			result := e.executeRunTask(ctx, task, &wrt)
			if result.err != nil {
				execErrors <- result.err
			} else {
				resultIDs <- result.resultID
			}
		}(wrt)
	}

	// Wait for all webhooks to be sent
	wg.Wait()
	close(resultIDs)
	close(execErrors)

	// Collect result IDs
	var pendingResultIDs []string
	for id := range resultIDs {
		pendingResultIDs = append(pendingResultIDs, id)
	}

	// Check for execution errors
	for err := range execErrors {
		log.Printf("[RunTask] Execution error: %v", err)
	}

	if len(pendingResultIDs) == 0 {
		log.Printf("[RunTask] No run tasks were successfully triggered")
		return true, nil
	}

	log.Printf("[RunTask] Waiting for %d run task callbacks...", len(pendingResultIDs))

	// Wait for all callbacks to complete
	return e.waitForCallbacks(ctx, pendingResultIDs, task.ID)
}

// waitForCallbacks waits for all run task callbacks to complete
func (e *RunTaskExecutor) waitForCallbacks(ctx context.Context, resultIDs []string, taskID uint) (bool, error) {
	// Poll interval
	pollInterval := 2 * time.Second
	// Maximum wait time (use the max timeout from all results)
	maxWaitTime := 10 * time.Minute

	deadline := time.Now().Add(maxWaitTime)

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			log.Printf("[RunTask] Timeout waiting for callbacks")
			return false, fmt.Errorf("timeout waiting for run task callbacks")
		}

		// Check status of all results
		allCompleted := true
		hasMandatoryFailure := false

		for _, resultID := range resultIDs {
			var result models.RunTaskResult
			if err := e.db.Where("result_id = ?", resultID).First(&result).Error; err != nil {
				log.Printf("[RunTask] Error fetching result %s: %v", resultID, err)
				continue
			}

			// Check if completed
			switch result.Status {
			case models.RunTaskResultPassed:
				log.Printf("[RunTask] Result %s passed", resultID)
			case models.RunTaskResultFailed:
				log.Printf("[RunTask] Result %s failed", resultID)
				// Check enforcement level
				var enforcementLevel models.RunTaskEnforcementLevel
				if result.WorkspaceRunTaskID != nil {
					var wrt models.WorkspaceRunTask
					if err := e.db.Where("workspace_run_task_id = ?", *result.WorkspaceRunTaskID).First(&wrt).Error; err == nil {
						enforcementLevel = wrt.EnforcementLevel
					}
				} else if result.RunTaskID != nil {
					// Global run task
					var rt models.RunTask
					if err := e.db.Where("run_task_id = ?", *result.RunTaskID).First(&rt).Error; err == nil {
						enforcementLevel = rt.GlobalEnforcementLevel
					}
				}
				if enforcementLevel == models.RunTaskEnforcementMandatory {
					hasMandatoryFailure = true
					log.Printf("[RunTask] Mandatory run task %s failed for task %d", resultID, taskID)
				}
			case models.RunTaskResultTimeout, models.RunTaskResultError:
				log.Printf("[RunTask] Result %s has status %s", resultID, result.Status)
				// Check enforcement level for timeout/error
				var enforcementLevel models.RunTaskEnforcementLevel
				if result.WorkspaceRunTaskID != nil {
					var wrt models.WorkspaceRunTask
					if err := e.db.Where("workspace_run_task_id = ?", *result.WorkspaceRunTaskID).First(&wrt).Error; err == nil {
						enforcementLevel = wrt.EnforcementLevel
					}
				} else if result.RunTaskID != nil {
					var rt models.RunTask
					if err := e.db.Where("run_task_id = ?", *result.RunTaskID).First(&rt).Error; err == nil {
						enforcementLevel = rt.GlobalEnforcementLevel
					}
				}
				if enforcementLevel == models.RunTaskEnforcementMandatory {
					hasMandatoryFailure = true
					log.Printf("[RunTask] Mandatory run task %s timed out/errored for task %d", resultID, taskID)
				}
			default:
				// Still pending or running
				allCompleted = false
			}
		}

		if allCompleted {
			if hasMandatoryFailure {
				log.Printf("[RunTask] All callbacks completed, but mandatory task(s) failed")
				return false, nil
			}
			log.Printf("[RunTask] All callbacks completed successfully")
			return true, nil
		}

		// Wait before next poll
		time.Sleep(pollInterval)
	}
}

type runTaskExecResult struct {
	runTaskID   string
	resultID    string
	passed      bool
	enforcement models.RunTaskEnforcementLevel
	err         error
}

// isGlobalRunTask 检查是否为全局 Run Task（虚拟的 WorkspaceRunTask）
func isGlobalRunTask(wrt *models.WorkspaceRunTask) bool {
	return wrt.ID == 0 && strings.HasPrefix(wrt.WorkspaceRunTaskID, "global-")
}

// executeRunTask executes a single run task
func (e *RunTaskExecutor) executeRunTask(
	ctx context.Context,
	task *models.WorkspaceTask,
	wrt *models.WorkspaceRunTask,
) *runTaskExecResult {
	result := &runTaskExecResult{
		runTaskID:   wrt.RunTaskID,
		enforcement: wrt.EnforcementLevel,
		passed:      true,
	}

	// Generate result ID
	resultID := generateResultID()

	// Create run task result record
	now := time.Now()
	timeoutAt := now.Add(time.Duration(wrt.RunTask.TimeoutSeconds) * time.Second)
	maxRunTimeoutAt := now.Add(time.Duration(wrt.RunTask.MaxRunSeconds) * time.Second)

	taskResult := &models.RunTaskResult{
		ResultID:        resultID,
		TaskID:          task.ID,
		Stage:           wrt.Stage,
		Status:          models.RunTaskResultPending,
		CallbackURL:     fmt.Sprintf("%s/api/v1/run-task-results/%s/callback", e.getBaseURL(), resultID),
		TimeoutSeconds:  wrt.RunTask.TimeoutSeconds,
		MaxRunSeconds:   wrt.RunTask.MaxRunSeconds,
		TimeoutAt:       &timeoutAt,
		MaxRunTimeoutAt: &maxRunTimeoutAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// 根据是否为全局 Run Task 设置不同的关联字段
	if isGlobalRunTask(wrt) {
		// 全局 Run Task：使用 RunTaskID
		taskResult.RunTaskID = &wrt.RunTaskID
		log.Printf("[RunTask] Creating result for global run task %s", wrt.RunTaskID)
	} else {
		// Workspace 级别 Run Task：使用 WorkspaceRunTaskID
		taskResult.WorkspaceRunTaskID = &wrt.WorkspaceRunTaskID
		log.Printf("[RunTask] Creating result for workspace run task %s", wrt.WorkspaceRunTaskID)
	}

	if err := e.db.Create(taskResult).Error; err != nil {
		result.err = fmt.Errorf("failed to create run task result: %w", err)
		result.passed = false
		return result
	}

	// Build webhook payload
	payload := e.buildWebhookPayload(task, wrt, taskResult)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		result.err = fmt.Errorf("failed to marshal payload: %w", err)
		e.updateResultStatus(taskResult, models.RunTaskResultError, err.Error())
		result.passed = false
		return result
	}

	// Save request payload
	taskResult.RequestPayload = payload
	taskResult.Status = models.RunTaskResultRunning
	taskResult.StartedAt = &now
	e.db.Save(taskResult)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", wrt.RunTask.EndpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		result.err = fmt.Errorf("failed to create request: %w", err)
		e.updateResultStatus(taskResult, models.RunTaskResultError, err.Error())
		result.passed = false
		return result
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "IaC-Platform/1.0")

	// Add HMAC signature if configured
	if wrt.RunTask.HMACKeyEncrypted != "" {
		hmacKey, err := crypto.DecryptValue(wrt.RunTask.HMACKeyEncrypted)
		if err == nil && hmacKey != "" {
			signature := calculateHMAC(payloadBytes, hmacKey)
			req.Header.Set("X-TFC-Task-Signature", "sha512="+signature)
		}
	}

	// Send request
	log.Printf("[RunTask] Sending webhook to %s for result %s", wrt.RunTask.EndpointURL, resultID)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		result.err = fmt.Errorf("failed to send request: %w", err)
		e.updateResultStatus(taskResult, models.RunTaskResultError, err.Error())
		result.passed = false
		return result
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		errMsg := fmt.Sprintf("external service returned status %d", resp.StatusCode)
		result.err = fmt.Errorf(errMsg)
		e.updateResultStatus(taskResult, models.RunTaskResultError, errMsg)
		result.passed = false
		return result
	}

	log.Printf("[RunTask] Webhook sent successfully for result %s, waiting for callback", resultID)

	// Set the result ID for tracking
	result.resultID = resultID

	// For async responses, the result will be updated via callback
	// Return true for now, the actual result will be determined by callback
	return result
}

// buildWebhookPayload builds the webhook payload
func (e *RunTaskExecutor) buildWebhookPayload(
	task *models.WorkspaceTask,
	wrt *models.WorkspaceRunTask,
	result *models.RunTaskResult,
) models.JSONB {
	payload := models.JSONB{
		"payload_version": 1,
		"stage":           string(wrt.Stage),
		"access_token":    result.AccessToken,
		"capabilities": map[string]interface{}{
			"outcomes": true,
		},
		"task_result_id":                result.ResultID,
		"task_result_callback_url":      result.CallbackURL,
		"task_result_enforcement_level": string(wrt.EnforcementLevel),
		"task_id":                       task.ID,
		"task_type":                     task.TaskType,
		"task_status":                   task.Status,
		"task_description":              task.Description,
		"task_created_at":               task.CreatedAt.Format(time.RFC3339),
		"task_app_url":                  fmt.Sprintf("%s/workspaces/%s/tasks/%d", e.getBaseURL(), task.WorkspaceID, task.ID),
		"workspace_id":                  task.WorkspaceID,
		"timeout_seconds":               wrt.RunTask.TimeoutSeconds,
	}

	// Add plan data URLs for post_plan/pre_apply/post_apply stages
	if wrt.Stage != models.RunTaskStagePrePlan {
		baseURL := e.getBaseURL()
		payload["plan_json_api_url"] = fmt.Sprintf("%s/api/v1/workspaces/%s/tasks/%d/plan-json", baseURL, task.WorkspaceID, task.ID)
		payload["resource_changes_api_url"] = fmt.Sprintf("%s/api/v1/workspaces/%s/tasks/%d/resource-changes", baseURL, task.WorkspaceID, task.ID)
	}

	return payload
}

// updateResultStatus updates the run task result status
func (e *RunTaskExecutor) updateResultStatus(result *models.RunTaskResult, status models.RunTaskResultStatus, message string) {
	now := time.Now()
	result.Status = status
	result.Message = message
	result.CompletedAt = &now
	result.UpdatedAt = now
	e.db.Save(result)
}

// HandleCallback handles callback from external service
func (e *RunTaskExecutor) HandleCallback(resultID string, callbackData *models.RunTaskCallbackPayload) error {
	// Note: Removed global mutex lock to prevent blocking.
	// Database transactions provide sufficient concurrency control.

	var result models.RunTaskResult
	if err := e.db.Where("result_id = ?", resultID).First(&result).Error; err != nil {
		return fmt.Errorf("result not found: %w", err)
	}

	// Check if already completed
	if result.Status == models.RunTaskResultPassed ||
		result.Status == models.RunTaskResultFailed ||
		result.Status == models.RunTaskResultTimeout {
		return fmt.Errorf("result already completed with status: %s", result.Status)
	}

	now := time.Now()
	attrs := callbackData.Data.Attributes

	// Update status
	switch attrs.Status {
	case "running":
		result.Status = models.RunTaskResultRunning
		result.LastHeartbeatAt = &now
		// Reset timeout
		timeoutAt := now.Add(time.Duration(result.TimeoutSeconds) * time.Second)
		result.TimeoutAt = &timeoutAt
	case "passed":
		result.Status = models.RunTaskResultPassed
		result.CompletedAt = &now
	case "failed":
		result.Status = models.RunTaskResultFailed
		result.CompletedAt = &now
	default:
		return fmt.Errorf("invalid status: %s", attrs.Status)
	}

	result.Message = attrs.Message
	result.URL = attrs.URL
	result.UpdatedAt = now

	// Save response payload as map
	responseMap := map[string]interface{}{
		"data": map[string]interface{}{
			"type": callbackData.Data.Type,
			"attributes": map[string]interface{}{
				"status":  callbackData.Data.Attributes.Status,
				"message": callbackData.Data.Attributes.Message,
				"url":     callbackData.Data.Attributes.URL,
			},
		},
	}
	result.ResponsePayload = responseMap

	if err := e.db.Save(&result).Error; err != nil {
		return fmt.Errorf("failed to update result: %w", err)
	}

	// Save outcomes if provided
	if callbackData.Data.Relationships != nil && callbackData.Data.Relationships.Outcomes != nil {
		for _, outcomeData := range callbackData.Data.Relationships.Outcomes.Data {
			outcome := &models.RunTaskOutcome{
				RunTaskResultID: resultID,
				OutcomeID:       outcomeData.Attributes.OutcomeID,
				Description:     outcomeData.Attributes.Description,
				Body:            outcomeData.Attributes.Body,
				URL:             outcomeData.Attributes.URL,
				Tags:            outcomeData.Attributes.Tags,
				CreatedAt:       now,
			}
			e.db.Create(outcome)
		}
	}

	log.Printf("[RunTask] Callback processed for result %s, status: %s", resultID, result.Status)
	return nil
}

// GetResultByID gets a run task result by ID
func (e *RunTaskExecutor) GetResultByID(resultID string) (*models.RunTaskResult, error) {
	var result models.RunTaskResult
	if err := e.db.Preload("Outcomes").Where("result_id = ?", resultID).First(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

// GetResultsForTask gets all run task results for a task
func (e *RunTaskExecutor) GetResultsForTask(taskID uint) ([]models.RunTaskResult, error) {
	var results []models.RunTaskResult
	if err := e.db.Preload("WorkspaceRunTask.RunTask").Preload("Outcomes").
		Where("task_id = ?", taskID).
		Order("stage, created_at").
		Find(&results).Error; err != nil {
		return nil, err
	}

	// 对于全局 Run Task，需要单独查询 Run Task 信息
	// 因为全局 Run Task 使用 RunTaskID 而不是 WorkspaceRunTaskID
	for i := range results {
		if results[i].RunTaskID != nil && results[i].WorkspaceRunTask == nil {
			var runTask models.RunTask
			if err := e.db.Where("run_task_id = ?", *results[i].RunTaskID).First(&runTask).Error; err == nil {
				// 创建一个虚拟的 WorkspaceRunTask 来存储 Run Task 信息
				results[i].WorkspaceRunTask = &models.WorkspaceRunTask{
					RunTaskID: runTask.RunTaskID,
					RunTask:   &runTask,
				}
			}
		}
	}

	return results, nil
}

// calculateHMAC calculates HMAC-SHA512 signature
func calculateHMAC(payload []byte, key string) string {
	h := hmac.New(sha512.New, []byte(key))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}
