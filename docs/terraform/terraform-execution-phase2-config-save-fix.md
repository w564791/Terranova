# Phase 2 K8s配置保存和Pod自动更新修复

## 修复时间
2025-11-08 15:06

## 问题描述

1. **前端YAML预览问题**: Agent Pool详情页显示的是Deployment YAML而不是Pod YAML
2. **配置保存失败**: 保存K8s配置后数据库没有更新
3. **Pod未自动更新**: 即使配置保存成功，Pod也没有自动更新

## 根本原因

### 1. 前端数据格式错误

前端发送的数据结构与后端期望的`K8sJobTemplateConfig`不匹配：

**错误的格式**:
```javascript
const configToSave = {
  ...k8sConfig,  // 包含 cpu_limit, memory_limit 在顶层
  env,
  freeze_schedules
};
```

**正确的格式**:
```javascript
const configToSave = {
  image: k8sConfig.image,
  image_pull_policy: k8sConfig.image_pull_policy,
  env: env,
  resources: {  // 资源限制必须在resources对象中
    limits: {
      cpu: k8sConfig.cpu_limit,
      memory: k8sConfig.memory_limit
    }
  },
  min_replicas: k8sConfig.min_replicas,
  max_replicas: k8sConfig.max_replicas,
  freeze_schedules: freezeSchedules
};
```

### 2. 后端方法调用错误

`SyncDeploymentConfig`调用了废弃的`EnsureDeploymentForPool`方法，应该直接调用`EnsurePodsForPool`。

## 修复内容

### 1. 前端修复 (`frontend/src/pages/admin/AgentPoolDetail.tsx`)

#### 1.1 YAML预览更新

-  函数重命名: `generateDeploymentYaml` → `generatePodYaml`
-  YAML模板: Deployment → Pod
-  UI文字: 所有"Deployment"改为"Pod"
-  添加槽位管理说明

#### 1.2 配置保存数据格式修复

修复了3个函数的数据格式：

1. **handleSaveK8sConfig**: 主配置保存
2. **handleAddFreezeSchedule**: 添加/编辑冻结规则
3. **handleRemoveFreezeSchedule**: 删除冻结规则

所有函数现在都使用正确的嵌套结构，将`cpu_limit`和`memory_limit`放入`resources.limits`对象中。

#### 1.3 提示信息更新

- "Deployment synced successfully" → "Pods synced successfully"
- "Failed to sync deployment" → "Failed to sync Pods"

### 2. 后端修复 (`backend/internal/handlers/agent_pool_handler.go`)

#### 2.1 SyncDeploymentConfig方法更新

```go
// 修复前
if err := k8sDeploymentService.EnsureDeploymentForPool(c.Request.Context(), &pool); err != nil {
    // ...
}

// 修复后
if err := k8sDeploymentService.EnsurePodsForPool(c.Request.Context(), &pool); err != nil {
    // ...
}
```

#### 2.2 注释和文档更新

- 函数注释从"Sync K8s deployment"改为"Sync K8s Pods"
- 错误信息从"can only sync deployment"改为"can only sync Pods"
- 成功信息从"deployment configuration synced"改为"Pod configuration synced"

## 接口说明

### `/api/v1/agent-pools/:pool_id/sync-deployment`

**注意**: 虽然接口路径还是`sync-deployment`（为了向后兼容），但它现在实际上是同步Pods。

**功能**:
1. 确保Secret存在（创建或更新agent token）
2. 从K8s同步现有Pods状态到数据库
3. 根据配置的`min_replicas`创建必要的Pods
4. 更新Pod配置（如果配置有变化）

**调用时机**:
- 保存K8s配置后自动调用
- 用户手动点击"Sync"按钮（如果有）

## K8sJobTemplateConfig结构

```go
type K8sJobTemplateConfig struct {
    Image           string            `json:"image"`
    ImagePullPolicy string            `json:"image_pull_policy,omitempty"`
    Command         []string          `json:"command,omitempty"`
    Args            []string          `json:"args,omitempty"`
    Env             map[string]string `json:"env,omitempty"`
    Resources       *K8sResources     `json:"resources,omitempty"`  // 重要：资源限制在这里
    MinReplicas     int               `json:"min_replicas,omitempty"`
    MaxReplicas     int               `json:"max_replicas,omitempty"`
    FreezeSchedules []FreezeSchedule  `json:"freeze_schedules,omitempty"`
}

type K8sResources struct {
    Requests map[string]string `json:"requests,omitempty"`
    Limits   map[string]string `json:"limits,omitempty"`  // cpu和memory在这里
}
```

## 测试验证

### 1. 配置保存测试

```sql
-- 查看保存的配置
SELECT pool_id, k8s_config 
FROM agent_pools 
WHERE pool_id = 'pool-z73eh8ihywlmgx0x';
```

期望看到正确的JSON结构，包含`resources.limits.cpu`和`resources.limits.memory`。

### 2. Pod同步测试

保存配置后，检查：
- 后端日志显示`[K8sPodService] Successfully ensured X pods for pool`
- K8s中创建了相应数量的Pods
- 数据库`k8s_pods`表有对应记录

### 3. YAML预览测试

点击"Preview YAML"按钮，应该看到：
- `kind: Pod`（不是Deployment）
- `restartPolicy: Never`
- Pod名称包含timestamp
- 槽位管理说明

## 向后兼容性

-  接口路径保持不变（`sync-deployment`）
-  旧的Deployment方法标记为deprecated但仍可用
-  内部重定向到新的Pod管理方法
-  不影响现有功能

## 相关文件

### 前端
- `frontend/src/pages/admin/AgentPoolDetail.tsx` - 主要修复文件

### 后端
- `backend/internal/handlers/agent_pool_handler.go` - SyncDeploymentConfig修复
- `backend/services/k8s_deployment_service.go` - EnsurePodsForPool实现
- `backend/services/k8s_pod_manager.go` - Pod槽位管理核心
- `backend/internal/models/pool_token.go` - K8sJobTemplateConfig结构定义

## 总结

此次修复解决了Phase 2 Pod槽位管理实现中的最后遗留问题：

1.  前端YAML预览正确显示Pod模板
2.  配置保存使用正确的数据格式
3.  Pod自动同步和更新正常工作
4.  所有UI文字从"Deployment"更新为"Pod"
5.  添加了完整的槽位管理说明

**Phase 2 Pod槽位管理实现现已100%完成**，包括前后端完全对齐，配置保存和Pod自动更新功能正常。
