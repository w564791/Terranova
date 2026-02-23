import React, { useState, useEffect } from 'react';
import api from '../services/api';
import PlanCompleteView from './PlanCompleteView';
import ApplyingView from './ApplyingView';
import { useTerraformOutput } from '../hooks/useTerraformOutput';
import styles from './StructuredRunOutput.module.css';

interface Task {
  id: number;
  workspace_id: string;
  task_type: string;
  status: string;
  stage?: string;
  agent_id?: number;
  agent_name?: string;
  // Apply confirmation fields
  apply_confirmed_by?: string;
  apply_confirmed_at?: string;
}

interface ResourceChange {
  id: number;
  resource_address: string;
  resource_type: string;
  resource_name: string;
  module_address: string;
  action: string;
  changes_before: Record<string, any>;
  changes_after: Record<string, any>;
  apply_status: string;
  resource_id?: string; // AWS/云资源的实际 ID（如 i-xxx, lt-xxx 等）
}

interface OutputChange {
  name: string;
  action: string;
  before: any;
  after: any;
  after_unknown: boolean;
  sensitive: boolean;
}

interface ActionInvocation {
  name: string;
  type: string;
  address: string;
  config_values?: Record<string, any>;
  config_unknown?: Record<string, any>;
  provider_name?: string;
  lifecycle_action_trigger?: {
    actions_list_index: number;
    action_trigger_event: string;
    action_trigger_block_index: number;
    triggering_resource_address: string;
  };
}

interface ActionResource {
  name: string;
  type: string;
  address: string;
  full_address?: string;
  module_address?: string;
  provider_config_key?: string;
}

interface ResourceChangesResponse {
  summary: {
    add: number;
    change: number;
    destroy: number;
  };
  resources: ResourceChange[];
  output_changes?: Record<string, {
    actions: string[];
    before: any;
    after: any;
    after_unknown: boolean;
    before_sensitive: boolean;
    after_sensitive: boolean;
  }>;
  action_invocations?: ActionInvocation[];
  actions?: ActionResource[];
}

interface Props {
  task: Task;
  workspaceId: number | string;
  workspace?: any;
  mode?: 'plan' | 'apply'; // 如果指定，只显示该阶段内容，不显示标签
}

type StageKey = 'planning' | 'applying';

interface Stage {
  key: StageKey;
  label: string;
  status: 'pending' | 'active' | 'completed' | 'error';
}

