# Plan-Apply竞态条件修复方案

## 问题描述

当前系统的Plan+Apply流程存在严重的竞态条件bug：

### 问题场景
1. **Plan阶段**：从数据库读取资源、变量、state等数据，生成plan
2. **空档期**：Plan完成后等待用户确认Apply
3. **并发修改**：在空档期，其他用户/操作可能修改了：
   - 资源配置（workspace_resources）
   - 变量值（workspace_variables）
   - State状态（workspace_state_versions）
4. **Apply阶段**：强制从数据库重新获取最新数据，导致Apply与Plan不一致

### 风险
- Apply执行的配置与Plan预览的完全不同
- 可能导致意外的资源变更或破坏
- 违反了"Plan what you see, Apply what you planned"的原则

## 当前保护机制的不足

### 现有的ValidateResourceSnapshot
```go
// 只在ConfirmApply时检查，且只检查资源版本
if err := c.executor.ValidateResourceSnapshot(&task); err != nil {
    return conflict
}
```

**不足之处**：
1. ✗ 只检查资源版本，不检查变量和state
2. ✗ 检查时机太早（Apply入队时），真正执行时可能又被修改
3. ✗ 没有锁定机制防止并发修改
4. ✗ 不支持Agent模式

## 修复方案

### 方案1：资源版本快照（推荐）⭐

#### 核心思路
在Plan阶段记录资源的版本号快照，Apply时根据版本号获取对应的资源配置。这样既不会增加太多存储开销，也能确保Apply使用Plan时的配置。

**关键洞察**：
- ✓ State不会并发修改（workspace的apply任务是串行的）
- ✓ 只需要快照资源版本号和变量，不需要快照State
- ✓ Apply时根据版本号获取真实的资源配置
- ✓ 如果资源版本已变更，可以检测到并报错

#### 实现步骤

##### 1. 扩展Task模型存储版本快照

```sql
-- 添加版本快照字段到workspace_tasks表
ALTER TABLE workspace_tasks ADD COLUMN snapshot_resource_versions JSONB;
ALTER TABLE workspace_tasks ADD COLUMN snapshot_variables JSONB;
ALTER TABLE workspace_tasks ADD COLUMN snapshot_provider_config JSONB;
ALTER TABLE workspace_tasks ADD COLUMN snapshot_created_at TIMESTAMP;

-- snapshot_resource_versions 格式示例：
-- {
--   "resource_id_1": {"version_id": 123, "version": 5},
--   "resource_id_2": {"version_id": 456, "version": 3}
-- }
```

##### 2. Plan阶段：创建版本快照

```go
// 在ExecutePlan的SavePlanData阶段
func (s *TerraformExecutor) CreateResourceVersionSnapshot(task *models.WorkspaceTask, workspace *models.Workspace) error {
    // 1. 快照资源版本号（只存版本号，不存完整数据）
    resources, err := s.dataAccessor.GetWorkspaceResources(workspace.WorkspaceID)
    if err != nil {
        return err
    }
    
    resourceVersions := make(map[string]map[string]interface{})
    for _, r := range resources {
        if r.CurrentVersion != nil {
            resourceVersions[r.ResourceID] = map[string]interface{}{
                "version_id": r.CurrentVersion.ID,
                "version":    r.CurrentVersion.Version,
            }
        }
    }
    task.SnapshotResourceVersions = resourceVersions
    
    // 2. 快照变量（变量数据量小，直接存完整数据）
    variables, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
    if err != nil {
        return err
    }
    task.SnapshotVariables = variables
    
    // 3. 快照Provider配置
    task.SnapshotProviderConfig = workspace.ProviderConfig
    
    // 4. 记录快照时间
    task.SnapshotCreatedAt = timePtr(time.Now())
    
    log.Printf("Created resource version snapshot for task %d: %d resources, %d variables",
        task.ID, len(resourceVersions), len(variables))
    
    return s.dataAccessor.UpdateTask(task)
}
```

##### 3. Apply阶段：根据版本快照获取资源配置

