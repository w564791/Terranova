# Workspace模块 - 前端设计

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 完整设计

## 📘 概述

本文档定义Workspace模块的前端页面设计、交互规范和用户体验，严格遵循项目前端规范。

## 🎯 核心原则

### 1. 永远保留用户输入
- ❌ 验证失败时不清空输入
- ❌ API错误时不重置表单
-  只清除错误信息，保留数据
-  重要表单使用localStorage持久化

### 2. 统一通知系统
-  使用左下角通知框（Toast）
-  成功/错误/警告统一样式
-  通知可复制内容
- ❌ 禁止使用alert()

### 3. 弹窗规范
-  创建/编辑使用ConfirmDialog
-  删除确认使用ConfirmDialog
-  结果通知使用Toast

## 📄 页面设计

### 1. Workspace列表页

**路径**: `/workspaces`

**功能**:
- 展示所有Workspace
- 筛选（状态、执行模式、标签）
- 排序（名称、创建时间、更新时间）
- 批量操作（删除、锁定）
- 创建新Workspace

**布局**:
```
┌─────────────────────────────────────────────────┐
│ Workspaces                          [+ 创建]     │
├─────────────────────────────────────────────────┤
│ 筛选: [状态▼] [执行模式▼] [标签▼]  搜索: [___] │
├─────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────┐ │
│ │ production-infra          [Created] [Local] │ │
│ │ 描述: Production infrastructure             │ │
│ │ 创建: 2025-10-09  更新: 2025-10-09          │ │
│ │ [查看] [编辑] [删除]                        │ │
│ └─────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────┐ │
│ │ staging-infra            [Planning] [Agent] │ │
│ │ ...                                         │ │
│ └─────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────┤
│ 第1页 共10页                    [上一页] [下一页] │
└─────────────────────────────────────────────────┘
```

**组件**:
```tsx
<div className={styles.container}>
  <div className={styles.header}>
    <h1>Workspaces</h1>
    <button onClick={handleCreate}>+ 创建</button>
  </div>
  
  <div className={styles.filters}>
    <select value={stateFilter} onChange={handleStateFilter}>
      <option value="">所有状态</option>
      <option value="created">Created</option>
      <option value="planning">Planning</option>
      {/* ... */}
    </select>
    
    <select value={modeFilter} onChange={handleModeFilter}>
      <option value="">所有模式</option>
      <option value="local">Local</option>
      <option value="agent">Agent</option>
      <option value="k8s">K8s</option>
    </select>
    
    <input 
      type="text" 
      placeholder="搜索..." 
      value={searchQuery}
      onChange={handleSearch}
    />
  </div>
  
  <div className={styles.list}>
    {workspaces.map(ws => (
      <WorkspaceCard key={ws.id} workspace={ws} />
    ))}
  </div>
  
  <Pagination 
    page={page} 
    totalPages={totalPages}
    onPageChange={handlePageChange}
  />
</div>
```

### 2. 创建Workspace弹窗

**触发**: 点击"+ 创建"按钮

**使用组件**: `ConfirmDialog`

