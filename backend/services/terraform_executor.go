package services

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// TerraformExecutor Terraform执行器
type TerraformExecutor struct {
	db                  *gorm.DB     // 保留用于向后兼容
	dataAccessor        DataAccessor // 新增：数据访问接口
	streamManager       *OutputStreamManager
	signalManager       *SignalManager
	downloader          *TerraformDownloader // Terraform二进制下载器
	cachedBinaryPath    string               // 缓存的二进制文件路径
	cachedBinaryVersion string               // 缓存的版本号
	runTaskExecutor     *RunTaskExecutor     // Run Task 执行器
	notificationSender  *NotificationSender  // 通知发送器
}

// NewTerraformExecutor 创建Terraform执行器（向后兼容）
func NewTerraformExecutor(db *gorm.DB, streamManager *OutputStreamManager) *TerraformExecutor {
	// 从平台配置获取 baseURL
	platformConfigService := NewPlatformConfigService(db)
	baseURL := platformConfigService.GetBaseURL()

	return &TerraformExecutor{
		db:                 db,
		dataAccessor:       NewLocalDataAccessor(db), // 使用 LocalDataAccessor
		streamManager:      streamManager,
		signalManager:      GetSignalManager(),
		downloader:         NewTerraformDownloader(db), // 初始化下载器
		runTaskExecutor:    nil,                        // 延迟初始化，需要 baseURL
		notificationSender: NewNotificationSender(db, baseURL),
	}
}

// SetRunTaskExecutor 设置 Run Task 执行器
func (s *TerraformExecutor) SetRunTaskExecutor(executor *RunTaskExecutor) {
	s.runTaskExecutor = executor
}

// executeRunTasksForStage 执行指定阶段的 Run Tasks
// 返回 true 表示所有任务通过（或没有配置任务），false 表示有 mandatory 任务失败
func (s *TerraformExecutor) executeRunTasksForStage(
	ctx context.Context,
	task *models.WorkspaceTask,
	stage models.RunTaskStage,
	logger *TerraformLogger,
) (bool, error) {
	// 如果没有配置 Run Task 执行器，跳过
	if s.runTaskExecutor == nil {
		logger.Debug("Run Task executor not configured, skipping %s stage", stage)
		return true, nil
	}

	// 只在 Local 模式下执行 Run Tasks（Agent 模式由服务端处理）
	if s.db == nil {
		logger.Debug("Skipping Run Tasks in Agent mode for %s stage", stage)
		return true, nil
	}

	logger.Info("Executing Run Tasks for stage: %s", stage)

	passed, err := s.runTaskExecutor.ExecuteRunTasksForStage(ctx, task, stage)
	if err != nil {
		logger.Error("Run Tasks execution failed for stage %s: %v", stage, err)
		return false, err
	}

	if !passed {
		logger.Warn("Run Tasks blocked execution at stage %s (mandatory task failed)", stage)
		return false, nil
	}

	logger.Info("✓ Run Tasks completed for stage: %s", stage)
	return true, nil
}

// NewTerraformExecutorWithAccessor 创建Terraform执行器（使用 DataAccessor）
func NewTerraformExecutorWithAccessor(accessor DataAccessor, streamManager *OutputStreamManager) *TerraformExecutor {
	// Agent 模式下也需要下载器，但需要特殊的实现
	var downloader *TerraformDownloader
	if remoteAccessor, ok := accessor.(*RemoteDataAccessor); ok {
		// Agent 模式：创建使用 RemoteDataAccessor 的下载器
		downloader = NewTerraformDownloaderForAgent(remoteAccessor)
	}

	return &TerraformExecutor{
		db:            nil, // Agent 模式下不需要直接访问数据库
		dataAccessor:  accessor,
		streamManager: streamManager,
		signalManager: GetSignalManager(),
		downloader:    downloader, // Agent 模式下也使用下载器
	}
}

// ============================================================================
// Terraform Binary Management
// ============================================================================

// getTerraformBinary 获取Terraform二进制文件路径（带缓存）
func (s *TerraformExecutor) getTerraformBinary(version string) (string, error) {
	// 如果已缓存且版本匹配，直接返回
	if s.cachedBinaryPath != "" && s.cachedBinaryVersion == version {
		return s.cachedBinaryPath, nil
	}

	// 否则下载/验证并缓存
	if s.downloader == nil {
		return "", fmt.Errorf("terraform downloader not initialized")
	}

	binaryPath, err := s.downloader.EnsureTerraformBinary(version)
	if err != nil {
		return "", err
	}

	// 缓存结果
	s.cachedBinaryPath = binaryPath
	s.cachedBinaryVersion = version

	return binaryPath, nil
}

// ============================================================================
// 工作目录管理
// ============================================================================

// PrepareWorkspace 准备工作目录
func (s *TerraformExecutor) PrepareWorkspace(task *models.WorkspaceTask) (string, error) {
	workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%s/%d",
		task.WorkspaceID, task.ID)

	if err := os.MkdirAll(workDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create work directory: %w", err)
	}

	log.Printf("Created work directory: %s", workDir)
	return workDir, nil
}

// CleanupWorkspace 清理工作目录
func (s *TerraformExecutor) CleanupWorkspace(workDir string) error {
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return nil
	}

	if err := os.RemoveAll(workDir); err != nil {
		log.Printf("Warning: failed to cleanup workspace %s: %v", workDir, err)
		return err
	}

	log.Printf("Cleaned up workspace: %s", workDir)
	return nil
}

// ============================================================================
// 配置文件生成
// ============================================================================

// GenerateConfigFiles 生成所有配置文件
func (s *TerraformExecutor) GenerateConfigFiles(
	workspace *models.Workspace,
	workDir string,
) error {
	// 1. 生成 main.tf.json
	// 优先从资源聚合生成，如果没有资源则使用workspace.TFCode
	mainTF, err := s.generateMainTF(workspace)
	if err != nil {
		return fmt.Errorf("failed to generate main.tf: %w", err)
	}

	if err := s.writeJSONFile(workDir, "main.tf.json", mainTF); err != nil {
		return fmt.Errorf("failed to write main.tf.json: %w", err)
	}

	// 2. 生成 provider.tf.json
	// 清理空的terraform块，避免Terraform尝试读取不存在的backend state
	cleanedProviderConfig := s.cleanProviderConfig(workspace.ProviderConfig)
	if err := s.writeJSONFile(workDir, "provider.tf.json", cleanedProviderConfig); err != nil {
		return fmt.Errorf("failed to write provider.tf.json: %w", err)
	}

	// 3. 生成 variables.tf.json
	if err := s.generateVariablesTFJSON(workspace, workDir); err != nil {
		return fmt.Errorf("failed to write variables.tf.json: %w", err)
	}

	// 4. 生成 variables.tfvars
	if err := s.generateVariablesTFVars(workspace, workDir); err != nil {
		return fmt.Errorf("failed to write variables.tfvars: %w", err)
	}

	// 5. 生成 outputs.tf.json（如果有配置outputs）
	if err := s.generateOutputsTFJSON(workspace, workDir); err != nil {
		return fmt.Errorf("failed to write outputs.tf.json: %w", err)
	}

	// 6. 生成 remote_data.tf.json（如果有配置远程数据引用）
	if err := s.generateRemoteDataTFJSON(workspace, workDir, nil); err != nil {
		return fmt.Errorf("failed to write remote_data.tf.json: %w", err)
	}

	log.Printf("Generated all config files in %s", workDir)
	return nil
}

// generateVariablesTFJSON 生成variables.tf.json
func (s *TerraformExecutor) generateVariablesTFJSON(
	workspace *models.Workspace,
	workDir string,
) error {
	variables := make(map[string]interface{})

	// 使用 DataAccessor 获取变量定义
	workspaceVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
	if err != nil {
		log.Printf("Warning: failed to get variables: %v", err)
		// 如果获取变量失败，不生成 variables.tf.json 文件
		return nil
	}

	for _, v := range workspaceVars {
		varDef := map[string]interface{}{
			"type": "string", // 暂时简化，都使用string类型
		}

		if v.Description != "" {
			varDef["description"] = v.Description
		}

		if v.Sensitive {
			varDef["sensitive"] = true
		}

		variables[v.Key] = varDef
	}

	// 如果没有变量，不生成 variables.tf.json 文件
	if len(variables) == 0 {
		log.Printf("No terraform variables found, skipping variables.tf.json generation")
		return nil
	}

	config := map[string]interface{}{
		"variable": variables,
	}

	return s.writeJSONFile(workDir, "variables.tf.json", config)
}

// generateVariablesTFVars 生成variables.tfvars
func (s *TerraformExecutor) generateVariablesTFVars(
	workspace *models.Workspace,
	workDir string,
) error {
	var tfvars strings.Builder

	// 使用 DataAccessor 获取变量值
	workspaceVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
	if err != nil {
		log.Printf("Warning: failed to get variables: %v", err)
		return s.writeFile(workDir, "variables.tfvars", "")
	}

	for _, v := range workspaceVars {
		// 根据ValueFormat处理
		if v.ValueFormat == models.ValueFormatHCL {
			// HCL格式：需要判断是否为string类型
			// 如果值不是以 { [ 开头，且不是 true/false/数字，则认为是string，需要加引号
			trimmedValue := strings.TrimSpace(v.Value)
			needsQuotes := !strings.HasPrefix(trimmedValue, "{") &&
				!strings.HasPrefix(trimmedValue, "[") &&
				trimmedValue != "true" &&
				trimmedValue != "false" &&
				!isNumeric(trimmedValue)

			if needsQuotes {
				// String类型的HCL值需要加引号
				escapedValue := strings.ReplaceAll(v.Value, "\"", "\\\"")
				escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
				tfvars.WriteString(fmt.Sprintf("%s = \"%s\"\n", v.Key, escapedValue))
			} else {
				// 其他HCL格式直接使用
				tfvars.WriteString(fmt.Sprintf("%s = %s\n", v.Key, v.Value))
			}
		} else {
			// String格式需要加引号
			escapedValue := strings.ReplaceAll(v.Value, "\"", "\\\"")
			escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
			tfvars.WriteString(fmt.Sprintf("%s = \"%s\"\n", v.Key, escapedValue))
		}
	}

	return s.writeFile(workDir, "variables.tfvars", tfvars.String())
}

// isNumeric 检查字符串是否为数字
func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// generateOutputsTFJSON 生成outputs.tf.json
func (s *TerraformExecutor) generateOutputsTFJSON(
	workspace *models.Workspace,
	workDir string,
) error {
	var outputs []models.WorkspaceOutput
	var err error

	// 根据模式获取outputs配置
	if s.db != nil {
		// Local 模式：直接查询数据库
		if err := s.db.Where("workspace_id = ?", workspace.WorkspaceID).Find(&outputs).Error; err != nil {
			log.Printf("Warning: failed to get outputs: %v", err)
			return nil
		}
		log.Printf("[generateOutputsTFJSON] Local mode: found %d outputs for workspace %s", len(outputs), workspace.WorkspaceID)
	} else {
		// Agent 模式：使用 RemoteDataAccessor 从缓存获取
		if remoteAccessor, ok := s.dataAccessor.(*RemoteDataAccessor); ok {
			outputs, err = remoteAccessor.GetWorkspaceOutputs(workspace.WorkspaceID)
			if err != nil {
				log.Printf("Warning: failed to get outputs in Agent mode: %v", err)
				return nil
			}
			log.Printf("[generateOutputsTFJSON] Agent mode: found %d outputs for workspace %s", len(outputs), workspace.WorkspaceID)
		} else {
			log.Printf("Warning: dataAccessor is not RemoteDataAccessor in Agent mode")
			return nil
		}
	}

	// 如果没有outputs配置，不生成文件
	if len(outputs) == 0 {
		log.Printf("No outputs configured, skipping outputs.tf.json generation")
		return nil
	}

	// 获取活跃资源列表，用于过滤已删除资源的 outputs
	// 这是一个防御性措施，避免 "Reference to undeclared module" 错误
	activeResourceNames := make(map[string]bool)
	if s.db != nil {
		// Local 模式：查询活跃资源
		var activeResources []models.WorkspaceResource
		if err := s.db.Where("workspace_id = ? AND is_active = ?", workspace.WorkspaceID, true).
			Select("resource_name").Find(&activeResources).Error; err != nil {
			log.Printf("Warning: failed to get active resources for output filtering: %v", err)
		} else {
			for _, r := range activeResources {
				activeResourceNames[r.ResourceName] = true
			}
		}
	} else {
		// Agent 模式：从 DataAccessor 获取资源
		resources, err := s.dataAccessor.GetWorkspaceResources(workspace.WorkspaceID)
		if err != nil {
			log.Printf("Warning: failed to get resources for output filtering in Agent mode: %v", err)
		} else {
			for _, r := range resources {
				if r.IsActive {
					activeResourceNames[r.ResourceName] = true
				}
			}
		}
	}

	// 构建outputs配置
	// output key 格式: {module_name}-{output_name} 或 static-{output_name}
	outputsConfig := make(map[string]interface{})
	skippedCount := 0
	for _, output := range outputs {
		// 判断是否为静态输出
		isStaticOutput := output.IsStaticOutput()

		if !isStaticOutput {
			// 资源关联输出：过滤掉已删除资源的 outputs（防御性检查）
			if len(activeResourceNames) > 0 && !activeResourceNames[output.ResourceName] {
				log.Printf("Skipping output %s for deleted resource %s", output.OutputName, output.ResourceName)
				skippedCount++
				continue
			}
		}

		var outputDef map[string]interface{}
		var outputKey string

		if isStaticOutput {
			// 静态输出：直接使用值
			outputDef = map[string]interface{}{
				"value": output.OutputValue, // 直接使用静态值，不加 ${} 包装
			}
			// 静态输出的 key 格式: static-{output_name}
			outputKey = fmt.Sprintf("static-%s", output.OutputName)
			log.Printf("Generated static output key: %s (value=%s)", outputKey, output.OutputValue)
		} else {
			// 资源关联输出：使用表达式引用
			outputDef = map[string]interface{}{
				"value": fmt.Sprintf("${%s}", output.OutputValue),
			}

			// 从 output_value 中提取 module 名称
			// output_value 格式: module.{module_name}.{output_name}
			moduleName := output.ResourceName // 默认使用 resource_name
			if strings.HasPrefix(output.OutputValue, "module.") {
				parts := strings.Split(output.OutputValue, ".")
				if len(parts) >= 2 {
					moduleName = parts[1] // 提取 module 名称
					log.Printf("Extracted module name from output_value: %s -> %s", output.OutputValue, moduleName)
				}
			}

			// 使用 module_name-output_name 作为 key
			outputKey = fmt.Sprintf("%s-%s", moduleName, output.OutputName)
			log.Printf("Generated output key: %s (resource_name=%s, module_name=%s)", outputKey, output.ResourceName, moduleName)
		}

		if output.Description != "" {
			outputDef["description"] = output.Description
		}

		if output.Sensitive {
			outputDef["sensitive"] = true
		}

		outputsConfig[outputKey] = outputDef
	}

	// 如果所有 outputs 都被跳过（关联的资源都已删除），不生成文件
	if len(outputsConfig) == 0 {
		if skippedCount > 0 {
			log.Printf("All %d outputs were skipped (associated resources deleted), not generating outputs.tf.json", skippedCount)
		}
		return nil
	}

	config := map[string]interface{}{
		"output": outputsConfig,
	}

	if skippedCount > 0 {
		log.Printf("Generated outputs.tf.json with %d outputs (skipped %d orphaned outputs)", len(outputsConfig), skippedCount)
	} else {
		log.Printf("Generated outputs.tf.json with %d outputs", len(outputsConfig))
	}
	return s.writeJSONFile(workDir, "outputs.tf.json", config)
}

// PrepareStateFile 准备State文件
func (s *TerraformExecutor) PrepareStateFile(
	workspace *models.Workspace,
	workDir string,
) error {
	// 获取最新的State版本
	var stateVersion models.WorkspaceStateVersion
	err := s.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Order("version DESC").
		First(&stateVersion).Error

	if err == gorm.ErrRecordNotFound {
		log.Printf("No existing state for workspace %s", workspace.WorkspaceID)
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get state version: %w", err)
	}

	// 写入State文件
	stateFile := filepath.Join(workDir, "terraform.tfstate")
	stateContent, err := json.Marshal(stateVersion.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(stateFile, stateContent, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	log.Printf("Prepared state file: %s (version %d)", stateFile, stateVersion.Version)
	return nil
}

// ============================================================================
// Terraform命令执行
// ============================================================================

// TerraformInit 执行terraform init
func (s *TerraformExecutor) TerraformInit(
	ctx context.Context,
	workDir string,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
) error {
	// 获取Terraform二进制文件路径（已在Fetching阶段下载）
	// 必须使用下载的版本，不允许回退到系统terraform
	if s.downloader == nil {
		return fmt.Errorf("terraform downloader not initialized")
	}

	// 使用EnsureTerraformBinary确保二进制文件存在，并获取实际路径
	// 这样可以正确处理"latest"等特殊版本标识
	binaryPath, err := s.downloader.EnsureTerraformBinary(workspace.TerraformVersion)
	if err != nil {
		return fmt.Errorf("failed to ensure terraform binary for version %s: %w", workspace.TerraformVersion, err)
	}

	terraformCmd := binaryPath
	log.Printf("Using terraform binary: %s", terraformCmd)

	// 配置Provider插件缓存到工作目录（随工作目录一起清理）
	pluginCacheDir := filepath.Join(workDir, ".terraform-plugin-cache")
	if err := os.MkdirAll(pluginCacheDir, 0755); err != nil {
		log.Printf("Warning: failed to create plugin cache dir: %v", err)
		// 不阻塞执行，继续不使用缓存
		pluginCacheDir = ""
	}

	// 构建命令（必须包含-upgrade）
	args := []string{
		"init",
		"-no-color",
		"-input=false",
		"-upgrade", // 每次都升级Provider
	}

	cmd := exec.CommandContext(ctx, terraformCmd, args...)
	cmd.Dir = workDir

	// 设置环境变量
	cmd.Env = s.buildEnvironmentVariables(workspace)

	// 添加插件缓存目录（如果创建成功）
	if pluginCacheDir != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TF_PLUGIN_CACHE_DIR=%s", pluginCacheDir))
	}

	// 执行
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("Executing: terraform init -upgrade in %s", workDir)
	startTime := time.Now()

	if err := cmd.Run(); err != nil {
		s.saveTaskLog(task.ID, "init", stderr.String(), "error")
		return fmt.Errorf("terraform init failed: %w\n%s", err, stderr.String())
	}

	duration := time.Since(startTime)
	log.Printf("terraform init completed in %v", duration)

	s.saveTaskLog(task.ID, "init", stdout.String(), "info")
	return nil
}

// buildEnvironmentVariables 构建环境变量
func (s *TerraformExecutor) buildEnvironmentVariables(
	workspace *models.Workspace,
) []string {
	env := append(os.Environ(),
		"TF_IN_AUTOMATION=true",
		"TF_INPUT=false",
		// 设置 Registry 客户端超时为 60 秒（默认 10 秒对于慢速网络不够）
		// 这会影响 terraform init 时访问 registry.terraform.io 和 registry.opentofu.org 的超时时间
		"TF_REGISTRY_CLIENT_TIMEOUT=60",
	)

	// 从workspace_variables表读取环境变量（使用 DataAccessor）
	envVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeEnvironment)
	if err != nil {
		log.Printf("WARNING: Failed to get environment variables for workspace %s: %v", workspace.WorkspaceID, err)
	} else {
		log.Printf("DEBUG: Loaded %d environment variables for workspace %s", len(envVars), workspace.WorkspaceID)

		// 检查用户是否已设置AWS region变量
		hasAWSRegion := false
		for _, v := range envVars {
			if v.Key == "AWS_REGION" || v.Key == "AWS_DEFAULT_REGION" {
				hasAWSRegion = true
			}
		}

		// 注入环境变量
		for _, v := range envVars {
			// 跳过TF_CLI_ARGS，它会被特殊处理添加到命令参数中
			if v.Key == "TF_CLI_ARGS" {
				continue
			}
			env = append(env, fmt.Sprintf("%s=%s", v.Key, v.Value))
			// 绝对禁止打印变量值 - 只打印变量名
			log.Printf("DEBUG: Added environment variable: %s", v.Key)
		}

		// AWS Provider - 使用IAM Role（仅在用户未设置时）
		if !hasAWSRegion && workspace.ProviderConfig != nil {
			if awsConfig, ok := workspace.ProviderConfig["aws"].([]interface{}); ok && len(awsConfig) > 0 {
				aws := awsConfig[0].(map[string]interface{})

				// 设置region（必需）
				if region, ok := aws["region"].(string); ok {
					env = append(env, fmt.Sprintf("AWS_DEFAULT_REGION=%s", region))
					env = append(env, fmt.Sprintf("AWS_REGION=%s", region))
					log.Printf("DEBUG: Added AWS region from provider config: %s", region)
				}
			}
		}
	}

	return env
}