```go
// 修改ExecuteApply，根据版本快照获取资源
func (s *TerraformExecutor) ExecuteApply(ctx context.Context, task *models.WorkspaceTask) error {
    // 获取Plan任务
    planTask, err := s.dataAccessor.GetPlanTask(*task.PlanTaskID)
    if err != nil {
        return err
    }
    
    // ✓ 验证快照是否存在
    if planTask.SnapshotCreatedAt == nil {
        return fmt.Errorf("plan task has no snapshot data")
    }
    
    // ✓ 根据版本快照获取资源配置
    resources, err := s.GetResourcesByVersionSnapshot(planTask.SnapshotResourceVersions)
    if err != nil {
        return fmt.Errorf("failed to get resources by snapshot: %w", err)
    }
    
    // ✓ 使用快照的变量和Provider配置
    workspace := &models.Workspace{
        WorkspaceID:    planTask.WorkspaceID,
        ProviderConfig: planTask.SnapshotProviderConfig,
    }
    
    // ✓ 生成配置文件时使用快照数据
    if err := s.GenerateConfigFilesFromSnapshot(workspace, resources, planTask.SnapshotVariables, workDir, logger); err != nil {
        return err
    }
    
    // State不需要快照（因为apply任务是串行的）
    if err := s.PrepareStateFileWithLogging(workspace, workDir, logger); err != nil {
        return err
    }
    
    // ... 继续执行Apply
}

// GetResourcesByVersionSnapshot 根据版本快照获取资源配置
func (s *TerraformExecutor) GetResourcesByVersionSnapshot(snapshotVersions map[string]map[string]interface{}) ([]models.WorkspaceResource, error) {
    var resources []models.WorkspaceResource
    
    for resourceID, versionInfo := range snapshotVersions {
        versionID := uint(versionInfo["version_id"].(float64))
        expectedVersion := int(versionInfo["version"].(float64))
        
        // 获取指定版本的资源
        resource, err := s.dataAccessor.GetResourceByVersionID(resourceID, versionID)
        if err != nil {
            return nil, fmt.Errorf("failed to get resource %s version %d: %w", resourceID, versionID, err)
        }
        
        // 验证版本号是否匹配
        if resource.CurrentVersion.Version != expectedVersion {
            return nil, fmt.Errorf("resource %s version mismatch: expected v%d, got v%d",
                resourceID, expectedVersion, resource.CurrentVersion.Version)
        }
        
        resources = append(resources, *resource)
    }
    
    return resources, nil
}
```

##### 4. 添加快照验证（双重保险）

```go
// 在Apply执行前验证快照完整性
func (s *TerraformExecutor) ValidateResourceVersionSnapshot(planTask *models.WorkspaceTask) error {
    if planTask.SnapshotCreatedAt == nil {
        return fmt.Errorf("no snapshot data")
    }
    
    if planTask.SnapshotResourceVersions == nil || len(planTask.SnapshotResourceVersions) == 0 {
        return fmt.Errorf("snapshot resource versions missing")
    }
    
    if planTask.SnapshotVariables == nil {
        return fmt.Errorf("snapshot variables missing")
    }
    
    if planTask.SnapshotProviderConfig == nil {
        return fmt.Errorf("snapshot provider config missing")
    }
    
    // 可选：检查快照是否过期（例如超过24小时）
    if time.Since(*planTask.SnapshotCreatedAt) > 24*time.Hour {
        return fmt.Errorf("snapshot expired (created %v ago)", time.Since(*planTask.SnapshotCreatedAt))
    }
    
    // 验证所有资源版本是否仍然存在
    for resourceID, versionInfo := range planTask.SnapshotResourceVersions {
        versionID := uint(versionInfo["version_id"].(float64))
        
        // 检查版本是否存在
        exists, err := s.dataAccessor.CheckResourceVersionExists(resourceID, versionID)
        if err != nil {
            return fmt.Errorf("failed to check resource %s version %d: %w", resourceID, versionID, err)
        }
        if !exists {
            return fmt.Errorf("resource %s version %d no longer exists", resourceID, versionID)
        }
    }
    
    return nil
}
```

#### 优点
- ✓ 完全消除竞态条件
- ✓ Apply严格使用Plan时的资源版本
- ✓ 支持Local和Agent模式
- ✓ 存储开销小（只存版本号，不存完整数据）
- ✓ 可以审计Plan时使用的资源版本
- ✓ 符合"Plan what you see, Apply what you planned"原则
- ✓ 利用现有的资源版本管理机制

#### 缺点
- 需要修改数据库schema（但改动较小）
- 需要添加根据版本号获取资源的查询接口
- 如果资源版本被删除，Apply会失败（但这是合理的保护机制）

---

### 方案2：完整数据快照（备选方案）

#### 核心思路
在Plan阶段将所有需要的数据完整快照保存到task表，Apply时直接使用快照数据。

#### 优点
- ✓ 不依赖资源版本管理
- ✓ 即使资源被删除也能Apply

#### 缺点
- ✗ 存储开销大（需要存储完整的资源配置）
- ✗ 数据冗余

**结论**：方案1（资源版本快照）更优，因为系统已有完善的资源版本管理机制。

---

### 方案3：乐观锁+版本号验证（不推荐）

#### 核心思路
为关键数据表添加版本号，Apply前验证所有版本号是否与Plan时一致。

#### 实现步骤

##### 1. 添加版本号字段

