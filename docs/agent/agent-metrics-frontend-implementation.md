# Agent Metrics 前端实现指南

## 已完成的组件

### 1. AgentMetricsBar 组件
- 文件: `frontend/src/components/AgentMetricsBar.tsx`
- 样式: `frontend/src/components/AgentMetricsBar.module.css`
- 功能: 显示CPU/内存使用率的横向柱状图，带颜色编码

## 需要在 AgentPoolDetail.tsx 中添加的代码

### 1. 添加类型定义

在文件顶部添加：

```typescript
interface AgentMetrics {
  agent_id: string;
  agent_name: string;
  cpu_usage: number;
  memory_usage: number;
  running_tasks: RunningTask[];
  last_update_time: string;
  status: string;
}

interface RunningTask {
  task_id: number;
  task_type: string;
  workspace_id: string;
  started_at: string;
}
```

### 2. 添加状态管理

在组件内添加：

```typescript
const [agentMetrics, setAgentMetrics] = useState<Map<string, AgentMetrics>>(new Map());
const [metricsWs, setMetricsWs] = useState<WebSocket | null>(null);
```

### 3. 添加WebSocket连接逻辑

在useEffect中添加：

```typescript
useEffect(() => {
  if (!poolId) return;

  // 建立WebSocket连接
  const token = localStorage.getItem('token');
  const wsUrl = `ws://localhost:8080/api/v1/ws/agent-pools/${poolId}/metrics`;
  const ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    console.log('Agent metrics WebSocket connected');
  };

  ws.onmessage = (event) => {
    try {
      const message = JSON.parse(event.data);
      
      if (message.type === 'initial_metrics') {
        // 初始化所有agent的metrics
        const metricsMap = new Map<string, AgentMetrics>();
        message.metrics.forEach((m: AgentMetrics) => {
          metricsMap.set(m.agent_id, m);
        });
        setAgentMetrics(metricsMap);
      } else if (message.type === 'metrics_update') {
        // 更新单个agent的metrics
        setAgentMetrics(prev => {
          const newMap = new Map(prev);
          newMap.set(message.metrics.agent_id, message.metrics);
          return newMap;
        });
      } else if (message.type === 'agent_offline') {
        // 移除离线agent的metrics
        setAgentMetrics(prev => {
          const newMap = new Map(prev);
          newMap.delete(message.metrics.agent_id);
          return newMap;
        });
      }
    } catch (error) {
      console.error('Failed to parse WebSocket message:', error);
    }
  };

  ws.onerror = (error) => {
    console.error('WebSocket error:', error);
  };

  ws.onclose = () => {
    console.log('Agent metrics WebSocket disconnected');
  };

  setMetricsWs(ws);

  return () => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.close();
    }
  };
}, [poolId]);
```

### 4. 更新Agents表格显示

在agents表格的`<thead>`中添加列：

```tsx
<th>CPU使用率</th>
<th>内存使用率</th>
<th>运行任务</th>
```

在agents表格的`<tbody>`中添加单元格：

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
      <td style={{ minWidth: '200px' }}>
        {metrics ? (
          <AgentMetricsBar 
            label="CPU" 
            value={metrics.cpu_usage} 
          />
        ) : (
          <span style={{ color: '#8c8c8c' }}>-</span>
        )}
      </td>
      <td style={{ minWidth: '200px' }}>
        {metrics ? (
          <AgentMetricsBar 
            label="Memory" 
            value={metrics.memory_usage} 
          />
        ) : (
          <span style={{ color: '#8c8c8c' }}>-</span>
        )}
      </td>
      <td>
        {metrics && metrics.running_tasks.length > 0 ? (
          <div style={{ fontSize: '12px' }}>
            {metrics.running_tasks.map((task, idx) => (
              <div key={idx} style={{ marginBottom: '4px' }}>
                <span style={{ fontWeight: 500 }}>Task #{task.task_id}</span>
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

### 5. 添加import

在文件顶部添加：

```typescript
import AgentMetricsBar from '../../components/AgentMetricsBar';
```

## 完整的实现效果

1. **实时更新**: Agent每次心跳时，前端会实时收到metrics更新
2. **颜色编码**: 
   - 绿色 (0-70%): 正常
   - 黄色 (70-90%): 警告
   - 红色 (90-100%): 危险
3. **运行任务**: 显示当前agent正在执行的任务列表
4. **自动清理**: 超过5分钟未更新的metrics会自动清理

## 测试步骤

1. 启动后端服务
2. 访问 Agent Pool 详情页面
3. 确认WebSocket连接成功（查看浏览器控制台）
4. Agent发送心跳时应该能看到实时更新的metrics
5. 验证颜色编码是否正确

## 注意事项

1. WebSocket URL需要根据环境配置调整（开发/生产）
2. 需要处理WebSocket断线重连
3. 考虑添加连接状态指示器
4. 大量agent时考虑性能优化