// getTFCLIArgs 获取TF_CLI_ARGS变量值并按空格分割为参数数组
func (s *TerraformExecutor) getTFCLIArgs(workspaceID string) []string {
	// 使用 DataAccessor 获取环境变量
	envVars, err := s.dataAccessor.GetWorkspaceVariables(workspaceID, models.VariableTypeEnvironment)
	if err != nil {
		return []string{}
	}

	// 查找 TF_CLI_ARGS
	for _, v := range envVars {
		if v.Key == "TF_CLI_ARGS" {
			// 如果值为空，返回空数组
			if strings.TrimSpace(v.Value) == "" {
				return []string{}
			}
			// 按空格分割参数
			return strings.Fields(v.Value)
		}
	}

	return []string{}
}

// ExecutePlan 执行Plan任务（流式输出版本 + 详细日志）
func (s *TerraformExecutor) ExecutePlan(
	ctx context.Context,
	task *models.WorkspaceTask,
) error {
	// Determine execution mode
	executionMode := "LOCAL"
	if s.db == nil {
		executionMode = "AGENT"
	}
	log.Printf("[%s MODE] ExecutePlan started for task %d, workspace %s", executionMode, task.ID, task.WorkspaceID)

	// 获取Workspace配置 - 使用 DataAccessor
	workspace, err := s.dataAccessor.GetWorkspace(task.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// 从workspace_variables表读取TF_LOG
	tfLogLevel := "info" // 默认
	if s.db != nil {
		var tfLogVar models.WorkspaceVariable
		if err := s.db.Where("workspace_id = ? AND key = ? AND variable_type = ?",
			workspace.WorkspaceID, "TF_LOG", models.VariableTypeEnvironment).First(&tfLogVar).Error; err == nil {
			tfLogLevel = tfLogVar.Value
		}
	}

	// 创建输出流和日志记录器
	stream := s.streamManager.GetOrCreate(task.ID)
	defer s.streamManager.Close(task.ID)

	// 检测是否为 Agent 模式
	isAgentMode := (s.db == nil)
	logger := NewTerraformLoggerWithLevelAndMode(stream, tfLogLevel, isAgentMode)

	// ========== 阶段1: Fetching ==========
	log.Printf("[DEBUG] Task %d: Starting Fetching stage", task.ID)
	logger.StageBegin("fetching")

	// 打印当前日志级别（用于调试）
	logger.Info("Current log level: %v (from workspace.system_variables)", logger.logLevel)

	// 1.0 确保IaC引擎二进制文件存在（第一步）
	if s.downloader != nil {
		// 检测引擎类型
		engineType := models.IaCEngineTerraform
		engineDisplayName := "Terraform"

		// 尝试从版本配置获取引擎类型
		if s.db != nil {
			var tfVersion models.TerraformVersion
			if err := s.db.Where("version = ?", workspace.TerraformVersion).First(&tfVersion).Error; err == nil {
				engineType = tfVersion.GetEngineType()
				engineDisplayName = engineType.GetDisplayName()
			}
		}

		logger.Info("Ensuring %s binary for version: %s", engineDisplayName, workspace.TerraformVersion)
		binaryPath, err := s.downloader.EnsureTerraformBinary(workspace.TerraformVersion)
		if err != nil {
			logger.Error("Failed to ensure %s binary: %v", engineDisplayName, err)
			logger.LogError("fetching", err, map[string]interface{}{
				"task_id":           task.ID,
				"workspace_id":      task.WorkspaceID,
				"terraform_version": workspace.TerraformVersion,
				"engine_type":       string(engineType),
			}, nil)
			logger.StageEnd("fetching")
			s.saveTaskFailure(task, logger, err, "plan")
			return fmt.Errorf("failed to ensure %s binary: %w", engineDisplayName, err)
		}
		logger.Info("✓ %s binary ready: %s", engineDisplayName, binaryPath)
	}

	// 1.1 准备工作目录
	logger.Info("Creating work directory for task #%d", task.ID)
	workDir, err := s.PrepareWorkspace(task)
	if err != nil {
		logger.LogError("fetching", err, map[string]interface{}{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
		}, nil)
		logger.StageEnd("fetching")
		return err
	}
	// DO NOT use defer for cleanup - it will delete the directory while terraform is still running
	// Cleanup will be done explicitly at the end of the function
	logger.Info("✓ Work directory created: %s", workDir)

	// 1.2 重新加载Workspace配置（确保最新） - 使用 DataAccessor
	logger.Info("Reloading workspace %s configuration...", task.WorkspaceID)

	workspaceReloaded, err := s.dataAccessor.GetWorkspace(task.WorkspaceID)
	if err != nil {
		logger.LogError("fetching", err, map[string]interface{}{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
		}, nil)
		logger.StageEnd("fetching")
		s.saveTaskFailure(task, logger, err, "plan")
		return fmt.Errorf("failed to get workspace: %w", err)
	}
	workspace = workspaceReloaded
	logger.Info("✓ Workspace configuration loaded")
	logger.Info("  - Name: %s", workspace.Name)
	logger.Info("  - Execution mode: %s", workspace.ExecutionMode)
	logger.Info("  - Terraform version: %s", workspace.TerraformVersion)

	// 1.3 获取资源（使用 DataAccessor）
	logger.Info("Fetching workspace resources...")
	resources, err := s.dataAccessor.GetWorkspaceResources(workspace.WorkspaceID)
	if err != nil {
		logger.Warn("Failed to fetch resources: %v", err)
	} else if len(resources) > 0 {
		logger.Info("Total: %d resources loaded", len(resources))
		for _, resource := range resources {
			if resource.CurrentVersion != nil {
				logger.Info("✓ Resource: %s (version: %d)",
					resource.ResourceID, resource.CurrentVersion.Version)
			}
		}
	}

	// 1.4 获取变量（使用 DataAccessor）
	logger.Info("Fetching workspace variables...")
	variables, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
	if err != nil {
		logger.Warn("Failed to fetch variables: %v", err)
	} else {
		normalCount := 0
		sensitiveCount := 0
		for _, v := range variables {
			if v.Sensitive {
				logger.Info("✓ Variable: %s = ***SENSITIVE***", v.Key)
				sensitiveCount++
			} else {
				logger.Info("✓ Variable: %s = %s", v.Key, v.Value)
				normalCount++
			}
		}
		logger.Info("Total: %d variables loaded (%d normal, %d sensitive)",
			len(variables), normalCount, sensitiveCount)
	}

	// 1.5 获取Provider配置
	logger.Info("Fetching provider configuration...")
	if workspace.ProviderConfig != nil {
		if awsConfig, ok := workspace.ProviderConfig["aws"].([]interface{}); ok && len(awsConfig) > 0 {
			aws := awsConfig[0].(map[string]interface{})
			if region, ok := aws["region"].(string); ok {
				logger.Info("✓ Provider: AWS (region: %s)", region)
			}
		}
	}

	// 1.6 获取State版本（使用 DataAccessor）
	logger.Info("Fetching latest state version...")
	stateVersion, err := s.dataAccessor.GetLatestStateVersion(workspace.WorkspaceID)
	if err != nil {
		logger.Warn("Failed to fetch state version: %v", err)
	} else if stateVersion != nil {
		logger.Info("✓ Found state version #%d", stateVersion.Version)
		logger.Info("  - Size: %.1f KB", float64(stateVersion.SizeBytes)/1024)
		// Safe checksum display - handle empty checksum
		if len(stateVersion.Checksum) >= 16 {
			logger.Info("  - Checksum: %s", stateVersion.Checksum[:16]+"...")
		} else if len(stateVersion.Checksum) > 0 {
			logger.Info("  - Checksum: %s", stateVersion.Checksum)
		} else {
			logger.Info("  - Checksum: (empty)")
		}
	} else {
		logger.Info("No existing state found (first run)")
	}

	// 1.7 生成配置文件
	logger.Info("Generating configuration files from resources...")
	if err := s.GenerateConfigFilesWithLogging(workspace, workDir, logger); err != nil {
		logger.LogError("fetching", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
			"work_dir":     workDir,
		}, nil)
		logger.StageEnd("fetching")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "plan")
		return err
	}

	// 1.8 准备State文件
	logger.Info("Preparing state file...")
	if err := s.PrepareStateFileWithLogging(workspace, workDir, logger); err != nil {
		logger.LogError("fetching", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
			"work_dir":     workDir,
		}, nil)
		logger.StageEnd("fetching")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "plan")
		return err
	}

	// 1.9 恢复 .terraform.lock.hcl 文件（加速 terraform init）
	logger.Info("Restoring terraform lock file...")
	s.restoreTerraformLockHCL(workDir, workspace.WorkspaceID, logger)

	logger.Info("Configuration fetch completed successfully")
	logger.StageEnd("fetching")

	// ========== 阶段2: Init ==========
	logger.StageBegin("init")

	if err := s.TerraformInitWithLogging(ctx, workDir, task, workspace, logger); err != nil {
		logger.LogError("init", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
			"work_dir":     workDir,
		}, nil)
		logger.StageEnd("init")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "plan")
		return err
	}

	logger.StageEnd("init")

	// ========== 阶段3: Planning ==========
	logger.StageBegin("planning")

	planFile := filepath.Join(workDir, "plan.out")
	args := []string{"plan", "-out=" + planFile, "-no-color", "-var-file=variables.tfvars"}

	// Drift Check 任务：添加 --refresh-only 参数
	if task.TaskType == models.TaskTypeDriftCheck {
		args = append(args, "-refresh-only")
		logger.Info("Drift check mode: adding -refresh-only flag")
	}

	// 添加TF_CLI_ARGS参数（如果有）
	tfCliArgs := s.getTFCLIArgs(workspace.WorkspaceID) // 保持使用内部数字ID

	// Drift Check 任务：移除 --target 参数（需要检查所有资源）
	if task.TaskType == models.TaskTypeDriftCheck {
		tfCliArgs = removeTargetArgs(tfCliArgs)
		logger.Debug("Drift check mode: removed --target args from TF_CLI_ARGS")
	}
	logger.Debug("TF_CLI_ARGS parameters: %v (count: %d)", tfCliArgs, len(tfCliArgs))
	if len(tfCliArgs) > 0 {
		args = append(args, tfCliArgs...)
		logger.Info("Adding TF_CLI_ARGS parameters: %v", tfCliArgs)
		logger.Debug("Plan args after adding TF_CLI_ARGS: %v", args)
	}

	// 添加target参数（如果有）
	if targets, ok := task.Context["targets"].([]string); ok && len(targets) > 0 {
		for _, target := range targets {
			args = append(args, "-target="+target)
			logger.Info("Adding target: %s", target)
		}
	}

	// 获取Terraform二进制文件路径（已在Fetching阶段下载）
	// 必须使用下载的版本，不允许回退到系统terraform
	if s.downloader == nil {
		err := fmt.Errorf("terraform downloader not initialized")
		logger.LogError("planning", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
		}, nil)
		logger.StageEnd("planning")
		s.saveTaskFailure(task, logger, err, "plan")
		return err
	}

	// 使用EnsureTerraformBinary获取实际路径（处理"latest"等特殊版本）
	binaryPath, err := s.downloader.EnsureTerraformBinary(workspace.TerraformVersion)
	if err != nil {
		logger.LogError("planning", err, map[string]interface{}{
			"workspace_id":      workspace.WorkspaceID,
			"terraform_version": workspace.TerraformVersion,
		}, nil)
		logger.StageEnd("planning")
		s.saveTaskFailure(task, logger, err, "plan")
		return fmt.Errorf("failed to ensure terraform binary for version %s: %w", workspace.TerraformVersion, err)
	}

	terraformCmd := binaryPath
	logger.Info("Using downloaded terraform binary: %s", terraformCmd)

	logger.Debug("Final plan command args: %v", args)
	logger.Info("Executing: %s plan with %d arguments", terraformCmd, len(args))

	cmd := exec.CommandContext(ctx, terraformCmd, args...)
	cmd.Dir = workDir
	cmd.Env = s.buildEnvironmentVariables(workspace)

	// 使用Pipe实时捕获输出
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logger.LogError("planning", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
		}, nil)
		logger.StageEnd("planning")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "plan")
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logger.LogError("planning", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
		}, nil)
		logger.StageEnd("planning")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "plan")
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 启动命令
	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		// 检查是否是context取消导致的
		if ctx.Err() == context.Canceled {
			logger.Info("Task cancelled by user during plan startup")
			s.saveTaskCancellation(task, logger, "plan")
			return fmt.Errorf("task cancelled by user")
		}
		logger.LogError("planning", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
		}, nil)
		logger.StageEnd("planning")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "plan")
		return fmt.Errorf("failed to start terraform: %w", err)
	}

	// 实时读取stdout
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logger.RawOutput(scanner.Text())
		}
	}()

	// 实时读取stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.RawOutput(scanner.Text())
		}
	}()

	// 等待命令完成
	cmdErr := cmd.Wait()
	wg.Wait()

	duration := time.Since(startTime)

	if cmdErr != nil {
		// 检查是否是context取消导致的
		if ctx.Err() == context.Canceled {
			logger.Info("Task cancelled by user during plan execution")
			s.saveTaskCancellation(task, logger, "plan")
			return fmt.Errorf("task cancelled by user")
		}
		logger.Error("Terraform plan failed: %v", cmdErr)
		logger.LogError("planning", cmdErr, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
			"duration":     duration.Seconds(),
		}, nil)
		logger.StageEnd("planning")

		// 保存失败信息（包含所有阶段的日志）
		s.saveTaskFailure(task, logger, cmdErr, "plan")
		return fmt.Errorf("terraform plan failed: %w", cmdErr)
	}

	logger.Info("✓ Plan completed successfully")
	logger.Info("Plan execution time: %.1f seconds", duration.Seconds())
	logger.StageEnd("planning")

	// ========== 阶段4: Saving Plan Data ==========
	logger.StageBegin("saving_plan")

	// 【Phase 1优化】计算plan文件的hash
	logger.Info("Calculating plan file hash for optimization...")
	planHash, err := s.calculatePlanHash(planFile)
	if err != nil {
		logger.Warn("Failed to calculate plan hash: %v", err)
	} else {
		task.PlanHash = planHash
		logger.Info("✓ Plan hash calculated: %s", planHash[:16]+"...")
	}

	// 生成Plan JSON
	logger.Info("Generating plan JSON for analysis...")
	planJSON, err := s.GeneratePlanJSON(ctx, workDir, planFile, workspace)
	if err != nil {
		logger.Warn("Failed to generate plan JSON: %v", err)
	} else {
		logger.Info("✓ Generated plan.json (%.1f KB)", float64(len(fmt.Sprintf("%v", planJSON)))/1024)
	}

	// 解析资源变更统计
	if planJSON != nil {
		add, change, destroy := s.parsePlanChanges(planJSON)
		task.ChangesAdd = add
		task.ChangesChange = change
		task.ChangesDestroy = destroy

		logger.Info("Plan Summary:")
		logger.Info("  - Resources to add: %d", add)
		logger.Info("  - Resources to change: %d", change)
		logger.Info("  - Resources to destroy: %d", destroy)
		logger.Info("  - Total changes: %d", add+change+destroy)
	}

	// 保存Plan数据到数据库
	logger.Info("Saving plan data to database...")
	log.Printf("[CRITICAL] About to call SavePlanDataWithLogging for task %d", task.ID)
	s.SavePlanDataWithLogging(task, planFile, planJSON, logger)
	log.Printf("[CRITICAL] SavePlanDataWithLogging completed for task %d", task.ID)

	// 【新增】异步解析并存储资源变更（用于Structured Run Output）
	// 在 Local 和 Agent 模式下都执行
	go func() {
		if s.db != nil {
			// Local 模式：直接调用 PlanParserService
			planParserService := NewPlanParserService(s.db)
			if err := planParserService.ParseAndStorePlanChanges(task.ID); err != nil {
				log.Printf("Warning: failed to parse plan changes for task %d: %v", task.ID, err)
			} else {
				log.Printf("Successfully parsed and stored resource changes for task %d", task.ID)
			}
		} else {
			// Agent 模式：在本地解析 plan_json，然后通过 API 上传解析结果
			if planJSON != nil {
				// 解析 resource_changes
				resourceChanges := s.parseResourceChangesFromPlanJSON(planJSON)

				// 通过 API 上传解析结果
				if err := s.uploadResourceChanges(task.ID, resourceChanges); err != nil {
					log.Printf("Warning: failed to upload resource changes in Agent mode for task %d: %v", task.ID, err)
				} else {
					log.Printf("Successfully uploaded %d resource changes in Agent mode for task %d", len(resourceChanges), task.ID)
				}
			} else {
				log.Printf("Warning: no plan_json available for task %d in Agent mode", task.ID)
			}
		}
	}()

	// 快照只在任务创建时创建一次，不在Plan完成后更新
	// 这样可以确保快照格式一致，避免被覆盖为旧格式
	logger.Debug("Snapshot was created at task creation time, no update needed")

	logger.StageEnd("saving_plan")

	// ========== 阶段4.5: Post-Plan Run Tasks ==========
	// 在 Plan 数据保存后执行 post_plan 阶段的 Run Tasks
	// Run Task 需要访问 plan_json 来分析变更，所以必须在保存后执行
	logger.StageBegin("post_plan_run_tasks")
	logger.Info("Executing post-plan Run Tasks...")

	runTasksPassed, runTasksErr := s.executeRunTasksForStage(ctx, task, models.RunTaskStagePostPlan, logger)
	if runTasksErr != nil {
		logger.Error("Post-plan Run Tasks execution error: %v", runTasksErr)
		logger.StageEnd("post_plan_run_tasks")
		s.saveTaskFailure(task, logger, runTasksErr, "plan")
		return fmt.Errorf("post-plan run tasks failed: %w", runTasksErr)
	}

	if !runTasksPassed {
		// Mandatory Run Task 失败，任务被阻止
		logger.Error("Post-plan Run Tasks blocked execution (mandatory task failed)")
		logger.StageEnd("post_plan_run_tasks")
		// 设置任务状态为失败
		task.Status = models.TaskStatusFailed
		task.ErrorMessage = "Post-plan Run Task failed (mandatory)"
		task.CompletedAt = timePtr(time.Now())
		s.dataAccessor.UpdateTask(task)
		return fmt.Errorf("post-plan run tasks blocked execution")
	}

	// 检查是否有 Advisory 失败（不阻止执行，但需要用户确认）
	// Advisory 失败时，任务仍然进入 apply_pending 状态，用户可以选择 Override
	logger.Info("✓ Post-plan Run Tasks completed (no mandatory failures)")
	logger.StageEnd("post_plan_run_tasks")

	// 发送完成消息
	stream.Broadcast(OutputMessage{
		Type:      "completed",
		Timestamp: time.Now(),
	})

	// 在所有阶段完成后获取完整输出（包含Fetching/Init/Planning/Saving Plan所有阶段）
	planOutput := logger.GetFullOutput()

	// 根据任务类型决定最终状态
	if task.TaskType == models.TaskTypePlanAndApply {
		// 检查是否有变更（资源变更或 output 变更）
		totalChanges := task.ChangesAdd + task.ChangesChange + task.ChangesDestroy

		// 检查是否有 output 变更
		hasOutputChanges := false
		if planJSON != nil {
			// 检查 output_changes 字段（Terraform 标准格式）
			// 注意：需要检查每个 output 的 actions，只有非 no-op 且值有实际变化才算有变更
			if outputChanges, ok := planJSON["output_changes"].(map[string]interface{}); ok && len(outputChanges) > 0 {
				actualOutputChanges := 0
				for outputName, change := range outputChanges {
					if changeMap, ok := change.(map[string]interface{}); ok {
						if actions, ok := changeMap["actions"].([]interface{}); ok {
							// 检查是否是 no-op
							isNoOp := len(actions) == 1 && actions[0] == "no-op"
							if isNoOp {
								logger.Debug("Output %s is no-op, skipping", outputName)
								continue
							}

							// 额外检查：即使 actions 不是 no-op，也要比较 before 和 after 的值
							// 因为 Terraform 有时会报告 "update" 即使值没有变化
							beforeVal := changeMap["before"]
							afterVal := changeMap["after"]

							// 使用 JSON 序列化比较值（处理复杂类型）
							beforeJSON, _ := json.Marshal(beforeVal)
							afterJSON, _ := json.Marshal(afterVal)

							if string(beforeJSON) == string(afterJSON) {
								logger.Debug("Output %s has same before/after value, skipping (actions=%v)", outputName, actions)
								continue
							}

							// 有实际变更
							actualOutputChanges++
							logger.Debug("Output %s has actual change: %v (before=%v, after=%v)", outputName, actions, beforeVal, afterVal)
						}
					}
				}
				if actualOutputChanges > 0 {
					hasOutputChanges = true
					logger.Info("Detected %d actual output changes (from output_changes)", actualOutputChanges)
				} else {
					logger.Debug("All %d outputs have no actual changes (no-op or same value)", len(outputChanges))
				}
			}
		}

		// 【防御】检查任务是否在 plan 输出解析期间被取消
		if ctx.Err() == context.Canceled {
			logger.Info("Task cancelled by user during plan output processing")
			s.saveTaskCancellation(task, logger, "plan")
			return fmt.Errorf("task cancelled by user")
		}

		if totalChanges == 0 && !hasOutputChanges {
			// 没有资源变更也没有 output 变更，直接完成任务，不需要Apply
			task.Status = models.TaskStatusPlannedAndFinished
			task.Stage = "planned_and_finished"
			log.Printf("Task %d (plan_and_apply) has no changes (resources: 0, outputs: 0), marked as planned_and_finished", task.ID)
			logger.Info("No changes detected (resources or outputs). Plan completed, apply will not run.")

			// 没有变更，清理工作目录
			logger.Info("Cleaning up work directory (no changes to apply): %s", workDir)
			if err := s.CleanupWorkspace(workDir); err != nil {
				logger.Warn("Failed to cleanup work directory: %v", err)
			} else {
				logger.Info("✓ Work directory cleaned up successfully")
			}
		} else {
			// 有变更，自动转换为apply_pending状态
			// 注意：这里直接设置为apply_pending，而不是plan_completed
			// TaskQueueManager会根据workspace的auto_apply设置决定是否自动执行
			task.Status = models.TaskStatusApplyPending
			task.Stage = "apply_pending"
			// 设置 PlanTaskID 指向自己（plan_and_apply 任务的 plan 数据在自己身上）
			task.PlanTaskID = &task.ID
			log.Printf("Task %d (plan_and_apply) plan completed, status changed to apply_pending, plan_task_id set to %d", task.ID, task.ID)
			logger.Info("Plan completed with changes, status changed to apply_pending")

			// 自动锁定workspace，防止在Plan-Apply期间修改配置
			logger.Info("Locking workspace to prevent configuration changes during plan-apply gap...")
			lockReason := fmt.Sprintf("Locked for apply (task #%d). Do not modify resources/variables until apply completes.", task.ID)
			if err := s.lockWorkspace(workspace.WorkspaceID, "system", lockReason); err != nil {
				logger.Warn("Failed to lock workspace: %v", err)
			} else {
				logger.Info("✓ Workspace locked successfully")
			}

			// 保留工作目录给Apply使用
			logger.Info("Preserving work directory for apply: %s", workDir)
			log.Printf("Task %d: Work directory preserved at %s (plan_hash: %s)", task.ID, workDir, task.PlanHash[:16]+"...")
		}
	} else {
		// 单独的Plan任务：直接完成并清理工作目录
		task.Status = models.TaskStatusSuccess
		task.Stage = "completed"
		log.Printf("Task %d (plan) completed successfully", task.ID)

		// 清理工作目录（plan-only任务不需要保留）
		logger.Info("Cleaning up work directory for plan-only task: %s", workDir)
		if err := s.CleanupWorkspace(workDir); err != nil {
			logger.Warn("Failed to cleanup work directory: %v", err)
		} else {
			logger.Info("✓ Work directory cleaned up successfully")
		}
	}

	// 更新任务状态（包括完整的日志输出）
	task.PlanOutput = planOutput
	task.CompletedAt = timePtr(time.Now())
	task.Duration = int(duration.Seconds())

	// 使用 DataAccessor 更新任务
	// 注意：不要覆盖 plan_data 和 plan_json（它们已经在 SavePlanDataWithLogging 中保存）
	if s.db != nil {
		// Local 模式：使用 Updates 只更新指定字段，避免覆盖 plan_data/plan_json
		updates := map[string]interface{}{
			"status":          task.Status,
			"stage":           task.Stage,
			"plan_output":     task.PlanOutput,
			"completed_at":    task.CompletedAt,
			"duration":        task.Duration,
			"changes_add":     task.ChangesAdd,
			"changes_change":  task.ChangesChange,
			"changes_destroy": task.ChangesDestroy,
			"plan_hash":       task.PlanHash, // 【Phase 1优化】保存plan hash
		}
		// 如果设置了 PlanTaskID，也要更新（plan_and_apply 任务需要）
		if task.PlanTaskID != nil {
			updates["plan_task_id"] = task.PlanTaskID
		}
		if err := s.db.Model(&models.WorkspaceTask{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}
	} else {
		// Agent 模式：使用 DataAccessor
		if err := s.dataAccessor.UpdateTask(task); err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}
	}

	// 同时保存到task_logs表
	s.saveTaskLog(task.ID, "plan", planOutput, "info")

	log.Printf("Task %d plan output saved (%d bytes)", task.ID, len(planOutput))

	// 【Phase 1优化】Plan完成后不清理工作目录，保留给Apply使用
	logger.Info("Preserving work directory for potential apply: %s", workDir)
	log.Printf("Task %d: Work directory preserved at %s (plan_hash: %s)", task.ID, workDir, task.PlanHash[:16]+"...")

	// 发送 Plan 完成通知（仅在 Local 模式下）
	if s.notificationSender != nil && s.db != nil {
		go func() {
			ctx := context.Background()
			// 根据任务状态决定发送哪种通知
			if task.Status == models.TaskStatusApplyPending {
				// Plan 完成，等待 Apply 确认 - 发送 approval_required 通知
				log.Printf("[Notification] Triggering approval_required notification for task %d", task.ID)
				if err := s.notificationSender.TriggerNotifications(
					ctx,
					task.WorkspaceID,
					models.NotificationEventApprovalRequired,
					task,
				); err != nil {
					log.Printf("[Notification] Failed to send approval_required notification for task %d: %v", task.ID, err)
				}
			} else {
				// Plan 完成（无变更或 plan-only 任务）- 发送 task_planned 通知
				log.Printf("[Notification] Triggering task_planned notification for task %d", task.ID)
				if err := s.notificationSender.TriggerNotifications(
					ctx,
					task.WorkspaceID,
					models.NotificationEventTaskPlanned,
					task,
				); err != nil {
					log.Printf("[Notification] Failed to send task_planned notification for task %d: %v", task.ID, err)
				}
			}
		}()
	}

	return nil
}

