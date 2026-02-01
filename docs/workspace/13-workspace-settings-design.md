# Workspace模块 - 设置页面详细设计

> **文档版本**: v1.0  
> **创建日期**: 2025-10-10  
> **状态**: 完整设计

## 📘 概述

本文档详细定义Workspace设置页面的完整设计，包括所有设置选项、交互流程和实现细节。设置页面是Workspace详情页的一个标签页，提供对Workspace配置的完整管理能力。

## 🎯 设计原则

1. **分组清晰**: 设置按功能分组，便于查找
2. **即时保存**: 每个设置项独立保存，无需整体提交
3. **权限控制**: 根据用户权限显示/隐藏设置项
4. **危险操作**: 删除等危险操作需要二次确认
5. **实时反馈**: 设置更改后立即显示结果

## 📄 页面结构

### 路径
```
/workspaces/:id?tab=settings&section=general
```

### 标签页位置
```
Overview | Runs | States | Variables | Health | [Settings]
```

### Settings子页面导航
Settings页面采用左侧tab导航设计，包含以下子页面：
- **General** - 常规设置（基本信息、执行配置、Apply方法）
- **Locking** - 锁定设置
- **Notifications** - 通知设置（Coming Soon）
- **Destruction and Deletion** - 删除设置

## 🎨 UI设计

### 布局结构
```
┌─────────────────────────────────────────────────────────┐
│  Workspace Settings (左侧导航栏 - 260px)                  │
│  ┌─────────────────┐  ┌──────────────────────────────┐  │
│  │ General         │  │  General Settings            │  │
│  │ Locking         │  │  ─────────────────────────   │  │
│  │ Notifications   │  │  [内容区域]                   │  │
│  │ Destruction     │  │                              │  │
│  └─────────────────┘  └──────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 设计特点
1. **左侧固定导航** - 260px宽度，固定在左侧
2. **右侧内容区** - 自适应宽度，最大1200px
3. **清晰的视觉层次** - 使用卡片和分组
4. **保存按钮** - 每个设置页面底部有统一的保存按钮
5. **响应式设计** - 移动端自动调整布局

## 🔧 设置分组

### 1. 常规设置 (General)

#### 1.1 基本信息

**字段**:
- **Name** (名称) - 必填，唯一
- **Description** (描述) - 可选
- **Tags** (标签) - 可选，多个标签

**布局**:
```tsx
<div className={styles.section}>
  <h2 className={styles.sectionTitle}>基本信息</h2>
  
  <div className={styles.field}>
    <label className={styles.label}>
      名称 <span className={styles.required}>*</span>
    </label>
    <input
      type="text"
      value={name}
      onChange={handleNameChange}
      onBlur={handleNameSave}
      className={styles.input}
      placeholder="workspace-name"
    />
    {nameError && (
      <div className={styles.error}>{nameError}</div>
    )}
    <div className={styles.hint}>
      名称只能包含字母、数字、横线和下划线
    </div>
  </div>
  
  <div className={styles.field}>
    <label className={styles.label}>描述</label>
    <textarea
      value={description}
      onChange={handleDescriptionChange}
      onBlur={handleDescriptionSave}
      className={styles.textarea}
      rows={3}
      placeholder="描述此Workspace的用途..."
    />
  </div>
  
  <div className={styles.field}>
    <label className={styles.label}>标签</label>
    <TagInput
      tags={tags}
      onChange={handleTagsChange}
      onBlur={handleTagsSave}
      placeholder="添加标签..."
    />
    <div className={styles.hint}>
      标签用于组织和筛选Workspace
    </div>
  </div>
