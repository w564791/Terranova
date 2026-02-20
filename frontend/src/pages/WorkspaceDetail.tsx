import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import api from '../services/api';
import WorkspaceStateBadge from '../components/WorkspaceStateBadge';
import type { WorkspaceState } from '../components/WorkspaceStateBadge';
import VariablesTab from './VariablesTab';
import StatesTab from './StatesTab';
import ResourcesTab from './ResourcesTab';
import HealthTab from './HealthTab';
import WorkspaceSettings from './WorkspaceSettings';
import ProviderSettings from './ProviderSettings';
import WorkspaceOutputs from '../components/WorkspaceOutputs';
import ConfirmDialog from '../components/ConfirmDialog';
import NewRunDialog from '../components/NewRunDialog';
import DateRangePicker from '../components/DateRangePicker';
import TopBar from '../components/TopBar';
import { useTaskAutoRefresh } from '../hooks/useTaskAutoRefresh';
import styles from './WorkspaceDetail.module.css';

interface Workspace {
  id: number;
  workspace_id: string;
  name: string;
  description: string;
  state_backend: string;
  terraform_version: string;
  execution_mode: string;
  current_state: WorkspaceState;
  is_locked: boolean;
  auto_apply: boolean;
  created_at: string;
  updated_at: string;
}

interface WorkspaceOverview {
  id: number;
  name: string;
  description: string;
  is_locked: boolean;
  locked_by: number | null;
  lock_reason: string;
  execution_mode: string;
  terraform_version: string;
  working_directory: string;
  auto_apply: boolean;
  resource_count: number;
  last_plan_at: string | null;
  last_apply_at: string | null;
  drift_count: number;
  last_drift_check: string | null;
  latest_run: {
    id: number;
    task_type: string;
    message: string;
    created_by: string;
    status: string;
    plan_duration: number;
    apply_duration: number;
    changes_add: number;
    changes_change: number;
    changes_destroy: number;
    created_at: string;
  } | null;
  resources: Array<{
    type: string;
    count: number;
  }>;
  created_at: string;
  updated_at: string;
}

type TabType = 'overview' | 'runs' | 'states' | 'resources' | 'variables' | 'outputs' | 'health' | 'settings';
type SettingsSection = 'general' | 'locking' | 'provider' | 'run-tasks' | 'run-triggers' | 'notifications' | 'destruction';

const WorkspaceDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  // 从URL获取当前标签页和section，默认为overview
  const searchParams = new URLSearchParams(window.location.search);
  const tabFromUrl = (searchParams.get('tab') as TabType) || 'overview';
  const sectionFromUrl = (searchParams.get('section') as SettingsSection) || 'general';
  
  const [activeTab, setActiveTab] = useState<TabType>(tabFromUrl);
  const [activeSection, setActiveSection] = useState<SettingsSection>(sectionFromUrl);
  const [settingsExpanded, setSettingsExpanded] = useState(tabFromUrl === 'settings');
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [overview, setOverview] = useState<WorkspaceOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [showLockDialog, setShowLockDialog] = useState(false);
  const [lockReason, setLockReason] = useState('');
  const [lockLoading, setLockLoading] = useState(false);
  const [showNewRunDialog, setShowNewRunDialog] = useState(false);
  
  // 全局Latest Run状态（Overview和Runs标签页共享）
  const [globalLatestRun, setGlobalLatestRun] = useState<any>(null);
  const prevGlobalLatestRunRef = React.useRef<any>(null);
  
  // 全局Resources状态（Overview标签页使用）
  const [globalResources, setGlobalResources] = useState<any[]>([]);
  const [globalResourcesTotal, setGlobalResourcesTotal] = useState<number>(0);
  
  // 当前State资源数量
  const [currentStateResourcesCount, setCurrentStateResourcesCount] = useState<number>(0);
  
  // 切换标签页时更新URL
  const handleTabChange = async (tab: TabType) => {
    setActiveTab(tab);
    
    // 切换到Overview时立即刷新数据
    if (tab === 'overview') {
      try {
        const workspaceData: any = await api.get(`/workspaces/${id}`);
        setWorkspace(workspaceData.data || workspaceData);
        
        const overviewData: any = await api.get(`/workspaces/${id}/overview`);
        setOverview(overviewData.data || overviewData);
      } catch (error) {
        console.error('Failed to refresh overview:', error);
      }
    }
    
    if (tab === 'settings') {
      setSettingsExpanded(true);
      navigate(`/workspaces/${id}?tab=${tab}&section=${activeSection}`, { replace: true });
    } else {
      setSettingsExpanded(false);
      navigate(`/workspaces/${id}?tab=${tab}`, { replace: true });
    }
  };

  // 切换Settings展开/收起
  const handleSettingsToggle = () => {
    if (settingsExpanded) {
      setSettingsExpanded(false);
      // 如果当前在settings tab，切换到overview
      if (activeTab === 'settings') {
        setActiveTab('overview');
        navigate(`/workspaces/${id}?tab=overview`, { replace: true });
      }
    } else {
      setSettingsExpanded(true);
      setActiveTab('settings');
      navigate(`/workspaces/${id}?tab=settings&section=${activeSection}`, { replace: true });
    }
  };

  // 切换Settings子菜单
  const handleSectionChange = (section: SettingsSection) => {
    setActiveSection(section);
    setActiveTab('settings');
    navigate(`/workspaces/${id}?tab=settings&section=${section}`, { replace: true });
  };

  // 全局Resources获取函数（Overview标签页使用）
  const fetchGlobalResources = React.useCallback(async () => {
    if (!id) return;
    
    try {
      // 获取前5个资源
      const params = new URLSearchParams({
        page: '1',
        page_size: '5',
        sort_by: 'created_at',
        sort_order: 'desc',
        include_inactive: 'false',
      });
      
      const response = await api.get(`/workspaces/${id}/resources?${params.toString()}`);
      const data = response.data || response;
      setGlobalResources(data.resources || []);
      setGlobalResourcesTotal(data.pagination?.total || 0);
    } catch (error) {
      console.error('Failed to fetch global resources:', error);
    }
  }, [id]);
  
  // 获取当前State资源数量
  const fetchCurrentStateResourcesCount = React.useCallback(async () => {
    if (!id) return;
    
    try {
      const response = await api.get(`/workspaces/${id}/current-state`);
      if (response.data) {
        setCurrentStateResourcesCount(response.data.resources_count || 0);
      }
    } catch (error) {
      // 没有当前state是正常的
      console.log('No current state');
      setCurrentStateResourcesCount(0);
    }
  }, [id]);
  
  // 全局Latest Run获取函数（Overview和Runs标签页共享）
  const fetchGlobalLatestRun = React.useCallback(async () => {
    if (!id) return;
    
    try {
      // 获取所有任务的第一页（不带时间过滤）
      const params = new URLSearchParams({
        page: '1',
        page_size: '10',
      });
      
      const data: any = await api.get(`/workspaces/${id}/tasks?${params.toString()}`);
      
      if (data && data.tasks && data.tasks.length > 0) {
        const allTasks = data.tasks;
        
        // 优先级：1. Needs Attention任务 2. Running任务 3. 最新任务
        const needsAttentionTask = allTasks.find((t: any) => 
          t.status === 'requires_approval' || t.status === 'plan_completed'
        );
        
        if (needsAttentionTask) {
          const prev = prevGlobalLatestRunRef.current;
          if (!prev || prev.id !== needsAttentionTask.id || prev.status !== needsAttentionTask.status) {
            console.log('[Global] Updating Latest Run to needs attention task:', needsAttentionTask.id);
            setGlobalLatestRun(needsAttentionTask);
            prevGlobalLatestRunRef.current = needsAttentionTask;
          }
          return;
        }
        
        const runningTask = allTasks.find((t: any) => t.status === 'running');
        if (runningTask) {
          const prev = prevGlobalLatestRunRef.current;
          if (!prev || prev.id !== runningTask.id || prev.status !== runningTask.status) {
            console.log('[Global] Updating Latest Run to running task:', runningTask.id);
            setGlobalLatestRun(runningTask);
            prevGlobalLatestRunRef.current = runningTask;
          }
          return;
        }
        
        // 使用最新任务
        const latestTask = allTasks[0];
        const prev = prevGlobalLatestRunRef.current;
        if (!prev || prev.id !== latestTask.id || prev.status !== latestTask.status) {
          console.log('[Global] Updating Latest Run to latest task:', latestTask.id);
          setGlobalLatestRun(latestTask);
          prevGlobalLatestRunRef.current = latestTask;
        }
      }
    } catch (error) {
      console.error('Failed to fetch global latest run:', error);
    }
  }, [id]);

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        
        // 获取workspace基本信息
        const workspaceData: any = await api.get(`/workspaces/${id}`);
        setWorkspace(workspaceData.data || workspaceData);
        
        // 获取overview数据
        const overviewData: any = await api.get(`/workspaces/${id}/overview`);
        setOverview(overviewData.data || overviewData);
        
        // 获取全局Latest Run
        await fetchGlobalLatestRun();
        
        // 获取全局Resources
        await fetchGlobalResources();
        
        // 获取当前State资源数量
        await fetchCurrentStateResourcesCount();
      } catch (error) {
        const message = extractErrorMessage(error);
        showToast(message, 'error');
        navigate('/workspaces');
      } finally {
        setLoading(false);
      }
    };

    if (id) {
      fetchData();
      
      // 已取消定时刷新：工作空间详情只在首次加载和用户操作时刷新
      // const interval = setInterval(() => {
      //   // 静默刷新，不显示loading
      //   const refreshData = async () => {
      //     try {
      //       const workspaceData: any = await api.get(`/workspaces/${id}`);
      //       const newWorkspace = workspaceData.data || workspaceData;
      //       
      //       const overviewData: any = await api.get(`/workspaces/${id}/overview`);
      //       const newOverview = overviewData.data || overviewData;
      //       
      //       // 只在关键字段改变时更新state（排除updated_at，因为它总是在变）
      //       if (!workspace || 
      //           workspace.is_locked !== newWorkspace.is_locked ||
      //           workspace.name !== newWorkspace.name ||
      //           workspace.terraform_version !== newWorkspace.terraform_version) {
      //         setWorkspace(newWorkspace);
      //       }
      //       
      //       if (!overview ||
      //           overview.resource_count !== newOverview.resource_count ||
      //           overview.drift_count !== newOverview.drift_count ||
      //           overview.latest_run?.id !== newOverview.latest_run?.id ||
      //           overview.latest_run?.status !== newOverview.latest_run?.status) {
      //         setOverview(newOverview);
      //       }
      //       
      //       // 刷新全局Latest Run
      //       await fetchGlobalLatestRun();
      //       
      //       // 刷新全局Resources
      //       await fetchGlobalResources();
      //       
      //       // 刷新当前State资源数量
      //       await fetchCurrentStateResourcesCount();
      //     } catch (error) {
      //       console.error('Failed to refresh data:', error);
      //     }
      //   };
      //   refreshData();
      // }, 10000); // 改为10秒刷新一次
      // 
      // return () => clearInterval(interval);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]); // 移除函数依赖，只依赖id

  // 检查是否有未完成的PLAN_AND_APPLY任务
  const hasActivePlanAndApplyTask = React.useMemo(() => {
    if (!globalLatestRun) return false;
    
    // 检查最新任务是否是PLAN_AND_APPLY且未完成
    if (globalLatestRun.task_type === 'plan_and_apply') {
      const nonFinalStatuses = ['pending', 'waiting', 'running', 'plan_completed', 'apply_pending'];
      return nonFinalStatuses.includes(globalLatestRun.status);
    }
    
    return false;
  }, [globalLatestRun]);

  const handleLockWorkspace = () => {
    // 如果有活跃的PLAN_AND_APPLY任务，不允许操作
    if (hasActivePlanAndApplyTask) {
      return;
    }
    
    if (workspace?.is_locked) {
      // 直接解锁
      handleUnlock();
    } else {
      // 显示锁定对话框
      setShowLockDialog(true);
      setLockReason('');
    }
  };

  const handleUnlock = async () => {
    try {
      setLockLoading(true);
      await api.post(`/workspaces/${id}/unlock`);
      showToast('Workspace已解锁', 'success');
      
      // 重新加载workspace信息
      const workspaceData: any = await api.get(`/workspaces/${id}`);
      setWorkspace(workspaceData.data || workspaceData);
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    } finally {
      setLockLoading(false);
    }
  };

  const handleConfirmLock = async () => {
    if (!lockReason.trim()) {
      showToast('请输入锁定原因', 'warning');
      return;
    }

    try {
      setLockLoading(true);
      await api.post(`/workspaces/${id}/lock`, { reason: lockReason });
      showToast('Workspace已锁定', 'success');
      
      // 重新加载workspace信息
      const workspaceData: any = await api.get(`/workspaces/${id}`);
      setWorkspace(workspaceData.data || workspaceData);
      
      setShowLockDialog(false);
      setLockReason('');
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    } finally {
      setLockLoading(false);
    }
  };

  const handleNewRun = () => {
    setShowNewRunDialog(true);
  };

  const handleNewRunSuccess = async () => {
    // 刷新overview数据和全局Latest Run
    try {
      const overviewData: any = await api.get(`/workspaces/${id}/overview`);
      setOverview(overviewData.data || overviewData);
      
      // 刷新全局Latest Run
      await fetchGlobalLatestRun();
    } catch (error) {
      console.error('Failed to refresh overview:', error);
    }
  };

  const renderTabContent = () => {
    switch (activeTab) {
      case 'overview':
        return <OverviewTab overview={overview} workspace={workspace} globalLatestRun={globalLatestRun} globalResources={globalResources} globalResourcesTotal={globalResourcesTotal} currentStateResourcesCount={currentStateResourcesCount} workspaceId={id!} onTabChange={handleTabChange} />;
      case 'runs':
        return <RunsTab workspaceId={id!} globalLatestRun={globalLatestRun} />;
      case 'states':
        return <StatesTab workspaceId={id!} />;
      case 'resources':
        return <ResourcesTab workspaceId={id!} />;
      case 'variables':
        return <VariablesTab workspaceId={id!} />;
      case 'outputs':
        return <WorkspaceOutputs workspaceId={id!} />;
      case 'health':
        return <HealthTab workspaceId={id!} />;
      case 'settings':
        return <WorkspaceSettings section={activeSection} />;
      default:
        return null;
    }
  };

  // 导航菜单项
  const navItems = [
    { id: 'overview', label: 'Overview' },
    { id: 'runs', label: 'Runs' },
    { id: 'states', label: 'States' },
    { id: 'resources', label: 'Resources' },
    { id: 'variables', label: 'Variables' },
    { id: 'outputs', label: 'Outputs' },
    { id: 'health', label: 'Health' },
  ];

  // Settings子菜单项
  const settingsItems = [
    { id: 'general', label: 'General' },
    { id: 'locking', label: 'Locking' },
    { id: 'provider', label: 'Provider' },
    { id: 'run-tasks', label: 'Run Tasks' },
    { id: 'run-triggers', label: 'Run Triggers' },
    { id: 'notifications', label: 'Notifications' },
    { id: 'destruction', label: 'Destruction and Deletion' },
  ];

  // 移动端导航处理函数
  const handleMobileSidebarNavClick = (tab: TabType) => {
    handleTabChange(tab);
    setMobileSidebarOpen(false);
  };

  const handleMobileSidebarSectionClick = (section: SettingsSection) => {
    handleSectionChange(section);
    setMobileSidebarOpen(false);
  };

  return (
    <div className={styles.workspaceLayout}>
      {/* 移动端汉堡菜单按钮 */}
      <button 
        className={styles.mobileSidebarButton}
        onClick={() => setMobileSidebarOpen(true)}
        aria-label="打开菜单"
      >
        ☰
      </button>

      {/* 移动端遮罩层 */}
      {mobileSidebarOpen && (
        <div 
          className={styles.mobileSidebarOverlay}
          onClick={() => setMobileSidebarOpen(false)}
        />
      )}

      {/* 左侧导航栏 - 简洁版 */}
      <aside className={`${styles.workspaceSidebar} ${mobileSidebarOpen ? styles.sidebarMobileOpen : ''}`}>
        <div className={styles.workspaceHeader}>
          <button onClick={() => navigate('/workspaces')} className={styles.backButton}>
            ← Workspaces
          </button>
          
          <h1 className={styles.workspaceTitle}>{workspace?.name || 'Loading...'}</h1>
        </div>

        {/* 导航菜单 */}
        <nav className={styles.workspaceNav}>
          {navItems.map((item) => (
            <Link
              key={item.id}
              to={`/workspaces/${id}?tab=${item.id}`}
              className={`${styles.navItem} ${
                activeTab === item.id ? styles.navItemActive : ''
              }`}
              onClick={(e) => {
                e.preventDefault();
                handleMobileSidebarNavClick(item.id as TabType);
              }}
              onAuxClick={(e) => {
                // 允许中键点击在新标签页打开
                if (e.button === 1) {
                  // 不阻止默认行为，让浏览器处理
                }
              }}
            >
              <span className={styles.navLabel}>{item.label}</span>
            </Link>
          ))}
          
          {/* Settings可展开菜单 */}
          <button
            className={`${styles.navItem} ${styles.navItemExpandable} ${
              activeTab === 'settings' ? styles.navItemActive : ''
            }`}
            onClick={handleSettingsToggle}
          >
            <span className={styles.navLabel}>Settings</span>
            <span className={`${styles.expandIcon} ${settingsExpanded ? styles.expandIconOpen : ''}`}>
              ▼
            </span>
          </button>
          
          {/* Settings子菜单 */}
          {settingsExpanded && (
            <div className={styles.subMenu}>
              {settingsItems.map((item) => (
                <Link
                  key={item.id}
                  to={`/workspaces/${id}?tab=settings&section=${item.id}`}
                  className={`${styles.subMenuItem} ${
                    activeTab === 'settings' && activeSection === item.id ? styles.subMenuItemActive : ''
                  }`}
                  onClick={(e) => {
                    e.preventDefault();
                    handleMobileSidebarSectionClick(item.id as SettingsSection);
                  }}
                >
                  <span className={styles.navLabel}>{item.label}</span>
                </Link>
              ))}
            </div>
          )}
        </nav>
      </aside>

      {/* 右侧内容区 */}
      <main className={styles.workspaceMain}>
        <TopBar title="工作空间" />
        
        {/* 全局头部 - 显示workspace信息和操作按钮（Settings页面除外） */}
        {activeTab !== 'settings' && workspace && (
          <div className={styles.globalHeader}>
            <div className={styles.globalHeaderLeft}>
              <h1 className={styles.globalTitle}>{workspace.name}</h1>
              <div className={styles.globalMeta}>
                <span className={styles.metaItem}>ID: {workspace.workspace_id}</span>
                <span className={styles.metaItem}>
                  {workspace.is_locked ? 'Locked' : 'Unlocked'}
                </span>
                <span className={styles.metaItem}>
                  Resources {currentStateResourcesCount}
                </span>
                <span className={styles.metaItem}>
                  Terraform v{workspace.terraform_version}
                </span>
                <span className={styles.metaItem}>
                  Updated {formatRelativeTime(workspace.updated_at)}
                </span>
              </div>
              {workspace.description && (
                <p className={styles.globalDescription}>{workspace.description}</p>
              )}
            </div>
            <div className={styles.globalHeaderRight}>
              <button 
                className={`${styles.lockButton} ${(workspace.is_locked || hasActivePlanAndApplyTask) ? styles.locked : ''}`}
                onClick={handleLockWorkspace}
                disabled={hasActivePlanAndApplyTask}
                title={hasActivePlanAndApplyTask ? 'Workspace is locked by active PLAN_AND_APPLY task' : ''}
              >
                {workspace.is_locked || hasActivePlanAndApplyTask ? 'Locked' : 'Lock'}
              </button>
              <button 
                className={styles.newRunButton}
                onClick={handleNewRun}
              >
                + New run
              </button>
            </div>
          </div>
        )}
        
        <div className={styles.workspaceContent}>
          {renderTabContent()}
        </div>
      </main>

      {/* Lock对话框 */}
      <ConfirmDialog
        isOpen={showLockDialog}
        title="锁定Workspace"
        confirmText="锁定"
        cancelText="取消"
        onConfirm={handleConfirmLock}
        onCancel={() => setShowLockDialog(false)}
        loading={lockLoading}
        confirmDisabled={!lockReason.trim()}
      >
        <div style={{ marginBottom: '16px' }}>
          <label style={{ display: 'block', marginBottom: '8px', fontWeight: 500, color: 'var(--color-gray-700)' }}>
            锁定原因 *
          </label>
          <input
            type="text"
            value={lockReason}
            onChange={(e) => setLockReason(e.target.value)}
            placeholder="请输入锁定原因"
            style={{
              width: '100%',
              padding: '10px 12px',
              border: '1px solid var(--color-gray-300)',
              borderRadius: 'var(--radius-md)',
              fontSize: '14px'
            }}
            autoFocus
          />
        </div>
      </ConfirmDialog>

      {/* New Run对话框 */}
      <NewRunDialog
        isOpen={showNewRunDialog}
        workspaceId={id!}
        onClose={() => setShowNewRunDialog(false)}
        onSuccess={handleNewRunSuccess}
      />
    </div>
  );
};