```sql
ALTER TABLE workspace_resources ADD COLUMN version_number INTEGER DEFAULT 1;
ALTER TABLE workspace_variables ADD COLUMN version_number INTEGER DEFAULT 1;
ALTER TABLE workspaces ADD COLUMN config_version INTEGER DEFAULT 1;

-- 创建触发器自动递增版本号
CREATE OR REPLACE FUNCTION increment_version()
RETURNS TRIGGER AS $$
BEGIN
    NEW.version_number = OLD.version_number + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER resource_version_trigger
BEFORE UPDATE ON workspace_resources
FOR EACH ROW EXECUTE FUNCTION increment_version();
```

##### 2. Plan阶段：记录版本号

```go
type SnapshotVersions struct {
    ResourceVersions map[string]int `json:"resource_versions"`
    VariableVersions map[string]int `json:"variable_versions"`
    ConfigVersion    int            `json:"config_version"`
    StateVersion     int            `json:"state_version"`
}

func (s *TerraformExecutor) RecordVersions(task *models.WorkspaceTask, workspace *models.Workspace) error {
    versions := &SnapshotVersions{
        ResourceVersions: make(map[string]int),
        VariableVersions: make(map[string]int),
        ConfigVersion:    workspace.ConfigVersion,
    }
    
    // 记录所有资源版本号
    resources, _ := s.dataAccessor.GetWorkspaceResources(workspace.WorkspaceID)
    for _, r := range resources {
        versions.ResourceVersions[r.ResourceID] = r.VersionNumber
    }
    
    // 记录所有变量版本号
    variables, _ := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
    for _, v := range variables {
        versions.VariableVersions[v.Key] = v.VersionNumber
    }
    
    // 记录State版本
    stateVersion, _ := s.dataAccessor.GetLatestStateVersion(workspace.WorkspaceID)
    if stateVersion != nil {
        versions.StateVersion = stateVersion.Version
    }
    
    task.SnapshotVersions = versions
    return s.dataAccessor.UpdateTask(task)
}
```

##### 3. Apply阶段：验证版本号

```go
func (s *TerraformExecutor) ValidateVersions(planTask *models.WorkspaceTask, workspace *models.Workspace) error {
    if planTask.SnapshotVersions == nil {
        return fmt.Errorf("no version snapshot")
    }
    
    versions := planTask.SnapshotVersions
    
    // 验证配置版本
    if workspace.ConfigVersion != versions.ConfigVersion {
        return fmt.Errorf("workspace config changed (expected v%d, got v%d)", 
            versions.ConfigVersion, workspace.ConfigVersion)
    }
    
    // 验证资源版本
    resources, _ := s.dataAccessor.GetWorkspaceResources(workspace.WorkspaceID)
    for _, r := range resources {
        expectedVersion, exists := versions.ResourceVersions[r.ResourceID]
        if !exists {
            return fmt.Errorf("new resource added: %s", r.ResourceID)
        }
        if r.VersionNumber != expectedVersion {
            return fmt.Errorf("resource %s changed (expected v%d, got v%d)",
                r.ResourceID, expectedVersion, r.VersionNumber)
        }
    }
    
    // 验证变量版本
    variables, _ := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
    for _, v := range variables {
        expectedVersion, exists := versions.VariableVersions[v.Key]
        if !exists {
            return fmt.Errorf("new variable added: %s", v.Key)
        }
        if v.VersionNumber != expectedVersion {
            return fmt.Errorf("variable %s changed (expected v%d, got v%d)",
                v.Key, expectedVersion, v.VersionNumber)
        }
    }
    
    // 验证State版本
    stateVersion, _ := s.dataAccessor.GetLatestStateVersion(workspace.WorkspaceID)
    if stateVersion != nil && stateVersion.Version != versions.StateVersion {
        return fmt.Errorf("state changed (expected v%d, got v%d)",
            versions.StateVersion, stateVersion.Version)
    }
    
    return nil
}
```

#### 优点
- ✓ 可以检测到任何数据变更
- ✓ 实现相对简单
- ✓ 存储开销小

#### 缺点
- ✗ 只能检测变更，不能防止变更
- ✗ 如果验证失败，用户需要重新Plan
- ✗ 需要修改多个表结构

---

### 方案4：Workspace锁定机制（补充方案）

#### 核心思路
Plan完成后自动锁定workspace，Apply完成后解锁。

#### 实现步骤

```go
// Plan完成后自动锁定
func (s *TerraformExecutor) ExecutePlan(...) error {
    // ... Plan执行 ...
    
    // Plan成功后，如果是plan_and_apply类型，自动锁定workspace
    if task.TaskType == models.TaskTypePlanAndApply {
        if err := s.lockWorkspaceForApply(workspace.WorkspaceID, task.ID); err != nil {
            logger.Warn("Failed to lock workspace: %v", err)
        } else {
            logger.Info("Workspace locked for apply")
        }
    }
    
    return nil
}

// Apply完成后解锁
func (s *TerraformExecutor) ExecuteApply(...) error {
    defer func() {
        // 无论成功失败都解锁
        s.unlockWorkspace(workspace.WorkspaceID)
    }()
    
    // ... Apply执行 ...
}

func (s *TerraformExecutor) lockWorkspaceForApply(workspaceID string, taskID uint) error {
    return s.dataAccessor.LockWorkspace(
        workspaceID,
        "system",
        fmt.Sprintf("Locked for apply (task #%d). Do not modify resources/variables until apply completes.", taskID),
    )
}
```

