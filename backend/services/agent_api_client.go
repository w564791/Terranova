package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries  int           // 最大重试次数
	BaseDelay   time.Duration // 基础延迟时间
	MaxDelay    time.Duration // 最大延迟时间
	RetryOn5xx  bool          // 是否在5xx错误时重试
	RetryOnConn bool          // 是否在连接错误时重试
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxRetries:  3,
	BaseDelay:   1 * time.Second,
	MaxDelay:    10 * time.Second,
	RetryOn5xx:  true,
	RetryOnConn: true,
}

// AgentAPIClient handles HTTP communication with IAC Server
type AgentAPIClient struct {
	baseURL     string
	token       string
	httpClient  *http.Client
	retryConfig RetryConfig
}

// NewAgentAPIClient creates a new API client
func NewAgentAPIClient(baseURL, token string) *AgentAPIClient {
	// 创建带有连接池优化的 Transport
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &AgentAPIClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout:   60 * time.Second, // 增加超时时间到60秒
			Transport: transport,
		},
		retryConfig: DefaultRetryConfig,
	}
}

// Register registers the agent with the server
func (c *AgentAPIClient) Register(agentName string) (string, string, error) {
	reqBody := map[string]interface{}{
		"name": agentName,
	}

	respBody, err := c.doRequest("POST", "/api/v1/agents/register", reqBody)
	if err != nil {
		return "", "", fmt.Errorf("failed to register: %w", err)
	}

	agentID, _ := respBody["agent_id"].(string)
	poolID, _ := respBody["pool_id"].(string)

	return agentID, poolID, nil
}

// GetTaskData retrieves complete task execution data
func (c *AgentAPIClient) GetTaskData(taskID uint) (map[string]interface{}, error) {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/data", taskID)
	return c.doRequest("GET", path, nil)
}

// UploadLogChunk uploads a chunk of task log
func (c *AgentAPIClient) UploadLogChunk(taskID uint, phase, content string, offset int64, checksum string) (int64, error) {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/logs/chunk", taskID)

	reqBody := map[string]interface{}{
		"phase":    phase,
		"content":  content,
		"offset":   offset,
		"checksum": checksum,
	}

	respBody, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return 0, fmt.Errorf("failed to upload log chunk: %w", err)
	}

	nextOffset, ok := respBody["next_offset"].(float64)
	if !ok {
		return 0, fmt.Errorf("invalid next_offset in response")
	}

	return int64(nextOffset), nil
}

// UpdateTaskStatus updates task status
func (c *AgentAPIClient) UpdateTaskStatus(taskID uint, status string, updates map[string]interface{}) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/status", taskID)

	reqBody := map[string]interface{}{
		"status": status,
	}

	// Merge additional updates
	for k, v := range updates {
		reqBody[k] = v
	}

	_, err := c.doRequest("PUT", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// SaveTaskState saves task state version
func (c *AgentAPIClient) SaveTaskState(taskID uint, content map[string]interface{}, checksum string, size int) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/state", taskID)

	reqBody := map[string]interface{}{
		"content":  content,
		"checksum": checksum,
		"size":     size,
	}

	_, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// doRequest performs HTTP request with authentication
func (c *AgentAPIClient) doRequest(method, path string, body interface{}) (map[string]interface{}, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respData))
	}

	// Parse response
	var result map[string]interface{}
	if len(respData) > 0 {
		if err := json.Unmarshal(respData, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return result, nil
}

// Ping sends heartbeat to server (legacy, replaced by C&C)
func (c *AgentAPIClient) Ping(agentID, status string) error {
	path := fmt.Sprintf("/api/v1/agents/%s/ping", agentID)

	reqBody := map[string]interface{}{
		"status": status,
	}

	_, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to ping: %w", err)
	}

	return nil
}

// ============================================================================
// New methods for Agent Mode refactoring
// ============================================================================

