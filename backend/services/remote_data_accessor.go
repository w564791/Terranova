package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"time"
)

// RemoteDataAccessor implements DataAccessor interface for Agent mode
// It accesses data through HTTP API calls to the server
type RemoteDataAccessor struct {
	apiClient     *AgentAPIClient
	taskData      map[string]interface{} // Cached task data
	streamManager *OutputStreamManager   // For WebSocket updates
}

// NewRemoteDataAccessor creates a new remote data accessor
func NewRemoteDataAccessor(apiClient *AgentAPIClient) *RemoteDataAccessor {
	return &RemoteDataAccessor{
		apiClient: apiClient,
		taskData:  make(map[string]interface{}),
	}
}

// SetStreamManager sets the stream manager for WebSocket updates
func (a *RemoteDataAccessor) SetStreamManager(streamManager *OutputStreamManager) {
	a.streamManager = streamManager
}

// LoadTaskData loads and caches task data from server (带重试)
func (a *RemoteDataAccessor) LoadTaskData(taskID uint) error {
	// 使用带重试的方法获取任务数据
	data, err := a.apiClient.GetTaskDataWithRetry(taskID)
	if err != nil {
		return fmt.Errorf("failed to load task data: %w", err)
	}

	a.taskData = data

	// 记录加载的数据概要
	log.Printf("[RemoteDataAccessor] Task %d data loaded: workspace=%v, resources=%d, variables=%d, outputs=%d, remote_data=%d",
		taskID,
		data["workspace"] != nil,
		len(getSlice(data, "resources")),
		len(getSlice(data, "variables")),
		len(getSlice(data, "outputs")),
		len(getSlice(data, "remote_data")))

	// 记录 module_versions
	if moduleVersions, ok := data["module_versions"].(map[string]interface{}); ok {
		log.Printf("[RemoteDataAccessor] Module versions loaded: %d items", len(moduleVersions))
	}

	return nil
}

// getSlice 辅助函数：安全获取 slice
func getSlice(m map[string]interface{}, key string) []interface{} {
	if v, ok := m[key].([]interface{}); ok {
		return v
	}
	return []interface{}{}
}

// ============================================================================
// Workspace 相关
// ============================================================================

// GetWorkspace 获取 Workspace
func (a *RemoteDataAccessor) GetWorkspace(workspaceID string) (*models.Workspace, error) {
	workspaceData, ok := a.taskData["workspace"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("workspace data not found in cache")
	}

	// Convert map to Workspace struct
	workspace := &models.Workspace{
		WorkspaceID:      getString(workspaceData, "workspace_id"),
		Name:             getString(workspaceData, "name"),
		TerraformVersion: getString(workspaceData, "terraform_version"),
		ExecutionMode:    models.ExecutionMode(getString(workspaceData, "execution_mode")),
		ProviderConfig:   getMap(workspaceData, "provider_config"),
		TFCode:           getMap(workspaceData, "tf_code"),
		SystemVariables:  getMap(workspaceData, "system_variables"),
	}

	return workspace, nil
}