const StructuredRunOutput: React.FC<Props> = ({ task, workspaceId, workspace, mode }) => {
  const [activeStage, setActiveStage] = useState<StageKey>(mode === 'apply' ? 'applying' : 'planning');
  const [resourceChanges, setResourceChanges] = useState<ResourceChange[]>([]);
  const [outputChanges, setOutputChanges] = useState<OutputChange[]>([]);
  const [actionInvocations, setActionInvocations] = useState<ActionInvocation[]>([]);
  const [actions, setActions] = useState<ActionResource[]>([]);
  const [summary, setSummary] = useState({ add: 0, change: 0, destroy: 0 });
  const [loading, setLoading] = useState(false);
  
  // 使用 WebSocket 获取实时阶段信息（仅在任务运行时）
  const { lines: wsLines } = useTerraformOutput(task.id);
  
  // 从 WebSocket 日志中解析当前实时阶段
  const [realtimeStage, setRealtimeStage] = useState<string | null>(null);
  const [completedStages, setCompletedStages] = useState<Set<string>>(new Set());
  
  // 解析 WebSocket 日志获取实时阶段
  useEffect(() => {
    if (task.status !== 'running') {
      // 任务不在运行状态，清空实时阶段
      setRealtimeStage(null);
      setCompletedStages(new Set());
      return;
    }
    
    let latestStage: string | null = null;
    const completed = new Set<string>();
    
    for (const line of wsLines) {
      if (line.type === 'stage_marker') {
        const stage = line.stage?.toLowerCase() || '';
        if (line.status === 'begin') {
          latestStage = stage;
        } else if (line.status === 'end') {
          completed.add(stage);
        }
      }
    }
    
    if (latestStage !== realtimeStage) {
      console.log('[StructuredRunOutput] Realtime stage changed:', realtimeStage, '->', latestStage);
      setRealtimeStage(latestStage);
    }
    setCompletedStages(completed);
  }, [wsLines, task.status, realtimeStage]);

  // 根据task状态判断当前阶段（仅在未指定mode时）
  useEffect(() => {
    if (!mode) {
      const currentStage = determineCurrentStage(task);
      setActiveStage(currentStage);
    }
  }, [task.status, task.stage, mode]);

  // 加载资源变更数据
  useEffect(() => {
    if (task.status === 'success' || task.status === 'plan_completed' || task.status === 'apply_pending' || task.status === 'applied' || task.status === 'cancelled' || task.status === 'running' || task.status === 'failed') {
      // 取消/失败的任务也可能有Plan数据，running状态也需要加载（Apply阶段）
      console.log('Triggering loadResourceChanges for task:', task.id, 'status:', task.status);
      loadResourceChanges();
    }
  }, [task.id, task.status, workspaceId]); // 添加workspaceId依赖

  // Apply 完成后，从 state 获取实际的 output 值
  useEffect(() => {
    if (task.status === 'applied') {
      loadStateOutputs();
    }
  }, [task.status, workspaceId]);

  const loadStateOutputs = async () => {
    try {
      const response: any = await api.get(`/workspaces/${workspaceId}/state-outputs`);
      // 直接使用 state 中的 outputs，忽略 plan 中的 output_changes
      // 因为 state 是最终的真实状态，已删除的 outputs 不会出现在 state 中
      if (response.outputs) {
        const stateOutputs: OutputChange[] = Object.entries(response.outputs).map(([name, output]: [string, any]) => ({
          name,
          action: 'no-op', // 表示这是现有的 output
          before: output.value,
          after: output.value,
          after_unknown: false,
          sensitive: output.sensitive || false,
        }));
        setOutputChanges(stateOutputs);
        console.log(`✓ State outputs loaded: ${stateOutputs.length} outputs`);
      } else {
        setOutputChanges([]);
        console.log('✓ No outputs in state');
      }
    } catch (err) {
      console.error('Error loading state outputs:', err);
    }
  };

  // WebSocket实时更新资源状态
  useEffect(() => {
    // 只在Apply阶段监听WebSocket更新
    if (task.status !== 'running' || !task.stage || 
        (task.stage !== 'applying' && task.stage !== 'pre_apply' && task.stage !== 'restoring_plan')) {
      console.log('WebSocket: Not connecting - task not in apply stage', { status: task.status, stage: task.stage });
      return;
    }

    // 构建WebSocket URL - 使用与API相同的域名自适应逻辑
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const hostname = window.location.hostname;
    // 如果是开发环境的默认端口（5173），使用 8080 作为 API 端口
    const apiPort = window.location.port === '5173' ? '8080' : window.location.port;
    const token = localStorage.getItem('token');
    const wsUrl = `${wsProtocol}//${hostname}:${apiPort}/api/v1/tasks/${task.id}/output/stream`;

    console.log('WebSocket: Attempting to connect to:', wsUrl);
    console.log('WebSocket: Current location:', window.location.href);

    let ws: WebSocket | null = null;
    let reconnectTimer: number | null = null;
    let reconnectAttempts = 0;
    const maxReconnectAttempts = 5;

    const connect = () => {
      try {
        // 使用 Sec-WebSocket-Protocol 传递 token
        ws = new WebSocket(wsUrl, ['access_token', token || '']);

        ws.onopen = () => {
          console.log('✓ WebSocket connected successfully for real-time updates');
          reconnectAttempts = 0; // 重置重连计数
        };

        ws.onmessage = (event) => {
          try {
            const message = JSON.parse(event.data);
            
            // 处理连接确认消息
            if (message.type === 'connected') {
              console.log('✓ WebSocket connection confirmed:', message);
              return;
            }
            
            // 处理资源状态更新事件
            if (message.type === 'resource_status_update') {
              const data = JSON.parse(message.line);
              console.log('Received resource update:', data);
              
              // 更新资源状态 - 使用 resource_address 匹配（更可靠）
              setResourceChanges(prev => {
                const updated = prev.map(resource => {
                  // 优先使用 resource_address 匹配，因为这在 Agent 模式下更可靠
                  if (resource.resource_address === data.resource_address) {
                    console.log(`Updating resource ${resource.resource_address}: ${resource.apply_status} -> ${data.apply_status}`);
                    return {
                      ...resource,
                      apply_status: data.apply_status,
                      apply_started_at: data.apply_started_at,
                      apply_completed_at: data.apply_completed_at
                    };
                  }
                  // 备用：使用 resource_id 匹配（Local 模式）
                  if (data.resource_id && resource.id === data.resource_id) {
                    console.log(`Updating resource ${resource.resource_address} by ID: ${resource.apply_status} -> ${data.apply_status}`);
                    return {
                      ...resource,
                      apply_status: data.apply_status,
                      apply_started_at: data.apply_started_at,
                      apply_completed_at: data.apply_completed_at
                    };
                  }
                  return resource;
                });
                return updated;
              });
            }
            
            // 处理资源 ID 更新事件（Apply 完成后从 State 提取）
            if (message.type === 'resource_id_update') {
              const data = JSON.parse(message.line);
              console.log('Received resource ID update:', data);
              
              // 更新资源的 resource_id 字段
              setResourceChanges(prev => {
                const updated = prev.map(resource => {
                  // 使用 resource_address 匹配
                  if (resource.resource_address === data.resource_address) {
                    console.log(`Updating resource ID for ${resource.resource_address}: ${data.resource_id}`);
                    return {
                      ...resource,
                      resource_id: data.resource_id
                    };
                  }
                  // 备用：使用数据库 ID 匹配
                  if (data.id && resource.id === data.id) {
                    console.log(`Updating resource ID for ${resource.resource_address} by DB ID: ${data.resource_id}`);
                    return {
                      ...resource,
                      resource_id: data.resource_id
                    };
                  }
                  return resource;
                });
                return updated;
              });
            }
          } catch (err) {
            console.error('Error parsing WebSocket message:', err, event.data);
          }
        };

        ws.onerror = (error) => {
          console.error('✗ WebSocket error:', error);
          console.error('WebSocket URL:', wsUrl);
          console.error('Task status:', task.status, 'Stage:', task.stage);
        };

        ws.onclose = (event) => {
          console.log('WebSocket closed:', { code: event.code, reason: event.reason, wasClean: event.wasClean });
          
          // 如果任务还在运行且未达到最大重连次数，尝试重连
          if (task.status === 'running' && reconnectAttempts < maxReconnectAttempts) {
            reconnectAttempts++;
            const delay = Math.min(1000 * Math.pow(2, reconnectAttempts - 1), 10000); // 指数退避，最多10秒
            console.log(`Attempting to reconnect WebSocket (attempt ${reconnectAttempts}/${maxReconnectAttempts}) in ${delay}ms...`);
            
            reconnectTimer = setTimeout(() => {
              connect();
            }, delay);
          } else if (reconnectAttempts >= maxReconnectAttempts) {
            console.error('✗ Max reconnection attempts reached, giving up');
          }
        };
      } catch (err) {
        console.error('Error creating WebSocket:', err);
      }
    };

    connect();

    return () => {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
      }
      if (ws) {
        ws.close();
      }
    };
  }, [task.id, task.status, task.stage, workspaceId]);

  const determineCurrentStage = (task: Task): StageKey => {
    // If apply has been confirmed, we're in applying stage
    if (task.apply_confirmed_by || task.apply_confirmed_at) {
      return 'applying';
    }
    
    // Apply相关阶段：pre_apply, applying, post_apply, applied
    if (task.status === 'running') {
      if (task.stage === 'pre_apply' || task.stage === 'restoring_plan' || 
          task.stage === 'applying' || task.stage === 'post_apply' || task.stage === 'saving_state' ||
          task.stage === 'apply_pending') {
        return 'applying';
      }
    }
    if (task.status === 'applied') {
      return 'applying'; // Apply完成后，applying tab显示为"Complete"
    }
    
    // plan_completed状态保持在planning阶段，以便查看资源变更
    // 其他都是Planning阶段
    return 'planning';
  };

  const getStageStatus = (stageKey: StageKey): 'pending' | 'active' | 'completed' | 'error' => {
    const currentStage = determineCurrentStage(task);

    // 处理失败和取消状态
    if (task.status === 'failed') {
      if (currentStage === stageKey) return 'error';
      if (stageKey === 'planning') return 'error';
      return 'pending';
    }
    if (task.status === 'cancelled') {
      if (currentStage === stageKey) return 'error';
      if (stageKey === 'planning') return 'error';
      return 'pending';
    }

    // Pending状态：所有阶段都是pending
    if (task.status === 'pending') {
      return 'pending';
    }

    // Planning阶段
    if (stageKey === 'planning') {
      if (task.status === 'success' || task.status === 'plan_completed' || task.status === 'apply_pending' || task.status === 'applied') {
        return 'completed';
      }
      if (task.status === 'running' && currentStage === 'planning') {
        return 'active';
      }
      return 'pending';
    }

    // Applying阶段
    if (stageKey === 'applying') {
      if (task.status === 'applied') {
        return 'completed';
      }
      if (task.status === 'running' && currentStage === 'applying') {
        return 'active';
      }
      if (task.status === 'success' || task.status === 'plan_completed' || task.status === 'apply_pending') {
        return 'pending';
      }
      return 'pending';
    }

    return 'pending';
  };

  const loadResourceChanges = async () => {
    console.log('🔍 loadResourceChanges called');
    console.log('Task ID:', task.id);
    console.log('Workspace ID:', workspaceId);
    console.log('Task status:', task.status);
    
    try {
      setLoading(true);
      console.log('✓ Loading state set to true');
      
      const apiUrl = `/workspaces/${workspaceId}/tasks/${task.id}/resource-changes`;
      console.log('📡 Making API request to:', apiUrl);
      
      const response: ResourceChangesResponse = await api.get(apiUrl);
      
      console.log('✓ API response received');
      console.log('Response data:', response);
      console.log('Resources count:', response.resources?.length || 0);
      console.log('Summary:', response.summary);
      
      setResourceChanges(response.resources || []);
      setSummary(response.summary || { add: 0, change: 0, destroy: 0 });
      
      // 解析 output_changes - 只在非 applied 状态下设置
      // applied 状态下，outputs 由 loadStateOutputs 从 state 中获取
      if (task.status !== 'applied') {
        if (response.output_changes) {
          const outputs: OutputChange[] = Object.entries(response.output_changes).map(([name, change]) => ({
            name,
            action: change.actions?.[0] || 'unknown',
            before: change.before,
            after: change.after,
            after_unknown: change.after_unknown || false,
            sensitive: change.after_sensitive || false,
          }));
          setOutputChanges(outputs);
          console.log('✓ Output changes loaded:', outputs.length);
        } else {
          setOutputChanges([]);
        }
      } else {
        console.log('✓ Skipping output_changes for applied task (will use state outputs)');
      }
      
      // 解析 action_invocations (Terraform 1.14+ 新特性)
      if (response.action_invocations) {
        setActionInvocations(response.action_invocations);
        console.log('✓ Action invocations loaded:', response.action_invocations.length);
      } else {
        setActionInvocations([]);
      }
      
      // 解析 actions 资源定义 (Terraform 1.14+ 新特性)
      if (response.actions) {
        setActions(response.actions);
        console.log('✓ Actions loaded:', response.actions.length);
      } else {
        setActions([]);
      }
      
      console.log('✓ State updated successfully');
    } catch (err) {
      console.error('❌ Error loading resource changes');
      console.error('Error object:', err);
      console.error('Error message:', err instanceof Error ? err.message : 'Unknown error');
      console.error('Error stack:', err instanceof Error ? err.stack : 'No stack trace');
      
      // 设置空数据以显示"No Resource Changes Data"消息
      setResourceChanges([]);
      setSummary({ add: 0, change: 0, destroy: 0 });
    } finally {
      console.log('✓ Finally block executed, setting loading to false');
      setLoading(false);
    }
  };

  // 动态生成标签文字
  const getPlanningLabel = () => {
    if (task.status === 'success' || task.status === 'plan_completed' || task.status === 'apply_pending' || task.status === 'applied') {
      return 'Planned';
    }
    if (task.status === 'running' && (task.stage === 'post_plan' || task.stage === 'saving_plan')) {
      return 'Post Plan';
    }
    return 'Planning';
  };

  const getApplyingLabel = () => {
    if (task.status === 'applied') {
      return 'Applied';
    }
    if (task.status === 'failed') {
      return 'Error';
    }
    if (task.status === 'running') {
      if (task.stage === 'pre_apply' || task.stage === 'restoring_plan') {
        return 'Apply Pending';
      }
      if (task.stage === 'applying') {
        return 'Applying';
      }
      if (task.stage === 'post_apply' || task.stage === 'saving_state') {
        return 'Post Apply';
      }
    }
    if (task.status === 'plan_completed' || task.status === 'apply_pending') {
      return 'Apply Pending';
    }
    return 'Applying';
  };

  // 根据任务状态决定显示哪些Tab
  const getVisibleStages = (): Stage[] => {
    const allStages: Stage[] = [
      { key: 'planning', label: getPlanningLabel(), status: getStageStatus('planning') },
      { key: 'applying', label: getApplyingLabel(), status: getStageStatus('applying') },
    ];

    // 只显示已执行或正在执行的阶段
    const visibleStages: Stage[] = [];

    // Planning阶段：总是显示
    visibleStages.push(allStages[0]);

    // Applying阶段：只有在Plan完成后才显示
    if (task.status === 'plan_completed' || task.status === 'apply_pending' || task.status === 'applied' || 
        (task.status === 'running' && (task.stage === 'pre_apply' || task.stage === 'restoring_plan' || 
         task.stage === 'applying' || task.stage === 'post_apply' || task.stage === 'saving_state'))) {
      visibleStages.push(allStages[1]);
    }

    return visibleStages;
  };

  const stages = getVisibleStages();

  // Helper: Check if plan is complete (for rendering purposes)
  const isPlanComplete = () => {
    // Plan is complete if:
    // 1. Apply has been confirmed
    if (task.apply_confirmed_by || task.apply_confirmed_at) return true;
    // 2. Status indicates plan is done
    if (['success', 'plan_completed', 'apply_pending', 'applied', 'cancelled', 'failed'].includes(task.status)) return true;
    // 3. Task is running but in apply stage
    if (task.status === 'running') {
      const applyStages = ['apply', 'applying', 'pre_apply', 'restoring_plan', 'post_apply', 'saving_state', 'apply_pending'];
      if (task.stage && applyStages.includes(task.stage)) return true;
    }
    return false;
  };

  // 如果指定了 mode，直接渲染对应内容，不显示标签
  const renderContent = () => {
    const currentMode = mode || (activeStage === 'planning' ? 'plan' : 'apply');
    
    if (currentMode === 'plan') {
      return (
        <>
          {isPlanComplete() ? (
            loading ? (
              <div className={styles.loading}>
                <div className={styles.loadingSpinner}></div>
                <p>加载资源变更数据...</p>
              </div>
            ) : (
              <PlanCompleteView resources={resourceChanges} summary={summary} outputChanges={outputChanges} actionInvocations={actionInvocations} actions={actions} />
            )
          ) : task.status === 'pending' ? (
            <div className={styles.stageInfo}>
              <p>Task is pending. Waiting for previous tasks to complete...</p>
            </div>
          ) : task.status === 'running' ? (
            <div className={styles.runningState}>
              <div className={styles.runningSpinner}></div>
              <h3 className={styles.runningTitle}>
                {/* 使用实时阶段（来自 WebSocket）来显示标题，如果没有则使用 task.stage */}
                {(() => {
                  const stage = realtimeStage || task.stage;
                  if (stage === 'fetching') return 'Fetching Configuration';
                  if (stage === 'init') return 'Initializing Terraform';
                  if (stage === 'plan' || stage === 'planning') return 'Executing Terraform Plan';
                  if (stage === 'post_plan') return 'Post-Plan Processing';
                  if (stage === 'saving_plan') return 'Saving Plan Data';
                  if (stage === 'pending') return 'Waiting to Start';
                  return 'Plan Execution in Progress';
                })()}
              </h3>
              <div className={styles.runningSteps}>
                {/* 使用实时阶段（来自 WebSocket）来判断步骤状态 */}
                {(() => {
                  // 优先使用 WebSocket 实时阶段，否则使用 task.stage
                  const currentStage = realtimeStage || task.stage || 'fetching';
                  
                  // 判断步骤是否完成（使用 WebSocket 的 completedStages）
                  const isStageCompleted = (stageName: string) => completedStages.has(stageName);
                  
                  // 判断步骤是否正在执行
                  const isStageActive = (stageName: string) => currentStage === stageName;
                  
                  // 判断步骤是否已经过去（基于阶段顺序）
                  // 只有当 completedStages 有数据时才使用 isPastStage，否则只依赖 completedStages
                  const stageOrder = ['fetching', 'init', 'planning', 'post_plan', 'saving_plan'];
                  const currentIndex = stageOrder.indexOf(currentStage === 'plan' ? 'planning' : currentStage);
                  const isPastStage = (stageName: string) => {
                    // 只有当我们有 WebSocket 数据时才使用顺序判断
                    // 否则只依赖 completedStages
                    if (completedStages.size === 0 && !realtimeStage) {
                      return false;
                    }
                    const stageIndex = stageOrder.indexOf(stageName);
                    return stageIndex >= 0 && stageIndex < currentIndex;
                  };
                  
                  // 获取步骤状态
                  const getStepStatus = (stageName: string): 'active' | 'completed' | 'pending' => {
                    if (isStageActive(stageName) || (stageName === 'planning' && currentStage === 'plan')) {
                      return 'active';
                    }
                    // 优先使用 completedStages（来自 WebSocket）
                    if (isStageCompleted(stageName)) {
                      return 'completed';
                    }
                    // 只有当有 WebSocket 数据时才使用 isPastStage
                    if (isPastStage(stageName)) {
                      return 'completed';
                    }
                    return 'pending';
                  };
                  
                  const fetchingStatus = getStepStatus('fetching');
                  const initStatus = getStepStatus('init');
                  const planningStatus = getStepStatus('planning');
                  const postPlanStatus = getStepStatus('post_plan');
                  const savingPlanStatus = getStepStatus('saving_plan');
                  
                  return (
                    <>
                      {/* Step 1: Fetching */}
                      <div className={`${styles.step} ${
                        fetchingStatus === 'active' ? styles.stepActive : 
                        fetchingStatus === 'completed' ? styles.stepCompleted : ''
                      }`}>
                        <span className={styles.stepIcon}>
                          {fetchingStatus === 'active' ? '⟳' : fetchingStatus === 'completed' ? '✓' : '○'}
                        </span>
                        <span className={styles.stepText}>Fetching - Get configuration from database</span>
                      </div>
                      {/* Step 2: Init */}
                      <div className={`${styles.step} ${
                        initStatus === 'active' ? styles.stepActive : 
                        initStatus === 'completed' ? styles.stepCompleted : ''
                      }`}>
                        <span className={styles.stepIcon}>
                          {initStatus === 'active' ? '⟳' : initStatus === 'completed' ? '✓' : '○'}
                        </span>
                        <span className={styles.stepText}>Init - Initialize Terraform and providers</span>
                      </div>
                      {/* Step 3: Planning */}
                      <div className={`${styles.step} ${
                        planningStatus === 'active' ? styles.stepActive : 
                        planningStatus === 'completed' ? styles.stepCompleted : ''
                      }`}>
                        <span className={styles.stepIcon}>
                          {planningStatus === 'active' ? '⟳' : planningStatus === 'completed' ? '✓' : '○'}
                        </span>
                        <span className={styles.stepText}>Planning - Execute terraform plan</span>
                      </div>
                      {/* Step 4: Post Plan */}
                      <div className={`${styles.step} ${
                        postPlanStatus === 'active' ? styles.stepActive : 
                        postPlanStatus === 'completed' ? styles.stepCompleted : ''
                      }`}>
                        <span className={styles.stepIcon}>
                          {postPlanStatus === 'active' ? '⟳' : postPlanStatus === 'completed' ? '✓' : '○'}
                        </span>
                        <span className={styles.stepText}>Post Plan - Parse and analyze plan output</span>
                      </div>
                      {/* Step 5: Saving Plan */}
                      <div className={`${styles.step} ${
                        savingPlanStatus === 'active' ? styles.stepActive : 
                        savingPlanStatus === 'completed' ? styles.stepCompleted : ''
                      }`}>
                        <span className={styles.stepIcon}>
                          {savingPlanStatus === 'active' ? '⟳' : savingPlanStatus === 'completed' ? '✓' : '○'}
                        </span>
                        <span className={styles.stepText}>Saving Plan - Store plan data to database</span>
                      </div>
                    </>
                  );
                })()}
              </div>
            </div>
          ) : (
            <div className={styles.stageInfo}>
              <p>Task status: {task.status}</p>
            </div>
          )}
        </>
      );
    } else {
      // Apply mode
      return (
        <>
          {task.status === 'applied' ? (
            loading ? (
              <div className={styles.loading}>
                <div className={styles.loadingSpinner}></div>
                <p>加载资源详情...</p>
              </div>
            ) : (
              <ApplyingView resources={resourceChanges} summary={summary} outputChanges={outputChanges} actionInvocations={actionInvocations} actions={actions} isApplied={true} />
            )
          ) : task.status === 'failed' ? (
            loading ? (
              <div className={styles.loading}>
                <div className={styles.loadingSpinner}></div>
                <p>加载资源数据...</p>
              </div>
            ) : (
              <ApplyingView resources={resourceChanges} summary={summary} outputChanges={outputChanges} actionInvocations={actionInvocations} actions={actions} isApplied={true} />
            )
          ) : task.status === 'running' && (task.stage === 'applying' || task.stage === 'pre_apply' || task.stage === 'restoring_plan') ? (
            loading ? (
              <div className={styles.loading}>
                <div className={styles.loadingSpinner}></div>
                <p>加载资源数据...</p>
              </div>
            ) : (
              <ApplyingView resources={resourceChanges} summary={summary} actionInvocations={actionInvocations} actions={actions} />
            )
          ) : (task.status === 'plan_completed' || task.status === 'apply_pending') ? (
            loading ? (
              <div className={styles.loading}>
                <div className={styles.loadingSpinner}></div>
                <p>加载资源数据...</p>
              </div>
            ) : (
              <>
                <div className={styles.stageInfo}>
                  <p>Waiting for apply confirmation...</p>
                </div>
                <ApplyingView resources={resourceChanges} summary={summary} actionInvocations={actionInvocations} actions={actions} />
              </>
            )
          ) : (
            <div className={styles.stageInfo}>
              <p>
                {task.status === 'running' && 'Apply阶段准备中...'}
                {task.status === 'pending' && 'Task is pending...'}
              </p>
            </div>
          )}
        </>
      );
    }
  };

  return (
    <div className={styles.structuredOutput}>
      {/* Resource Changes Summary Bars - 只在 Plan 模式下显示，Apply 模式不显示 */}
      {mode !== 'apply' && (summary.add > 0 || summary.change > 0 || summary.destroy > 0) && (() => {
        const total = summary.add + summary.change + summary.destroy;
        return (
          <div className={styles.resourceSummaryContainer}>
            {summary.add > 0 && (
              <div 
                className={styles.resourceBarAdd}
                style={{ flex: `${summary.add / total} 1 0%` }}
              >
                <span className={styles.changeIcon}>+</span>
                <span className={styles.changeCount}>{summary.add}</span>
                <span className={styles.changeText}>to create</span>
              </div>
            )}
            {summary.change > 0 && (
              <div 
                className={styles.resourceBarModify}
                style={{ flex: `${summary.change / total} 1 0%` }}
              >
                <span className={styles.changeIcon}>~</span>
                <span className={styles.changeCount}>{summary.change}</span>
                <span className={styles.changeText}>to change</span>
              </div>
            )}
            {summary.destroy > 0 && (
              <div 
                className={styles.resourceBarDestroy}
                style={{ flex: `${summary.destroy / total} 1 0%` }}
              >
                <span className={styles.changeIcon}>-</span>
                <span className={styles.changeCount}>{summary.destroy}</span>
                <span className={styles.changeText}>to destroy</span>
              </div>
            )}
          </div>
        );
      })()}

      {/* 阶段Tab - 仅在未指定 mode 时显示 */}
      {!mode && (
        <div className={styles.stageTabs}>
          {stages.map((stage) => (
            <button
              key={stage.key}
              className={`${styles.stageTab} ${
                activeStage === stage.key ? styles.stageTabActive : ''
              } ${styles[`stageTab${stage.status.charAt(0).toUpperCase() + stage.status.slice(1)}`]}`}
              onClick={() => setActiveStage(stage.key)}
            >
              <span className={styles.stageIcon}>
                {stage.status === 'completed' && '✓'}
                {stage.status === 'active' && <span className={styles.spinner}></span>}
                {stage.status === 'pending' && '○'}
                {stage.status === 'error' && '✗'}
              </span>
              <span className={styles.stageLabel}>{stage.label}</span>
            </button>
          ))}
        </div>
      )}

      {/* 阶段内容 */}
      <div className={styles.stageContent}>
        {renderContent()}
      </div>
    </div>
  );
};

export default StructuredRunOutput;
