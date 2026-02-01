# Terraform执行流程详细设计

> **文档版本**: v1.0  
> **创建日期**: 2025-10-11  
> **状态**: 完整设计  
> **前置阅读**: [01-lifecycle.md](./01-lifecycle.md), [04-task-workflow.md](./04-task-workflow.md)  
> **相关文档**: [22-logging-specification.md](./22-logging-specification.md) - 日志记录规范

## 📋 概述

本文档详细描述Terraform执行的完整流程，包括执行准备、环境配置、命令执行、结果处理等所有细节。

## 🎯 核心设计原则

### 1. 数据来源
- **Workspace资源**: 来自两个途径
  1. 在Workspace中直接添加平台支持的Module
  2. 在Module侧提交表单后插入到Workspace
- **代码存储**: 数据库JSONB字段（高效存储）
- **本地文件**: 执行时从数据库拉取生成临时文件

### 2. 安全策略
- **敏感变量**: 标记为sensitive的变量不返回到前端
- **认证信息**: 暂不加密，后续接入Vault
- **Provider配置**: 存储在workspace.provider_config

### 3. 执行隔离
- **工作目录**: 每次执行创建新的临时目录
- **并发控制**: 通过目录隔离实现任务并发
- **资源清理**: 执行完成后清理临时目录

### 4. State管理
- **存储位置**: 数据库（高效存储格式）
- **版本策略**: 永久保留所有版本
- **本地文件**: 执行时命名为terraform.tfstate
- **回写机制**: 执行完成后保存回数据库

### 5. Plan和Apply分离
- **Plan任务**: 生成执行计划并保存到数据库
- **Apply任务**: 必须使用数据库中的Plan数据
- **强耦合**: Apply执行时强制从数据库读取Plan
- **扩展性**: Plan和Apply之间可插入检测功能

## 🗂️ 文件结构设计

### 工作目录结构

```
/tmp/iac-platform/workspaces/{workspace_id}/{task_id}/
├── main.tf.json          # 主配置文件（Module定义）
├── provider.tf.json      # Provider配置
├── variables.tf.json     # 变量定义
├── variables.tfvars      # 变量赋值
├── terraform.tfstate     # State文件（从数据库拉取）
├── plan.out              # Plan输出文件（二进制）
├── plan.json             # Plan JSON格式（用于解析）
└── .terraform/           # Terraform初始化目录
    └── providers/        # Provider插件
```

### 文件内容示例

#### 1. main.tf.json
```json
{
  "module": {
    "accessanalyzermonitorservicepolicy_qx25imwh37_policy": [
      {
        "attach_to_roles": [
          "AccessAnalyzerMonitorServiceRole_QXQU564Y3L"
        ],
        "create_policy": true,
        "create_role": false,
        "iam_path": "/service-role/",
        "name": "AccessAnalyzerMonitorServicePolicy_QX25IMWH37",
        "policy_document": "${jsonencode(\n    {\n        \"Version\": \"2012-10-17\",\n        \"Statement\": [\n            {\n                \"Effect\": \"Allow\",\n                \"Action\": \"cloudtrail:GetTrail\",\n                \"Resource\": \"*\"\n            }\n        ]\n    }\n  )}",
        "source": "tfe-applications.kcprd.com/default/iam/kucoin",
        "use_business_line_as_path": false
      }
    ]
  }
}
```

#### 2. provider.tf.json
```json
{
  "provider": {
    "aws": [
      {
        "assume_role": [
          {
            "role_arn": "arn:aws:iam::817275903355:role/ops-privileged-tfe"
          }
        ],
        "region": "ap-northeast-1"
      }
    ]
  }
}
```

#### 3. variables.tf.json
```json
{
  "variable": {
    "environment": {
      "type": "string",
      "description": "Environment name",
      "default": "production"
    },
    "db_password": {
      "type": "string",
      "description": "Database password",
      "sensitive": true
    }
  }
}
```

#### 4. variables.tfvars
```hcl
environment = "production"
db_password = "secret123"
```

## 🔄 完整执行流程（基于TFE标准）

### 执行阶段概览

参考Terraform Enterprise的标准流程，我们的平台支持以下执行阶段：

```
┌─────────────────────────────────────────────────────────────────┐
│                        Run Lifecycle                             │
├─────────────────────────────────────────────────────────────────┤
│ 1. Pending Stage          - 任务排队等待                         │
│ 2. Fetching Stage         - 获取代码和配置                       │
│ 3. Pre-Plan Stage         - Plan前置处理（可扩展）               │
│ 4. Plan Stage             - 执行terraform plan                   │
│ 5. Post-Plan Stage        - Plan后置处理（可扩展）               │
│ 6. Cost Estimation Stage  - 成本估算（可选，未来扩展）           │
│ 7. Policy Check Stage     - 策略检查（可选，未来扩展）           │
│ 8. Pre-Apply Stage        - Apply前置处理（可扩展）              │
│ 9. Apply Stage            - 执行terraform apply                  │
│ 10. Post-Apply Stage      - Apply后置处理（可扩展）              │
│ 11. Completion            - 任务完成                             │
└─────────────────────────────────────────────────────────────────┘
```

### 阶段和状态定义（基于TFE标准）

#### 运行阶段 (Run Stage)

```go
type RunStage string

const (
    // 执行阶段
    StagePending         RunStage = "pending"          // 等待执行
    StageFetching        RunStage = "fetching"         // 获取配置
    StagePrePlan         RunStage = "pre_plan"         // Plan前置
    StagePlanning        RunStage = "planning"         // 执行Plan
    StagePostPlan        RunStage = "post_plan"        // Plan后置
    StageCostEstimation  RunStage = "cost_estimation"  // 成本估算
    StagePolicyCheck     RunStage = "policy_check"     // 策略检查（OPA/Sentinel）
    StagePreApply        RunStage = "pre_apply"        // Apply前置
    StageApplying        RunStage = "applying"         // 执行Apply
    StagePostApply       RunStage = "post_apply"       // Apply后置
    StageCompletion      RunStage = "completion"       // 完成阶段
)
```

#### 运行状态 (Run State)

```go
type RunState string

const (
    // Pending阶段状态
    StatePending         RunState = "pending"           // 等待队列中
    
    // Fetching阶段状态
    StateFetching        RunState = "fetching"          // 从VCS获取配置
    
    // Pre-Plan阶段状态
    StatePrePlanRunning  RunState = "pre_plan_running"  // Pre-plan任务运行中
    
    // Plan阶段状态
    StatePlanning        RunState = "planning"          // 正在执行plan
    StateNeedsConfirm    RunState = "needs_confirmation" // Plan完成，等待确认
    
    // Post-Plan阶段状态
    StatePostPlanRunning RunState = "post_plan_running" // Post-plan任务运行中
    
    // Cost Estimation阶段状态
    StateCostEstimating  RunState = "cost_estimating"   // 成本估算中
    StateCostEstimated   RunState = "cost_estimated"    // 成本估算完成
    
    // Policy Check阶段状态
    StatePolicyCheck     RunState = "policy_check"      // 策略检查中
    StatePolicyOverride  RunState = "policy_override"   // 策略失败，等待覆盖
    StatePolicyChecked   RunState = "policy_checked"    // 策略检查通过
    
    // Pre-Apply阶段状态
    StatePreApplyRunning RunState = "pre_apply_running" // Pre-apply任务运行中
    
    // Apply阶段状态
    StateApplying        RunState = "applying"          // 正在执行apply
    
    // Post-Apply阶段状态
    StatePostApplyRunning RunState = "post_apply_running" // Post-apply任务运行中
    
    // 完成状态
    StateApplied         RunState = "applied"           // Apply成功
    StatePlannedFinished RunState = "planned_finished"  // Plan完成但无变更
    StateApplyErrored    RunState = "apply_errored"     // Apply失败
    StatePlanErrored     RunState = "plan_errored"      // Plan失败
    StateDiscarded       RunState = "discarded"         // 用户丢弃
    StateCanceled        RunState = "canceled"          // 用户取消
)
```

#### 状态转换规则

```go
// 状态转换映射
var StateTransitions = map[RunStage][]RunState{
    StagePending: {
        StatePending,
        StateDiscarded,  // 用户在开始前丢弃
    },
    StageFetching: {
        StateFetching,
        StatePlanErrored, // VCS获取失败
    },
    StagePrePlan: {
        StatePrePlanRunning,
        StatePlanErrored, // 强制任务失败
        StateCanceled,    // 用户取消
    },
    StagePlanning: {
        StatePlanning,
        StateNeedsConfirm,
        StatePlannedFinished, // 无变更
        StatePlanErrored,     // Plan失败
        StateCanceled,        // 用户取消
    },
    StagePostPlan: {
        StatePostPlanRunning,
        StatePlanErrored, // 强制任务失败
        StateCanceled,    // 用户取消
    },
    StageCostEstimation: {
        StateCostEstimating,
        StateCostEstimated,
        StatePlannedFinished, // 无后续操作
    },
    StagePolicyCheck: {
        StatePolicyCheck,
        StatePolicyOverride, // 软强制策略失败
        StatePolicyChecked,  // 策略通过
        StatePlanErrored,    // 硬强制策略失败
        StateDiscarded,      // 用户丢弃
    },
    StagePreApply: {
        StatePreApplyRunning,
        StateApplyErrored, // 强制任务失败
        StateCanceled,     // 用户取消
    },
    StageApplying: {
        StateApplying,
        StateApplied,      // Apply成功
        StateApplyErrored, // Apply失败
        StateCanceled,     // 用户取消
    },
    StagePostApply: {
        StatePostApplyRunning,
        StateApplied, // 完成（即使advisory任务失败）
        StateCanceled, // 用户取消
    },
}

// 阶段配置
type StageConfig struct {
    Enabled  bool                   `json:"enabled"`   // 是否启用
    Timeout  int                    `json:"timeout"`   // 超时时间（秒）
    Hooks    []string               `json:"hooks"`     // 钩子脚本
    Metadata map[string]interface{} `json:"metadata"`  // 元数据
}

// 任务运行配置
type RunConfig struct {
    PrePlan        StageConfig `json:"pre_plan"`
    PostPlan       StageConfig `json:"post_plan"`
    CostEstimation StageConfig `json:"cost_estimation"`
    PolicyCheck    StageConfig `json:"policy_check"`
    PreApply       StageConfig `json:"pre_apply"`
    PostApply      StageConfig `json:"post_apply"`
}
```

### Stage 1: Pending Stage（等待执行）

**目的**: 任务排队，等待资源分配

**操作**:
```go
func (s *TerraformExecutor) HandlePendingStage(task *models.WorkspaceTask) error {
    // 1. 更新任务状态
    task.Stage = StagePending
    task.Status = models.TaskStatusPending
    
    // 2. 检查资源可用性
    if !s.checkResourceAvailability() {
        log.Printf("Task %d waiting for resources", task.ID)
        return nil
    }
    
    // 3. 检查Workspace锁定状态
    workspace, err := s.getWorkspace(task.WorkspaceID)
    if err != nil {
        return err
    }
    
    if workspace.IsLocked {
        log.Printf("Task %d waiting for workspace unlock", task.ID)
        return nil
    }
    
    // 4. 进入下一阶段
    return s.TransitionToFetching(task)
}
```

### Stage 2: Fetching Stage（获取配置）

**目的**: 从数据库获取Workspace配置、代码、State等

