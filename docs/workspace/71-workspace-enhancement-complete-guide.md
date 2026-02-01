# Workspace增强功能完整开发文档

> **文档版本**: v1.0  
> **创建日期**: 2025-01-02  
> **适用范围**: 多人/多AI协作开发  
> **状态**: 设计完成，待实现

## 📋 目录

1. [需求概述](#需求概述)
2. [核心功能详解](#核心功能详解)
3. [数据库设计](#数据库设计)
4. [后端实现](#后端实现)
5. [前端实现](#前端实现)
6. [API接口规范](#api接口规范)
7. [业务流程](#业务流程)
8. [错误处理](#错误处理)
9. [测试用例](#测试用例)
10. [部署说明](#部署说明)

---

## 需求概述

### 项目背景

当前workspace功能较为单一，需要增强以支持更复杂的基础设施管理场景。本次增强主要包括：

- 多种执行模式支持（Local/Agent/K8s Pod）
- 灵活的Apply策略（自动/手动）
- 完善的文件存储和版本控制
- Workspace锁定机制
- 任务队列管理
- Provider配置管理

### 核心目标

1. **提升灵活性**: 支持多种执行环境和策略
2. **增强可靠性**: 文件存储重试机制和版本控制
3. **改善安全性**: Workspace锁定和权限控制
4. **优化体验**: 任务队列管理和状态可视化

---

## 核心功能详解

### 1. 执行模式 (Execution Mode)

#### 1.1 Local模式

**描述**: 在服务器本地直接执行Terraform命令

**特点**:
- 无需额外的Agent或K8s集群
- 执行速度快，适合开发测试环境
- 需要服务器安装Terraform

**使用场景**:
- 开发环境快速测试
- 小规模基础设施管理
- 无分布式需求的场景

**实现要点**:
```go
// 本地执行示例
func (s *WorkspaceService) ExecuteLocal(task *models.WorkspaceTask) error {
    // 1. 准备工作目录
    workdir := filepath.Join("/tmp/workspaces", fmt.Sprintf("%d", task.WorkspaceID))
    
    // 2. 写入tf文件
    if err := s.writeTerraformFiles(workdir, task.Workspace.TFCode); err != nil {
        return err
    }
    
    // 3. 执行terraform命令
    cmd := exec.Command("terraform", task.TaskType, "-no-color")
    cmd.Dir = workdir
    
    // 4. 捕获输出
    output, err := cmd.CombinedOutput()
    
    // 5. 保存结果
    return s.saveTaskOutput(task, string(output), err)
}
```

#### 1.2 Agent模式

**描述**: 将任务分发到已注册的Agent节点执行

**特点**:
- 分布式执行，负载均衡
- Agent可以部署在不同网络环境
- 支持Agent能力匹配（Terraform版本等）

**使用场景**:
- 多区域基础设施管理
- 需要特定网络环境的场景
- 大规模并发执行

**Agent选择逻辑**:
```go
// Agent选择示例
func (s *WorkspaceService) SelectAgent(workspace *models.Workspace) (*models.Agent, error) {
    // 1. 使用workspace配置的agent
    if workspace.AgentID != nil {
        agent, err := s.GetAgent(*workspace.AgentID)
        if err != nil {
            return nil, err
        }
        
        // 2. 检查agent状态
        if agent.Status != models.AgentStatusOnline {
            return nil, errors.New("agent is not online")
        }
        
        // 3. 检查agent能力
        if !s.checkAgentCapability(agent, workspace.TerraformVersion) {
            return nil, errors.New("agent does not support required terraform version")
        }
        
        return agent, nil
    }
    
    return nil, errors.New("no agent configured")
}
```

#### 1.3 K8s Pod模式

**描述**: 动态创建Kubernetes Pod执行任务

**特点**:
- 按需创建，任务完成后自动清理
- 资源隔离，互不影响
- 通过Secret挂载token实现自动注册

**使用场景**:
- 云原生环境
- 需要资源隔离的场景
- 弹性伸缩需求

**Pod创建流程**:
```go
// K8s Pod创建示例
func (s *WorkspaceService) CreateK8sPod(task *models.WorkspaceTask) error {
    // 1. 生成注册token
    token := s.generateAgentToken(task.WorkspaceID)
    
    // 2. 创建Secret
    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("agent-token-%d", task.ID),
            Namespace: "iac-platform",
        },
        StringData: map[string]string{
            "token":      token,
            "server_url": s.config.ServerURL,
        },
    }
    
    // 3. 创建Pod
    pod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("terraform-agent-%d", task.ID),
            Namespace: "iac-platform",
            Labels: map[string]string{
                "app":         "terraform-agent",
                "task-id":     fmt.Sprintf("%d", task.ID),
                "workspace-id": fmt.Sprintf("%d", task.WorkspaceID),
            },
        },
        Spec: corev1.PodSpec{
            Containers: []corev1.Container{
                {
                    Name:  "agent",
                    Image: "iac-platform/terraform-agent:latest",
                    Env: []corev1.EnvVar{
                        {
                            Name: "AGENT_TOKEN",
                            ValueFrom: &corev1.EnvVarSource{
                                SecretKeyRef: &corev1.SecretKeySelector{
                                    LocalObjectReference: corev1.LocalObjectReference{
                                        Name: secret.Name,
                                    },
                                    Key: "token",
                                },
                            },
                        },
                        {
                            Name: "SERVER_URL",
                            ValueFrom: &corev1.EnvVarSource{
                                SecretKeyRef: &corev1.SecretKeySelector{
                                    LocalObjectReference: corev1.LocalObjectReference{
                                        Name: secret.Name,
                                    },
                                    Key: "server_url",
                                },
                            },
                        },
                    },
                },
            },
            RestartPolicy: corev1.RestartPolicyNever,
        },
    }
    
    // 4. 提交到K8s
    _, err := s.k8sClient.CoreV1().Pods("iac-platform").Create(context.Background(), pod, metav1.CreateOptions{})
    return err
}
```

### 2. Apply方法 (Apply Method)

#### 2.1 自动Apply (Auto Apply)

**描述**: Plan成功后自动执行Apply

**特点**:
- 无需人工干预
- 适合CI/CD流程
- 需要充分的测试保障

**使用场景**:
- 自动化部署流程
- 开发/测试环境
- 经过充分测试的配置

**实现逻辑**:
```go
// 自动Apply示例
func (s *WorkspaceService) HandlePlanSuccess(task *models.WorkspaceTask) error {
    workspace := task.Workspace
    
    // 检查Apply方法
    if workspace.ApplyMethod == models.ApplyMethodAuto {
        // 自动创建Apply任务
        applyTask := &models.WorkspaceTask{
            WorkspaceID:   task.WorkspaceID,
            TaskType:      models.TaskTypeApply,
            Status:        models.TaskStatusPending,
            ExecutionMode: task.ExecutionMode,
            AgentID:       task.AgentID,
            CreatedBy:     task.CreatedBy,
        }
        
        return s.CreateTask(applyTask)
    }
    
    return nil
}
```

#### 2.2 手动Apply (Manual Apply)

**描述**: Plan成功后需要用户手动确认Apply

**特点**:
- 人工审核变更
- 降低误操作风险
- 适合生产环境

**使用场景**:
- 生产环境部署
- 重要基础设施变更
- 需要审批流程的场景

**前端交互**:
```typescript
// 手动Apply按钮示例
const handleManualApply = async (taskId: number) => {
    try {
        // 1. 确认对话框
        const confirmed = await showConfirmDialog({
            title: '确认Apply',
            message: '确定要执行Apply操作吗？此操作将修改基础设施。',
            type: 'warning'
        });
        
        if (!confirmed) return;
        
        // 2. 创建Apply任务
        const response = await api.post(`/workspaces/${workspaceId}/tasks/apply`, {
            plan_task_id: taskId
        });
        
        // 3. 跳转到任务详情
        navigate(`/workspaces/${workspaceId}/tasks/${response.data.id}`);
        
        showSuccess('Apply任务已创建');
    } catch (error) {
        showError(`创建Apply任务失败: ${error.message}`);
    }
};
```

### 3. Workspace锁定 (Workspace Locking)

#### 3.1 锁定机制

**描述**: 锁定Workspace以防止意外操作

**特点**:
- 锁定后任务进入pending队列
- 只有管理员可以解锁
- 可以查看历史记录

**锁定场景**:
- 维护期间
- 故障排查
- 重要变更前的冻结期

**实现逻辑**:
```go
// 锁定Workspace
func (s *WorkspaceService) LockWorkspace(workspaceID uint, userID uint, reason string) error {
    // 1. 检查用户权限
    if !s.isAdmin(userID) {
        return errors.New("only admin can lock workspace")
    }
    
    // 2. 更新workspace状态
    now := time.Now()
    return s.db.Model(&models.Workspace{}).
        Where("id = ?", workspaceID).
        Updates(map[string]interface{}{
            "is_locked":   true,
            "locked_by":   userID,
            "locked_at":   &now,
            "lock_reason": reason,
        }).Error
}

// 解锁Workspace
func (s *WorkspaceService) UnlockWorkspace(workspaceID uint, userID uint) error {
    // 1. 检查用户权限
    if !s.isAdmin(userID) {
        return errors.New("only admin can unlock workspace")
    }
    
    // 2. 更新workspace状态
    if err := s.db.Model(&models.Workspace{}).
        Where("id = ?", workspaceID).
        Updates(map[string]interface{}{
            "is_locked":   false,
            "locked_by":   nil,
            "locked_at":   nil,
            "lock_reason": "",
        }).Error; err != nil {
        return err
    }
    
    // 3. 触发pending任务执行
    return s.processPendingTasks(workspaceID)
}
```

#### 3.2 任务队列处理

**描述**: 锁定期间任务进入队列，解锁后自动执行

**队列处理逻辑**:
```go
// 处理pending任务
func (s *WorkspaceService) processPendingTasks(workspaceID uint) error {
    // 1. 获取所有pending任务
    var tasks []models.WorkspaceTask
    if err := s.db.Where("workspace_id = ? AND status = ?", 
        workspaceID, models.TaskStatusPending).
        Order("created_at ASC").
        Find(&tasks).Error; err != nil {
        return err
    }
    
    // 2. 逐个执行任务
    for _, task := range tasks {
        // 异步执行
        go s.ExecuteTask(&task)
    }
    
    return nil
}
```

### 4. 文件存储和版本控制

#### 4.1 TF代码存储

**描述**: 存储Terraform配置代码（JSON格式）

**存储时机**: 任务执行前

**数据结构**:
```json
{
    "data": {
        "aws_caller_identity": {
            "current": [{}]
        }
    },
    "locals": [
        {
            "bucket_name": "s3-bucket-${random_pet.this.id}",
            "region": "eu-west-1"
        }
    ],
    "module": {
        "s3_bucket": [
            {
                "bucket": "${local.bucket_name}",
                "source": "../../"
            }
        ]
    },
    "provider": {
        "aws": [
            {
                "region": "${local.region}",
                "alias": "primary"
            }
        ]
    },
    "resource": {
        "random_pet": {
            "this": [
                {
                    "length": 2
                }
            ]
        }
    }
}
```

#### 4.2 State文件存储

**描述**: 存储Terraform state文件（JSON格式）

**存储时机**: Apply任务成功后

**重试机制**:
```go
// State文件保存with重试
func (s *WorkspaceService) SaveStateWithRetry(workspaceID uint, state json.RawMessage, taskID uint) error {
    maxRetries := 3
    retryDelay := time.Second * 2
    
    for attempt := 1; attempt <= maxRetries; attempt++ {
        // 1. 计算checksum
        checksum := s.calculateChecksum(state)
        
        // 2. 获取当前最大版本号
        var maxVersion int
        s.db.Model(&models.WorkspaceStateVersion{}).
            Where("workspace_id = ?", workspaceID).
            Select("COALESCE(MAX(version), 0)").
            Scan(&maxVersion)
        
        // 3. 创建新版本
        stateVersion := &models.WorkspaceStateVersion{
            WorkspaceID: workspaceID,
            Content:     state,
            Version:     maxVersion + 1,
            Checksum:    checksum,
            TaskID:      &taskID,
        }
        
        // 4. 保存到数据库
        if err := s.db.Create(stateVersion).Error; err != nil {
            if attempt < maxRetries {
                log.Printf("Save state failed (attempt %d/%d): %v", attempt, maxRetries, err)
                time.Sleep(retryDelay)
                continue
            }
            return fmt.Errorf("save state failed after %d attempts: %w", maxRetries, err)
        }
        
        // 5. 更新workspace的当前state
        if err := s.db.Model(&models.Workspace{}).
            Where("id = ?", workspaceID).
            Update("tf_state", state).Error; err != nil {
            log.Printf("Update workspace state failed: %v", err)
            // 不影响版本保存成功
        }
        
        log.Printf("State saved successfully (version %d)", stateVersion.Version)
        return nil
    }
    
    return errors.New("unreachable")
}
```

#### 4.3 版本控制

**描述**: 保存state文件的所有历史版本

**功能**:
- 查看历史版本列表
- 下载指定版本
- 版本对比（可选）

**实现示例**:
```go
// 获取state版本列表
func (s *WorkspaceService) GetStateVersions(workspaceID uint, page, pageSize int) ([]models.WorkspaceStateVersion, int64, error) {
    var versions []models.WorkspaceStateVersion
    var total int64
    
    query := s.db.Model(&models.WorkspaceStateVersion{}).
        Where("workspace_id = ?", workspaceID)
    
    // 获取总数
    query.Count(&total)
    
    // 分页查询
    offset := (page - 1) * pageSize
    err := query.Order("version DESC").
        Offset(offset).
        Limit(pageSize).
        Preload("Task").
        Preload("CreatedByUser").
        Find(&versions).Error
    
    return versions, total, err
}

// 下载指定版本
func (s *WorkspaceService) DownloadStateVersion(workspaceID uint, version int) (json.RawMessage, error) {
    var stateVersion models.WorkspaceStateVersion
    
    err := s.db.Where("workspace_id = ? AND version = ?", workspaceID, version).
        First(&stateVersion).Error
    
    if err != nil {
        return nil, err
    }
    
    return stateVersion.Content, nil
}
```

### 5. Terraform版本管理

#### 5.1 版本设置

**描述**: 在workspace级别设置Terraform版本

**规则**: 只能升级，不能降级

**支持版本**:
- 1.0.0
- 1.1.0
- 1.2.0
- 1.3.0
- 1.4.0
- 1.5.0
- 1.6.0
- latest (最新稳定版)

**版本验证**:
```go
// 验证版本升级
func (s *WorkspaceService) ValidateTerraformVersionUpgrade(currentVersion, newVersion string) error {
    // 1. latest总是允许
    if newVersion == "latest" {
        return nil
    }
    
    // 2. 如果当前是latest，不允许降级到具体版本
    if currentVersion == "latest" {
        return errors.New("cannot downgrade from latest to specific version")
    }
    
    // 3. 解析版本号
    current, err := version.NewVersion(currentVersion)
    if err != nil {
        return fmt.Errorf("invalid current version: %w", err)
    }
    
    new, err := version.NewVersion(newVersion)
    if err != nil {
        return fmt.Errorf("invalid new version: %w", err)
    }
    
    // 4. 检查是否降级
    if new.LessThan(current) {
        return fmt.Errorf("cannot downgrade from %s to %s", currentVersion, newVersion)
    }
    
    return nil
}
```

### 6. Provider配置

#### 6.1 配置结构

**描述**: 支持多provider和多region配置

**示例配置**:
```json
{
    "aws": [
        {
            "region": "us-east-1",
            "alias": "primary"
        },
        {
            "region": "ap-northeast-1",
            "alias": "tokyo"
        },
        {
            "region": "eu-west-1",
            "alias": "ireland"
        }
    ],
    "azure": [
        {
            "location": "eastus",
            "alias": "primary"
        }
    ]
}
```

#### 6.2 配置管理

**特点**:
- 创建时设置
- 创建后不可修改
- 支持多provider
- 支持同一provider的多个配置（通过alias区分）

**验证逻辑**:
```go
// 验证Provider配置
func (s *WorkspaceService) ValidateProviderConfig(config models.ProviderConfig) error {
    // 1. 检查是否为空
    if len(config) == 0 {
        return errors.New("provider config cannot be empty")
    }
    
    // 2. 验证每个provider
    for providerName, providerConfigs := range config {
        // 2.1 检查provider名称
        if !s.isSupportedProvider(providerName) {
            return fmt.Errorf("unsupported provider: %s", providerName)
        }
        
        // 2.2 检查配置列表
        if len(providerConfigs) == 0 {
            return fmt.Errorf("provider %s has no configurations", providerName)
        }
        
        // 2.3 检查alias唯一性
        aliases := make(map[string]bool)
        for _, cfg := range providerConfigs {
            if alias, ok := cfg["alias"].(string); ok {
                if aliases[alias] {
                    return fmt.Errorf("duplicate alias %s in provider %s", alias, providerName)
                }
                aliases[alias] = true
            }
        }
    }
    
    return nil
}

// 支持的provider列表
func (s *WorkspaceService) isSupportedProvider(name string) bool {
    supported := []string{"aws", "azure", "gcp", "alicloud"}
    for _, p := range supported {
        if p == name {
            return true
        }
    }
    return false
}
```

### 7. 任务管理

#### 7.1 Plan任务

**描述**: 执行terraform plan，展示变更预览

**执行流程**:
1. 检查workspace锁定状态
2. 准备工作目录和文件
3. 执行terraform init
4. 执行terraform plan
5. 保存plan输出
6. 根据Apply方法决定是否自动创建Apply任务

**输出示例**:
```
Terraform will perform the following actions:

  # aws_s3_bucket.example will be created
  + resource "aws_s3_bucket" "example" {
      + bucket        = "my-bucket-name"
      + force_destroy = false
      + id            = (known after apply)
      ...
    }

Plan: 1 to add, 0 to change, 0 to destroy.
```

#### 7.2 Apply任务

**描述**: 执行terraform apply，实际创建/修改基础设施

**执行流程**:
1. 检查workspace锁定状态
2. 准备工作目录和文件
3. 执行terraform init
4. 重新执行terraform plan
5. 执行terraform apply -auto-approve
6. 保存apply输出
7. 保存state文件（带重试）

**状态流转**:
```
pending -> running -> success/failed
```

---

## 数据库设计

### 表结构设计

#### 1. workspaces表（增强版）

```sql
CREATE TABLE workspaces (
    -- 基础字段
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    
    -- 执行模式配置
    execution_mode VARCHAR(20) NOT NULL DEFAULT 'local', -- local, agent, k8s
    agent_id INTEGER REFERENCES agents(id), -- 选择的agent（agent模式）
    
    -- Apply方法
    apply_method VARCHAR(20) NOT NULL DEFAULT 'manual', -- auto, manual
    
    -- Terraform配置
    terraform_version VARCHAR(20) DEFAULT 'latest',
    workdir VARCHAR(500) DEFAULT '/workspace',
    
    -- 锁定状态
    is_locked BOOLEAN DEFAULT false,
    locked_by INTEGER REFERENCES users(id),
    locked_at TIMESTAMP,
    lock_reason TEXT,
    
    -- State后端配置
    state_backend VARCHAR(20) NOT NULL DEFAULT 'local', -- local, s3, remote
    state_config JSONB, -- 状态后端配置
    
    -- 文件存储（local模式）
    tf_code JSONB, -- Terraform代码（JSON格式）
    tf_state JSONB, -- Terraform state（JSON格式）
    
    -- Provider配置
    provider_config JSONB, -- Provider配置（AWS等）
    
    -- 初始化配置
    init_config JSONB, -- terraform backend配置
    
    -- 重试配置
    retry_enabled BOOLEAN DEFAULT true,
    max_retries INTEGER DEFAULT 3,
    
    -- 元数据
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(name, created_by)
);

-- 创建索引
CREATE INDEX idx_workspaces_execution_mode ON workspaces(execution_mode);
CREATE INDEX idx_workspaces_agent_id ON workspaces(agent_id);
CREATE INDEX idx_workspaces_is_locked ON workspaces(is_locked);
CREATE INDEX idx_workspaces_locked_by ON workspaces(locked_by);
CREATE INDEX idx_workspaces_created_by ON workspaces(created_by);
CREATE INDEX idx_workspaces_tf_code_gin ON workspaces USING GIN(tf_code);
CREATE INDEX idx_workspaces_tf_state_gin ON workspaces USING GIN(tf_state);
CREATE INDEX idx_workspaces_provider_config_gin ON workspaces USING GIN(provider_config);
CREATE INDEX idx_workspaces_init_config_gin ON workspaces USING GIN(init_config);
```

#### 2. workspace_tasks表（新增）

```sql
CREATE TABLE workspace_tasks (
    -- 基础字段
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 任务类型
    task_type VARCHAR(20) NOT NULL, -- plan, apply
    
    -- 任务状态
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, running, success, failed, cancelled
    
    -- 执行信息
    execution_mode VARCHAR(20) NOT NULL, -- local, agent, k8s
    agent_id INTEGER REFERENCES agents(id),
    k8s_pod_name VARCHAR(100),
    k8s_namespace VARCHAR(100) DEFAULT 'iac-platform',
    
    -- Terraform输出
    plan_output TEXT,
    apply_output TEXT,
    error_message TEXT,
    
    -- 执行时间
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration INTEGER, -- 执行时长（秒）
    
    -- 重试信息
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    
    -- 元数据
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 创建索引
CREATE INDEX idx_workspace_tasks_workspace_id ON workspace_tasks(workspace_id);
CREATE INDEX idx_workspace_tasks_task_type ON workspace_tasks(task_type);
CREATE INDEX idx_workspace_tasks_status ON workspace_tasks(status);
CREATE INDEX idx_workspace_tasks_execution_mode ON workspace_tasks(execution_mode);
CREATE INDEX idx_workspace_tasks_agent_id ON workspace_tasks(agent_id);
CREATE INDEX idx_workspace_tasks_created_by ON workspace_tasks(created_by);
CREATE INDEX idx_workspace_tasks_created_at ON workspace_tasks(created_at);
```

#### 3. workspace_state_versions表（新增）

```sql
CREATE TABLE workspace_state_versions (
    -- 基础字段
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 文件内容
    content JSONB NOT NULL, -- State文件内容
    
    -- 版本信息
    version INTEGER NOT NULL, -- 版本号，从1开始递增
    checksum VARCHAR(64) NOT NULL, -- SHA256校验和
    size_bytes INTEGER, -- 文件大小（字节）
    
    -- 关联任务
    task_id INTEGER REFERENCES workspace_tasks(id),
    
    -- 元数据
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(workspace_id, version)
);

-- 创建索引
CREATE INDEX idx_workspace_state_versions_workspace_id ON workspace_state_versions(workspace_id);
CREATE INDEX idx_workspace_state_versions_version ON workspace_state_versions(version);
CREATE INDEX idx_workspace_state_versions_task_id ON workspace_state_versions(task_id);
CREATE INDEX idx_workspace_state_versions_created_at ON workspace_state_versions(created_at);
```

### 数据迁移脚本

```sql
-- 迁移脚本：从旧表结构迁移到新表结构
-- 文件：scripts/migrate_workspace_enhancement.sql

BEGIN;

-- 1. 备份现有数据
CREATE TABLE workspaces_backup AS SELECT * FROM workspaces;

-- 2. 添加新字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS execution_mode VARCHAR(20) DEFAULT 'local';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS agent_id INTEGER REFERENCES agents(id);
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS apply_method VARCHAR(20) DEFAULT 'manual';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS workdir VARCHAR(500) DEFAULT '/workspace';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT false;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS locked_by INTEGER REFERENCES users(id);
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS locked_at TIMESTAMP;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS lock_reason TEXT;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tf_code JSONB;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tf_state JSONB;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS provider_config JSONB;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS init_config JSONB;
ALTER TABLE workspaces ADD COLUMN IF