// GetPlanTask retrieves a plan task by ID
// 返回值包含：task对象 + snapshot_resources数据（用于缓存）
func (c *AgentAPIClient) GetPlanTask(taskID uint) (*models.WorkspaceTask, error) {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/plan-task", taskID)

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan task: %w", err)
	}

	// Parse response into WorkspaceTask
	task := &models.WorkspaceTask{}
	if taskData, ok := respBody["task"].(map[string]interface{}); ok {
		task.ID = getUint(taskData, "id")
		task.WorkspaceID = getString(taskData, "workspace_id")
		task.TaskType = models.TaskType(getString(taskData, "task_type"))
		task.Context = getMap(taskData, "context")

		// 解析 created_at
		if createdAtStr, ok := taskData["created_at"].(string); ok && createdAtStr != "" {
			if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
				task.CreatedAt = t
			}
		}

		// 【修复】解析 plan_task_id 字段
		if planTaskID := getUint(taskData, "plan_task_id"); planTaskID > 0 {
			task.PlanTaskID = &planTaskID
		}

		// 【Phase 1优化】解析 agent_id 字段
		if agentID, ok := taskData["agent_id"].(string); ok && agentID != "" {
			task.AgentID = &agentID
			log.Printf("[AgentAPIClient] Parsed agent_id: %s", agentID)
		} else {
			log.Printf("[AgentAPIClient] Failed to parse agent_id, taskData[\"agent_id\"] = %v (type: %T)", taskData["agent_id"], taskData["agent_id"])
		}

		// 【新增】解析 agent_name 字段（hostname）
		if agentName, ok := taskData["agent_name"].(string); ok && agentName != "" {
			// 将 agent_name 存储到 Context 中，供后续使用
			if task.Context == nil {
				task.Context = make(map[string]interface{})
			}
			task.Context["_agent_name"] = agentName
			log.Printf("[AgentAPIClient] Parsed agent_name: %s", agentName)
		}

		// 【Phase 1优化】解析 plan_hash 字段
		if planHash, ok := taskData["plan_hash"].(string); ok && planHash != "" {
			task.PlanHash = planHash
		}

		// 【修复】解析快照字段
		if snapshotCreatedAt, ok := taskData["snapshot_created_at"].(string); ok && snapshotCreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, snapshotCreatedAt); err == nil {
				task.SnapshotCreatedAt = &t
			}
		}

		if snapshotResourceVersions, ok := taskData["snapshot_resource_versions"].(map[string]interface{}); ok {
			task.SnapshotResourceVersions = models.JSONB(snapshotResourceVersions)
		}

		// 处理 snapshot_variables - 支持两种格式:
		// 1. 数组格式: [...]
		// 2. 对象格式: {"_array": [...]}
		if snapshotVariables, ok := taskData["snapshot_variables"].([]interface{}); ok {
			// 格式1: 直接是数组
			variables := make([]models.WorkspaceVariable, 0, len(snapshotVariables))
			for _, item := range snapshotVariables {
				if varMap, ok := item.(map[string]interface{}); ok {
					variable := models.WorkspaceVariable{
						ID:           getUint(varMap, "id"),
						WorkspaceID:  getString(varMap, "workspace_id"),
						VariableID:   getString(varMap, "variable_id"),
						Version:      getInt(varMap, "version"),
						Key:          getString(varMap, "key"),
						Value:        getString(varMap, "value"),
						VariableType: models.VariableType(getString(varMap, "variable_type")),
						Sensitive:    getBool(varMap, "sensitive"),
						Description:  getString(varMap, "description"),
						ValueFormat:  models.ValueFormat(getString(varMap, "value_format")),
					}
					variables = append(variables, variable)
				}
			}
			// Convert to JSONB format (map with _array key for compatibility)
			task.SnapshotVariables = models.JSONB{"_array": variables}
		} else if snapshotVarsMap, ok := taskData["snapshot_variables"].(map[string]interface{}); ok {
			// 格式2: {"_array": [...]} 对象格式
			if arrayData, hasArray := snapshotVarsMap["_array"].([]interface{}); hasArray {
				variables := make([]models.WorkspaceVariable, 0, len(arrayData))
				for _, item := range arrayData {
					if varMap, ok := item.(map[string]interface{}); ok {
						variable := models.WorkspaceVariable{
							ID:           getUint(varMap, "id"),
							WorkspaceID:  getString(varMap, "workspace_id"),
							VariableID:   getString(varMap, "variable_id"),
							Version:      getInt(varMap, "version"),
							Key:          getString(varMap, "key"),
							Value:        getString(varMap, "value"),
							VariableType: models.VariableType(getString(varMap, "variable_type")),
							Sensitive:    getBool(varMap, "sensitive"),
							Description:  getString(varMap, "description"),
							ValueFormat:  models.ValueFormat(getString(varMap, "value_format")),
						}
						variables = append(variables, variable)
					}
				}
				task.SnapshotVariables = models.JSONB{"_array": variables}
			} else {
				// 直接使用原始 map
				task.SnapshotVariables = models.JSONB(snapshotVarsMap)
			}
		}

		if snapshotProviderConfig, ok := taskData["snapshot_provider_config"].(map[string]interface{}); ok {
			task.SnapshotProviderConfig = models.JSONB(snapshotProviderConfig)
		}
	}

	// 【新增】将 snapshot_resources 存储到 task 的临时字段中，供 RemoteDataAccessor 使用
	// 注意：这不是 models.WorkspaceTask 的标准字段，而是用于传递数据
	if snapshotResources, ok := respBody["snapshot_resources"].([]interface{}); ok {
		// 将 snapshot_resources 存储为 Context 的一部分（临时方案）
		if task.Context == nil {
			task.Context = make(map[string]interface{})
		}
		task.Context["_snapshot_resources"] = snapshotResources
	}

	// Decode plan_data if present (API returns base64, we need binary)
	if planDataStr, ok := respBody["plan_data"].(string); ok && planDataStr != "" {
		decodedData, err := base64.StdEncoding.DecodeString(planDataStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode plan_data: %w", err)
		}
		task.PlanData = decodedData
	}

	return task, nil
}

