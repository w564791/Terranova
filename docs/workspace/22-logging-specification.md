# Terraform执行日志记录规范

> **文档版本**: v1.0  
> **创建日期**: 2025-10-11  
> **状态**: 完整规范  
> **优先级**: P0（最高优先级）  
> **前置阅读**: [15-terraform-execution-detail.md](./15-terraform-execution-detail.md), [17-resource-level-version-control.md](./17-resource-level-version-control.md)

## 📋 概述

本文档定义Terraform执行过程中的完整日志记录规范，确保每个阶段的操作都有详细、可追溯的日志记录，方便用户通过IaC平台实时查看执行进度和排查问题。

## 🎯 核心原则

### 1. 日志详细程度
采用**详细模式**，包含：
- 操作描述
- 结果状态（✓/✗）
- 关键数据摘要
- 日志级别标记（DEBUG/INFO/WARN/ERROR）
- 时间戳

### 2. 资源级别版本信息
必须打印每个资源的名称和版本号：
```
✓ Resource: aws_s3_bucket.my_bucket (version: 3)
✓ Resource: aws_iam_role.service_role (version: 2)
```

### 3. 敏感信息处理
标记为`sensitive`的变量显示为：
```
✓ Variable: db_password = ***SENSITIVE*** (string)
```

### 4. 完整的Terraform输出
必须显示terraform init/plan/apply的完整输出，包括：
- Provider下载进度
- 资源变更详情
- 执行时间统计

### 5. 详细的错误信息
失败时必须包含：
- 详细的错误堆栈
- 失败时的系统状态
- 重试次数和间隔

### 6. 日志级别控制
通过环境变量`TF_LOG`控制日志级别：
- `TF_LOG=debug` - 显示所有日志
- `TF_LOG=info` - 显示INFO/WARN/ERROR（默认）
- `TF_LOG=warn` - 显示WARN/ERROR
- `TF_LOG=error` - 只显示ERROR

## 📊 日志级别定义

### LogLevel枚举

```go
type LogLevel int

const (
    LogLevelDebug LogLevel = iota  // 0 - 调试信息
    LogLevelInfo                    // 1 - 一般信息
    LogLevelWarn                    // 2 - 警告信息
    LogLevelError                   // 3 - 错误信息
)
```

### 日志级别使用场景

| 级别 | 使用场景 | 示例 |
|------|---------|------|
| DEBUG | 数据库查询、详细步骤 | `Query: SELECT * FROM workspaces WHERE id = 45` |
| INFO | 正常操作、状态变更 | `✓ Workspace configuration loaded` |
| WARN | 警告但不影响执行 | `Warning: State file is large (>100MB)` |
| ERROR | 错误、失败 | `Failed to initialize terraform` |

## 🔧 日志记录实现

### 1. 日志记录器接口

