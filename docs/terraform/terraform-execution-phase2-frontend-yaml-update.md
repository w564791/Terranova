# Phase 2 前端优化 - YAML预览更新

## 任务: 更新Agent Pool详情页的YAML预览

### 当前状态
前端的Agent Pool详情页(`frontend/src/pages/admin/AgentPoolDetail.tsx`)仍然显示Deployment的YAML预览和说明文字。

### 需要更新的内容

#### 1. 函数名称
```typescript
// 当前
const generateDeploymentYaml = () => { ... }

// 应改为
const generatePodYaml = () => { ... }
```

#### 2. YAML内容
```yaml
# 当前: Deployment YAML
apiVersion: apps/v1
kind: Deployment
metadata:
  name: iac-agent-{pool_id}
spec:
  replicas: 1
  selector: ...
  template: ...

# 应改为: Pod YAML
apiVersion: v1
kind: Pod
metadata:
  name: iac-agent-{pool_id}-{timestamp}
  labels:
    app: iac-platform
    component: agent
    pool-id: {pool_id}
spec:
  restartPolicy: Never
  containers:
  - name: agent
    image: {image}
    env:
    - name: POOL_ID
      value: {pool_id}
    - name: IAC_AGENT_NAME
      value: iac-agent-{pool_id}-{timestamp}
    - name: IAC_AGENT_TOKEN
      valueFrom:
        secretKeyRef:
          name: iac-agent-token-{pool_id}
          key: token
    resources:
      limits:
        cpu: {cpu}
        memory: {memory}
      requests:
        cpu: {cpu}
        memory: {memory}
```

#### 3. 说明文字更新

**当前文字**:
```
Kubernetes Deployment模板预览。平台会为每个pool创建一个长期运行的Deployment，并根据pending任务数量自动扩缩容。
```

**应改为**:
```
Kubernetes Pod模板预览。平台会为每个pool直接管理多个Pod（而非Deployment），并根据槽位利用率自动扩缩容。每个Pod有3个槽位，可以并发执行多个plan任务。
```

#### 4. 配置说明更新

**当前**:
```html
<h4>Deployment配置说明：</h4>
<ul>
  <li><strong>Deployment名称</strong>: iac-agent-{pool.pool_id} (pool级别)</li>
  <li><strong>自动扩缩容</strong>: 根据pending任务数量自动调整副本数 (min: {k8sConfig.min_replicas}, max: {k8sConfig.max_replicas})</li>
</ul>
```

**应改为**:
```html
<h4>Pod配置说明：</h4>
<ul>
  <li><strong>Pod命名</strong>: iac-agent-{pool.pool_id}-{timestamp} (每个Pod独立命名)</li>
  <li><strong>槽位管理</strong>: 每个Pod有3个槽位，Slot 0可执行任何任务，Slot 1-2只能执行plan任务</li>
  <li><strong>自动扩缩容</strong>: 根据槽位利用率自动调整Pod数量 (min: {k8sConfig.min_replicas}, max: {k8sConfig.max_replicas})</li>
  <li><strong>安全缩容</strong>: 只删除所有槽位都空闲的Pod，保护正在执行任务的Pod</li>
  <li><strong>Apply保护</strong>: apply_pending任务会预留槽位，确保Pod不会在等待用户确认期间被删除</li>
</ul>
```

#### 5. 英文说明更新

**当前**:
```
Configure the Kubernetes Deployment for this pool. The platform will automatically create and manage a Deployment with auto-scaling based on pending tasks.
```

**应改为**:
```
Configure the Kubernetes Pods for this pool. The platform will directly manage individual Pods (not Deployments) with slot-based auto-scaling. Each Pod has 3 slots for concurrent task execution.
```

#### 6. Token轮换说明更新

**当前**:
```
- 强制重启Deployment (滚动更新)
```

**应改为**:
```
- 强制重启所有Pods (删除并重建)
```

### 实施步骤

1. 读取`frontend/src/pages/admin/AgentPoolDetail.tsx`
2. 找到`generateDeploymentYaml`函数
3. 重命名为`generatePodYaml`
4. 更新YAML模板内容
5. 更新所有相关的UI文字
6. 更新函数调用（handleCopyYaml等）
7. 测试前端显示

### 优先级

**优先级**: 中等

**原因**: 
- 这是一个UI优化，不影响核心功能
- 后端已经使用Pod管理，前端只是显示问题
- 可以在Phase 2测试完成后再优化

### 预计工作量

- 前端代码修改: 0.5天
- 测试验证: 0.2天
- 总计: 0.7天

### 相关文件

- `frontend/src/pages/admin/AgentPoolDetail.tsx` - 主要修改文件
- 可能需要更新的其他文件：
  - `frontend/src/services/agent.ts` - 如果有相关API调用
  - CSS文件 - 如果需要调整样式

### 注意事项

1. **向后兼容**: 虽然后端已改为Pod管理，但前端显示Deployment YAML不会影响功能
2. **用户体验**: 更新后用户会更清楚地了解Pod槽位管理的工作原理
3. **文档一致性**: 确保前端显示与后端实现一致

### 建议

可以作为Phase 2的收尾工作，在核心功能测试通过后进行此优化。