// LockWorkspace locks a workspace
func (c *AgentAPIClient) LockWorkspace(workspaceID, userID, reason string) error {
	path := fmt.Sprintf("/api/v1/agents/workspaces/%s/lock", workspaceID)

	reqBody := map[string]interface{}{
		"user_id": userID,
		"reason":  reason,
	}

	_, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to lock workspace: %w", err)
	}

	return nil
}

// UnlockWorkspace unlocks a workspace
func (c *AgentAPIClient) UnlockWorkspace(workspaceID string) error {
	path := fmt.Sprintf("/api/v1/agents/workspaces/%s/unlock", workspaceID)

	_, err := c.doRequest("POST", path, nil)
	if err != nil {
		return fmt.Errorf("failed to unlock workspace: %w", err)
	}

	return nil
}

// ParsePlanChanges requests server to parse plan changes
func (c *AgentAPIClient) ParsePlanChanges(taskID uint, planOutput string) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/parse-plan-changes", taskID)

	reqBody := map[string]interface{}{
		"plan_output": planOutput,
	}

	_, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to parse plan changes: %w", err)
	}

	return nil
}

// GetTaskLogs retrieves task logs
func (c *AgentAPIClient) GetTaskLogs(taskID uint) ([]models.TaskLog, error) {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/logs", taskID)

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get task logs: %w", err)
	}

	// Parse logs from response
	logs := []models.TaskLog{}
	if logsData, ok := respBody["logs"].([]interface{}); ok {
		for _, item := range logsData {
			if logMap, ok := item.(map[string]interface{}); ok {
				log := models.TaskLog{
					ID:      getUint(logMap, "id"),
					TaskID:  taskID,
					Phase:   getString(logMap, "phase"),
					Content: getString(logMap, "content"),
					Level:   getString(logMap, "level"),
				}
				logs = append(logs, log)
			}
		}
	}

	return logs, nil
}

// GetMaxStateVersion retrieves the maximum state version for a workspace
func (c *AgentAPIClient) GetMaxStateVersion(workspaceID string) (int, error) {
	path := fmt.Sprintf("/api/v1/agents/workspaces/%s/state/max-version", workspaceID)

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get max state version: %w", err)
	}

	maxVersion := 0
	if v, ok := respBody["max_version"].(float64); ok {
		maxVersion = int(v)
	}

	return maxVersion, nil
}