</div>
```

**保存逻辑**:
```typescript
const handleNameSave = async () => {
  if (!validateName(name)) {
    setNameError('名称格式不正确');
    return;
  }
  
  try {
    await api.patch(`/workspaces/${workspaceId}`, { name });
    toast.success('名称已更新');
    setNameError('');
  } catch (error) {
    if (error.code === 'NAME_EXISTS') {
      setNameError('名称已存在');
    } else {
      toast.error(`更新失败: ${error.message}`);
    }
    // 保留用户输入，不回滚
  }
};
```

#### 1.2 执行配置

**字段**:
- **Execution Mode** (执行模式) - local/agent/k8s
- **Agent Pool** (Agent池) - 仅agent模式
- **K8s Config** (K8s配置) - 仅k8s模式
- **Terraform Version** (Terraform版本)
- **Working Directory** (工作目录)

**布局**:
```tsx
<div className={styles.section}>
  <h2 className={styles.sectionTitle}>执行配置</h2>
  
  <div className={styles.field}>
    <label className={styles.label}>
      执行模式 <span className={styles.required}>*</span>
    </label>
    <select
      value={executionMode}
      onChange={handleExecutionModeChange}
      className={styles.select}
    >
      <option value="local">Local - 本地执行</option>
      <option value="agent">Agent - 远程Agent执行</option>
      <option value="k8s">K8s - Kubernetes执行</option>
    </select>
    <div className={styles.hint}>
      更改执行模式将影响后续所有任务的执行方式
    </div>
  </div>
  
  {/* Agent模式配置 */}
  {executionMode === 'agent' && (
    <div className={styles.field}>
      <label className={styles.label}>
        Agent Pool <span className={styles.required}>*</span>
      </label>
      <select
        value={agentPoolId}
        onChange={handleAgentPoolChange}
        className={styles.select}
      >
        <option value="">选择Agent Pool</option>
        {agentPools.map(pool => (
          <option key={pool.id} value={pool.id}>
            {pool.name} ({pool.selection_strategy})
          </option>
        ))}
      </select>
      {agentPoolError && (
        <div className={styles.error}>{agentPoolError}</div>
      )}
    </div>
  )}
  
  {/* K8s模式配置 */}
  {executionMode === 'k8s' && (
    <div className={styles.field}>
      <label className={styles.label}>
        K8s配置 <span className={styles.required}>*</span>
      </label>
      <select
        value={k8sConfigId}
        onChange={handleK8sConfigChange}
        className={styles.select}
      >
        <option value="">选择K8s配置</option>
        {k8sConfigs.map(config => (
          <option key={config.id} value={config.id}>
            {config.name} ({config.namespace})
          </option>
        ))}
      </select>
    </div>
  )}
  
  <div className={styles.field}>
    <label className={styles.label}>Terraform版本</label>
    <select
      value={terraformVersion}
      onChange={handleTerraformVersionChange}
      className={styles.select}
    >
      {terraformVersions.map(version => (
        <option key={version} value={version}>
          {version}
        </option>
      ))}
    </select>
    <div className={styles.hint}>
      更改版本将在下次运行时生效
    </div>
  </div>
  
  <div className={styles.field}>
    <label className={styles.label}>工作目录</label>
    <input
      type="text"
      value={workingDirectory}
      onChange={handleWorkingDirectoryChange}
      onBlur={handleWorkingDirectorySave}
      className={styles.input}
      placeholder="/"
    />
    <div className={styles.hint}>
      Terraform配置文件所在的目录路径
    </div>
  </div>