#### 优点
- ✓ 防止并发修改
- ✓ 实现简单
- ✓ 用户体验好（明确提示锁定原因）

#### 缺点
- ✗ 如果Apply失败或取消，需要手动解锁
- ✗ 不能完全防止直接数据库修改

---

## 推荐实施方案

### 最佳实践：方案1（资源版本快照）+ 方案4（锁定机制）

1. **使用方案1（资源版本快照）作为主要方案** ⭐
   - 完全消除竞态条件
   - Apply严格使用Plan时的资源版本
   - 存储开销小，实现优雅
   
2. **使用方案4（锁定机制）作为补充**
   - 防止用户在Plan-Apply期间误操作
   - 提供更好的用户体验

### 为什么选择资源版本快照？

1. **State不需要快照**
   - Workspace的apply任务是串行的
   - 不会出现并发修改State的情况
   
2. **资源版本管理已存在**
   - 系统已有完善的`workspace_resources`和`workspace_resource_versions`表
   - 只需要记录版本号，Apply时根据版本号获取配置
   
3. **存储开销小**
   - 只存储版本号（几KB），不存储完整资源配置（可能几MB）
   - 变量数据量小，可以直接存储
   
4. **实现优雅**
   - 利用现有的版本管理机制
   - 代码改动相对较小

### 实施优先级

#### Phase 1: 紧急修复（1-2天）
- [ ] 实现方案3：Plan完成后自动锁定workspace
- [ ] 在Apply前添加基本验证（检查snapshot_id）
- [ ] 添加告警日志记录数据变更

#### Phase 2: 完整修复（3-5天）
- [ ] 实现方案1：添加版本快照字段到数据库
- [ ] 在DataAccessor中添加GetResourceByVersionID接口
- [ ] 修改ExecutePlan保存资源版本快照
- [ ] 修改ExecuteApply根据版本快照获取资源
- [ ] 添加快照验证逻辑

#### Phase 3: 增强（可选）
- [ ] 添加快照过期检查（超过24小时警告）
- [ ] 添加快照清理机制（Apply完成后可选清理）
- [ ] 添加资源版本删除保护（被快照引用的版本不能删除）

---

## 测试场景

### 测试用例1：并发修改资源
1. 创建Plan任务
2. Plan完成后，修改某个资源配置
3. 确认Apply
4. **预期**：Apply失败或使用Plan时的配置

### 测试用例2：并发修改变量
1. 创建Plan任务
2. Plan完成后，修改某个变量值
3. 确认Apply
4. **预期**：Apply失败或使用Plan时的变量

### 测试用例3：并发State变更
1. 创建Plan任务
2. Plan完成后，另一个Apply修改了State
3. 确认Apply
4. **预期**：Apply失败或使用Plan时的State

### 测试用例4：Workspace锁定
1. 创建Plan任务
2. Plan完成后，尝试修改资源
3. **预期**：修改被拒绝（workspace已锁定）

---

## 回滚计划

如果新方案出现问题：

1. **数据库回滚**
   ```sql
   ALTER TABLE workspace_tasks DROP COLUMN snapshot_resource_versions;
   ALTER TABLE workspace_tasks DROP COLUMN snapshot_variables;
   ALTER TABLE workspace_tasks DROP COLUMN snapshot_provider_config;
   ALTER TABLE workspace_tasks DROP COLUMN snapshot_created_at;
   ```

2. **代码回滚**
   - 恢复ExecuteApply从数据库查询数据
   - 移除快照相关逻辑

3. **临时缓解措施**
   - 启用方案3（锁定机制）
   - 添加告警提示用户不要在Plan-Apply期间修改配置

---

## 总结

这是一个严重的竞态条件bug，可能导致Apply执行与Plan预览完全不同的配置。

### 核心洞察
- State不需要快照（apply任务是串行的）
- 只需要快照资源版本号和变量
- 利用现有的资源版本管理机制

### 推荐方案
使用**方案1（资源版本快照）+ 方案4（锁定机制）**的组合方案：
- 技术上：通过版本快照完全消除竞态条件
- 用户体验：通过锁定机制防止误操作
- 实现上：存储开销小，代码改动少

实施后，系统将真正实现"Plan what you see, Apply what you planned"的原则，确保Apply的安全性和可预测性。