// UploadPlanData uploads plan_data to server
func (c *AgentAPIClient) UploadPlanData(taskID uint, encodedData string) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/plan-data", taskID)

	reqBody := map[string]interface{}{
		"plan_data": encodedData,
	}

	_, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to upload plan data: %w", err)
	}

	return nil
}

// UploadResourceChanges uploads parsed resource changes to server
func (c *AgentAPIClient) UploadResourceChanges(taskID uint, resourceChanges []map[string]interface{}) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/parse-plan-changes", taskID)

	reqBody := map[string]interface{}{
		"resource_changes": resourceChanges,
	}

	_, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to upload resource changes: %w", err)
	}

	return nil
}

// UploadPlanJSON uploads plan_json to server
func (c *AgentAPIClient) UploadPlanJSON(taskID uint, planJSON map[string]interface{}) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/plan-json", taskID)

	reqBody := map[string]interface{}{
		"plan_json": planJSON,
	}

	_, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to upload plan json: %w", err)
	}

	return nil
}

// GetTerraformVersion retrieves terraform version configuration from server
func (c *AgentAPIClient) GetTerraformVersion(version string) (*models.TerraformVersion, error) {
	var path string
	if version == "latest" || version == "" {
		path = "/api/v1/agents/terraform-versions/default"
	} else {
		path = fmt.Sprintf("/api/v1/agents/terraform-versions/%s", version)
	}

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get terraform version: %w", err)
	}

	// Parse response into TerraformVersion
	tfVersion := &models.TerraformVersion{}
	if versionData, ok := respBody["version"].(map[string]interface{}); ok {
		if id, ok := versionData["id"].(float64); ok {
			tfVersion.ID = int(id)
		}
		if ver, ok := versionData["version"].(string); ok {
			tfVersion.Version = ver
		}
		if url, ok := versionData["download_url"].(string); ok {
			tfVersion.DownloadURL = url
		}
		if checksum, ok := versionData["checksum"].(string); ok {
			tfVersion.Checksum = checksum
		}
		if enabled, ok := versionData["enabled"].(bool); ok {
			tfVersion.Enabled = enabled
		}
		if deprecated, ok := versionData["deprecated"].(bool); ok {
			tfVersion.Deprecated = deprecated
		}
		if isDefault, ok := versionData["is_default"].(bool); ok {
			tfVersion.IsDefault = isDefault
		}
	}

	return tfVersion, nil
}

// GetPoolSecrets retrieves HCP secrets for the agent's pool
func (c *AgentAPIClient) GetPoolSecrets() (map[string]interface{}, error) {
	path := "/api/v1/agents/pool/secrets"

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool secrets: %w", err)
	}

	return respBody, nil
}

// UpdateResourceStatus 更新资源状态（Agent 模式）
func (c *AgentAPIClient) UpdateResourceStatus(taskID uint, resourceAddress, status, action string) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/resource-status", taskID)

	payload := map[string]interface{}{
		"resource_address": resourceAddress,
		"apply_status":     status,
		"action":           action,
	}

	_, err := c.doRequest("POST", path, payload)
	if err != nil {
		return fmt.Errorf("failed to update resource status: %w", err)
	}

	return nil
}

// UpdateWorkspaceFields 更新 Workspace 的指定字段（Agent 模式）
func (c *AgentAPIClient) UpdateWorkspaceFields(workspaceID string, updates map[string]interface{}) error {
	path := fmt.Sprintf("/api/v1/agents/workspaces/%s/fields", workspaceID)

	_, err := c.doRequest("PATCH", path, updates)
	if err != nil {
		return fmt.Errorf("failed to update workspace fields: %w", err)
	}

	return nil
}