// GeneratePlanJSON 生成Plan JSON格式
func (s *TerraformExecutor) GeneratePlanJSON(
	ctx context.Context,
	workDir string,
	planFile string,
	workspace *models.Workspace,
) (map[string]interface{}, error) {
	// 使用下载的terraform二进制文件（确保版本一致）
	terraformCmd := "terraform"

	// 优先使用缓存的二进制文件路径（Plan阶段已经下载并缓存）
	if workspace != nil {
		// 使用带缓存的 getTerraformBinary 方法
		binaryPath, err := s.getTerraformBinary(workspace.TerraformVersion)
		if err != nil {
			log.Printf("Warning: failed to get terraform binary path, using 'terraform' command: %v", err)
		} else {
			terraformCmd = binaryPath
			log.Printf("Using terraform binary for show command: %s", terraformCmd)
		}
	}

	cmd := exec.CommandContext(ctx, terraformCmd, "show", "-json", planFile)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("terraform show -json failed: %w", err)
	}

	var planJSON map[string]interface{}
	if err := json.Unmarshal(output, &planJSON); err != nil {
		return nil, fmt.Errorf("failed to parse plan JSON: %w", err)
	}

	return planJSON, nil
}

// SavePlanData 保存Plan数据（带重试，不阻塞）
func (s *TerraformExecutor) SavePlanData(
	task *models.WorkspaceTask,
	planFile string,
	planJSON map[string]interface{},
) {
	planData, err := os.ReadFile(planFile)
	if err != nil {
		log.Printf("ERROR: Failed to read plan file: %v", err)
		return
	}

	// 带简单重试
	maxRetries := 3
	var saveErr error

	for i := 0; i < maxRetries; i++ {
		task.PlanData = planData
		task.PlanJSON = planJSON

		saveErr = s.db.Save(task).Error
		if saveErr == nil {
			log.Printf("Plan data saved successfully")
			return
		}

		log.Printf("Failed to save plan data (attempt %d/%d): %v", i+1, maxRetries, saveErr)

		if i < maxRetries-1 {
			time.Sleep(time.Second)
		}
	}

	// 保存失败 - 告警但不阻塞
	log.Printf("WARNING: Failed to save plan data after %d retries", maxRetries)
}

// ============================================================================
// 流式输出管理
// ============================================================================

// broadcastStageMarker 广播阶段标记
func (s *TerraformExecutor) broadcastStageMarker(
	stream *OutputStream,
	stage string,
	status string, // "begin" or "end"
	fullOutput *strings.Builder,
	mutex *sync.Mutex,
) {
	timestamp := time.Now()
	marker := fmt.Sprintf("========== %s %s at %s ==========",
		strings.ToUpper(stage),
		strings.ToUpper(status),
		timestamp.Format("2006-01-02 15:04:05.000"))

	// 创建阶段标记消息
	msg := OutputMessage{
		Type:      "stage_marker",
		Line:      marker,
		Timestamp: timestamp,
		Stage:     stage,
		Status:    status,
	}

	// 广播到所有客户端
	stream.Broadcast(msg)

	// 保存到完整输出
	mutex.Lock()
	fullOutput.WriteString(marker)
	fullOutput.WriteString("\n")
	mutex.Unlock()
}

// streamOutput 实时流式读取输出
func (s *TerraformExecutor) streamOutput(
	pipe io.ReadCloser,
	stream *OutputStream,
	fullOutput *strings.Builder,
	mutex *sync.Mutex,
	lineNum *int,
	source string,
) {
	scanner := bufio.NewScanner(pipe)

	for scanner.Scan() {
		line := scanner.Text()

		mutex.Lock()
		*lineNum++
		currentLineNum := *lineNum
		mutex.Unlock()

		// 创建消息
		msg := OutputMessage{
			Type:      "output",
			Line:      line,
			Timestamp: time.Now(),
			LineNum:   currentLineNum,
		}

		// 广播到所有WebSocket客户端
		stream.Broadcast(msg)

		// 保存到完整输出
		mutex.Lock()
		fullOutput.WriteString(line)
		fullOutput.WriteString("\n")
		mutex.Unlock()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading %s: %v", source, err)

		// 发送错误消息
		stream.Broadcast(OutputMessage{
			Type:      "error",
			Line:      fmt.Sprintf("Error reading %s: %v", source, err),
			Timestamp: time.Now(),
		})
	}
}

// ============================================================================
// 日志管理
// ============================================================================

// saveTaskLog 保存任务日志
func (s *TerraformExecutor) saveTaskLog(
	taskID uint,
	phase string,
	content string,
	level string,
) error {
	// Agent 模式下使用 DataAccessor
	if s.db == nil {
		return s.dataAccessor.SaveTaskLog(taskID, phase, content, level)
	}

	log := &models.TaskLog{
		TaskID:  taskID,
		Phase:   phase,
		Content: content,
		Level:   level,
	}

	return s.db.Create(log).Error
}

// saveTaskCancellation 保存任务取消信息（不显示为错误）
func (s *TerraformExecutor) saveTaskCancellation(
	task *models.WorkspaceTask,
	logger *TerraformLogger,
	taskType string, // "plan" or "apply"
) {
	fullOutput := logger.GetFullOutput()

	// 更新task字段 - 必须同时更新status和completed_at
	task.Status = models.TaskStatusCancelled
	task.CompletedAt = timePtr(time.Now())
	task.ErrorMessage = "Task cancelled by user"

	if taskType == "plan" {
		task.PlanOutput = fullOutput
	} else {
		task.ApplyOutput = fullOutput
	}

	// 使用 DataAccessor 更新任务
	if err := s.dataAccessor.UpdateTask(task); err != nil {
		log.Printf("[ERROR] Failed to update cancelled task: %v", err)
	}

	// 同时保存到task_logs表（使用info级别，不是error）
	s.saveTaskLog(task.ID, taskType, fullOutput, "info")

	log.Printf("Task %d cancelled, status updated to cancelled, output saved (%d bytes)", task.ID, len(fullOutput))
}

// saveTaskFailure 保存任务失败信息（包括完整日志）
func (s *TerraformExecutor) saveTaskFailure(
	task *models.WorkspaceTask,
	logger *TerraformLogger,
	err error,
	taskType string, // "plan" or "apply"
) {
	fullOutput := logger.GetFullOutput()

	// 从完整输出中提取真实的错误信息
	errorMessage := s.extractRealError(fullOutput, err)

	log.Printf("[DEBUG] saveTaskFailure: extracted error length: %d", len(errorMessage))
	log.Printf("[DEBUG] saveTaskFailure: error preview: %s", errorMessage[:min(100, len(errorMessage))])

	// 更新task字段
	task.Status = models.TaskStatusFailed
	task.ErrorMessage = errorMessage // 使用提取的真实错误
	task.CompletedAt = timePtr(time.Now())

	log.Printf("[DEBUG] saveTaskFailure: task.ErrorMessage set to: %s", task.ErrorMessage[:min(100, len(task.ErrorMessage))])

	if taskType == "plan" {
		task.PlanOutput = fullOutput

		// Plan失败时，清理工作目录
		workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%s/%d", task.WorkspaceID, task.ID)
		logger.Info("Cleaning up work directory after plan failure: %s", workDir)
		if cleanupErr := s.CleanupWorkspace(workDir); cleanupErr != nil {
			logger.Warn("Failed to cleanup work directory: %v", cleanupErr)
		} else {
			logger.Info("✓ Work directory cleaned up")
		}
	} else {
		task.ApplyOutput = fullOutput

		// Apply失败时，解锁workspace（如果之前被锁定）
		logger.Info("Unlocking workspace after apply failure...")
		if unlockErr := s.dataAccessor.UnlockWorkspace(task.WorkspaceID); unlockErr != nil {
			logger.Warn("Failed to unlock workspace: %v", unlockErr)
		} else {
			logger.Info("✓ Workspace unlocked")
		}

		// Apply失败时，尝试保存 partial state（terraform 可能已创建部分资源）
		workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%s/%d", task.WorkspaceID, task.ID)
		if s.db != nil {
			stateFile := filepath.Join(workDir, "terraform.tfstate")
			if _, statErr := os.Stat(stateFile); statErr == nil {
				logger.Info("Attempting to save partial state after apply failure...")
				var workspace models.Workspace
				if dbErr := s.db.Where("workspace_id = ?", task.WorkspaceID).First(&workspace).Error; dbErr == nil {
					if saveErr := s.SaveNewStateVersion(&workspace, task, workDir); saveErr != nil {
						logger.Warn("Failed to save partial state: %v", saveErr)
					} else {
						logger.Info("✓ Partial state saved to database")
					}
				}
			}
		}

		// 清理工作目录
		logger.Info("Cleaning up work directory after apply failure: %s", workDir)
		if cleanupErr := s.CleanupWorkspace(workDir); cleanupErr != nil {
			logger.Warn("Failed to cleanup work directory: %v", cleanupErr)
		} else {
			logger.Info("✓ Work directory cleaned up")
		}

		// 注意：CMDB 同步由 TaskQueueManager.syncCMDBAfterApply 统一处理
	}

	// 使用 DataAccessor 更新任务
	if err := s.dataAccessor.UpdateTask(task); err != nil {
		log.Printf("[ERROR] Failed to update task: %v", err)
	}

	// 同时保存到task_logs表
	s.saveTaskLog(task.ID, taskType, fullOutput, "error")

	log.Printf("Task %d failed, full output saved (%d bytes)", task.ID, len(fullOutput))

	// 发送任务失败通知（仅在 Local 模式下）
	log.Printf("[Notification] saveTaskFailure: notificationSender=%v, db=%v", s.notificationSender != nil, s.db != nil)
	if s.notificationSender != nil && s.db != nil {
		log.Printf("[Notification] Triggering task_failed notification for task %d", task.ID)
		go func() {
			ctx := context.Background()
			if err := s.notificationSender.TriggerNotifications(
				ctx,
				task.WorkspaceID,
				models.NotificationEventTaskFailed,
				task,
			); err != nil {
				log.Printf("[Notification] Failed to send task_failed notification for task %d: %v", task.ID, err)
			} else {
				log.Printf("[Notification] Successfully triggered task_failed notification for task %d", task.ID)
			}
		}()
	} else {
		log.Printf("[Notification] Skipping task_failed notification: notificationSender=%v, db=%v", s.notificationSender != nil, s.db != nil)
	}
}

// extractRealError 从完整输出中提取真实的错误信息
func (s *TerraformExecutor) extractRealError(fullOutput string, cmdErr error) string {
	log.Printf("[DEBUG] extractRealError called, output length: %d", len(fullOutput))

	lines := strings.Split(fullOutput, "\n")
	log.Printf("[DEBUG] Total lines: %d", len(lines))

	// 查找Error:开头的行
	var errorLines []string
	inErrorBlock := false
	errorBlockCount := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检测错误块开始（Error:开头，忽略日志级别的ERROR）
		if strings.HasPrefix(trimmed, "Error:") || strings.HasPrefix(trimmed, "│ Error:") {
			inErrorBlock = true
			errorBlockCount++
			log.Printf("[DEBUG] Found Error: at line %d: %s", i, trimmed)
			// 移除前缀符号
			cleanLine := strings.TrimPrefix(trimmed, "│ ")
			errorLines = append(errorLines, cleanLine)
			continue
		}

		// 如果在错误块中
		if inErrorBlock {
			// 遇到日志行（[时间戳]开头）表示错误块结束
			if strings.HasPrefix(trimmed, "[") {
				log.Printf("[DEBUG] Error block ended at line %d (log line detected)", i)
				inErrorBlock = false
				continue
			}

			// 空行保留（错误信息中的空行）
			if trimmed == "" {
				errorLines = append(errorLines, "")
				continue
			}

			// 继续收集错误行
			cleanLine := strings.TrimPrefix(trimmed, "│ ")
			errorLines = append(errorLines, cleanLine)
		}
	}

	log.Printf("[DEBUG] Found %d error blocks, collected %d error lines", errorBlockCount, len(errorLines))

	// 如果找到了错误信息，返回提取的内容
	if len(errorLines) > 0 {
		// 移除末尾的空行
		for len(errorLines) > 0 && errorLines[len(errorLines)-1] == "" {
			errorLines = errorLines[:len(errorLines)-1]
		}
		extracted := strings.Join(errorLines, "\n")
		log.Printf("[DEBUG] Extracted error message (%d chars): %s", len(extracted), extracted[:min(200, len(extracted))])
		return extracted
	}

	// 如果没有找到Error:行，返回命令错误
	log.Printf("[DEBUG] No Error: blocks found, returning command error: %s", cmdErr.Error())
	return cmdErr.Error()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================================
// 辅助函数
// ============================================================================

// writeJSONFile 写入JSON文件
func (s *TerraformExecutor) writeJSONFile(workDir, filename string, data interface{}) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(workDir, filename)
	return os.WriteFile(filePath, content, 0644)
}

// writeFile 写入文件
func (s *TerraformExecutor) writeFile(workDir, filename, content string) error {
	filePath := filepath.Join(workDir, filename)
	return os.WriteFile(filePath, []byte(content), 0644)
}

