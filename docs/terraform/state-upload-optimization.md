# Terraform State 上传优化方案

## 1. 概述

本方案旨在优化 Terraform State 上传机制，增加安全校验、强制上传选项、回滚机制和导入标记功能。

## 2. 当前问题

1. **缺少校验机制**：上传 state 时没有 lineage 和 serial 校验
2. **无强制上传选项**：无法在特殊情况下强制覆盖 state
3. **缺少回滚机制**：state 上传错误后无法回滚到之前版本
4. **无导入标记**：无法区分用户手动导入的 state 和系统生成的 state

## 3. 解决方案

### 3.1 State 校验机制

#### 3.1.1 数据模型扩展

在 `workspace_state_versions` 表中添加字段：

```sql
ALTER TABLE workspace_state_versions ADD COLUMN lineage VARCHAR(255);
ALTER TABLE workspace_state_versions ADD COLUMN serial INTEGER;
ALTER TABLE workspace_state_versions ADD COLUMN is_imported BOOLEAN DEFAULT FALSE;
ALTER TABLE workspace_state_versions ADD COLUMN import_source VARCHAR(50); -- 'user_upload', 'api', 'terraform_apply'
ALTER TABLE workspace_state_versions ADD COLUMN is_rollback BOOLEAN DEFAULT FALSE;
ALTER TABLE workspace_state_versions ADD COLUMN rollback_from_version INTEGER;
```

**注意：** `created_by` 字段已存在于表中，用于记录操作用户：
- **上传 State**：`created_by` = 上传用户的 user_id
- **回滚 State**：`created_by` = 执行回滚的用户 user_id
- **Terraform Apply**：`created_by` = 执行 Apply 的用户 user_id

所有操作都会自动记录操作用户，确保完整的审计追踪。

#### 3.1.2 校验逻辑

**正常上传（不带 force）：**
```go
func (s *StateService) ValidateStateUpload(newState map[string]interface{}, workspaceID string) error {
    // 1. 获取当前最新 state
    currentState, err := s.GetLatestState(workspaceID)
    if err != nil {
        return err
    }
    
    // 2. 提取 lineage 和 serial
    newLineage := newState["lineage"].(string)
    newSerial := int(newState["serial"].(float64))
    
    if currentState != nil {
        currentLineage := currentState.Lineage
        currentSerial := currentState.Serial
        
        // 3. Lineage 校验
        if newLineage != currentLineage {
            return fmt.Errorf("lineage mismatch: expected %s, got %s", currentLineage, newLineage)
        }
        
        // 4. Serial 校验（必须递增）
        if newSerial <= currentSerial {
            return fmt.Errorf("serial must be greater than current (%d), got %d", currentSerial, newSerial)
        }
    }
    
    return nil
}
```

**强制上传（带 force）：**
```go
func (s *StateService) ForceUploadState(newState map[string]interface{}, workspaceID string, userID string) error {
    // 跳过所有校验，直接保存
    // 但需要记录审计日志
    s.auditLog.Log(AuditLog{
        Action:      "state_force_upload",
        WorkspaceID: workspaceID,
        UserID:      userID,
        Details:     "Force uploaded state, bypassing validation",
        Severity:    "WARNING",
    })
    
    return s.SaveState(newState, workspaceID, true) // force=true
}
```

### 3.2 前端 UI 设计

#### 3.2.1 上传界面