**表单字段**:
```tsx
<ConfirmDialog
  isOpen={isCreateDialogOpen}
  onClose={handleCloseDialog}
  onConfirm={handleCreateWorkspace}
  title="创建Workspace"
  confirmText="创建"
  cancelText="取消"
>
  <form className={styles.form}>
    {/* 基础信息 */}
    <div className={styles.field}>
      <label>
        名称 <span className={styles.required}>*</span>
      </label>
      <input
        type="text"
        value={formData.name}
        onChange={(e) => handleFieldChange('name', e.target.value)}
        placeholder="例如: production-infra"
      />
      {errors.name && (
        <div className={styles.error}>{errors.name}</div>
      )}
    </div>
    
    <div className={styles.field}>
      <label>描述</label>
      <textarea
        value={formData.description}
        onChange={(e) => handleFieldChange('description', e.target.value)}
        placeholder="描述此Workspace的用途"
        rows={3}
      />
    </div>
    
    {/* 执行配置 */}
    <div className={styles.section}>
      <h3>执行配置</h3>
      
      <div className={styles.field}>
        <label>
          执行模式 <span className={styles.required}>*</span>
        </label>
        <select
          value={formData.execution_mode}
          onChange={(e) => handleFieldChange('execution_mode', e.target.value)}
        >
          <option value="local">Local - 本地执行</option>
          <option value="agent">Agent - 远程Agent执行</option>
          <option value="k8s">K8s - Kubernetes执行</option>
        </select>
      </div>
      
      {/* Agent模式配置 */}
      {formData.execution_mode === 'agent' && (
        <div className={styles.field}>
          <label>
            Agent Pool <span className={styles.required}>*</span>
          </label>
          <select
            value={formData.agent_pool_id}
            onChange={(e) => handleFieldChange('agent_pool_id', e.target.value)}
          >
            <option value="">选择Agent Pool</option>
            {agentPools.map(pool => (
              <option key={pool.id} value={pool.id}>
                {pool.name}
              </option>
            ))}
          </select>
          {errors.agent_pool_id && (
            <div className={styles.error}>{errors.agent_pool_id}</div>
          )}
        </div>
      )}
      
      {/* K8s模式配置 */}
      {formData.execution_mode === 'k8s' && (
        <div className={styles.field}>
          <label>
            K8s配置 <span className={styles.required}>*</span>
          </label>
          <select
            value={formData.k8s_config_id}
            onChange={(e) => handleFieldChange('k8s_config_id', e.target.value)}
          >
            <option value="">选择K8s配置</option>
            {k8sConfigs.map(config => (
              <option key={config.id} value={config.id}>
                {config.name}
              </option>
            ))}
          </select>
        </div>
      )}
      
      <div className={styles.field}>
        <label>Terraform版本</label>
        <select
          value={formData.terraform_version}
          onChange={(e) => handleFieldChange('terraform_version', e.target.value)}
        >
          {terraformVersions.map(version => (
            <option key={version} value={version}>
              {version}
            </option>
          ))}
        </select>
      </div>
    </div>
    
    {/* 自动化配置 */}
    <div className={styles.section}>
      <h3>自动化配置</h3>
      
      <div className={styles.field}>
        <label className={styles.switchLabel}>
          <span>自动Apply</span>
          <Switch
            checked={formData.auto_apply}
            onChange={(checked) => handleFieldChange('auto_apply', checked)}
          />
        </label>
        <div className={styles.description}>
          Plan成功后自动执行Apply
        </div>
      </div>
      
      <div className={styles.field}>
        <label className={styles.switchLabel}>
          <span>自动Destroy</span>
          <Switch
            checked={formData.auto_destroy}
            onChange={(checked) => handleFieldChange('auto_destroy', checked)}
          />
        </label>
        <div className={styles.description}>
          删除Workspace时自动销毁资源
        </div>
      </div>
    </div>
    
    {/* 标签 */}
    <div className={styles.field}>
      <label>标签</label>
      <TagInput
        tags={formData.tags}
        onChange={(tags) => handleFieldChange('tags', tags)}
        placeholder="添加标签..."
      />
    </div>
  </form>
</ConfirmDialog>
```

**表单验证**:
```typescript
const validateForm = (): boolean => {
  const newErrors: Record<string, string> = {};
  
  // 名称验证
  if (!formData.name.trim()) {
    newErrors.name = '名称不能为空';
  } else if (!/^[a-zA-Z0-9-_]+$/.test(formData.name)) {
    newErrors.name = '名称只能包含字母、数字、横线和下划线';
  }
  
  // Agent Pool验证
  if (formData.execution_mode === 'agent' && !formData.agent_pool_id) {
    newErrors.agent_pool_id = '请选择Agent Pool';
  }
  
  // K8s配置验证
  if (formData.execution_mode === 'k8s' && !formData.k8s_config_id) {
    newErrors.k8s_config_id = '请选择K8s配置';
  }
  
  setErrors(newErrors);
  return Object.keys(newErrors).length === 0;
};
```

**提交处理**:
```typescript
const handleCreateWorkspace = async () => {
  if (!validateForm()) {
    return; // 验证失败，保留用户输入
  }
  
  setIsSubmitting(true);
  
  try {
    const response = await api.post('/workspaces', formData);
    
    // 成功通知
    toast.success('Workspace创建成功');
    
    // 关闭弹窗
    setIsCreateDialogOpen(false);
    
    // 清空表单（只在成功后清空）
    setFormData(initialFormData);
    
    // 刷新列表
    fetchWorkspaces();
    
    // 跳转到详情页
    navigate(`/workspaces/${response.data.id}`);
    
  } catch (error) {
    // 错误通知
    toast.error(`创建失败: ${error.message}`);
    
    // 保留用户输入，不清空表单
    // formData保持不变
  } finally {
    setIsSubmitting(false);
  }
};
```

