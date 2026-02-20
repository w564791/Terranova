# AgentPoolDetail WebSocket 集成补丁

## 需要添加的useEffect (WebSocket连接)

在 `useEffect(() => { if (pool && pool.pool_type === 'k8s') { loadK8sConfig(); } }, [pool?.pool_type]);` 之后添加：

```typescript
// WebSocket connection for agent metrics
useEffect(() => {
  if (!poolId) return;

  const wsUrl = `ws://localhost:8080/api/v1/ws/agent-pools/${poolId}/metrics`;
  const ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    console.log(' Agent metrics WebSocket connected');
  };

  ws.onmessage = (event) => {
    try {
      const message = JSON.parse(event.data);
      
      if (message.type === 'initial_metrics') {
        const metricsMap = new Map<string, AgentMetrics>();
        message.metrics.forEach((m: AgentMetrics) => {
          metricsMap.set(m.agent_id, m);
        });
        setAgentMetrics(metricsMap);
        console.log(`📊 Received initial metrics for ${message.metrics.length} agents`);
      } else if (message.type === 'metrics_update') {
        setAgentMetrics(prev => {
          const newMap = new Map(prev);
          newMap.set(message.metrics.agent_id, message.metrics);
          return newMap;
        });
        console.log(`📊 Updated metrics for agent ${message.metrics.agent_id}`);
      } else if (message.type === 'agent_offline') {
        setAgentMetrics(prev => {
          const newMap = new Map(prev);
          newMap.delete(message.metrics.agent_id);
          return newMap;
        });
        console.log(`📊 Agent ${message.metrics.agent_id} went offline`);
      }
    } catch (error) {
      console.error('Failed to parse WebSocket message:', error);
    }
  };

  ws.onerror = (error) => {
    console.error('❌ WebSocket error:', error);
  };

  ws.onclose = () => {
    console.log('❌ Agent metrics WebSocket disconnected');
  };

  setMetricsWs(ws);

  return () => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.close();
    }
  };
}, [poolId]);
```

## 需要更新的Agents表格

### 1. 更新表头 (在 `<thead>` 中)

将：
```tsx
<thead>
  <tr>
    <th>Agent Name</th>
    <th>Agent ID</th>
    <th>Status</th>
    <th>Version</th>
    <th>IP Address</th>
    <th>Last Ping</th>
  </tr>
</thead>
```

替换为：
```tsx
<thead>
  <tr>
    <th>Agent Name</th>
    <th>Agent ID</th>
    <th>Status</th>
    <th>Version</th>
    <th>IP Address</th>
    <th>Last Ping</th>
    <th style={{ minWidth: '200px' }}>CPU使用率</th>
    <th style={{ minWidth: '200px' }}>内存使用率</th>
    <th>运行任务</th>
  </tr>
</thead>
```

### 2. 更新表格行 (在 `<tbody>` 中)

将：
```tsx
{agents.map((agent) => (
  <tr key={agent.agent_id}>
    <td className={styles.agentName}>{agent.name}</td>
    <td className={styles.agentId}>{agent.agent_id}</td>
    <td>{getStatusBadge(agent.status)}</td>
    <td>{agent.version || '-'}</td>
    <td>{agent.ip_address || '-'}</td>
    <td>
      {agent.last_ping_at 
        ? new Date(agent.last_ping_at).toLocaleString()
        : 'Never'}
    </td>
  </tr>
))}
```

替换为：
```tsx
{agents.map((agent) => {
  const metrics = agentMetrics.get(agent.agent_id);
  
  return (
    <tr key={agent.agent_id}>
      <td className={styles.agentName}>{agent.name}</td>
      <td className={styles.agentId}>{agent.agent_id}</td>
      <td>{getStatusBadge(agent.status)}</td>
      <td>{agent.version || '-'}</td>
      <td>{agent.ip_address || '-'}</td>
      <td>
        {agent.last_ping_at 
          ? new Date(agent.last_ping_at).toLocaleString()
          : 'Never'}
      </td>
      <td style={{ minWidth: '200px', padding: '12px 16px' }}>
        {metrics ? (
          <AgentMetricsBar 
            label="CPU" 
            value={metrics.cpu_usage} 
          />
        ) : (
          <span style={{ color: '#8c8c8c' }}>等待数据...</span>
        )}
      </td>
      <td style={{ minWidth: '200px', padding: '12px 16px' }}>
        {metrics ? (
          <AgentMetricsBar 
            label="Memory" 
            value={metrics.memory_usage} 
          />
        ) : (
          <span style={{ color: '#8c8c8c' }}>等待数据...</span>
        )}
      </td>
      <td>
        {metrics && metrics.running_tasks.length > 0 ? (
          <div style={{ fontSize: '12px' }}>
            {metrics.running_tasks.map((task, idx) => (
              <div key={idx} style={{ marginBottom: '4px' }}>
                <span style={{ fontWeight: 500, color: '#1890ff' }}>
                  Task #{task.task_id}
                </span>
                <span style={{ color: '#8c8c8c', marginLeft: '8px' }}>
                  {task.task_type}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <span style={{ color: '#8c8c8c' }}>-</span>
        )}
      </td>
    </tr>
  );
})}
```

## 应用步骤

1. 打开 `frontend/src/pages/admin/AgentPoolDetail.tsx`
2. 在适当位置添加WebSocket useEffect
3. 更新agents表格的thead和tbody
4. 保存文件
5. 测试功能

## 预期效果

-  页面加载时自动连接WebSocket
-  实时显示agent的CPU和内存使用率
-  颜色编码：绿色(0-70%)、黄色(70-90%)、红色(90-100%)
-  显示当前运行的任务列表
-  Agent离线时自动移除metrics显示