**操作**:
```go
func (s *TerraformExecutor) HandleFetchingStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StageFetching
    task.Status = models.TaskStatusRunning
    s.db.Save(task)
    
    // 2. 获取Workspace配置
    workspace, err := s.getWorkspace(task.WorkspaceID)
    if err != nil {
        return fmt.Errorf("failed to fetch workspace: %w", err)
    }
    
    // 3. 验证配置完整性
    if err := s.validateWorkspaceConfig(workspace); err != nil {
        return fmt.Errorf("invalid workspace config: %w", err)
    }
    
    // 4. 获取变量
    variables, err := s.getWorkspaceVariables(task.WorkspaceID)
    if err != nil {
        return fmt.Errorf("failed to fetch variables: %w", err)
    }
    
    // 5. 获取最新State（如果存在）
    stateVersion, err := s.getLatestStateVersion(task.WorkspaceID)
    if err != nil && err != gorm.ErrRecordNotFound {
        return fmt.Errorf("failed to fetch state: %w", err)
    }
    
    // 6. 缓存配置到任务上下文
    task.Context = map[string]interface{}{
        "workspace":     workspace,
        "variables":     variables,
        "state_version": stateVersion,
    }
    
    log.Printf("Task %d: Fetching completed", task.ID)
    
    // 7. 进入下一阶段
    return s.TransitionToPrePlan(task)
}
```

### Stage 3: Pre-Plan Stage（Plan前置处理）

**目的**: Plan执行前的准备工作和扩展点

**扩展能力**:
- 代码语法检查
- 安全扫描（静态分析）
- 自定义验证脚本
- 通知发送

**操作**:
```go
func (s *TerraformExecutor) HandlePrePlanStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StagePrePlan
    s.db.Save(task)
    
    // 2. 获取Pre-Plan配置
    workspace := task.Context["workspace"].(*models.Workspace)
    runConfig := s.getRunConfig(workspace)
    
    if !runConfig.PrePlan.Enabled {
        log.Printf("Task %d: Pre-Plan stage skipped", task.ID)
        return s.TransitionToPlanning(task)
    }
    
    // 3. 执行Pre-Plan钩子
    for _, hook := range runConfig.PrePlan.Hooks {
        if err := s.executeHook(ctx, task, hook, "pre_plan"); err != nil {
            return fmt.Errorf("pre-plan hook failed: %w", err)
        }
    }
    
    // 4. 代码语法检查（可选）
    if runConfig.PrePlan.Metadata["syntax_check"] == true {
        if err := s.validateTerraformSyntax(workspace); err != nil {
            return fmt.Errorf("syntax validation failed: %w", err)
        }
    }
    
    // 5. 发送通知
    s.notifySystem.Notify(models.EventPrePlanStart, workspace, task)
    
    log.Printf("Task %d: Pre-Plan completed", task.ID)
    
    // 6. 进入Plan阶段
    return s.TransitionToPlanning(task)
}
```

### Stage 4: Plan Stage（执行Plan）

**目的**: 执行terraform plan，生成执行计划

**操作**: （保持原有实现，添加阶段管理）

```go
func (s *TerraformExecutor) HandlePlanStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StagePlanning
    s.db.Save(task)
    
    // 2. 准备工作目录
    workDir, err := s.PrepareWorkspace(task)
    if err != nil {
        return err
    }
    defer s.CleanupWorkspace(workDir)
    
    // 3. 获取配置
    workspace := task.Context["workspace"].(*models.Workspace)
    
    // 4. 生成配置文件
    if err := s.GenerateConfigFiles(workspace, workDir); err != nil {
        return err
    }
    
    // 5. 准备State文件
    if err := s.PrepareStateFile(workspace, workDir); err != nil {
        return err
    }
    
    // 6. Terraform初始化
    if err := s.TerraformInit(ctx, workDir, task); err != nil {
        return err
    }
    
    // 7. 执行Plan
    planFile := filepath.Join(workDir, "plan.out")
    cmd := exec.CommandContext(ctx, "terraform", "plan",
        "-out="+planFile,
        "-no-color",
        "-var-file=variables.tfvars",
    )
    cmd.Dir = workDir
    cmd.Env = append(os.Environ(),
        "TF_IN_AUTOMATION=true",
        "TF_INPUT=false",
    )
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    log.Printf("Executing: terraform plan in %s", workDir)
    startTime := time.Now()
    
    if err := cmd.Run(); err != nil {
        s.saveTaskLog(task.ID, "plan", stderr.String(), "error")
        return fmt.Errorf("terraform plan failed: %w\n%s", err, stderr.String())
    }
    
    duration := time.Since(startTime)
    log.Printf("terraform plan completed in %v", duration)
    
    // 8. 保存Plan输出
    s.saveTaskLog(task.ID, "plan", stdout.String(), "info")
    
    // 9. 生成Plan JSON
    planJSON, err := s.GeneratePlanJSON(ctx, workDir, planFile)
    if err != nil {
        log.Printf("Warning: failed to generate plan JSON: %v", err)
    }
    
    // 10. 保存Plan数据到数据库
    if err := s.SavePlanData(task, planFile, planJSON); err != nil {
        return fmt.Errorf("failed to save plan data: %w", err)
    }
    
    // 11. 更新任务
    task.PlanOutput = stdout.String()
    task.Duration = int(duration.Seconds())
    s.db.Save(task)
    
    log.Printf("Task %d: Plan completed", task.ID)
    
    // 12. 进入Post-Plan阶段
    return s.TransitionToPostPlan(task)
}
```

### Stage 5: Post-Plan Stage（Plan后置处理）

**目的**: Plan执行后的分析和扩展点

**扩展能力**:
- Plan结果分析
- 变更通知
- 审批流程触发
- 自定义验证

**操作**:
```go
func (s *TerraformExecutor) HandlePostPlanStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StagePostPlan
    s.db.Save(task)
    
    // 2. 获取Post-Plan配置
    workspace := task.Context["workspace"].(*models.Workspace)
    runConfig := s.getRunConfig(workspace)
    
    if !runConfig.PostPlan.Enabled {
        log.Printf("Task %d: Post-Plan stage skipped", task.ID)
        return s.TransitionToCostEstimation(task)
    }
    
    // 3. 分析Plan结果
    planAnalysis := s.analyzePlanResult(task.PlanJSON)
    task.Context["plan_analysis"] = planAnalysis
    
    log.Printf("Task %d: Plan analysis - Add: %d, Change: %d, Destroy: %d",
        task.ID, planAnalysis.Add, planAnalysis.Change, planAnalysis.Destroy)
    
    // 4. 执行Post-Plan钩子
    for _, hook := range runConfig.PostPlan.Hooks {
        if err := s.executeHook(ctx, task, hook, "post_plan"); err != nil {
            return fmt.Errorf("post-plan hook failed: %w", err)
        }
    }
    
    // 5. 发送Plan完成通知
    s.notifySystem.Notify(models.EventPlanDone, workspace, task)
    
    // 6. 检查是否需要审批
    if runConfig.PostPlan.Metadata["require_approval"] == true {
        return s.WaitForApproval(task)
    }
    
    log.Printf("Task %d: Post-Plan completed", task.ID)
    
    // 7. 进入下一阶段
    return s.TransitionToCostEstimation(task)
}

// Plan结果分析
type PlanAnalysis struct {
    Add     int `json:"add"`
    Change  int `json:"change"`
    Destroy int `json:"destroy"`
}

func (s *TerraformExecutor) analyzePlanResult(planJSON map[string]interface{}) *PlanAnalysis {
    analysis := &PlanAnalysis{}
    
    if resourceChanges, ok := planJSON["resource_changes"].([]interface{}); ok {
        for _, rc := range resourceChanges {
            change := rc.(map[string]interface{})
            actions := change["change"].(map[string]interface{})["actions"].([]interface{})
            
            for _, action := range actions {
                switch action.(string) {
                case "create":
                    analysis.Add++
                case "update":
                    analysis.Change++
                case "delete":
                    analysis.Destroy++
                }
            }
        }
    }
    
    return analysis
}
```

### Stage 6: Cost Estimation Stage（成本估算）

**目的**: 估算基础设施变更的成本影响

**状态**: 未来扩展功能

**操作**:
```go
func (s *TerraformExecutor) HandleCostEstimationStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StageCostEstimation
    s.db.Save(task)
    
    // 2. 获取配置
    workspace := task.Context["workspace"].(*models.Workspace)
    runConfig := s.getRunConfig(workspace)
    
    if !runConfig.CostEstimation.Enabled {
        log.Printf("Task %d: Cost Estimation stage skipped", task.ID)
        return s.TransitionToPolicyCheck(task)
    }
    
    // 3. 调用成本估算服务（未来实现）
    // costEstimate, err := s.costEstimationService.Estimate(task.PlanJSON)
    // if err != nil {
    //     log.Printf("Cost estimation failed: %v", err)
    //     // 不阻塞流程
    // } else {
    //     task.Context["cost_estimate"] = costEstimate
    //     log.Printf("Task %d: Estimated cost change: $%.2f/month", 
    //         task.ID, costEstimate.MonthlyDelta)
    // }
    
    log.Printf("Task %d: Cost Estimation completed (skipped)", task.ID)
    
    // 4. 进入下一阶段
    return s.TransitionToPolicyCheck(task)
}
```

### Stage 7: Policy Check Stage（策略检查）

**目的**: 执行安全、合规策略检查

**状态**: 未来扩展功能

**扩展能力**:
- OPA (Open Policy Agent) 策略检查
- Sentinel策略检查
- 自定义合规规则
- 安全扫描

**操作**:
```go
func (s *TerraformExecutor) HandlePolicyCheckStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StagePolicyCheck
    s.db.Save(task)
    
    // 2. 获取配置
    workspace := task.Context["workspace"].(*models.Workspace)
    runConfig := s.getRunConfig(workspace)
    
    if !runConfig.PolicyCheck.Enabled {
        log.Printf("Task %d: Policy Check stage skipped", task.ID)
        return s.TransitionToPreApply(task)
    }
    
    // 3. 执行策略检查（未来实现）
    // policyResult, err := s.policyCheckService.Check(task.PlanJSON)
    // if err != nil {
    //     return fmt.Errorf("policy check failed: %w", err)
    // }
    //
    // if !policyResult.Passed {
    //     return fmt.Errorf("policy violations: %v", policyResult.Violations)
    // }
    //
    // task.Context["policy_result"] = policyResult
    
    log.Printf("Task %d: Policy Check completed (skipped)", task.ID)
    
    // 4. 进入下一阶段
    return s.TransitionToPreApply(task)
}
```

### Stage 8: Pre-Apply Stage（Apply前置处理）

**目的**: Apply执行前的最后检查和准备

**扩展能力**:
- 最终确认检查
- 备份当前State
- 通知发送
- 自定义前置脚本

**操作**:
```go
func (s *TerraformExecutor) HandlePreApplyStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StagePreApply
    s.db.Save(task)
    
    // 2. 获取配置
    workspace := task.Context["workspace"].(*models.Workspace)
    runConfig := s.getRunConfig(workspace)
    
    if !runConfig.PreApply.Enabled {
        log.Printf("Task %d: Pre-Apply stage skipped", task.ID)
        return s.TransitionToApplying(task)
    }
    
    // 3. 备份当前State
    if err := s.backupCurrentState(workspace); err != nil {
        log.Printf("Warning: failed to backup state: %v", err)
        // 不阻塞流程
    }
    
    // 4. 执行Pre-Apply钩子
    for _, hook := range runConfig.PreApply.Hooks {
        if err := s.executeHook(ctx, task, hook, "pre_apply"); err != nil {
            return fmt.Errorf("pre-apply hook failed: %w", err)
        }
    }
    
    // 5. 锁定Workspace
    if err := s.lockWorkspace(workspace.ID, *task.CreatedBy, "applying"); err != nil {
        return fmt.Errorf("failed to lock workspace: %w", err)
    }
    
    // 6. 发送Apply开始通知
    s.notifySystem.Notify(models.EventApplyStart, workspace, task)
    
    log.Printf("Task %d: Pre-Apply completed", task.ID)
    
    // 7. 进入Apply阶段
    return s.TransitionToApplying(task)
}
```