### 3. Workspace详情页

**路径**: `/workspaces/:id`

**标签页**: Overview | Runs | States | Variables | Health | Settings

#### 3.1 Overview标签页

**布局**:
```
┌─────────────────────────────────────────────────┐
│ ← 返回  production-infra    [Unlocked] [Local] │
├─────────────────────────────────────────────────┤
│ [Overview] Runs  States  Variables  Health  Settings │
├─────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────┐ │
│ │ Workspace信息                               │ │
│ │ ID: ws-abc123                               │ │
│ │ Description: Production infrastructure      │ │
│ │ Status: Unlocked                            │ │
│ │ Resources: 45 managed                       │ │
│ │ Terraform Version: 1.6.0                    │ │
│ │ Last Plan: 2025-10-09 15:30 (2h ago)       │ │
│ │ Last Apply: 2025-10-09 14:00 (3h ago)      │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ ┌─────────────────────────────────────────────┐ │
│ │ 最近运行                                    │ │
│ │ Run #123 - Update security rules            │ │
│ │ By: admin@example.com                       │ │
│ │ Status: [Success]                           │ │
│ │ Plan: 45.2s | Apply: 120.5s                 │ │
│ │ Changes: +3 ~2 -1                           │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ ┌─────────────────────────────────────────────┐ │
│ │ 配置                                        │ │
│ │ Auto Apply: [Enabled]                       │ │
│ │ Execution Mode: Local                       │ │
│ │ Working Directory: /                        │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ ┌─────────────────────────────────────────────┐ │
│ │ Health状态                                  │ │
│ │ Drift Detected: 3 resources                 │ │
│ │ Last Check: 2025-10-09 16:00                │ │
│ │ [查看详情]                                  │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ ┌─────────────────────────────────────────────┐ │
│ │ 管理的资源 (45)                             │ │
│ │ aws_instance.web (3)                        │ │
│ │ aws_security_group.main (1)                 │ │
│ │ aws_s3_bucket.data (2)                      │ │
│ │ ... [查看全部]                              │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

**数据获取**:
```typescript
interface WorkspaceOverview {
  id: string;
  name: string;
  description: string;
  is_locked: boolean;
  execution_mode: string;
  terraform_version: string;
  resource_count: number;
  last_plan_at: string;
  last_apply_at: string;
  auto_apply: boolean;
  working_directory: string;
  drift_count: number;
  last_drift_check: string;
  latest_run: {
    id: number;
    message: string;
    created_by: string;
    status: string;
    plan_duration: number;
    apply_duration: number;
    changes: {
      add: number;
      change: number;
      destroy: number;
    };
  };
  resources: Array<{
    type: string;
    count: number;
  }>;
}
```

#### 3.2 Runs标签页

**布局**:
```
┌─────────────────────────────────────────────────┐
│ Overview [Runs] States  Variables  Health  Settings │
├─────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────┐ │
│ │ Current Run                                 │ │
│ │ Run #125 - Deploy new features              │ │
│ │ Status: [Running]                           │ │
│ │ Started: 2025-10-09 17:00                   │ │
│ │ By: admin@example.com                       │ │
│ │ [查看日志] [取消]                           │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ 快速过滤: [All] Needs Attention  Errored  Running  On Hold  Success │
│                                                  │
│ ┌─────────────────────────────────────────────┐ │
│ │ #125 Deploy new features      [Running]     │ │
│ │ 2025-10-09 17:00 | admin@example.com        │ │
│ │ [查看详情]                                  │ │
│ ├─────────────────────────────────────────────┤ │
│ │ #124 Update security rules    [Success]     │ │
│ │ 2025-10-09 15:30 | admin@example.com        │ │
│ │ Plan: 45.2s | Apply: 120.5s | +3 ~2 -1     │ │
│ │ [查看详情]                                  │ │
│ ├─────────────────────────────────────────────┤ │
│ │ #123 Fix configuration        [Failed]      │ │
│ │ 2025-10-09 14:00 | user@example.com         │ │
│ │ Error: Invalid configuration                │ │
│ │ [查看详情]                                  │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

