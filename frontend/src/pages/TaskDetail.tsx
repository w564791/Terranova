import React, { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { Tag } from 'antd';
import ConfirmDialog from '../components/ConfirmDialog';
import { WarningOutlined } from '@ant-design/icons';
import NewRunDialog from '../components/NewRunDialog';
import TaskComments from '../components/TaskComments';
import CommentInput from '../components/CommentInput';
import AIErrorAnalysis from '../components/AIErrorAnalysis';
import TaskTimeline from '../components/TaskTimeline';
import SmartLogViewer from '../components/SmartLogViewer';
import { useNotificationContext } from '../contexts/NotificationContext';
import api from '../services/api';
import { getTaskStatusLabel } from '../utils/taskStatus';
import styles from './TaskDetail.module.css';

interface Task {
  id: number;
  workspace_id: string;
  task_type: string;
  status: string;
  stage?: string;
  description?: string;
  created_at: string;
  created_by?: number;
  created_by_username?: string;
  started_at?: string;
  completed_at?: string;
  duration?: number;
  error_message?: string;
  plan_json?: any;
  plan_output?: string;
  apply_output?: string;
  changes_add?: number;
  changes_change?: number;
  changes_destroy?: number;
  snapshot_id?: string;
  apply_description?: string;
  agent_id?: number;
  agent_name?: string;
  // Apply confirmation fields
  apply_confirmed_by?: string;
  apply_confirmed_at?: string;
}

const TaskDetail: React.FC = () => {
  const { workspaceId, taskId } = useParams<{ workspaceId: string; taskId: string }>();
  const navigate = useNavigate();
  const { showSuccess, showError } = useNotificationContext();
  const [task, setTask] = useState<Task | null>(null);
  const [workspace, setWorkspace] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showNewRunDialog, setShowNewRunDialog] = useState(false);
  const [showCommentInput, setShowCommentInput] = useState(false);
  const [commentAction, setCommentAction] = useState<'comment' | 'confirm_apply' | 'cancel' | 'cancel_previous' | 'override'>('comment');
  const [submittingAction, setSubmittingAction] = useState(false);
  const [commentsKey, setCommentsKey] = useState(0);
  const [commentCount, setCommentCount] = useState(0);

  const [canCancelTask, setCanCancelTask] = useState(false);
  const [canConfirmApply, setCanConfirmApply] = useState(false);
  const [runTaskResults, setRunTaskResults] = useState<any[]>([]);
  const [needsOverride, setNeedsOverride] = useState(false);
  const [triggerExecutions, setTriggerExecutions] = useState<any[]>([]);
  const [viewMode, setViewMode] = useState<'structured' | 'classic'>(() => {
    // 从 URL 参数读取视图模式
    const params = new URLSearchParams(window.location.search);
    return (params.get('view') as 'structured' | 'classic') || 'structured';
  });
  const [logViewMode, setLogViewMode] = useState<'plan' | 'apply'>('plan');

  // taskId 变化时重置状态（如从 new run 导航到新任务）
  useEffect(() => {
    setTask(null);
    setLoading(true);
    setError(null);
    setShowCommentInput(false);
    setConfirmModalVisible(false);
  }, [taskId]);

  useEffect(() => {
    fetchTask();
    fetchWorkspace();
    fetchCommentCount();
    checkPermissions();
    fetchRunTaskResults();
    fetchTriggerExecutions();
    
    const interval = setInterval(() => {
      if (task && (task.status === 'running' || task.status === 'pending' || task.status === 'plan_completed' || task.status === 'apply_pending' || task.status === 'decision_required')) {
        fetchTask();
        fetchRunTaskResults();
        fetchTriggerExecutions();
      }
    }, 3000);
    
    return () => clearInterval(interval);
  }, [workspaceId, taskId, task?.status]);

  const fetchTriggerExecutions = async () => {
    try {
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}/trigger-executions`);
      setTriggerExecutions(data.trigger_executions || []);
    } catch (err) {
      console.error('Failed to fetch trigger executions:', err);
    }
  };

  const handleToggleTrigger = async (executionId: number, disabled: boolean) => {
    try {
      await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/trigger-executions/${executionId}/toggle`, {
        disabled
      });
      showSuccess(`Trigger ${disabled ? 'disabled' : 'enabled'}`);
      fetchTriggerExecutions();
    } catch (err: any) {
      showError('Failed to toggle trigger');
    }
  };

  const checkPermissions = async () => {
    try {
      const cancelCheck: any = await api.post('/iam/permissions/check', {
        resource_type: 'WORKSPACE_EXECUTION',
        scope_type: 'WORKSPACE',
        scope_id: workspaceId!,
        required_level: 'ADMIN'
      });
      setCanCancelTask(cancelCheck?.is_allowed || false);

      const confirmCheck: any = await api.post('/iam/permissions/check', {
        resource_type: 'WORKSPACE_EXECUTION',
        scope_type: 'WORKSPACE',
        scope_id: workspaceId!,
        required_level: 'ADMIN'
      });
      setCanConfirmApply(confirmCheck?.is_allowed || false);
    } catch (err) {
      console.error('Failed to check permissions:', err);
      setCanCancelTask(false);
      setCanConfirmApply(false);
    }
  };

  const isStateSaveFailure = task?.error_message?.includes('state save failed');

  const extractBackupPath = (errorMessage?: string): string | null => {
    if (!errorMessage) return null;
    const match = errorMessage.match(/backup at: (.+)$/);
    return match ? match[1].trim() : null;
  };

  const backupPath = extractBackupPath(task?.error_message);

  const handleRetryStateSave = async () => {
    if (!confirm('确定要重试State保存吗？')) {
      return;
    }

    try {
      await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/retry-state-save`);
      showSuccess('State保存成功，workspace已解锁');
      fetchTask();
    } catch (err: any) {
      const message = err.response?.data?.error || err.message || 'Failed to retry';
      showError(`重试失败: ${message}`);
    }
  };

  const handleDownloadStateBackup = () => {
    window.open(
      `http://localhost:8080/api/v1/workspaces/${workspaceId}/tasks/${taskId}/state-backup`,
      '_blank'
    );
  };

  const fetchTask = async () => {
    try {
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}`);
      let taskData = data.task || data;
      
      if (taskData.plan_json && 
          (taskData.changes_add === undefined || taskData.changes_add === null)) {
        const changes = parsePlanChanges(taskData.plan_json);
        taskData = {
          ...taskData,
          changes_add: changes.add,
          changes_change: changes.change,
          changes_destroy: changes.destroy
        };
      }
      
      setTask(taskData);
    } catch (err: any) {
      setError(err.message || 'Failed to fetch task');
    } finally {
      setLoading(false);
    }
  };

  const fetchCommentCount = async () => {
    try {
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}/comments`);
      setCommentCount(data.total || 0);
    } catch (err) {
      console.error('Failed to fetch comment count:', err);
    }
  };

  const fetchRunTaskResults = async () => {
    try {
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}/run-task-results`);
      const results = data.run_task_results || [];
      setRunTaskResults(results);
      
      // Check if there are any Advisory failures that need override
      const hasAdvisoryFailure = results.some((result: any) => {
        const isFailed = result.status === 'failed' || result.status === 'error' || result.status === 'timeout';
        const isAdvisory = result.enforcement_level === 'advisory';
        return isFailed && isAdvisory;
      });
      
      // Check if there are any Mandatory failures (task should be blocked)
      const hasMandatoryFailure = results.some((result: any) => {
        const isFailed = result.status === 'failed' || result.status === 'error' || result.status === 'timeout';
        const isMandatory = result.enforcement_level === 'mandatory';
        return isFailed && isMandatory;
      });
      
      // Check if all failures have been overridden
      const allOverridden = results.every((result: any) => {
        const isFailed = result.status === 'failed' || result.status === 'error' || result.status === 'timeout';
        if (!isFailed) return true;
        return result.status === 'overridden';
      });
      
      // Only need override if there's an advisory failure, no mandatory failure, and not all overridden
      setNeedsOverride(hasAdvisoryFailure && !hasMandatoryFailure && !allOverridden);
    } catch (err) {
      console.error('Failed to fetch run task results:', err);
    }
  };

  const parsePlanChanges = (planJSON: any): { add: number; change: number; destroy: number } => {
    let add = 0, change = 0, destroy = 0;
    
    try {
      if (planJSON && planJSON.resource_changes) {
        for (const rc of planJSON.resource_changes) {
          if (rc.change && rc.change.actions) {
            for (const action of rc.change.actions) {
              if (action === 'create') add++;
              else if (action === 'update') change++;
              else if (action === 'delete') destroy++;
            }
          }
        }
      }
    } catch (err) {
      console.error('Failed to parse plan changes:', err);
    }
    
    return { add, change, destroy };
  };

  const fetchWorkspace = async () => {
    try {
      const data: any = await api.get(`/workspaces/${workspaceId}`);
      setWorkspace(data.data || data);
    } catch (err: any) {
      console.error('Failed to fetch workspace:', err);
    }
  };

  const handleStageChange = useCallback((stage: string) => {
    console.log('Stage changed:', stage);
    // 在 Classic 模式下，自动切换到对应的日志视图
    if (stage === 'apply' && logViewMode !== 'apply') {
      setLogViewMode('apply');
    } else if ((stage === 'plan' || stage === 'init' || stage === 'fetching') && logViewMode !== 'plan') {
      setLogViewMode('plan');
    }
  }, [logViewMode]);

  // 更新 URL 参数
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    params.set('view', viewMode);
    const newUrl = `${window.location.pathname}?${params.toString()}`;
    window.history.replaceState({}, '', newUrl);
  }, [viewMode]);

  const formatRelativeTime = (dateString: string | null) => {
    if (!dateString) return '从未';
    if (dateString.startsWith('0001-01-01')) return '从未';
    
    const date = new Date(dateString);
    const now = new Date();
    
    if (isNaN(date.getTime())) return '无效日期';
    
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);

    if (diffMins < 5) return 'just now';
    if (diffMins < 60) return `${diffMins} minutes ago`;
    if (diffHours < 24) return `${diffHours} hours ago`;
    return date.toLocaleString();
  };

  const proceedWithAction = (action: string) => {
    setCommentAction(action as any);
    setShowCommentInput(true);
    setTimeout(() => {
      window.scrollTo({ top: document.documentElement.scrollHeight, behavior: 'smooth' });
    }, 100);
  };

  const [confirmModalVisible, setConfirmModalVisible] = useState(false);
  const [confirmModalTitle, setConfirmModalTitle] = useState('');
  const [confirmModalContent, setConfirmModalContent] = useState('');

  const handleActionWithComment = async (action: 'comment' | 'confirm_apply' | 'cancel' | 'cancel_previous' | 'override') => {
    // Confirm Apply 时检查 plan summary 状态
    if (action === 'confirm_apply' && task && workspaceId && !confirmModalVisible) {
      try {
        const summary: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}/plan-summary`);
        const data = summary?.data || summary;
        if (data) {
          if (data.status === 'running' || data.status === 'pending') {
            setConfirmModalTitle('AI 分析尚未完成');
            setConfirmModalContent('AI 变更影响分析正在进行中，建议等待分析完成后再确认 Apply。是否仍要继续？');
            setConfirmModalVisible(true);
            return;
          } else if (data.requires_confirmation && !data.user_decision_code) {
            setConfirmModalTitle('AI 风险决策未确认');
            setConfirmModalContent('AI 判断本次变更存在风险，建议先在 Plan Summary 中提交风险决策确认。是否仍要继续 Apply？');
            setConfirmModalVisible(true);
            return;
          }
        }
      } catch {
        // summary 不存在（404）或请求失败，不阻塞
      }
    }

    proceedWithAction(action);
  };

  const handleCommentSubmit = async (comment: string) => {
    try {
      setSubmittingAction(true);

      // For override action, the backend already creates a comment, so skip the separate comment API call
      if (commentAction === 'override') {
        await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/override-run-tasks`, {
          comment: comment || undefined
        });
      } else {
        // Add the comment only if non-empty
        if (comment) {
          await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/comments`, {
            comment,
            action_type: commentAction
          });
        }

        // Then perform the action
        if (commentAction === 'confirm_apply') {
          await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/confirm-apply`, {
            apply_description: comment || undefined
          });
        } else if (commentAction === 'cancel') {
          await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/cancel`);
        } else if (commentAction === 'cancel_previous') {
          const res: any = await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/cancel-previous`);
          const cancelled = res?.cancelled_count ?? 0;
          showSuccess(`已取消 ${cancelled} 个之前的任务`);
        }
      }

      if (commentAction === 'confirm_apply') {
        showSuccess('Apply 已确认，任务开始执行');
      } else if (commentAction === 'cancel') {
        showSuccess('任务已取消');
      } else if (commentAction === 'override') {
        showSuccess('已覆盖失败的 Run Tasks');
      }

      setShowCommentInput(false);
      
      setTimeout(() => {
        setCommentsKey(prev => prev + 1);
        fetchCommentCount();
        
        if (commentAction !== 'comment') {
          fetchTask();
          fetchRunTaskResults();
        }
      }, 300);
    } catch (err: any) {
      const message = err.response?.data?.error || err.message || 'Failed to perform action';
      showError(`操作失败: ${message}`);
    } finally {
      setSubmittingAction(false);
    }
  };

  const getCommentPlaceholder = () => {
    switch (commentAction) {
      case 'confirm_apply':
        return 'Enter a comment to confirm apply...';
      case 'cancel':
        return 'Enter a comment to cancel this task...';
      case 'cancel_previous':
        return 'Enter a comment to cancel previous tasks...';
      case 'override':
        return 'Enter a comment to override failed run tasks...';
      default:
        return 'Add your comment here...';
    }
  };

  const getCommentSubmitLabel = () => {
    switch (commentAction) {
      case 'confirm_apply':
        return 'Confirm Apply';
      case 'cancel':
        return 'Cancel Task';
      case 'cancel_previous':
        return 'Cancel Previous';
      case 'override':
        return 'Override & Continue';
      default:
        return 'Add Comment';
    }
  };

  // 渲染主内容区域
  const renderMainContent = () => {
    if (loading) {
      return (
        <div className={styles.loadingContainer}>
          <div className={styles.loading}>
            <div className={styles.spinner}></div>
            <span>加载中...</span>
          </div>
        </div>
      );
    }

    if (error || !task) {
      return (
        <div className={styles.errorContainer}>
          <div className={styles.error}>
            <span>{error || '任务不存在'}</span>
            <button onClick={() => navigate(`/workspaces/${workspaceId}`)} className={styles.backBtn}>
              返回Workspace
            </button>
          </div>
        </div>
      );
    }

    return (
      <>
        {/* Header */}
        {workspace && (
          <div className={styles.globalHeader}>
            <div className={styles.globalHeaderLeft}>
              <h1 className={styles.globalTitle}>{workspace.name}</h1>
              <div className={styles.globalMeta}>
                <span 
                  className={styles.metaItem}
                  style={{ cursor: 'pointer' }}
                  onClick={() => {
                    navigator.clipboard.writeText(workspace.workspace_id);
                    showSuccess('Workspace ID copied to clipboard');
                  }}
                  title="Click to copy workspace ID"
                >
                  ID: {workspace.workspace_id}
                </span>
                <span className={styles.metaItem}>
                  Terraform {workspace.terraform_version || 'latest'}
                </span>
                <span className={styles.metaItem}>
                  Updated {formatRelativeTime(workspace.updated_at)}
                </span>
              </div>
            </div>
            <div className={styles.globalHeaderRight}>
              <button 
                className={styles.newRunButton}
                onClick={() => setShowNewRunDialog(true)}
              >
                + New run
              </button>
            </div>
          </div>
        )}

        {/* Task Title with Status */}
        <div className={styles.taskTitleSection}>
          <div className={styles.taskTitleRow}>
            <h2 className={styles.taskTitle}>
              {task.description || `${task.task_type.toUpperCase()} #${task.id}`}
            </h2>
            <div className={styles.taskStatusBadge}>
              {(() => {
                const label = getTaskStatusLabel(
                  {
                    status: task.status,
                    task_type: task.task_type,
                    stage: task.stage,
                    apply_confirmed_by: task.apply_confirmed_by,
                    apply_confirmed_at: task.apply_confirmed_at,
                  },
                  { detailedRunning: true }
                );
                const tagClass =
                  task.status === 'pending' || task.status === 'waiting'
                    ? styles.statusTagPending
                    : task.status === 'running'
                      ? styles.statusTagRunning
                      : task.status === 'plan_completed' ||
                          task.status === 'apply_pending' ||
                          task.status === 'decision_required' ||
                          task.status === 'requires_approval'
                        ? styles.statusTagWarning
                        : task.status === 'failed'
                          ? styles.statusTagError
                          : task.status === 'cancelled'
                            ? styles.statusTagCancelled
                            : styles.statusTagSuccess;
                return <span className={tagClass}>{label}</span>;
              })()}
            </div>
          </div>
        </div>

        {/* Summary Stats with View Mode Toggle */}
        <div className={styles.statsRow}>
          <div className={styles.inlineStats}>
            <div className={styles.statItem}>
              <span className={styles.statNum}>{runTaskResults.length > 0 ? runTaskResults.length : '0'}</span>
              <span className={styles.statLbl}>Run Tasks</span>
            </div>
            <span className={styles.statSep}>|</span>
            <div className={styles.statItem}>
              <span className={styles.statNum}>{task.duration ? `${Math.floor(task.duration / 60)}m` : '<1m'}</span>
              <span className={styles.statLbl}>Duration</span>
            </div>
            <span className={styles.statSep}>|</span>
            <div className={styles.statItem}>
              <span className={styles.statNum}>
                <span className={styles.changeAdd}>+{task.changes_add || 0}</span>
                {' '}
                <span className={styles.changeModify}>~{task.changes_change || 0}</span>
                {' '}
                <span className={styles.changeDestroy}>-{task.changes_destroy || 0}</span>
              </span>
              <span className={styles.statLbl}>Resources</span>
            </div>
          </div>
          <div className={styles.viewModeToggle}>
            <button
              className={`${styles.viewModeBtn} ${viewMode === 'structured' ? styles.viewModeBtnActive : ''}`}
              onClick={() => setViewMode('structured')}
            >
              Structured
            </button>
            <button
              className={`${styles.viewModeBtn} ${viewMode === 'classic' ? styles.viewModeBtnActive : ''}`}
              onClick={() => setViewMode('classic')}
            >
              Classic
            </button>
          </div>
        </div>

        {/* Content based on view mode */}
        {viewMode === 'structured' ? (
          <TaskTimeline
            task={task}
            workspaceId={workspaceId!}
            workspace={workspace}
            onStageChange={handleStageChange}
            triggerExecutions={triggerExecutions}
          />
        ) : (
          <div className={styles.classicView}>
            {/* Classic View - Real-time Logs */}
            {task.task_type === 'plan_and_apply' && (
              <div className={styles.logTabs}>
                <button
                  className={`${styles.logTab} ${logViewMode === 'plan' ? styles.logTabActive : ''}`}
                  onClick={() => setLogViewMode('plan')}
                >
                  Plan
                </button>
                <button
                  className={`${styles.logTab} ${logViewMode === 'apply' ? styles.logTabActive : ''}`}
                  onClick={() => setLogViewMode('apply')}
                >
                  Apply
                </button>
              </div>
            )}
            <div className={styles.logViewerWrapper}>
              <SmartLogViewer
                taskId={task.id}
                viewMode={logViewMode}
                onStageChange={handleStageChange}
                currentTaskStage={task.stage}
              />
            </div>
          </div>
        )}

        {/* State Save Failure Actions */}
        {isStateSaveFailure && (
          <div className={styles.errorActions}>
            <button
              className={styles.retryButton}
              onClick={handleRetryStateSave}
            >
              Retry State Save
            </button>
            <button
              className={styles.downloadButton}
              onClick={handleDownloadStateBackup}
            >
              Download State Backup
            </button>
            {backupPath && (
              <div className={styles.backupPath}>
                Backup: <code>{backupPath}</code>
              </div>
            )}
          </div>
        )}

        {/* AI Error Analysis - Now shown inside TaskTimeline for structured view */}
        {/* Only show here for classic view */}
        {/* 安全修复：只传入 task_id，其他信息从后端数据库获取，防止 prompt injection 攻击 */}
        {viewMode === 'classic' && task.error_message && task.status !== 'cancelled' && (
          <div className={styles.aiAnalysisSection}>
            <AIErrorAnalysis
              workspaceId={workspaceId!}
              taskId={parseInt(taskId!)}
            />
          </div>
        )}

        {/* Comments Section */}
        {commentCount > 0 && (
          <div className={styles.commentsWrapper}>
            <TaskComments 
              key={commentsKey}
              workspaceId={workspaceId!} 
              taskId={parseInt(taskId!)} 
            />
          </div>
        )}

        {/* Action Buttons */}
        <div className={styles.bottomSection}>
          {!showCommentInput && (
            <div className={styles.actionButtons}>
              <button
                className={styles.addCommentButton}
                onClick={() => handleActionWithComment('comment')}
              >
                Add Comment
              </button>

              {task.status === 'pending' && canCancelTask && (
                <button
                  className={styles.cancelPreviousButton}
                  onClick={() => handleActionWithComment('cancel_previous')}
                >
                  Cancel Previous
                </button>
              )}
              
              {(task.status !== 'success' && task.status !== 'applied' && task.status !== 'failed' && task.status !== 'cancelled' && task.status !== 'planned_and_finished') && canCancelTask && (
                <button
                  className={styles.cancelButton}
                  onClick={() => handleActionWithComment('cancel')}
                >
                  Cancel
                </button>
              )}
              
              {/* Override button for Advisory Run Task failures */}
              {needsOverride && (task.status === 'apply_pending' || task.status === 'plan_completed') && canConfirmApply && (
                <button
                  className={styles.overrideButton}
                  onClick={() => handleActionWithComment('override')}
                >
                  Override Run Tasks
                </button>
              )}

              {/* Confirm Apply button - only show if no override needed */}
              {!needsOverride && (task.status === 'apply_pending' || task.status === 'plan_completed') && task.task_type === 'plan_and_apply' && canConfirmApply && (
                <button
                  className={styles.confirmApplyButton}
                  onClick={() => handleActionWithComment('confirm_apply')}
                >
                  Confirm Apply
                </button>
              )}
            </div>
          )}

          {showCommentInput && (
            <div className={styles.commentInputWrapper}>
              <div className={styles.commentInputSection}>
                <h3 className={styles.commentInputTitle}>
                  {commentAction === 'comment' && 'Add Comment'}
                  {commentAction === 'confirm_apply' && 'Confirm Apply - Add Comment'}
                  {commentAction === 'cancel' && 'Cancel Task - Add Comment'}
                  {commentAction === 'cancel_previous' && 'Cancel Previous Tasks - Add Comment'}
                  {commentAction === 'override' && 'Override Run Tasks - Add Comment'}
                </h3>
                <CommentInput
                  onSubmit={handleCommentSubmit}
                  onCancel={() => setShowCommentInput(false)}
                  placeholder={getCommentPlaceholder()}
                  submitLabel={getCommentSubmitLabel()}
                  isSubmitting={submittingAction}
                  submitTone={
                    commentAction === 'cancel' || commentAction === 'cancel_previous'
                      ? 'danger'
                      : 'brand'
                  }
                />
              </div>
            </div>
          )}
        </div>
      </>
    );
  };

  return (
    <div className={styles.taskPage}>
      <div className={styles.mainContent}>
        {renderMainContent()}
      </div>

      <NewRunDialog
        isOpen={showNewRunDialog}
        workspaceId={workspaceId!}
        onClose={() => setShowNewRunDialog(false)}
        onSuccess={() => {
          setShowNewRunDialog(false);
        }}
      />

      <ConfirmDialog
        isOpen={confirmModalVisible}
        title={confirmModalTitle}
        message={confirmModalContent}
        confirmText="继续 Apply"
        cancelText="返回"
        type="warning"
        onConfirm={() => {
          setConfirmModalVisible(false);
          proceedWithAction('confirm_apply');
        }}
        onCancel={() => setConfirmModalVisible(false)}
      />
    </div>
  );
};

export default TaskDetail;