### Stage 9: Apply Stage（执行Apply）

**目的**: 执行terraform apply，实际创建/修改/删除资源

**操作**: （保持原有实现，添加阶段管理）

```go
func (s *TerraformExecutor) HandleApplyStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StageApplying
    s.db.Save(task)
    
    // 2. 准备工作目录
    workDir, err := s.PrepareWorkspace(task)
    if err != nil {
        return err
    }
    defer s.CleanupWorkspace(workDir)
    
    // 3. 获取配置
    workspace := task.Context["workspace"].(*models.Workspace)
    
    // 4. 生成配置文件
    if err := s.GenerateConfigFiles(workspace, workDir); err != nil {
        return err
    }
    
    // 5. 准备State文件
    if err := s.PrepareStateFile(workspace, workDir); err != nil {
        return err
    }
    
    // 6. Terraform初始化
    if err := s.TerraformInit(ctx, workDir, task); err != nil {
        return err
    }
    
    // 7. 从数据库恢复Plan文件
    planFile, err := s.RestorePlanFile(task, workDir)
    if err != nil {
        return fmt.Errorf("failed to restore plan file: %w", err)
    }
    
    // 8. 执行Apply
    cmd := exec.CommandContext(ctx, "terraform", "apply",
        "-no-color",
        "-auto-approve",
        planFile,
    )
    cmd.Dir = workDir
    cmd.Env = append(os.Environ(),
        "TF_IN_AUTOMATION=true",
        "TF_INPUT=false",
    )
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    log.Printf("Executing: terraform apply in %s", workDir)
    startTime := time.Now()
    
    if err := cmd.Run(); err != nil {
        s.saveTaskLog(task.ID, "apply", stderr.String(), "error")
        return fmt.Errorf("terraform apply failed: %w\n%s", err, stderr.String())
    }
    
    duration := time.Since(startTime)
    log.Printf("terraform apply completed in %v", duration)
    
    // 9. 保存Apply输出
    s.saveTaskLog(task.ID, "apply", stdout.String(), "info")
    
    // 10. 保存新的State版本
    if err := s.SaveNewStateVersion(workspace, task, workDir); err != nil {
        return fmt.Errorf("failed to save state: %w", err)
    }
    
    // 11. 更新任务
    task.ApplyOutput = stdout.String()
    task.Duration += int(duration.Seconds())
    s.db.Save(task)
    
    log.Printf("Task %d: Apply completed", task.ID)
    
    // 12. 进入Post-Apply阶段
    return s.TransitionToPostApply(task)
}
```

### Stage 10: Post-Apply Stage（Apply后置处理）

**目的**: Apply执行后的清理和扩展点

**扩展能力**:
- 资源验证
- 健康检查
- 通知发送
- 自定义后置脚本
- 文档生成

**操作**:
```go
func (s *TerraformExecutor) HandlePostApplyStage(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段
    task.Stage = StagePostApply
    s.db.Save(task)
    
    // 2. 获取配置
    workspace := task.Context["workspace"].(*models.Workspace)
    runConfig := s.getRunConfig(workspace)
    
    if !runConfig.PostApply.Enabled {
        log.Printf("Task %d: Post-Apply stage skipped", task.ID)
        return s.TransitionToCompleted(task)
    }
    
    // 3. 执行Post-Apply钩子
    for _, hook := range runConfig.PostApply.Hooks {
        if err := s.executeHook(ctx, task, hook, "post_apply"); err != nil {
            log.Printf("Warning: post-apply hook failed: %v", err)
            // 不阻塞流程
        }
    }
    
    // 4. 提取Terraform outputs
    outputs, err := s.extractTerraformOutputs(workspace)
    if err != nil {
        log.Printf("Warning: failed to extract outputs: %v", err)
    } else {
        task.Context["outputs"] = outputs
    }
    
    // 5. 解锁Workspace
    if err := s.unlockWorkspace(workspace.ID); err != nil {
        log.Printf("Warning: failed to unlock workspace: %v", err)
    }
    
    // 6. 发送Apply完成通知
    s.notifySystem.Notify(models.EventCompleted, workspace, task)
    
    log.Printf("Task %d: Post-Apply completed", task.ID)
    
    // 7. 进入完成阶段
    return s.TransitionToCompleted(task)
}
```

### Stage 11: Completion（任务完成）

**目的**: 标记任务完成，清理资源

**操作**:
```go
func (s *TerraformExecutor) HandleCompletionStage(
    task *models.WorkspaceTask,
) error {
    // 1. 更新阶段和状态
    task.Stage = StageCompleted
    task.Status = models.TaskStatusSuccess
    task.CompletedAt = timePtr(time.Now())
    
    // 2. 记录指标
    s.RecordTaskMetrics(task)
    
    // 3. 保存任务
    if err := s.db.Save(task).Error; err != nil {
        return fmt.Errorf("failed to save task: %w", err)
    }
    
    // 4. 更新Workspace状态
    workspace := task.Context["workspace"].(*models.Workspace)
    workspace.State = models.WorkspaceStateCompleted
    workspace.LastRunAt = timePtr(time.Now())
    
    if err := s.db.Save(workspace).Error; err != nil {
        return fmt.Errorf("failed to update workspace: %w", err)
    }
    
    log.Printf("Task %d: Completed successfully", task.ID)
    
    return nil
}
```

## 🔌 钩子系统设计

### 钩子执行器

```go
// 钩子类型
type HookType string

const (
    HookTypeScript   HookType = "script"   // Shell脚本
    HookTypeHTTP     HookType = "http"     // HTTP请求
    HookTypeFunction HookType = "function" // Go函数
)

// 钩子定义
type Hook struct {
    Name    string                 `json:"name"`
    Type    HookType               `json:"type"`
    Content string                 `json:"content"` // 脚本内容或URL
    Timeout int                    `json:"timeout"` // 超时时间（秒）
    Env     map[string]string      `json:"env"`     // 环境变量
    Params  map[string]interface{} `json:"params"`  // 参数
}

// 执行钩子
func (s *TerraformExecutor) executeHook(
    ctx context.Context,
    task *models.WorkspaceTask,
    hookName string,
    stage string,
) error {
    // 1. 获取钩子配置
    hook, err := s.getHook(hookName)
    if err != nil {
        return fmt.Errorf("hook not found: %s", hookName)
    }
    
    // 2. 设置超时
    timeout := time.Duration(hook.Timeout) * time.Second
    if timeout == 0 {
        timeout = 5 * time.Minute
    }
    
    hookCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    // 3. 根据类型执行
    switch hook.Type {
    case HookTypeScript:
        return s.executeScriptHook(hookCtx, task, hook, stage)
    case HookTypeHTTP:
        return s.executeHTTPHook(hookCtx, task, hook, stage)
    case HookTypeFunction:
        return s.executeFunctionHook(hookCtx, task, hook, stage)
    default:
        return fmt.Errorf("unknown hook type: %s", hook.Type)
    }
}

// 执行脚本钩子
func (s *TerraformExecutor) executeScriptHook(
    ctx context.Context,
    task *models.WorkspaceTask,
    hook *Hook,
    stage string,
) error {
    // 创建临时脚本文件
    tmpFile, err := os.CreateTemp("", "hook-*.sh")
    if err != nil {
        return err
    }
    defer os.Remove(tmpFile.Name())
    
    if _, err := tmpFile.WriteString(hook.Content); err != nil {
        return err
    }
    tmpFile.Close()
    
    // 执行脚本
    cmd := exec.CommandContext(ctx, "bash", tmpFile.Name())
    
    // 设置环境变量
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("TASK_ID=%d", task.ID),
        fmt.Sprintf("WORKSPACE_ID=%d", task.WorkspaceID),
        fmt.Sprintf("STAGE=%s", stage),
    )
    
    // 添加自定义环境变量
    for k, v := range hook.Env {
        cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
    }
    
    // 执行并捕获输出
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("script hook failed: %w\nOutput: %s", err, string(output))
    }
    
    log.Printf("Hook %s executed successfully: %s", hook.Name, string(output))
    return nil
}

// 执行HTTP钩子
func (s *TerraformExecutor) executeHTTPHook(
    ctx context.Context,
    task *models.WorkspaceTask,
    hook *Hook,
    stage string,
) error {
    // 构建请求体
    payload := map[string]interface{}{
        "task_id":      task.ID,
        "workspace_id": task.WorkspaceID,
        "stage":        stage,
        "params":       hook.Params,
    }
    
    jsonData, err := json.Marshal(payload)
    if err != nil {
        return err
    }
    
    // 创建HTTP请求
    req, err := http.NewRequestWithContext(ctx, "POST", hook.Content, bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    // 执行请求
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("HTTP hook failed: %w", err)
    }
    defer resp.Body.Close()
    
    // 检查响应
    if resp.StatusCode >= 400 {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("HTTP hook returned error: %d - %s", resp.StatusCode, string(body))
    }
    
    log.Printf("Hook %s executed successfully (HTTP %d)", hook.Name, resp.StatusCode)
    return nil
}

// 执行函数钩子
func (s *TerraformExecutor) executeFunctionHook(
    ctx context.Context,
    task *models.WorkspaceTask,
    hook *Hook,
    stage string,
) error {
    // 从注册表获取函数
    fn, exists := s.hookRegistry[hook.Content]
    if !exists {
        return fmt.Errorf("function hook not registered: %s", hook.Content)
    }
    
    // 执行函数
    if err := fn(ctx, task, stage, hook.Params); err != nil {
        return fmt.Errorf("function hook failed: %w", err)
    }
    
    log.Printf("Hook %s executed successfully (function)", hook.Name)
    return nil
}
```

## 🎯 阶段转换管理

### 状态转换函数