```go
// Logger 日志记录器接口
type Logger interface {
    Debug(format string, args ...interface{})
    Info(format string, args ...interface{})
    Warn(format string, args ...interface{})
    Error(format string, args ...interface{})
    
    // 阶段标记
    StageBegin(stage string)
    StageEnd(stage string)
}

// TerraformLogger Terraform执行日志记录器
type TerraformLogger struct {
    stream         *OutputStream
    fullOutput     *strings.Builder
    fullOutputMutex sync.Mutex
    logLevel       LogLevel
}

// NewTerraformLogger 创建日志记录器
func NewTerraformLogger(stream *OutputStream) *TerraformLogger {
    return &TerraformLogger{
        stream:     stream,
        fullOutput: &strings.Builder{},
        logLevel:   getLogLevelFromEnv(),
    }
}

// getLogLevelFromEnv 从环境变量获取日志级别
func getLogLevelFromEnv() LogLevel {
    level := os.Getenv("TF_LOG")
    switch strings.ToLower(level) {
    case "debug":
        return LogLevelDebug
    case "info":
        return LogLevelInfo
    case "warn":
        return LogLevelWarn
    case "error":
        return LogLevelError
    default:
        return LogLevelInfo // 默认INFO级别
    }
}

// log 记录日志
func (l *TerraformLogger) log(level LogLevel, format string, args ...interface{}) {
    if level < l.logLevel {
        return // 跳过低于当前级别的日志
    }
    
    prefix := ""
    switch level {
    case LogLevelDebug:
        prefix = "[DEBUG]"
    case LogLevelInfo:
        prefix = "[INFO]"
    case LogLevelWarn:
        prefix = "[WARN]"
    case LogLevelError:
        prefix = "[ERROR]"
    }
    
    timestamp := time.Now().Format("15:04:05.000")
    message := fmt.Sprintf(format, args...)
    logLine := fmt.Sprintf("[%s] %s %s", timestamp, prefix, message)
    
    // 广播到WebSocket
    if l.stream != nil {
        l.stream.Broadcast(OutputMessage{
            Type:      "output",
            Line:      logLine,
            Timestamp: time.Now(),
        })
    }
    
    // 保存到完整输出
    l.fullOutputMutex.Lock()
    l.fullOutput.WriteString(logLine)
    l.fullOutput.WriteString("\n")
    l.fullOutputMutex.Unlock()
}

// Debug 记录DEBUG级别日志
func (l *TerraformLogger) Debug(format string, args ...interface{}) {
    l.log(LogLevelDebug, format, args...)
}

// Info 记录INFO级别日志
func (l *TerraformLogger) Info(format string, args ...interface{}) {
    l.log(LogLevelInfo, format, args...)
}

// Warn 记录WARN级别日志
func (l *TerraformLogger) Warn(format string, args ...interface{}) {
    l.log(LogLevelWarn, format, args...)
}

// Error 记录ERROR级别日志
func (l *TerraformLogger) Error(format string, args ...interface{}) {
    l.log(LogLevelError, format, args...)
}

// StageBegin 记录阶段开始
func (l *TerraformLogger) StageBegin(stage string) {
    timestamp := time.Now()
    marker := fmt.Sprintf("========== %s BEGIN at %s ==========",
        strings.ToUpper(stage),
        timestamp.Format("2006-01-02 15:04:05.000"))
    
    // 广播阶段标记
    if l.stream != nil {
        l.stream.Broadcast(OutputMessage{
            Type:      "stage_marker",
            Line:      marker,
            Timestamp: timestamp,
            Stage:     stage,
            Status:    "begin",
        })
    }
    
    // 保存到完整输出
    l.fullOutputMutex.Lock()
    l.fullOutput.WriteString(marker)
    l.fullOutput.WriteString("\n")
    l.fullOutputMutex.Unlock()
}

// StageEnd 记录阶段结束
func (l *TerraformLogger) StageEnd(stage string) {
    timestamp := time.Now()
    marker := fmt.Sprintf("========== %s END at %s ==========",
        strings.ToUpper(stage),
        timestamp.Format("2006-01-02 15:04:05.000"))
    
    // 广播阶段标记
    if l.stream != nil {
        l.stream.Broadcast(OutputMessage{
            Type:      "stage_marker",
            Line:      marker,
            Timestamp: timestamp,
            Stage:     stage,
            Status:    "end",
        })
    }
    
    // 保存到完整输出
    l.fullOutputMutex.Lock()
    l.fullOutput.WriteString(marker)
    l.fullOutput.WriteString("\n")
    l.fullOutputMutex.Unlock()
}

// GetFullOutput 获取完整输出
func (l *TerraformLogger) GetFullOutput() string {
    l.fullOutputMutex.Lock()
    defer l.fullOutputMutex.Unlock()
    return l.fullOutput.String()
}
```

## 📝 各阶段日志规范

### Stage 1: Pending（等待执行）

```
[INFO] Task #123 created, entering pending queue
[INFO] Checking resource availability...
[INFO] ✓ Resource check passed
[INFO] Checking workspace lock status...
[DEBUG] Query: SELECT is_locked, locked_by FROM workspaces WHERE id = 45
[INFO] ✓ Workspace #45 is not locked
[INFO] Ready to proceed to fetching stage
```

**实现示例**：
```go
func (e *TerraformExecutor) HandlePendingStage(task *models.WorkspaceTask) error {
    logger := e.getLogger(task.ID)
    
    logger.Info("Task #%d created, entering pending queue", task.ID)
    logger.Info("Checking resource availability...")
    
    if !e.checkResourceAvailability() {
        logger.Warn("Resources not available, waiting...")
        return nil
    }
    logger.Info("✓ Resource check passed")
    
    logger.Info("Checking workspace lock status...")
    logger.Debug("Query: SELECT is_locked, locked_by FROM workspaces WHERE id = %d", task.WorkspaceID)
    
    workspace, err := e.getWorkspace(task.WorkspaceID)
    if err != nil {
        logger.Error("Failed to get workspace: %v", err)
        return err
    }
    
    if workspace.IsLocked {
        logger.Warn("Workspace #%d is locked by user #%d", workspace.ID, *workspace.LockedBy)
        return nil
    }
    logger.Info("✓ Workspace #%d is not locked", workspace.ID)
    
    logger.Info("Ready to proceed to fetching stage")
    return e.TransitionToFetching(task)
}
```