</div>
```

**执行模式切换逻辑**:
```typescript
const handleExecutionModeChange = async (e: React.ChangeEvent<HTMLSelectElement>) => {
  const newMode = e.target.value;
  
  // 确认对话框
  const confirmed = await confirmDialog({
    title: '更改执行模式',
    message: `确定要将执行模式从 ${executionMode} 更改为 ${newMode} 吗？这将影响后续所有任务的执行方式。`,
    confirmText: '确定',
    cancelText: '取消'
  });
  
  if (!confirmed) return;
  
  try {
    // 更新执行模式
    await api.patch(`/workspaces/${workspaceId}`, {
      execution_mode: newMode,
      // 清除旧模式的配置
      agent_pool_id: newMode === 'agent' ? agentPoolId : null,
      k8s_config_id: newMode === 'k8s' ? k8sConfigId : null
    });
    
    setExecutionMode(newMode);
    toast.success('执行模式已更新');
    
    // 如果切换到agent或k8s模式，提示选择配置
    if (newMode === 'agent' && !agentPoolId) {
      toast.info('请选择Agent Pool');
    } else if (newMode === 'k8s' && !k8sConfigId) {
      toast.info('请选择K8s配置');
    }
  } catch (error) {
    toast.error(`更新失败: ${error.message}`);
    // 保持原值
  }
};
```

#### 1.3 自动化配置

**字段**:
- **Apply Method** (Apply方式) - Manual/Auto
- **Auto Destroy** (自动销毁) - 开关

**布局**:
```tsx
<div className={styles.section}>
  <h2 className={styles.sectionTitle}>自动化配置</h2>
  
  <div className={styles.field}>
    <label className={styles.label}>Apply方式</label>
    <select
      value={applyMethod}
      onChange={handleApplyMethodChange}
      className={styles.select}
    >
      <option value="manual">Manual - 手动审批</option>
      <option value="auto">Auto - 自动执行</option>
    </select>
    <div className={styles.hint}>
      {applyMethod === 'manual' 
        ? 'Plan成功后需要手动审批才能执行Apply'
        : 'Plan成功后自动执行Apply，无需人工干预'
      }
    </div>
  </div>
  
  <div className={styles.field}>
    <div className={styles.switchField}>
      <div className={styles.switchLabel}>
        <span>自动销毁</span>
        <Switch
          checked={autoDestroy}
          onChange={handleAutoDestroyChange}
        />
      </div>
      <div className={styles.hint}>
        删除Workspace时自动销毁所有管理的资源
      </div>
    </div>
  </div>
  
  {autoDestroy && (
    <div className={styles.warning}>
      <span className={styles.warningIcon}></span>
      <span>启用自动销毁后，删除Workspace将不可恢复地销毁所有资源</span>
    </div>
  )}
</div>
```

#### 1.4 用户界面

**字段**:
- **Structured Run Output** (结构化输出) - 开关
- **Console UI** (控制台UI) - 开关

**布局**:
```tsx
<div className={styles.section}>
  <h2 className={styles.sectionTitle}>用户界面</h2>
  
  <div className={styles.field}>
    <div className={styles.switchField}>
      <div className={styles.switchLabel}>
        <span>结构化运行输出</span>
        <Switch
          checked={structuredOutput}
          onChange={handleStructuredOutputChange}
        />
      </div>
      <div className={styles.hint}>
        以JSON格式显示Terraform输出，便于解析和处理
      </div>
    </div>
  </div>
  
  <div className={styles.field}>
    <div className={styles.switchField}>
      <div className={styles.switchLabel}>
        <span>控制台UI</span>
        <Switch
          checked={consoleUI}
          onChange={handleConsoleUIChange}
        />
      </div>
      <div className={styles.hint}>
        显示原始Terraform控制台输出
      </div>
    </div>
  </div>
</div>
```

### 2. 锁定设置 (Locking)

**用途**: 防止并发修改，保护Workspace

**布局**:
```tsx
<div className={styles.section}>
  <h2 className={styles.sectionTitle}>Workspace锁定</h2>
  
  <div className={styles.lockStatus}>
    <div className={styles.statusBadge}>
      {isLocked ? (
        <>
          <span className={styles.lockedIcon}>🔒</span>
          <span>已锁定</span>
        </>
      ) : (
        <>
          <span className={styles.unlockedIcon}>🔓</span>
          <span>未锁定</span>
        </>
      )}
    </div>
    
    {isLocked && (
      <div className={styles.lockInfo}>
        <div>锁定者: {lockedBy}</div>
        <div>锁定时间: {formatDate(lockedAt)}</div>
        <div>原因: {lockReason || '无'}</div>
      </div>
    )}
  </div>
  
  <div className={styles.field}>
    <div className={styles.switchField}>
      <div className={styles.switchLabel}>
        <span>锁定此Workspace</span>
        <Switch
          checked={isLocked}
          onChange={handleLockToggle}
          disabled={isLocked && !canUnlock}
        />
      </div>
      <div className={styles.hint}>
        锁定后将阻止所有Plan和Apply操作
      </div>
    </div>
  </div>
  
  {showLockReason && (
    <div className={styles.field}>
      <label className={styles.label}>锁定原因</label>
      <textarea
        value={lockReason}
        onChange={(e) => setLockReason(e.target.value)}
        className={styles.textarea}
        rows={2}
        placeholder="说明锁定原因..."
      />
    </div>
  )}
  
  <div className={styles.actions}>
    {isLocked ? (
      <button
        onClick={handleUnlock}
        className={styles.dangerButton}
        disabled={!canUnlock}
      >
        解锁Workspace
      </button>
    ) : (
      <button
        onClick={handleLock}
        className={styles.primaryButton}
      >
        锁定Workspace
      </button>
    )}
  </div>