// 格式化相对时间
const formatRelativeTime = (dateString: string | null) => {
  if (!dateString) return '从未';
  
  // 处理无效日期（如 "0001-01-01T00:00:00Z"）
  if (dateString.startsWith('0001-01-01')) return '从未';
  
  // 直接解析 ISO 8601 格式的时间字符串
  // 后端返回的时间是 UTC 时间（带 Z 后缀），JavaScript 会自动转换为本地时间
  const date = new Date(dateString);
  const now = new Date();
  
  // 验证日期是否有效
  if (isNaN(date.getTime())) return '无效日期';
  
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  // 5分钟以内显示"刚刚"
  if (diffMins < 5) return '刚刚';
  // 1小时以内显示"X分钟前"
  if (diffMins < 60) return `${diffMins}分钟前`;
  // 24小时以内显示"X小时前"
  if (diffHours < 24) return `${diffHours}小时前`;
  // 超过1天显示具体日期时间（精确到分钟）
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  });
};

// Overview标签页组件
const OverviewTab: React.FC<{ 
  overview: WorkspaceOverview | null; 
  workspace: Workspace | null;
  globalLatestRun: any;
  globalResources: any[];
  globalResourcesTotal: number;
  currentStateResourcesCount: number;
  workspaceId: string;
  onTabChange: (tab: TabType) => void;
}> = ({ 
  overview, 
  workspace,
  globalLatestRun,
  globalResources,
  globalResourcesTotal,
  currentStateResourcesCount,
  workspaceId,
  onTabChange
}) => {
  const navigate = useNavigate();
  
  // 格式化日期
  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  };
  
  // 获取任务的最终状态显示（与Runs页面一致）
  const getFinalStatus = (run: any): string => {
    if (run.status === 'apply_pending') {
      return 'Apply Pending';
    } else if (run.status === 'success' || run.status === 'applied') {
      if (run.task_type === 'plan' || (run.task_type === 'plan_and_apply' && run.status === 'success')) {
        return 'Planned';
      } else if (run.task_type === 'apply' || (run.task_type === 'plan_and_apply' && run.status === 'applied')) {
        return 'Applied';
      }
      return 'Success';
    } else if (run.status === 'failed') {
      return 'Errored';
    } else if (run.status === 'cancelled') {
      return 'Cancelled';
    } else if (run.status === 'running') {
      return 'Running';
    } else if (run.status === 'pending') {
      return 'Pending';
    }
    return run.status;
  };

  // 获取状态分类（与Runs页面一致）
  const getStatusCategory = (status: string): string => {
    if (status === 'success' || status === 'applied' || status === 'planned_and_finished') {
      return 'success';
    }
    if (status === 'requires_approval' || status === 'apply_pending') {
      return 'attention';
    }
    if (status === 'failed') {
      return 'error';
    }
    if (status === 'running') {
      return 'running';
    }
    if (status === 'pending') {
      return 'pending';
    }
    return 'neutral';
  };
  
  if (!overview || !workspace) {
    return <div className={styles.loading}>加载中...</div>;
  }

  return (
    <div className={styles.overviewLayout}>
      {/* 主内容区 */}
      <div className={styles.mainContent}>
        {/* Latest Run - 使用全局Latest Run，与Runs页面完全一致 */}
        <div className={styles.latestRunSection}>
          <div className={styles.latestRunHeader}>
            <h2 className={styles.latestRunTitle}>Latest Run</h2>
            <a 
              href="#"
              onClick={(e) => {
                e.preventDefault();
                onTabChange('runs');
              }}
              className={styles.viewAllLink}
            >
              View all runs
            </a>
          </div>
          
          {globalLatestRun ? (
            <Link 
              to={`/workspaces/${workspaceId}/tasks/${globalLatestRun.id}`}
              className={styles.latestRunCompact}
            >
              {/* 左侧状态指示条 */}
              <div className={`${styles.statusIndicator} ${styles[`indicator-${getStatusCategory(globalLatestRun.status)}`]}`}></div>
              {/* 左侧头像 */}
              <div className={styles.runAvatar}>
                <span className={styles.avatarIcon}>👤</span>
              </div>
              
              {/* 中间内容区 */}
              <div className={styles.runMainContent}>
                {/* 第一行：标题 + CURRENT标签 */}
                <div className={styles.runTitleRow}>
                  <span className={styles.runTitleText}>
                    {globalLatestRun.description || 'Triggered via UI'}
                  </span>
                  <span className={styles.currentBadge}>CURRENT</span>
                </div>
                
                {/* 第二行：元信息 */}
                <div className={styles.runMetaRow}>
                  <span className={styles.runIdMeta}>#{globalLatestRun.id}</span>
                  <span className={styles.metaSeparator}>|</span>
                  <span className={styles.runUserMeta}>
                    {globalLatestRun.created_by ? `user-${globalLatestRun.created_by}` : 'system'} triggered via UI
                  </span>
                  {(globalLatestRun.changes_add !== undefined || globalLatestRun.changes_change !== undefined || globalLatestRun.changes_destroy !== undefined) && (
                    <>
                      <span className={styles.metaSeparator}>|</span>
                      <span className={styles.runChangesMeta}>
                        <span className={styles.changeAddMeta}>+{globalLatestRun.changes_add || 0}</span>
                        <span className={styles.changeModifyMeta}>~{globalLatestRun.changes_change || 0}</span>
                        <span className={styles.changeDestroyMeta}>-{globalLatestRun.changes_destroy || 0}</span>
                      </span>
                    </>
                  )}
                </div>
              </div>
              
              {/* 右侧状态区 */}
              <div className={styles.runStatusArea}>
                <span className={`${styles.runStatusBadge} ${styles[`statusBadge-${getStatusCategory(globalLatestRun.status)}`]}`}>
                  {globalLatestRun.status === 'applied' || globalLatestRun.status === 'success' ? '✓ ' : ''}
                  {getFinalStatus(globalLatestRun)}
                </span>
                <span className={styles.runTimeMeta}>
                  {formatRelativeTime(globalLatestRun.created_at)}
                </span>
              </div>
            </Link>
          ) : (
            <div className={styles.emptyState}>
              <p>No runs yet</p>
            </div>
          )}
        </div>

        {/* Resources */}
        <div className={styles.section}>
          <div className={styles.sectionHeader}>
            <h2 className={styles.sectionTitle}>Resources</h2>
            <span className={styles.resourceCount}>{globalResourcesTotal}</span>
          </div>
          
          <div className={styles.resourcesInfo}>
            <p className={styles.infoText}>
              Current as of the most recent state version.
            </p>
          </div>

          {globalResources.length > 0 ? (
            <div className={styles.resourcesList}>
              <div className={styles.resourcesTable}>
                <div className={styles.tableHeader}>
                  <div>NAME</div>
                  <div>TYPE</div>
                  <div>VERSION</div>
                  <div>STATUS</div>
                  <div>CREATED</div>
                </div>
                <div className={styles.tableBody}>
                  {globalResources.map((resource) => (
                    <Link 
                      key={resource.id} 
                      to={`/workspaces/${workspaceId}/resources/${resource.id}`}
                      className={styles.resourceRow}
                    >
                      <div className={styles.resourceName}>
                        <div className={styles.resourceNameText}>{resource.resource_name}</div>
                        {/* 移动端显示的元信息行 */}
                        <div className={styles.resourceMobileMeta}>
                          <span>{resource.resource_type}</span>
                          <span className={styles.resourceMetaSeparator}>•</span>
                          <span>v{resource.current_version?.version || 1}.0</span>
                          <span className={styles.resourceMetaSeparator}>•</span>
                          <span className={resource.is_active ? styles.resourceMetaEnabled : styles.resourceMetaDeprecated}>
                            {resource.is_active ? 'Enabled' : 'Deprecated'}
                          </span>
                        </div>
                      </div>
                      <div className={styles.resourceType}>
                        {resource.resource_type}
                      </div>
                      <div className={styles.resourceVersion}>
                        <span className={styles.versionNumber}>
                          {resource.current_version?.version || 1}.0
                        </span>
                        {resource.current_version?.is_latest && (
                          <span className={styles.defaultBadge}>DEFAULT</span>
                        )}
                      </div>
                      <div className={styles.resourceStatus}>
                        <span
                          className={`${styles.statusBadge} ${
                            resource.is_active ? styles.statusEnabled : styles.statusDeprecated
                          }`}
                        >
                          {resource.is_active ? 'Enabled' : 'Deprecated'}
                        </span>
                      </div>
                      <div className={styles.resourceCreated}>
                        {formatDate(resource.created_at)}
                      </div>
                    </Link>
                  ))}
                  {globalResourcesTotal > 5 && (
                    <div className={styles.viewAllRow}>
                      <button 
                        onClick={(e) => {
                          e.stopPropagation();
                          onTabChange('resources');
                        }}
                        className={styles.viewAllButton}
                      >
                        View all {globalResourcesTotal} resources →
                      </button>
                    </div>
                  )}
                </div>
              </div>
            </div>
          ) : (
            <div className={styles.emptyState}>
              <p>No resources managed</p>
            </div>
          )}
        </div>
      </div>

      {/* 右侧边栏 */}
      <div className={styles.sidebar}>
        {/* Workspace Info */}
        <div className={styles.sidebarCard}>
          <div className={styles.sidebarItem}>
            <div className={styles.sidebarContent}>
              <div className={styles.sidebarLabel}>Execution mode:</div>
              <div className={styles.sidebarValue}>{workspace.execution_mode}</div>
            </div>
          </div>
          <div className={styles.sidebarItem}>
            <div className={styles.sidebarContent}>
              <div className={styles.sidebarLabel}>Auto apply:</div>
              <div className={styles.sidebarValue}>{workspace.auto_apply ? 'On' : 'Off'}</div>
            </div>
          </div>
          <div className={styles.sidebarItem}>
            <div className={styles.sidebarContent}>
              <div className={styles.sidebarLabel}>Project:</div>
              <div className={styles.sidebarValue}>default</div>
            </div>
          </div>
        </div>

        {/* Health */}
        <div className={styles.sidebarCard}>
          <div className={styles.sidebarHeader}>
            <h3 className={styles.sidebarTitle}>Health</h3>
            <span className={styles.sidebarTime}>{formatRelativeTime(overview.last_drift_check)}</span>
          </div>
          <div className={styles.healthStatus}>
            <div className={styles.healthItem}>
              <span className={styles.healthLabel}>Drift</span>
              <a 
                href="#" 
                className={styles.healthLink}
                onClick={(e) => {
                  e.preventDefault();
                  onTabChange('health');
                }}
              >
                View Details
              </a>
            </div>
            <div className={styles.healthBar}>
              <div className={styles.healthBarGreen} style={{ width: '100%' }}></div>
            </div>
            <div className={styles.healthStats}>
              <div className={styles.healthStat}>
                <span className={styles.healthStatLabel}>Drifted resources</span>
                <span className={styles.healthStatValue}>{overview.drift_count}</span>
              </div>
              <div className={styles.healthStat}>
                <span className={styles.healthStatLabel}>Not drifted</span>
                <span className={styles.healthStatValue}>{overview.resource_count - overview.drift_count}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Metrics */}
        <div className={styles.sidebarCard}>
          <h3 className={styles.sidebarTitle}>Metrics</h3>
          <div className={styles.metricsItem}>
            <div className={styles.metricsLabel}>Workspace resources</div>
            <div className={styles.metricsValue}>{globalResourcesTotal}</div>
          </div>
          <div className={styles.metricsItem}>
            <div className={styles.metricsLabel}>State resources</div>
            <div className={styles.metricsValue}>{currentStateResourcesCount}</div>
          </div>
        </div>
      </div>
    </div>
  );
};

