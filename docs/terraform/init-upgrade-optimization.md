# Terraform Init Upgrade 优化实施文档

## 1. 背景

### 1.1 问题描述
当前 IAC 平台在执行 `terraform init` 时默认启用了 `-upgrade` 参数，导致每次 init 都会重新下载 provider 插件，即使 provider 配置没有任何变化。这造成了：
- init 阶段耗时很长（可能占整个执行时间的 85-96%）
- 不必要的网络流量和资源消耗
- 用户体验差

### 1.2 目标
只在 workspace 的 provider 配置发生变更时才使用 `-upgrade` 参数，其他情况下跳过 upgrade 以加速 init 过程。

## 2. 技术方案

### 2.1 核心思路
通过计算 `provider_config` 的 hash 值来跟踪配置变更：
1. 在保存 `provider_config` 时计算并存储 hash
2. 在执行 init 时比较当前 hash 与上次成功 init 时的 hash
3. 如果 hash 相同，跳过 `-upgrade`；如果不同，使用 `-upgrade`

### 2.2 新增字段

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `provider_config_hash` | VARCHAR(64) | provider_config 的 SHA256 hash |
| `last_init_hash` | VARCHAR(64) | 上次成功 init 时的 provider_config hash |
| `terraform_version_hash` | VARCHAR(64) | 上次成功 init 时的 terraform 版本（可选，用于版本变更检测） |

### 2.3 判断逻辑

```
需要 upgrade = true  // 默认需要

if provider_config_hash 不为空 AND last_init_hash 不为空:
    if provider_config_hash == last_init_hash:
        需要 upgrade = false
    else:
        需要 upgrade = true  // provider 配置变更

if terraform_version 变更:
    需要 upgrade = true  // terraform 版本变更

if 首次运行（last_init_hash 为空）:
    需要 upgrade = true
```

## 3. 实施步骤

### 3.1 数据库迁移

**文件**: `scripts/add_provider_config_hash_fields.sql`

```sql
-- 添加 provider_config_hash 字段
ALTER TABLE workspaces 
ADD COLUMN IF NOT EXISTS provider_config_hash VARCHAR(64);

-- 添加 last_init_hash 字段
ALTER TABLE workspaces 
ADD COLUMN IF NOT EXISTS last_init_hash VARCHAR(64);

-- 添加 last_init_terraform_version 字段（可选）
ALTER TABLE workspaces 
ADD COLUMN IF NOT EXISTS last_init_terraform_version VARCHAR(20);

-- 添加注释
COMMENT ON COLUMN workspaces.provider_config_hash IS 'SHA256 hash of provider_config, updated when provider_config changes';
COMMENT ON COLUMN workspaces.last_init_hash IS 'provider_config_hash value when last successful terraform init with -upgrade';
COMMENT ON COLUMN workspaces.last_init_terraform_version IS 'terraform version when last successful terraform init';
```

### 3.2 更新 Workspace Model

**文件**: `backend/internal/models/workspace.go`

在 `Workspace` 结构体中添加：

```go
// Provider配置变更跟踪
ProviderConfigHash       string `json:"provider_config_hash" gorm:"type:varchar(64)"`
LastInitHash             string `json:"last_init_hash" gorm:"type:varchar(64)"`
LastInitTerraformVersion string `json:"last_init_terraform_version" gorm:"type:varchar(20)"`
```

### 3.3 更新 Workspace Controller

**文件**: `backend/controllers/workspace_controller.go`

在更新 `provider_config` 时计算 hash：

```go
// 在 UpdateWorkspace 或 PatchWorkspace 方法中
if providerConfig, ok := updates["provider_config"]; ok {
    // 计算 provider_config 的 hash
    hash := calculateProviderConfigHash(providerConfig)
    updates["provider_config_hash"] = hash
}

// calculateProviderConfigHash 计算 provider_config 的 SHA256 hash
func calculateProviderConfigHash(config interface{}) string {
    if config == nil {
        return ""
    }
    
    // 序列化为 JSON（排序 key 以确保一致性）
    jsonBytes, err := json.Marshal(config)
    if err != nil {
        return ""
    }
    
    // 计算 SHA256
    hash := sha256.Sum256(jsonBytes)
    return hex.EncodeToString(hash[:])
}
```