// GetWorkspaceResources 获取 Workspace 资源列表
func (a *RemoteDataAccessor) GetWorkspaceResources(workspaceID string) ([]models.WorkspaceResource, error) {
	resourcesData, ok := a.taskData["resources"].([]interface{})
	if !ok {
		return []models.WorkspaceResource{}, nil
	}

	resources := make([]models.WorkspaceResource, 0, len(resourcesData))
	for _, item := range resourcesData {
		resMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		resource := models.WorkspaceResource{
			ID:           getUint(resMap, "id"),
			WorkspaceID:  getString(resMap, "workspace_id"),
			ResourceID:   getString(resMap, "resource_id"),
			ResourceName: getString(resMap, "resource_name"),
			ResourceType: getString(resMap, "resource_type"), // 【关键】加载 resource_type 用于查找 module version
			IsActive:     getBool(resMap, "is_active"),
		}

		// Load current version if exists
		if versionData, ok := resMap["current_version"].(map[string]interface{}); ok {
			version := &models.ResourceCodeVersion{
				ID:      getUint(versionData, "id"),
				Version: getInt(versionData, "version"),
				TFCode:  getMap(versionData, "tf_code"),
			}
			resource.CurrentVersion = version
			versionID := version.ID
			resource.CurrentVersionID = &versionID
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// GetWorkspaceVariables 获取 Workspace 变量列表
func (a *RemoteDataAccessor) GetWorkspaceVariables(workspaceID string, varType models.VariableType) ([]models.WorkspaceVariable, error) {
	variablesData, ok := a.taskData["variables"].([]interface{})
	if !ok {
		return []models.WorkspaceVariable{}, nil
	}

	variables := make([]models.WorkspaceVariable, 0)
	for _, item := range variablesData {
		varMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Filter by variable type
		if getString(varMap, "variable_type") != string(varType) {
			continue
		}

		variable := models.WorkspaceVariable{
			ID:           getUint(varMap, "id"),
			WorkspaceID:  getString(varMap, "workspace_id"),
			Key:          getString(varMap, "key"),
			Value:        getString(varMap, "value"),
			VariableType: models.VariableType(getString(varMap, "variable_type")),
			Sensitive:    getBool(varMap, "sensitive"),
			Description:  getString(varMap, "description"),
			ValueFormat:  models.ValueFormat(getString(varMap, "value_format")),
		}

		variables = append(variables, variable)
	}

	return variables, nil
}

// ============================================================================
// State 相关
// ============================================================================

// GetLatestStateVersion 获取最新的 State 版本
func (a *RemoteDataAccessor) GetLatestStateVersion(workspaceID string) (*models.WorkspaceStateVersion, error) {
	stateData, ok := a.taskData["state_version"].(map[string]interface{})
	if !ok {
		return nil, nil // No state version
	}

	stateVersion := &models.WorkspaceStateVersion{
		WorkspaceID: workspaceID,
		Version:     getInt(stateData, "version"),
		Content:     getMap(stateData, "content"),
		Checksum:    getString(stateData, "checksum"),
		SizeBytes:   getInt(stateData, "size"),
	}

	return stateVersion, nil
}

// SaveStateVersion 保存 State 版本（带重试）
func (a *RemoteDataAccessor) SaveStateVersion(version *models.WorkspaceStateVersion) error {
	// Get task ID from cached data
	taskData, ok := a.taskData["task"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("task data not found in cache")
	}

	taskID := getUint(taskData, "id")

	// 使用带重试的方法保存 State（这是最关键的写操作）
	return a.apiClient.SaveTaskStateWithRetry(taskID, version.Content, version.Checksum, version.SizeBytes)
}

// UpdateWorkspaceState 更新 Workspace 的 State
func (a *RemoteDataAccessor) UpdateWorkspaceState(workspaceID string, stateContent map[string]interface{}) error {
	// This is handled by SaveStateVersion in Agent mode
	return nil
}

// ============================================================================
// Task 相关
// ============================================================================

// GetTask 获取任务
func (a *RemoteDataAccessor) GetTask(taskID uint) (*models.WorkspaceTask, error) {
	taskData, ok := a.taskData["task"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("task data not found in cache")
	}

	task := &models.WorkspaceTask{
		ID:          getUint(taskData, "id"),
		WorkspaceID: getString(taskData, "workspace_id"),
		TaskType:    models.TaskType(getString(taskData, "task_type")),
		Context:     getMap(taskData, "context"),
	}

	// 【修复】解析 plan_task_id 字段
	if planTaskID := getUint(taskData, "plan_task_id"); planTaskID > 0 {
		task.PlanTaskID = &planTaskID
	}

	return task, nil
}

// UpdateTask 更新任务（带重试）
func (a *RemoteDataAccessor) UpdateTask(task *models.WorkspaceTask) error {
	updates := map[string]interface{}{
		"stage":           task.Stage,
		"changes_add":     task.ChangesAdd,
		"changes_change":  task.ChangesChange,
		"changes_destroy": task.ChangesDestroy,
		"duration":        task.Duration,
	}

	if task.ErrorMessage != "" {
		updates["error_message"] = task.ErrorMessage
	}

	// Add completed_at if set
	if task.CompletedAt != nil {
		updates["completed_at"] = task.CompletedAt
	}

	// Add plan_output if set
	if task.PlanOutput != "" {
		updates["plan_output"] = task.PlanOutput
	}

	// Add apply_output if set
	if task.ApplyOutput != "" {
		updates["apply_output"] = task.ApplyOutput
	}

	// 【Phase 1优化】Add plan_hash if set
	if task.PlanHash != "" {
		updates["plan_hash"] = task.PlanHash
	}

	// Add plan_task_id if set (for plan_and_apply tasks)
	if task.PlanTaskID != nil {
		updates["plan_task_id"] = *task.PlanTaskID
	}

	// Use task status if set, otherwise use "running" as default
	status := string(task.Status)
	if status == "" {
		status = "running"
	}

	// 使用带重试的方法更新任务状态
	return a.apiClient.UpdateTaskStatusWithRetry(task.ID, status, updates)
}

// SaveTaskLog 保存任务日志
func (a *RemoteDataAccessor) SaveTaskLog(taskID uint, phase, content, level string) error {
	// Upload log chunk
	_, err := a.apiClient.UploadLogChunk(taskID, phase, content, 0, "")
	return err
}

// ============================================================================
// Resource 相关
// ============================================================================

// GetResourceVersion 获取资源版本
func (a *RemoteDataAccessor) GetResourceVersion(versionID uint) (*models.ResourceCodeVersion, error) {
	// Resource versions are already loaded in GetWorkspaceResources
	return nil, fmt.Errorf("not implemented in remote mode")
}

// CountActiveResources 统计活跃资源数量
func (a *RemoteDataAccessor) CountActiveResources(workspaceID string) (int64, error) {
	resources, err := a.GetWorkspaceResources(workspaceID)
	if err != nil {
		return 0, err
	}
	return int64(len(resources)), nil
}

// GetWorkspaceResourcesWithVersions 获取 Workspace 资源列表（包含版本信息）
func (a *RemoteDataAccessor) GetWorkspaceResourcesWithVersions(workspaceID string) ([]models.WorkspaceResource, error) {
	// 复用现有的 GetWorkspaceResources 方法，它已经加载了版本信息
	return a.GetWorkspaceResources(workspaceID)
}

// GetResourceByVersionID 根据版本ID获取资源（Agent模式）
func (a *RemoteDataAccessor) GetResourceByVersionID(resourceID string, versionID uint) (*models.WorkspaceResource, error) {
	// 在Agent模式下，资源版本数据应该已经在taskData中
	// 从缓存的resources中查找
	resources, err := a.GetWorkspaceResources("")
	if err != nil {
		return nil, err
	}

	for _, resource := range resources {
		if resource.ResourceID == resourceID && resource.CurrentVersion != nil && resource.CurrentVersion.ID == versionID {
			return &resource, nil
		}
	}

	return nil, fmt.Errorf("resource %s version %d not found in cached data", resourceID, versionID)
}

// CheckResourceVersionExists 检查资源版本是否存在（Agent模式）
func (a *RemoteDataAccessor) CheckResourceVersionExists(resourceID string, versionID uint) (bool, error) {
	// 在Agent模式下，检查缓存的资源数据
	_, err := a.GetResourceByVersionID(resourceID, versionID)
	if err != nil {
		return false, nil // 不存在，但不返回错误
	}
	return true, nil
}

// ============================================================================
// Workspace Locking 相关
// ============================================================================

// LockWorkspace 锁定 Workspace
func (a *RemoteDataAccessor) LockWorkspace(workspaceID, userID, reason string) error {
	return a.apiClient.LockWorkspace(workspaceID, userID, reason)
}

// UnlockWorkspace 解锁 Workspace
func (a *RemoteDataAccessor) UnlockWorkspace(workspaceID string) error {
	return a.apiClient.UnlockWorkspace(workspaceID)
}

// UpdateWorkspaceFields 更新 Workspace 的指定字段
func (a *RemoteDataAccessor) UpdateWorkspaceFields(workspaceID string, updates map[string]interface{}) error {
	return a.apiClient.UpdateWorkspaceFields(workspaceID, updates)
}

// GetTerraformLockHCL 获取 Terraform Lock 文件内容
func (a *RemoteDataAccessor) GetTerraformLockHCL(workspaceID string) (string, error) {
	// 优先从缓存的 workspace 数据中获取
	if workspaceData, ok := a.taskData["workspace"].(map[string]interface{}); ok {
		if lockHCL, ok := workspaceData["terraform_lock_hcl"].(string); ok {
			return lockHCL, nil
		}
	}
	// 如果缓存中没有，通过 API 获取
	return a.apiClient.GetTerraformLockHCL(workspaceID)
}

// SaveTerraformLockHCL 保存 Terraform Lock 文件内容
func (a *RemoteDataAccessor) SaveTerraformLockHCL(workspaceID string, lockContent string) error {
	return a.apiClient.SaveTerraformLockHCL(workspaceID, lockContent)
}

// ============================================================================
// State 相关（扩展）
// ============================================================================

// GetMaxStateVersion 获取最大 State 版本号
func (a *RemoteDataAccessor) GetMaxStateVersion(workspaceID string) (int, error) {
	return a.apiClient.GetMaxStateVersion(workspaceID)
}

// ============================================================================
// Task 相关（扩展）
// ============================================================================

// GetPlanTask 获取 Plan 任务（带重试）
func (a *RemoteDataAccessor) GetPlanTask(taskID uint) (*models.WorkspaceTask, error) {
	// 使用带重试的方法获取 Plan 任务（这是最关键的 API）
	task, err := a.apiClient.GetPlanTaskWithRetry(taskID)
	if err != nil {
		return nil, err
	}

	// 【新增】将 snapshot_resources 和 snapshot_variables 缓存到 taskData 中，供后续查询使用
	if task.Context != nil {
		if snapshotResources, ok := task.Context["_snapshot_resources"].([]interface{}); ok {
			// 缓存 snapshot_resources 到 taskData
			a.taskData["snapshot_resources"] = snapshotResources
		}

		// 【修复】缓存 snapshot_variables（GetPlanTask API已将引用转换为完整数据）
		if snapshotVariables, ok := task.Context["_snapshot_variables"].([]interface{}); ok {
			a.taskData["snapshot_variables"] = snapshotVariables
		}
	}

	return task, nil
}

// GetTaskLogs 获取任务日志列表
func (a *RemoteDataAccessor) GetTaskLogs(taskID uint) ([]models.TaskLog, error) {
	return a.apiClient.GetTaskLogs(taskID)
}

// ============================================================================
// Plan Parsing 相关
// ============================================================================

// ParsePlanChanges 解析 Plan 变更
func (a *RemoteDataAccessor) ParsePlanChanges(taskID uint, planOutput string) error {
	// Agent 模式：在本地解析 plan_json，然后上传解析结果
	// 注意：这里我们不能直接调用 PlanParserService（循环依赖）
	// 所以暂时返回 nil，实际解析逻辑需要在 terraform_executor.go 中处理
	return a.apiClient.ParsePlanChanges(taskID, planOutput)
}

// ============================================================================
// Transaction 支持
// ============================================================================

// BeginTransaction 开始事务（Agent 模式不支持）
func (a *RemoteDataAccessor) BeginTransaction() (DataAccessor, error) {
	return nil, fmt.Errorf("transactions not supported in remote mode")
}

// Commit 提交事务（Agent 模式不支持）
func (a *RemoteDataAccessor) Commit() error {
	return fmt.Errorf("transactions not supported in remote mode")
}

// Rollback 回滚事务（Agent 模式不支持）
func (a *RemoteDataAccessor) Rollback() error {
	return fmt.Errorf("transactions not supported in remote mode")
}

// ============================================================================
// 辅助函数
// ============================================================================

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func getUint(m map[string]interface{}, key string) uint {
	if v, ok := m[key].(float64); ok {
		return uint(v)
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return make(map[string]interface{})
}

// UpdateResourceStatus 更新资源状态（Agent 模式）
func (a *RemoteDataAccessor) UpdateResourceStatus(taskID uint, resourceAddress, status, action string) error {
	// Agent 模式：通过 WebSocket 发送资源状态更新
	// 这会被 C&C 通道的 forwardLogsToServer 捕获并转发到服务器
	if a.streamManager != nil {
		stream := a.streamManager.GetOrCreate(taskID)
		if stream != nil {
			// 构造资源状态更新消息
			data := map[string]interface{}{
				"task_id":          taskID,
				"resource_address": resourceAddress,
				"apply_status":     status,
				"action":           action,
			}

			dataJSON, _ := json.Marshal(data)

			message := OutputMessage{
				Type:      "resource_status_update",
				Line:      string(dataJSON),
				Timestamp: time.Now(),
			}

			stream.Broadcast(message)
			return nil
		}
	}

	// 如果 streamManager 不可用，回退到 HTTP API
	return a.apiClient.UpdateResourceStatus(taskID, resourceAddress, status, action)
}

// GetResourceByVersion 根据版本号获取资源（Agent模式）
// resourceID: workspace_resources.resource_id (string, 如 "aws_s3_bucket.my_bucket")
// version: resource_code_versions.version (int, 版本号如 1, 2, 3...)
func (a *RemoteDataAccessor) GetResourceByVersion(resourceID string, version int) (*models.WorkspaceResource, error) {
	// 【修复】优先从 snapshot_resources 缓存中查找（GetPlanTask 返回的快照资源数据）
	if snapshotResources, ok := a.taskData["snapshot_resources"].([]interface{}); ok {
		for _, item := range snapshotResources {
			resMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			// 检查 resource_id 和 version 是否匹配
			if getString(resMap, "resource_id") == resourceID {
				if versionData, ok := resMap["current_version"].(map[string]interface{}); ok {
					if getInt(versionData, "version") == version {
						// 找到匹配的资源，构造返回对象
						resource := &models.WorkspaceResource{
							ID:          getUint(resMap, "id"),
							WorkspaceID: getString(resMap, "workspace_id"),
							ResourceID:  getString(resMap, "resource_id"),
							IsActive:    getBool(resMap, "is_active"),
						}

						codeVersion := &models.ResourceCodeVersion{
							ID:      getUint(versionData, "id"),
							Version: getInt(versionData, "version"),
							TFCode:  getMap(versionData, "tf_code"),
						}

						resource.CurrentVersion = codeVersion
						versionID := codeVersion.ID
						resource.CurrentVersionID = &versionID

						return resource, nil
					}
				}
			}
		}
	}

	// 如果 snapshot_resources 中没有找到，尝试从当前资源缓存中查找
	resources, err := a.GetWorkspaceResources("")
	if err == nil {
		for _, resource := range resources {
			if resource.ResourceID == resourceID && resource.CurrentVersion != nil && resource.CurrentVersion.Version == version {
				return &resource, nil
			}
		}
	}

	// 都没找到，返回错误
	return nil, fmt.Errorf("resource %s version %d not found in cached data", resourceID, version)
}

// CheckResourceVersionExistsByVersion 检查资源版本是否存在（按版本号，Agent模式）
// resourceID: workspace_resources.resource_id (string, 如 "aws_s3_bucket.my_bucket")
// version: resource_code_versions.version (int, 版本号如 1, 2, 3...)
func (a *RemoteDataAccessor) CheckResourceVersionExistsByVersion(resourceID string, version int) (bool, error) {
	// 在Agent模式下，检查缓存的资源数据
	_, err := a.GetResourceByVersion(resourceID, version)
	if err != nil {
		return false, nil // 不存在，但不返回错误
	}
	return true, nil
}

// GetRemoteDataConfig 获取 Workspace 的 remote data 配置（Agent模式）
// 返回服务端已经生成好token的remote data配置
func (a *RemoteDataAccessor) GetRemoteDataConfig() []map[string]interface{} {
	remoteDataData, ok := a.taskData["remote_data"].([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}

	result := make([]map[string]interface{}, 0, len(remoteDataData))
	for _, item := range remoteDataData {
		if rdMap, ok := item.(map[string]interface{}); ok {
			result = append(result, rdMap)
		}
	}

	return result
}

// GetModuleVersions 获取 module 版本映射（Agent模式）
// 返回 map[string]string，key 是 {provider}_{name}，value 是 version
func (a *RemoteDataAccessor) GetModuleVersions() map[string]string {
	moduleVersions, ok := a.taskData["module_versions"].(map[string]interface{})
	if !ok {
		return map[string]string{}
	}

	result := make(map[string]string)
	for key, value := range moduleVersions {
		if v, ok := value.(string); ok {
			result[key] = v
		}
	}

	return result
}

// GetWorkspaceOutputs 获取 Workspace 的 outputs 配置（Agent模式）
func (a *RemoteDataAccessor) GetWorkspaceOutputs(workspaceID string) ([]models.WorkspaceOutput, error) {
	outputsData, ok := a.taskData["outputs"].([]interface{})
	if !ok {
		return []models.WorkspaceOutput{}, nil
	}

	outputs := make([]models.WorkspaceOutput, 0, len(outputsData))
	for _, item := range outputsData {
		outMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		output := models.WorkspaceOutput{
			ID:           getUint(outMap, "id"),
			WorkspaceID:  getString(outMap, "workspace_id"),
			OutputName:   getString(outMap, "output_name"),
			OutputValue:  getString(outMap, "output_value"),
			ResourceName: getString(outMap, "resource_name"),
			Description:  getString(outMap, "description"),
			Sensitive:    getBool(outMap, "sensitive"),
		}

		outputs = append(outputs, output)
	}

	return outputs, nil
}