</div>
```

**锁定逻辑**:
```typescript
const handleLock = async () => {
  if (!lockReason.trim()) {
    toast.warning('请输入锁定原因');
    return;
  }
  
  try {
    await api.post(`/workspaces/${workspaceId}/lock`, {
      reason: lockReason
    });
    
    setIsLocked(true);
    setLockedBy(currentUser.email);
    setLockedAt(new Date().toISOString());
    toast.success('Workspace已锁定');
  } catch (error) {
    toast.error(`锁定失败: ${error.message}`);
  }
};

const handleUnlock = async () => {
  const confirmed = await confirmDialog({
    title: '解锁Workspace',
    message: '确定要解锁此Workspace吗？解锁后其他用户可以执行Plan和Apply操作。',
    confirmText: '解锁',
    cancelText: '取消'
  });
  
  if (!confirmed) return;
  
  try {
    await api.post(`/workspaces/${workspaceId}/unlock`);
    
    setIsLocked(false);
    setLockReason('');
    toast.success('Workspace已解锁');
  } catch (error) {
    toast.error(`解锁失败: ${error.message}`);
  }
};
```

### 3. 通知设置 (Notifications)

**用途**: 配置Webhook通知

**布局**:
```tsx
<div className={styles.section}>
  <h2 className={styles.sectionTitle}>通知设置</h2>
  
  <div className={styles.sectionHeader}>
    <p className={styles.sectionDescription}>
      配置Webhook以在特定事件发生时接收通知
    </p>
    <button
      onClick={handleAddWebhook}
      className={styles.addButton}
    >
      + 添加Webhook
    </button>
  </div>
  
  {webhooks.length === 0 ? (
    <div className={styles.empty}>
      <div className={styles.emptyIcon}>🔔</div>
      <div className={styles.emptyText}>暂无Webhook配置</div>
      <div className={styles.emptyHint}>
        点击"添加Webhook"创建第一个通知配置
      </div>
    </div>
  ) : (
    <div className={styles.webhookList}>
      {webhooks.map(webhook => (
        <div key={webhook.id} className={styles.webhookCard}>
          <div className={styles.webhookHeader}>
            <h4 className={styles.webhookName}>{webhook.name}</h4>
            <div className={styles.webhookActions}>
              <button
                onClick={() => handleTestWebhook(webhook.id)}
                className={styles.testButton}
              >
                测试
              </button>
              <button
                onClick={() => handleEditWebhook(webhook.id)}
                className={styles.editButton}
              >
                编辑
              </button>
              <button
                onClick={() => handleDeleteWebhook(webhook.id)}
                className={styles.deleteButton}
              >
                删除
              </button>
            </div>
          </div>
          
          <div className={styles.webhookDetails}>
            <div className={styles.webhookUrl}>
              <span className={styles.label}>URL:</span>
              <code className={styles.code}>{webhook.url}</code>
            </div>
            
            <div className={styles.webhookEvents}>
              <span className={styles.label}>事件:</span>
              <div className={styles.eventTags}>
                {webhook.events.map(event => (
                  <span key={event} className={styles.eventTag}>
                    {eventLabels[event]}
                  </span>
                ))}
              </div>
            </div>
            
            <div className={styles.webhookStatus}>
              <span className={styles.label}>状态:</span>
              <span className={`${styles.statusBadge} ${webhook.enabled ? styles.enabled : styles.disabled}`}>
                {webhook.enabled ? '启用' : '禁用'}
              </span>
            </div>
          </div>
        </div>
      ))}
    </div>
  )}