// calculateChecksum 计算校验和
func (s *TerraformExecutor) calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ExecuteApply 执行Apply任务（流式输出版本 + 详细日志）
func (s *TerraformExecutor) ExecuteApply(
	ctx context.Context,
	task *models.WorkspaceTask,
) error {
	// Determine execution mode
	executionMode := "LOCAL"
	if s.db == nil {
		executionMode = "AGENT"
	}
	log.Printf("[%s MODE] ExecuteApply started for task %d, workspace %s", executionMode, task.ID, task.WorkspaceID)

	// 创建输出流和日志记录器
	stream := s.streamManager.GetOrCreate(task.ID)
	defer s.streamManager.Close(task.ID)

	// 检测是否为 Agent 模式
	isAgentMode := (s.db == nil)
	logger := NewTerraformLoggerWithLevelAndMode(stream, "info", isAgentMode)

	// ========== 阶段1: Fetching ==========
	logger.StageBegin("fetching")

	// 1.1 准备工作目录
	// 【Phase 1优化】对于 plan_and_apply 任务，检查是否可以复用 Plan 阶段的工作目录
	workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%s/%d", task.WorkspaceID, task.ID)
	workDirExists := false

	// 检查工作目录是否已存在（Plan 阶段保留的）
	if _, err := os.Stat(workDir); err == nil {
		workDirExists = true
		logger.Info("Work directory already exists (preserved from plan): %s", workDir)
	} else {
		logger.Info("Creating work directory for task #%d", task.ID)
		workDir, err = s.PrepareWorkspace(task)
		if err != nil {
			logger.LogError("fetching", err, map[string]interface{}{
				"task_id":      task.ID,
				"workspace_id": task.WorkspaceID,
			}, nil)
			logger.StageEnd("fetching")
			return err
		}
		logger.Info("✓ Work directory created: %s", workDir)
	}
	// DO NOT use defer for cleanup - it will delete the directory while terraform is still running
	// Cleanup will be done explicitly at the end of the function

	// 1.2 获取Plan任务和快照数据
	logger.Info("Loading plan task and snapshot data...")

	// Check and fix plan_task_id for plan_and_apply tasks
	if task.PlanTaskID == nil {
		// For plan_and_apply tasks, plan_task_id should point to itself
		if task.TaskType == models.TaskTypePlanAndApply {
			logger.Warn("plan_task_id is NULL for plan_and_apply task %d, auto-fixing to self-reference", task.ID)
			task.PlanTaskID = &task.ID

			// Update in database
			if err := s.dataAccessor.UpdateTask(task); err != nil {
				logger.Warn("Failed to update plan_task_id in database: %v (continuing with in-memory fix)", err)
			} else {
				logger.Info("✓ Successfully updated plan_task_id to %d in database", task.ID)
			}
		} else {
			// For standalone apply tasks, this is an error
			err := fmt.Errorf("apply task has no associated plan task")
			logger.LogError("fetching", err, map[string]interface{}{
				"task_id":      task.ID,
				"workspace_id": task.WorkspaceID,
				"task_type":    task.TaskType,
			}, nil)
			logger.StageEnd("fetching")
			s.saveTaskFailure(task, logger, err, "apply")
			return err
		}
	}

	planTask, err := s.dataAccessor.GetPlanTask(*task.PlanTaskID)
	if err != nil {
		logger.LogError("fetching", err, map[string]interface{}{
			"task_id":      task.ID,
			"plan_task_id": *task.PlanTaskID,
		}, nil)
		logger.StageEnd("fetching")
		s.saveTaskFailure(task, logger, err, "apply")
		return fmt.Errorf("failed to get plan task: %w", err)
	}

	logger.Info("✓ Found plan task #%d (created %s)",
		planTask.ID, planTask.CreatedAt.Format("2006-01-02 15:04:05"))

	// 1.3 验证快照数据
	logger.Info("Validating snapshot data...")
	if err := s.ValidateResourceVersionSnapshot(planTask, logger); err != nil {
		logger.LogError("fetching", err, map[string]interface{}{
			"task_id":      task.ID,
			"plan_task_id": planTask.ID,
		}, nil)
		logger.StageEnd("fetching")
		s.saveTaskFailure(task, logger, err, "apply")
		return fmt.Errorf("snapshot validation failed: %w", err)
	}
	logger.Info("✓ Snapshot validation passed")

	// 1.4 从快照重建Workspace配置
	logger.Info("Reconstructing workspace configuration from snapshot...")
	workspace := &models.Workspace{
		WorkspaceID:    planTask.WorkspaceID,
		ProviderConfig: planTask.SnapshotProviderConfig,
	}

	// 获取workspace的基本信息（名称、执行模式等，这些不影响terraform执行）
	workspaceInfo, err := s.dataAccessor.GetWorkspace(task.WorkspaceID)
	if err != nil {
		logger.Warn("Failed to get workspace info: %v (continuing with snapshot data)", err)
	} else {
		workspace.Name = workspaceInfo.Name
		workspace.ExecutionMode = workspaceInfo.ExecutionMode
		workspace.TerraformVersion = workspaceInfo.TerraformVersion
		workspace.SystemVariables = workspaceInfo.SystemVariables
	}

	logger.Info("✓ Workspace configuration reconstructed from snapshot")
	logger.Info("  - Name: %s", workspace.Name)
	logger.Info("  - Execution mode: %s", workspace.ExecutionMode)
	logger.Info("  - Using snapshot from: %s", planTask.SnapshotCreatedAt.Format("2006-01-02 15:04:05"))

	// 1.4.1 确保Terraform二进制文件存在（在获取workspace信息后）
	if s.downloader != nil && workspace.TerraformVersion != "" {
		logger.Info("Ensuring Terraform binary for version: %s", workspace.TerraformVersion)
		binaryPath, err := s.downloader.EnsureTerraformBinary(workspace.TerraformVersion)
		if err != nil {
			logger.Error("Failed to ensure terraform binary: %v", err)
			logger.LogError("fetching", err, map[string]interface{}{
				"task_id":           task.ID,
				"workspace_id":      task.WorkspaceID,
				"terraform_version": workspace.TerraformVersion,
			}, nil)
			logger.StageEnd("fetching")
			s.saveTaskFailure(task, logger, err, "apply")
			return fmt.Errorf("failed to ensure terraform binary: %w", err)
		}
		logger.Info("✓ Terraform binary ready: %s", binaryPath)
	}

	// 【Phase 1优化】如果工作目录已存在（Plan 阶段保留的），跳过配置文件生成
	if workDirExists {
		logger.Info("Skipping configuration file generation (using preserved files from plan)")
	} else {
		// 1.5 根据快照获取资源配置
		logger.Info("Loading resources from snapshot...")
		resources, err := s.GetResourcesByVersionSnapshot(planTask.SnapshotResourceVersions, logger)
		if err != nil {
			logger.LogError("fetching", err, map[string]interface{}{
				"task_id":      task.ID,
				"plan_task_id": planTask.ID,
			}, nil)
			logger.StageEnd("fetching")
			s.saveTaskFailure(task, logger, err, "apply")
			return fmt.Errorf("failed to get resources from snapshot: %w", err)
		}
		logger.Info("✓ Loaded %d resources from snapshot", len(resources))

		// 1.6 生成配置文件（使用快照数据）
		logger.Info("Generating configuration files from snapshot...")

		// 【修复】在Agent模式下,优先使用Context中的完整变量数据
		var variableSnapshots interface{} = planTask.SnapshotVariables
		if planTask.Context != nil {
			if snapVars, ok := planTask.Context["_snapshot_variables"]; ok && snapVars != nil {
				variableSnapshots = snapVars
				logger.Debug("Using snapshot_variables from context (Agent mode with full data)")
			}
		}

		if err := s.GenerateConfigFilesFromSnapshot(workspace, resources, variableSnapshots, workDir, logger); err != nil {
			logger.LogError("fetching", err, map[string]interface{}{
				"workspace_id": workspace.WorkspaceID,
				"work_dir":     workDir,
			}, nil)
			logger.StageEnd("fetching")
			s.saveTaskFailure(task, logger, err, "apply")
			return err
		}

		// 1.7 准备State文件
		logger.Info("Preparing state file...")
		if err := s.PrepareStateFileWithLogging(workspace, workDir, logger); err != nil {
			logger.LogError("fetching", err, map[string]interface{}{
				"workspace_id": workspace.WorkspaceID,
				"work_dir":     workDir,
			}, nil)
			logger.StageEnd("fetching")

			// 保存失败信息
			s.saveTaskFailure(task, logger, err, "apply")
			return err
		}
	}

	logger.Info("Configuration fetch completed successfully")
	logger.StageEnd("fetching")

	// ========== 阶段2: Init（可能跳过）==========
	// 【Phase 1优化】检查是否可以跳过init
	canSkipInit := false
	if planTask.PlanHash != "" && planTask.AgentID != nil {
		// 获取当前 Agent 的 hostname
		currentHostname, err := os.Hostname()
		if err != nil {
			logger.Warn("Failed to get current hostname: %v, will run init", err)
		} else {
			// 获取 Plan 任务的 agent name（hostname）
			planAgentID := *planTask.AgentID
			var planAgentName string

			// 优先从 planTask.Context 中获取 agent_name（GetPlanTask API 返回的）
			if planTask.Context != nil {
				if name, ok := planTask.Context["_agent_name"].(string); ok && name != "" {
					planAgentName = name
				}
			}

			// 如果没有从 Context 获取到，尝试查询数据库（仅 Local 模式）
			if planAgentName == "" && s.db != nil {
				var agent models.Agent
				if err := s.db.Where("agent_id = ?", planAgentID).First(&agent).Error; err == nil {
					planAgentName = agent.Name
				}
			}

			// 比较 hostname
			isSameAgent := (planAgentName != "" && planAgentName == currentHostname)

			if isSameAgent {
				logger.Info("Checking if init can be skipped (same agent detected)...")
				logger.Info("  - Plan agent: %s", planAgentID)
				logger.Info("  - Current hostname: %s", currentHostname)

				// 在同一 agent 上，验证 plan hash
				if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
					canSkipInit = true
					logger.Info("✓ Same agent and plan hash verified, skipping init")
					log.Printf("Task %d: Skipping init (same agent optimization, saved ~85-96%% time)", task.ID)
				} else {
					logger.Warn("Plan hash mismatch, will run init")
				}
			} else {
				// 不在同一 agent 上，必须重新 init
				logger.Info("Different agent detected, must run init:")
				logger.Info("  - Plan agent: %s", planAgentID)
				logger.Info("  - Current hostname: %s", currentHostname)
			}
		}
	}

	if !canSkipInit {
		logger.StageBegin("init")

		if err := s.TerraformInitWithLogging(ctx, workDir, task, workspace, logger); err != nil {
			logger.LogError("init", err, map[string]interface{}{
				"workspace_id": workspace.WorkspaceID,
				"work_dir":     workDir,
			}, nil)
			logger.StageEnd("init")

			// 保存失败信息
			s.saveTaskFailure(task, logger, err, "apply")
			return err
		}

		logger.StageEnd("init")
	} else {
		// 跳过init，直接记录
		logger.Info("Init stage skipped (using preserved workspace from plan on same agent)")
	}

	// ========== 阶段3: Restoring Plan（可能跳过）==========
	// 【Phase 1优化】如果在同一 agent 且 plan 文件已存在，跳过恢复
	planFile := filepath.Join(workDir, "plan.out")
	needRestorePlan := true

	if canSkipInit && planTask.PlanHash != "" {
		logger.Info("Checking if plan file already exists on same agent...")
		if _, err := os.Stat(planFile); err == nil {
			if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
				needRestorePlan = false
				logger.Info("✓ Plan file already exists on same agent, skipping restore")
				log.Printf("Task %d: Reusing existing plan file (same agent optimization)", task.ID)
			}
		}
	}

	if needRestorePlan {
		logger.StageBegin("restoring_plan")

		logger.Info("Restoring plan file from plan task #%d...", planTask.ID)
		logger.Info("  - Plan data size: %.1f KB", float64(len(planTask.PlanData))/1024)

		if len(planTask.PlanData) == 0 {
			logger.Error("Plan data is empty")
			logger.LogError("restoring_plan", fmt.Errorf("plan data is empty"), map[string]interface{}{
				"plan_task_id": planTask.ID,
			}, nil)
			logger.StageEnd("restoring_plan")

			// 保存失败信息
			s.saveTaskFailure(task, logger, fmt.Errorf("plan data is empty"), "apply")
			return fmt.Errorf("plan data is empty")
		}

		if err := os.WriteFile(planFile, planTask.PlanData, 0644); err != nil {
			logger.Error("Failed to write plan file: %v", err)
			logger.LogError("restoring_plan", err, map[string]interface{}{
				"plan_file": planFile,
			}, nil)
			logger.StageEnd("restoring_plan")

			// 保存失败信息
			s.saveTaskFailure(task, logger, err, "apply")
			return fmt.Errorf("failed to write plan file: %w", err)
		}

		logger.Info("✓ Restored plan file to work directory")
		logger.Info("Validating plan file...")
		logger.Info("✓ Plan file is valid and ready for apply")
		logger.StageEnd("restoring_plan")
	} else {
		logger.Info("Plan restore skipped (using preserved plan file from same agent)")
	}

	// ========== 阶段4: Applying ==========
	logger.StageBegin("applying")

	// ========== 进入关键区：Applying ==========
	s.signalManager.EnterCriticalSection("applying")

	// 获取Terraform二进制文件路径（已在Fetching阶段下载）
	// 必须使用下载的版本，不允许回退到系统terraform
	if s.downloader == nil {
		err := fmt.Errorf("terraform downloader not initialized")
		logger.LogError("applying", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
		}, nil)
		logger.StageEnd("applying")
		s.saveTaskFailure(task, logger, err, "apply")
		return err
	}

	// 使用EnsureTerraformBinary获取实际路径（处理"latest"等特殊版本）
	binaryPath, err := s.downloader.EnsureTerraformBinary(workspace.TerraformVersion)
	if err != nil {
		logger.LogError("applying", err, map[string]interface{}{
			"workspace_id":      workspace.WorkspaceID,
			"terraform_version": workspace.TerraformVersion,
		}, nil)
		logger.StageEnd("applying")
		s.saveTaskFailure(task, logger, err, "apply")
		return fmt.Errorf("failed to ensure terraform binary for version %s: %w", workspace.TerraformVersion, err)
	}

	terraformCmd := binaryPath
	logger.Info("Using downloaded terraform binary: %s", terraformCmd)

	args := []string{"apply", "-no-color", "-auto-approve", planFile}
	logger.Info("Executing: %s apply -no-color -auto-approve plan.out", terraformCmd)

	cmd := exec.CommandContext(ctx, terraformCmd, args...)
	cmd.Dir = workDir
	cmd.Env = s.buildEnvironmentVariables(workspace)

	// 使用Pipe实时捕获输出
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logger.LogError("applying", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
		}, nil)
		logger.StageEnd("applying")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "apply")
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logger.LogError("applying", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
		}, nil)
		logger.StageEnd("applying")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "apply")
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 启动命令
	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		// 检查是否是context取消导致的
		if ctx.Err() == context.Canceled {
			logger.Info("Task cancelled by user during apply startup")
			s.saveTaskCancellation(task, logger, "apply")
			return fmt.Errorf("task cancelled by user")
		}
		logger.LogError("applying", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
		}, nil)
		logger.StageEnd("applying")

		// 保存失败信息
		s.saveTaskFailure(task, logger, err, "apply")
		return fmt.Errorf("failed to start terraform: %w", err)
	}

	// 创建Apply解析器用于实时解析资源状态
	// 使用 NewApplyOutputParserWithAccessor 以支持 Agent 模式
	applyParser := NewApplyOutputParserWithAccessor(task.ID, s.dataAccessor, s.streamManager)

	// 实时读取stdout
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			logger.RawOutput(line)
			// 解析Apply输出以更新资源状态
			applyParser.ParseLine(line)
		}
	}()

	// 实时读取stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			logger.RawOutput(line)
			// 也解析stderr（有些输出可能在stderr）
			applyParser.ParseLine(line)
		}
	}()

	// 等待命令完成
	cmdErr := cmd.Wait()
	wg.Wait()

	duration := time.Since(startTime)

	if cmdErr != nil {
		// 检查是否是context取消导致的
		if ctx.Err() == context.Canceled {
			logger.Info("Task cancelled by user during apply execution")
			s.saveTaskCancellation(task, logger, "apply")
			return fmt.Errorf("task cancelled by user")
		}
		logger.Error("Terraform apply failed: %v", cmdErr)
		logger.LogError("applying", cmdErr, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
			"duration":     duration.Seconds(),
		}, nil)
		logger.StageEnd("applying")

		// 保存失败信息（包含所有阶段的日志）
		s.saveTaskFailure(task, logger, cmdErr, "apply")
		return fmt.Errorf("terraform apply failed: %w", cmdErr)
	}

	logger.Info("✓ Apply completed successfully")
	logger.Info("Apply execution time: %.1f seconds", duration.Seconds())

	// ========== 退出关键区：Applying ==========
	s.signalManager.ExitCriticalSection("applying")

	// 提取terraform outputs
	logger.Info("Extracting terraform outputs...")
	outputs, err := s.extractTerraformOutputs(ctx, workDir)
	if err != nil {
		logger.Warn("Failed to extract outputs: %v", err)
	} else if len(outputs) > 0 {
		logger.Info("✓ Found %d outputs", len(outputs))
		for key := range outputs {
			logger.Info("  - %s", key)
		}
	}

	logger.StageEnd("applying")

	// ========== 阶段5: Saving State ==========
	logger.StageBegin("saving_state")

	// ========== 进入关键区：Saving State ==========
	s.signalManager.EnterCriticalSection("saving_state")
	defer s.signalManager.ExitCriticalSection("saving_state")

	logger.Info("Reading state file from work directory...")

	// 先获取并保存日志（在State保存之前）
	applyOutputBeforeState := logger.GetFullOutput()
	task.ApplyOutput = applyOutputBeforeState
	s.dataAccessor.UpdateTask(task)
	s.saveTaskLog(task.ID, "apply", applyOutputBeforeState, "info")
	log.Printf("Task %d apply output saved before state (%d bytes)", task.ID, len(applyOutputBeforeState))

	if err := s.SaveNewStateVersionWithLogging(workspace, task, workDir, logger); err != nil {
		logger.LogError("saving_state", err, map[string]interface{}{
			"workspace_id": workspace.WorkspaceID,
			"task_id":      task.ID,
		}, nil)
		logger.StageEnd("saving_state")

		// State保存失败，但日志已保存，更新最终日志
		finalOutput := logger.GetFullOutput()
		task.ApplyOutput = finalOutput
		task.Status = models.TaskStatusFailed
		task.ErrorMessage = err.Error()
		task.CompletedAt = timePtr(time.Now())
		s.dataAccessor.UpdateTask(task)
		s.saveTaskLog(task.ID, "apply", finalOutput, "error")

		// State保存失败是严重错误，但Apply已成功
		return fmt.Errorf("apply succeeded but state save failed: %w", err)
	}

	// 在所有阶段完成后获取完整输出（包含Fetching/Init/Restoring/Applying/Saving State所有阶段）
	applyOutput := logger.GetFullOutput()

	// 先保存日志（无论State保存成功与否）
	task.ApplyOutput = applyOutput
	s.dataAccessor.UpdateTask(task)
	s.saveTaskLog(task.ID, "apply", applyOutput, "info")
	log.Printf("Task %d apply output saved (%d bytes)", task.ID, len(applyOutput))

	// 【新增】从State提取资源详情（仅在Local模式下）
	if s.db != nil {
		logger.Info("Extracting resource details from state...")
		log.Printf("[DEBUG] Task %d: Starting resource ID extraction", task.ID)

		// 读取并解析state文件以提取资源详情
		stateFile := filepath.Join(workDir, "terraform.tfstate")
		log.Printf("[DEBUG] Task %d: Reading state file: %s", task.ID, stateFile)

		stateData, err := os.ReadFile(stateFile)
		if err != nil {
			logger.Warn("Failed to read state file for resource details: %v", err)
			log.Printf("[ERROR] Task %d: Failed to read state file: %v", task.ID, err)
		} else {
			log.Printf("[DEBUG] Task %d: State file read successfully, size: %d bytes", task.ID, len(stateData))

			var stateContent map[string]interface{}
			if err := json.Unmarshal(stateData, &stateContent); err != nil {
				logger.Warn("Failed to parse state for resource details: %v", err)
				log.Printf("[ERROR] Task %d: Failed to parse state JSON: %v", task.ID, err)
			} else {
				log.Printf("[DEBUG] Task %d: State JSON parsed successfully", task.ID)

				applyParserService := NewApplyParserService(s.db, s.streamManager)
				logger.Debug("Calling ExtractResourceDetailsFromState for task %d", task.ID)

				if err := applyParserService.ExtractResourceDetailsFromState(task.ID, stateContent, logger); err != nil {
					logger.Warn("Failed to extract resource details: %v", err)
				} else {
					logger.Info("✓ Resource details extracted successfully")
				}
			}
		}
	} else {
		logger.Debug("Skipping resource details extraction in Agent mode")
	}

	logger.Info("State save completed successfully")
	logger.StageEnd("saving_state")

	// 发送完成消息
	stream.Broadcast(OutputMessage{
		Type:      "completed",
		Timestamp: time.Now(),
	})

	// 更新任务状态为Applied（Apply成功完成）
	task.Status = models.TaskStatusApplied
	task.Stage = "applied"
	task.CompletedAt = timePtr(time.Now())
	task.Duration += int(duration.Seconds())

	if err := s.dataAccessor.UpdateTask(task); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	// 更新所有剩余pending状态的资源为completed
	// 这对于Agent模式特别重要，因为实时更新可能没有写入数据库
	if s.db != nil {
		now := time.Now()
		if err := s.db.Model(&models.WorkspaceTaskResourceChange{}).
			Where("task_id = ? AND apply_status = ?", task.ID, "pending").
			Updates(map[string]interface{}{
				"apply_status":       "completed",
				"apply_completed_at": now,
				"updated_at":         now,
			}).Error; err != nil {
			logger.Warn("Failed to update pending resources to completed: %v", err)
		} else {
			logger.Info("✓ Updated all pending resources to completed status")
		}
	}

	// Apply成功完成后，解锁workspace
	logger.Info("Unlocking workspace after successful apply...")
	if err := s.dataAccessor.UnlockWorkspace(workspace.WorkspaceID); err != nil {
		logger.Warn("Failed to unlock workspace: %v", err)
	} else {
		logger.Info("✓ Workspace unlocked successfully")
	}

	// 清理工作目录（Apply完成后不再需要）
	logger.Info("Cleaning up work directory after successful apply: %s", workDir)
	if err := s.CleanupWorkspace(workDir); err != nil {
		logger.Warn("Failed to cleanup work directory: %v", err)
	} else {
		logger.Info("✓ Work directory cleaned up successfully")
	}

	// 注意：CMDB 同步已移至 TaskQueueManager.syncCMDBAfterApply 统一处理
	// 这样可以确保 Local、Agent、K8s Agent 三种模式都能正确同步

	log.Printf("Task %d applied successfully", task.ID)

	// 注意：Run Triggers 在 Server 端处理（通过 TaskQueueManager.OnTaskCompleted）
	// 这样可以确保 Local、Agent、K8s Agent 三种模式都能正确触发

	// 发送任务完成通知（仅在 Local 模式下）
	if s.notificationSender != nil && s.db != nil {
		go func() {
			ctx := context.Background()
			log.Printf("[Notification] Triggering task_completed notification for task %d", task.ID)
			if err := s.notificationSender.TriggerNotifications(
				ctx,
				task.WorkspaceID,
				models.NotificationEventTaskCompleted,
				task,
			); err != nil {
				log.Printf("[Notification] Failed to send task_completed notification for task %d: %v", task.ID, err)
			} else {
				log.Printf("[Notification] Successfully triggered task_completed notification for task %d", task.ID)
			}
		}()
	}

	return nil
}

