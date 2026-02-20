import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import styles from './TaskTriggerExecutions.module.css';

interface TriggerExecution {
  id: number;
  source_task_id: number;
  run_trigger_id: number;
  target_task_id?: number;
  status: 'pending' | 'triggered' | 'skipped' | 'failed';
  temporarily_disabled: boolean;
  disabled_by?: string;
  disabled_at?: string;
  error_message?: string;
  run_trigger?: {
    id: number;
    target_workspace_id: string;
    target_workspace?: {
      workspace_id: string;
      name: string;
      auto_apply: boolean;
    };
  };
  target_task?: {
    id: number;
    status: string;
  };
}

interface RunTrigger {
  id: number;
  source_workspace_id: string;
  target_workspace_id: string;
  enabled: boolean;
  trigger_condition: string;
  target_workspace?: {
    workspace_id: string;
    name: string;
    auto_apply: boolean;
  };
}

interface Props {
  workspaceId: string;
  taskId: number;
  taskStatus: string;
}

const TaskTriggerExecutions: React.FC<Props> = ({ workspaceId, taskId, taskStatus }) => {
  const { success, error: showError } = useToast();
  const [executions, setExecutions] = useState<TriggerExecution[]>([]);
  const [triggers, setTriggers] = useState<RunTrigger[]>([]);
  const [loading, setLoading] = useState(true);
  const [initialLoadDone, setInitialLoadDone] = useState(false);

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  const fetchExecutions = async () => {
    try {
      const response = await fetch(
        `/api/v1/workspaces/${workspaceId}/tasks/${taskId}/trigger-executions`,
        { headers: getAuthHeaders() }
      );
      if (response.ok) {
        const data = await response.json();
        setExecutions(data.trigger_executions || []);
      }
    } catch (error) {
      console.error('Failed to fetch trigger executions:', error);
    }
  };

  const fetchTriggers = async () => {
    try {
      const response = await fetch(
        `/api/v1/workspaces/${workspaceId}/run-triggers`,
        { headers: getAuthHeaders() }
      );
      if (response.ok) {
        const data = await response.json();
        // 只显示启用的 triggers
        setTriggers((data.run_triggers || []).filter((t: RunTrigger) => t.enabled));
      }
    } catch (error) {
      console.error('Failed to fetch run triggers:', error);
    }
  };

  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      await Promise.all([fetchExecutions(), fetchTriggers()]);
      setLoading(false);
      setInitialLoadDone(true);
    };
    
    loadData();
    
    // 如果任务还在执行中，定期刷新
    if (['pending', 'planning', 'plan_completed', 'applying', 'apply_pending', 'running'].includes(taskStatus)) {
      const interval = setInterval(fetchExecutions, 5000);
      return () => clearInterval(interval);
    }
  }, [workspaceId, taskId, taskStatus]);

  const handleToggle = async (executionId: number, currentlyDisabled: boolean) => {
    try {
      const response = await fetch(
        `/api/v1/workspaces/${workspaceId}/tasks/${taskId}/trigger-executions/${executionId}/toggle`,
        {
          method: 'POST',
          headers: getAuthHeaders(),
          body: JSON.stringify({ disabled: !currentlyDisabled }),
        }
      );
      if (response.ok) {
        success(`Trigger ${currentlyDisabled ? 'enabled' : 'disabled'}`);
        fetchExecutions();
      } else {
        showError('Failed to toggle trigger');
      }
    } catch (error) {
      showError('Failed to toggle trigger');
    }
  };

  const getStatusClass = (execution: TriggerExecution) => {
    if (execution.temporarily_disabled) return styles.statusDisabled;
    switch (execution.status) {
      case 'pending': return styles.statusPending;
      case 'triggered': return styles.statusTriggered;
      case 'skipped': return styles.statusSkipped;
      case 'failed': return styles.statusFailed;
      default: return '';
    }
  };

  const getStatusText = (execution: TriggerExecution) => {
    if (execution.temporarily_disabled) return 'Disabled';
    switch (execution.status) {
      case 'pending': return 'Pending';
      case 'triggered': return 'Triggered';
      case 'skipped': return 'Skipped';
      case 'failed': return 'Failed';
      default: return execution.status;
    }
  };

  const canModify = ['pending', 'planning', 'plan_completed', 'applying', 'apply_pending', 'running'].includes(taskStatus);
  const hasExecutions = executions.length > 0;
  const displayData = hasExecutions ? executions : triggers;

  // 如果初始加载完成且没有任何数据，不显示组件
  if (initialLoadDone && displayData.length === 0) {
    return null;
  }

  // 加载中时也不显示（避免闪烁）
  if (loading && !initialLoadDone) {
    return null;
  }

  const hasAutoApplyTargets = hasExecutions
    ? executions.some(
        (e) => e.run_trigger?.target_workspace?.auto_apply && e.status === 'pending' && !e.temporarily_disabled
      )
    : triggers.some((t) => t.target_workspace?.auto_apply);

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <span className={styles.icon}>⚡</span>
          <h3 className={styles.title}>Run Triggers</h3>
          <span className={styles.count}>{displayData.length}</span>
        </div>
        <Link 
          to={`/workspaces/${workspaceId}?tab=settings&section=run-triggers`}
          className={styles.configLink}
        >
          Configure
        </Link>
      </div>

      <div className={styles.content}>
        {hasAutoApplyTargets && canModify && (
          <div className={styles.warningAlert}>
            <span className={styles.warningIcon}>⚠</span>
            <div className={styles.warningContent}>
              <div className={styles.warningTitle}>Auto Apply Warning</div>
              <div className={styles.warningText}>
                Some target workspaces have Auto Apply enabled. They will automatically 
                apply changes without manual confirmation when triggered.
              </div>
            </div>
          </div>
        )}

        <p className={styles.description}>
          {hasExecutions
            ? canModify
              ? 'The following workspaces will be triggered when this task completes successfully. You can temporarily disable triggers before the task finishes.'
              : 'The following workspaces were configured to be triggered by this task.'
            : 'The following workspaces will be triggered when this task\'s apply completes successfully.'}
        </p>

        <table className={styles.table}>
          <thead className={styles.tableHeader}>
            <tr>
              <th>Target Workspace</th>
              {hasExecutions ? (
                <>
                  <th>Status</th>
                  <th>Triggered Task</th>
                  {canModify && <th>Actions</th>}
                </>
              ) : (
                <>
                  <th>Trigger Condition</th>
                  <th>Status</th>
                </>
              )}
            </tr>
          </thead>
          <tbody className={styles.tableBody}>
            {hasExecutions ? (
              executions.map((execution) => (
                <tr key={execution.id}>
                  <td>
                    <div className={styles.targetCell}>
                      <span className={styles.linkIcon}>🔗</span>
                      <span className={styles.targetName}>
                        {execution.run_trigger?.target_workspace?.name || 
                         execution.run_trigger?.target_workspace_id || 
                         'Unknown'}
                      </span>
                      {execution.run_trigger?.target_workspace?.auto_apply && (
                        <span className={styles.autoApplyTag}>Auto Apply</span>
                      )}
                    </div>
                  </td>
                  <td>
                    <span className={`${styles.statusTag} ${getStatusClass(execution)}`}>
                      {getStatusText(execution)}
                    </span>
                  </td>
                  <td>
                    {execution.target_task_id ? (
                      <Link 
                        to={`/workspaces/${execution.run_trigger?.target_workspace_id}/tasks/${execution.target_task_id}`}
                        className={styles.taskLink}
                      >
                        Task #{execution.target_task_id}
                      </Link>
                    ) : (
                      <span className={styles.noTask}>-</span>
                    )}
                  </td>
                  {canModify && (
                    <td>
                      {execution.status === 'pending' && (
                        <button
                          className={`${styles.actionButton} ${
                            execution.temporarily_disabled 
                              ? styles.enableButton 
                              : styles.disableButton
                          }`}
                          onClick={() => handleToggle(execution.id, execution.temporarily_disabled)}
                        >
                          {execution.temporarily_disabled ? 'Enable' : 'Disable'}
                        </button>
                      )}
                    </td>
                  )}
                </tr>
              ))
            ) : (
              triggers.map((trigger) => (
                <tr key={trigger.id}>
                  <td>
                    <div className={styles.targetCell}>
                      <span className={styles.linkIcon}>🔗</span>
                      <span className={styles.targetName}>
                        {trigger.target_workspace?.name || trigger.target_workspace_id}
                      </span>
                      {trigger.target_workspace?.auto_apply && (
                        <span className={styles.autoApplyTag}>Auto Apply</span>
                      )}
                    </div>
                  </td>
                  <td>
                    <span className={styles.conditionTag}>
                      {trigger.trigger_condition === 'apply_success' 
                        ? 'After Apply Success' 
                        : trigger.trigger_condition}
                    </span>
                  </td>
                  <td>
                    <span className={`${styles.statusTag} ${styles.statusPending}`}>
                      Will Trigger
                    </span>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};

export default TaskTriggerExecutions;