**快速过滤**:
- All: 所有运行
- Needs Attention: 需要关注（等待审批、有错误）
- Errored: 失败的运行
- Running: 正在运行
- On Hold: 等待审批
- Success: 成功的运行

**Run详情**:
```typescript
interface Run {
  id: number;
  message: string;
  status: 'pending' | 'running' | 'success' | 'failed' | 'on_hold';
  created_at: string;
  created_by: string;
  plan_duration?: number;
  apply_duration?: number;
  changes?: {
    add: number;
    change: number;
    destroy: number;
  };
  error?: string;
}
```

#### 3.3 States标签页

**布局**:
```
┌─────────────────────────────────────────────────┐
│ Overview  Runs [States] Variables  Health  Settings │
├─────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────┐ │
│ │ v5 (Current) - Run #124                     │ │
│ │ 2025-10-09 15:30:00                         │ │
│ │ Size: 1.2MB | Checksum: sha256:abc123...    │ │
│ │ Resources: 45                               │ │
│ │ [查看] [下载]                               │ │
│ ├─────────────────────────────────────────────┤ │
│ │ v4 - Run #123                               │ │
│ │ 2025-10-09 14:00:00                         │ │
│ │ Size: 1.1MB | Checksum: sha256:def456...    │ │
│ │ Resources: 43                               │ │
│ │ [查看] [下载] [对比] [回滚]                 │ │
│ ├─────────────────────────────────────────────┤ │
│ │ v3 - Run #122                               │ │
│ │ 2025-10-09 12:00:00                         │ │
│ │ Size: 1.0MB | Checksum: sha256:ghi789...    │ │
│ │ Resources: 42                               │ │
│ │ [查看] [下载] [对比] [回滚]                 │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

**回滚功能**:
```typescript
const handleRollback = async (versionId: number) => {
  // 确认对话框
  const confirmed = await confirmDialog({
    title: '回滚State版本',
    message: '这将创建一个新的Apply任务来回滚到此版本的状态。确定继续？',
    confirmText: '回滚',
    cancelText: '取消'
  });
  
  if (!confirmed) return;
  
  try {
    // 创建回滚任务（通过Terraform apply回滚，而不是直接修改state）
    await api.post(`/workspaces/${workspaceId}/state-versions/${versionId}/rollback`);
    toast.success('回滚任务已创建');
  } catch (error) {
    toast.error(`回滚失败: ${error.message}`);
  }
};
```

#### 3.4 Variables标签页

**布局**:
```
┌─────────────────────────────────────────────────┐
│ Overview  Runs  States [Variables] Health  Settings │
├─────────────────────────────────────────────────┤
│ [+ 添加变量]                                     │
│                                                  │
│ ┌─────────────────────────────────────────────┐ │
│ │ environment                                 │ │
│ │ Type: [Terraform Variable ▼]                │ │
│ │ Value: production                           │ │
│ │ Format: [String ▼] □ Sensitive              │ │
│ │ Description: Environment name               │ │
│ │ [编辑] [删除]                               │ │
│ ├─────────────────────────────────────────────┤ │
│ │ aws_region                                  │ │
│ │ Type: [Terraform Variable ▼]                │ │
│ │ Value: us-west-2                            │ │
│ │ Format: [String ▼] □ Sensitive              │ │
│ │ Description: AWS region                     │ │
│ │ [编辑] [删除]                               │ │
│ ├─────────────────────────────────────────────┤ │
│ │ instance_count                              │ │
│ │ Type: [Terraform Variable ▼]                │ │
│ │ Value: 3                                    │ │
│ │ Format: [HCL ▼] □ Sensitive                 │ │
│ │ Description: Number of instances            │ │
│ │ [编辑] [删除]                               │ │
│ ├─────────────────────────────────────────────┤ │
│ │ AWS_ACCESS_KEY_ID                           │ │
│ │ Type: [Environment Variable ▼]              │ │
│ │ Value: ******** (Sensitive)                 │ │
│ │ Format: [String ▼] ☑ Sensitive              │ │
│ │ Description: AWS access key                 │ │
│ │ [编辑] [删除]                               │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