### Stage 2: Fetching（获取配置）

```
========== FETCHING BEGIN at 2025-10-11 19:30:00.123 ==========
[INFO] Fetching workspace #45 configuration from database...
[DEBUG] Query: SELECT * FROM workspaces WHERE id = 45
[INFO] ✓ Workspace configuration loaded
[INFO]   - Name: production-network
[INFO]   - Execution mode: local
[INFO]   - Terraform version: 1.6.0

[INFO] Fetching workspace resources from workspace_resources table...
[DEBUG] Query: SELECT r.*, v.* FROM workspace_resources r 
       JOIN resource_code_versions v ON r.current_version_id = v.id 
       WHERE r.workspace_id = 45 AND r.is_active = true
[INFO] ✓ Resource: aws_s3_bucket.my_bucket (version: 3)
[INFO] ✓ Resource: aws_iam_role.service_role (version: 2)
[INFO] ✓ Resource: aws_instance.web_server (version: 5)
[INFO] Total: 3 resources loaded

[INFO] Fetching workspace variables...
[DEBUG] Query: SELECT * FROM workspace_variables WHERE workspace_id = 45
[INFO] ✓ Variable: environment = "production" (string)
[INFO] ✓ Variable: instance_type = "t3.medium" (string)
[INFO] ✓ Variable: db_password = ***SENSITIVE*** (string)
[INFO] ✓ Variable: api_key = ***SENSITIVE*** (string)
[INFO] ✓ Variable: enable_monitoring = true (bool)
[INFO] Total: 5 variables loaded (3 normal, 2 sensitive)

[INFO] Fetching provider configuration...
[INFO] ✓ Provider: AWS (region: ap-northeast-1)
[DEBUG] Provider config: {"region":"ap-northeast-1","assume_role":[{"role_arn":"arn:aws:iam::123456789012:role/terraform"}]}

[INFO] Fetching latest state version...
[DEBUG] Query: SELECT * FROM workspace_state_versions 
       WHERE workspace_id = 45 ORDER BY version DESC LIMIT 1
[INFO] ✓ Found state version #12
[INFO]   - Size: 15.2 KB
[INFO]   - Checksum: abc123def456...
[INFO]   - Resources: 8
[INFO]   - Created: 2025-10-11 18:30:00

[INFO] Validating configuration...
[INFO] ✓ All required fields present
[INFO] ✓ Provider configuration valid
[INFO] ✓ Variables configuration valid
[INFO] ✓ Resources configuration valid

[INFO] Configuration fetch completed successfully
========== FETCHING END at 2025-10-11 19:30:05.456 ==========
```

