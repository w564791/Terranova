import React, { useState, useEffect } from 'react';
import TerraformOutputViewer from './TerraformOutputViewer';
import StageLogViewer from './StageLogViewer';
import api from '../services/api';

interface Props {
  taskId: number;
  viewMode?: 'plan' | 'apply'; // 从父组件接收viewMode
  onStageChange?: (stage: string) => void; // 新增：通知父组件当前阶段变化
  currentTaskStage?: string; // 从父组件接收当前任务阶段
}

const SmartLogViewer: React.FC<Props> = ({ taskId, viewMode = 'plan', onStageChange, currentTaskStage }) => {
  const [taskStatus, setTaskStatus] = useState<string>('');
  const [taskType, setTaskType] = useState<string>('plan');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchTaskStatus();
    
    // 定期检查状态（移除taskStatus依赖，避免轮询停止）
    const interval = setInterval(() => {
      fetchTaskStatus();
    }, 2000); // 缩短到2秒，更快响应状态变化

    return () => clearInterval(interval);
  }, [taskId]); // 只依赖taskId

  const fetchTaskStatus = async () => {
    try {
      // 从URL获取workspaceId
      const pathParts = window.location.pathname.split('/');
      const workspaceId = pathParts[2]; // /workspaces/{workspaceId}/tasks/{taskId}
      
      // 使用api服务（会自动添加baseURL和处理响应）
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}`);
      const task = data.task || data;
      
      // 添加调试日志
      console.log('[SmartLogViewer] Task status:', task.status, 'Task type:', task.task_type);
      
      setTaskStatus(task.status);
      setTaskType(task.task_type || 'plan');
      setError(null);
    } catch (err: any) {
      console.error('Failed to fetch task status:', err);
      setError(err.message || 'Failed to fetch task status');
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div style={{ 
        display: 'flex', 
        alignItems: 'center', 
        justifyContent: 'center', 
        height: '400px',
        color: 'var(--color-gray-600)'
      }}>
        加载中...
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ 
        display: 'flex', 
        flexDirection: 'column',
        alignItems: 'center', 
        justifyContent: 'center', 
        height: '400px',
        gap: '16px',
        color: 'var(--color-red-600)'
      }}>
        <span>❌ 加载失败: {error}</span>
        <button 
          onClick={fetchTaskStatus}
          style={{
            padding: '8px 24px',
            background: 'var(--color-blue-500)',
            color: 'white',
            border: 'none',
            borderRadius: 'var(--radius-md)',
            cursor: 'pointer'
          }}
        >
          重试
        </button>
      </div>
    );
  }

  // 添加调试日志
  console.log('[SmartLogViewer] Rendering decision - status:', taskStatus, 'type:', taskType);
  
  // 如果任务正在运行或等待中，使用WebSocket实时查看
  if (taskStatus === 'running' || taskStatus === 'pending' || taskStatus === 'waiting' || taskStatus === 'apply_pending') {
    console.log('[SmartLogViewer] Using TerraformOutputViewer (WebSocket)');
    return <TerraformOutputViewer taskId={taskId} onStageChange={onStageChange} currentTaskStage={currentTaskStage} />;
  }

  // 如果任务已完成、失败或取消，使用HTTP查看历史日志（按阶段分组）
  console.log('[SmartLogViewer] Using StageLogViewer (HTTP), viewMode:', viewMode);
  
  // 对于plan_and_apply任务，使用传入的viewMode
  if (taskType === 'plan_and_apply') {
    return <StageLogViewer taskId={taskId} taskType={viewMode} />;
  }
  
  // 对于单独的plan或apply任务，直接显示对应日志
  return <StageLogViewer taskId={taskId} taskType={taskType as 'plan' | 'apply'} />;
};

export default SmartLogViewer;