```go
// 阶段转换管理器
type StageTransitionManager struct {
    executor *TerraformExecutor
}

// 转换到Fetching阶段
func (m *StageTransitionManager) TransitionToFetching(task *models.WorkspaceTask) error {
    task.Stage = StageFetching
    task.Status = models.TaskStatusRunning
    m.executor.db.Save(task)
    
    return m.executor.HandleFetchingStage(context.Background(), task)
}

// 转换到PrePlan阶段
func (m *StageTransitionManager) TransitionToPrePlan(task *models.WorkspaceTask) error {
    task.Stage = StagePrePlan
    m.executor.db.Save(task)
    
    return m.executor.HandlePrePlanStage(context.Background(), task)
}

// 转换到Planning阶段
func (m *StageTransitionManager) TransitionToPlanning(task *models.WorkspaceTask) error {
    task.Stage = StagePlanning
    m.executor.db.Save(task)
    
    return m.executor.HandlePlanStage(context.Background(), task)
}

// 转换到PostPlan阶段
func (m *StageTransitionManager) TransitionToPostPlan(task *models.WorkspaceTask) error {
    task.Stage = StagePostPlan
    m.executor.db.Save(task)
    
    return m.executor.HandlePostPlanStage(context.Background(), task)
}

// 转换到CostEstimation阶段
func (m *StageTransitionManager) TransitionToCostEstimation(task *models.WorkspaceTask) error {
    task.Stage = StageCostEstimation
    m.executor.db.Save(task)
    
    return m.executor.HandleCostEstimationStage(context.Background(), task)
}

// 转换到PolicyCheck阶段
func (m *StageTransitionManager) TransitionToPolicyCheck(task *models.WorkspaceTask) error {
    task.Stage = StagePolicyCheck
    m.executor.db.Save(task)
    
    return m.executor.HandlePolicyCheckStage(context.Background(), task)
}

// 转换到PreApply阶段
func (m *StageTransitionManager) TransitionToPreApply(task *models.WorkspaceTask) error {
    task.Stage = StagePreApply
    m.executor.db.Save(task)
    
    return m.executor.HandlePreApplyStage(context.Background(), task)
}

// 转换到Applying阶段
func (m *StageTransitionManager) TransitionToApplying(task *models.WorkspaceTask) error {
    task.Stage = StageApplying
    m.executor.db.Save(task)
    
    return m.executor.HandleApplyStage(context.Background(), task)
}

// 转换到PostApply阶段
func (m *StageTransitionManager) TransitionToPostApply(task *models.WorkspaceTask) error {
    task.Stage = StagePostApply
    m.executor.db.Save(task)
    
    return m.executor.HandlePostApplyStage(context.Background(), task)
}

// 转换到Completed阶段
func (m *StageTransitionManager) TransitionToCompleted(task *models.WorkspaceTask) error {
    task.Stage = StageCompleted
    m.executor.db.Save(task)
    
    return m.executor.HandleCompletionStage(task)
}
```

## 📊 阶段配置示例

### Workspace运行配置

```json
{
  "run_config": {
    "pre_plan": {
      "enabled": true,
      "timeout": 300,
      "hooks": ["syntax-check", "security-scan"],
      "metadata": {
        "syntax_check": true,
        "scan_level": "high"
      }
    },
    "post_plan": {
      "enabled": true,
      "timeout": 600,
      "hooks": ["plan-analysis", "notify-team"],
      "metadata": {
        "require_approval": true,
        "approval_timeout": 3600
      }
    },
    "cost_estimation": {
      "enabled": false,
      "timeout": 300,
      "hooks": [],
      "metadata": {}
    },
    "policy_check": {
      "enabled": false,
      "timeout": 600,
      "hooks": [],
      "metadata": {}
    },
    "pre_apply": {
      "enabled": true,
      "timeout": 300,
      "hooks": ["backup-state", "notify-start"],
      "metadata": {
        "backup_enabled": true
      }
    },
    "post_apply": {
      "enabled": true,
      "timeout": 600,
      "hooks": ["health-check", "notify-complete"],
      "metadata": {
        "health_check_enabled": true,
        "generate_docs": true
      }
    }
  }
}
```

## 🔄 完整执行流程示例

### Plan任务完整流程

```go
func (s *TerraformExecutor) ExecutePlanTask(taskID uint) error {
    // 1. 获取任务
    var task models.WorkspaceTask
    if err := s.db.First(&task, taskID).Error; err != nil {
        return err
    }
    
    // 2. Pending阶段
    if err := s.HandlePendingStage(&task); err != nil {
        return s.handleStageError(&task, err)
    }
    
    // 3. Fetching阶段
    if err := s.HandleFetchingStage(context.Background(), &task); err != nil {
        return s.handleStageError(&task, err)
    }
    
    // 4. Pre-Plan阶段
    if err := s.HandlePrePlanStage(context.Background(), &task); err != nil {
        return s.handleStageError(&task, err)
    }
    
    // 5. Plan阶段
    if err := s.HandlePlanStage(context.Background(), &task); err != nil {
        return s.handleStageError(&task, err)
    }
    
    // 6. Post-Plan阶段
    if err := s.HandlePostPlanStage(context.Background(), &task); err != nil {
        return s.handleStageError(&task, err)
    }
    
    // 7. Cost Estimation阶段（可选）
    if err := s.HandleCostEstimationStage(context.Background(), &task); err != nil {
        return s.handleStageError(&task, err)
    }
    
    // 8. Policy Check阶段（可选）
    if err := s.HandlePolicyCheckStage(context.Background(), &task); err != nil {
        return s.handleStageError(&task, err)
    }
    
    // 9. 完成
    return s.HandleCompletionStage(&task)
}

// 处理阶段错误
func (s *TerraformExecutor) handleStageError(task *models.WorkspaceTask, err error) error {
    task.Stage = StageFailed
    task.Status = models.TaskStatusFailed
    task.ErrorMessage = err.Error()
    task.CompletedAt = timePtr(time.Now())
    
    s.db.Save(task)
    
    // 发送失败通知
    workspace := task.Context["workspace"].(*models.Workspace)
    s.notifySystem.Notify(models.EventFailed, workspace, task)
    
    return err
}
```

## 📝 数据库Schema更新

### workspace_tasks表添加stage字段

```sql
-- 添加stage字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS stage VARCHAR(30) DEFAULT 'pending';

-- 添加context字段（存储阶段上下文）
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS context JSONB;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_stage ON workspace_tasks(stage);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_stage_status ON workspace_tasks(stage, status);
```

### workspaces表添加run_config字段

```sql
-- 添加运行配置字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS run_config JSONB;

-- 设置默认配置
UPDATE workspaces 
SET run_config = '{
  "pre_plan": {"enabled": false, "timeout": 300, "hooks": [], "metadata": {}},
  "post_plan": {"enabled": false, "timeout": 600, "hooks": [], "metadata": {}},
  "cost_estimation": {"enabled": false, "timeout": 300, "hooks": [], "metadata": {}},
  "policy_check": {"enabled": false, "timeout": 600, "hooks": [], "metadata": {}},
  "pre_apply": {"enabled": false, "timeout": 300, "hooks": [], "metadata": {}},
  "post_apply": {"enabled": false, "timeout": 600, "hooks": [], "metadata": {}}
}'::jsonb
WHERE run_config IS NULL;
```

## 🎯 总结

### 核心改进

1. ** 完整的11阶段执行流程** - 参考TFE标准，支持完整的Run Lifecycle
2. ** 灵活的钩子系统** - 支持Script/HTTP/Function三种钩子类型
3. ** 可配置的阶段控制** - 每个阶段可独立启用/禁用
4. ** 扩展性设计** - Pre/Post阶段为未来功能预留扩展点
5. ** 成本估算和策略检查** - 为未来功能预留接口

### 实施优先级

**Phase 1 (核心阶段)**:
- Pending → Fetching → Planning → Applying → Completion

**Phase 2 (扩展阶段)**:
- Pre-Plan → Post-Plan → Pre-Apply → Post-Apply

**Phase 3 (高级功能)**:
- Cost Estimation → Policy Check

---

**文档已完整更新，包含TFE标准的11个执行阶段！** 🚀

```go
func (s *TerraformExecutor) PrepareWorkspace(task *models.WorkspaceTask) (string, error) {
    // 1. 创建工作目录
    workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%d/%d", 
        task.WorkspaceID, task.ID)
    
    if err := os.MkdirAll(workDir, 0755); err != nil {
        return "", fmt.Errorf("failed to create work directory: %w", err)
    }
    
    log.Printf("Created work directory: %s", workDir)
    return workDir, nil
}
```

#### 1.2 生成配置文件（最终修复版本）

```go
func (s *TerraformExecutor) GenerateConfigFiles(
    workspace *models.Workspace, 
    workDir string,
) error {
    // 1. 生成 main.tf.json
    if err := s.writeJSONFile(workDir, "main.tf.json", workspace.TFCode); err != nil {
        return fmt.Errorf("failed to write main.tf.json: %w", err)
    }
    
    // 2. 生成 provider.tf.json
    if err := s.writeJSONFile(workDir, "provider.tf.json", workspace.ProviderConfig); err != nil {
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
    
    log.Printf("Generated all config files in %s", workDir)
    return nil
}

// 生成variables.tf.json
func (s *TerraformExecutor) generateVariablesTFJSON(
    workspace *models.Workspace,
    workDir string,
) error {
    variables := make(map[string]interface{})
    
    // 从workspace_variables表获取变量定义
    var workspaceVars []models.WorkspaceVariable
    s.db.Where("workspace_id = ?", workspace.ID).Find(&workspaceVars)
    
    for _, v := range workspaceVars {
        varDef := map[string]interface{}{
            "type": v.Type,
        }
        
        if v.Description != "" {
            varDef["description"] = v.Description
        }
        
        if v.Sensitive {
            varDef["sensitive"] = true
        }
        
        // 不设置default，让用户通过tfvars赋值
        variables[v.Key] = varDef
    }
    
    config := map[string]interface{}{
        "variable": variables,
    }
    
    return s.writeJSONFile(workDir, "variables.tf.json", config)
}

// 生成variables.tfvars（修复版本）
func (s *TerraformExecutor) generateVariablesTFVars(
    workspace *models.Workspace,
    workDir string,
) error {
    var tfvars strings.Builder
    
    // 从workspace_variables表获取变量值
    var workspaceVars []models.WorkspaceVariable
    s.db.Where("workspace_id = ?", workspace.ID).Find(&workspaceVars)
    
    for _, v := range workspaceVars {
        switch v.Type {
        case "string":
            // 转义特殊字符
            escapedValue := strings.ReplaceAll(v.Value, "\"", "\\\"")
            escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
            tfvars.WriteString(fmt.Sprintf("%s = \"%s\"\n", v.Key, escapedValue))
            
        case "number", "bool":
            // 数字和布尔值直接使用
            tfvars.WriteString(fmt.Sprintf("%s = %s\n", v.Key, v.Value))
            
        case "list", "map", "object":
            // 复杂类型：v.Value已经是JSON字符串
            // Terraform支持在tfvars中使用JSON格式
            tfvars.WriteString(fmt.Sprintf("%s = %s\n", v.Key, v.Value))
            
        default:
            log.Printf("Warning: unsupported variable type: %s", v.Type)
        }
    }
    
    return s.writeFile(workDir, "variables.tfvars", tfvars.String())
}
```

#### 1.3 准备State文件

```go
func (s *TerraformExecutor) PrepareStateFile(
    workspace *models.Workspace,
    workDir string,
) error {
    // 1. 获取最新的State版本
    var stateVersion models.WorkspaceStateVersion
    err := s.db.Where("workspace_id = ?", workspace.ID).
        Order("version DESC").
        First(&stateVersion).Error
    
    if err == gorm.ErrRecordNotFound {
        // 首次执行，没有State文件
        log.Printf("No existing state for workspace %d", workspace.ID)
        return nil
    }
    
    if err != nil {
        return fmt.Errorf("failed to get state version: %w", err)
    }
    
    // 2. 写入State文件
    stateFile := filepath.Join(workDir, "terraform.tfstate")
    
    // 从数据库JSONB字段读取State内容
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
```

### Phase 2: Terraform初始化

```go
func (s *TerraformExecutor) TerraformInit(
    ctx context.Context,
    workDir string,
    task *models.WorkspaceTask,
) error {
    // 1. 构建init命令
    cmd := exec.CommandContext(ctx, "terraform", "init", "-no-color")
    cmd.Dir = workDir
    
    // 2. 设置环境变量
    cmd.Env = append(os.Environ(),
        "TF_IN_AUTOMATION=true",
        "TF_INPUT=false",
    )
    
    // 3. 捕获输出
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    // 4. 执行命令
    log.Printf("Executing: terraform init in %s", workDir)
    startTime := time.Now()
    
    if err := cmd.Run(); err != nil {
        // 保存错误日志
        s.saveTaskLog(task.ID, "init", stderr.String(), "error")
        return fmt.Errorf("terraform init failed: %w\n%s", err, stderr.String())
    }
    
    duration := time.Since(startTime)
    log.Printf("terraform init completed in %v", duration)
    
    // 5. 保存成功日志
    s.saveTaskLog(task.ID, "init", stdout.String(), "info")
    
    return nil
}
```