**实现示例**：
```go
func (e *TerraformExecutor) HandleFetchingStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    logger := e.getLogger(task.ID)
    logger.StageBegin("fetching")
    defer logger.StageEnd("fetching")
    
    // 1. 获取Workspace配置
    logger.Info("Fetching workspace #%d configuration from database...", task.WorkspaceID)
    logger.Debug("Query: SELECT * FROM workspaces WHERE id = %d", task.WorkspaceID)
    
    workspace, err := e.getWorkspace(task.WorkspaceID)
    if err != nil {
        logger.Error("Failed to fetch workspace: %v", err)
        return fmt.Errorf("failed to fetch workspace: %w", err)
    }
    logger.Info("✓ Workspace configuration loaded")
    logger.Info("  - Name: %s", workspace.Name)
    logger.Info("  - Execution mode: %s", workspace.ExecutionMode)
    logger.Info("  - Terraform version: %s", workspace.TerraformVersion)
    
    // 2. 获取资源
    logger.Info("Fetching workspace resources from workspace_resources table...")
    logger.Debug("Query: SELECT r.*, v.* FROM workspace_resources r JOIN resource_code_versions v ON r.current_version_id = v.id WHERE r.workspace_id = %d AND r.is_active = true", task.WorkspaceID)
    
    resources, err := e.getWorkspaceResources(task.WorkspaceID)
    if err != nil {
        logger.Error("Failed to fetch resources: %v", err)
        return fmt.Errorf("failed to fetch resources: %w", err)
    }
    
    for _, resource := range resources {
        logger.Info("✓ Resource: %s (version: %d)", 
            resource.ResourceID, resource.CurrentVersion.Version)
    }
    logger.Info("Total: %d resources loaded", len(resources))
    
    // 3. 获取变量
    logger.Info("Fetching workspace variables...")
    logger.Debug("Query: SELECT * FROM workspace_variables WHERE workspace_id = %d", task.WorkspaceID)
    
    variables, err := e.getWorkspaceVariables(task.WorkspaceID)
    if err != nil {
        logger.Error("Failed to fetch variables: %v", err)
        return fmt.Errorf("failed to fetch variables: %w", err)
    }
    
    normalCount := 0
    sensitiveCount := 0
    for _, variable := range variables {
        if variable.Sensitive {
            logger.Info("✓ Variable: %s = ***SENSITIVE*** (%s)", 
                variable.Key, variable.Type)
            sensitiveCount++
        } else {
            logger.Info("✓ Variable: %s = %s (%s)", 
                variable.Key, variable.Value, variable.Type)
            normalCount++
        }
    }
    logger.Info("Total: %d variables loaded (%d normal, %d sensitive)", 
        len(variables), normalCount, sensitiveCount)
    
    // 4. 获取Provider配置
    logger.Info("Fetching provider configuration...")
    if providerConfig, ok := workspace.ProviderConfig["aws"].([]interface{}); ok && len(providerConfig) > 0 {
        aws := providerConfig[0].(map[string]interface{})
        region := aws["region"].(string)
        logger.Info("✓ Provider: AWS (region: %s)", region)
        logger.Debug("Provider config: %s", toJSON(workspace.ProviderConfig))
    }
    
    // 5. 获取State版本
    logger.Info("Fetching latest state version...")
    logger.Debug("Query: SELECT * FROM workspace_state_versions WHERE workspace_id = %d ORDER BY version DESC LIMIT 1", task.WorkspaceID)
    
    stateVersion, err := e.getLatestStateVersion(task.WorkspaceID)
    if err != nil && err != gorm.ErrRecordNotFound {
        logger.Error("Failed to fetch state: %v", err)
        return fmt.Errorf("failed to fetch state: %w", err)
    }
    
    if stateVersion != nil {
        logger.Info("✓ Found state version #%d", stateVersion.Version)
        logger.Info("  - Size: %.1f KB", float64(stateVersion.SizeBytes)/1024)
        logger.Info("  - Checksum: %s", stateVersion.Checksum[:16]+"...")
        logger.Info("  - Created: %s", stateVersion.CreatedAt.Format("2006-01-02 15:04:05"))
    } else {
        logger.Info("No existing state found (first run)")
    }
    
    // 6. 验证配置
    logger.Info("Validating configuration...")
    if err := e.validateWorkspaceConfig(workspace); err != nil {
        logger.Error("Configuration validation failed: %v", err)
        return fmt.Errorf("invalid workspace config: %w", err)
    }
    logger.Info("✓ All required fields present")
    logger.Info("✓ Provider configuration valid")
    logger.Info("✓ Variables configuration valid")
    logger.Info("✓ Resources configuration valid")
    
    logger.Info("Configuration fetch completed successfully")
    
    return nil
}
```

### Stage 3: Init（Terraform初始化）

```
========== INIT BEGIN at 2025-10-11 19:30:05.500 ==========
[INFO] Creating work directory: /tmp/iac-platform/workspaces/45/123
[INFO] ✓ Work directory created

[INFO] Generating configuration files from resources...
[DEBUG] Aggregating TF code from 3 resources
[INFO] ✓ Generated main.tf.json from 3 resources (2.5 KB)
[DEBUG] File content: 156 lines, 3 resource blocks
[INFO] ✓ Generated provider.tf.json (AWS provider)
[INFO] ✓ Generated variables.tf.json (5 variables)
[INFO] ✓ Generated variables.tfvars (5 assignments, 2 sensitive)

[INFO] Preparing state file...
[INFO] ✓ Restored state version #12 to terraform.tfstate (15.2 KB)

[INFO] Executing: terraform init -no-color -upgrade
Initializing the backend...

Initializing provider plugins...
- Finding hashicorp/aws versions matching "~> 5.0"...
- Downloading plugin for provider "aws" (hashicorp/aws) 5.31.0...
- Downloaded hashicorp/aws v5.31.0 (15.2 MB in 3.5s)
- Installing hashicorp/aws v5.31.0...
- Installed hashicorp/aws v5.31.0 (signed by HashiCorp)

Terraform has been successfully initialized!

You may now begin working with Terraform. Try running "terraform plan" to see
any changes that are required for your infrastructure. All Terraform commands
should now work.

If you ever set or change modules or backend configuration for Terraform,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.

[INFO] ✓ Terraform initialization completed successfully
[INFO] Initialization time: 10.3 seconds
========== INIT END at 2025-10-11 19:30:15.789 ==========
```