// RestorePlanFile 从数据库恢复Plan文件
func (s *TerraformExecutor) RestorePlanFile(
	task *models.WorkspaceTask,
	workDir string,
) (string, error) {
	if task.PlanTaskID == nil {
		return "", fmt.Errorf("apply task has no associated plan task")
	}

	var planTask models.WorkspaceTask
	if err := s.db.First(&planTask, *task.PlanTaskID).Error; err != nil {
		return "", fmt.Errorf("failed to get plan task: %w", err)
	}

	if len(planTask.PlanData) == 0 {
		return "", fmt.Errorf("plan data is empty")
	}

	planFile := filepath.Join(workDir, "plan.out")
	if err := os.WriteFile(planFile, planTask.PlanData, 0644); err != nil {
		return "", fmt.Errorf("failed to write plan file: %w", err)
	}

	log.Printf("Restored plan file from task %d (size: %d bytes)",
		planTask.ID, len(planTask.PlanData))

	return planFile, nil
}

// SaveNewStateVersion 保存新的State版本（带容错）
func (s *TerraformExecutor) SaveNewStateVersion(
	workspace *models.Workspace,
	task *models.WorkspaceTask,
	workDir string,
) error {
	stateFile := filepath.Join(workDir, "terraform.tfstate")

	// 1. 读取State文件
	stateData, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	// 2. 立即备份到文件系统（第一道保险）
	backupDir := "/var/backup/states"
	os.MkdirAll(backupDir, 0700)
	backupPath := filepath.Join(backupDir,
		fmt.Sprintf("ws_%s_task_%d_%d.tfstate",
			workspace.WorkspaceID, task.ID, time.Now().Unix()))

	if err := os.WriteFile(backupPath, stateData, 0600); err != nil {
		log.Printf("WARNING: Failed to backup state to file: %v", err)
	} else {
		log.Printf("State backed up to: %s", backupPath)
	}

	// 3. 尝试保存到数据库（带重试）
	maxRetries := 5
	var saveErr error

	for i := 0; i < maxRetries; i++ {
		saveErr = s.SaveStateToDatabase(workspace, task, stateData)
		if saveErr == nil {
			log.Printf("State saved to database successfully")
			// 注意：CMDB 同步由 TaskQueueManager.syncCMDBAfterApply 统一处理
			return nil
		}

		log.Printf("Failed to save state (attempt %d/%d): %v", i+1, maxRetries, saveErr)

		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
		}
	}

	// 4. 所有重试失败 - 自动锁定workspace
	log.Printf("CRITICAL: Failed to save state after %d retries", maxRetries)

	// 4.1 自动锁定workspace
	lockErr := s.lockWorkspace(
		workspace.WorkspaceID, // 保持使用内部数字ID
		*task.CreatedBy,
		fmt.Sprintf("Auto-locked: State save failed for task %d. State backed up to %s",
			task.ID, backupPath),
	)
	if lockErr != nil {
		log.Printf("ERROR: Failed to auto-lock workspace: %v", lockErr)
	}

	// 4.2 更新任务状态为部分成功
	task.Status = "partial_success"
	task.ErrorMessage = fmt.Sprintf(
		"Apply succeeded but state save failed. Workspace auto-locked. State backed up to: %s",
		backupPath)
	s.dataAccessor.UpdateTask(task)

	return fmt.Errorf("state save failed, workspace locked, backup at: %s", backupPath)
}

// SaveStateToDatabase 保存State到数据库（公开方法，供重试使用）
func (s *TerraformExecutor) SaveStateToDatabase(
	workspace *models.Workspace,
	task *models.WorkspaceTask,
	stateData []byte,
) error {
	var stateContent map[string]interface{}
	if err := json.Unmarshal(stateData, &stateContent); err != nil {
		return fmt.Errorf("failed to parse state: %w", err)
	}

	checksum := s.calculateChecksum(stateData)

	// 使用 DataAccessor 获取最大版本号
	maxVersion, err := s.dataAccessor.GetMaxStateVersion(workspace.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get max state version: %w", err)
	}

	newVersion := maxVersion + 1

	// Agent 模式不支持事务，需要顺序执行
	if s.db != nil {
		// Local 模式：使用事务
		return s.db.Transaction(func(tx *gorm.DB) error {
			stateVersion := &models.WorkspaceStateVersion{
				WorkspaceID: workspace.WorkspaceID,
				Version:     newVersion,
				Content:     stateContent,
				Checksum:    checksum,
				SizeBytes:   len(stateData),
				TaskID:      &task.ID,
				CreatedBy:   task.CreatedBy,
			}

			if err := tx.Create(stateVersion).Error; err != nil {
				return err
			}

			// 更新workspace的tf_state
			return tx.Model(workspace).Update("tf_state", stateContent).Error
		})
	} else {
		// Agent 模式：顺序执行（通过 DataAccessor）
		stateVersion := &models.WorkspaceStateVersion{
			WorkspaceID: workspace.WorkspaceID,
			Version:     newVersion,
			Content:     stateContent,
			Checksum:    checksum,
			SizeBytes:   len(stateData),
			TaskID:      &task.ID,
			CreatedBy:   task.CreatedBy,
		}

		// 创建新的 state version
		if err := s.dataAccessor.SaveStateVersion(stateVersion); err != nil {
			return err
		}

		// 更新 workspace 的 tf_state
		return s.dataAccessor.UpdateWorkspaceState(workspace.WorkspaceID, stateContent)
	}
}

// lockWorkspace 锁定workspace
func (s *TerraformExecutor) lockWorkspace(
	workspaceID string,
	userID string,
	reason string,
) error {
	return s.dataAccessor.LockWorkspace(workspaceID, userID, reason)
}

// GetTaskLogs 获取任务日志
func (s *TerraformExecutor) GetTaskLogs(taskID uint) ([]models.TaskLog, error) {
	return s.dataAccessor.GetTaskLogs(taskID)
}

// generateMainTF 生成main.tf.json（支持资源级别版本管理）
func (s *TerraformExecutor) generateMainTF(workspace *models.Workspace) (map[string]interface{}, error) {
	// 使用 DataAccessor 获取资源
	resources, err := s.dataAccessor.GetWorkspaceResources(workspace.WorkspaceID)
	if err != nil {
		log.Printf("Warning: failed to get resources: %v", err)
		// 如果获取资源失败，使用workspace.TFCode
		if workspace.TFCode != nil {
			return workspace.TFCode, nil
		}
		return make(map[string]interface{}), nil
	}

	// 如果有资源记录，从资源聚合生成
	if len(resources) > 0 {
		return s.generateMainTFFromResources(resources)
	}

	// 否则使用workspace.TFCode（向后兼容）
	if workspace.TFCode != nil {
		return workspace.TFCode, nil
	}

	// 都没有，返回空配置
	return make(map[string]interface{}), nil
}

// generateMainTFFromResources 从资源聚合生成main.tf.json
func (s *TerraformExecutor) generateMainTFFromResources(resources []models.WorkspaceResource) (map[string]interface{}, error) {
	// 聚合所有资源的TF代码
	mainTF := make(map[string]interface{})

	// 预加载所有 modules 用于查找 version
	moduleVersions := make(map[string]string) // key: provider_name, value: version
	if s.db != nil {
		// Local 模式：从数据库加载
		var modules []models.Module
		if err := s.db.Select("name, provider, version").Find(&modules).Error; err == nil {
			for _, m := range modules {
				key := fmt.Sprintf("%s_%s", m.Provider, m.Name)
				if m.Version != "" {
					moduleVersions[key] = m.Version
				}
			}
		}
	} else if s.dataAccessor != nil {
		// Agent 模式：从 RemoteDataAccessor 获取（通过 GetTaskData API 返回的 module_versions）
		if remoteAccessor, ok := s.dataAccessor.(*RemoteDataAccessor); ok {
			moduleVersions = remoteAccessor.GetModuleVersions()
		}
	}

	for _, resource := range resources {
		if resource.CurrentVersion == nil {
			log.Printf("Warning: Resource %s has no CurrentVersion, skipping", resource.ResourceID)
			continue
		}

		// 复制 tf_code 以避免修改原始数据
		tfCode := s.copyTFCode(resource.CurrentVersion.TFCode)

		// 检查并添加 version 字段（如果缺失）
		s.ensureModuleVersion(tfCode, resource.ResourceType, moduleVersions)

		// 合并资源的TF代码到main.tf
		s.mergeTFCode(mainTF, tfCode)
	}

	return mainTF, nil
}

// copyTFCode 深拷贝 tf_code
func (s *TerraformExecutor) copyTFCode(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return nil
	}
	result := make(map[string]interface{})
	for k, v := range source {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = s.copyTFCode(val)
		case []interface{}:
			newSlice := make([]interface{}, len(val))
			for i, item := range val {
				if itemMap, ok := item.(map[string]interface{}); ok {
					newSlice[i] = s.copyTFCode(itemMap)
				} else {
					newSlice[i] = item
				}
			}
			result[k] = newSlice
		default:
			result[k] = v
		}
	}
	return result
}

// ensureModuleVersion 确保 module 配置中包含 version 字段
// 【已禁用】版本信息现在由前端在创建/编辑资源时直接写入 tf_code
// 保留此函数以便将来可能需要恢复
func (s *TerraformExecutor) ensureModuleVersion(tfCode map[string]interface{}, resourceType string, moduleVersions map[string]string) {
	// 【已禁用】不再自动注入 version，由前端负责
	// 如果 tf_code 中没有 version，说明前端没有传递，这是预期行为
	log.Printf("[ensureModuleVersion] DISABLED - version injection is now handled by frontend for resource_type: %s", resourceType)
	return

	/* 原始逻辑已注释
	// 获取 module 块
	moduleBlock, ok := tfCode["module"].(map[string]interface{})
	if !ok {
		log.Printf("[ensureModuleVersion] No module block found in tfCode for resource_type: %s", resourceType)
		return
	}

	log.Printf("[ensureModuleVersion] Processing resource_type: %s, moduleVersions has %d entries", resourceType, len(moduleVersions))

	// 遍历所有 module 定义
	for moduleName, moduleConfig := range moduleBlock {
		// module 配置通常是一个数组
		configArray, ok := moduleConfig.([]interface{})
		if !ok || len(configArray) == 0 {
			log.Printf("[ensureModuleVersion] Module %s config is not array or empty", moduleName)
			continue
		}

		// 获取第一个配置对象
		config, ok := configArray[0].(map[string]interface{})
		if !ok {
			log.Printf("[ensureModuleVersion] Module %s first config is not map", moduleName)
			continue
		}

		// 检查是否已有 version 字段
		if existingVersion, hasVersion := config["version"]; hasVersion {
			log.Printf("[ensureModuleVersion] Module %s already has version: %v", moduleName, existingVersion)
			continue
		}

		// 检查是否有 source 字段
		source, hasSource := config["source"].(string)
		if !hasSource || source == "" {
			log.Printf("[ensureModuleVersion] Module %s has no source field", moduleName)
			continue
		}

		log.Printf("[ensureModuleVersion] Module %s has source: %s, looking up version for resource_type: %s", moduleName, source, resourceType)

		// 尝试从 moduleVersions 中查找 version（使用 resourceType 作为 key）
		if version, found := moduleVersions[resourceType]; found && version != "" {
			config["version"] = version
			log.Printf("[ensureModuleVersion] ✓ Added version %s to module %s (resource_type: %s)", version, moduleName, resourceType)
		} else {
			log.Printf("[ensureModuleVersion] ✗ No version found for resource_type: %s (available keys: %v)", resourceType, getMapKeys(moduleVersions))
		}
	}
	*/
}

// getMapKeys 获取 map 的所有 key（用于调试）
func getMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// cleanProviderConfig 清理provider配置，移除空的terraform块
func (s *TerraformExecutor) cleanProviderConfig(providerConfig map[string]interface{}) map[string]interface{} {
	if providerConfig == nil {
		return providerConfig
	}

	cleaned := make(map[string]interface{})
	for key, value := range providerConfig {
		// 如果是terraform块
		if key == "terraform" {
			// 检查是否为空数组或空对象
			if arr, ok := value.([]interface{}); ok && len(arr) == 0 {
				// 跳过空的terraform块
				log.Printf("Skipping empty terraform block in provider config")
				continue
			}
			if obj, ok := value.(map[string]interface{}); ok && len(obj) == 0 {
				// 跳过空的terraform对象
				log.Printf("Skipping empty terraform object in provider config")
				continue
			}
		}
		cleaned[key] = value
	}

	return cleaned
}

// mergeTFCode 合并TF代码
func (s *TerraformExecutor) mergeTFCode(target, source map[string]interface{}) {
	for key, value := range source {
		if existing, ok := target[key]; ok {
			// 如果key已存在，合并内容
			if existingMap, ok := existing.(map[string]interface{}); ok {
				if sourceMap, ok := value.(map[string]interface{}); ok {
					for k, v := range sourceMap {
						existingMap[k] = v
					}
					continue
				}
			}
		}
		target[key] = value
	}
}

// parsePlanChanges 解析Plan JSON获取资源变更统计
func (s *TerraformExecutor) parsePlanChanges(planJSON map[string]interface{}) (int, int, int) {
	add, change, destroy := 0, 0, 0

	if resourceChanges, ok := planJSON["resource_changes"].([]interface{}); ok {
		for _, rc := range resourceChanges {
			if changeMap, ok := rc.(map[string]interface{}); ok {
				if changeDetail, ok := changeMap["change"].(map[string]interface{}); ok {
					if actions, ok := changeDetail["actions"].([]interface{}); ok {
						for _, action := range actions {
							switch action.(string) {
							case "create":
								add++
							case "update":
								change++
							case "delete":
								destroy++
							}
						}
					}
				}
			}
		}
	}

	return add, change, destroy
}

// timePtr 返回时间指针（使用 UTC 时间，与 GORM autoCreateTime 保持一致）
func timePtr(t time.Time) *time.Time {
	utc := t.UTC()
	return &utc
}

// ============================================================================
// Phase 1 优化：Plan Hash 管理
// ============================================================================

// calculatePlanHash 计算plan文件的SHA256 hash
func (s *TerraformExecutor) calculatePlanHash(planFile string) (string, error) {
	planData, err := os.ReadFile(planFile)
	if err != nil {
		return "", fmt.Errorf("failed to read plan file: %w", err)
	}

	hash := sha256.Sum256(planData)
	return hex.EncodeToString(hash[:]), nil
}

// verifyPlanHash 验证plan文件的hash是否匹配
// restoreTerraformLockHCL 从数据库恢复 .terraform.lock.hcl 文件到工作目录
// 这个文件记录了 provider 的精确版本和 hash，有了它 terraform init 可以跳过 provider 下载
func (s *TerraformExecutor) restoreTerraformLockHCL(workDir, workspaceID string, logger *TerraformLogger) {
	lockContent, err := s.dataAccessor.GetTerraformLockHCL(workspaceID)
	if err != nil {
		logger.Debug("Failed to get terraform lock hcl: %v", err)
		return
	}

	if lockContent == "" {
		logger.Debug("No terraform lock hcl found for workspace %s", workspaceID)
		return
	}

	lockFile := filepath.Join(workDir, ".terraform.lock.hcl")
	if err := os.WriteFile(lockFile, []byte(lockContent), 0644); err != nil {
		logger.Warn("Failed to restore .terraform.lock.hcl: %v", err)
		return
	}

	logger.Info("✓ Restored .terraform.lock.hcl (%.1f KB)", float64(len(lockContent))/1024)
}

// saveTerraformLockHCL 保存 .terraform.lock.hcl 文件内容到数据库
// 在 terraform init 成功后调用，确保下次运行可以复用 provider 缓存
func (s *TerraformExecutor) saveTerraformLockHCL(workDir, workspaceID string, logger *TerraformLogger) {
	lockFile := filepath.Join(workDir, ".terraform.lock.hcl")
	lockContent, err := os.ReadFile(lockFile)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Failed to read .terraform.lock.hcl: %v", err)
		}
		return
	}

	if len(lockContent) == 0 {
		return
	}

	if err := s.dataAccessor.SaveTerraformLockHCL(workspaceID, string(lockContent)); err != nil {
		logger.Warn("Failed to save .terraform.lock.hcl: %v", err)
		return
	}

	logger.Info("✓ Saved .terraform.lock.hcl (%.1f KB)", float64(len(lockContent))/1024)
}

func (s *TerraformExecutor) verifyPlanHash(workDir string, expectedHash string, logger *TerraformLogger) bool {
	planFile := filepath.Join(workDir, "plan.out")

	// 检查文件是否存在
	if _, err := os.Stat(planFile); os.IsNotExist(err) {
		logger.Debug("Plan file does not exist: %s", planFile)
		return false
	}

	// 计算当前hash
	currentHash, err := s.calculatePlanHash(planFile)
	if err != nil {
		logger.Debug("Failed to calculate plan hash: %v", err)
		return false
	}

	// 比较hash
	if currentHash != expectedHash {
		logger.Debug("Plan hash mismatch: expected %s, got %s", expectedHash[:16]+"...", currentHash[:16]+"...")
		return false
	}

	logger.Debug("Plan hash verified: %s", currentHash[:16]+"...")
	return true
}

// workDirExists 检查工作目录是否存在且包含必要文件
func (s *TerraformExecutor) workDirExists(workDir string) bool {
	// 检查目录是否存在
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return false
	}

	// 检查关键文件是否存在
	requiredFiles := []string{
		"main.tf.json",
		"provider.tf.json",
		".terraform",
	}

	for _, file := range requiredFiles {
		filePath := filepath.Join(workDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return false
		}
	}

	return true
}

// ============================================================================
// 资源版本快照管理
// ============================================================================

// CreateResourceVersionSnapshot 创建资源版本快照（新版本，保存到task表）
func (s *TerraformExecutor) CreateResourceVersionSnapshot(
	task *models.WorkspaceTask,
	workspace *models.Workspace,
	logger *TerraformLogger,
) error {
	logger.Debug("Starting resource version snapshot creation...")

	// 1. 快照资源版本号（只存版本号，不存完整数据）
	resources, err := s.dataAccessor.GetWorkspaceResources(workspace.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace resources: %w", err)
	}

	resourceVersions := make(map[string]interface{})
	for _, r := range resources {
		if r.CurrentVersion != nil {
			resourceVersions[r.ResourceID] = map[string]interface{}{
				"version_id": r.CurrentVersion.ID,
				"version":    r.CurrentVersion.Version,
			}
		}
	}
	logger.Debug("Captured %d resource versions", len(resourceVersions))

	// 2. 快照变量（只保存variable_id和version引用）
	// 使用子查询只获取每个变量的最新版本
	var variables []models.WorkspaceVariable
	if err := s.db.Raw(`
		SELECT wv.*
		FROM workspace_variables wv
		INNER JOIN (
			SELECT variable_id, MAX(version) as max_version
			FROM workspace_variables
			WHERE workspace_id = ? AND is_deleted = false
			GROUP BY variable_id
		) latest ON wv.variable_id = latest.variable_id AND wv.version = latest.max_version
		WHERE wv.workspace_id = ? AND wv.is_deleted = false
	`, workspace.WorkspaceID, workspace.WorkspaceID).Scan(&variables).Error; err != nil {
		return fmt.Errorf("failed to get latest variables: %w", err)
	}

	// 构建变量快照：只保存必要字段（引用格式）
	// 使用 map 而不是结构体，避免 JSON 序列化包含零值字段
	variableSnapshots := make([]map[string]interface{}, 0, len(variables))
	for _, v := range variables {
		variableSnapshots = append(variableSnapshots, map[string]interface{}{
			"workspace_id":  v.WorkspaceID,
			"variable_id":   v.VariableID,
			"version":       v.Version,
			"variable_type": string(v.VariableType),
		})
	}
	logger.Debug("Captured %d variable references", len(variableSnapshots))

	// 3. 快照Provider配置
	providerConfig := workspace.ProviderConfig
	logger.Debug("Captured provider configuration")

	// 4. 记录快照时间
	snapshotTime := time.Now()

	// 5. 保存快照到task
	if s.db != nil {
		// Local模式：使用原始SQL确保JSON格式正确
		variablesJSON, err := json.Marshal(variableSnapshots)
		if err != nil {
			return fmt.Errorf("failed to marshal variables: %w", err)
		}

		resourceVersionsJSON, err := json.Marshal(models.JSONB(resourceVersions))
		if err != nil {
			return fmt.Errorf("failed to marshal resource versions: %w", err)
		}

		providerConfigJSON, err := json.Marshal(models.JSONB(providerConfig))
		if err != nil {
			return fmt.Errorf("failed to marshal provider config: %w", err)
		}

		if err := s.db.Exec(`
			UPDATE workspace_tasks 
			SET snapshot_resource_versions = ?::jsonb,
			    snapshot_variables = ?::jsonb,
			    snapshot_provider_config = ?::jsonb,
			    snapshot_created_at = ?
			WHERE id = ?
		`, resourceVersionsJSON, variablesJSON, providerConfigJSON, snapshotTime, task.ID).Error; err != nil {
			return fmt.Errorf("failed to save snapshot: %w", err)
		}
		logger.Debug("Snapshot saved to database")
	} else {
		// Agent模式：通过DataAccessor保存（需要在task对象中设置）
		task.SnapshotResourceVersions = models.JSONB(resourceVersions)
		// 将 map 数组转换为 WorkspaceVariable 数组以兼容现有字段类型
		snapshotVars := make([]models.WorkspaceVariable, 0, len(variableSnapshots))
		for _, snap := range variableSnapshots {
			snapshotVars = append(snapshotVars, models.WorkspaceVariable{
				WorkspaceID:  snap["workspace_id"].(string),
				VariableID:   snap["variable_id"].(string),
				Version:      snap["version"].(int),
				VariableType: models.VariableType(snap["variable_type"].(string)),
			})
		}
		// Convert to JSONB format (map with _array key for compatibility)
		task.SnapshotVariables = models.JSONB{"_array": snapshotVars}
		task.SnapshotProviderConfig = models.JSONB(providerConfig)
		task.SnapshotCreatedAt = &snapshotTime

		if err := s.dataAccessor.UpdateTask(task); err != nil {
			return fmt.Errorf("failed to save snapshot in agent mode: %w", err)
		}
		logger.Debug("Snapshot saved via DataAccessor")
	}

	logger.Info("Snapshot summary:")
	logger.Info("  - Resources: %d", len(resourceVersions))
	logger.Info("  - Variables: %d", len(variables))
	logger.Info("  - Provider config: %v", providerConfig != nil)
	logger.Info("  - Created at: %s", snapshotTime.Format("2006-01-02 15:04:05"))

	return nil
}