### Phase 3: Plan任务执行

```go
func (s *TerraformExecutor) ExecutePlan(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 准备工作目录
    workDir, err := s.PrepareWorkspace(task)
    if err != nil {
        return err
    }
    defer s.CleanupWorkspace(workDir) // 确保清理
    
    // 2. 获取Workspace配置
    var workspace models.Workspace
    if err := s.db.First(&workspace, task.WorkspaceID).Error; err != nil {
        return fmt.Errorf("failed to get workspace: %w", err)
    }
    
    // 3. 生成配置文件
    if err := s.GenerateConfigFiles(&workspace, workDir); err != nil {
        return err
    }
    
    // 4. 准备State文件
    if err := s.PrepareStateFile(&workspace, workDir); err != nil {
        return err
    }
    
    // 5. Terraform初始化
    if err := s.TerraformInit(ctx, workDir, task); err != nil {
        return err
    }
    
    // 6. 执行Plan
    planFile := filepath.Join(workDir, "plan.out")
    cmd := exec.CommandContext(ctx, "terraform", "plan",
        "-out="+planFile,
        "-no-color",
        "-var-file=variables.tfvars",
    )
    cmd.Dir = workDir
    cmd.Env = append(os.Environ(),
        "TF_IN_AUTOMATION=true",
        "TF_INPUT=false",
    )
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    log.Printf("Executing: terraform plan in %s", workDir)
    startTime := time.Now()
    
    if err := cmd.Run(); err != nil {
        s.saveTaskLog(task.ID, "plan", stderr.String(), "error")
        return fmt.Errorf("terraform plan failed: %w\n%s", err, stderr.String())
    }
    
    duration := time.Since(startTime)
    log.Printf("terraform plan completed in %v", duration)
    
    // 7. 保存Plan输出
    s.saveTaskLog(task.ID, "plan", stdout.String(), "info")
    
    // 8. 生成Plan JSON
    planJSON, err := s.GeneratePlanJSON(ctx, workDir, planFile)
    if err != nil {
        log.Printf("Warning: failed to generate plan JSON: %v", err)
        // 不影响Plan成功
    }
    
    // 9. 保存Plan数据到数据库（关键！）
    if err := s.SavePlanData(task, planFile, planJSON); err != nil {
        return fmt.Errorf("failed to save plan data: %w", err)
    }
    
    // 10. 更新任务状态
    task.Status = models.TaskStatusSuccess
    task.PlanOutput = stdout.String()
    task.CompletedAt = timePtr(time.Now())
    task.Duration = int(duration.Seconds())
    
    if err := s.db.Save(task).Error; err != nil {
        return fmt.Errorf("failed to update task: %w", err)
    }
    
    return nil
}

// 生成Plan JSON格式
func (s *TerraformExecutor) GeneratePlanJSON(
    ctx context.Context,
    workDir string,
    planFile string,
) (map[string]interface{}, error) {
    // 使用terraform show -json命令
    cmd := exec.CommandContext(ctx, "terraform", "show", "-json", planFile)
    cmd.Dir = workDir
    
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }
    
    var planJSON map[string]interface{}
    if err := json.Unmarshal(output, &planJSON); err != nil {
        return nil, err
    }
    
    return planJSON, nil
}

// 保存Plan数据到数据库
func (s *TerraformExecutor) SavePlanData(
    task *models.WorkspaceTask,
    planFile string,
    planJSON map[string]interface{},
) error {
    // 1. 读取Plan二进制文件
    planData, err := os.ReadFile(planFile)
    if err != nil {
        return fmt.Errorf("failed to read plan file: %w", err)
    }
    
    // 2. 保存到任务记录
    task.PlanData = planData // []byte字段
    task.PlanJSON = planJSON // JSONB字段
    
    if err := s.db.Save(task).Error; err != nil {
        return fmt.Errorf("failed to save plan data: %w", err)
    }
    
    log.Printf("Saved plan data for task %d (size: %d bytes)", 
        task.ID, len(planData))
    
    return nil
}
```

### Phase 4: Apply任务执行

```go
func (s *TerraformExecutor) ExecuteApply(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 准备工作目录
    workDir, err := s.PrepareWorkspace(task)
    if err != nil {
        return err
    }
    defer s.CleanupWorkspace(workDir)
    
    // 2. 获取Workspace配置
    var workspace models.Workspace
    if err := s.db.First(&workspace, task.WorkspaceID).Error; err != nil {
        return fmt.Errorf("failed to get workspace: %w", err)
    }
    
    // 3. 生成配置文件
    if err := s.GenerateConfigFiles(&workspace, workDir); err != nil {
        return err
    }
    
    // 4. 准备State文件
    if err := s.PrepareStateFile(&workspace, workDir); err != nil {
        return err
    }
    
    // 5. Terraform初始化
    if err := s.TerraformInit(ctx, workDir, task); err != nil {
        return err
    }
    
    // 6. 从数据库恢复Plan文件（关键！强制使用数据库Plan）
    planFile, err := s.RestorePlanFile(task, workDir)
    if err != nil {
        return fmt.Errorf("failed to restore plan file: %w", err)
    }
    
    // 7. 执行Apply
    cmd := exec.CommandContext(ctx, "terraform", "apply",
        "-no-color",
        "-auto-approve",
        planFile,
    )
    cmd.Dir = workDir
    cmd.Env = append(os.Environ(),
        "TF_IN_AUTOMATION=true",
        "TF_INPUT=false",
    )
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    log.Printf("Executing: terraform apply in %s", workDir)
    startTime := time.Now()
    
    if err := cmd.Run(); err != nil {
        s.saveTaskLog(task.ID, "apply", stderr.String(), "error")
        return fmt.Errorf("terraform apply failed: %w\n%s", err, stderr.String())
    }
    
    duration := time.Since(startTime)
    log.Printf("terraform apply completed in %v", duration)
    
    // 8. 保存Apply输出
    s.saveTaskLog(task.ID, "apply", stdout.String(), "info")
    
    // 9. 保存新的State版本
    if err := s.SaveNewStateVersion(&workspace, task, workDir); err != nil {
        // State保存失败是严重错误
        return fmt.Errorf("failed to save state: %w", err)
    }
    
    // 10. 更新任务状态
    task.Status = models.TaskStatusSuccess
    task.ApplyOutput = stdout.String()
    task.CompletedAt = timePtr(time.Now())
    task.Duration = int(duration.Seconds())
    
    if err := s.db.Save(task).Error; err != nil {
        return fmt.Errorf("failed to update task: %w", err)
    }
    
    return nil
}

// 从数据库恢复Plan文件
func (s *TerraformExecutor) RestorePlanFile(
    task *models.WorkspaceTask,
    workDir string,
) (string, error) {
    // 1. 获取最近的成功Plan任务
    var planTask models.WorkspaceTask
    err := s.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
        task.WorkspaceID, models.TaskTypePlan, models.TaskStatusSuccess).
        Order("created_at DESC").
        First(&planTask).Error
    
    if err != nil {
        return "", fmt.Errorf("no successful plan task found: %w", err)
    }
    
    // 2. 检查Plan数据是否存在
    if len(planTask.PlanData) == 0 {
        return "", fmt.Errorf("plan data is empty for task %d", planTask.ID)
    }
    
    // 3. 写入Plan文件
    planFile := filepath.Join(workDir, "plan.out")
    if err := os.WriteFile(planFile, planTask.PlanData, 0644); err != nil {
        return "", fmt.Errorf("failed to write plan file: %w", err)
    }
    
    log.Printf("Restored plan file from task %d (size: %d bytes)",
        planTask.ID, len(planTask.PlanData))
    
    return planFile, nil
}

// 保存新的State版本
func (s *TerraformExecutor) SaveNewStateVersion(
    workspace *models.Workspace,
    task *models.WorkspaceTask,
    workDir string,
) error {
    // 1. 读取State文件
    stateFile := filepath.Join(workDir, "terraform.tfstate")
    stateData, err := os.ReadFile(stateFile)
    if err != nil {
        return fmt.Errorf("failed to read state file: %w", err)
    }
    
    // 2. 解析State JSON
    var stateContent map[string]interface{}
    if err := json.Unmarshal(stateData, &stateContent); err != nil {
        return fmt.Errorf("failed to parse state: %w", err)
    }
    
    // 3. 计算checksum
    checksum := s.calculateChecksum(stateData)
    
    // 4. 获取当前最大版本号
    var maxVersion int
    s.db.Model(&models.WorkspaceStateVersion{}).
        Where("workspace_id = ?", workspace.ID).
        Select("COALESCE(MAX(version), 0)").
        Scan(&maxVersion)
    
    newVersion := maxVersion + 1
    
    // 5. 创建新版本记录
    stateVersion := &models.WorkspaceStateVersion{
        WorkspaceID: workspace.ID,
        Version:     newVersion,
        Content:     stateContent, // JSONB字段
        Checksum:    checksum,
        SizeBytes:   len(stateData),
        TaskID:      &task.ID,
        CreatedBy:   task.CreatedBy,
    }
    
    if err := s.db.Create(stateVersion).Error; err != nil {
        return fmt.Errorf("failed to create state version: %w", err)
    }
    
    // 6. 更新Workspace的当前State版本
    workspace.CurrentStateID = &stateVersion.ID
    workspace.CurrentVersion = newVersion
    
    if err := s.db.Save(workspace).Error; err != nil {
        return fmt.Errorf("failed to update workspace: %w", err)
    }
    
    log.Printf("Saved state version %d for workspace %d (size: %d bytes)",
        newVersion, workspace.ID, len(stateData))
    
    return nil
}
```

### Phase 5: 资源清理

```go
func (s *TerraformExecutor) CleanupWorkspace(workDir string) error {
    // 1. 检查目录是否存在
    if _, err := os.Stat(workDir); os.IsNotExist(err) {
        return nil
    }
    
    // 2. 删除整个工作目录
    if err := os.RemoveAll(workDir); err != nil {
        log.Printf("Warning: failed to cleanup workspace %s: %v", workDir, err)
        return err
    }
    
    log.Printf("Cleaned up workspace: %s", workDir)
    return nil
}

// 定期清理旧的工作目录
func (s *TerraformExecutor) CleanupOldWorkspaces() error {
    baseDir := "/tmp/iac-platform/workspaces"
    
    // 遍历所有workspace目录
    workspaces, err := os.ReadDir(baseDir)
    if err != nil {
        return err
    }
    
    now := time.Now()
    for _, ws := range workspaces {
        if !ws.IsDir() {
            continue
        }
        
        wsPath := filepath.Join(baseDir, ws.Name())
        
        // 遍历任务目录
        tasks, err := os.ReadDir(wsPath)
        if err != nil {
            continue
        }
        
        for _, task := range tasks {
            if !task.IsDir() {
                continue
            }
            
            taskPath := filepath.Join(wsPath, task.Name())
            info, err := os.Stat(taskPath)
            if err != nil {
                continue
            }
            
            // 删除超过1小时的目录
            if now.Sub(info.ModTime()) > time.Hour {
                os.RemoveAll(taskPath)
                log.Printf("Cleaned up old task directory: %s", taskPath)
            }
        }
    }
    
    return nil
}
```