// GetTerraformLockHCL 获取 Terraform Lock 文件内容（Agent 模式）
func (c *AgentAPIClient) GetTerraformLockHCL(workspaceID string) (string, error) {
	path := fmt.Sprintf("/api/v1/agents/workspaces/%s/terraform-lock-hcl", workspaceID)

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get terraform lock hcl: %w", err)
	}

	lockHCL, _ := respBody["terraform_lock_hcl"].(string)
	return lockHCL, nil
}

// SaveTerraformLockHCL 保存 Terraform Lock 文件内容（Agent 模式）
func (c *AgentAPIClient) SaveTerraformLockHCL(workspaceID string, lockContent string) error {
	path := fmt.Sprintf("/api/v1/agents/workspaces/%s/terraform-lock-hcl", workspaceID)

	reqBody := map[string]interface{}{
		"terraform_lock_hcl": lockContent,
	}

	_, err := c.doRequest("PUT", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to save terraform lock hcl: %w", err)
	}

	return nil
}

// ============================================================================
// 带重试的请求方法
// ============================================================================

// isRetryableError 判断错误是否可以重试
func (c *AgentAPIClient) isRetryableError(err error, statusCode int) bool {
	if err != nil {
		// 网络连接错误可以重试
		if c.retryConfig.RetryOnConn {
			// 检查是否是网络相关错误
			if netErr, ok := err.(net.Error); ok {
				return netErr.Timeout() || netErr.Temporary()
			}
		}
		return true // 其他错误也尝试重试
	}

	// 5xx 服务端错误可以重试
	if c.retryConfig.RetryOn5xx && statusCode >= 500 && statusCode < 600 {
		return true
	}

	return false
}

// calculateBackoff 计算退避时间（指数退避）
func (c *AgentAPIClient) calculateBackoff(attempt int) time.Duration {
	delay := c.retryConfig.BaseDelay * time.Duration(1<<uint(attempt))
	if delay > c.retryConfig.MaxDelay {
		delay = c.retryConfig.MaxDelay
	}
	return delay
}

// doRequestWithRetry 带重试的 HTTP 请求
func (c *AgentAPIClient) doRequestWithRetry(method, path string, body interface{}, maxRetries int) (map[string]interface{}, error) {
	var lastErr error
	var lastStatusCode int

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 构建请求
		url := c.baseURL + path

		var reqBody io.Reader
		var bodyBytes []byte
		if body != nil {
			var err error
			bodyBytes, err = json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			reqBody = bytes.NewBuffer(bodyBytes)
		}

		req, err := http.NewRequest(method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)

		// Execute request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			lastStatusCode = 0

			// 判断是否可以重试
			if attempt < maxRetries && c.isRetryableError(err, 0) {
				delay := c.calculateBackoff(attempt)
				log.Printf("[AgentAPIClient] Request failed (attempt %d/%d): %v, retrying in %v",
					attempt+1, maxRetries+1, err, delay)
				time.Sleep(delay)
				continue
			}
			return nil, fmt.Errorf("failed to execute request after %d attempts: %w", attempt+1, err)
		}

		// Read response body
		respData, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				delay := c.calculateBackoff(attempt)
				log.Printf("[AgentAPIClient] Failed to read response (attempt %d/%d): %v, retrying in %v",
					attempt+1, maxRetries+1, err, delay)
				time.Sleep(delay)
				continue
			}
			return nil, fmt.Errorf("failed to read response after %d attempts: %w", attempt+1, err)
		}

		lastStatusCode = resp.StatusCode

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respData))

			// 判断是否可以重试（5xx 错误）
			if attempt < maxRetries && c.isRetryableError(nil, resp.StatusCode) {
				delay := c.calculateBackoff(attempt)
				log.Printf("[AgentAPIClient] Request failed with status %d (attempt %d/%d), retrying in %v",
					resp.StatusCode, attempt+1, maxRetries+1, delay)
				time.Sleep(delay)
				continue
			}
			return nil, lastErr
		}

		// Parse response
		var result map[string]interface{}
		if len(respData) > 0 {
			if err := json.Unmarshal(respData, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}
		}

		// 成功时记录重试信息（如果有重试）
		if attempt > 0 {
			log.Printf("[AgentAPIClient] Request succeeded after %d retries", attempt)
		}

		return result, nil
	}

	return nil, fmt.Errorf("request failed after %d attempts, last error: %w, last status: %d",
		maxRetries+1, lastErr, lastStatusCode)
}