**变量类型**:
- Terraform Variable: 作为`-var`传递给Terraform
- Environment Variable: 作为环境变量传递

**值格式**:
- String: 普通字符串
- HCL: Terraform HCL格式（如数字、列表、对象）

**变量模型**:
```typescript
interface WorkspaceVariable {
  id: number;
  key: string;
  value: string;
  type: 'terraform' | 'environment';
  format: 'string' | 'hcl';
  sensitive: boolean;
  description: string;
}
```

#### 3.5 Health标签页

**布局**:
```
┌─────────────────────────────────────────────────┐
│ Overview  Runs  States  Variables [Health] Settings │
├─────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────┐ │
│ │ Drift检测状态                               │ │
│ │ Last Check: 2025-10-09 16:00 (1h ago)      │ │
│ │ Status: [Drift Detected]                    │ │
│ │ Drifted Resources: 3 / 45                   │ │
│ │ [立即检测] [查看历史]                       │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ ┌─────────────────────────────────────────────┐ │
│ │ Drift详情                                   │ │
│ │                                             │ │
│ │ aws_instance.web-01                         │ │
│ │ Type: Configuration Drift                   │ │
│ │ Risk: [Medium]                              │ │
│ │ Changed: instance_type                      │ │
│ │   Expected: t2.micro                        │ │
│ │   Actual: t2.small                          │ │
│ │ Detected: 2025-10-09 16:00                  │ │
│ │ [查看详情] [修复]                           │ │
│ │                                             │ │
│ ├─────────────────────────────────────────────┤ │
│ │ aws_security_group.main                     │ │
│ │ Type: Configuration Drift                   │ │
│ │ Risk: [High]                                │ │
│ │ Changed: ingress rules                      │ │
│ │   Added: 0.0.0.0/0:22                       │ │
│ │ Detected: 2025-10-09 16:00                  │ │
│ │ [查看详情] [修复]                           │ │
│ │                                             │ │
│ ├─────────────────────────────────────────────┤ │
│ │ aws_s3_bucket.logs                          │ │
│ │ Type: Resource Deleted                      │ │
│ │ Risk: [Critical]                            │ │
│ │ Status: Resource not found                  │ │
│ │ Detected: 2025-10-09 16:00                  │ │
│ │ [查看详情] [修复]                           │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

**Drift类型**:
- Configuration Drift: 配置漂移
- Resource Deleted: 资源被删除
- Unauthorized Resource: 未授权资源

**风险等级**:
- Low: 低风险（标签变更等）
- Medium: 中风险（实例类型变更等）
- High: 高风险（安全组规则变更等）
- Critical: 严重（资源删除、数据丢失风险）

#### 3.6 Settings标签页

**布局**:
```
┌─────────────────────────────────────────────────┐
│ Overview  Runs  States  Variables  Health [Settings] │
├─────────────────────────────────────────────────┤
│ 【6.1 常规设置】                                 │
│ ┌─────────────────────────────────────────────┐ │
│ │ 基本信息                                    │ │
│ │ Name: production-infra                      │ │
│ │ Description: [___________________________]  │ │
│ │                                             │ │
│ │ 执行配置                                    │ │
│ │ Execution Mode: [Local ▼]                   │ │
│ │ Apply Method: [Manual ▼] (Manual/Auto)     │ │
│ │ Terraform Version: [1.6.0 ▼]               │ │
│ │ Working Directory: [/]                      │ │
│ │                                             │ │
│ │ 用户界面                                    │ │
│ │ ☑ Structured Run Output                     │ │
│ │ □ Console UI                                │ │
│ │                                             │ │
│ │ [保存更改]                                  │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ 【6.2 锁定设置】                                 │
│ ┌─────────────────────────────────────────────┐ │
│ │ Workspace锁定                               │ │
│ │ Status: [Unlocked]                          │ │
│ │ □ 锁定此Workspace                           │ │
│ │ Reason: [___________________________]       │ │
│ │ [应用]                                      │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ 【6.3 通知设置】                                 │
│ ┌─────────────────────────────────────────────┐ │
│ │ Webhook通知                                 │ │
│ │ [+ 添加Webhook]                             │ │
│ │                                             │ │
│ │ Slack Notification                          │ │
│ │ URL: https://hooks.slack.com/...            │ │
│ │ Events: plan_completed, apply_completed     │ │
│ │ [编辑] [删除] [测试]                        │ │
│ └─────────────────────────────────────────────┘ │
│                                                  │
│ 【6.4 团队访问】                                 │
│ ┌─────────────────────────────────────────────┐ │
│ │ 团队成员                                    │ │
│ │ [+ 添加成员]                                │ │
│ │                                             │ │
│ │ admin@example.com        [Admin ▼]          │ │
│ │ user@example.com         [Write ▼]          │ │
│ │ viewer@example.com       [Read ▼]           │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