// Runs标签页组件
interface Run {
  id: number;
  task_type: string;
  status: string;
  created_at: string;
  completed_at?: string;
  created_by?: number;
  description?: string;
  changes_add?: number;
  changes_change?: number;
  changes_destroy?: number;
  output?: string;
}

type FilterType = 'all' | 'needs_attention' | 'errored' | 'running' | 'on_hold' | 'success' | 'cancelled';
type TimeFilter = 'all' | 'today' | '24h' | '7d' | '30d' | 'custom';

const RunsTab: React.FC<{ workspaceId: string; globalLatestRun: any }> = ({ workspaceId, globalLatestRun }) => {
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  // 从URL读取过滤器状态
  const searchParams = new URLSearchParams(window.location.search);
  const filterFromUrl = (searchParams.get('filter') as FilterType) || 'all';
  const timeFilterFromUrl = (searchParams.get('timeFilter') as TimeFilter) || 'all';
  const pageFromUrl = parseInt(searchParams.get('page') || '1');
  const pageSizeFromUrl = parseInt(searchParams.get('pageSize') || '10');
  const searchFromUrl = searchParams.get('search') || '';
  const startDateFromUrl = searchParams.get('startDate') || '';
  const endDateFromUrl = searchParams.get('endDate') || '';
  
  const [filter, setFilter] = useState<FilterType>(filterFromUrl);
  const [timeFilter, setTimeFilter] = useState<TimeFilter>(timeFilterFromUrl);
  const [customStartDate, setCustomStartDate] = useState(startDateFromUrl);
  const [customEndDate, setCustomEndDate] = useState(endDateFromUrl);
  const [showCustomDatePicker, setShowCustomDatePicker] = useState(timeFilterFromUrl === 'custom');
  const [searchQuery, setSearchQuery] = useState(searchFromUrl);
  const [page, setPage] = useState(pageFromUrl);
  const [pageSize, setPageSize] = useState(pageSizeFromUrl);
  const [runs, setRuns] = useState<Run[]>([]);
  const [loading, setLoading] = useState(true);
  const [total, setTotal] = useState(0);
  const [showCancelDialog, setShowCancelDialog] = useState(false);
  const [cancellingRunId, setCancellingRunId] = useState<number | null>(null);
  const [cancellingRun, setCancellingRun] = useState(false);
  const [filterCounts, setFilterCounts] = useState({
    all: 0,
    needsAttention: 0,
    errored: 0,
    running: 0,
    onHold: 0,
    success: 0,
    cancelled: 0,
  });
  
  // 使用ref存储上一次的数据，用于精确比较
  const prevAllRunsRef = React.useRef<Run[]>([]);

  // Auto-refresh incomplete tasks
  const handleIncompleteTasksUpdate = React.useCallback((incompleteTasks: any[]) => {
    if (incompleteTasks.length === 0) return;

    setRuns(prevRuns => {
      // Create a map of incomplete tasks by ID
      const incompleteMap = new Map(incompleteTasks.map(task => [task.id, task]));
      
      // Update existing runs with new data from incomplete tasks
      const updatedRuns = prevRuns.map(run => {
        const updated = incompleteMap.get(run.id);
        return updated || run;
      });
      
      // Check if any changes were made
      const hasChanges = updatedRuns.some((run, index) => {
        const prev = prevRuns[index];
        return !prev || run.status !== prev.status || 
               run.changes_add !== prev.changes_add ||
               run.changes_change !== prev.changes_change ||
               run.changes_destroy !== prev.changes_destroy;
      });
      
      if (hasChanges) {
        console.log('[RunsTab] Updated incomplete tasks in list');
        return updatedRuns;
      }
      
      return prevRuns;
    });
  }, []);

  // Enable auto-refresh (only when no filters/search are active to avoid confusion)
  const autoRefreshEnabled = filter === 'all' && timeFilter === 'all' && !searchQuery;
  
  useTaskAutoRefresh({
    workspaceId,
    enabled: autoRefreshEnabled,
    onUpdate: handleIncompleteTasksUpdate,
    interval: 5000, // 5 seconds
  });

  useEffect(() => {
    fetchRuns();
    
    // 已取消定时刷新：运行列表只在首次加载和筛选条件改变时刷新
    // const interval = setInterval(() => {
    //   fetchRuns();
    // }, 5000);
    // return () => clearInterval(interval);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, pageSize, filter, timeFilter, searchQuery, customStartDate, customEndDate, workspaceId]);

  const fetchRuns = async () => {
    try {
      // 只在首次加载时显示loading
      if (prevAllRunsRef.current.length === 0) {
        setLoading(true);
      }
      
      // 构建查询参数
      const params = new URLSearchParams({
        page: page.toString(),
        page_size: pageSize.toString(),
      });
      
      // 添加搜索参数
      if (searchQuery) {
        params.append('search', searchQuery);
      }
      
      // 添加时间范围参数
      if (timeFilter === 'custom') {
        if (customStartDate) {
          // 转换为ISO 8601格式
          const startDateTime = new Date(customStartDate);
          startDateTime.setHours(0, 0, 0, 0);
          params.append('start_date', startDateTime.toISOString());
        }
        if (customEndDate) {
          const endDateTime = new Date(customEndDate);
          endDateTime.setHours(23, 59, 59, 999);
          params.append('end_date', endDateTime.toISOString());
        }
      } else if (timeFilter !== 'all') {
        // 计算相对时间范围
        const endDate = new Date();
        let startDate = new Date();
        
        if (timeFilter === 'today') {
          // 当天00:00:00到现在
          startDate = new Date();
          startDate.setHours(0, 0, 0, 0);
          // endDate保持当前时间
        } else if (timeFilter === '24h') {
          // 24小时前
          startDate = new Date(endDate.getTime() - 24 * 60 * 60 * 1000);
        } else if (timeFilter === '7d') {
          // 7天前
          startDate = new Date(endDate.getTime() - 7 * 24 * 60 * 60 * 1000);
        } else if (timeFilter === '30d') {
          // 30天前
          startDate = new Date(endDate.getTime() - 30 * 24 * 60 * 60 * 1000);
        }
        params.append('start_date', startDate.toISOString());
        params.append('end_date', endDate.toISOString());
      }
      
      // 添加状态过滤参数
      if (filter !== 'all') {
        params.append('status', filter);
      }
      
      // 使用后端分页API
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks?${params.toString()}`);
      
      if (data && data.tasks) {
        const tasks = data.tasks || [];
        
        // 深度比较：检查tasks是否真的改变了
        const prevTasks = prevAllRunsRef.current;
        let hasChanged = tasks.length !== prevTasks.length;
        
        if (!hasChanged) {
          // 长度相同，逐个比较关键字段
          for (let i = 0; i < tasks.length; i++) {
            const newTask = tasks[i];
            const oldTask = prevTasks[i];
            if (!oldTask || 
                newTask.id !== oldTask.id || 
                newTask.status !== oldTask.status ||
                newTask.changes_add !== oldTask.changes_add ||
                newTask.changes_change !== oldTask.changes_change ||
                newTask.changes_destroy !== oldTask.changes_destroy) {
              hasChanged = true;
              break;
            }
          }
        }
        
        // 只在数据真正改变时更新state
        if (hasChanged) {
          prevAllRunsRef.current = tasks;
          setRuns(tasks);
        }
        
        // 使用后端返回的total和filter_counts
        setTotal(data.total || 0);
        
        // 更新filter counts（如果后端返回了）
        if (data.filter_counts) {
          setFilterCounts({
            all: data.filter_counts.all || 0,
            needsAttention: data.filter_counts.needs_attention || 0,
            errored: data.filter_counts.errored || 0,
            running: data.filter_counts.running || 0,
            onHold: data.filter_counts.on_hold || 0,
            success: data.filter_counts.success || 0,
            cancelled: data.filter_counts.cancelled || 0,
          });
        }
      } else {
        if (prevAllRunsRef.current.length > 0) {
          prevAllRunsRef.current = [];
          setRuns([]);
        }
        setTotal(0);
      }
    } catch (error) {
      console.error('fetchRuns error:', error);
      const message = extractErrorMessage(error);
      showToast(message, 'error');
      if (prevAllRunsRef.current.length > 0) {
        prevAllRunsRef.current = [];
        setRuns([]);
      }
      setTotal(0);
    } finally {
      setLoading(false);
    }
  };
  
  // 更新URL参数
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    params.set('tab', 'runs');
    params.set('filter', filter);
    params.set('timeFilter', timeFilter);
    params.set('page', page.toString());
    params.set('pageSize', pageSize.toString());
    if (searchQuery) {
      params.set('search', searchQuery);
    } else {
      params.delete('search');
    }
    if (customStartDate) {
      params.set('startDate', customStartDate);
    } else {
      params.delete('startDate');
    }
    if (customEndDate) {
      params.set('endDate', customEndDate);
    } else {
      params.delete('endDate');
    }
    navigate(`/workspaces/${workspaceId}?${params.toString()}`, { replace: true });
  }, [filter, timeFilter, page, pageSize, searchQuery, customStartDate, customEndDate, workspaceId, navigate]);

  // 当filter或timeFilter改变时，重置到第一页
  useEffect(() => {
    if (page !== 1) {
      setPage(1);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter, timeFilter, searchQuery]);

  const handleCancelRun = (runId: number) => {
    setCancellingRunId(runId);
    setShowCancelDialog(true);
  };

  const confirmCancelRun = async () => {
    if (!cancellingRunId) return;

    try {
      setCancellingRun(true);
      await api.post(`/workspaces/${workspaceId}/tasks/${cancellingRunId}/cancel`);
      showToast('运行已取消', 'success');
      setShowCancelDialog(false);
      setCancellingRunId(null);
      fetchRuns();
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    } finally {
      setCancellingRun(false);
    }
  };

  // 计算总页数（基于过滤后的结果）
  const totalPages = Math.ceil(total / pageSize);
  
  // 获取任务的最终状态显示
  const getFinalStatus = (run: Run): string => {
    // apply_pending状态表示plan完成，等待apply确认
    if (run.status === 'apply_pending') {
      return 'Apply Pending';
    } else if (run.status === 'planned_and_finished') {
      // Plan完成，无需Apply（无变更）
      return 'Planned and Finished';
    } else if (run.status === 'success' || run.status === 'applied') {
      // 根据任务类型和stage判断最终状态
      if (run.task_type === 'plan' || (run.task_type === 'plan_and_apply' && run.status === 'success')) {
        return 'Planned';
      } else if (run.task_type === 'apply' || (run.task_type === 'plan_and_apply' && run.status === 'applied')) {
        return 'Applied';
      }
      return 'Success';
    } else if (run.status === 'failed') {
      return 'Errored';
    } else if (run.status === 'cancelled') {
      return 'Cancelled';
    } else if (run.status === 'running') {
      return 'Running';
    } else if (run.status === 'pending') {
      return 'Pending';
    }
    return run.status;
  };

  // 获取状态分类（用于左侧指示条颜色）
  const getStatusCategory = (status: string): string => {
    // 成功状态 - 绿色
    if (status === 'success' || status === 'applied' || status === 'planned_and_finished') {
      return 'success';
    }
    // Needs Attention状态 - 黄色
    if (status === 'requires_approval' || status === 'apply_pending') {
      return 'attention';
    }
    // 失败状态 - 红色
    if (status === 'failed') {
      return 'error';
    }
    // 运行中状态 - 蓝色
    if (status === 'running') {
      return 'running';
    }
    // Pending状态 - 青色
    if (status === 'pending') {
      return 'pending';
    }
    // 其他状态 - 灰色
    return 'neutral';
  };

  return (
    <div className={styles.runsContainer}>
      {/* Cancel Run确认对话框 */}
      <ConfirmDialog
        isOpen={showCancelDialog}
        title="取消运行"
        message="确定要取消这个运行吗？取消后任务将停止执行。"
        confirmText="确认取消"
        cancelText="返回"
        onConfirm={confirmCancelRun}
        onCancel={() => {
          setShowCancelDialog(false);
          setCancellingRunId(null);
        }}
      />

      {/* Latest Run - 使用全局Latest Run，与Overview页面完全一致 */}
      {globalLatestRun && (
        <div className={styles.latestRunSection}>
          <div className={styles.latestRunHeader}>
            <h2 className={styles.latestRunTitle}>Latest Run</h2>
          </div>
          <Link 
            to={`/workspaces/${workspaceId}/tasks/${globalLatestRun.id}`}
            className={styles.latestRunCompact}
          >
            {/* 左侧状态指示条 */}
            <div className={`${styles.statusIndicator} ${styles[`indicator-${getStatusCategory(globalLatestRun.status)}`]}`}></div>
            {/* 左侧头像 */}
            <div className={styles.runAvatar}>
              <span className={styles.avatarIcon}>👤</span>
            </div>
            
            {/* 中间内容区 */}
            <div className={styles.runMainContent}>
              {/* 第一行：标题 + CURRENT标签 */}
              <div className={styles.runTitleRow}>
                <span className={styles.runTitleText}>
                  {globalLatestRun.description || 'Triggered via UI'}
                </span>
                <span className={styles.currentBadge}>CURRENT</span>
              </div>
              
              {/* 第二行：元信息 */}
              <div className={styles.runMetaRow}>
                <span className={styles.runIdMeta}>#{globalLatestRun.id}</span>
                <span className={styles.metaSeparator}>|</span>
                <span className={styles.runUserMeta}>
                  {globalLatestRun.created_by ? `user-${globalLatestRun.created_by}` : 'system'} triggered via UI
                </span>
                {(globalLatestRun.changes_add !== undefined || globalLatestRun.changes_change !== undefined || globalLatestRun.changes_destroy !== undefined) && (
                  <>
                    <span className={styles.metaSeparator}>|</span>
                    <span className={styles.runChangesMeta}>
                      <span className={styles.changeAddMeta}>+{globalLatestRun.changes_add || 0}</span>
                      <span className={styles.changeModifyMeta}>~{globalLatestRun.changes_change || 0}</span>
                      <span className={styles.changeDestroyMeta}>-{globalLatestRun.changes_destroy || 0}</span>
                    </span>
                  </>
                )}
              </div>
            </div>
            
            {/* 右侧状态区 */}
            <div className={styles.runStatusArea}>
              <span className={`${styles.runStatusBadge} ${styles[`statusBadge-${getStatusCategory(globalLatestRun.status)}`]}`}>
                {globalLatestRun.status === 'applied' || globalLatestRun.status === 'success' ? '✓ ' : ''}
                {getFinalStatus(globalLatestRun)}
              </span>
              <span className={styles.runTimeMeta}>
                {formatRelativeTime(globalLatestRun.created_at)}
              </span>
            </div>
          </Link>
        </div>
      )}

      {/* Run List Header */}
      <div className={styles.section}>
        <h2 className={styles.sectionTitle}>Run List</h2>
        
        <div className={styles.filterBar}>
          <button
            className={`${styles.filterButton} ${filter === 'all' ? styles.filterActive : ''}`}
            onClick={() => setFilter('all')}
          >
            All <span className={styles.filterCount}>{filterCounts.all}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'needs_attention' ? styles.filterActive : ''}`}
            onClick={() => setFilter('needs_attention')}
          >
            Needs Attention <span className={styles.filterCount}>{filterCounts.needsAttention}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'errored' ? styles.filterActive : ''}`}
            onClick={() => setFilter('errored')}
          >
            Errored <span className={styles.filterCount}>{filterCounts.errored}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'running' ? styles.filterActive : ''}`}
            onClick={() => setFilter('running')}
          >
            Running <span className={styles.filterCount}>{filterCounts.running}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'on_hold' ? styles.filterActive : ''}`}
            onClick={() => setFilter('on_hold')}
          >
            On Hold <span className={styles.filterCount}>{filterCounts.onHold}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'success' ? styles.filterActive : ''}`}
            onClick={() => setFilter('success')}
          >
            Success <span className={styles.filterCount}>{filterCounts.success}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'cancelled' ? styles.filterActive : ''}`}
            onClick={() => setFilter('cancelled')}
          >
            Cancelled <span className={styles.filterCount}>{filterCounts.cancelled}</span>
          </button>
        </div>

        <div className={styles.filterRow}>
          <div className={styles.timeFilterBar}>
            <button
              className={`${styles.timeFilterButton} ${timeFilter === 'all' ? styles.timeFilterActive : ''}`}
              onClick={() => {
                setTimeFilter('all');
                setShowCustomDatePicker(false);
              }}
            >
              All Time
            </button>
            <button
              className={`${styles.timeFilterButton} ${timeFilter === 'today' ? styles.timeFilterActive : ''}`}
              onClick={() => {
                setTimeFilter('today' as TimeFilter);
                setShowCustomDatePicker(false);
              }}
            >
              Today
            </button>
            <button
              className={`${styles.timeFilterButton} ${timeFilter === '24h' ? styles.timeFilterActive : ''}`}
              onClick={() => {
                setTimeFilter('24h');
                setShowCustomDatePicker(false);
              }}
            >
              Last 24 Hours
            </button>
            <button
              className={`${styles.timeFilterButton} ${timeFilter === '7d' ? styles.timeFilterActive : ''}`}
              onClick={() => {
                setTimeFilter('7d');
                setShowCustomDatePicker(false);
              }}
            >
              Last 7 Days
            </button>
            <button
              className={`${styles.timeFilterButton} ${timeFilter === '30d' ? styles.timeFilterActive : ''}`}
              onClick={() => {
                setTimeFilter('30d');
                setShowCustomDatePicker(false);
              }}
            >
              Last 30 Days
            </button>
            <button
              className={`${styles.timeFilterButton} ${timeFilter === 'custom' ? styles.timeFilterActive : ''}`}
              onClick={() => {
                setTimeFilter('custom');
                setShowCustomDatePicker(true);
              }}
            >
              Custom Range
            </button>
          </div>
          
          <div className={styles.searchBar}>
            <input 
              type="text" 
              placeholder="Search by description, ID, or type" 
              className={styles.searchInput}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
        </div>

        {/* 专业日期范围选择器 */}
        {showCustomDatePicker && (
          <DateRangePicker
            startDate={customStartDate}
            endDate={customEndDate}
            onApply={(start, end) => {
              setCustomStartDate(start);
              setCustomEndDate(end);
            }}
            onClear={() => {
              setCustomStartDate('');
              setCustomEndDate('');
              setShowCustomDatePicker(false);
            }}
            onCancel={() => {
              setShowCustomDatePicker(false);
            }}
          />
        )}
      </div>

      {/* Runs List */}
      <div className={styles.section}>
        {loading ? (
          <div className={styles.loading}>加载中...</div>
        ) : runs.length === 0 ? (
          <div className={styles.emptyState}>
            <p>No runs found</p>
          </div>
        ) : (
          <div className={styles.runsList}>
            {runs.map((run, index) => (
              <Link 
                key={run.id} 
                to={`/workspaces/${workspaceId}/tasks/${run.id}`}
                className={`${styles.runItemCompact} ${index === 0 && filter === 'all' && page === 1 ? styles.runItemFirst : ''}`}
              >
                {/* 第一个任务显示左侧状态指示条 */}
                {index === 0 && filter === 'all' && page === 1 && (
                  <div className={`${styles.statusIndicator} ${styles[`indicator-${getStatusCategory(run.status)}`]}`}></div>
                )}
                {/* 左侧头像 */}
                <div className={styles.runAvatar}>
                  <span className={styles.avatarIcon}>👤</span>
                </div>
                
                {/* 中间内容区 */}
                <div className={styles.runMainContent}>
                  {/* 第一行：标题 + CURRENT标签 */}
                  <div className={styles.runTitleRow}>
                    <span className={styles.runTitleText}>
                      {run.description || 'Triggered via UI'}
                    </span>
                    {index === 0 && filter === 'all' && page === 1 && (
                      <span className={styles.currentBadge}>CURRENT</span>
                    )}
                  </div>
                  
                  {/* 第二行：元信息 */}
                  <div className={styles.runMetaRow}>
                    <span className={styles.runIdMeta}>#{run.id}</span>
                    <span className={styles.metaSeparator}>|</span>
                    <span className={styles.runUserMeta}>
                      {run.created_by ? `user-${run.created_by}` : 'system'} triggered via UI
                    </span>
                    {(run.changes_add !== undefined || run.changes_change !== undefined || run.changes_destroy !== undefined) && (
                      <>
                        <span className={styles.metaSeparator}>|</span>
                        <span className={styles.runChangesMeta}>
                          <span className={styles.changeAddMeta}>+{run.changes_add || 0}</span>
                          <span className={styles.changeModifyMeta}>~{run.changes_change || 0}</span>
                          <span className={styles.changeDestroyMeta}>-{run.changes_destroy || 0}</span>
                        </span>
                      </>
                    )}
                  </div>
                </div>
                
                {/* 右侧状态区 */}
                <div className={styles.runStatusArea}>
                  <span className={`${styles.runStatusBadge} ${styles[`statusBadge-${getStatusCategory(run.status)}`]}`}>
                    {run.status === 'applied' || run.status === 'success' ? '✓ ' : ''}
                    {getFinalStatus(run)}
                  </span>
                  <span className={styles.runTimeMeta}>
                    {(run.status === 'success' || run.status === 'applied' || run.status === 'failed' || run.status === 'cancelled') && run.completed_at
                      ? formatRelativeTime(run.completed_at)
                      : formatRelativeTime(run.created_at)}
                  </span>
                </div>
              </Link>
            ))}
          </div>
        )}

        {/* Pagination */}
        {total > 0 && (
          <div className={styles.paginationContainer}>
            <div className={styles.paginationLeft}>
              <div className={styles.paginationInfo}>
                Showing {Math.min((page - 1) * pageSize + 1, total)} to {Math.min(page * pageSize, total)} of {total} runs
              </div>
              <div className={styles.pageSizeSelector}>
                <label className={styles.pageSizeLabel}>Per page:</label>
                <select 
                  value={pageSize} 
                  onChange={(e) => {
                    setPageSize(Number(e.target.value));
                    setPage(1); // 重置到第一页
                  }}
                  className={styles.pageSizeSelect}
                >
                  <option value={10}>10</option>
                  <option value={20}>20</option>
                  <option value={50}>50</option>
                  <option value={100}>100</option>
                </select>
              </div>
            </div>
            <div className={styles.paginationControls}>
              <button
                onClick={() => setPage(page - 1)}
                disabled={page === 1}
                className={styles.paginationButton}
              >
                ← Previous
              </button>
              <span className={styles.paginationPages}>
                Page {page} of {Math.ceil(total / pageSize)}
              </span>
              <button
                onClick={() => setPage(page + 1)}
                disabled={page >= Math.ceil(total / pageSize)}
                className={styles.paginationButton}
              >
                Next →
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

export default WorkspaceDetail;