**实现示例**：
```go
func (e *TerraformExecutor) TerraformInit(
    ctx context.Context,
    workDir string,
    task *models.WorkspaceTask,
    workspace *models.Workspace,
) error {
    logger := e.getLogger(task.ID)
    
    // 1. 构建命令
    args := []string{"init", "-no-color", "-input=false", "-upgrade"}
    cmd := exec.CommandContext(ctx, "terraform", args...)
    cmd.Dir = workDir
    cmd.Env = e.buildEnvironmentVariables(workspace)
    
    // 2. 创建Pipe捕获输出
    stdoutPipe, _ := cmd.StdoutPipe()
    stderrPipe, _ := cmd.StderrPipe()
    
    // 3. 启动命令
    logger.Info("Executing: terraform init -no-color -upgrade")
    startTime := time.Now()
    
    if err := cmd.Start(); err != nil {
        logger.Error("Failed to start terraform init: %v", err)
        return err
    }
    
    // 4. 实时读取输出
    var wg sync.WaitGroup
    wg.Add(2)
    
    go func() {
        defer wg.Done()
        scanner := bufio.NewScanner(stdoutPipe)
        for scanner.Scan() {
            line := scanner.Text()
            // 直接输出terraform的原始输出（不加前缀）
            logger.stream.Broadcast(OutputMessage{
                Type:      "output",
                Line:      line,
                Timestamp: time.Now(),
            })
        }
    }()
    
    go func() {
        defer wg.Done()
        scanner := bufio.NewScanner(stderrPipe)
        for scanner.Scan() {
            line := scanner.Text()
            logger.stream.Broadcast(OutputMessage{
                Type:      "output",
                Line:      line,
                Timestamp: time.Now(),
            })
        }
    }()
    
    // 5. 等待命令完成
    cmdErr := cmd.Wait()
    wg.Wait()
    
    duration := time.Since(startTime)
    
    if cmdErr != nil {
        logger.Error("Terraform init failed: %v", cmdErr)
        return fmt.Errorf("terraform init failed: %w", cmdErr)
    }
    
    logger.Info("✓ Terraform initialization completed successfully")
    logger.Info("Initialization time: %.1f seconds", duration.Seconds())
    
    return nil
}
```

### Stage 4: Planning（执行Plan）