</div>
```

**Webhook事件类型**:
```typescript
const eventLabels = {
  'run.created': 'Run创建',
  'run.planning': 'Plan开始',
  'run.planned': 'Plan完成',
  'run.applying': 'Apply开始',
  'run.applied': 'Apply完成',
  'run.errored': 'Run失败',
  'run.cancelled': 'Run取消',
  'drift.detected': 'Drift检测',
  'state.updated': 'State更新',
  'workspace.locked': 'Workspace锁定',
  'workspace.unlocked': 'Workspace解锁'
};
```

**添加Webhook对话框**:
```tsx
<ConfirmDialog
  isOpen={isWebhookDialogOpen}
  onClose={() => setIsWebhookDialogOpen(false)}
  onConfirm={handleSaveWebhook}
  title={editingWebhook ? '编辑Webhook' : '添加Webhook'}
  confirmText="保存"
  cancelText="取消"
>
  <form className={styles.webhookForm}>
    <div className={styles.field}>
      <label className={styles.label}>
        名称 <span className={styles.required}>*</span>
      </label>
      <input
        type="text"
        value={webhookForm.name}
        onChange={(e) => setWebhookForm({...webhookForm, name: e.target.value})}
        placeholder="例如: Slack通知"
      />
    </div>
    
    <div className={styles.field}>
      <label className={styles.label}>
        Webhook URL <span className={styles.required}>*</span>
      </label>
      <input
        type="url"
        value={webhookForm.url}
        onChange={(e) => setWebhookForm({...webhookForm, url: e.target.value})}
        placeholder="https://hooks.slack.com/services/..."
      />
    </div>
    
    <div className={styles.field}>
      <label className={styles.label}>
        触发事件 <span className={styles.required}>*</span>
      </label>
      <div className={styles.checkboxGroup}>
        {Object.entries(eventLabels).map(([event, label]) => (
          <label key={event} className={styles.checkbox}>
            <input
              type="checkbox"
              checked={webhookForm.events.includes(event)}
              onChange={(e) => handleEventToggle(event, e.target.checked)}
            />
            <span>{label}</span>
          </label>
        ))}
      </div>
    </div>
    
    <div className={styles.field}>
      <label className={styles.switchLabel}>
        <span>启用</span>
        <Switch
          checked={webhookForm.enabled}
          onChange={(checked) => setWebhookForm({...webhookForm, enabled: checked})}
        />
      </label>
    </div>
  </form>