```tsx
// WorkspaceSettings.tsx - State Upload Section
<div className="state-upload-section">
  <h3>Upload Terraform State</h3>
  
  <FileUpload
    accept=".tfstate,.json"
    onChange={handleStateFileSelect}
  />
  
  <div className="upload-options">
    <Checkbox
      checked={forceUpload}
      onChange={(e) => {
        if (e.target.checked) {
          // 立即显示警告（不是弹窗，而是内联警告）
          setShowForceWarning(true);
        } else {
          setShowForceWarning(false);
        }
        setForceUpload(e.target.checked);
      }}
    >
      Force Upload (跳过校验)
    </Checkbox>
  </div>
  
  {showForceWarning && (
    <Alert type="error" className="force-warning">
      <AlertTitle> 危险操作警告</AlertTitle>
      <AlertDescription>
        <p><strong>强制上传将跳过所有安全校验，可能导致：</strong></p>
        <ul>
          <li>覆盖其他用户的更改</li>
          <li>State 不一致</li>
          <li>资源管理混乱</li>
        </ul>
        <p><strong>请确认：</strong></p>
        <ul>
          <li>✓ 已备份当前 state</li>
          <li>✓ 了解此操作的风险</li>
          <li>✓ 已与团队成员确认</li>
        </ul>
      </AlertDescription>
    </Alert>
  )}
  
  <Button
    onClick={handleUpload}
    disabled={!stateFile}
    danger={forceUpload}
  >
    {forceUpload ? '强制上传 State' : '上传 State'}
  </Button>
</div>
```

#### 3.2.2 警告样式

```css
.force-warning {
  margin: 16px 0;
  padding: 16px;
  background: #fff2e8;
  border: 2px solid #ff4d4f;
  border-radius: 4px;
}

.force-warning ul {
  margin: 8px 0;
  padding-left: 24px;
}

.force-warning li {
  margin: 4px 0;
  color: #d4380d;
}
```

### 3.3 State 回滚机制

#### 3.3.1 回滚 API