**Apply Method**:
- Manual: 手动审批后执行Apply
- Auto: Plan成功后自动执行Apply

**User Interface选项**:
- Structured Run Output: 结构化输出（JSON格式）
- Console UI: 控制台UI（原始Terraform输出）

**团队访问权限**:
- Admin: 完全控制（包括删除）
- Write: 可以运行Plan/Apply
- Read: 只读访问

### 4. 通知系统

**成功通知**:
```typescript
toast.success('Workspace创建成功');
toast.success('Plan任务已启动');
toast.success('Apply执行成功');
```

**错误通知**:
```typescript
toast.error('创建失败: 名称已存在');
toast.error('Plan执行失败: Terraform配置错误');
toast.error('网络错误，请稍后重试');
```

**警告通知**:
```typescript
toast.warning('Workspace已锁定，无法执行Apply');
toast.warning('State文件较大，下载可能需要一些时间');
```

**信息通知**:
```typescript
toast.info('任务已加入队列，等待执行');
toast.info('正在准备执行环境...');
```

## 🎨 样式规范

### 1. 颜色系统
```css
/* 状态颜色 */
--color-created: #6b7280;      /* 灰色 */
--color-planning: #3b82f6;     /* 蓝色 */
--color-plan-done: #10b981;    /* 绿色 */
--color-applying: #f59e0b;     /* 橙色 */
--color-completed: #10b981;    /* 绿色 */
--color-failed: #ef4444;       /* 红色 */

/* 执行模式颜色 */
--color-local: #8b5cf6;        /* 紫色 */
--color-agent: #3b82f6;        /* 蓝色 */
--color-k8s: #06b6d4;          /* 青色 */
```

### 2. 状态徽章
```tsx
<span className={`${styles.badge} ${styles[state]}`}>
  {stateLabels[state]}
</span>
```

```css
.badge {
  padding: 4px 12px;
  border-radius: var(--radius-md);
  font-size: 12px;
  font-weight: 500;
}

.created { background: #f3f4f6; color: #6b7280; }
.planning { background: #dbeafe; color: #1e40af; }
.planDone { background: #d1fae5; color: #065f46; }
.applying { background: #fed7aa; color: #92400e; }
.completed { background: #d1fae5; color: #065f46; }
.failed { background: #fee2e2; color: #991b1b; }
```

## 📝 表单持久化

**重要表单使用localStorage**:
```typescript
// 保存表单数据
useEffect(() => {
  if (formData.name || formData.description) {
    localStorage.setItem('workspace_form', JSON.stringify(formData));
  }
}, [formData]);

// 恢复表单数据
useEffect(() => {
  const saved = localStorage.getItem('workspace_form');
  if (saved) {
    setFormData(JSON.parse(saved));
  }
}, []);

// 成功后清理
const handleSuccess = () => {
  localStorage.removeItem('workspace_form');
  setFormData(initialFormData);
};
```

## 🔄 加载状态

**按钮加载状态**:
```tsx
<button 
  onClick={handleSubmit}
  disabled={isSubmitting}
  className={styles.submitButton}
>
  {isSubmitting ? '创建中...' : '创建'}
</button>
```

**页面加载状态**:
```tsx
{isLoading ? (
  <div className={styles.loading}>
    <Spinner />
    <p>加载中...</p>
  </div>
) : (
  <WorkspaceList workspaces={workspaces} />
)}
```

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [12-global-configuration.md](./12-global-configuration.md) - 全局配置
- [frontend-form-style-guide.md](../frontend-form-style-guide.md) - 表单规范
- [frontend-ux-rules.md](../.amazonq/prompts/frontend-ux-rules.md) - UX规则