```
========== PLANNING BEGIN at 2025-10-11 19:30:15.800 ==========
[INFO] Executing: terraform plan -out=plan.out -no-color -var-file=variables.tfvars

Terraform used the selected providers to generate the following execution plan.
Resource actions are indicated with the following symbols:
  + create
  ~ update in-place
  - destroy

Terraform will perform the following actions:

  # aws_s3_bucket.my_bucket will be created
  + resource "aws_s3_bucket" "my_bucket" {
      + acceleration_status         = (known after apply)
      + acl                          = (known after apply)
      + arn                          = (known after apply)
      + bucket                       = "my-unique-bucket-name"
      + bucket_domain_name           = (known after apply)
      + bucket_regional_domain_name  = (known after apply)
      + force_destroy                = false
      + hosted_zone_id               = (known after apply)
      + id                           = (known after apply)
      + object_lock_enabled          = (known after apply)
      + policy                       = (known after apply)
      + region                       = (known after apply)
      + request_payer                = (known after apply)
      + tags_all                     = (known after apply)
      + website_domain               = (known after apply)
      + website_endpoint             = (known after apply)
    }

  # aws_iam_role.service_role will be updated in-place
  ~ resource "aws_iam_role" "service_role" {
        id                    = "service-role"
        name                  = "service-role"
      ~ assume_role_policy    = jsonencode(
          ~ {
              ~ Statement = [
                  ~ {
                      ~ Principal = {
                          ~ Service = [
                              - "ec2.amazonaws.com",
                              + "ecs-tasks.amazonaws.com",
                            ]
                        }
                    }
                ]
            }
        )
        tags                  = {}
    }

Plan: 1 to add, 1 to change, 0 to destroy.

Changes to Outputs:
  + bucket_name = "my-unique-bucket-name"
  ~ role_arn    = "arn:aws:iam::123456789012:role/service-role" -> (known after apply)

[INFO] ✓ Plan completed successfully
[INFO] Plan execution time: 89.4 seconds

[INFO] Generating plan JSON for analysis...
[INFO] ✓ Generated plan.json (128.5 KB)

[INFO] Saving plan data to database...
[INFO] ✓ Plan saved to database (task #123)
[INFO]   - Plan file size: 45.2 KB
[INFO]   - Plan JSON size: 128.5 KB

[INFO] Plan Summary:
[INFO]   - Resources to add: 1
[INFO]   - Resources to change: 1
[INFO]   - Resources to destroy: 0
[INFO]   - Total changes: 2

========== PLANNING END at 2025-10-11 19:31:45.234 ==========
```

### Stage 5: Applying（执行Apply）

```
========== APPLYING BEGIN at 2025-10-11 19:32:01.500 ==========
[INFO] Executing: terraform apply -no-color -auto-approve plan.out

aws_s3_bucket.my_bucket: Creating...
aws_s3_bucket.my_bucket: Still creating... [10s elapsed]
aws_s3_bucket.my_bucket: Creation complete after 12s [id=my-unique-bucket-name]

aws_iam_role.service_role: Modifying... [id=service-role]
aws_iam_role.service_role: Modifications complete after 2s [id=service-role]

Apply complete! Resources: 1 added, 1 changed, 0 destroyed.

Outputs:

bucket_name = "my-unique-bucket-name"
role_arn = "arn:aws:iam::123456789012:role/service-role"

[INFO] ✓ Apply completed successfully
[INFO] Apply execution time: 89.5 seconds

[INFO] Extracting terraform outputs...
[INFO] ✓ Found 2 outputs
[INFO]   - bucket_name: "my-unique-bucket-name"
[INFO]   - role_arn: "arn:aws:iam::123456789012:role/service-role"

========== APPLYING END at 2025-10-11 19:33:31.000 ==========
```

### Stage 6: Saving State（保存State）

```
========== SAVING_STATE BEGIN at 2025-10-11 19:33:31.100 ==========
[INFO] Reading state file from work directory...
[INFO] ✓ State file read successfully (18.7 KB)

[INFO] Parsing state content...
[INFO] ✓ State version: 4
[INFO] ✓ Terraform version: 1.6.0
[INFO] ✓ Resources count: 12
[INFO] ✓ Outputs count: 2

[INFO] Calculating checksum...
[INFO] ✓ Checksum: ghi789abc123...

[INFO] Saving to database...
[DEBUG] Query: SELECT COALESCE(MAX(version), 0) FROM workspace_state_versions WHERE workspace_id = 45
[INFO] ✓ Current max version: 12
[INFO] ✓ Creating new version: 13
[DEBUG] Query: INSERT INTO workspace_state_versions (workspace_id, version, content, checksum, size_bytes, task_id, created_by) VALUES (...)
[INFO] ✓ State version #13 created successfully
[INFO] ✓ Updated workspace current_state_id

[INFO] State save completed successfully
[INFO] Version: 13
[INFO] Size: 18.7 KB
[INFO] Resources: 12
[INFO] Checksum: ghi789abc123...

========== SAVING_STATE END at 2025-10-11 19:33:32.567 ==========
```

## ❌ 错误日志规范

### 错误日志格式