```go
// POST /api/workspaces/:workspace_id/state/rollback
func (h *StateHandler) RollbackState(c *gin.Context) {
    workspaceID := c.Param("workspace_id")
    
    var req struct {
        TargetVersion int    `json:"target_version" binding:"required"`
        Reason        string `json:"reason"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 1. 获取目标版本
    targetState, err := h.stateService.GetStateVersion(workspaceID, req.TargetVersion)
    if err != nil {
        c.JSON(404, gin.H{"error": "target version not found"})
        return
    }
    
    // 2. 创建新版本（标记为 rollback）
    newVersion := &models.WorkspaceStateVersion{
        WorkspaceID:         workspaceID,
        Content:             targetState.Content,
        Lineage:             targetState.Lineage,
        Serial:              targetState.Serial + 1, // 递增 serial
        IsRollback:          true,
        RollbackFromVersion: &req.TargetVersion,
        CreatedBy:           c.GetString("user_id"),
    }
    
    if err := h.stateService.SaveStateVersion(newVersion); err != nil {
        c.JSON(500, gin.H{"error": "failed to rollback"})
        return
    }
    
    // 3. 记录审计日志
    h.auditLog.Log(AuditLog{
        Action:      "state_rollback",
        WorkspaceID: workspaceID,
        UserID:      c.GetString("user_id"),
        Details:     fmt.Sprintf("Rolled back to version %d. Reason: %s", req.TargetVersion, req.Reason),
    })
    
    c.JSON(200, gin.H{"message": "rollback successful", "new_version": newVersion.Version})
}
```

#### 3.3.2 回滚 UI

```tsx
// StateVersionHistory.tsx
<div className="state-version-history">
  <h3>State Version History</h3>
  
  <Table>
    <thead>
      <tr>
        <th>Version</th>
        <th>Created At</th>
        <th>Created By</th>
        <th>Source</th>
        <th>Description</th>
        <th>Actions</th>
      </tr>
    </thead>
    <tbody>
      {versions.map(version => (
        <tr key={version.version}>
          <td>
            #{version.version}
            {version.is_imported && <Tag color="blue">Imported</Tag>}
            {version.is_rollback && (
              <Tooltip title={`从版本 #${version.rollback_from_version} 回滚`}>
                <Tag color="orange">Rollback from #{version.rollback_from_version}</Tag>
              </Tooltip>
            )}
          </td>
          <td>{formatDate(version.created_at)}</td>
          <td>
            <UserAvatar userId={version.created_by} />
            <span style={{ marginLeft: 8 }}>
              {getUserDisplayName(version.created_by)}
            </span>
          </td>
          <td>
            {version.is_rollback ? (
              <span>
                Rollback
                <Tooltip title="点击查看源版本">
                  <a 
                    onClick={() => scrollToVersion(version.rollback_from_version)}
                    style={{ marginLeft: 8 }}
                  >
                    (from #{version.rollback_from_version})
                  </a>
                </Tooltip>
              </span>
            ) : (
              getSourceLabel(version.import_source)
            )}
          </td>
          <td>
            {version.is_rollback && (
              <span className="rollback-description">
                从版本 #{version.rollback_from_version} 回滚
                {version.description && `: ${version.description}`}
              </span>
            )}
            {version.is_imported && version.description && (
              <span className="import-description">{version.description}</span>
            )}
          </td>
          <td>
            <Button
              size="small"
              onClick={() => handleDownload(version)}
            >
              Download
            </Button>
            <Button
              size="small"
              onClick={() => handleRollback(version)}
              disabled={version.version === currentVersion}
            >
              Rollback
            </Button>
            {version.is_rollback && (
              <Button
                size="small"
                onClick={() => handleViewDiff(version.rollback_from_version, version.version)}
              >
                View Changes
              </Button>
            )}
          </td>
        </tr>
      ))}
    </tbody>
  </Table>
</div>

// 回滚确认对话框
<Modal
  title="确认回滚 State"
  visible={rollbackModalVisible}
  onOk={handleConfirmRollback}
  onCancel={() => setRollbackModalVisible(false)}
>
  <Alert type="warning" message="回滚操作将创建新的 State 版本" style={{ marginBottom: 16 }} />
  
  <div className="rollback-info">
    <p><strong>当前版本：</strong>#{currentVersion}</p>
    <p><strong>回滚到版本：</strong>#{targetRollbackVersion}</p>
    <p><strong>新版本号：</strong>#{currentVersion + 1} (标记为从 #{targetRollbackVersion} 回滚)</p>
  </div>
  
  <Form.Item label="回滚原因" required>
    <TextArea
      rows={4}
      placeholder="请说明回滚原因，例如：修复错误的 state 上传、恢复到稳定版本等"
      value={rollbackReason}
      onChange={(e) => setRollbackReason(e.target.value)}
    />
  </Form.Item>
  
  <Alert 
    type="info" 
    message="提示" 
    description="回滚后将自动锁定 workspace，需要手动解锁后才能继续操作。"
  />
</Modal>
```

**样式补充：**

```css
.rollback-description {
  color: #fa8c16;
  font-style: italic;
}

.import-description {
  color: #1890ff;
  font-style: italic;
}

.rollback-info {
  background: #f5f5f5;
  padding: 16px;
  border-radius: 4px;
  margin-bottom: 16px;
}

.rollback-info p {
  margin: 8px 0;
}
```

### 3.4 导入标记

#### 3.4.1 上传时标记

```go
func (s *StateService) UploadState(
    stateContent map[string]interface{},
    workspaceID string,
    userID string,
    force bool,
) error {
    // 1. 先锁定 workspace（防止并发修改）
    lockReason := "State upload in progress"
    if err := s.lockWorkspace(workspaceID, userID, lockReason); err != nil {
        return fmt.Errorf("failed to lock workspace: %w", err)
    }
    
    // 确保函数退出时释放锁（使用 defer）
    defer func() {
        if unlockErr := s.unlockWorkspace(workspaceID); unlockErr != nil {
            log.Printf("Warning: failed to unlock workspace %s: %v", workspaceID, unlockErr)
        }
    }()
    
    // 2. 校验（除非 force=true）
    if !force {
        if err := s.ValidateStateUpload(stateContent, workspaceID); err != nil {
            // 校验失败，锁会被 defer 自动释放
            return err
        }
    }
    
    // 3. 提取 lineage 和 serial
    lineage := stateContent["lineage"].(string)
    serial := int(stateContent["serial"].(float64))
    
    // 4. 创建新版本
    newVersion := &models.WorkspaceStateVersion{
        WorkspaceID:  workspaceID,
        Content:      stateContent,
        Lineage:      lineage,
        Serial:       serial,
        IsImported:   true,                    // 标记为导入
        ImportSource: "user_upload",           // 来源：用户上传
        CreatedBy:    userID,
    }
    
    // 5. 保存 state 版本
    if err := s.SaveStateVersion(newVersion); err != nil {
        // 保存失败，锁会被 defer 自动释放
        return err
    }
    
    // 6. 记录审计日志
    s.auditLog.Log(AuditLog{
        Action:      "state_upload",
        WorkspaceID: workspaceID,
        UserID:      userID,
        Details:     fmt.Sprintf("Uploaded state version %d (force=%v)", newVersion.Version, force),
        Severity:    "INFO",
    })
    
    // 7. 成功完成，锁会被 defer 自动释放
    return nil
}

// lockWorkspace 锁定 workspace
func (s *StateService) lockWorkspace(workspaceID, userID, reason string) error {
    lock := &models.ResourceLock{
        ResourceType: "workspace",
        ResourceID:   workspaceID,
        LockedBy:     userID,
        Reason:       reason,
        LockedAt:     time.Now(),
    }
    
    return s.db.Create(lock).Error
}

// unlockWorkspace 解锁 workspace
func (s *StateService) unlockWorkspace(workspaceID string) error {
    return s.db.Where("resource_type = ? AND resource_id = ?", "workspace", workspaceID).
        Delete(&models.ResourceLock{}).Error
}
```

#### 3.4.2 显示导入标记

在 State 版本列表中显示：
- 蓝色 "Imported" 标签：用户手动上传的 state
- 绿色 "Apply" 标签：terraform apply 生成的 state
- 橙色 "Rollback from #xxx" 标签：回滚操作生成的 state，明确显示从哪个版本回滚

**标记显示规则：**
1. **回滚版本**：
   - 标签文本：`Rollback from #120`（明确显示源版本号）
   - 鼠标悬停提示：`从版本 #120 回滚`
   - Description 列：`从版本 #120 回滚: [用户填写的原因]`
   - Source 列：显示可点击链接，点击后滚动到源版本

2. **导入版本**：
   - 标签文本：`Imported`
   - Description 列：显示用户上传时填写的说明

3. **Apply 版本**：
   - 无特殊标签
   - Source 列：显示 `Terraform Apply`

## 4. API 设计

### 4.1 上传 State

```
POST /api/workspaces/:workspace_id/state/upload

Request:
{
  "state": { ... },           // state 文件内容
  "force": false,             // 是否强制上传
  "description": "string"     // 可选：上传说明
}

Response (成功):
{
  "message": "state uploaded successfully",
  "version": 123,
  "warnings": []              // 如果使用 force，返回警告信息
}

Response (校验失败):
{
  "error": "lineage mismatch",
  "details": {
    "expected_lineage": "abc123",
    "got_lineage": "def456"
  },
  "suggestion": "Use force=true to bypass validation"
}
```

### 4.2 回滚 State

```
POST /api/workspaces/:workspace_id/state/rollback

Request:
{
  "target_version": 120,
  "reason": "Rollback due to incorrect state"
}

Response:
{
  "message": "rollback successful",
  "new_version": 124,
  "rollback_from_version": 120,
  "description": "从版本 #120 回滚: Rollback due to incorrect state"
}
```

### 4.3 获取 State 历史

```
GET /api/workspaces/:workspace_id/state/versions?limit=50&offset=0

Response:
{
  "versions": [
    {
      "version": 124,
      "created_at": "2026-01-08T17:30:00Z",
      "created_by": "user-123",  // 执行回滚的用户ID
      "is_imported": false,
      "import_source": "terraform_apply",
      "is_rollback": true,
      "rollback_from_version": 120,
      "description": "从版本 #120 回滚: Rollback due to incorrect state",
      "size_bytes": 12345,
      "checksum": "abc123..."
    },
    {
      "version": 123,
      "created_at": "2026-01-08T17:00:00Z",
      "created_by": "user-456",  // 上传 State 的用户ID
      "is_imported": true,
      "import_source": "user_upload",
      "is_rollback": false,
      "rollback_from_version": null,
      "description": "Manual upload for testing",
      "size_bytes": 12345,
      "checksum": "abc123..."
    }
  ],
  "total": 124,
  "current_version": 124
}
```

## 5. 数据库迁移脚本

```sql
-- scripts/add_state_upload_optimization_fields.sql

-- 添加新字段
ALTER TABLE workspace_state_versions 
ADD COLUMN lineage VARCHAR(255),
ADD COLUMN serial INTEGER,
ADD COLUMN is_imported BOOLEAN DEFAULT FALSE,
ADD COLUMN import_source VARCHAR(50),
ADD COLUMN is_rollback BOOLEAN DEFAULT FALSE,
ADD COLUMN rollback_from_version INTEGER,
ADD COLUMN description TEXT;

-- 为现有数据填充默认值
UPDATE workspace_state_versions 
SET is_imported = FALSE,
    import_source = 'terraform_apply',
    is_rollback = FALSE
WHERE is_imported IS NULL;

-- 从 content 中提取 lineage 和 serial（如果存在）
UPDATE workspace_state_versions
SET lineage = content->>'lineage',
    serial = CAST(content->>'serial' AS INTEGER)
WHERE content->>'lineage' IS NOT NULL;

-- 添加索引
CREATE INDEX idx_state_versions_lineage ON workspace_state_versions(workspace_id, lineage);
CREATE INDEX idx_state_versions_is_imported ON workspace_state_versions(workspace_id, is_imported);
CREATE INDEX idx_state_versions_is_rollback ON workspace_state_versions(workspace_id, is_rollback);

-- 添加外键约束（rollback_from_version）
ALTER TABLE workspace_state_versions
ADD CONSTRAINT fk_rollback_from_version
FOREIGN KEY (rollback_from_version)
REFERENCES workspace_state_versions(id)
ON DELETE SET NULL;
```

## 6. 安全考虑

### 6.1 权限控制

- **上传 State**：需要 `workspace:state:write` 权限
- **强制上传**：需要 `workspace:state:force_write` 权限（更高级别）
- **回滚 State**：需要 `workspace:state:rollback` 权限

### 6.2 审计日志

所有 state 操作都需要记录审计日志：
- **上传（正常/强制）**：
  - 操作用户：记录在 `workspace_state_versions.created_by`
  - 审计日志：记录操作详情、是否强制上传
  - 示例：`User user-123 uploaded state version 124 (force=true)`

- **回滚**：
  - 操作用户：记录在 `workspace_state_versions.created_by`
  - 审计日志：记录回滚原因、源版本号
  - 示例：`User user-456 rolled back to version 120. Reason: Fix incorrect state`

- **下载**：
  - 审计日志：记录下载用户、版本号
  - 示例：`User user-789 downloaded state version 120`

**用户信息追踪：**
- `created_by` 字段：记录操作用户的 user_id
- UI 显示：显示用户头像和用户名
- 审计日志：记录完整的操作上下文

### 6.3 Workspace 锁定

**锁定策略：**

1. **上传过程中的临时锁定**：
   - 上传开始时：自动锁定 workspace（防止并发修改）
   - 上传成功后：自动释放锁
   - 上传失败后：自动释放锁
   - 锁定原因：`State upload in progress`

2. **强制上传后的持久锁定**：
   - 强制上传成功后：保持 workspace 锁定状态
   - 锁定原因：`Locked after force upload (task #xxx). Please verify state before unlocking.`
   - 需要用户手动解锁并确认

3. **回滚后的持久锁定**：
   - 回滚成功后：保持 workspace 锁定状态
   - 锁定原因：`Locked after state rollback (from version #xxx). Please verify state before unlocking.`
   - 需要用户手动解锁并确认

**锁定实现：**

```go
func (s *StateService) UploadState(
    stateContent map[string]interface{},
    workspaceID string,
    userID string,
    force bool,
) error {
    // 1. 临时锁定（上传过程中）
    if err := s.lockWorkspace(workspaceID, userID, "State upload in progress"); err != nil {
        return fmt.Errorf("failed to lock workspace: %w", err)
    }
    
    // 2. 根据是否强制上传决定是否自动释放锁
    shouldAutoUnlock := !force
    
    defer func() {
        if shouldAutoUnlock {
            // 正常上传：自动释放锁
            if unlockErr := s.unlockWorkspace(workspaceID); unlockErr != nil {
                log.Printf("Warning: failed to unlock workspace: %v", unlockErr)
            }
        } else {
            // 强制上传：保持锁定，更新锁定原因
            s.updateLockReason(workspaceID, 
                fmt.Sprintf("Locked after force upload. Please verify state before unlocking."))
        }
    }()
    
    // 3. 执行上传逻辑...
    // ...
}
```

**锁定时序图：**

```
正常上传流程：
1. Lock workspace (临时)
2. Validate state
3. Save state
4. Unlock workspace (自动)
✓ 用户可以继续操作

强制上传流程：
1. Lock workspace (临时)
2. Skip validation
3. Save state
4. Keep locked (持久)
 需要用户手动解锁

回滚流程：
1. Lock workspace (临时)
2. Create rollback version
3. Save state
4. Keep locked (持久)
 需要用户手动解锁
```

## 7. 实施进度

### Phase 1: 数据模型和后端 API（2天）
- [x] 设计数据模型扩展
- [ ] 编写数据库迁移脚本
- [ ] 实现 State 校验逻辑
- [ ] 实现强制上传 API
- [ ] 实现回滚 API
- [ ] 添加审计日志
- [ ] 单元测试

### Phase 2: 前端 UI（2天）
- [ ] 设计上传界面
- [ ] 实现强制上传警告（内联警告，非弹窗）
- [ ] 实现 State 版本历史列表
- [ ] 实现回滚功能 UI
- [ ] 添加导入标记显示
- [ ] 集成测试

### Phase 3: 权限和安全（1天）
- [ ] 实现权限控制
- [ ] 添加 workspace 自动锁定
- [ ] 完善审计日志
- [ ] 安全测试

### Phase 4: 文档和发布（1天）
- [ ] 编写用户文档
- [ ] 编写 API 文档
- [ ] 代码审查
- [ ] 发布到生产环境

**总计：6个工作日**

## 8. 测试计划

### 8.1 单元测试

- State 校验逻辑测试
  - Lineage 匹配/不匹配
  - Serial 递增/不递增
  - 空 state 处理
- 强制上传测试
- 回滚逻辑测试

### 8.2 集成测试

- 完整上传流程测试
- 强制上传流程测试
- 回滚流程测试
- 权限控制测试

### 8.3 UI 测试

- 上传界面交互测试
- 强制上传警告显示测试
- 回滚界面测试
- 标记显示测试

## 9. 风险和缓解

### 9.1 风险

1. **数据迁移风险**：现有 state 数据可能缺少 lineage/serial
2. **向后兼容性**：旧版本 state 可能无法校验
3. **用户误操作**：强制上传可能导致数据丢失

### 9.2 缓解措施

1. **数据迁移**：
   - 从 content 中提取 lineage/serial
   - 对于缺失的数据，生成默认值
   - 提供数据修复工具

2. **向后兼容**：
   - 对于缺少 lineage 的 state，跳过 lineage 校验
   - 只校验 serial（如果存在）

3. **防止误操作**：
   - 强制上传需要更高权限
   - 显示醒目的内联警告
   - 自动锁定 workspace
   - 记录详细审计日志

## 10. 后续优化

1. **State 差异对比**：显示两个版本之间的差异
2. **自动备份**：在上传前自动备份当前 state
3. **State 验证**：上传后自动验证 state 的完整性
4. **批量回滚**：支持批量回滚多个 workspace
5. **State 导出**：支持导出 state 历史记录

## 11. 参考资料

- Terraform State 文档：https://www.terraform.io/docs/language/state/
- Terraform State Push 命令：https://www.terraform.io/docs/cli/commands/state/push.html
- State Locking：https://www.terraform.io/docs/language/state/locking.html