### 3.4 修改 TerraformInitWithLogging

**文件**: `backend/services/terraform_executor.go`

```go
// TerraformInitWithLogging 执行terraform init（带详细日志和流式输出）
func (s *TerraformExecutor) TerraformInitWithLogging(
    ctx context.Context,
    workDir string,
    task *models.WorkspaceTask,
    workspace *models.Workspace,
    logger *TerraformLogger,
) error {
    // ... 前面的代码保持不变 ...

    // 判断是否需要 upgrade
    needUpgrade := s.shouldUseUpgrade(workspace, logger)

    // 构建命令
    args := []string{
        "init",
        "-no-color",
        "-input=false",
        "-reconfigure",
    }

    if needUpgrade {
        args = append(args, "-upgrade")
        logger.Info("Using -upgrade flag (provider config or terraform version changed)")
    } else {
        logger.Info("Skipping -upgrade flag (provider config unchanged)")
    }

    // ... 执行 init 的代码 ...

    // init 成功后更新 last_init_hash
    if cmdErr == nil && needUpgrade {
        s.updateLastInitHash(workspace, logger)
    }

    return nil
}

// shouldUseUpgrade 判断是否需要使用 -upgrade 参数
func (s *TerraformExecutor) shouldUseUpgrade(workspace *models.Workspace, logger *TerraformLogger) bool {
    // 1. 首次运行（没有 last_init_hash）
    if workspace.LastInitHash == "" {
        logger.Debug("First init run, need upgrade")
        return true
    }

    // 2. provider_config 变更
    if workspace.ProviderConfigHash != workspace.LastInitHash {
        logger.Debug("Provider config changed: current=%s, last=%s", 
            workspace.ProviderConfigHash[:16]+"...", 
            workspace.LastInitHash[:16]+"...")
        return true
    }

    // 3. terraform 版本变更
    if workspace.LastInitTerraformVersion != "" && 
       workspace.LastInitTerraformVersion != workspace.TerraformVersion {
        logger.Debug("Terraform version changed: current=%s, last=%s",
            workspace.TerraformVersion,
            workspace.LastInitTerraformVersion)
        return true
    }

    // 4. 没有变更，不需要 upgrade
    logger.Debug("No changes detected, skipping upgrade")
    return false
}

// updateLastInitHash 更新 last_init_hash
func (s *TerraformExecutor) updateLastInitHash(workspace *models.Workspace, logger *TerraformLogger) {
    if workspace.ProviderConfigHash == "" {
        return
    }

    updates := map[string]interface{}{
        "last_init_hash":              workspace.ProviderConfigHash,
        "last_init_terraform_version": workspace.TerraformVersion,
    }

    if s.db != nil {
        // Local 模式
        if err := s.db.Model(&models.Workspace{}).
            Where("workspace_id = ?", workspace.WorkspaceID).
            Updates(updates).Error; err != nil {
            logger.Warn("Failed to update last_init_hash: %v", err)
        } else {
            logger.Debug("Updated last_init_hash to %s", workspace.ProviderConfigHash[:16]+"...")
        }
    } else {
        // Agent 模式：通过 API 更新
        if err := s.dataAccessor.UpdateWorkspaceFields(workspace.WorkspaceID, updates); err != nil {
            logger.Warn("Failed to update last_init_hash in Agent mode: %v", err)
        }
    }
}
```

### 3.5 同步修改 TerraformInit 函数

**文件**: `backend/services/terraform_executor.go`

对 `TerraformInit` 函数进行相同的修改（如果该函数仍在使用）。

### 3.6 更新 DataAccessor 接口

**文件**: `backend/services/data_accessor.go`

添加新方法：

```go
// DataAccessor 接口添加
UpdateWorkspaceFields(workspaceID string, updates map[string]interface{}) error
```

## 4. 测试计划

### 4.1 单元测试