```
[ERROR] ========== INIT FAILED at 2025-10-11 19:30:20.123 ==========
[ERROR] Failed to initialize terraform
[ERROR] Error: Failed to download provider hashicorp/aws
[ERROR] 
[ERROR] Caused by: connection timeout after 30s
[ERROR] 
[ERROR] Stack trace:
[ERROR]   at TerraformExecutor.TerraformInit (terraform_executor.go:245)
[ERROR]     workDir: /tmp/iac-platform/workspaces/45/123
[ERROR]     command: terraform init -no-color -upgrade
[ERROR]   at TerraformExecutor.ExecutePlan (terraform_executor.go:180)
[ERROR]     taskID: 123
[ERROR]     workspaceID: 45
[ERROR]   at TaskWorker.processTask (task_worker.go:89)
[ERROR] 
[ERROR] System state:
[ERROR]   - Workspace: #45 (production-network)
[ERROR]   - Task: #123 (plan)
[ERROR]   - Resources: 3 loaded
[ERROR]   - Variables: 5 loaded
[ERROR]   - State version: 12
[ERROR] 
[ERROR] Retry information:
[ERROR]   - Current attempt: 1/3
[ERROR]   - Next retry in: 5 seconds
[ERROR]   - Retry strategy: exponential backoff
========== INIT FAILED END ==========
```

### 错误日志实现

```go
// LogError 记录详细错误
func (l *TerraformLogger) LogError(
    stage string,
    err error,
    context map[string]interface{},
    retryInfo *RetryInfo,
) {
    l.Error("========== %s FAILED at %s ==========", 
        strings.ToUpper(stage), 
        time.Now().Format("2006-01-02 15:04:05.000"))
    
    l.Error("Failed to %s", stage)
    l.Error("Error: %v", err)
    l.Error("")
    
    // 错误堆栈
    if stack := getStackTrace(); stack != "" {
        l.Error("Stack trace:")
        for _, line := range strings.Split(stack, "\n") {
            l.Error("  %s", line)
        }
        l.Error("")
    }
    
    // 系统状态
    if context != nil {
        l.Error("System state:")
        for key, value := range context {
            l.Error("  - %s: %v", key, value)
        }
        l.Error("")
    }
    
    // 重试信息
    if retryInfo != nil {
        l.Error("Retry information:")
        l.Error("  - Current attempt: %d/%d", retryInfo.CurrentAttempt, retryInfo.MaxRetries)
        l.Error("  - Next retry in: %v", retryInfo.NextRetryDelay)
        l.Error("  - Retry strategy: %s", retryInfo.Strategy)
    }
    
    l.Error("========== %s FAILED END ==========", strings.ToUpper(stage))
}

// RetryInfo 重试信息
type RetryInfo struct {
    CurrentAttempt int
    MaxRetries     int
    NextRetryDelay time.Duration
    Strategy       string
}
```

## 📋 实施检查清单

### 开发阶段
- [ ] 实现TerraformLogger结构
- [ ] 实现日志级别控制
- [ ] 实现阶段标记
- [ ] 修改所有执行阶段使用Logger
- [ ] 添加资源版本信息日志
- [ ] 添加敏感信息过滤
- [ ] 实现错误日志格式

### 测试阶段
- [ ] 测试不同日志级别
- [ ] 测试阶段标记显示
- [ ] 测试资源版本信息
- [ ] 测试敏感信息过滤
- [ ] 测试错误日志格式
- [ ] 测试WebSocket实时推送

### 文档阶段
- [ ] 更新开发文档
- [ ] 编写使用示例
- [ ] 编写故障排查指南

## 🔗 相关文档

- **上一篇**: [15-terraform-execution-detail.md](./15-terraform-execution-detail.md) - Terraform执行流程详细设计
- **相关**: [17-resource-level-version-control.md](./17-resource-level-version-control.md) - 资源级别版本控制
- **相关**: [21-terraform-output-streaming.md](./21-terraform-output-streaming.md) - 输出实时流式传输

## 📝 总结

本文档定义了完整的Terraform执行日志记录规范，包括：

1.  **日志级别控制** - 通过TF_LOG环境变量控制
2.  **资源版本信息** - 必须打印资源名称和版本号
3.  **敏感信息处理** - 标记为sensitive的变量显示为***SENSITIVE***
4.  **完整Terraform输出** - 包括provider下载进度
5.  **详细错误信息** - 包含堆栈、系统状态、重试信息
6.  **阶段标记** - 清晰的BEGIN/END标记
7.  **实时流式传输** - 通过WebSocket推送到前端

这个规范确保了用户可以通过IaC平台实时查看Terraform执行的每一个细节，方便排查问题和监控进度。

---

**实施优先级**: P0（最高优先级）  
**预计工时**: 3-4天  
**依赖**: 21-terraform-output-streaming.md（已完成）
