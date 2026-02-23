import React, { useState } from 'react';
import styles from './TaskTimeline.module.css';
import RunTaskResults from './RunTaskResults';
import StructuredRunOutput from './StructuredRunOutput';
import SmartLogViewer from './SmartLogViewer';
import AIErrorAnalysis from './AIErrorAnalysis';
import { parseBackendTime } from '../utils/time';

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
  // Apply confirmation fields
  apply_confirmed_by?: string;
  apply_confirmed_at?: string;
}

interface Props {
  task: Task;
  workspaceId: string;
  workspace?: any;
  onStageChange?: (stage: string) => void;
  showAIAnalysis?: boolean;
}

const TaskTimeline: React.FC<Props> = ({ task, workspaceId, workspace, onStageChange, showAIAnalysis = true }) => {
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['plan', 'run_tasks', 'plan_error', 'apply_error']));
  
  // 记录上一次的 apply_confirmed_by 状态，用于检测变化
  const [prevApplyConfirmed, setPrevApplyConfirmed] = useState<boolean>(false);
  
  // 当用户 confirm apply 后，自动折叠 Plan 卡片并展开 Apply 卡片
  React.useEffect(() => {
    const isCurrentlyConfirmed = !!(task.apply_confirmed_by || task.apply_confirmed_at);
    
    // 检测从未确认变为已确认的状态变化
    if (isCurrentlyConfirmed && !prevApplyConfirmed) {
      console.log('[TaskTimeline] Apply confirmed, collapsing Plan and expanding Apply');
      setExpandedSections(prev => {
        const next = new Set(prev);
        next.delete('plan'); // 折叠 Plan 卡片
        next.add('apply');   // 展开 Apply 卡片
        return next;
      });
    }
    
    setPrevApplyConfirmed(isCurrentlyConfirmed);
  }, [task.apply_confirmed_by, task.apply_confirmed_at, prevApplyConfirmed]);

  const toggleSection = (section: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      if (next.has(section)) {
        next.delete(section);
      } else {
        next.add(section);
      }
      return next;
    });
  };

  const formatRelativeTime = (dateString?: string) => {
    if (!dateString) return '';
    const date = parseBackendTime(dateString);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins} minutes ago`;
    if (diffHours < 24) return `${diffHours} hours ago`;
    return date.toLocaleString();
  };

  // Helper: Check if apply has been confirmed by user
  const isApplyConfirmed = () => {
    return !!(task.apply_confirmed_by || task.apply_confirmed_at);
  };

  // Helper: Check if task is in apply-related stage
  const isApplyStage = () => {
    const applyStages = ['applying', 'restoring_plan', 'pre_apply_run_tasks', 'saving_state', 'post_apply', 'applied', 'apply_pending'];
    return task.stage ? applyStages.includes(task.stage) : false;
  };

  // Helper: Check if task is in plan-related stage
  const isPlanStage = () => {
    const planStages = ['pending', 'init', 'fetching', 'planning', 'saving_plan', 'post_plan_run_tasks'];
    return task.stage ? planStages.includes(task.stage) : true; // Default to plan stage if no stage
  };

  // Helper: Check if task failed due to Run Task (not plan/apply itself)
  const isRunTaskFailure = () => {
    return task.stage === 'post_plan_run_tasks' || task.stage === 'pre_apply_run_tasks';
  };

  // Helper: Check if task was cancelled (either by status or error message)
  const isCancelledTask = () => {
    if (task.status === 'cancelled') return true;
    // Also check error_message for "cancelled" - handles Agent mode edge case
    if (task.error_message?.toLowerCase().includes('cancelled')) return true;
    return false;
  };

  // Determine plan status
  // Plan is considered complete when:
  // 1. status is success/plan_completed/apply_pending/applied
  // 2. apply has been confirmed (apply_confirmed_by is set)
  // 3. task is running but in apply stage (not plan stage)
  const getPlanStatus = () => {
    if (task.status === 'pending') return 'pending';
    
    // Check for cancelled task (including error_message check for Agent mode)
    if (isCancelledTask()) return 'cancelled';
    
    // If apply has been confirmed, plan is definitely complete
    if (isApplyConfirmed()) return 'passed';
    
    // Check for planning stage (database stores 'planning', not 'plan')
    if (task.status === 'running') {
      // If in plan-related stage, show as running
      if (isPlanStage() && !isApplyStage()) return 'running';
      // If in apply-related stage, plan is complete
      if (isApplyStage()) return 'passed';
      // Default: if stage is unclear but running, check if we have plan data
      // If changes_add/change/destroy are set, plan is likely complete
      if (task.changes_add !== undefined || task.changes_change !== undefined || task.changes_destroy !== undefined) {
        return 'passed';
      }
      return 'running';
    }
    
    if (task.status === 'failed') {
      // Run Task failure: plan itself succeeded, Run Task blocked execution
      if (isRunTaskFailure()) return 'passed';
      // If failed during plan stage
      if (isPlanStage() && !isApplyStage()) return 'failed';
      // If failed during apply stage, plan was successful
      if (isApplyStage()) return 'passed';
      // If no stage info, check task type
      if (task.task_type === 'plan') return 'failed';
      // Default: assume plan failed if no other info
      return 'failed';
    }
    
    if (['success', 'plan_completed', 'apply_pending', 'applied', 'planned_and_finished'].includes(task.status)) return 'passed';
    
    return 'pending';
  };

  // Determine apply status
  // Apply is shown when:
  // 1. task_type is plan_and_apply or apply
  // 2. status indicates apply phase (apply_pending, applied, or running with apply stage)
  const getApplyStatus = () => {
    if (task.task_type !== 'plan_and_apply' && task.task_type !== 'apply') return null;
    
    // If planned_and_finished, apply will not run (no changes)
    if (task.status === 'planned_and_finished') return 'skipped';
    
    if (task.status === 'applied') return 'passed';
    
    // If apply has been confirmed and task is running, show apply as running
    if (isApplyConfirmed() && task.status === 'running') return 'running';
    
    // Check for applying stage (database stores 'applying', not 'apply')
    if (task.status === 'running') {
      if (isApplyStage()) return 'running';
      // If running but not in apply stage, apply is pending
      return null; // Don't show apply card yet
    }
    
    if (task.status === 'failed') {
      // Run Task failure: apply itself never started, don't show apply failed
      if (isRunTaskFailure()) return null;
      if (isApplyStage()) return 'failed';
      // If failed during plan, don't show apply card
      return null;
    }
    
    if (task.status === 'apply_pending' || task.status === 'plan_completed') return 'pending';
    if (task.status === 'cancelled') {
      // Only show cancelled apply if we were in apply phase
      if (isApplyStage() || isApplyConfirmed()) return 'cancelled';
      return null;
    }
    
    return null;
  };

  const planStatus = getPlanStatus();
  const applyStatus = getApplyStatus();

  const statusConfig: Record<string, { icon: string; color: string; bgColor: string }> = {
    pending: { icon: '○', color: '#6b7280', bgColor: '#f3f4f6' },
    running: { icon: '◐', color: '#3b82f6', bgColor: '#dbeafe' },
    passed: { icon: '✓', color: '#10b981', bgColor: '#d1fae5' },
    failed: { icon: '✗', color: '#ef4444', bgColor: '#fee2e2' },
    cancelled: { icon: '⊘', color: '#6b7280', bgColor: '#f3f4f6' },
    skipped: { icon: '⊘', color: '#6b7280', bgColor: '#f3f4f6' },
  };

  return (
    <div className={styles.timeline}>
      {/* Trigger Info Card */}
      <div className={styles.timelineCard}>
        <div 
          className={`${styles.cardHeader} ${expandedSections.has('trigger') ? styles.cardHeaderExpanded : ''}`}
          onClick={() => toggleSection('trigger')}
        >
          <div className={styles.triggerInfo}>
            <span className={styles.triggerAvatar}>👤</span>
            <span className={styles.triggerText}>
              <strong>{task.created_by_username || 'System'}</strong> triggered a{' '}
              <strong>{task.task_type}</strong> {formatRelativeTime(task.created_at)}
            </span>
          </div>
          <span className={styles.expandButton}>
            Run Details {expandedSections.has('trigger') ? '∧' : '∨'}
          </span>
        </div>
        {expandedSections.has('trigger') && (
          <div className={styles.cardContent}>
            <div className={styles.detailsGrid}>
              <div className={styles.detailItem}>
                <span className={styles.detailLabel}>Task ID</span>
                <span className={styles.detailValue}>#{task.id}</span>
              </div>
              <div className={styles.detailItem}>
                <span className={styles.detailLabel}>Type</span>
                <span className={styles.detailValue}>{task.task_type}</span>
              </div>
              <div className={styles.detailItem}>
                <span className={styles.detailLabel}>Status</span>
                <span className={styles.detailValue}>{task.status}</span>
              </div>
              {task.duration && (
                <div className={styles.detailItem}>
                  <span className={styles.detailLabel}>Duration</span>
                  <span className={styles.detailValue}>{task.duration}s</span>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Plan Card */}
      <div className={`${styles.timelineCard} ${styles[`card${planStatus}`]}`}>
        <div 
          className={`${styles.cardHeader} ${expandedSections.has('plan') ? styles.cardHeaderExpanded : ''}`}
          onClick={() => toggleSection('plan')}
        >
          <div className={styles.cardTitle}>
            <span
              className={styles.statusIcon}
              style={{
                color: statusConfig[planStatus].color,
                backgroundColor: statusConfig[planStatus].bgColor,
              }}
            >
              {statusConfig[planStatus].icon}
            </span>
            <span className={styles.titleText}>
              {planStatus === 'passed' && 'Plan finished'}
              {planStatus === 'running' && 'Planning...'}
              {planStatus === 'pending' && 'Plan pending'}
              {planStatus === 'failed' && 'Plan failed'}
              {planStatus === 'cancelled' && 'Plan cancelled'}
            </span>
            <span className={styles.cardTime}>
              {task.started_at && formatRelativeTime(task.started_at)}
            </span>
          </div>
          <div className={styles.cardSummary}>
            {(task.changes_add !== undefined || task.changes_change !== undefined || task.changes_destroy !== undefined) && (
              <span className={styles.resourceChanges}>
                Resources: <span className={styles.changeAdd}>{task.changes_add || 0}</span> to add,{' '}
                <span className={styles.changeModify}>{task.changes_change || 0}</span> to change,{' '}
                <span className={styles.changeDestroy}>{task.changes_destroy || 0}</span> to destroy
              </span>
            )}
            <span className={styles.expandButton}>
              {expandedSections.has('plan') ? '∧' : '∨'}
            </span>
          </div>
        </div>
        {expandedSections.has('plan') && (
          <div className={styles.cardContent}>
            {/* Plan 阶段错误信息（可折叠） */}
            {planStatus === 'failed' && task.error_message && (
              <>
                <div className={styles.inlineError}>
                  <div 
                    className={styles.inlineErrorHeader}
                    onClick={(e) => {
                      e.stopPropagation();
                      toggleSection('plan_error');
                    }}
                  >
                    <span className={styles.inlineErrorIcon}>✗</span>
                    <span className={styles.inlineErrorTitle}>Error</span>
                    <span className={styles.inlineErrorToggle}>
                      {expandedSections.has('plan_error') ? '∧' : '∨'}
                    </span>
                  </div>
                  {expandedSections.has('plan_error') && (
                    <pre className={styles.inlineErrorMessage}>{task.error_message}</pre>
                  )}
                </div>
                {/* AI 错误分析 - 安全修复：只传入 task_id，其他信息从后端数据库获取 */}
                {showAIAnalysis && (
                  <div className={styles.aiAnalysisWrapper}>
                    <AIErrorAnalysis
                      workspaceId={workspaceId}
                      taskId={task.id}
                    />
                  </div>
                )}
              </>
            )}
            <StructuredRunOutput
              task={task}
              workspaceId={workspaceId}
              workspace={workspace}
              mode="plan"
            />
          </div>
        )}
      </div>

      {/* Post-plan Run Tasks */}
      <RunTaskResults
        workspaceId={workspaceId}
        taskId={task.id}
        stage="post_plan"
      />

      {/* Apply Card (if applicable) */}
      {applyStatus && (
        <div className={`${styles.timelineCard} ${styles[`card${applyStatus}`]}`}>
          <div 
            className={`${styles.cardHeader} ${expandedSections.has('apply') ? styles.cardHeaderExpanded : ''}`}
            onClick={() => toggleSection('apply')}
          >
            <div className={styles.cardTitle}>
              <span
                className={styles.statusIcon}
                style={{
                  color: statusConfig[applyStatus].color,
                  backgroundColor: statusConfig[applyStatus].bgColor,
                }}
              >
                {statusConfig[applyStatus].icon}
              </span>
              <span className={styles.titleText}>
                {applyStatus === 'passed' && 'Apply finished'}
                {applyStatus === 'running' && 'Applying...'}
                {applyStatus === 'pending' && 'Apply pending'}
                {applyStatus === 'failed' && 'Apply failed'}
                {applyStatus === 'cancelled' && 'Apply cancelled'}
                {applyStatus === 'skipped' && 'Apply will not run'}
              </span>
              <span className={styles.cardTime}>
                {task.completed_at && applyStatus === 'passed' && formatRelativeTime(task.completed_at)}
              </span>
            </div>
            <div className={styles.cardSummary}>
              {applyStatus === 'passed' && (
                <span className={styles.resourceChanges}>
                  Resources: <span className={styles.changeAdd}>{task.changes_add || 0}</span> added,{' '}
                  <span className={styles.changeModify}>{task.changes_change || 0}</span> changed,{' '}
                  <span className={styles.changeDestroy}>{task.changes_destroy || 0}</span> destroyed
                </span>
              )}
              {applyStatus === 'skipped' && (
                <span className={styles.resourceChanges}>
                  No changes to apply
                </span>
              )}
              <span className={styles.expandButton}>
                {expandedSections.has('apply') ? '∧' : '∨'}
              </span>
            </div>
          </div>
          {expandedSections.has('apply') && (
            <div className={styles.cardContent}>
              {/* Apply 阶段错误信息（可折叠） */}
              {applyStatus === 'failed' && task.error_message && (
                <>
                  <div className={styles.inlineError}>
                    <div 
                      className={styles.inlineErrorHeader}
                      onClick={(e) => {
                        e.stopPropagation();
                        toggleSection('apply_error');
                      }}
                    >
                      <span className={styles.inlineErrorIcon}>✗</span>
                      <span className={styles.inlineErrorTitle}>Error</span>
                      <span className={styles.inlineErrorToggle}>
                        {expandedSections.has('apply_error') ? '∧' : '∨'}
                      </span>
                    </div>
                    {expandedSections.has('apply_error') && (
                      <pre className={styles.inlineErrorMessage}>{task.error_message}</pre>
                    )}
                  </div>
                  {/* AI 错误分析 - 安全修复：只传入 task_id，其他信息从后端数据库获取 */}
                  {showAIAnalysis && (
                    <div className={styles.aiAnalysisWrapper}>
                      <AIErrorAnalysis
                        workspaceId={workspaceId}
                        taskId={task.id}
                      />
                    </div>
                  )}
                </>
              )}
              <StructuredRunOutput
                task={task}
                workspaceId={workspaceId}
                workspace={workspace}
                mode="apply"
              />
            </div>
          )}
        </div>
      )}

      {/* Post-apply Run Tasks */}
      {applyStatus === 'passed' && (
        <RunTaskResults
          workspaceId={workspaceId}
          taskId={task.id}
          stage="post_apply"
        />
      )}

      {/* Error Card - only show if error is NOT in plan or apply stage (to avoid duplication) */}
      {/* Also hide for cancelled tasks (including those with "cancelled" in error_message) */}
      {task.error_message && !isCancelledTask() && 
       planStatus !== 'failed' && applyStatus !== 'failed' && (
        <div className={`${styles.timelineCard} ${styles.cardFailed}`}>
          <div className={styles.cardHeader}>
            <div className={styles.cardTitle}>
              <span
                className={styles.statusIcon}
                style={{ color: '#ef4444', backgroundColor: '#fee2e2' }}
              >
                ✗
              </span>
              <span className={styles.titleText}>Error</span>
            </div>
          </div>
          <div className={styles.cardContent}>
            <pre className={styles.errorMessage}>{task.error_message}</pre>
          </div>
        </div>
      )}
    </div>
  );
};

export default TaskTimeline;