</ConfirmDialog>
```

### 4. 团队访问 (Team Access)

**用途**: 管理团队成员访问权限

**权限级别**:
- **Admin**: 完全控制（包括删除Workspace）
- **Write**: 可以运行Plan/Apply，管理变量
- **Read**: 只读访问

**布局**:
```tsx
<div className={styles.section}>
  <h2 className={styles.sectionTitle}>团队访问</h2>
  
  <div className={styles.sectionHeader}>
    <p className={styles.sectionDescription}>
      管理团队成员对此Workspace的访问权限
    </p>
    <button
      onClick={handleAddMember}
      className={styles.addButton}
    >
      + 添加成员
    </button>
  </div>
  
  <div className={styles.memberList}>
    <table className={styles.memberTable}>
      <thead>
        <tr>
          <th>成员</th>
          <th>权限</th>
          <th>添加时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        {members.map(member => (
          <tr key={member.id}>
            <td>
              <div className={styles.memberInfo}>
                <div className={styles.memberAvatar}>
                  {member.name.charAt(0).toUpperCase()}
                </div>
                <div>
                  <div className={styles.memberName}>{member.name}</div>
                  <div className={styles.memberEmail}>{member.email}</div>
                </div>
              </div>
            </td>
            <td>
              <select
                value={member.role}
                onChange={(e) => handleRoleChange(member.id, e.target.value)}
                className={styles.roleSelect}
                disabled={member.id === currentUser.id}
              >
                <option value="admin">Admin</option>
                <option value="write">Write</option>
                <option value="read">Read</option>
              </select>
            </td>
            <td>{formatDate(member.added_at)}</td>
            <td>
              {member.id !== currentUser.id && (
                <button
                  onClick={() => handleRemoveMember(member.id)}
                  className={styles.removeButton}
                >
                  移除
                </button>
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  </div>
</div>
```

**添加成员对话框**:
```tsx
<ConfirmDialog
  isOpen={isMemberDialogOpen}
  onClose={() => setIsMemberDialogOpen(false)}
  onConfirm={handleSaveMember}
  title="添加团队成员"
  confirmText="添加"
  cancelText="取消"
>
  <form className={styles.memberForm}>
    <div className={styles.field}>
      <label className={styles.label}>
        用户 <span className={styles.required}>*</span>
      </label>
      <select
        value={memberForm.userId}
        onChange={(e) => setMemberForm({...memberForm, userId: e.target.value})}
        className={styles.select}
      >
        <option value="">选择用户</option>
        {availableUsers.map(user => (
          <option key={user.id} value={user.id}>
            {user.name} ({user.email})
          </option>
        ))}
      </select>
    </div>
    
    <div className={styles.field}>
      <label className={styles.label}>
        权限 <span className={styles.required}>*</span>
      </label>
      <select
        value={memberForm.role}
        onChange={(e) => setMemberForm({...memberForm, role: e.target.value})}
        className={styles.select}
      >
        <option value="read">Read - 只读访问</option>
        <option value="write">Write - 可运行Plan/Apply</option>
        <option value="admin">Admin - 完全控制</option>
      </select>
      
      <div className={styles.roleDescription}>
        {roleDescriptions[memberForm.role]}
      </div>
    </div>
  </form>
</ConfirmDialog>
```

### 5. 危险区域 (Danger Zone)

**用途**: 删除Workspace等危险操作

**布局**:
```tsx
<div className={styles.section}>
  <h2 className={`${styles.sectionTitle} ${styles.danger}`}>
    危险区域
  </h2>
  
  <div className={styles.dangerZone}>
    <div className={styles.dangerItem}>
      <div className={styles.dangerInfo}>
        <h4 className={styles.dangerTitle}>删除此Workspace</h4>
        <p className={styles.dangerDescription}>
          删除后将无法恢复。{autoDestroy ? '所有管理的资源将被销毁。' : '请先手动销毁资源。'}
        </p>
      </div>
      <button
        onClick={handleDeleteWorkspace}
        className={styles.dangerButton}
        disabled={isLocked}
      >
        删除Workspace
      </button>
    </div>
    
    {isLocked && (
      <div className={styles.dangerWarning}>
        <span className={styles.warningIcon}></span>
        <span>Workspace已锁定，请先解锁才能删除</span>
      </div>
    )}
  </div>
</div>
```

**删除确认流程**:
```typescript
const handleDeleteWorkspace = async () => {
  // 第一次确认
  const confirmed1 = await confirmDialog({
    title: '删除Workspace',
    message: `确定要删除 "${workspaceName}" 吗？此操作无法撤销。`,
    confirmText: '继续',
    cancelText: '取消'
  });
  
  if (!confirmed1) return;
  
  // 第二次确认（输入名称）
  const confirmed2 = await confirmDialog({
    title: '确认删除',
    message: (
      <div>
        <p>请输入Workspace名称以确认删除：</p>
        <input
          type="text"
          placeholder={workspaceName}
          onChange={(e) => setDeleteConfirmName(e.target.value)}
          className={styles.confirmInput}
        />
      </div>
    ),
    confirmText: '删除',
    cancelText: '取消',
    confirmDisabled: deleteConfirmName !== workspaceName
  });
  
  if (!confirmed2) return;
  
  try {
    await api.delete(`/workspaces/${workspaceId}`);
    toast.success('Workspace已删除');
    navigate('/workspaces');
  } catch (error) {
    toast.error(`删除失败: ${error.message}`);
  }
};
```

## 🎨 样式规范

### 颜色定义
```css
/* 设置页面专用颜色 */
--color-danger: #ef4444;
--color-danger-bg: #fee2e2;
--color-warning: #f59e0b;
--color-warning-bg: #fef3c7;
--color-success: #10b981;
--color-success-bg: #d1fae5;
```

### 组件样式
```css
.section {
  background: white;
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
  margin-bottom: var(--spacing-lg);
  box-shadow: var(--shadow-sm);
}

.sectionTitle {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--color-gray-900);
  margin-bottom: var(--spacing-lg);
}

.sectionTitle.danger {
  color: var(--color-danger);
}

.field {
  margin-bottom: var(--spacing-lg);
}

.label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
  color: var(--color-gray-700);
  margin-bottom: var(--spacing-xs);
}

.required {
  color: var(--color-danger);
}

.hint {
  font-size: var(--font-size-xs);
  color: var(--color-gray-500);
  margin-top: var(--spacing-xs);
}

.error {
  font-size: var(--font-size-xs);
  color: var(--color-danger);
  margin-top: var(--spacing-xs);
}

.warning {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  background: var(--color-warning-bg);
  border: 1px solid var(--color-warning);
  border-radius: var(--radius-md);
  margin-top: var(--spacing-md);
}

.warningIcon {
  font-size: var(--font-size-lg);
}

.dangerZone {
  border: 2px solid var(--color-danger);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
}

.dangerItem {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.dangerTitle {
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-semibold);
  color: var(--color-danger);
  margin-bottom: var(--spacing-xs);
}

.dangerDescription {
  font-size: var(--font-size-sm);
  color: var(--color-gray-600);
}

.dangerButton {
  padding: var(--spacing-sm) var(--spacing-lg);
  background: var(--color-danger);
  color: white;
  border: none;
  border-radius: var(--radius-md);
  font-weight: var(--font-weight-medium);
  cursor: pointer;
  transition: all 0.2s;
}

.dangerButton:hover:not(:disabled) {
  background: #dc2626;
}

.dangerButton:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
```

## 📱 响应式设计

### 移动端适配
```css
@media (max-width: 768px) {
  .section {
    padding: var(--spacing-md);
  }
  
  .dangerItem {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-md);
  }
  
  .memberTable {
    display: block;
    overflow-x: auto;
  }
}
```

## 🔐 权限控制

### 权限矩阵

| 设置项 | Read | Write | Admin |
|--------|------|-------|-------|
| 查看设置 |  |  |  |
| 修改基本信息 | ❌ | ❌ |  |
| 修改执行配置 | ❌ | ❌ |  |
| 修改自动化配置 | ❌ | ❌ |  |
| 锁定/解锁 | ❌ |  |  |
| 管理Webhook | ❌ | ❌ |  |
| 管理团队成员 | ❌ | ❌ |  |
| 删除Workspace | ❌ | ❌ |  |

### 权限检查实现
```typescript
const canEditSettings = userRole === 'admin';
const canLock = ['write', 'admin'].includes(userRole);
const canDelete = userRole === 'admin';

// 根据权限显示/隐藏UI
{canEditSettings && (
  <div className={styles.field}>
    {/* 编辑表单 */}
  </div>
)}

{!canEditSettings && (
  <div className={styles.readOnly}>
    {/* 只读显示 */}
  </div>
)}
```

## 📊 数据模型

### WorkspaceSettings接口
```typescript
interface WorkspaceSettings {
  // 基本信息
  name: string;
  description: string;
  tags: string[];
  
  // 执行配置
  execution_mode: 'local' | 'agent' | 'k8s';
  agent_pool_id?: number;
  k8s_config_id?: number;
  terraform_version: string;
  working_directory: string;
  
  // 自动化配置
  auto_apply: boolean;
  auto_destroy: boolean;
  
  // 用户界面
  structured_output: boolean;
  console_ui: boolean;
  
  // 锁定状态
  is_locked: boolean;
  locked_by?: string;
  locked_at?: string;
  lock_reason?: string;
  
  // 通知配置
  webhooks: Webhook[];
  
  // 团队访问
  members: WorkspaceMember[];
}

interface Webhook {
  id: number;
  name: string;
  url: string;
  events: string[];
  enabled: boolean;
  created_at: string;
}

interface WorkspaceMember {
  id: number;
  name: string;
  email: string;
  role: 'admin' | 'write' | 'read';
  added_at: string;
}
```

## 🔄 API端点

### 设置相关API
```typescript
// 获取设置
GET /api/v1/workspaces/:id/settings

// 更新基本信息
PATCH /api/v1/workspaces/:id
{
  "name": "new-name",
  "description": "new description",
  "tags": ["tag1", "tag2"]
}

// 更新执行配置
PATCH /api/v1/workspaces/:id
{
  "execution_mode": "agent",
  "agent_pool_id": 1,
  "terraform_version": "1.6.0"
}

// 锁定/解锁
POST /api/v1/workspaces/:id/lock
{
  "reason": "Maintenance"
}

POST /api/v1/workspaces/:id/unlock

// Webhook管理
GET /api/v1/workspaces/:id/webhooks
POST /api/v1/workspaces/:id/webhooks
PUT /api/v1/workspaces/:id/webhooks/:webhook_id
DELETE /api/v1/workspaces/:id/webhooks/:webhook_id
POST /api/v1/workspaces/:id/webhooks/:webhook_id/test

// 团队成员管理
GET /api/v1/workspaces/:id/members
POST /api/v1/workspaces/:id/members
PUT /api/v1/workspaces/:id/members/:member_id
DELETE /api/v1/workspaces/:id/members/:member_id

// 删除Workspace
DELETE /api/v1/workspaces/:id
```

## 🧪 测试场景

### 功能测试
1. **基本信息更新**
   - 修改名称（成功/失败）
   - 修改描述
   - 添加/删除标签

2. **执行模式切换**
   - Local → Agent
   - Agent → K8s
   - K8s → Local
   - 验证配置清除

3. **锁定功能**
   - 锁定Workspace
   - 尝试执行Plan（应失败）
   - 解锁Workspace
   - 验证权限控制

4. **Webhook管理**
   - 添加Webhook
   - 编辑Webhook
   - 测试Webhook
   - 删除Webhook

5. **团队访问**
   - 添加成员
   - 修改权限
   - 移除成员
   - 验证权限生效

6. **删除Workspace**
   - 锁定状态下删除（应失败）
   - 正常删除流程
   - 验证二次确认

### 边界测试
- 名称重复
- 无效的执行模式配置
- 权限不足的操作
- 网络错误处理

## 📝 实现清单

### 后端任务
- [x] 创建设置API端点
- [x] 实现权限检查中间件
- [ ] Webhook管理服务
- [ ] 团队成员管理服务
- [x] 锁定机制实现
- [x] 删除Workspace逻辑

### 前端任务
- [x] 创建Settings页面组件（左侧tab导航）
- [x] 实现General Settings子页面
- [x] 实现Locking子页面
- [ ] 实现Notifications子页面（标记为Coming Soon）
- [x] 实现Destruction and Deletion子页面
- [ ] Webhook管理UI
- [ ] 团队成员管理UI
- [x] 权限控制逻辑
- [x] 表单验证
- [x] 错误处理
- [x] 样式实现（参考TFE设计）
- [x] 添加保存按钮和未保存提示

### 测试任务
- [ ] 单元测试
- [ ] 集成测试
- [ ] E2E测试
- [ ] 权限测试

## 🎯 实现亮点

### 1. 左侧Tab导航
- 参考TFE和全局系统管理的设计
- 清晰的导航结构，易于扩展
- 支持URL参数（section）切换

### 2. 统一的保存机制
- General Settings页面有统一的"Save Settings"按钮
- 实时跟踪表单修改状态（hasChanges）
- 显示"You have unsaved changes"提示
- 保存成功后自动重新加载数据

### 3. 表单验证
- 实时验证（如名称格式）
- 清晰的错误提示
- 防止重复提交

### 4. 用户体验优化
- 执行模式切换需要确认
- 删除操作需要输入名称确认
- 锁定状态清晰显示
- Coming Soon页面友好提示

### 5. 响应式设计
- 桌面端：左侧固定导航 + 右侧内容
- 移动端：上下布局，导航在顶部
- 自适应表单布局

---

**相关文档**:
- [11-frontend-design.md](./11-frontend-design.md) - 前端设计总览
- [12-global-configuration.md](./12-global-configuration.md) - 全局配置
- [08-database-design.md](./08-database-design.md) - 数据库设计
- [09-api-specification.md](./09-api-specification.md) - API规范
- [../frontend-form-style-guide.md](../frontend-form-style-guide.md) - 表单规范