// ============================================================================
// 使用重试的关键 API 方法
// ============================================================================

// GetTaskDataWithRetry 获取任务数据（带重试）
func (c *AgentAPIClient) GetTaskDataWithRetry(taskID uint) (map[string]interface{}, error) {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/data", taskID)
	return c.doRequestWithRetry("GET", path, nil, c.retryConfig.MaxRetries)
}

// GetPlanTaskWithRetry 获取 Plan 任务（带重试）- 这是最关键的 API
func (c *AgentAPIClient) GetPlanTaskWithRetry(taskID uint) (*models.WorkspaceTask, error) {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/plan-task", taskID)

	respBody, err := c.doRequestWithRetry("GET", path, nil, c.retryConfig.MaxRetries)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan task: %w", err)
	}

	// 复用原有的解析逻辑
	return c.parsePlanTaskResponse(respBody)
}

// parsePlanTaskResponse 解析 Plan 任务响应（提取公共逻辑）
func (c *AgentAPIClient) parsePlanTaskResponse(respBody map[string]interface{}) (*models.WorkspaceTask, error) {
	// Parse response into WorkspaceTask
	task := &models.WorkspaceTask{}
	if taskData, ok := respBody["task"].(map[string]interface{}); ok {
		task.ID = getUint(taskData, "id")
		task.WorkspaceID = getString(taskData, "workspace_id")
		task.TaskType = models.TaskType(getString(taskData, "task_type"))
		task.Context = getMap(taskData, "context")

		// 解析 created_at
		if createdAtStr, ok := taskData["created_at"].(string); ok && createdAtStr != "" {
			if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
				task.CreatedAt = t
			}
		}

		// 解析 plan_task_id 字段
		if planTaskID := getUint(taskData, "plan_task_id"); planTaskID > 0 {
			task.PlanTaskID = &planTaskID
		}

		// 解析 agent_id 字段
		if agentID, ok := taskData["agent_id"].(string); ok && agentID != "" {
			task.AgentID = &agentID
		}

		// 解析 agent_name 字段（hostname）
		if agentName, ok := taskData["agent_name"].(string); ok && agentName != "" {
			if task.Context == nil {
				task.Context = make(map[string]interface{})
			}
			task.Context["_agent_name"] = agentName
		}

		// 解析 plan_hash 字段
		if planHash, ok := taskData["plan_hash"].(string); ok && planHash != "" {
			task.PlanHash = planHash
		}

		// 解析快照字段
		if snapshotCreatedAt, ok := taskData["snapshot_created_at"].(string); ok && snapshotCreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, snapshotCreatedAt); err == nil {
				task.SnapshotCreatedAt = &t
			}
		}

		if snapshotResourceVersions, ok := taskData["snapshot_resource_versions"].(map[string]interface{}); ok {
			task.SnapshotResourceVersions = models.JSONB(snapshotResourceVersions)
		}

		// 处理 snapshot_variables
		if snapshotVariables, ok := taskData["snapshot_variables"].([]interface{}); ok {
			variables := make([]models.WorkspaceVariable, 0, len(snapshotVariables))
			for _, item := range snapshotVariables {
				if varMap, ok := item.(map[string]interface{}); ok {
					variable := models.WorkspaceVariable{
						ID:           getUint(varMap, "id"),
						WorkspaceID:  getString(varMap, "workspace_id"),
						VariableID:   getString(varMap, "variable_id"),
						Version:      getInt(varMap, "version"),
						Key:          getString(varMap, "key"),
						Value:        getString(varMap, "value"),
						VariableType: models.VariableType(getString(varMap, "variable_type")),
						Sensitive:    getBool(varMap, "sensitive"),
						Description:  getString(varMap, "description"),
						ValueFormat:  models.ValueFormat(getString(varMap, "value_format")),
					}
					variables = append(variables, variable)
				}
			}
			task.SnapshotVariables = models.JSONB{"_array": variables}
		} else if snapshotVarsMap, ok := taskData["snapshot_variables"].(map[string]interface{}); ok {
			if arrayData, hasArray := snapshotVarsMap["_array"].([]interface{}); hasArray {
				variables := make([]models.WorkspaceVariable, 0, len(arrayData))
				for _, item := range arrayData {
					if varMap, ok := item.(map[string]interface{}); ok {
						variable := models.WorkspaceVariable{
							ID:           getUint(varMap, "id"),
							WorkspaceID:  getString(varMap, "workspace_id"),
							VariableID:   getString(varMap, "variable_id"),
							Version:      getInt(varMap, "version"),
							Key:          getString(varMap, "key"),
							Value:        getString(varMap, "value"),
							VariableType: models.VariableType(getString(varMap, "variable_type")),
							Sensitive:    getBool(varMap, "sensitive"),
							Description:  getString(varMap, "description"),
							ValueFormat:  models.ValueFormat(getString(varMap, "value_format")),
						}
						variables = append(variables, variable)
					}
				}
				task.SnapshotVariables = models.JSONB{"_array": variables}
			} else {
				task.SnapshotVariables = models.JSONB(snapshotVarsMap)
			}
		}

		if snapshotProviderConfig, ok := taskData["snapshot_provider_config"].(map[string]interface{}); ok {
			task.SnapshotProviderConfig = models.JSONB(snapshotProviderConfig)
		}
	}

	// 处理 snapshot_resources
	if snapshotResources, ok := respBody["snapshot_resources"].([]interface{}); ok {
		if task.Context == nil {
			task.Context = make(map[string]interface{})
		}
		task.Context["_snapshot_resources"] = snapshotResources
	}

	// Decode plan_data
	if planDataStr, ok := respBody["plan_data"].(string); ok && planDataStr != "" {
		decodedData, err := base64.StdEncoding.DecodeString(planDataStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode plan_data: %w", err)
		}
		task.PlanData = decodedData
	}

	return task, nil
}