## 📊 日志管理

### 日志存储策略

```go
// 日志记录结构
type TaskLog struct {
    ID        uint      `gorm:"primaryKey"`
    TaskID    uint      `gorm:"index;not null"`
    Phase     string    `gorm:"type:varchar(20);not null"` // init, plan, apply
    Content   string    `gorm:"type:text"`
    Level     string    `gorm:"type:varchar(10)"` // info, error, warning
    CreatedAt time.Time `gorm:"autoCreateTime"`
}

// 保存任务日志
func (s *TerraformExecutor) saveTaskLog(
    taskID uint,
    phase string,
    content string,
    level string,
) error {
    log := &TaskLog{
        TaskID:  taskID,
        Phase:   phase,
        Content: content,
        Level:   level,
    }
    
    return s.db.Create(log).Error
}

// 获取任务日志
func (s *TerraformExecutor) GetTaskLogs(taskID uint) ([]TaskLog, error) {
    var logs []TaskLog
    err := s.db.Where("task_id = ?", taskID).
        Order("created_at ASC").
        Find(&logs).Error
    
    return logs, err
}
```

### 实时日志流（WebSocket）

```go
// WebSocket日志推送
func (s *TerraformExecutor) StreamLogs(
    ctx context.Context,
    taskID uint,
    ws *websocket.Conn,
) error {
    // 1. 发送历史日志
    logs, err := s.GetTaskLogs(taskID)
    if err != nil {
        return err
    }
    
    for _, log := range logs {
        if err := ws.WriteJSON(log); err != nil {
            return err
        }
    }
    
    // 2. 订阅新日志（使用channel）
    logChan := s.subscribeTaskLogs(taskID)
    defer s.unsubscribeTaskLogs(taskID)
    
    for {
        select {
        case <-ctx.Done():
            return nil
        case log := <-logChan:
            if err := ws.WriteJSON(log); err != nil {
                return err
            }
        }
    }
}
```

## 🔒 并发控制和锁机制

### Workspace锁定

```go
// 锁定Workspace
func (s *WorkspaceService) LockWorkspace(
    workspaceID uint,
    userID uint,
    reason string,
) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        var workspace models.Workspace
        
        // 使用行锁
        if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
            First(&workspace, workspaceID).Error; err != nil {
            return err
        }
        
        // 检查是否已锁定
        if workspace.IsLocked {
            return fmt.Errorf("workspace is already locked by user %d", 
                *workspace.LockedBy)
        }
        
        // 锁定
        now := time.Now()
        workspace.IsLocked = true
        workspace.LockedBy = &userID
        workspace.LockedAt = &now
        workspace.LockReason = reason
        
        return tx.Save(&workspace).Error
    })
}

// 解锁Workspace
func (s *WorkspaceService) UnlockWorkspace(workspaceID uint) error {
    return s.db.Model(&models.Workspace{}).
        Where("id = ?", workspaceID).
        Updates(map[string]interface{}{
            "is_locked":   false,
            "locked_by":   nil,
            "locked_at":   nil,
            "lock_reason": "",
        }).Error
}
```

## 🚨 错误处理和重试

### 错误分类

```go
type ErrorType string

const (
    ErrorTypeRetryable    ErrorType = "retryable"    // 可重试
    ErrorTypeNonRetryable ErrorType = "non_retryable" // 不可重试
    ErrorTypeFatal        ErrorType = "fatal"         // 致命错误
)

// 错误分类函数
func (s *TerraformExecutor) ClassifyError(err error) ErrorType {
    errMsg := err.Error()
    
    // 网络相关错误 - 可重试
    if strings.Contains(errMsg, "timeout") ||
       strings.Contains(errMsg, "connection refused") ||
       strings.Contains(errMsg, "temporary failure") {
        return ErrorTypeRetryable
    }
    
    // Provider临时错误 - 可重试
    if strings.Contains(errMsg, "rate limit") ||
       strings.Contains(errMsg, "throttling") ||
       strings.Contains(errMsg, "service unavailable") {
        return ErrorTypeRetryable
    }
    
    // 配置错误 - 不可重试
    if strings.Contains(errMsg, "syntax error") ||
       strings.Contains(errMsg, "invalid configuration") ||
       strings.Contains(errMsg, "missing required") {
        return ErrorTypeNonRetryable
    }
    
    // 权限错误 - 不可重试
    if strings.Contains(errMsg, "access denied") ||
       strings.Contains(errMsg, "unauthorized") ||
       strings.Contains(errMsg, "permission denied") {
        return ErrorTypeNonRetryable
    }
    
    // State冲突 - 不可重试
    if strings.Contains(errMsg, "state locked") ||
       strings.Contains(errMsg, "state conflict") {
        return ErrorTypeNonRetryable
    }
    
    // 默认为可重试
    return ErrorTypeRetryable
}
```

### 重试策略

```go
// 重试配置
type RetryConfig struct {
    MaxRetries     int           // 最大重试次数
    InitialDelay   time.Duration // 初始延迟
    MaxDelay       time.Duration // 最大延迟
    BackoffFactor  float64       // 退避因子
}

var DefaultRetryConfig = RetryConfig{
    MaxRetries:    3,
    InitialDelay:  5 * time.Second,
    MaxDelay:      60 * time.Second,
    BackoffFactor: 2.0,
}

// 执行任务带重试
func (s *TerraformExecutor) ExecuteWithRetry(
    ctx context.Context,
    task *models.WorkspaceTask,
    executor func(context.Context, *models.WorkspaceTask) error,
) error {
    config := DefaultRetryConfig
    
    for attempt := 0; attempt <= config.MaxRetries; attempt++ {
        // 执行任务
        err := executor(ctx, task)
        
        if err == nil {
            // 成功
            return nil
        }
        
        // 分类错误
        errorType := s.ClassifyError(err)
        
        // 不可重试的错误直接返回
        if errorType == ErrorTypeNonRetryable || errorType == ErrorTypeFatal {
            return err
        }
        
        // 达到最大重试次数
        if attempt >= config.MaxRetries {
            return fmt.Errorf("max retries exceeded: %w", err)
        }
        
        // 计算延迟时间（指数退避）
        delay := time.Duration(float64(config.InitialDelay) * 
            math.Pow(config.BackoffFactor, float64(attempt)))
        if delay > config.MaxDelay {
            delay = config.MaxDelay
        }
        
        log.Printf("Task %d failed (attempt %d/%d), retrying in %v: %v",
            task.ID, attempt+1, config.MaxRetries, delay, err)
        
        // 更新重试计数
        task.RetryCount = attempt + 1
        s.db.Save(task)
        
        // 等待后重试
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(delay):
            continue
        }
    }
    
    return fmt.Errorf("unexpected retry loop exit")
}
```

## 📈 监控和指标

### Prometheus指标

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    // 任务执行时间
    taskDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "iac_task_duration_seconds",
            Help:    "Task execution duration in seconds",
            Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
        },
        []string{"workspace_id", "task_type", "status"},
    )
    
    // 任务计数
    taskCounter = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "iac_task_total",
            Help: "Total number of tasks",
        },
        []string{"workspace_id", "task_type", "status"},
    )
    
    // 当前执行中的任务
    tasksInProgress = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "iac_tasks_in_progress",
            Help: "Number of tasks currently in progress",
        },
        []string{"task_type"},
    )
    
    // State版本数量
    stateVersions = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "iac_state_versions",
            Help: "Number of state versions per workspace",
        },
        []string{"workspace_id"},
    )
)

func init() {
    prometheus.MustRegister(taskDuration)
    prometheus.MustRegister(taskCounter)
    prometheus.MustRegister(tasksInProgress)
    prometheus.MustRegister(stateVersions)
}