// CreateResourceSnapshot 创建资源版本快照（旧版本，生成snapshot_id）
func (s *TerraformExecutor) CreateResourceSnapshot(workspaceID string) (string, error) {
	// 使用 DataAccessor 获取资源（包含版本信息）
	resources, err := s.dataAccessor.GetWorkspaceResourcesWithVersions(workspaceID)
	if err != nil {
		return "", err
	}

	// 创建快照
	type ResourceSnapshot struct {
		ResourceID string `json:"resource_id"`
		VersionID  uint   `json:"version_id"`
		Version    int    `json:"version"`
	}

	snapshot := []ResourceSnapshot{}
	for _, resource := range resources {
		if resource.CurrentVersion != nil {
			snapshot = append(snapshot, ResourceSnapshot{
				ResourceID: resource.ResourceID,
				VersionID:  resource.CurrentVersion.ID,
				Version:    resource.CurrentVersion.Version,
			})
		}
	}

	// 生成快照ID（使用JSON序列化后的hash）
	snapshotJSON, _ := json.Marshal(snapshot)
	hash := sha256.Sum256(snapshotJSON)
	snapshotID := hex.EncodeToString(hash[:])

	log.Printf("Created resource snapshot for workspace %s: %s (%d resources)",
		workspaceID, snapshotID[:16], len(snapshot))

	return snapshotID, nil
}

// ValidateResourceSnapshot 验证资源版本快照
func (s *TerraformExecutor) ValidateResourceSnapshot(task *models.WorkspaceTask) error {
	if task.SnapshotID == "" {
		return fmt.Errorf("no snapshot ID found")
	}

	// 获取当前快照 (task.WorkspaceID 现在是 string)
	currentSnapshotID, err := s.CreateResourceSnapshot(task.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to create current snapshot: %w", err)
	}

	// 对比
	if currentSnapshotID != task.SnapshotID {
		return fmt.Errorf("resources have changed since plan (expected: %s, current: %s)",
			task.SnapshotID[:16], currentSnapshotID[:16])
	}

	log.Printf("Resource snapshot validation passed for task %d", task.ID)
	return nil
}

// ============================================================================
// 带日志的辅助方法
// ============================================================================

// GenerateConfigFilesWithLogging 生成配置文件（带详细日志）
func (s *TerraformExecutor) GenerateConfigFilesWithLogging(
	workspace *models.Workspace,
	workDir string,
	logger *TerraformLogger,
) error {
	logger.Debug("Aggregating TF code from resources...")

	// 1. 生成 main.tf.json
	mainTF, err := s.generateMainTF(workspace)
	if err != nil {
		return fmt.Errorf("failed to generate main.tf: %w", err)
	}

	if err := s.writeJSONFile(workDir, "main.tf.json", mainTF); err != nil {
		return fmt.Errorf("failed to write main.tf.json: %w", err)
	}

	// 计算文件大小
	mainTFData, _ := json.MarshalIndent(mainTF, "", "  ")
	logger.Info("✓ Generated main.tf.json (%.1f KB)", float64(len(mainTFData))/1024)

	// 只在TRACE级别打印完整内容
	logger.Trace("========== main.tf.json Content ==========")
	logger.Trace("%s", string(mainTFData))
	logger.Trace("==========================================")

	// 2. 生成 provider.tf.json
	// 清理空的terraform块，避免Terraform尝试读取不存在的backend state
	cleanedProviderConfig := s.cleanProviderConfig(workspace.ProviderConfig)
	if err := s.writeJSONFile(workDir, "provider.tf.json", cleanedProviderConfig); err != nil {
		return fmt.Errorf("failed to write provider.tf.json: %w", err)
	}
	providerData, _ := json.MarshalIndent(cleanedProviderConfig, "", "  ")
	logger.Info("✓ Generated provider.tf.json")

	// 只在TRACE级别打印完整内容
	logger.Trace("========== provider.tf.json Content ==========")
	logger.Trace("%s", string(providerData))
	logger.Trace("==============================================")

	// 3. 生成 variables.tf.json
	if err := s.generateVariablesTFJSON(workspace, workDir); err != nil {
		return fmt.Errorf("failed to write variables.tf.json: %w", err)
	}

	// 获取变量数量（使用 DataAccessor）
	variables, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
	varCount := len(variables)
	sensitiveCount := 0
	if err == nil {
		for _, v := range variables {
			if v.Sensitive {
				sensitiveCount++
			}
		}
	}
	logger.Info("✓ Generated variables.tf.json (%d variables)", varCount)

	// 只在TRACE级别打印完整内容
	varsTFData, _ := os.ReadFile(filepath.Join(workDir, "variables.tf.json"))
	logger.Trace("========== variables.tf.json Content ==========")
	logger.Trace("%s", string(varsTFData))
	logger.Trace("===============================================")

	// 4. 生成 variables.tfvars
	if err := s.generateVariablesTFVars(workspace, workDir); err != nil {
		return fmt.Errorf("failed to write variables.tfvars: %w", err)
	}

	logger.Info("✓ Generated variables.tfvars (%d assignments, %d sensitive)", varCount, sensitiveCount)

	// 只在TRACE级别打印完整内容（脱敏处理）
	varsTFVarsData, _ := os.ReadFile(filepath.Join(workDir, "variables.tfvars"))
	if s.db != nil {
		maskedContent := s.maskSensitiveVariables(string(varsTFVarsData), workspace.WorkspaceID)
		logger.Trace("========== variables.tfvars Content (sensitive values masked) ==========")
		logger.Trace("%s", maskedContent)
		logger.Trace("=========================================================================")
	} else {
		// Agent 模式下跳过脱敏（因为 maskSensitiveVariables 使用 s.db）
		logger.Trace("========== variables.tfvars Content ==========")
		logger.Trace("%s", string(varsTFVarsData))
		logger.Trace("==============================================")
	}

	// 5. 生成 outputs.tf.json（如果有配置outputs）
	if err := s.generateOutputsTFJSONWithLogger(workspace, workDir, logger); err != nil {
		return fmt.Errorf("failed to write outputs.tf.json: %w", err)
	}

	// 6. 生成 remote_data.tf.json（如果有配置远程数据引用）
	if err := s.generateRemoteDataTFJSONWithLogging(workspace, workDir, nil, logger); err != nil {
		return fmt.Errorf("failed to write remote_data.tf.json: %w", err)
	}

	// 检查是否生成了 remote_data.tf.json
	remoteDataFile := filepath.Join(workDir, "remote_data.tf.json")
	if _, err := os.Stat(remoteDataFile); err == nil {
		remoteDataData, _ := os.ReadFile(remoteDataFile)
		logger.Info("✓ Generated remote_data.tf.json (%.1f KB)", float64(len(remoteDataData))/1024)
	} else {
		logger.Debug("No remote data configured, skipping remote_data.tf.json generation")
	}

	return nil
}

// maskSensitiveVariables 脱敏处理敏感变量
func (s *TerraformExecutor) maskSensitiveVariables(content string, workspaceID string) string {
	// 使用 DataAccessor 获取所有敏感变量
	allVars, err := s.dataAccessor.GetWorkspaceVariables(workspaceID, models.VariableTypeTerraform)
	if err != nil {
		log.Printf("Warning: failed to get variables for masking: %v", err)
		return content
	}

	// 过滤出敏感变量
	var sensitiveVars []models.WorkspaceVariable
	for _, v := range allVars {
		if v.Sensitive {
			sensitiveVars = append(sensitiveVars, v)
		}
	}

	// 如果没有敏感变量，直接返回
	if len(sensitiveVars) == 0 {
		return content
	}

	// 对每个敏感变量进行脱敏
	maskedContent := content
	for _, v := range sensitiveVars {
		// 匹配 key = "value" 或 key = value 格式
		// 使用正则表达式替换值部分为 ***SENSITIVE***
		lines := strings.Split(maskedContent, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, v.Key+" =") {
				// 找到敏感变量行，替换值部分
				lines[i] = v.Key + " = \"***SENSITIVE***\""
			}
		}
		maskedContent = strings.Join(lines, "\n")
	}

	return maskedContent
}

// PrepareStateFileWithLogging 准备State文件（带详细日志）
func (s *TerraformExecutor) PrepareStateFileWithLogging(
	workspace *models.Workspace,
	workDir string,
	logger *TerraformLogger,
) error {
	stateFile := filepath.Join(workDir, "terraform.tfstate")

	// 使用 DataAccessor 获取最新的State版本
	stateVersion, err := s.dataAccessor.GetLatestStateVersion(workspace.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get state version: %w", err)
	}

	if stateVersion == nil {
		logger.Info("No existing state found (first run)")
		logger.Info("Terraform will create a new state file after first apply")

		// 删除可能存在的旧state文件（避免格式错误）
		if _, err := os.Stat(stateFile); err == nil {
			logger.Debug("Removing old state file from work directory")
			if err := os.Remove(stateFile); err != nil {
				logger.Warn("Failed to remove old state file: %v", err)
			}
		}

		// 不创建任何state文件，让Terraform在首次运行时自然创建
		return nil
	}

	// 写入State文件
	stateContent, err := json.Marshal(stateVersion.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(stateFile, stateContent, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	logger.Info("✓ Restored state version #%d to terraform.tfstate (%.1f KB)",
		stateVersion.Version, float64(len(stateContent))/1024)

	return nil
}

// TerraformInitWithLogging 执行terraform init（带详细日志、流式输出和重试机制）
func (s *TerraformExecutor) TerraformInitWithLogging(
	ctx context.Context,
	workDir string,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
	logger *TerraformLogger,
) error {
	// 重试配置
	maxRetries := 3
	baseDelay := 5 * time.Second
	maxDelay := 30 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// 计算退避延迟（指数退避）
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > maxDelay {
				delay = maxDelay
			}
			logger.Warn("Terraform init failed (attempt %d/%d), retrying in %v...", attempt, maxRetries, delay)
			time.Sleep(delay)
		}

		err := s.terraformInitOnce(ctx, workDir, task, workspace, logger, attempt+1, maxRetries)
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否是用户取消
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("task cancelled by user")
		}

		// 检查是否是可重试的错误（网络超时、注册表访问失败等）
		if !s.isRetryableInitError(err) {
			logger.Error("Non-retryable error, stopping retry: %v", err)
			return err
		}

		// 注意：不清理 .terraform 目录，保留已下载的部分以加速重试
		// 网络超时错误通常是临时的，已下载的 provider/module 可以复用
		logger.Debug("Preserving .terraform directory for retry (partial downloads may be reused)")
	}

	return fmt.Errorf("terraform init failed after %d attempts: %w", maxRetries, lastErr)
}

// isRetryableInitError 判断是否是可重试的 init 错误
func (s *TerraformExecutor) isRetryableInitError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 网络相关错误
	retryablePatterns := []string{
		"context deadline exceeded",
		"Client.Timeout exceeded",
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"TLS handshake timeout",
		"failed to request discovery document",
		"Error accessing remote module registry",
		"Failed to retrieve available versions",
		"registry.terraform.io",
		"registry.opentofu.org",
		"could not download module",
		"error downloading",
		"network is unreachable",
		"temporary failure in name resolution",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// terraformInitOnce 执行单次 terraform init
func (s *TerraformExecutor) terraformInitOnce(
	ctx context.Context,
	workDir string,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
	logger *TerraformLogger,
	attempt int,
	maxAttempts int,
) error {
	// 获取Terraform二进制文件路径（已在Fetching阶段下载）
	// 必须使用下载的版本，不允许回退到系统terraform
	if s.downloader == nil {
		return fmt.Errorf("terraform downloader not initialized")
	}

	// 使用EnsureTerraformBinary确保二进制文件存在，并获取实际路径
	// 这样可以正确处理"latest"等特殊版本标识
	binaryPath, err := s.downloader.EnsureTerraformBinary(workspace.TerraformVersion)
	if err != nil {
		return fmt.Errorf("failed to ensure terraform binary for version %s: %w", workspace.TerraformVersion, err)
	}

	terraformCmd := binaryPath
	if attempt == 1 {
		logger.Info("Using downloaded terraform binary: %s", terraformCmd)
	}

	// 清理可能存在的 .terraform 目录和后端配置文件（仅首次尝试）
	// 这些文件可能来自之前失败的任务，会导致 terraform init 失败
	if attempt == 1 {
		terraformDir := filepath.Join(workDir, ".terraform")
		if _, err := os.Stat(terraformDir); err == nil {
			logger.Debug("Removing old .terraform directory")
			if err := os.RemoveAll(terraformDir); err != nil {
				logger.Warn("Failed to remove .terraform directory: %v", err)
			}
		}

		// 创建 .terraformrc 配置文件，设置 HTTP 超时为 60 秒
		// 这对于访问慢速网络的 registry 很重要
		terraformrcPath := filepath.Join(workDir, ".terraformrc")
		terraformrcContent := `# Auto-generated by IAC Platform
# HTTP timeout for registry access (default is 10s, which is too short for slow networks)
plugin_cache_may_break_dependency_lock_file = true
`
		if err := os.WriteFile(terraformrcPath, []byte(terraformrcContent), 0644); err != nil {
			logger.Warn("Failed to create .terraformrc: %v", err)
		} else {
			logger.Debug("Created .terraformrc with custom settings")
		}
	}

	// 检查是否已经设置了全局 TF_PLUGIN_CACHE_DIR（优先使用全局缓存）
	globalPluginCacheDir := os.Getenv("TF_PLUGIN_CACHE_DIR")
	pluginCacheDir := ""
	if attempt == 1 {
		logger.Info("Checking TF_PLUGIN_CACHE_DIR environment variable: '%s'", globalPluginCacheDir)
	}
	if globalPluginCacheDir != "" {
		// 使用全局缓存目录
		pluginCacheDir = globalPluginCacheDir
		if attempt == 1 {
			logger.Info("Using global plugin cache directory: %s", pluginCacheDir)
		}
	} else {
		// 没有全局缓存，使用工作目录级别的缓存（随工作目录一起清理）
		pluginCacheDir = filepath.Join(workDir, ".terraform-plugin-cache")
		if err := os.MkdirAll(pluginCacheDir, 0755); err != nil {
			logger.Warn("Failed to create plugin cache dir: %v", err)
			// 不阻塞执行，继续不使用缓存
			pluginCacheDir = ""
		} else if attempt == 1 {
			logger.Info("Using workspace-level plugin cache directory: %s", pluginCacheDir)
		}
	}

	// 判断是否需要使用 -upgrade 参数
	// 只在 provider 配置变更或首次运行时使用 -upgrade，以加速 init 过程
	needUpgrade := s.shouldUseUpgrade(workspace, logger)

	// 构建命令
	args := []string{
		"init",
		"-no-color",
		"-input=false",
	}

	if needUpgrade {
		args = append(args, "-upgrade")
		if attempt == 1 {
			logger.Info("Using -upgrade flag (provider config changed or first run)")
		}
	} else if attempt == 1 {
		logger.Info("Skipping -upgrade flag (provider config unchanged, saving time)")
	}

	cmd := exec.CommandContext(ctx, terraformCmd, args...)
	cmd.Dir = workDir
	cmd.Env = s.buildEnvironmentVariables(workspace)

	// 添加插件缓存目录（仅当不是使用全局缓存时才需要添加）
	// 如果使用全局缓存，buildEnvironmentVariables 已经通过 os.Environ() 包含了 TF_PLUGIN_CACHE_DIR
	if pluginCacheDir != "" && globalPluginCacheDir == "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TF_PLUGIN_CACHE_DIR=%s", pluginCacheDir))
	}

	// 使用Pipe实时捕获输出
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 启动命令 - 显示实际执行的命令
	if attempt > 1 {
		logger.Info("Retry attempt %d/%d: %s %s", attempt, maxAttempts, terraformCmd, strings.Join(args, " "))
	} else {
		logger.Info("Executing: %s %s", terraformCmd, strings.Join(args, " "))
	}
	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		// 检查是否是context取消导致的
		if ctx.Err() == context.Canceled {
			logger.Info("Task cancelled by user during init startup")
			return fmt.Errorf("task cancelled by user")
		}
		return fmt.Errorf("failed to start terraform init: %w", err)
	}

	// 收集输出用于错误分析
	var outputBuffer strings.Builder

	// 实时读取stdout
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			logger.RawOutput(line)
			outputBuffer.WriteString(line)
			outputBuffer.WriteString("\n")
		}
	}()

	// 实时读取stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			logger.RawOutput(line)
			outputBuffer.WriteString(line)
			outputBuffer.WriteString("\n")
		}
	}()

	// 等待命令完成
	cmdErr := cmd.Wait()
	wg.Wait()

	duration := time.Since(startTime)

	if cmdErr != nil {
		// 检查是否是context取消导致的
		if ctx.Err() == context.Canceled {
			logger.Info("Task cancelled by user during init")
			return fmt.Errorf("task cancelled by user")
		}

		// 将输出内容包含在错误中，以便判断是否可重试
		fullOutput := outputBuffer.String()
		logger.Error("Terraform init failed (attempt %d/%d): %v", attempt, maxAttempts, cmdErr)

		// 返回包含输出的错误，以便 isRetryableInitError 可以分析
		return fmt.Errorf("terraform init failed: %w\nOutput:\n%s", cmdErr, fullOutput)
	}

	logger.Info("✓ Terraform initialization completed successfully")
	logger.Info("Initialization time: %.1f seconds", duration.Seconds())

	// 更新 last_init_hash（用于下次判断是否需要 -upgrade）
	if needUpgrade {
		s.updateLastInitHash(workspace, logger)
	}

	// 保存 .terraform.lock.hcl 文件到数据库（用于下次 init 加速）
	s.saveTerraformLockHCL(workDir, workspace.WorkspaceID, logger)

	return nil
}