// SaveTaskStateWithRetry 保存 State（带重试）- 最关键的写操作
func (c *AgentAPIClient) SaveTaskStateWithRetry(taskID uint, content map[string]interface{}, checksum string, size int) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/state", taskID)

	reqBody := map[string]interface{}{
		"content":  content,
		"checksum": checksum,
		"size":     size,
	}

	_, err := c.doRequestWithRetry("POST", path, reqBody, c.retryConfig.MaxRetries)
	if err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// UpdateTaskStatusWithRetry 更新任务状态（带重试）
func (c *AgentAPIClient) UpdateTaskStatusWithRetry(taskID uint, status string, updates map[string]interface{}) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/status", taskID)

	reqBody := map[string]interface{}{
		"status": status,
	}

	for k, v := range updates {
		reqBody[k] = v
	}

	_, err := c.doRequestWithRetry("PUT", path, reqBody, c.retryConfig.MaxRetries)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// UploadPlanDataWithRetry 上传 Plan 数据（带重试）
func (c *AgentAPIClient) UploadPlanDataWithRetry(taskID uint, encodedData string) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/plan-data", taskID)

	reqBody := map[string]interface{}{
		"plan_data": encodedData,
	}

	_, err := c.doRequestWithRetry("POST", path, reqBody, c.retryConfig.MaxRetries)
	if err != nil {
		return fmt.Errorf("failed to upload plan data: %w", err)
	}

	return nil
}

// UploadPlanJSONWithRetry 上传 Plan JSON（带重试）
func (c *AgentAPIClient) UploadPlanJSONWithRetry(taskID uint, planJSON map[string]interface{}) error {
	path := fmt.Sprintf("/api/v1/agents/tasks/%d/plan-json", taskID)

	reqBody := map[string]interface{}{
		"plan_json": planJSON,
	}

	_, err := c.doRequestWithRetry("POST", path, reqBody, c.retryConfig.MaxRetries)
	if err != nil {
		return fmt.Errorf("failed to upload plan json: %w", err)
	}

	return nil
}