// 记录任务指标
func (s *TerraformExecutor) RecordTaskMetrics(task *models.WorkspaceTask) {
    labels := prometheus.Labels{
        "workspace_id": fmt.Sprintf("%d", task.WorkspaceID),
        "task_type":    string(task.TaskType),
        "status":       string(task.Status),
    }
    
    // 记录执行时间
    if task.Duration > 0 {
        taskDuration.With(labels).Observe(float64(task.Duration))
    }
    
    // 增加计数
    taskCounter.With(labels).Inc()
}
```

## 🔐 安全考虑

### 1. 敏感变量处理

```go
// 过滤敏感变量
func (s *TerraformExecutor) FilterSensitiveVariables(
    variables []models.WorkspaceVariable,
) []models.WorkspaceVariable {
    filtered := make([]models.WorkspaceVariable, 0)
    
    for _, v := range variables {
        if v.Sensitive {
            // 不返回敏感变量的值
            v.Value = "***SENSITIVE***"
        }
        filtered = append(filtered, v)
    }
    
    return filtered
}
```

### 2. 工作目录权限

```go
func (s *TerraformExecutor) PrepareWorkspace(task *models.WorkspaceTask) (string, error) {
    workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%d/%d", 
        task.WorkspaceID, task.ID)
    
    // 创建目录，设置严格权限（仅所有者可访问）
    if err := os.MkdirAll(workDir, 0700); err != nil {
        return "", fmt.Errorf("failed to create work directory: %w", err)
    }
    
    return workDir, nil
}
```

### 3. 命令注入防护

```go
// 验证Terraform版本字符串
func (s *TerraformExecutor) ValidateTerraformVersion(version string) error {
    // 只允许版本号格式：x.y.z
    matched, err := regexp.MatchString(`^\d+\.\d+\.\d+$`, version)
    if err != nil || !matched {
        return fmt.Errorf("invalid terraform version format: %s", version)
    }
    return nil
}
```

## 📝 数据库Schema补充

### workspace_tasks表补充字段

```sql
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_data BYTEA;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_json JSONB;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS max_retries INTEGER DEFAULT 3;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_plan_lookup 
ON workspace_tasks(workspace_id, task_type, status, created_at DESC);
```

### task_logs表

```sql
CREATE TABLE IF NOT EXISTS task_logs (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    phase VARCHAR(20) NOT NULL, -- init, plan, apply
    content TEXT,
    level VARCHAR(10) NOT NULL, -- info, error, warning
    created_at TIMESTAMP DEFAULT NOW(),
    
    INDEX idx_task_logs_task_id (task_id),
    INDEX idx_task_logs_created_at (created_at)
);
```

## 🎯 最佳实践

### 1. 执行前检查清单

```go
func (s *TerraformExecutor) PreExecutionChecks(
    workspace *models.Workspace,
    task *models.WorkspaceTask,
) error {
    checks := []struct {
        name string
        fn   func() error
    }{
        {"Workspace not locked", func() error {
            if workspace.IsLocked {
                return fmt.Errorf("workspace is locked")
            }
            return nil
        }},
        {"Valid provider config", func() error {
            if workspace.ProviderConfig == nil {
                return fmt.Errorf("provider config is missing")
            }
            return nil
        }},
        {"Valid TF code", func() error {
            if workspace.TFCode == nil {
                return fmt.Errorf("terraform code is missing")
            }
            return nil
        }},
        {"Terraform binary exists", func() error {
            _, err := exec.LookPath("terraform")
            return err
        }},
        {"Sufficient disk space", func() error {
            // 检查磁盘空间
            return s.checkDiskSpace()
        }},
    }
    
    for _, check := range checks {
        if err := check.fn(); err != nil {
            return fmt.Errorf("%s failed: %w", check.name, err)
        }
    }
    
    return nil
}
```

### 2. 执行后验证

```go
func (s *TerraformExecutor) PostExecutionValidation(
    task *models.WorkspaceTask,
    workDir string,
) error {
    // 1. 验证State文件存在
    stateFile := filepath.Join(workDir, "terraform.tfstate")
    if _, err := os.Stat(stateFile); err != nil {
        return fmt.Errorf("state file not found: %w", err)
    }
    
    // 2. 验证State文件有效
    stateData, err := os.ReadFile(stateFile)
    if err != nil {
        return fmt.Errorf("failed to read state file: %w", err)
    }
    
    var state map[string]interface{}
    if err := json.Unmarshal(stateData, &state); err != nil {
        return fmt.Errorf("invalid state file: %w", err)
    }
    
    // 3. 验证State版本
    version, ok := state["version"].(float64)
    if !ok || version < 4 {
        return fmt.Errorf("unsupported state version: %v", version)
    }
    
    return nil
}
```

### 3. 超时控制

```go
func (s *TerraformExecutor) ExecuteWithTimeout(
    task *models.WorkspaceTask,
    timeout time.Duration,
) error {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    
    // 根据任务类型选择执行函数
    var executor func(context.Context, *models.WorkspaceTask) error
    switch task.TaskType {
    case models.TaskTypePlan:
        executor = s.ExecutePlan
    case models.TaskTypeApply:
        executor = s.ExecuteApply
    default:
        return fmt.Errorf("unknown task type: %s", task.TaskType)
    }
    
    // 执行任务
    errChan := make(chan error, 1)
    go func() {
        errChan <- executor(ctx, task)
    }()
    
    // 等待完成或超时
    select {
    case err := <-errChan:
        return err
    case <-ctx.Done():
        return fmt.Errorf("task execution timeout after %v", timeout)
    }
}
```

## 🔗 相关文档

- **上一篇**: [04-task-workflow.md](./04-task-workflow.md) - 任务工作流
- **相关**: [01-lifecycle.md](./01-lifecycle.md) - 生命周期状态机
- **相关**: [02-execution-modes.md](./02-execution-modes.md) - 执行模式
- **相关**: [03-state-management.md](./03-state-management.md) - State管理
- **扩展功能**: [47-structured-run-output-design.md](./47-structured-run-output-design.md) - Structured Run Output功能设计

## 📋 实施检查清单

### 开发阶段
- [ ] 实现TerraformExecutor服务
- [ ] 实现工作目录管理
- [ ] 实现配置文件生成
- [ ] 实现State文件管理
- [ ] 实现Plan任务执行
- [ ] 实现Apply任务执行
- [ ] 实现日志管理
- [ ] 实现错误处理和重试
- [ ] 实现资源清理

### 测试阶段
- [ ] 单元测试覆盖核心逻辑
- [ ] 集成测试验证完整流程
- [ ] 压力测试验证并发性能
- [ ] 错误场景测试
- [ ] State一致性测试

### 上线准备
- [ ] 监控指标配置
- [ ] 告警规则设置
- [ ] 日志收集配置
- [ ] 文档完善
- [ ] 运维手册编写

## 📖 TFE标准状态详细说明

### 完整状态转换流程（基于TFE官方文档）

#### 1. Pending Stage（等待阶段）

**状态**:
- `pending`: 任务尚未开始执行，在队列中等待

**离开条件**:
- 用户在开始前丢弃 → `discarded`
- 任务排到队首 → 自动进入`planning`

#### 2. Fetching Stage（获取阶段）

**状态**:
- `fetching`: 从数据库获取代码/从workspace的配置数据库获取变量/provider配置,从数据库获取state文件,初始化terraform二进制文件

**离开条件**:
- VCS获取失败 → `plan_errored`
- 成功获取配置 → 进入下一阶段

#### 3. Pre-Plan Stage（Plan前置阶段）

**状态**:
- `pre_plan_running`: 等待外部系统响应

**超时**: 10分钟

**离开条件**:
- 任何强制任务失败 → `plan_errored`
- 任何建议任务失败 → 继续到`planning`（带警告）
- 用户取消 → `canceled`

#### 4. Plan Stage（Plan阶段）

**状态**:
- `planning`: 正在执行terraform plan
- `needs_confirmation`: Plan完成，等待确认

**离开条件**:
- Plan命令失败 → `plan_errored`
- 用户取消 → `canceled`
- Plan成功但无变更且无成本估算/策略检查 → `planned_finished`
- Plan成功需要变更:
  - 启用成本估算 → 自动进入成本估算阶段
  - 禁用成本估算但启用策略 → 自动进入策略检查阶段
  - 无策略且可自动Apply → 自动进入Apply阶段
  - 无策略但不能自动Apply → 暂停在`needs_confirmation`

#### 5. Post-Plan Stage（Plan后置阶段）

**状态**:
- `post_plan_running`: 等待外部系统响应

**超时**: 10分钟

**离开条件**:
- 任何强制任务失败 → `plan_errored`
- 任何建议任务失败 → 继续到Apply（带警告）
- 用户取消 → `canceled`

#### 6. OPA Policy Check Stage（OPA策略检查阶段）

**状态**:
- `policy_check`: 检查Plan是否符合OPA策略
- `policy_override`: 强制策略失败，等待手动覆盖
- `policy_checked`: 策略检查通过

**离开条件**:
- 强制策略失败 → 暂停在`policy_override`
  - 用户丢弃 → `discarded`
  - 用户覆盖 → `policy_checked`
- 到达`policy_checked`状态:
  - 可自动Apply → 进入Apply阶段
  - 不能自动Apply → 暂停等待用户批准

#### 7. Cost Estimation Stage（成本估算阶段）

**状态**:
- `cost_estimating`: 正在估算成本
- `cost_estimated`: 成本估算完成

**离开条件**:
- 成本估算成功或失败 → 进入下一阶段
- 无策略检查或Apply → `planned_finished`

#### 8. Sentinel Policy Check Stage（Sentinel策略检查阶段）

**状态**:
- `policy_check`: 检查Plan是否符合Sentinel策略
- `policy_override`: 软强制策略失败，等待覆盖
- `policy_checked`: 策略检查通过

**离开条件**:
- 硬强制策略失败 → `plan_errored`
- 软强制策略失败 → 暂停在`policy_override`
  - 用户覆盖 → `policy_checked`
  - 用户丢弃 → `discarded`
- 到达`policy_checked`状态:
  - 可自动Apply → 进入Apply阶段
  - 不能自动Apply → 暂停等待批准

#### 9. Pre-Apply Stage（Apply前置阶段）

**状态**:
- `pre_apply_running`: 等待外部系统响应

**超时**: 10分钟

**离开条件**:
- 任何强制任务失败 → 跳到完成阶段
- 任何建议任务失败 → 继续到`applying`（带警告）
- 用户取消 → `canceled`

#### 10. Apply Stage（Apply阶段）

**状态**:
- `applying`: 正在执行terraform apply

**离开条件**:
- Apply成功 → `applied`
- Apply失败 → `apply_errored`
- 用户取消 → `canceled`

#### 11. Post-Apply Stage（Apply后置阶段）

**状态**:
- `post_apply_running`: 等待外部系统响应

**超时**: 10分钟

**特殊说明**: 此阶段只有建议任务，失败不会阻止运行

**离开条件**:
- 任何建议任务失败 → 继续到`applied`（带警告）
- 用户取消 → `canceled`

#### 12. Completion（完成阶段）

**最终状态**:
- `applied`: 成功应用
- `planned_finished`: Plan完成但无需Apply
- `apply_errored`: Apply失败
- `plan_errored`: Plan失败或硬强制策略失败
- `discarded`: 用户选择不继续
- `canceled`: 用户中断执行

### 状态转换图

```
pending
  ├─> discarded (用户丢弃)
  └─> fetching
        ├─> plan_errored (VCS失败)
        └─> pre_plan_running
              ├─> plan_errored (强制任务失败)
              ├─> canceled (用户取消)
              └─> planning
                    ├─> plan_errored (Plan失败)
                    ├─> canceled (用户取消)
                    ├─> planned_finished (无变更)
                    └─> needs_confirmation
                          └─> post_plan_running
                                ├─> plan_errored (强制任务失败)
                                ├─> canceled (用户取消)
                                └─> cost_estimating
                                      └─> cost_estimated
                                            └─> policy_check
                                                  ├─> plan_errored (硬强制失败)
                                                  ├─> policy_override (软强制失败)
                                                  │     ├─> discarded (用户丢弃)
                                                  │     └─> policy_checked (用户覆盖)
                                                  └─> policy_checked
                                                        └─> pre_apply_running
                                                              ├─> apply_errored (强制任务失败)
                                                              ├─> canceled (用户取消)
                                                              └─> applying
                                                                    ├─> apply_errored (Apply失败)
                                                                    ├─> canceled (用户取消)
                                                                    └─> post_apply_running
                                                                          ├─> canceled (用户取消)
                                                                          └─> applied (成功)