// SavePlanDataWithLogging 保存Plan数据（带详细日志）
func (s *TerraformExecutor) SavePlanDataWithLogging(
	task *models.WorkspaceTask,
	planFile string,
	planJSON map[string]interface{},
	logger *TerraformLogger,
) {
	planData, err := os.ReadFile(planFile)
	if err != nil {
		logger.Error("Failed to read plan file: %v", err)
		return
	}

	logger.Debug("Plan file size: %.1f KB", float64(len(planData))/1024)
	log.Printf("[CRITICAL] SavePlanDataWithLogging called for task %d, planData size: %d, planJSON exists: %v",
		task.ID, len(planData), planJSON != nil)

	// ========== 进入关键区：Saving Plan ==========
	s.signalManager.EnterCriticalSection("saving_plan")
	defer s.signalManager.ExitCriticalSection("saving_plan")

	// 设置 plan data
	task.PlanData = planData
	task.PlanJSON = planJSON
	log.Printf("[CRITICAL] Set task.PlanData (len=%d) and task.PlanJSON (exists=%v) for task %d",
		len(task.PlanData), task.PlanJSON != nil, task.ID)

	// 在 Local 模式下，直接使用 s.db.Save() 确保大字段被保存
	// 在 Agent 模式下，使用 DataAccessor（但 plan_data/plan_json 不会通过 API 传输）
	maxRetries := 3
	var saveErr error

	for i := 0; i < maxRetries; i++ {
		if s.db != nil {
			// Local 模式：使用 Updates 显式更新 plan_data 和 plan_json
			log.Printf("[CRITICAL] Task %d: Attempting to save plan_data (len=%d) and plan_json (exists=%v) using Updates",
				task.ID, len(planData), planJSON != nil)
			updates := map[string]interface{}{
				"plan_data": planData,
				"plan_json": planJSON,
			}
			saveErr = s.db.Model(&models.WorkspaceTask{}).Where("id = ?", task.ID).Updates(updates).Error
			log.Printf("[CRITICAL] Task %d: Updates result: error=%v", task.ID, saveErr)
		} else {
			// Agent 模式：需要通过 API 上传 plan_data 和 plan_json
			log.Printf("[CRITICAL] Task %d: Agent mode - uploading plan_data (len=%d) and plan_json to server", task.ID, len(planData))

			// 上传 plan_data（Apply 需要用到）
			saveErr = s.uploadPlanData(task.ID, planData)
			if saveErr != nil {
				log.Printf("[CRITICAL] Task %d: Upload plan_data failed: error=%v", task.ID, saveErr)
				continue
			}
			log.Printf("[CRITICAL] Task %d: Upload plan_data success", task.ID)

			// 上传 plan_json（Resource-Changes 和其他分析需要用到）
			if planJSON != nil {
				if err := s.uploadPlanJSON(task.ID, planJSON); err != nil {
					log.Printf("[CRITICAL] Task %d: Upload plan_json failed: error=%v", task.ID, err)
					saveErr = err
					continue
				}
				log.Printf("[CRITICAL] Task %d: Upload plan_json success", task.ID)
			}
		}

		if saveErr == nil {
			logger.Info("✓ Plan saved to database (task #%d)", task.ID)
			logger.Info("  - Plan file size: %.1f KB", float64(len(planData))/1024)
			if planJSON != nil {
				planJSONData, _ := json.Marshal(planJSON)
				logger.Info("  - Plan JSON size: %.1f KB", float64(len(planJSONData))/1024)
			}

			// 验证保存是否成功（仅 Local 模式）
			if s.db != nil {
				var verifyTask models.WorkspaceTask
				if err := s.db.Select("id, plan_data, plan_json").First(&verifyTask, task.ID).Error; err == nil {
					logger.Debug("Verification: plan_data size = %d bytes, plan_json exists = %v",
						len(verifyTask.PlanData), verifyTask.PlanJSON != nil)
					if len(verifyTask.PlanData) > 0 && verifyTask.PlanJSON != nil {
						logger.Info("✓ Verification passed: plan_data and plan_json saved successfully")
					} else {
						logger.Error("✗ Verification failed: plan_data size = %d, plan_json exists = %v",
							len(verifyTask.PlanData), verifyTask.PlanJSON != nil)
					}
				}
			}
			return
		}

		logger.Warn("Failed to save plan data (attempt %d/%d): %v", i+1, maxRetries, saveErr)

		if i < maxRetries-1 {
			time.Sleep(time.Second)
		}
	}

	// 保存失败 - 告警但不阻塞
	logger.Error("Failed to save plan data after %d retries", maxRetries)
}

// SaveNewStateVersionWithLogging 保存新的State版本（带详细日志）
func (s *TerraformExecutor) SaveNewStateVersionWithLogging(
	workspace *models.Workspace,
	task *models.WorkspaceTask,
	workDir string,
	logger *TerraformLogger,
) error {
	stateFile := filepath.Join(workDir, "terraform.tfstate")

	// 1. 读取State文件
	stateData, err := os.ReadFile(stateFile)
	if err != nil {
		logger.Error("Failed to read state file: %v", err)
		return fmt.Errorf("failed to read state file: %w", err)
	}
	logger.Info("✓ State file read successfully (%.1f KB)", float64(len(stateData))/1024)

	// 2. 解析State内容
	logger.Info("Parsing state content...")
	var stateContent map[string]interface{}
	if err := json.Unmarshal(stateData, &stateContent); err != nil {
		logger.Error("Failed to parse state: %v", err)
		return fmt.Errorf("failed to parse state: %w", err)
	}

	// 提取State信息
	if version, ok := stateContent["version"].(float64); ok {
		logger.Info("✓ State version: %.0f", version)
	}
	if tfVersion, ok := stateContent["terraform_version"].(string); ok {
		logger.Info("✓ Terraform version: %s", tfVersion)
	}
	if resources, ok := stateContent["resources"].([]interface{}); ok {
		logger.Info("✓ Resources count: %d", len(resources))
	}
	if outputs, ok := stateContent["outputs"].(map[string]interface{}); ok {
		logger.Info("✓ Outputs count: %d", len(outputs))
	}

	// 3. 计算checksum
	logger.Info("Calculating checksum...")
	checksum := s.calculateChecksum(stateData)
	logger.Info("✓ Checksum: %s", checksum[:16]+"...")

	// 4. 立即备份到文件系统（第一道保险）
	backupDir := "/var/backup/states"
	os.MkdirAll(backupDir, 0700)
	backupPath := filepath.Join(backupDir,
		fmt.Sprintf("ws_%s_task_%d_%d.tfstate",
			workspace.WorkspaceID, task.ID, time.Now().Unix()))

	if err := os.WriteFile(backupPath, stateData, 0600); err != nil {
		logger.Warn("Failed to backup state to file: %v", err)
	} else {
		logger.Debug("State backed up to: %s", backupPath)
	}

	// 5. 保存到数据库（带重试）
	logger.Info("Saving to database...")
	logger.Debug("Fetching max state version for workspace %s", workspace.WorkspaceID)

	maxVersion, err := s.dataAccessor.GetMaxStateVersion(workspace.WorkspaceID)
	if err != nil {
		logger.Error("Failed to get max state version: %v", err)
		return fmt.Errorf("failed to get max state version: %w", err)
	}

	newVersion := maxVersion + 1
	logger.Info("✓ Current max version: %d", maxVersion)
	logger.Info("✓ Creating new version: %d", newVersion)

	maxRetries := 5
	var saveErr error

	for i := 0; i < maxRetries; i++ {
		saveErr = s.SaveStateToDatabase(workspace, task, stateData)
		if saveErr == nil {
			logger.Info("✓ State version #%d created successfully", newVersion)
			logger.Info("✓ Updated workspace current_state_id")
			logger.Info("")
			logger.Info("State save completed successfully")
			logger.Info("Version: %d", newVersion)
			logger.Info("Size: %.1f KB", float64(len(stateData))/1024)
			logger.Info("Checksum: %s", checksum[:16]+"...")
			return nil
		}

		logger.Warn("Failed to save state (attempt %d/%d): %v", i+1, maxRetries, saveErr)

		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
		}
	}

	// 所有重试失败
	logger.Error("CRITICAL: Failed to save state after %d retries", maxRetries)
	logger.Error("State backed up to: %s", backupPath)

	// 自动锁定workspace
	lockErr := s.lockWorkspace(
		workspace.WorkspaceID, // 保持使用内部数字ID
		*task.CreatedBy,
		fmt.Sprintf("Auto-locked: State save failed for task %d. State backed up to %s",
			task.ID, backupPath),
	)
	if lockErr != nil {
		logger.Error("Failed to auto-lock workspace: %v", lockErr)
	} else {
		logger.Info("Workspace auto-locked for safety")
	}

	return fmt.Errorf("state save failed, workspace locked, backup at: %s", backupPath)
}

// extractTerraformOutputs 提取terraform outputs
func (s *TerraformExecutor) extractTerraformOutputs(
	ctx context.Context,
	workDir string,
) (map[string]interface{}, error) {
	terraformCmd := "terraform"
	if s.downloader != nil {
		// 使用系统terraform即可，因为已经在工作目录中初始化
		terraformCmd = "terraform"
	}

	cmd := exec.CommandContext(ctx, terraformCmd, "output", "-json")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var outputs map[string]interface{}
	if err := json.Unmarshal(output, &outputs); err != nil {
		return nil, err
	}

	return outputs, nil
}

// ============================================================================
// Agent 模式资源变更解析
// ============================================================================

// parseResourceChangesFromPlanJSON 从 plan_json 解析资源变更
func (s *TerraformExecutor) parseResourceChangesFromPlanJSON(planJSON map[string]interface{}) []map[string]interface{} {
	var resourceChanges []map[string]interface{}

	changes, ok := planJSON["resource_changes"].([]interface{})
	if !ok {
		return resourceChanges
	}

	for _, item := range changes {
		rc, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		change, ok := rc["change"].(map[string]interface{})
		if !ok {
			continue
		}

		actions, ok := change["actions"].([]interface{})
		if !ok {
			continue
		}

		// 忽略 no-op
		if len(actions) == 1 {
			if actionStr, ok := actions[0].(string); ok && actionStr == "no-op" {
				continue
			}
		}

		// 判断操作类型
		action := s.determineAction(actions)

		// 构造资源变更对象
		// 直接获取字符串值，避免函数名冲突
		getStr := func(m map[string]interface{}, key string) string {
			if val, ok := m[key]; ok {
				if str, ok := val.(string); ok {
					return str
				}
			}
			return ""
		}

		resourceChange := map[string]interface{}{
			"resource_address": getStr(rc, "address"),
			"resource_type":    getStr(rc, "type"),
			"resource_name":    getStr(rc, "name"),
			"module_address":   getStr(rc, "module_address"),
			"action":           action,
			"changes_before":   change["before"],
			"changes_after":    change["after"],
		}

		resourceChanges = append(resourceChanges, resourceChange)
	}

	return resourceChanges
}

// determineAction 判断操作类型
func (s *TerraformExecutor) determineAction(actions []interface{}) string {
	if len(actions) == 1 {
		if action, ok := actions[0].(string); ok {
			return action
		}
	}

	// ["delete", "create"] = replace
	if len(actions) == 2 {
		action0, ok0 := actions[0].(string)
		action1, ok1 := actions[1].(string)
		if ok0 && ok1 && action0 == "delete" && action1 == "create" {
			return "replace"
		}
	}

	return "unknown"
}

// uploadResourceChanges 上传资源变更到服务器（Agent 模式）
func (s *TerraformExecutor) uploadResourceChanges(taskID uint, resourceChanges []map[string]interface{}) error {
	log.Printf("[Agent Mode] Uploading %d resource changes for task %d", len(resourceChanges), taskID)

	// 通过 dataAccessor 调用 API
	if remoteAccessor, ok := s.dataAccessor.(*RemoteDataAccessor); ok {
		// 调用 AgentAPIClient 的 UploadResourceChanges 方法
		return remoteAccessor.apiClient.UploadResourceChanges(taskID, resourceChanges)
	}

	return fmt.Errorf("not in agent mode")
}

// uploadPlanData 上传 plan_data 到服务器（Agent 模式）
func (s *TerraformExecutor) uploadPlanData(taskID uint, planData []byte) error {
	// 通过 dataAccessor 调用 API
	if remoteAccessor, ok := s.dataAccessor.(*RemoteDataAccessor); ok {
		// 使用 base64 编码传输二进制数据
		encodedData := base64.StdEncoding.EncodeToString(planData)

		// 调用 API
		return remoteAccessor.apiClient.UploadPlanData(taskID, encodedData)
	}

	return fmt.Errorf("not in agent mode")
}

// uploadPlanJSON 上传 plan_json 到服务器（Agent 模式）
func (s *TerraformExecutor) uploadPlanJSON(taskID uint, planJSON map[string]interface{}) error {
	// 通过 dataAccessor 调用 API
	if remoteAccessor, ok := s.dataAccessor.(*RemoteDataAccessor); ok {
		// 调用 API
		return remoteAccessor.apiClient.UploadPlanJSON(taskID, planJSON)
	}

	return fmt.Errorf("not in agent mode")
}

// ============================================================================
// 快照相关辅助方法
// ============================================================================

// ValidateResourceVersionSnapshot 验证资源版本快照
func (s *TerraformExecutor) ValidateResourceVersionSnapshot(
	planTask *models.WorkspaceTask,
	logger *TerraformLogger,
) error {
	if planTask.SnapshotCreatedAt == nil {
		return fmt.Errorf("no snapshot data (snapshot_created_at is nil)")
	}

	// 允许空的资源版本快照（workspace可能没有资源）
	if planTask.SnapshotResourceVersions == nil {
		return fmt.Errorf("snapshot resource versions is nil (should be empty map if no resources)")
	}

	// 验证 SnapshotVariables - 支持多种格式:
	// 1. nil - 错误
	// 2. {"_array": [...]} - 正常格式
	// 3. [...] - 旧格式（直接数组）
	// 4. {} - 空对象，允许（workspace可能没有变量）
	if planTask.SnapshotVariables == nil {
		return fmt.Errorf("snapshot variables missing (nil)")
	}

	// 检查是否是空 map（允许，因为 workspace 可能没有变量）
	if len(planTask.SnapshotVariables) == 0 {
		logger.Debug("Snapshot variables is empty map (workspace may have no variables)")
	} else {
		// 检查 _array 格式
		if arrayData, hasArray := planTask.SnapshotVariables["_array"]; hasArray {
			// 检查 _array 是否为空
			switch arr := arrayData.(type) {
			case []interface{}:
				logger.Debug("Snapshot variables has %d items in _array format", len(arr))
			case []models.WorkspaceVariable:
				logger.Debug("Snapshot variables has %d items in _array format (WorkspaceVariable)", len(arr))
			default:
				logger.Debug("Snapshot variables _array has unknown type: %T", arrayData)
			}
		} else {
			// 可能是旧格式或其他格式
			logger.Debug("Snapshot variables format: %d keys", len(planTask.SnapshotVariables))
		}
	}

	if planTask.SnapshotProviderConfig == nil {
		return fmt.Errorf("snapshot provider config missing")
	}

	logger.Debug("Snapshot validation:")
	logger.Debug("  - Resources: %d", len(planTask.SnapshotResourceVersions))
	logger.Debug("  - Variables: %d", len(planTask.SnapshotVariables))
	logger.Debug("  - Provider config: %v", planTask.SnapshotProviderConfig != nil)
	logger.Debug("  - Created at: %s", planTask.SnapshotCreatedAt.Format("2006-01-02 15:04:05"))

	// 如果workspace没有资源，记录日志但不报错
	if len(planTask.SnapshotResourceVersions) == 0 {
		logger.Info("Workspace has no resources (empty snapshot is valid)")
	}

	// 可选：检查快照是否过期（例如超过24小时）
	snapshotAge := time.Since(*planTask.SnapshotCreatedAt)
	if snapshotAge > 24*time.Hour {
		logger.Warn("Snapshot is old (created %v ago)", snapshotAge)
	}

	// 验证所有资源版本是否仍然存在
	for resourceID, versionInfo := range planTask.SnapshotResourceVersions {
		versionMap, ok := versionInfo.(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid version info format for resource %s", resourceID)
		}

		// 【修复】使用 resource_db_id 和 version 字段
		resourceDBID := uint(versionMap["resource_db_id"].(float64))
		expectedVersion := int(versionMap["version"].(float64))

		// 直接用 resource_db_id 查询资源
		var resource models.WorkspaceResource
		if s.db != nil {
			if err := s.db.First(&resource, resourceDBID).Error; err != nil {
				return fmt.Errorf("resource %s (db_id=%d) not found: %w", resourceID, resourceDBID, err)
			}
		} else {
			// Agent 模式下无法直接查询数据库，跳过验证（在 GetPlanTask 时已验证）
			logger.Debug("Skipping resource validation in Agent mode for %s", resourceID)
			continue
		}

		// 检查版本是否存在
		var count int64
		if err := s.db.Model(&models.ResourceCodeVersion{}).
			Where("resource_id = ? AND version = ?", resourceDBID, expectedVersion).
			Count(&count).Error; err != nil {
			return fmt.Errorf("failed to check resource %s version %d: %w", resourceID, expectedVersion, err)
		}
		if count == 0 {
			return fmt.Errorf("resource %s version %d no longer exists", resourceID, expectedVersion)
		}
	}

	logger.Debug("All resource versions validated successfully")
	return nil
}

// GetResourcesByVersionSnapshot 根据版本快照获取资源配置
func (s *TerraformExecutor) GetResourcesByVersionSnapshot(
	snapshotVersions map[string]interface{},
	logger *TerraformLogger,
) ([]models.WorkspaceResource, error) {
	var resources []models.WorkspaceResource

	logger.Debug("Loading %d resources from snapshot...", len(snapshotVersions))

	for resourceID, versionInfo := range snapshotVersions {
		versionMap, ok := versionInfo.(map[string]interface{})
		if !ok {
			logger.Warn("Invalid version info format for resource %s, skipping", resourceID)
			continue
		}

		// 【修复】使用 resource_db_id 和 version 字段
		resourceDBID := uint(versionMap["resource_db_id"].(float64))
		expectedVersion := int(versionMap["version"].(float64))

		logger.Debug("Loading resource %s (db_id=%d) version %d...", resourceID, resourceDBID, expectedVersion)

		// 直接用 resource_db_id 查询资源
		var resource models.WorkspaceResource
		if s.db != nil {
			// Local 模式：直接查询数据库
			if err := s.db.First(&resource, resourceDBID).Error; err != nil {
				return nil, fmt.Errorf("failed to get resource %s (db_id=%d): %w", resourceID, resourceDBID, err)
			}

			// 获取指定版本的代码
			var codeVersion models.ResourceCodeVersion
			if err := s.db.Where("resource_id = ? AND version = ?", resourceDBID, expectedVersion).
				First(&codeVersion).Error; err != nil {
				return nil, fmt.Errorf("failed to get resource %s version %d: %w", resourceID, expectedVersion, err)
			}

			resource.CurrentVersion = &codeVersion
			resource.CurrentVersionID = &codeVersion.ID
		} else {
			// Agent 模式：从 RemoteDataAccessor 的缓存中获取（已在 GetPlanTask 时加载）
			remoteAccessor, ok := s.dataAccessor.(*RemoteDataAccessor)
			if !ok {
				return nil, fmt.Errorf("invalid data accessor type in agent mode")
			}

			// 从 snapshot_resources 缓存中查找
			resourcePtr, err := remoteAccessor.GetResourceByVersion(resourceID, expectedVersion)
			if err != nil {
				return nil, fmt.Errorf("failed to get resource %s version %d from cache: %w", resourceID, expectedVersion, err)
			}
			resource = *resourcePtr
		}

		// 验证版本号是否匹配
		if resource.CurrentVersion.Version != expectedVersion {
			return nil, fmt.Errorf("resource %s version mismatch: expected v%d, got v%d",
				resourceID, expectedVersion, resource.CurrentVersion.Version)
		}

		logger.Debug("✓ Loaded resource %s (version %d)", resourceID, resource.CurrentVersion.Version)
		resources = append(resources, resource)
	}

	logger.Debug("Successfully loaded all %d resources from snapshot", len(resources))
	return resources, nil
}