1. 测试 `calculateProviderConfigHash` 函数
   - 相同配置应产生相同 hash
   - 不同配置应产生不同 hash
   - nil 配置应返回空字符串

2. 测试 `shouldUseUpgrade` 函数
   - 首次运行返回 true
   - provider_config 变更返回 true
   - terraform 版本变更返回 true
   - 无变更返回 false

### 4.2 集成测试

1. **场景1**: 首次创建 workspace 并执行 plan
   - 预期：使用 `-upgrade`

2. **场景2**: 不修改 provider 配置，再次执行 plan
   - 预期：不使用 `-upgrade`

3. **场景3**: 修改 provider 配置（如 region），执行 plan
   - 预期：使用 `-upgrade`

4. **场景4**: 修改 terraform 版本，执行 plan
   - 预期：使用 `-upgrade`

### 4.3 性能测试

对比优化前后的 init 时间：
- 优化前：每次都下载 provider（预计 30-60 秒）
- 优化后（无变更）：跳过下载（预计 2-5 秒）

## 5. 回滚方案

如果出现问题，可以通过以下方式回滚：

1. 在 `shouldUseUpgrade` 函数中直接返回 `true`
2. 或者删除新增的判断逻辑，恢复原来的硬编码 `-upgrade`

## 6. 文件变更清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `scripts/add_provider_config_hash_fields.sql` | 新增 | 数据库迁移脚本 |
| `backend/internal/models/workspace.go` | 修改 | 添加新字段 |
| `backend/controllers/workspace_controller.go` | 修改 | 保存时计算 hash |
| `backend/services/terraform_executor.go` | 修改 | 判断是否使用 upgrade |
| `backend/services/data_accessor.go` | 修改 | 添加新方法（如需要） |

## 7. 时间估算

| 任务 | 预估时间 |
|------|----------|
| 数据库迁移脚本 | 0.5h |
| 更新 Workspace Model | 0.5h |
| 更新 Controller（计算 hash） | 1h |
| 修改 TerraformInitWithLogging | 1.5h |
| 修改 TerraformInit（如需要） | 0.5h |
| 测试验证 | 1h |
| **总计** | **5h** |

## 8. 注意事项

1. **Agent 模式兼容性**: 确保 Agent 模式下也能正确获取和更新 hash
2. **并发安全**: 多个任务同时执行时的 hash 更新需要考虑并发
3. **向后兼容**: 旧数据（hash 为空）应该默认使用 `-upgrade`
4. **日志记录**: 清晰记录是否使用了 `-upgrade` 以及原因

## 9. 实现状态

### 已完成
- [x] 数据库字段添加 (`last_init_hash`, `last_init_terraform_version`)
- [x] Workspace 模型更新
- [x] `computeInitHash` 函数实现
- [x] `shouldRunInitUpgrade` 函数实现
- [x] `terraform init` 执行逻辑优化
- [x] 首次运行检测（无 `.terraform` 目录时不使用 `-upgrade`）
- [x] Local 模式支持
- [x] Agent 模式支持（DataAccessor 接口 + API 端点）
- [x] K8s Agent 模式支持（通过 RemoteDataAccessor）
- [x] 文档编写

### 待完成
- [ ] 单元测试
- [ ] 集成测试

## 10. 三种执行模式支持

### 10.1 Local 模式
- 直接通过 `LocalDataAccessor.UpdateWorkspaceFields()` 更新数据库
- 使用 GORM 直接操作 `workspaces` 表

### 10.2 Agent 模式
- 通过 `RemoteDataAccessor.UpdateWorkspaceFields()` 调用 API
- API 端点: `PATCH /api/v1/agents/workspaces/{workspace_id}/fields`
- 使用 `AgentAPIClient.UpdateWorkspaceFields()` 发送请求

### 10.3 K8s Agent 模式
- 与 Agent 模式相同，使用 `RemoteDataAccessor`
- 通过 Pool Token 认证

### 10.4 安全措施
API 端点使用白名单机制，只允许更新以下字段：
- `last_init_hash`
- `last_init_terraform_version`
