import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { agentPoolAPI, poolAuthorizationAPI, poolTokenAPI, type AgentPool, type Agent, type PoolAllowedWorkspace, type PoolToken, type PoolTokenCreateResponse } from '../../services/agent';
import { workspaceService, type Workspace } from '../../services/workspaces';
import { useToast } from '../../contexts/ToastContext';
import api from '../../services/api';
import SecretsManager from '../../components/SecretsManager';
import AgentMetricsBar from '../../components/AgentMetricsBar';
import styles from './AgentPoolDetail.module.css';

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

const AgentPoolDetail: React.FC = () => {
  const { poolId } = useParams<{ poolId: string }>();
  const [pool, setPool] = useState<AgentPool | null>(null);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [allowedWorkspaces, setAllowedWorkspaces] = useState<PoolAllowedWorkspace[]>([]);
  const [allWorkspaces, setAllWorkspaces] = useState<Workspace[]>([]);
  const [showAddWorkspaceDialog, setShowAddWorkspaceDialog] = useState(false);
  const [selectedWorkspaceIds, setSelectedWorkspaceIds] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [workspacesLoading, setWorkspacesLoading] = useState(false);
  const [showOfflineAgents, setShowOfflineAgents] = useState(false);
  
  // Pool Token states
  const [poolTokens, setPoolTokens] = useState<PoolToken[]>([]);
  const [showCreateTokenDialog, setShowCreateTokenDialog] = useState(false);
  const [newTokenName, setNewTokenName] = useState('');
  const [createdToken, setCreatedToken] = useState<PoolTokenCreateResponse | null>(null);
  const [revokeTokenConfirm, setRevokeTokenConfirm] = useState<{ show: boolean; tokenName: string } | null>(null);
  const [rotateTokenConfirm, setRotateTokenConfirm] = useState<{ show: boolean; tokenName: string } | null>(null);
  const [revokeWorkspaceConfirm, setRevokeWorkspaceConfirm] = useState<{ show: boolean; workspaceId: string } | null>(null);
  const [unfreezeConfirm, setUnfreezeConfirm] = useState(false);
  
  // K8s Configuration states
  const [showK8sConfig, setShowK8sConfig] = useState(false);
  const [showYamlPreview, setShowYamlPreview] = useState(false);
  const [k8sConfig, setK8sConfig] = useState({
    image: '',
    image_pull_policy: 'Always',
    namespace: 'terraform',
    service_account: '',
    command: [] as string[],
    args: [] as string[],
    min_replicas: 1,
    max_replicas: 10,
    cpu_limit: '500m',
    memory_limit: '512Mi',
    env: {} as Record<string, string>,
  });
  const [envPairs, setEnvPairs] = useState<Array<{ key: string; value: string }>>([
    { key: '', value: '' }
  ]);
  
  // Freeze Schedule states
  const [showFreezeSchedule, setShowFreezeSchedule] = useState(false);
  const [freezeSchedules, setFreezeSchedules] = useState<Array<{
    from_time: string;
    to_time: string;
    weekdays: number[];
  }>>([]);
  const [newSchedule, setNewSchedule] = useState({
    from_time: '02:00',
    to_time: '06:00',
    weekdays: [] as number[],
  });
  const [editingScheduleIndex, setEditingScheduleIndex] = useState<number | null>(null);
  
  // Agent Metrics states
  const [agentMetrics, setAgentMetrics] = useState<Map<string, AgentMetrics>>(new Map());
  const [metricsWs, setMetricsWs] = useState<WebSocket | null>(null);
  
  const { showToast } = useToast();
  const navigate = useNavigate();

  useEffect(() => {
    if (poolId) {
      loadPoolDetail();
      loadAllowedWorkspaces();
      loadPoolTokens();
    }
  }, [poolId, showOfflineAgents]);

  // Load K8s config only after pool is loaded and only for k8s pools
  useEffect(() => {
    if (pool && pool.pool_type === 'k8s') {
      loadK8sConfig();
    }
  }, [pool?.pool_type]);

  // WebSocket connection for agent metrics
  useEffect(() => {
    if (!poolId) {
      console.log('  [AgentMetrics] No poolId, skipping WebSocket connection');
      return;
    }

    // Get JWT token from localStorage
    const token = localStorage.getItem('token');
    if (!token) {
      console.error('❌ [AgentMetrics] No JWT token found, cannot establish WebSocket connection');
      return;
    }

    console.log(`🔌 [AgentMetrics] Initializing WebSocket connection for pool: ${poolId}`);
    
    // Construct WebSocket URL with token as query parameter
    // Use the same logic as api.ts to determine the correct host
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const hostname = window.location.hostname;
    const apiPort = window.location.port === '5173' ? '8080' : window.location.port;
    const wsUrl = `${protocol}//${hostname}:${apiPort}/api/v1/ws/agent-pools/${poolId}/metrics?token=${encodeURIComponent(token)}`;
    
    console.log(`🔌 [AgentMetrics] WebSocket URL: ${wsUrl.replace(/token=[^&]+/, 'token=***')}`); // Hide token in logs
    
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      console.log(` [AgentMetrics] WebSocket connected successfully for pool: ${poolId}`);
      console.log(` [AgentMetrics] WebSocket readyState: ${ws.readyState} (OPEN)`);
    };

    ws.onmessage = (event) => {
      console.log(`📨 [AgentMetrics] Received WebSocket message:`, event.data);
      
      try {
        const message = JSON.parse(event.data);
        console.log(`📨 [AgentMetrics] Parsed message type: ${message.type}`, message);
        
        if (message.type === 'initial_metrics') {
          console.log(`📊 [AgentMetrics] Processing initial_metrics, count: ${message.metrics?.length || 0}`);
          
          if (!message.metrics || !Array.isArray(message.metrics)) {
            console.warn('  [AgentMetrics] Invalid initial_metrics format:', message);
            return;
          }
          
          const metricsMap = new Map<string, AgentMetrics>();
          message.metrics.forEach((m: AgentMetrics) => {
            console.log(`📊 [AgentMetrics] Adding initial metric for agent: ${m.agent_id}, CPU: ${m.cpu_usage}%, Memory: ${m.memory_usage}%`);
            metricsMap.set(m.agent_id, m);
          });
          
          setAgentMetrics(metricsMap);
          console.log(` [AgentMetrics] Initial metrics loaded: ${metricsMap.size} agents`);
          
        } else if (message.type === 'metrics_update') {
          console.log(`📊 [AgentMetrics] Processing metrics_update for agent: ${message.metrics?.agent_id}`);
          
          if (!message.metrics || !message.metrics.agent_id) {
            console.warn('  [AgentMetrics] Invalid metrics_update format:', message);
            return;
          }
          
          console.log(`📊 [AgentMetrics] Update details - Agent: ${message.metrics.agent_id}, CPU: ${message.metrics.cpu_usage}%, Memory: ${message.metrics.memory_usage}%, Tasks: ${message.metrics.running_tasks?.length || 0}`);
          
          setAgentMetrics(prev => {
            const newMap = new Map(prev);
            newMap.set(message.metrics.agent_id, message.metrics);
            console.log(` [AgentMetrics] Updated metrics map, total agents: ${newMap.size}`);
            return newMap;
          });
          
        } else if (message.type === 'agent_offline') {
          console.log(`📊 [AgentMetrics] Processing agent_offline for agent: ${message.metrics?.agent_id}`);
          
          if (!message.metrics || !message.metrics.agent_id) {
            console.warn('  [AgentMetrics] Invalid agent_offline format:', message);
            return;
          }
          
          setAgentMetrics(prev => {
            const newMap = new Map(prev);
            const deleted = newMap.delete(message.metrics.agent_id);
            console.log(`${deleted ? '' : ' '} [AgentMetrics] Agent ${message.metrics.agent_id} ${deleted ? 'removed' : 'not found'}, remaining agents: ${newMap.size}`);
            return newMap;
          });
          
        } else {
          console.warn(`  [AgentMetrics] Unknown message type: ${message.type}`, message);
        }
      } catch (error) {
        console.error('❌ [AgentMetrics] Failed to parse WebSocket message:', error);
        console.error('❌ [AgentMetrics] Raw message data:', event.data);
      }
    };

    ws.onerror = (error) => {
      console.error('❌ [AgentMetrics] WebSocket error occurred:', error);
      console.error('❌ [AgentMetrics] WebSocket readyState:', ws.readyState);
      console.error('❌ [AgentMetrics] WebSocket URL:', wsUrl);
    };

    ws.onclose = (event) => {
      console.log(`❌ [AgentMetrics] WebSocket disconnected for pool: ${poolId}`);
      console.log(`❌ [AgentMetrics] Close code: ${event.code}, reason: ${event.reason || 'No reason provided'}`);
      console.log(`❌ [AgentMetrics] Was clean close: ${event.wasClean}`);
    };

    setMetricsWs(ws);
    console.log(`🔌 [AgentMetrics] WebSocket instance created and stored`);

    return () => {
      console.log(`🔌 [AgentMetrics] Cleanup: Closing WebSocket for pool: ${poolId}`);
      if (ws.readyState === WebSocket.OPEN) {
        console.log(`🔌 [AgentMetrics] WebSocket is OPEN, closing...`);
        ws.close();
      } else {
        console.log(`🔌 [AgentMetrics] WebSocket already closed or closing, readyState: ${ws.readyState}`);
      }
    };
  }, [poolId]);

  const loadPoolDetail = async () => {
    if (!poolId) return;
    
    try {
      setLoading(true);
      const data = await agentPoolAPI.get(poolId, showOfflineAgents);
      setPool(data.pool);
      setAgents(data.agents || []);
    } catch (error: any) {
      console.error('Failed to load pool detail:', error);
      showToast(error.response?.data?.error || 'Failed to load pool details', 'error');
    } finally {
      setLoading(false);
    }
  };

  const loadAllowedWorkspaces = async () => {
    if (!poolId) return;
    
    try {
      const data = await poolAuthorizationAPI.getAllowedWorkspaces(poolId, { status: 'active' });
      setAllowedWorkspaces(data.workspaces || []);
    } catch (error: any) {
      console.error('Failed to load allowed workspaces:', error);
      // Don't show error toast for this, as it's not critical
    }
  };

  const loadPoolTokens = async () => {
    if (!poolId) return;
    
    try {
      const data = await poolTokenAPI.list(poolId);
      setPoolTokens(data.tokens || []);
    } catch (error: any) {
      console.error('Failed to load pool tokens:', error);
    }
  };

  const loadK8sConfig = async () => {
    if (!poolId) return;
    
    try {
      const response = await api.get(`/agent-pools/${poolId}/k8s-config`);
      const config = response.data || response;
      const envVars = config.env || {};
      const pairs = Object.keys(envVars).length > 0 
        ? Object.entries(envVars).map(([key, value]) => ({ key, value: String(value) }))
        : [{ key: '', value: '' }];
      
      // Extract CPU and memory limits from resources.limits structure
      const cpuLimit = config.resources?.limits?.cpu || config.cpu_limit || '500m';
      const memoryLimit = config.resources?.limits?.memory || config.memory_limit || '512Mi';
      
      setK8sConfig({
        image: config.image || '',
        image_pull_policy: config.image_pull_policy || 'Always',
        namespace: config.namespace || 'terraform',
        service_account: config.service_account || '',
        command: config.command || [],
        args: config.args || [],
        min_replicas: config.min_replicas || 1,
        max_replicas: config.max_replicas || 10,
        cpu_limit: cpuLimit,
        memory_limit: memoryLimit,
        env: envVars,
      });
      setEnvPairs(pairs);
      
      // Load freeze schedules
      setFreezeSchedules(config.freeze_schedules || []);
    } catch (error: any) {
      // 404 is expected if no config is set yet
      if (error.response?.status !== 404) {
        console.error('Failed to load K8s config:', error);
      }
    }
  };

  const handleSaveK8sConfig = async () => {
    if (!poolId) return;

    // Convert envPairs to env object
    const env: Record<string, string> = {};
    envPairs.forEach(pair => {
      if (pair.key.trim()) {
        env[pair.key.trim()] = pair.value.trim();
      }
    });

    // Build proper K8sJobTemplateConfig structure
    const configToSave = {
      image: k8sConfig.image,
      image_pull_policy: k8sConfig.image_pull_policy,
      command: k8sConfig.command,
      args: k8sConfig.args,
      env: env,
      resources: {
        limits: {
          cpu: k8sConfig.cpu_limit,
          memory: k8sConfig.memory_limit
        }
      },
      min_replicas: k8sConfig.min_replicas,
      max_replicas: k8sConfig.max_replicas,
      freeze_schedules: freezeSchedules
    };

    try {
      await api.put(`/agent-pools/${poolId}/k8s-config`, { 
        k8s_config: configToSave
      });
      
      showToast('K8s configuration saved successfully', 'success');
      setShowK8sConfig(false);
      
      // Reload to ensure consistency
      await loadK8sConfig();

      // Automatically sync Pods after saving config
      await handleSyncDeployment();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to save K8s configuration', 'error');
    }
  };

  const handleSyncDeployment = async () => {
    if (!poolId) return;

    try {
      await api.post(`/agent-pools/${poolId}/sync-deployment`);
      showToast('Pods synced successfully', 'success');
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to sync Pods', 'error');
    }
  };

  const handleAddFreezeSchedule = async () => {
    if (!poolId) return;
    
    // Validate inputs
    if (!newSchedule.from_time || !newSchedule.to_time) {
      showToast('请选择开始和结束时间', 'error');
      return;
    }
    
    if (newSchedule.weekdays.length === 0) {
      showToast('请至少选择一个生效日期', 'error');
      return;
    }

    let updatedSchedules;
    if (editingScheduleIndex !== null) {
      // 编辑模式
      updatedSchedules = freezeSchedules.map((schedule, i) => 
        i === editingScheduleIndex ? newSchedule : schedule
      );
    } else {
      // 添加模式
      updatedSchedules = [...freezeSchedules, newSchedule];
    }
    
    // Convert envPairs to env object
    const env: Record<string, string> = {};
    envPairs.forEach(pair => {
      if (pair.key.trim()) {
        env[pair.key.trim()] = pair.value.trim();
      }
    });

    // Build proper K8sJobTemplateConfig structure
    const configToSave = {
      image: k8sConfig.image,
      image_pull_policy: k8sConfig.image_pull_policy,
      command: k8sConfig.command,
      args: k8sConfig.args,
      env: env,
      resources: {
        limits: {
          cpu: k8sConfig.cpu_limit,
          memory: k8sConfig.memory_limit
        }
      },
      min_replicas: k8sConfig.min_replicas,
      max_replicas: k8sConfig.max_replicas,
      freeze_schedules: updatedSchedules
    };

    try {
      await api.put(`/agent-pools/${poolId}/k8s-config`, { 
        k8s_config: configToSave
      });
      
      setFreezeSchedules(updatedSchedules);
      setNewSchedule({ from_time: '', to_time: '', weekdays: [] });
      setEditingScheduleIndex(null);
      setShowFreezeSchedule(false);
      showToast(editingScheduleIndex !== null ? '冻结规则已更新' : '冻结规则已添加', 'success');
    } catch (error: any) {
      showToast(error.response?.data?.error || '保存冻结规则失败', 'error');
    }
  };

  const handleEditSchedule = (index: number) => {
    const schedule = freezeSchedules[index];
    setNewSchedule({
      from_time: schedule.from_time,
      to_time: schedule.to_time,
      weekdays: [...schedule.weekdays]
    });
    setEditingScheduleIndex(index);
    setShowFreezeSchedule(true);
  };

  const handleCancelEdit = () => {
    setNewSchedule({ from_time: '', to_time: '', weekdays: [] });
    setEditingScheduleIndex(null);
    setShowFreezeSchedule(false);
  };

  const handleRemoveFreezeSchedule = async (index: number) => {
    if (!poolId) return;

    const updatedSchedules = freezeSchedules.filter((_, i) => i !== index);
    
    // Convert envPairs to env object
    const env: Record<string, string> = {};
    envPairs.forEach(pair => {
      if (pair.key.trim()) {
        env[pair.key.trim()] = pair.value.trim();
      }
    });

    // Build proper K8sJobTemplateConfig structure
    const configToSave = {
      image: k8sConfig.image,
      image_pull_policy: k8sConfig.image_pull_policy,
      command: k8sConfig.command,
      args: k8sConfig.args,
      env: env,
      resources: {
        limits: {
          cpu: k8sConfig.cpu_limit,
          memory: k8sConfig.memory_limit
        }
      },
      min_replicas: k8sConfig.min_replicas,
      max_replicas: k8sConfig.max_replicas,
      freeze_schedules: updatedSchedules
    };

    try {
      await api.put(`/agent-pools/${poolId}/k8s-config`, { 
        k8s_config: configToSave
      });
      
      setFreezeSchedules(updatedSchedules);
      showToast('Freeze schedule removed successfully', 'success');
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to remove freeze schedule', 'error');
    }
  };

  const toggleWeekday = (day: number) => {
    setNewSchedule(prev => ({
      ...prev,
      weekdays: prev.weekdays.includes(day)
        ? prev.weekdays.filter(d => d !== day)
        : [...prev.weekdays, day].sort()
    }));
  };

  const getWeekdayName = (day: number): string => {
    const days = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
    return days[day - 1] || '';
  };

  const handleEnvChange = (index: number, field: 'key' | 'value', value: string) => {
    const newEnvPairs = [...envPairs];
    newEnvPairs[index][field] = value;
    setEnvPairs(newEnvPairs);
  };

  const addEnvPair = () => {
    setEnvPairs([...envPairs, { key: '', value: '' }]);
  };

  const removeEnvPair = (index: number) => {
    if (envPairs.length > 1) {
      setEnvPairs(envPairs.filter((_, i) => i !== index));
    }
  };

  const generatePodYaml = () => {
    if (!pool) return '';
    
    const saLine = k8sConfig.service_account ? `  serviceAccountName: ${k8sConfig.service_account}\n` : '';
    
    // Use current envPairs (not saved k8sConfig.env) for preview
    const customEnvVars = envPairs
      .filter(pair => pair.key.trim())
      .map(pair => `    - name: ${pair.key.trim()}\n      value: "${pair.value.trim()}"`)
      .join('\n');
    
    const envSection = customEnvVars ? `\n${customEnvVars}` : '';
    
    const timestamp = Date.now();
    
    return `apiVersion: v1
kind: Pod
metadata:
  name: iac-agent-${pool.pool_id}-${timestamp}
  namespace: ${k8sConfig.namespace}
  labels:
    app: iac-platform
    component: agent
    pool-id: ${pool.pool_id}
    pool-name: ${pool.name}
spec:
${saLine}  restartPolicy: Never  # One-time execution pod
  containers:
  - name: agent
    image: ${k8sConfig.image || 'terraform:latest'}
    imagePullPolicy: ${k8sConfig.image_pull_policy || 'Always'}
    env:
    - name: POOL_ID
      value: "${pool.pool_id}"
    - name: POOL_NAME
      value: "${pool.name}"
    - name: POOL_TYPE
      value: "k8s"
    - name: IAC_AGENT_NAME
      value: "iac-agent-${pool.pool_id}-${timestamp}"
    - name: IAC_AGENT_TOKEN
      valueFrom:
        secretKeyRef:
          name: iac-agent-token-${pool.pool_id}
          key: token${envSection}
    resources:
      limits:
        memory: "${k8sConfig.memory_limit}"
        cpu: "${k8sConfig.cpu_limit}"`;
  };

  const handleCopyYaml = () => {
    const yaml = generatePodYaml();
    navigator.clipboard.writeText(yaml);
    showToast('YAML copied to clipboard', 'success');
  };

  const loadAllWorkspaces = async () => {
    try {
      setWorkspacesLoading(true);
      const response = await workspaceService.getWorkspaces();
      // Handle response structure: {code, data: {items: []}} or {data: []}
      const data: any = response.data;
      const workspaces = data?.items || data || [];
      setAllWorkspaces(Array.isArray(workspaces) ? workspaces : []);
    } catch (error: any) {
      console.error('Failed to load workspaces:', error);
      showToast('Failed to load workspaces', 'error');
    } finally {
      setWorkspacesLoading(false);
    }
  };

  const handleAddWorkspaces = async () => {
    if (!poolId || selectedWorkspaceIds.length === 0) return;

    try {
      await poolAuthorizationAPI.allowWorkspaces(poolId, selectedWorkspaceIds);
      showToast(`Added ${selectedWorkspaceIds.length} workspace(s) successfully`, 'success');
      setShowAddWorkspaceDialog(false);
      setSelectedWorkspaceIds([]);
      loadAllowedWorkspaces();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to add workspaces', 'error');
    }
  };

  const handleRevokeWorkspace = async () => {
    if (!poolId || !revokeWorkspaceConfirm) return;

    try {
      await poolAuthorizationAPI.revokeWorkspace(poolId, revokeWorkspaceConfirm.workspaceId);
      showToast('Workspace access revoked successfully', 'success');
      setRevokeWorkspaceConfirm(null);
      loadAllowedWorkspaces();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to revoke workspace access', 'error');
    }
  };

  const handleOpenAddDialog = () => {
    loadAllWorkspaces();
    setShowAddWorkspaceDialog(true);
  };

  const toggleWorkspaceSelection = (workspaceId: string) => {
    setSelectedWorkspaceIds(prev =>
      prev.includes(workspaceId)
        ? prev.filter(id => id !== workspaceId)
        : [...prev, workspaceId]
    );
  };

  const handleCreateToken = async () => {
    if (!poolId || !newTokenName.trim()) {
      showToast('Token name is required', 'error');
      return;
    }

    try {
      const tokenData = await poolTokenAPI.create(poolId, { token_name: newTokenName });
      setCreatedToken(tokenData);
      setNewTokenName('');
      loadPoolTokens();
      showToast('Token created successfully', 'success');
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to create token', 'error');
    }
  };

  const handleRevokeToken = async () => {
    if (!poolId || !revokeTokenConfirm) return;

    try {
      await poolTokenAPI.revoke(poolId, revokeTokenConfirm.tokenName);
      showToast('Token revoked successfully', 'success');
      setRevokeTokenConfirm(null);
      loadPoolTokens();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to revoke token', 'error');
    }
  };

  const handleRotateToken = async () => {
    if (!poolId || !rotateTokenConfirm) return;

    try {
      await api.post(`/agent-pools/${poolId}/tokens/${rotateTokenConfirm.tokenName}/rotate`);
      showToast('Token rotated successfully. Deployment will be restarted.', 'success');
      setRotateTokenConfirm(null);
      loadPoolTokens();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to rotate token', 'error');
    }
  };

  const handleCopyToken = (token: string) => {
    navigator.clipboard.writeText(token);
    showToast('Token copied to clipboard', 'success');
  };

  const handleCloseTokenDialog = () => {
    setShowCreateTokenDialog(false);
    setCreatedToken(null);
    setNewTokenName('');
  };

  const handleOneTimeUnfreeze = async () => {
    if (!poolId) return;

    try {
      const result = await poolTokenAPI.activateOneTimeUnfreeze(poolId);
      showToast(`应急解冻已激活,有效期至 ${new Date(result.unfreeze_until).toLocaleString()}`, 'success');
      setUnfreezeConfirm(false);
      // 重新加载pool数据以显示解冻状态
      loadPoolDetail();
    } catch (error: any) {
      showToast(error.response?.data?.error || '激活应急解冻失败', 'error');
    }
  };

  const handleDelete = async () => {
    if (!pool || !poolId) return;

    if (agents.length > 0) {
      showToast('Cannot delete pool with assigned agents', 'error');
      return;
    }

    if (!confirm(`Are you sure you want to delete pool "${pool.name}"?`)) {
      return;
    }

    try {
      await agentPoolAPI.delete(poolId);
      showToast('Agent pool deleted successfully', 'success');
      navigate('/global/settings/agent-pools');
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to delete pool', 'error');
    }
  };

  const getStatusBadge = (status: string) => {
    const statusMap: Record<string, { label: string; className: string }> = {
      active: { label: 'Active', className: styles.statusActive },
      inactive: { label: 'Inactive', className: styles.statusInactive },
      maintenance: { label: 'Maintenance', className: styles.statusMaintenance },
      idle: { label: 'Idle', className: styles.statusIdle },
      busy: { label: 'Busy', className: styles.statusBusy },
      offline: { label: 'Offline', className: styles.statusOffline },
    };
    const config = statusMap[status] || { label: status || 'N/A', className: '' };
    return <span className={`${styles.statusBadge} ${config.className}`}>{config.label}</span>;
  };

  if (loading) {
    return <div className={styles.loading}>Loading pool details...</div>;
  }

  if (!pool) {
    return <div className={styles.error}>Pool not found</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div>
          <button className={styles.backButton} onClick={() => navigate('/global/settings/agent-pools')}>
            ← Back to Pools
          </button>
          <h1>{pool.name}</h1>
        </div>
        <div className={styles.actions}>
          <button 
            className={styles.editButton}
            onClick={() => navigate(`/global/settings/agent-pools/${poolId}/edit`)}
          >
            Edit Pool
          </button>
          <button 
            className={styles.deleteButton}
            onClick={handleDelete}
            disabled={agents.length > 0}
            title={agents.length > 0 ? 'Cannot delete pool with agents' : 'Delete pool'}
          >
            Delete Pool
          </button>
        </div>
      </div>

      <div className={styles.content}>
        {/* Pool Information */}
        <div className={styles.section}>
          <h2>Pool Information</h2>
          <div className={styles.infoGrid}>
            <div className={styles.infoItem}>
              <label>Pool ID</label>
              <div className={styles.value}>{pool.pool_id}</div>
            </div>
            <div className={styles.infoItem}>
              <label>Name</label>
              <div className={styles.value}>{pool.name}</div>
            </div>
            <div className={styles.infoItem}>
              <label>Description</label>
              <div className={styles.value}>{pool.description || '-'}</div>
            </div>
            <div className={styles.infoItem}>
              <label>Type</label>
              <div className={styles.value}>{pool.pool_type}</div>
            </div>
            <div className={styles.infoItem}>
              <label>Status</label>
              <div className={styles.value}>{getStatusBadge(pool.status)}</div>
            </div>
            <div className={styles.infoItem}>
              <label>Max Agents</label>
              <div className={styles.value}>{pool.max_agents}</div>
            </div>
            <div className={styles.infoItem}>
              <label>Is Shared</label>
              <div className={styles.value}>{pool.is_shared ? 'Yes' : 'No'}</div>
            </div>
            <div className={styles.infoItem}>
              <label>Created</label>
              <div className={styles.value}>{new Date(pool.created_at).toLocaleString()}</div>
            </div>
          </div>
        </div>

        {/* Agents in Pool */}
        <div className={styles.section}>
          <div className={styles.sectionHeader}>
            <h2>Agents in Pool ({agents.length})</h2>
            <button 
              className={styles.addButton}
              onClick={() => setShowOfflineAgents(!showOfflineAgents)}
            >
              {showOfflineAgents ? 'Hide Offline' : 'Show Offline'}
            </button>
          </div>
          {agents.length === 0 ? (
            <div className={styles.emptyState}>
              <p>No agents assigned to this pool</p>
            </div>
          ) : (
            <table className={styles.table}>
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
              <tbody>
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
                          <span style={{ color: '#8c8c8c', fontSize: '12px' }}>-</span>
                        )}
                      </td>
                      <td style={{ minWidth: '200px', padding: '12px 16px' }}>
                        {metrics ? (
                          <AgentMetricsBar 
                            label="Memory" 
                            value={metrics.memory_usage} 
                          />
                        ) : (
                          <span style={{ color: '#8c8c8c', fontSize: '12px' }}>-</span>
                        )}
                      </td>
                      <td>
                        {metrics && metrics.running_tasks && metrics.running_tasks.length > 0 ? (
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
              </tbody>
            </table>
          )}
        </div>

        {/* Allowed Workspaces */}
        <div className={styles.section}>
          <div className={styles.sectionHeader}>
            <h2>Allowed Workspaces ({allowedWorkspaces.length})</h2>
            <button 
              className={styles.addButton} 
              onClick={() => {
                if (showAddWorkspaceDialog) {
                  setShowAddWorkspaceDialog(false);
                  setSelectedWorkspaceIds([]);
                } else {
                  handleOpenAddDialog();
                }
              }}
            >
              {showAddWorkspaceDialog ? 'Cancel' : '+ Add Workspaces'}
            </button>
          </div>

          {/* Add Workspaces Form (Inline) */}
          {showAddWorkspaceDialog && (
            <div className={styles.addWorkspaceForm}>
              {workspacesLoading ? (
                <div className={styles.loading}>Loading workspaces...</div>
              ) : (
                <>
                  <p className={styles.formHint}>
                    Select workspaces that can use this pool. Selected workspaces will be able to choose this pool for their tasks.
                  </p>
                  <div className={styles.workspaceList}>
                    {allWorkspaces
                      .filter(ws => !allowedWorkspaces.some(aw => aw.workspace_id === (ws.workspace_id || String(ws.id))))
                      .map((workspace) => (
                        <label key={workspace.id} className={styles.workspaceItem}>
                          <input
                            type="checkbox"
                            checked={selectedWorkspaceIds.includes(workspace.workspace_id || String(workspace.id))}
                            onChange={() => toggleWorkspaceSelection(workspace.workspace_id || String(workspace.id))}
                          />
                          <div className={styles.workspaceInfo}>
                            <div className={styles.workspaceName}>{workspace.name}</div>
                            <div className={styles.workspaceIdSmall}>ID: {workspace.workspace_id || workspace.id}</div>
                            {workspace.description && (
                              <div className={styles.workspaceDesc}>{workspace.description}</div>
                            )}
                          </div>
                        </label>
                      ))}
                    {allWorkspaces.filter(ws => !allowedWorkspaces.some(aw => aw.workspace_id === (ws.workspace_id || String(ws.id)))).length === 0 && (
                      <div className={styles.emptyState}>
                        <p>All workspaces are already allowed</p>
                      </div>
                    )}
                  </div>
                  <div className={styles.formActions}>
                    <button
                      className={styles.confirmButton}
                      onClick={handleAddWorkspaces}
                      disabled={selectedWorkspaceIds.length === 0}
                    >
                      Add {selectedWorkspaceIds.length > 0 ? `(${selectedWorkspaceIds.length})` : 'Selected'}
                    </button>
                  </div>
                </>
              )}
            </div>
          )}

          {/* Allowed Workspaces Table */}
          {!showAddWorkspaceDialog && (
            <>
              {allowedWorkspaces.length === 0 ? (
                <div className={styles.emptyState}>
                  <p>No workspaces allowed for this pool</p>
                  <p className={styles.hint}>Click "Add Workspaces" to allow workspaces to use this pool</p>
                </div>
              ) : (
                <table className={styles.table}>
                  <thead>
                    <tr>
                      <th>Workspace Name</th>
                      <th>Workspace ID</th>
                      <th>Status</th>
                      <th>Allowed By</th>
                      <th>Allowed At</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {allowedWorkspaces.map((workspace) => (
                      <tr key={workspace.id}>
                        <td className={styles.workspaceName}>{workspace.workspace_name || workspace.workspace_id}</td>
                        <td className={styles.workspaceId}>{workspace.workspace_id}</td>
                        <td>{getStatusBadge(workspace.status)}</td>
                        <td>{workspace.allowed_by_name || workspace.allowed_by || '-'}</td>
                        <td>{new Date(workspace.allowed_at).toLocaleString()}</td>
                        <td>
                          <button
                            className={styles.revokeButton}
                            onClick={() => setRevokeWorkspaceConfirm({ show: true, workspaceId: workspace.workspace_id })}
                            title="Revoke access"
                          >
                            Revoke
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </>
          )}
        </div>

        {/* Pool Tokens - For both static and k8s pools */}
        {(pool.pool_type === 'static' || pool.pool_type === 'k8s') && (
          <div className={styles.section}>
            <div className={styles.sectionHeader}>
              <h2>Pool Tokens ({poolTokens.filter(t => t.is_active).length})</h2>
              {pool.pool_type === 'static' && (
                <button 
                  className={styles.addButton} 
                  onClick={() => setShowCreateTokenDialog(!showCreateTokenDialog)}
                >
                  {showCreateTokenDialog ? 'Cancel' : '+ Create Token'}
                </button>
              )}
            </div>

            {/* Create Token Form - Only for static pools */}
            {pool.pool_type === 'static' && showCreateTokenDialog && (
              <div className={styles.addWorkspaceForm}>
                {createdToken ? (
                  <div className={styles.tokenCreated}>
                    <h3>✓ Token Created Successfully</h3>
                    <p className={styles.warning}>
                       Please copy this token now. You won't be able to see it again!
                    </p>
                    <div className={styles.tokenDisplay}>
                      <code>{createdToken.token}</code>
                      <button 
                        className={styles.copyButton}
                        onClick={() => handleCopyToken(createdToken.token)}
                      >
                        Copy
                      </button>
                    </div>
                    <div className={styles.tokenInfo}>
                      <p><strong>Token Name:</strong> {createdToken.token_name}</p>
                      <p><strong>Created:</strong> {new Date(createdToken.created_at).toLocaleString()}</p>
                      {createdToken.expires_at && (
                        <p><strong>Expires:</strong> {new Date(createdToken.expires_at).toLocaleString()}</p>
                      )}
                    </div>
                    <button 
                      className={styles.confirmButton}
                      onClick={handleCloseTokenDialog}
                    >
                      Done
                    </button>
                  </div>
                ) : (
                  <>
                    <p className={styles.formHint}>
                      Create a static token for agents to authenticate with this pool. Agents will use this token to register and connect to the pool.
                    </p>
                    <div className={styles.formGroup}>
                      <label htmlFor="tokenName">Token Name *</label>
                      <input
                        id="tokenName"
                        type="text"
                        value={newTokenName}
                        onChange={(e) => setNewTokenName(e.target.value)}
                        placeholder="e.g., production-token"
                        className={styles.input}
                      />
                    </div>
                    <div className={styles.formActions}>
                      <button
                        className={styles.confirmButton}
                        onClick={handleCreateToken}
                        disabled={!newTokenName.trim()}
                      >
                        Create Token
                      </button>
                    </div>
                  </>
                )}
              </div>
            )}

            {/* Tokens Table */}
            {(!showCreateTokenDialog || pool.pool_type === 'k8s') && (
              <>
                {poolTokens.length === 0 ? (
                  <div className={styles.emptyState}>
                    <p>No tokens created for this pool</p>
                    {pool.pool_type === 'static' && (
                      <p className={styles.hint}>Click "Create Token" to generate a new authentication token</p>
                    )}
                  </div>
                ) : (
                  <table className={styles.table}>
                    <thead>
                      <tr>
                        <th>Token Name</th>
                        <th>Status</th>
                        <th>Created By</th>
                        <th>Created At</th>
                        <th>Last Used</th>
                        <th>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {poolTokens.map((token) => {
                        const activeTokenCount = poolTokens.filter(t => t.is_active).length;
                        const isLastActiveToken = token.is_active && activeTokenCount === 1;
                        const canRevoke = pool.pool_type === 'k8s' || !isLastActiveToken;
                        
                        return (
                          <tr key={token.token_name}>
                            <td className={styles.tokenName}>{token.token_name}</td>
                            <td>{getStatusBadge(token.is_active ? 'active' : 'inactive')}</td>
                            <td>{token.created_by || '-'}</td>
                            <td>{new Date(token.created_at).toLocaleString()}</td>
                            <td>
                              {token.last_used_at 
                                ? new Date(token.last_used_at).toLocaleString()
                                : 'Never'}
                            </td>
                            <td>
                              {token.is_active ? (
                                <div style={{ display: 'flex', gap: '8px' }}>
                                  {pool.pool_type === 'k8s' && (
                                    <button
                                      className={styles.editScheduleButton}
                                      onClick={() => setRotateTokenConfirm({ show: true, tokenName: token.token_name })}
                                      title="Rotate token (generates new token and rebuilds Pods)"
                                    >
                                      Rotate
                                    </button>
                                  )}
                                  {pool.pool_type === 'static' && (
                                    <button
                                      className={styles.revokeButton}
                                      onClick={() => setRevokeTokenConfirm({ show: true, tokenName: token.token_name })}
                                      disabled={!canRevoke}
                                      title={isLastActiveToken ? 'Cannot revoke the last active token, please create a new token first' : 'Revoke token'}
                                    >
                                      Revoke
                                    </button>
                                  )}
                                </div>
                              ) : (
                                <span className={styles.revokedText}>Revoked</span>
                              )}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                )}
              </>
            )}
          </div>
        )}

        {/* K8s Configuration - Only for k8s pools */}
        {pool.pool_type === 'k8s' && (
          <div className={styles.section}>
            <div className={styles.sectionHeader}>
              <h2>Kubernetes Configuration</h2>
              <div style={{ display: 'flex', gap: '8px' }}>
                {k8sConfig.image && (
                  <button 
                    className={styles.addButton} 
                    onClick={() => {
                      setShowYamlPreview(!showYamlPreview);
                      setShowK8sConfig(false);
                    }}
                  >
                    {showYamlPreview ? 'Hide YAML' : 'Preview YAML'}
                  </button>
                )}
                {showK8sConfig && (
                  <button 
                    className={styles.editScheduleButton} 
                    onClick={handleSaveK8sConfig}
                    disabled={!k8sConfig.image.trim()}
                    title="Save configuration and sync deployment"
                  >
                    Save Configuration
                  </button>
                )}
                <button 
                  className={styles.addButton} 
                  onClick={() => {
                    setShowK8sConfig(!showK8sConfig);
                    setShowYamlPreview(false);
                  }}
                >
                  {showK8sConfig ? 'Cancel' : 'Configure'}
                </button>
              </div>
            </div>

            {showYamlPreview ? (
              <div className={styles.addWorkspaceForm}>
                <p className={styles.formHint}>
                  Kubernetes Pod模板预览。平台会为每个pool直接管理多个Pod（而非Deployment），并根据槽位利用率自动扩缩容。每个Pod有3个槽位，可以并发执行多个plan任务。
                </p>
                <div className={styles.yamlPreview}>
                  <pre><code>{generatePodYaml()}</code></pre>
                </div>
                <div className={styles.yamlInfo}>
                  <h4>Pod配置说明：</h4>
                  <ul>
                    <li><strong>Pod名称</strong>: iac-agent-{pool.pool_id}-&#123;timestamp&#125; (每个Pod唯一)</li>
                    <li><strong>槽位管理</strong>: 每个Pod有3个槽位 (Slot 0可执行任何任务, Slot 1-2只能执行plan任务)</li>
                    <li><strong>智能扩缩容</strong>: 基于槽位利用率自动扩缩容 (&gt;80%扩容, &lt;20%缩容, min: {k8sConfig.min_replicas}, max: {k8sConfig.max_replicas})</li>
                    <li><strong>安全缩容</strong>: 只删除所有槽位都空闲的Pod，保护正在执行任务的Pod</li>
                    <li><strong>Apply保护</strong>: reserved槽位的Pod不会被删除，确保apply_pending任务不被中断</li>
                    <li><strong>冻结保护</strong>: 在配置的freeze window期间禁止扩容</li>
                    <li><strong>环境变量</strong>: IAC_AGENT_NAME, IAC_AGENT_TOKEN (从secret注入), 以及用户配置的自定义变量</li>
                    <li><strong>Token轮转</strong>: Secret每30天自动轮转，轮转时会重启所有Pod</li>
                  </ul>
                </div>
                <div className={styles.formActions}>
                  <button
                    className={styles.confirmButton}
                    onClick={handleCopyYaml}
                  >
                    Copy YAML
                  </button>
                </div>
              </div>
            ) : showK8sConfig ? (
              <div className={styles.addWorkspaceForm}>
                <p className={styles.formHint}>
                  Configure the Kubernetes Pod template for this pool. The platform will automatically create and manage multiple Pods with slot-based auto-scaling.
                </p>

                <div className={styles.formGroup}>
                  <label htmlFor="k8sImage">Container Image *</label>
                  <input
                    id="k8sImage"
                    type="text"
                    value={k8sConfig.image}
                    onChange={(e) => setK8sConfig({ ...k8sConfig, image: e.target.value })}
                    placeholder="e.g., terraform:latest"
                    className={styles.input}
                  />
                </div>

                <div className={styles.formGroup}>
                  <label htmlFor="imagePullPolicy">Image Pull Policy</label>
                  <select
                    id="imagePullPolicy"
                    value={k8sConfig.image_pull_policy}
                    onChange={(e) => setK8sConfig({ ...k8sConfig, image_pull_policy: e.target.value })}
                    className={styles.input}
                  >
                    <option value="Always">Always</option>
                    <option value="IfNotPresent">IfNotPresent</option>
                  </select>
                  <small className={styles.helpText}>
                    Always: 每次都拉取最新镜像 | IfNotPresent: 本地有镜像则不拉取
                  </small>
                </div>

                <div className={styles.formGroup}>
                  <label htmlFor="namespace">Namespace</label>
                  <input
                    id="namespace"
                    type="text"
                    value={k8sConfig.namespace}
                    onChange={(e) => setK8sConfig({ ...k8sConfig, namespace: e.target.value })}
                    placeholder="terraform"
                    className={styles.input}
                  />
                  <small className={styles.helpText}>
                    Kubernetes namespace for the agent deployment
                  </small>
                </div>

                <div className={styles.formGroup}>
                  <label htmlFor="serviceAccount">Service Account</label>
                  <input
                    id="serviceAccount"
                    type="text"
                    value={k8sConfig.service_account}
                    onChange={(e) => setK8sConfig({ ...k8sConfig, service_account: e.target.value })}
                    placeholder="e.g., terraform-agent"
                    className={styles.input}
                  />
                  <small className={styles.helpText}>
                    Kubernetes ServiceAccount for the agent pods
                  </small>
                </div>

                <div className={styles.formGroup}>
                  <label htmlFor="minReplicas">Minimum Replicas</label>
                  <input
                    id="minReplicas"
                    type="number"
                    min="0"
                    value={k8sConfig.min_replicas}
                    onChange={(e) => setK8sConfig({ ...k8sConfig, min_replicas: parseInt(e.target.value) || 0 })}
                    className={styles.input}
                  />
                  <small className={styles.helpText}>
                    Minimum number of agent replicas to maintain
                  </small>
                </div>

                <div className={styles.formGroup}>
                  <label htmlFor="maxReplicas">Maximum Replicas</label>
                  <input
                    id="maxReplicas"
                    type="number"
                    min="1"
                    value={k8sConfig.max_replicas}
                    onChange={(e) => setK8sConfig({ ...k8sConfig, max_replicas: parseInt(e.target.value) || 1 })}
                    className={styles.input}
                  />
                  <small className={styles.helpText}>
                    Maximum number of agent replicas allowed
                  </small>
                </div>

                <div className={styles.formGroup}>
                  <label>Resource Limits</label>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
                    <div>
                      <input
                        type="text"
                        value={k8sConfig.cpu_limit}
                        onChange={(e) => setK8sConfig({ ...k8sConfig, cpu_limit: e.target.value })}
                        placeholder="CPU Limit"
                        className={styles.input}
                      />
                      <small className={styles.helpText}>e.g., 500m, 2</small>
                    </div>
                    <div>
                      <input
                        type="text"
                        value={k8sConfig.memory_limit}
                        onChange={(e) => setK8sConfig({ ...k8sConfig, memory_limit: e.target.value })}
                        placeholder="Memory Limit"
                        className={styles.input}
                      />
                      <small className={styles.helpText}>e.g., 512Mi, 2Gi</small>
                    </div>
                  </div>
                </div>

                <div className={styles.formGroup}>
                  <label>Environment Variables</label>
                  <div className={styles.envList}>
                    {envPairs.map((pair, index) => (
                      <div key={index} className={styles.envPair}>
                        <input
                          type="text"
                          value={pair.key}
                          onChange={(e) => handleEnvChange(index, 'key', e.target.value)}
                          placeholder="Variable Name"
                          className={styles.input}
                          style={{ flex: 1 }}
                        />
                        <span style={{ padding: '0 8px', color: '#8c8c8c' }}>=</span>
                        <input
                          type="text"
                          value={pair.value}
                          onChange={(e) => handleEnvChange(index, 'value', e.target.value)}
                          placeholder="Value"
                          className={styles.input}
                          style={{ flex: 1 }}
                        />
                        {envPairs.length > 1 && (
                          <button
                            type="button"
                            onClick={() => removeEnvPair(index)}
                            className={styles.removeButton}
                          >
                            ✕
                          </button>
                        )}
                      </div>
                    ))}
                  </div>
                  <button
                    type="button"
                    onClick={addEnvPair}
                    className={styles.addEnvButton}
                  >
                    + Add Environment Variable
                  </button>
                  <small className={styles.helpText}>
                    Custom environment variables for the agent container
                  </small>
                </div>
              </div>
            ) : (
              <div className={styles.k8sConfigDisplay}>
                {k8sConfig.image ? (
                  <>
                    <div className={styles.configItem}>
                      <label>Container Image:</label>
                      <span>{k8sConfig.image}</span>
                    </div>
                    <div className={styles.configItem}>
                      <label>Image Pull Policy:</label>
                      <span>{k8sConfig.image_pull_policy || 'Always'}</span>
                    </div>
                    <div className={styles.configItem}>
                      <label>Namespace:</label>
                      <span>{k8sConfig.namespace}</span>
                    </div>
                    {k8sConfig.service_account && (
                      <div className={styles.configItem}>
                        <label>Service Account:</label>
                        <span>{k8sConfig.service_account}</span>
                      </div>
                    )}
                    <div className={styles.configItem}>
                      <label>Replica Limits:</label>
                      <span>Min: {k8sConfig.min_replicas}, Max: {k8sConfig.max_replicas}</span>
                    </div>
                    <div className={styles.configItem}>
                      <label>CPU Limit:</label>
                      <span>{k8sConfig.cpu_limit}</span>
                    </div>
                    <div className={styles.configItem}>
                      <label>Memory Limit:</label>
                      <span>{k8sConfig.memory_limit}</span>
                    </div>
                  </>
                ) : (
                  <div className={styles.emptyState}>
                    <p>No K8s configuration set</p>
                    <p className={styles.hint}>Click "Configure" to set up Kubernetes Pod template</p>
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        {/* Secrets Management */}
        {poolId && (
          <div className={styles.section}>
            <h2>Secrets</h2>
            <SecretsManager 
              resourceType="agent_pool" 
              resourceId={poolId} 
            />
          </div>
        )}

        {/* Freeze Schedule - Only for k8s pools */}
        {pool.pool_type === 'k8s' && (
          <div className={styles.section}>
            <div className={styles.sectionHeader}>
              <h2>Freeze Schedule</h2>
              <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                {pool.one_time_unfreeze_until && new Date(pool.one_time_unfreeze_until) > new Date() && (
                  <span style={{ 
                    padding: '4px 12px', 
                    background: '#52c41a', 
                    color: 'white', 
                    borderRadius: '4px',
                    fontSize: '12px',
                    fontWeight: 500
                  }}>
                    已解冻至 {new Date(pool.one_time_unfreeze_until).toLocaleTimeString()}
                  </span>
                )}
                <button 
                  className={styles.editScheduleButton}
                  onClick={() => setUnfreezeConfirm(true)}
                  disabled={!!(pool.one_time_unfreeze_until && new Date(pool.one_time_unfreeze_until) > new Date())}
                  title="应急解冻 - 临时绕过冻结规则直到今天结束"
                >
                  应急解冻
                </button>
                <button 
                  className={styles.addButton} 
                  onClick={() => {
                    if (showFreezeSchedule) {
                      handleCancelEdit();
                    } else {
                      setShowFreezeSchedule(true);
                    }
                  }}
                >
                  {showFreezeSchedule ? 'Cancel' : '+ Add Schedule'}
                </button>
              </div>
            </div>

            {showFreezeSchedule ? (
              <div className={styles.freezeScheduleForm}>
                <p className={styles.formHint}>
                  配置冻结时间段，在指定时间内自动禁止pool扩容。可用于维护窗口或成本控制。
                </p>
                
                <div className={styles.freezeFilters}>
                  <div className={styles.filterGroup}>
                    <label>开始时间:</label>
                    <div className={styles.timePickerGroup}>
                      <select
                        value={newSchedule.from_time.split(':')[0] || ''}
                        onChange={(e) => {
                          const hour = e.target.value;
                          const minute = newSchedule.from_time.split(':')[1] || '00';
                          setNewSchedule({ ...newSchedule, from_time: `${hour}:${minute}` });
                        }}
                        className={styles.timeSelect}
                      >
                        <option value="">时</option>
                        {Array.from({ length: 24 }, (_, i) => (
                          <option key={i} value={String(i).padStart(2, '0')}>
                            {String(i).padStart(2, '0')}
                          </option>
                        ))}
                      </select>
                      <span className={styles.timeSeparator}>:</span>
                      <select
                        value={newSchedule.from_time.split(':')[1] || ''}
                        onChange={(e) => {
                          const hour = newSchedule.from_time.split(':')[0] || '00';
                          const minute = e.target.value;
                          setNewSchedule({ ...newSchedule, from_time: `${hour}:${minute}` });
                        }}
                        className={styles.timeSelect}
                      >
                        <option value="">分</option>
                        {Array.from({ length: 60 }, (_, i) => (
                          <option key={i} value={String(i).padStart(2, '0')}>
                            {String(i).padStart(2, '0')}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>

                  <div className={styles.filterGroup}>
                    <label>结束时间:</label>
                    <div className={styles.timePickerGroup}>
                      <select
                        value={newSchedule.to_time.split(':')[0] || ''}
                        onChange={(e) => {
                          const hour = e.target.value;
                          const minute = newSchedule.to_time.split(':')[1] || '00';
                          setNewSchedule({ ...newSchedule, to_time: `${hour}:${minute}` });
                        }}
                        className={styles.timeSelect}
                      >
                        <option value="">时</option>
                        {Array.from({ length: 24 }, (_, i) => (
                          <option key={i} value={String(i).padStart(2, '0')}>
                            {String(i).padStart(2, '0')}
                          </option>
                        ))}
                      </select>
                      <span className={styles.timeSeparator}>:</span>
                      <select
                        value={newSchedule.to_time.split(':')[1] || ''}
                        onChange={(e) => {
                          const hour = newSchedule.to_time.split(':')[0] || '00';
                          const minute = e.target.value;
                          setNewSchedule({ ...newSchedule, to_time: `${hour}:${minute}` });
                        }}
                        className={styles.timeSelect}
                      >
                        <option value="">分</option>
                        {Array.from({ length: 60 }, (_, i) => (
                          <option key={i} value={String(i).padStart(2, '0')}>
                            {String(i).padStart(2, '0')}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>

                  <button 
                    className={styles.searchButton}
                    onClick={handleAddFreezeSchedule}
                    disabled={!newSchedule.from_time || !newSchedule.to_time || newSchedule.weekdays.length === 0}
                  >
                    {editingScheduleIndex !== null ? '更新规则' : '添加规则'}
                  </button>
                </div>

                <div className={styles.weekdaySection}>
                  <label className={styles.weekdayLabel}>生效日期:</label>
                  <div className={styles.weekdaySelector}>
                    {[
                      { name: '周一', value: 1 },
                      { name: '周二', value: 2 },
                      { name: '周三', value: 3 },
                      { name: '周四', value: 4 },
                      { name: '周五', value: 5 },
                      { name: '周六', value: 6 },
                      { name: '周日', value: 7 }
                    ].map((day) => (
                      <label 
                        key={day.value} 
                        className={`${styles.weekdayCheckbox} ${newSchedule.weekdays.includes(day.value) ? styles.weekdayChecked : ''}`}
                      >
                        <input 
                          type="checkbox"
                          checked={newSchedule.weekdays.includes(day.value)}
                          onChange={() => toggleWeekday(day.value)}
                        />
                        <span>{day.name}</span>
                      </label>
                    ))}
                  </div>
                  <small className={styles.helpText}>
                    支持跨天设置（例如：23:00 至 02:00 表示晚上11点到次日凌晨2点）
                  </small>
                </div>
              </div>
            ) : (
              <div className={styles.k8sConfigDisplay}>
                {freezeSchedules.length > 0 ? (
                  <div className={styles.scheduleList}>
                    {freezeSchedules.map((schedule, index) => (
                      <div key={index} className={styles.scheduleItem}>
                        <div className={styles.scheduleInfo}>
                          <div className={styles.scheduleTime}>
                            <span className={styles.scheduleIcon}>🕐</span>
                            <span className={styles.scheduleTimeText}>
                              {schedule.from_time} - {schedule.to_time}
                            </span>
                          </div>
                          <div className={styles.scheduleDays}>
                            <span className={styles.scheduleDaysLabel}>生效日期：</span>
                            <span className={styles.scheduleDaysText}>
                              {schedule.weekdays.map(d => getWeekdayName(d)).join('、')}
                            </span>
                          </div>
                        </div>
                        <div className={styles.scheduleActions}>
                          <button 
                            className={styles.editScheduleButton}
                            onClick={() => handleEditSchedule(index)}
                            title="编辑此冻结规则"
                          >
                            编辑
                          </button>
                          <button 
                            className={styles.revokeButton}
                            onClick={() => handleRemoveFreezeSchedule(index)}
                            title="删除此冻结规则"
                          >
                            删除
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className={styles.emptyState}>
                    <p>暂无冻结规则</p>
                    <p className={styles.hint}>点击"添加冻结规则"按钮配置冻结时间段</p>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Revoke Workspace Confirmation Dialog */}
      {revokeWorkspaceConfirm?.show && (
        <div className={styles.overlay}>
          <div className={styles.confirmDialog}>
            <div className={styles.dialogHeader}>
              <h3>撤销Workspace授权确认</h3>
            </div>
            <div className={styles.dialogContent}>
              <p>确定要撤销workspace <strong>"{revokeWorkspaceConfirm.workspaceId}"</strong> 的访问权限吗？</p>
              <p className={styles.warningText}>
                 撤销后，该workspace将无法使用此pool执行任务。
              </p>
            </div>
            <div className={styles.dialogActions}>
              <button
                className={styles.cancelButton}
                onClick={() => setRevokeWorkspaceConfirm(null)}
              >
                取消
              </button>
              <button
                className={styles.confirmButton}
                onClick={handleRevokeWorkspace}
              >
                确认撤销
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Revoke Token Confirmation Dialog */}
      {revokeTokenConfirm?.show && (
        <div className={styles.overlay}>
          <div className={styles.confirmDialog}>
            <div className={styles.dialogHeader}>
              <h3>撤销Token确认</h3>
            </div>
            <div className={styles.dialogContent}>
              <p>确定要撤销token <strong>"{revokeTokenConfirm.tokenName}"</strong> 吗？</p>
              <p className={styles.warningText}>
                 撤销后，使用此token的agents将无法继续认证。
              </p>
            </div>
            <div className={styles.dialogActions}>
              <button
                className={styles.cancelButton}
                onClick={() => setRevokeTokenConfirm(null)}
              >
                取消
              </button>
              <button
                className={styles.confirmButton}
                onClick={handleRevokeToken}
              >
                确认撤销
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Rotate Token Confirmation Dialog */}
      {rotateTokenConfirm?.show && (
        <div className={styles.overlay}>
          <div className={styles.confirmDialog}>
            <div className={styles.dialogHeader}>
              <h3>轮转Token确认</h3>
            </div>
            <div className={styles.dialogContent}>
              <p>确定要轮转token <strong>"{rotateTokenConfirm.tokenName}"</strong> 吗？</p>
              <p className={styles.warningText}>
                 轮转操作将会：
              </p>
              <ul className={styles.warningList}>
                <li>生成新的token并更新K8s Secret</li>
                <li>强制重启Deployment (滚动更新)</li>
                <li>旧token将被撤销</li>
                <li>所有agent pods将使用新token重新启动</li>
              </ul>
              <p className={styles.warningText}>
                此操作会导致短暂的服务中断，请确保当前没有正在执行的任务。
              </p>
            </div>
            <div className={styles.dialogActions}>
              <button
                className={styles.cancelButton}
                onClick={() => setRotateTokenConfirm(null)}
              >
                取消
              </button>
              <button
                className={styles.confirmButton}
                onClick={handleRotateToken}
              >
                确认轮转
              </button>
            </div>
          </div>
        </div>
      )}

      {/* One-Time Unfreeze Confirmation Dialog */}
      {unfreezeConfirm && (
        <div className={styles.overlay}>
          <div className={styles.confirmDialog}>
            <div className={styles.dialogHeader}>
              <h3>应急解冻确认</h3>
            </div>
            <div className={styles.dialogContent}>
              <p>确定要激活应急解冻吗？</p>
              <p className={styles.warningText}>
                 此操作将：
              </p>
              <ul className={styles.warningList}>
                <li>临时绕过所有冻结规则直到今天23:59:59</li>
                <li>允许pool在冻结时间段内正常扩容</li>
                <li>不会删除已配置的冻结规则</li>
                <li>解冻过期后自动恢复正常冻结策略</li>
              </ul>
              <p className={styles.warningText}>
                此功能仅用于紧急情况,请谨慎使用。
              </p>
            </div>
            <div className={styles.dialogActions}>
              <button
                className={styles.cancelButton}
                onClick={() => setUnfreezeConfirm(false)}
              >
                取消
              </button>
              <button
                className={styles.confirmButton}
                onClick={handleOneTimeUnfreeze}
              >
                确认解冻
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default AgentPoolDetail;