// ResolveVariableSnapshots 解析变量快照引用为实际变量值
// 支持新旧两种格式以保持向后兼容
func (s *TerraformExecutor) ResolveVariableSnapshots(
	snapshotData interface{},
	workspaceID string,
) ([]models.WorkspaceVariable, error) {
	// 处理nil情况
	if snapshotData == nil {
		return []models.WorkspaceVariable{}, nil
	}

	// 尝试将snapshotData转换为JSON
	snapshotBytes, err := json.Marshal(snapshotData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot data: %w", err)
	}

	// 先尝试解析为map数组格式（新格式，包含完整变量数据）
	var snapshots []map[string]interface{}
	if err := json.Unmarshal(snapshotBytes, &snapshots); err == nil && len(snapshots) > 0 {
		// 检查是否是map格式（有variable_id字段）
		if len(snapshots) > 0 {
			firstSnap := snapshots[0]
			// 新格式的特征：有variable_id字段
			if variableID, hasVarID := firstSnap["variable_id"].(string); hasVarID && variableID != "" {
				// 检查是否包含完整数据（有key字段）
				if keyVal, hasKey := firstSnap["key"]; hasKey && keyVal != nil {
					// 新格式（完整数据）：直接从map构建WorkspaceVariable对象
					log.Printf("[DEBUG] Detected new snapshot format with %d variables (full data)", len(snapshots))

					variables := make([]models.WorkspaceVariable, 0, len(snapshots))
					for _, snap := range snapshots {
						// 从map构建WorkspaceVariable
						variable := models.WorkspaceVariable{
							WorkspaceID:  getString(snap, "workspace_id"),
							VariableID:   getString(snap, "variable_id"),
							Version:      getInt(snap, "version"),
							VariableType: models.VariableType(getString(snap, "variable_type")),
							Key:          getString(snap, "key"),
							Value:        getString(snap, "value"),
							Sensitive:    getBool(snap, "sensitive"),
							Description:  getString(snap, "description"),
							ValueFormat:  models.ValueFormat(getString(snap, "value_format")),
						}

						// 验证key不为空
						if variable.Key == "" {
							log.Printf("[WARN] Variable %s version %d has empty key in snapshot",
								variable.VariableID, variable.Version)
							continue
						}

						variables = append(variables, variable)
						log.Printf("[DEBUG] Loaded variable from snapshot: id=%s, version=%d, key=%s, type=%s",
							variable.VariableID, variable.Version, variable.Key, variable.VariableType)
					}

					log.Printf("[DEBUG] Resolved %d variables from new snapshot format (full data)", len(variables))
					return variables, nil
				} else {
					// 旧的引用格式：需要从数据库查询
					log.Printf("[DEBUG] Detected old reference format with %d variable references", len(snapshots))

					// 使用map去重，确保每个variable_id只查询一次
					variableMap := make(map[string]models.WorkspaceVariable)

					for _, snap := range snapshots {
						varID := snap["variable_id"].(string)
						version := int(snap["version"].(float64))

						// 构建唯一key
						key := fmt.Sprintf("%s-v%d", varID, version)

						// 如果已经查询过，跳过
						if _, exists := variableMap[key]; exists {
							continue
						}

						// 从数据库查询实际变量数据
						var variable models.WorkspaceVariable
						var err error

						if s.db != nil {
							// Local模式：直接查询数据库，显式选择所有字段
							err = s.db.Select("*").
								Where("variable_id = ? AND version = ?", varID, version).
								First(&variable).Error
						} else {
							// Agent模式：变量数据应该已经在GetPlanTask时通过API获取
							// 在Agent模式下，快照变量应该已经是完整数据（旧格式）
							// 如果走到这里说明数据格式有问题
							err = fmt.Errorf("agent mode should not use new snapshot format with references only")
						}

						if err != nil {
							return nil, fmt.Errorf("variable %s version %d not found: %w", varID, version, err)
						}

						// 验证key字段不为空
						if variable.Key == "" {
							return nil, fmt.Errorf("variable %s version %d has empty key field (workspace_id=%s)",
								varID, version, variable.WorkspaceID)
						}

						log.Printf("[DEBUG] Loaded variable from DB: id=%s, version=%d, key=%s, type=%s",
							variable.VariableID, variable.Version, variable.Key, variable.VariableType)

						variableMap[key] = variable
					}

					// 转换为数组
					variables := make([]models.WorkspaceVariable, 0, len(variableMap))
					for _, v := range variableMap {
						variables = append(variables, v)
					}

					log.Printf("[DEBUG] Resolved %d unique variables from %d snapshot references", len(variables), len(snapshots))
					return variables, nil
				}
			}
		}
	}

	// 尝试解析为旧格式（完整的WorkspaceVariable数组）
	var variables []models.WorkspaceVariable
	if err := json.Unmarshal(snapshotBytes, &variables); err == nil {
		// 旧格式：直接返回，但也要去重
		log.Printf("[DEBUG] Using %d variables from old snapshot format", len(variables))

		// 使用map去重
		variableMap := make(map[string]models.WorkspaceVariable)
		for _, v := range variables {
			key := fmt.Sprintf("%s-v%d", v.VariableID, v.Version)
			variableMap[key] = v
		}

		// 转换回数组
		uniqueVariables := make([]models.WorkspaceVariable, 0, len(variableMap))
		for _, v := range variableMap {
			uniqueVariables = append(uniqueVariables, v)
		}

		if len(uniqueVariables) < len(variables) {
			log.Printf("[DEBUG] Deduplicated variables: %d -> %d", len(variables), len(uniqueVariables))
		}

		return uniqueVariables, nil
	}

	return nil, fmt.Errorf("failed to parse snapshot data in any known format")
}

// generateOutputsTFJSONWithLogger 生成outputs.tf.json（带日志）
func (s *TerraformExecutor) generateOutputsTFJSONWithLogger(
	workspace *models.Workspace,
	workDir string,
	logger *TerraformLogger,
) error {
	var outputs []models.WorkspaceOutput
	var err error

	// 根据模式获取outputs配置
	if s.db != nil {
		// Local 模式：直接查询数据库
		if err := s.db.Where("workspace_id = ?", workspace.WorkspaceID).Find(&outputs).Error; err != nil {
			logger.Warn("Failed to get outputs: %v", err)
			return nil
		}
		logger.Debug("Local mode: found %d outputs for workspace %s", len(outputs), workspace.WorkspaceID)
	} else {
		// Agent 模式：使用 RemoteDataAccessor 从缓存获取
		if remoteAccessor, ok := s.dataAccessor.(*RemoteDataAccessor); ok {
			outputs, err = remoteAccessor.GetWorkspaceOutputs(workspace.WorkspaceID)
			if err != nil {
				logger.Warn("Failed to get outputs in Agent mode: %v", err)
				return nil
			}
			logger.Debug("Agent mode: found %d outputs for workspace %s", len(outputs), workspace.WorkspaceID)
		} else {
			logger.Warn("dataAccessor is not RemoteDataAccessor in Agent mode")
			return nil
		}
	}

	// 如果没有outputs配置，不生成文件
	if len(outputs) == 0 {
		logger.Debug("No outputs configured, skipping outputs.tf.json generation")
		return nil
	}

	// 获取活跃资源列表，用于过滤已删除资源的 outputs
	activeResourceNames := make(map[string]bool)
	if s.db != nil {
		var activeResources []models.WorkspaceResource
		if err := s.db.Where("workspace_id = ? AND is_active = ?", workspace.WorkspaceID, true).
			Select("resource_name").Find(&activeResources).Error; err != nil {
			logger.Warn("Failed to get active resources for output filtering: %v", err)
		} else {
			for _, r := range activeResources {
				activeResourceNames[r.ResourceName] = true
			}
		}
	} else {
		resources, err := s.dataAccessor.GetWorkspaceResources(workspace.WorkspaceID)
		if err != nil {
			logger.Warn("Failed to get resources for output filtering in Agent mode: %v", err)
		} else {
			for _, r := range resources {
				if r.IsActive {
					activeResourceNames[r.ResourceName] = true
				}
			}
		}
	}

	// 构建outputs配置
	outputsConfig := make(map[string]interface{})
	skippedCount := 0
	for _, output := range outputs {
		isStaticOutput := output.IsStaticOutput()

		if !isStaticOutput {
			if len(activeResourceNames) > 0 && !activeResourceNames[output.ResourceName] {
				logger.Debug("Skipping output %s for deleted resource %s", output.OutputName, output.ResourceName)
				skippedCount++
				continue
			}
		}

		var outputDef map[string]interface{}
		var outputKey string

		if isStaticOutput {
			outputDef = map[string]interface{}{
				"value": output.OutputValue,
			}
			outputKey = fmt.Sprintf("static-%s", output.OutputName)
			logger.Debug("Generated static output key: %s (value=%s)", outputKey, output.OutputValue)
		} else {
			outputDef = map[string]interface{}{
				"value": fmt.Sprintf("${%s}", output.OutputValue),
			}

			moduleName := output.ResourceName
			if strings.HasPrefix(output.OutputValue, "module.") {
				parts := strings.Split(output.OutputValue, ".")
				if len(parts) >= 2 {
					moduleName = parts[1]
				}
			}

			outputKey = fmt.Sprintf("%s-%s", moduleName, output.OutputName)
			logger.Debug("Generated output key: %s (resource_name=%s)", outputKey, output.ResourceName)
		}

		if output.Description != "" {
			outputDef["description"] = output.Description
		}

		if output.Sensitive {
			outputDef["sensitive"] = true
		}

		outputsConfig[outputKey] = outputDef
	}

	if len(outputsConfig) == 0 {
		if skippedCount > 0 {
			logger.Debug("All %d outputs were skipped (associated resources deleted)", skippedCount)
		}
		return nil
	}

	config := map[string]interface{}{
		"output": outputsConfig,
	}

	if err := s.writeJSONFile(workDir, "outputs.tf.json", config); err != nil {
		return fmt.Errorf("failed to write outputs.tf.json: %w", err)
	}

	if skippedCount > 0 {
		logger.Info("✓ Generated outputs.tf.json (%d outputs, skipped %d orphaned)", len(outputsConfig), skippedCount)
	} else {
		logger.Info("✓ Generated outputs.tf.json (%d outputs)", len(outputsConfig))
	}
	return nil
}

// generateRemoteDataTFJSON 生成remote_data.tf.json（如果有配置远程数据引用）
func (s *TerraformExecutor) generateRemoteDataTFJSON(
	workspace *models.Workspace,
	workDir string,
	taskID *uint,
) error {
	// 只在 Local 模式下生成（需要访问数据库）
	if s.db == nil {
		log.Printf("Skipping remote_data.tf.json generation in Agent mode")
		return nil
	}

	// 获取 baseURL
	platformConfigService := NewPlatformConfigService(s.db)
	baseURL := platformConfigService.GetBaseURL()

	// 创建 RemoteDataTFGenerator
	generator := NewRemoteDataTFGenerator(s.db, baseURL)

	// 创建一个简单的 logger（用于非流式输出场景）
	// 这里我们使用 log.Printf 作为后备
	logger := &TerraformLogger{
		logLevel: LogLevelInfo,
	}

	// 生成 remote_data.tf.json
	return generator.GenerateRemoteDataTFWithLogging(workspace.WorkspaceID, workDir, taskID, logger)
}

// generateRemoteDataTFJSONWithLogging 生成remote_data.tf.json（带日志）
func (s *TerraformExecutor) generateRemoteDataTFJSONWithLogging(
	workspace *models.Workspace,
	workDir string,
	taskID *uint,
	logger *TerraformLogger,
) error {
	// Local 模式：使用 RemoteDataTFGenerator 生成
	if s.db != nil {
		// 获取 baseURL
		platformConfigService := NewPlatformConfigService(s.db)
		baseURL := platformConfigService.GetBaseURL()

		// 创建 RemoteDataTFGenerator
		generator := NewRemoteDataTFGenerator(s.db, baseURL)

		// 生成 remote_data.tf.json
		return generator.GenerateRemoteDataTFWithLogging(workspace.WorkspaceID, workDir, taskID, logger)
	}

	// Agent 模式：从 RemoteDataAccessor 获取已生成好token的配置
	remoteAccessor, ok := s.dataAccessor.(*RemoteDataAccessor)
	if !ok {
		logger.Debug("Skipping remote_data.tf.json generation: not in Agent mode")
		return nil
	}

	// 获取服务端已经准备好的 remote data 配置
	remoteDataConfig := remoteAccessor.GetRemoteDataConfig()
	if len(remoteDataConfig) == 0 {
		logger.Debug("No remote data configured, skipping remote_data.tf.json generation")
		return nil
	}

	logger.Info("Generating remote_data.tf.json with %d remote data references (Agent mode)...", len(remoteDataConfig))

	// 构建TF配置
	tfConfig := make(map[string]interface{})
	dataBlocks := make(map[string]interface{})
	localBlocks := make(map[string]interface{})

	for _, rd := range remoteDataConfig {
		dataName := getString(rd, "data_name")
		token := getString(rd, "token")
		url := getString(rd, "url")
		sourceWorkspaceID := getString(rd, "source_workspace_id")

		if dataName == "" || token == "" || url == "" {
			logger.Warn("Invalid remote data config, skipping: %v", rd)
			continue
		}

		// 生成data "http" block
		dataBlockName := fmt.Sprintf("remote_%s", sanitizeNameForTF(dataName))

		dataBlocks[dataBlockName] = []map[string]interface{}{
			{
				"url": url,
				"request_headers": map[string]interface{}{
					"Authorization": fmt.Sprintf("Bearer %s", token),
				},
			},
		}

		// 生成local block
		localBlocks[dataName] = fmt.Sprintf("${jsondecode(data.http.%s.response_body).outputs}", dataBlockName)

		logger.Info("✓ Added remote data reference: %s -> %s", dataName, sourceWorkspaceID)
	}

	// 只有当有有效的data blocks时才生成文件
	if len(dataBlocks) == 0 {
		logger.Warn("No valid remote data blocks generated")
		return nil
	}

	// 构建完整的TF配置
	tfConfig["data"] = map[string]interface{}{
		"http": dataBlocks,
	}

	if len(localBlocks) > 0 {
		tfConfig["locals"] = localBlocks
	}

	// 写入文件
	filePath := filepath.Join(workDir, "remote_data.tf.json")
	content, err := json.MarshalIndent(tfConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal remote_data.tf.json: %w", err)
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return fmt.Errorf("failed to write remote_data.tf.json: %w", err)
	}

	logger.Info("✓ Generated remote_data.tf.json (%.1f KB)", float64(len(content))/1024)
	return nil
}

// shouldUseUpgrade 判断是否需要使用 -upgrade 参数
// 只在以下情况使用 -upgrade：
// 1. provider_config 发生变更（hash 不匹配）
// 2. terraform 版本发生变更
// 注意：首次运行不需要 -upgrade，terraform init 会自动下载所需的 provider
func (s *TerraformExecutor) shouldUseUpgrade(workspace *models.Workspace, logger *TerraformLogger) bool {
	// 如果 provider_config_hash 为空，说明还没有计算过 hash
	// 这种情况下不需要 -upgrade，让 terraform 自动下载
	if workspace.ProviderConfigHash == "" {
		logger.Debug("Provider config hash not calculated yet, skipping -upgrade")
		return false
	}

	// 如果没有 last_init_hash，说明是首次运行或旧数据
	// 首次运行不需要 -upgrade，terraform init 会自动下载所需的 provider
	if workspace.LastInitHash == "" {
		logger.Debug("First run detected (last_init_hash is empty), skipping -upgrade")
		return false
	}

	// 比较 hash - 只有当配置变更时才需要 -upgrade
	if workspace.ProviderConfigHash != workspace.LastInitHash {
		logger.Debug("Provider config changed: current=%s, last_init=%s",
			workspace.ProviderConfigHash[:16]+"...", workspace.LastInitHash[:16]+"...")
		return true
	}

	// 检查 terraform 版本是否变更
	if workspace.LastInitTerraformVersion != "" && workspace.LastInitTerraformVersion != workspace.TerraformVersion {
		logger.Debug("Terraform version changed: current=%s, last_init=%s",
			workspace.TerraformVersion, workspace.LastInitTerraformVersion)
		return true
	}

	// 没有变更，可以跳过 -upgrade
	logger.Debug("No provider config or terraform version changes detected")
	return false
}

// updateLastInitHash 更新 last_init_hash（在 init 成功后调用）
// 支持 Local、Agent、K8s Agent 三种模式
func (s *TerraformExecutor) updateLastInitHash(workspace *models.Workspace, logger *TerraformLogger) {
	if workspace.ProviderConfigHash == "" {
		logger.Debug("Skipping last_init_hash update (provider_config_hash is empty)")
		return
	}

	updates := map[string]interface{}{
		"last_init_hash":              workspace.ProviderConfigHash,
		"last_init_terraform_version": workspace.TerraformVersion,
	}

	if s.db != nil {
		// Local 模式：直接更新数据库
		if err := s.db.Model(&models.Workspace{}).
			Where("workspace_id = ?", workspace.WorkspaceID).
			Updates(updates).Error; err != nil {
			logger.Warn("Failed to update last_init_hash: %v", err)
		} else {
			logger.Debug("Updated last_init_hash to %s", workspace.ProviderConfigHash[:16]+"...")
		}
	} else if s.dataAccessor != nil {
		// Agent/K8s Agent 模式：通过 DataAccessor 更新
		if err := s.dataAccessor.UpdateWorkspaceFields(workspace.WorkspaceID, updates); err != nil {
			logger.Warn("Failed to update last_init_hash in Agent mode: %v", err)
		} else {
			logger.Debug("Updated last_init_hash to %s (Agent mode)", workspace.ProviderConfigHash[:16]+"...")
		}
	}
}

// isOpenTofuVersion 检测版本是否为 OpenTofu
// 通过查询数据库中的版本配置来判断
func (s *TerraformExecutor) isOpenTofuVersion(version string) bool {
	if s.db == nil {
		// Agent 模式：从 downloader 获取版本信息
		if s.downloader != nil {
			return s.downloader.IsOpenTofuVersion(version)
		}
		return false
	}

	// Local 模式：查询数据库
	var tfVersion models.TerraformVersion
	if err := s.db.Where("version = ?", version).First(&tfVersion).Error; err != nil {
		return false
	}

	return tfVersion.GetEngineType() == models.IaCEngineOpenTofu
}

// removeTargetArgs 从参数数组中移除所有 --target 相关参数
// 用于 Drift Check 任务，需要检查所有资源，不受 --target 限制
func removeTargetArgs(args []string) []string {
	result := make([]string, 0, len(args))
	skipNext := false

	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		// 跳过 --target=xxx 格式
		if strings.HasPrefix(arg, "--target=") || strings.HasPrefix(arg, "-target=") {
			continue
		}

		// 跳过 --target xxx 格式（需要跳过下一个参数）
		if arg == "--target" || arg == "-target" {
			if i+1 < len(args) {
				skipNext = true
			}
			continue
		}

		result = append(result, arg)
	}

	return result
}

// sanitizeNameForTF 清理名称，使其符合Terraform命名规范
func sanitizeNameForTF(name string) string {
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, name)

	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}

	return result
}

// GenerateConfigFilesFromSnapshot 从快照数据生成配置文件
func (s *TerraformExecutor) GenerateConfigFilesFromSnapshot(
	workspace *models.Workspace,
	resources []models.WorkspaceResource,
	variableSnapshots interface{}, // 改为interface{}以支持两种格式
	workDir string,
	logger *TerraformLogger,
) error {
	logger.Debug("Generating config files from snapshot data...")

	// 解析变量快照为实际变量值
	snapshotVariables, err := s.ResolveVariableSnapshots(variableSnapshots, workspace.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to resolve variable snapshots: %w", err)
	}
	logger.Debug("Resolved %d variables from snapshot", len(snapshotVariables))

	// 1. 生成 main.tf.json（从快照的资源列表）
	mainTF, err := s.generateMainTFFromResources(resources)
	if err != nil {
		return fmt.Errorf("failed to generate main.tf from snapshot: %w", err)
	}

	if err := s.writeJSONFile(workDir, "main.tf.json", mainTF); err != nil {
		return fmt.Errorf("failed to write main.tf.json: %w", err)
	}

	mainTFData, _ := json.MarshalIndent(mainTF, "", "  ")
	logger.Info("✓ Generated main.tf.json from snapshot (%.1f KB)", float64(len(mainTFData))/1024)

	// 2. 生成 provider.tf.json（从快照）
	// 清理空的terraform块，避免Terraform尝试读取不存在的backend state
	cleanedProviderConfig := s.cleanProviderConfig(workspace.ProviderConfig)
	if err := s.writeJSONFile(workDir, "provider.tf.json", cleanedProviderConfig); err != nil {
		return fmt.Errorf("failed to write provider.tf.json: %w", err)
	}
	logger.Info("✓ Generated provider.tf.json from snapshot")

	// 3. 生成 variables.tf.json（从快照的变量）
	variablesDef := make(map[string]interface{})
	for _, v := range snapshotVariables {
		varDef := map[string]interface{}{
			"type": "string",
		}
		if v.Description != "" {
			varDef["description"] = v.Description
		}
		if v.Sensitive {
			varDef["sensitive"] = true
		}
		variablesDef[v.Key] = varDef
	}

	if len(variablesDef) > 0 {
		config := map[string]interface{}{
			"variable": variablesDef,
		}
		if err := s.writeJSONFile(workDir, "variables.tf.json", config); err != nil {
			return fmt.Errorf("failed to write variables.tf.json: %w", err)
		}
		logger.Info("✓ Generated variables.tf.json from snapshot (%d variables)", len(variablesDef))
	} else {
		// 如果没有变量，不生成 variables.tf.json 文件
		logger.Info("No terraform variables in snapshot, skipping variables.tf.json generation")
	}

	// 4. 生成 variables.tfvars（从快照的变量）
	var tfvars strings.Builder
	sensitiveCount := 0

	for _, v := range snapshotVariables {
		if v.Sensitive {
			sensitiveCount++
		}

		// 根据ValueFormat处理
		if v.ValueFormat == models.ValueFormatHCL {
			trimmedValue := strings.TrimSpace(v.Value)
			needsQuotes := !strings.HasPrefix(trimmedValue, "{") &&
				!strings.HasPrefix(trimmedValue, "[") &&
				trimmedValue != "true" &&
				trimmedValue != "false" &&
				!isNumeric(trimmedValue)

			if needsQuotes {
				escapedValue := strings.ReplaceAll(v.Value, "\"", "\\\"")
				escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
				tfvars.WriteString(fmt.Sprintf("%s = \"%s\"\n", v.Key, escapedValue))
			} else {
				tfvars.WriteString(fmt.Sprintf("%s = %s\n", v.Key, v.Value))
			}
		} else {
			escapedValue := strings.ReplaceAll(v.Value, "\"", "\\\"")
			escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
			tfvars.WriteString(fmt.Sprintf("%s = \"%s\"\n", v.Key, escapedValue))
		}
	}

	if err := s.writeFile(workDir, "variables.tfvars", tfvars.String()); err != nil {
		return fmt.Errorf("failed to write variables.tfvars: %w", err)
	}
	logger.Info("✓ Generated variables.tfvars from snapshot (%d assignments, %d sensitive)",
		len(snapshotVariables), sensitiveCount)

	logger.Debug("All config files generated successfully from snapshot")
	return nil
}