```

### 任务类型说明

#### Mandatory Tasks（强制任务）
- 失败会阻止运行继续
- 用于Pre-Plan、Post-Plan、Pre-Apply阶段
- 必须通过才能进入下一阶段

#### Advisory Tasks（建议任务）
- 失败不会阻止运行
- 会显示警告信息
- 用于Post-Apply阶段

### 自动Apply条件

Plan可以自动Apply需要满足以下条件：
1. Workspace启用了`auto-apply`设置
2. Plan由以下方式触发：
   - 新的VCS提交
   - 有Apply权限的用户手动触发

### 策略类型说明

#### OPA策略
- 在Plan后、成本估算前执行
- 支持强制策略（mandatory）

#### Sentinel策略
- 在成本估算后执行
- 支持硬强制（hard-mandatory）和软强制（soft-mandatory）
- 软强制失败可以被覆盖

## 🔧 关键修复和改进（完整实现）

### 修复1: terraform init添加-upgrade（P0）

**问题**: 不会更新Provider版本

**完整实现**:
```go
func (s *TerraformExecutor) TerraformInitV2(
    ctx context.Context,
    workDir string,
    task *models.WorkspaceTask,
    workspace *models.Workspace,
) error {
    // 1. 构建命令（必须包含-upgrade）
    args := []string{
        "init",
        "-no-color",
        "-input=false",
        "-upgrade", // 每次都升级Provider
    }
    
    cmd := exec.CommandContext(ctx, "terraform", args...)
    cmd.Dir = workDir
    
    // 2. 设置环境变量
    env := s.buildEnvironmentVariables(workspace)
    
    // 3. 配置Provider插件缓存
    pluginCacheDir := "/var/cache/terraform/plugins"
    os.MkdirAll(pluginCacheDir, 0755)
    env = append(env, fmt.Sprintf("TF_PLUGIN_CACHE_DIR=%s", pluginCacheDir))
    
    cmd.Env = env
    
    // 4. 执行
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
```

### 修复2: Provider认证环境变量（P0）

**问题**: 缺少Provider认证环境变量

**完整实现**:
```go
func (s *TerraformExecutor) buildEnvironmentVariables(
    workspace *models.Workspace,
) []string {
    env := append(os.Environ(),
        "TF_IN_AUTOMATION=true",
        "TF_INPUT=false",
    )
    
    // AWS Provider - 使用IAM Role
    if workspace.ProviderConfig != nil {
        if awsConfig, ok := workspace.ProviderConfig["aws"].([]interface{}); ok && len(awsConfig) > 0 {
            aws := awsConfig[0].(map[string]interface{})
            
            // 设置region（必需）
            if region, ok := aws["region"].(string); ok {
                env = append(env, fmt.Sprintf("AWS_DEFAULT_REGION=%s", region))
                env = append(env, fmt.Sprintf("AWS_REGION=%s", region))
            }
            
            // IAM Role会自动从以下位置获取凭证：
            // 1. EC2实例元数据服务
            // 2. ECS任务角色
            // 3. ~/.aws/credentials
            // 不需要额外设置AWS_ACCESS_KEY_ID和AWS_SECRET_ACCESS_KEY
        }
    }
    
    return env
}
```

### 修复3: State保存容错机制（P0）

**问题**: Apply成功但State保存失败会导致State丢失

**完整实现**:
```go
func (s *TerraformExecutor) SaveNewStateVersionWithRetry(
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
        fmt.Sprintf("ws_%d_task_%d_%d.tfstate", 
            workspace.ID, task.ID, time.Now().Unix()))
    
    if err := os.WriteFile(backupPath, stateData, 0600); err != nil {
        log.Printf("WARNING: Failed to backup state to file: %v", err)
    } else {
        log.Printf("State backed up to: %s", backupPath)
    }
    
    // 3. 尝试保存到数据库（带重试）
    maxRetries := 5
    var saveErr error
    
    for i := 0; i < maxRetries; i++ {
        saveErr = s.saveStateToDatabase(workspace, task, stateData)
        if saveErr == nil {
            log.Printf("State saved to database successfully")
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
    lockErr := s.workspaceService.LockWorkspace(
        workspace.ID,
        *task.CreatedBy,
        fmt.Sprintf("Auto-locked: State save failed for task %d. State backed up to %s", 
            task.ID, backupPath),
    )
    if lockErr != nil {
        log.Printf("ERROR: Failed to auto-lock workspace: %v", lockErr)
    }
    
    // 4.2 发送紧急告警
    s.notifySystem.NotifyEmergency("state_save_failed", workspace, task, map[string]interface{}{
        "backup_path": backupPath,
        "error":       saveErr.Error(),
        "retries":     maxRetries,
    })
    
    // 4.3 更新任务状态
    task.Status = models.TaskStatusPartialSuccess
    task.ErrorMessage = fmt.Sprintf(
        "Apply succeeded but state save failed. Workspace auto-locked. State backed up to: %s",
        backupPath)
    s.db.Save(task)
    
    return fmt.Errorf("state save failed, workspace locked, backup at: %s", backupPath)
}

func (s *TerraformExecutor) saveStateToDatabase(
    workspace *models.Workspace,
    task *models.WorkspaceTask,
    stateData []byte,
) error {
    var stateContent map[string]interface{}
    if err := json.Unmarshal(stateData, &stateContent); err != nil {
        return fmt.Errorf("failed to parse state: %w", err)
    }
    
    checksum := s.calculateChecksum(stateData)
    
    var maxVersion int
    s.db.Model(&models.WorkspaceStateVersion{}).
        Where("workspace_id = ?", workspace.ID).
        Select("COALESCE(MAX(version), 0)").
        Scan(&maxVersion)
    
    newVersion := maxVersion + 1
    
    return s.db.Transaction(func(tx *gorm.DB) error {
        stateVersion := &models.WorkspaceStateVersion{
            WorkspaceID: workspace.ID,
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
        
        return tx.Model(workspace).Updates(map[string]interface{}{
            "current_state_id": stateVersion.ID,
            "current_version":  newVersion,
        }).Error
    })
}
```

### 修复4: Plan-Apply明确关联（P0）

**问题**: Apply使用"最近的"Plan，可能不一致

**完整实现**:
```go
// 创建Plan任务（支持plan_only和plan_and_apply模式）
func (s *WorkspaceService) CreatePlanTask(workspaceID uint, userID uint) (*models.WorkspaceTask, error) {
    var workspace models.Workspace
    if err := s.db.First(&workspace, workspaceID).Error; err != nil {
        return nil, err
    }
    
    // 创建Plan任务
    planTask := &models.WorkspaceTask{
        WorkspaceID: workspaceID,
        TaskType:    models.TaskTypePlan,
        Status:      models.TaskStatusPending,
        CreatedBy:   &userID,
    }
    
    if err := s.db.Create(planTask).Error; err != nil {
        return nil, err
    }
    
    // 如果是plan_and_apply模式，预创建Apply任务
    if workspace.ExecutionMode == "plan_and_apply" {
        applyTask := &models.WorkspaceTask{
            WorkspaceID: workspaceID,
            TaskType:    models.TaskTypeApply,
            PlanTaskID:  &planTask.ID, // 明确关联
            Status:      models.TaskStatusWaiting,
            CreatedBy:   &userID,
        }
        s.db.Create(applyTask)
    }
    
    return planTask, nil
}

// 恢复Plan文件（使用关联的Plan任务）
func (s *TerraformExecutor) RestorePlanFileV2(
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
    
    return planFile, nil
}
```

### 修复5: Plan数据保存不阻塞（P0）

**问题**: Plan数据保存失败导致整个Plan失败

**完整实现**:
```go
func (s *TerraformExecutor) SavePlanDataV2(
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
    s.notifySystem.NotifyWarning("plan_data_save_failed", task, saveErr)
}
```

## 📊 代码版本管理（完整设计）

### 数据库设计

```sql
CREATE TABLE IF NOT EXISTS workspace_code_versions (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    tf_code JSONB NOT NULL,
    provider_config JSONB NOT NULL,
    state_version_id INTEGER REFERENCES workspace_state_versions(id),
    change_summary TEXT,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(workspace_id, version)
);

CREATE INDEX idx_workspace_code_versions_workspace 
ON workspace_code_versions(workspace_id, version DESC);

ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS current_code_version_id INTEGER;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS execution_mode VARCHAR(20) DEFAULT 'plan_and_apply';
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_task_id INTEGER;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS outputs JSONB;
```

### 代码版本创建

```go
func (s *WorkspaceService) UpdateWorkspaceCode(
    workspaceID uint,
    tfCode map[string]interface{},
    providerConfig map[string]interface{},
    userID uint,
    changeSummary string,
) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        var workspace models.Workspace
        if err := tx.First(&workspace, workspaceID).Error; err != nil {
            return err
        }
        
        var maxVersion int
        tx.Model(&models.WorkspaceCodeVersion{}).
            Where("workspace_id = ?", workspaceID).
            Select("COALESCE(MAX(version), 0)").
            Scan(&maxVersion)
        
        codeVersion := &models.WorkspaceCodeVersion{
            WorkspaceID:    workspaceID,
            Version:        maxVersion + 1,
            TFCode:         tfCode,
            ProviderConfig: providerConfig,
            ChangeSummary:  changeSummary,
            CreatedBy:      &userID,
        }
        
        if err := tx.Create(codeVersion).Error; err != nil {
            return err
        }
        
        workspace.TFCode = tfCode
        workspace.ProviderConfig = providerConfig
        workspace.CurrentCodeVersionID = &codeVersion.ID
        
        return tx.Save(&workspace).Error
    })
}
```

### 代码回滚

```go
func (s *WorkspaceService) RollbackToCodeVersion(
    workspaceID uint,
    versionID uint,
    userID uint,
) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        var codeVersion models.WorkspaceCodeVersion
        if err := tx.First(&codeVersion, versionID).Error; err != nil {
            return err
        }
        
        var workspace models.Workspace
        if err := tx.First(&workspace, workspaceID).Error; err != nil {
            return err
        }
        
        var maxVersion int
        tx.Model(&models.WorkspaceCodeVersion{}).
            Where("workspace_id = ?", workspaceID).
            Select("COALESCE(MAX(version), 0)").
            Scan(&maxVersion)
        
        newCodeVersion := &models.WorkspaceCodeVersion{
            WorkspaceID:    workspaceID,
            Version:        maxVersion + 1,
            TFCode:         codeVersion.TFCode,
            ProviderConfig: codeVersion.ProviderConfig,
            ChangeSummary:  fmt.Sprintf("Rollback to version %d", codeVersion.Version),
            CreatedBy:      &userID,
        }
        
        if err := tx.Create(newCodeVersion).Error; err != nil {
            return err
        }
        
        workspace.TFCode = codeVersion.TFCode
        workspace.ProviderConfig = codeVersion.ProviderConfig
        workspace.CurrentCodeVersionID = &newCodeVersion.ID
        
        return tx.Save(&workspace).Error
    })
}
```

### 版本管理关系图

```
Workspace
├── current_code_version_id → workspace_code_versions (代码版本)
└── current_state_id → workspace_state_versions (State版本)

workspace_code_versions (代码版本)
├── version: 1, 2, 3, ...
├── tf_code: {...}
├── provider_config: {...}
└── state_version_id → 关联的State版本（可选）

workspace_state_versions (State版本)
├── version: 1, 2, 3, ...
├── content: {...}
└── task_id → 创建此State的任务

关系说明：
1. 每次修改代码 → 创建新的代码版本
2. 每次Apply成功 → 创建新的State版本
3. 代码版本可以关联State版本（记录此代码对应的State）
4. 回滚代码 → 创建新的代码版本（内容是旧版本的）
5. 不能回滚State → State只能前进，不能后退
```

### 回滚流程设计

#### 代码回滚流程
```
1. 用户选择历史代码版本
   ↓
2. 系统创建新的代码版本（内容是历史版本的）
   ↓
3. 更新workspace.tf_code和provider_config
   ↓
4. 用户需要执行Plan查看变更
   ↓
5. 用户确认后执行Apply
   ↓
6. 创建新的State版本
```

#### 为什么不能回滚State？
1. **资源已存在**: State记录的是真实的云资源状态
2. **回滚State不会删除资源**: 只是让Terraform"忘记"这些资源
3. **会造成资源孤儿**: 资源存在但无法管理
4. **正确做法**: 回滚代码后重新Apply，Terraform会自动处理资源变更

### API接口设计

#### 代码版本管理API

```go
// 获取代码版本列表
GET /api/v1/workspaces/:id/code-versions
Response: {
    "versions": [
        {
            "id": 1,
            "version": 3,
            "change_summary": "Added new module",
            "state_version_id": 5,
            "created_by": 1,
            "created_at": "2025-10-11T10:00:00Z"
        }
    ]
}

// 获取指定代码版本详情
GET /api/v1/workspaces/:id/code-versions/:version
Response: {
    "id": 1,
    "version": 3,
    "tf_code": {...},
    "provider_config": {...},
    "state_version_id": 5,
    "change_summary": "Added new module",
    "created_by": 1,
    "created_at": "2025-10-11T10:00:00Z"
}

// 回滚到指定代码版本
POST /api/v1/workspaces/:id/code-versions/:version/rollback
Request: {
    "change_summary": "Rollback due to issue"
}
Response: {
    "new_version": 4,
    "message": "Rolled back to version 3, created new version 4"
}

// 比较两个代码版本
GET /api/v1/workspaces/:id/code-versions/compare?from=2&to=3
Response: {
    "tf_code_diff": {...},
    "provider_config_diff": {...}
}
```

### 版本管理说明

**关键原则**:
-  代码可以回滚（创建新版本）
- ❌ State不能回滚（只能前进）
-  代码版本关联State版本
-  回滚代码后需要重新Plan和Apply

**设计决策确认**:
1.  变量配置文件保持分离（4个文件）
2.  Workspace设置不需要版本管理
3.  Plan-Apply强制解耦，不需要过期时间
4.  代码和State都需要版本管理

---

**实施指南**: 所有修复代码已完整，所有设计决策已确认，可以直接开始开发。
