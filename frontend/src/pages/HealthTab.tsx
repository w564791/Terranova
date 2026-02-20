import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import DriftConfig from '../components/DriftConfig';
import * as driftService from '../services/drift';
import styles from './HealthTab.module.css';

interface HealthTabProps {
  workspaceId: string;
}

const HealthTab: React.FC<HealthTabProps> = ({ workspaceId }) => {
  const { showToast } = useToast();
  const [loading, setLoading] = useState(true);
  const [driftStatus, setDriftStatus] = useState<driftService.DriftResult | null>(null);
  const [resourceDriftStatuses, setResourceDriftStatuses] = useState<driftService.ResourceDriftStatus[]>([]);
  const [triggeringCheck, setTriggeringCheck] = useState(false);
  const [checkingTaskId, setCheckingTaskId] = useState<number | null>(null);
  const [configExpanded, setConfigExpanded] = useState(false);
  const [expandedResources, setExpandedResources] = useState<Set<string>>(new Set());
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // 加载 drift 状态
  const loadDriftStatus = useCallback(async () => {
    try {
      const status = await driftService.getDriftStatus(workspaceId);
      setDriftStatus(status);
    } catch (error) {
      console.error('Failed to load drift status:', error);
    }
  }, [workspaceId]);

  // 加载资源 drift 状态
  const loadResourceDriftStatuses = useCallback(async () => {
    try {
      const statuses = await driftService.getResourceDriftStatuses(workspaceId);
      setResourceDriftStatuses(statuses || []);
    } catch (error) {
      console.error('Failed to load resource drift statuses:', error);
      setResourceDriftStatuses([]);
    }
  }, [workspaceId]);

  // 清理轮询
  const cleanupPolling = useCallback(() => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
      pollingRef.current = null;
    }
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
  }, []);

  // 初始加载
  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      await Promise.all([loadDriftStatus(), loadResourceDriftStatuses()]);
      setLoading(false);
    };
    loadData();
  }, [loadDriftStatus, loadResourceDriftStatuses]);

  // 检查是否有正在进行的 drift 检测
  useEffect(() => {
    if (driftStatus?.check_status === 'running' || driftStatus?.check_status === 'pending') {
      // 有正在进行的检测，显示动画并开始轮询
      setTriggeringCheck(true);
      if (driftStatus.current_task_id) {
        setCheckingTaskId(driftStatus.current_task_id);
      }
      
      // 开始轮询
      if (!pollingRef.current) {
        let pollCount = 0;
        const maxPolls = 60;
        pollingRef.current = setInterval(async () => {
          pollCount++;
          try {
            const status = await driftService.getDriftStatus(workspaceId);
            if (status.check_status !== 'running' && status.check_status !== 'pending') {
              cleanupPolling();
              setTriggeringCheck(false);
              setCheckingTaskId(null);
              if (status.check_status === 'success') {
                showToast('Drift 检测完成', 'success');
              } else if (status.check_status === 'failed') {
                showToast(`Drift 检测失败: ${status.error_message || '未知错误'}`, 'error');
              } else if (status.check_status === 'skipped') {
                showToast('Drift 检测已跳过（无可用 Agent）', 'info');
              }
              await Promise.all([loadDriftStatus(), loadResourceDriftStatuses()]);
            } else if (pollCount >= maxPolls) {
              cleanupPolling();
              setTriggeringCheck(false);
              setCheckingTaskId(null);
            }
          } catch (error) {
            console.error('Failed to poll drift status:', error);
          }
        }, 2000);
      }
    }
  }, [driftStatus?.check_status, driftStatus?.current_task_id, workspaceId, cleanupPolling, showToast, loadDriftStatus, loadResourceDriftStatuses]);

  // 组件卸载时清理
  useEffect(() => {
    return () => {
      cleanupPolling();
    };
  }, [cleanupPolling]);

  // 轮询检查任务状态
  const pollTaskStatus = useCallback(async (taskId: number) => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/tasks/${taskId}`);
      if (!response.ok) {
        throw new Error('Failed to get task status');
      }
      const task = await response.json();
      
      // 检查任务是否完成
      if (task.status === 'success' || task.status === 'applied' || task.status === 'planned_and_finished') {
        cleanupPolling();
        setTriggeringCheck(false);
        setCheckingTaskId(null);
        showToast('Drift 检测完成', 'success');
        // 刷新状态
        await Promise.all([loadDriftStatus(), loadResourceDriftStatuses()]);
      } else if (task.status === 'failed' || task.status === 'cancelled') {
        cleanupPolling();
        setTriggeringCheck(false);
        setCheckingTaskId(null);
        showToast(task.status === 'cancelled' ? 'Drift 检测已取消' : `Drift 检测失败: ${task.error_message || '未知错误'}`, task.status === 'cancelled' ? 'info' : 'error');
        // 刷新状态
        await loadDriftStatus();
      }
      // 其他状态继续轮询
    } catch (error) {
      console.error('Failed to poll task status:', error);
    }
  }, [workspaceId, cleanupPolling, showToast, loadDriftStatus, loadResourceDriftStatuses]);

  // 手动触发 drift 检测
  const handleTriggerCheck = async () => {
    try {
      setTriggeringCheck(true);
      abortControllerRef.current = new AbortController();
      
      const result = await driftService.triggerDriftCheck(workspaceId);
      
      if (result && result.task_id) {
        setCheckingTaskId(result.task_id);
        showToast('Drift 检测已触发，正在执行...', 'info');
        
        // 开始轮询任务状态
        pollingRef.current = setInterval(() => {
          pollTaskStatus(result.task_id);
        }, 2000);
      } else {
        // 没有 task_id，使用轮询 drift status 来检测完成
        showToast('Drift 检测已触发，正在执行...', 'info');
        
        // 轮询 drift status 直到检测完成
        let pollCount = 0;
        const maxPolls = 60; // 最多轮询 2 分钟
        pollingRef.current = setInterval(async () => {
          pollCount++;
          try {
            const status = await driftService.getDriftStatus(workspaceId);
            // 检查是否完成（check_status 不是 running 或 pending）
            if (status.check_status !== 'running' && status.check_status !== 'pending') {
              cleanupPolling();
              setTriggeringCheck(false);
              if (status.check_status === 'success') {
                showToast('Drift 检测完成', 'success');
              } else if (status.check_status === 'failed') {
                showToast(`Drift 检测失败: ${status.error_message || '未知错误'}`, 'error');
              } else if (status.check_status === 'skipped') {
                showToast('Drift 检测已跳过（无可用 Agent）', 'info');
              }
              await Promise.all([loadDriftStatus(), loadResourceDriftStatuses()]);
            } else if (pollCount >= maxPolls) {
              cleanupPolling();
              setTriggeringCheck(false);
              showToast('Drift 检测超时', 'error');
            }
          } catch (error) {
            console.error('Failed to poll drift status:', error);
          }
        }, 2000);
      }
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
      setTriggeringCheck(false);
      setCheckingTaskId(null);
    }
  };

  // 取消 drift 检测
  const handleCancelCheck = async () => {
    // 如果有 task_id，尝试取消任务
    if (checkingTaskId) {
      try {
        const response = await fetch(`/api/v1/workspaces/${workspaceId}/tasks/${checkingTaskId}/cancel`, {
          method: 'POST',
        });
        
        if (!response.ok) {
          throw new Error('Failed to cancel task');
        }
        
        showToast('正在取消 Drift 检测...', 'info');
      } catch (error) {
        const message = extractErrorMessage(error);
        showToast(`取消失败: ${message}`, 'error');
      }
    }
    
    // 无论如何都停止轮询和重置状态
    cleanupPolling();
    setTriggeringCheck(false);
    setCheckingTaskId(null);
  };

  // 切换资源展开状态
  const toggleResource = (resourceId: string) => {
    setExpandedResources(prev => {
      const next = new Set(prev);
      if (next.has(resourceId)) {
        next.delete(resourceId);
      } else {
        next.add(resourceId);
      }
      return next;
    });
  };

  // Format relative time
  const formatRelativeTime = (dateString: string | null) => {
    if (!dateString) return 'Never';
    if (dateString.startsWith('0001-01-01')) return 'Never';
    
    const date = new Date(dateString);
    const now = new Date();
    
    if (isNaN(date.getTime())) return 'Invalid date';
    
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);

    if (diffMins < 5) return 'Just now';
    if (diffMins < 60) return `${diffMins} min ago`;
    if (diffHours < 24) return `${diffHours} hours ago`;
    return date.toLocaleString('en-US', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  };

  // 格式化值
  const formatValue = (value: unknown): string => {
    if (value === null) return 'null';
    if (value === undefined) return 'undefined';
    if (value === '') return '""';
    if (typeof value === 'boolean') return value ? 'true' : 'false';
    if (typeof value === 'number') return String(value);
    if (typeof value === 'string') return `"${value}"`;
    if (Array.isArray(value)) return JSON.stringify(value);
    if (typeof value === 'object') return JSON.stringify(value);
    return String(value);
  };

  // 获取变更类型配置
  // create 类型表示资源未应用（unapplied），使用灰色
  // update/delete 类型才是真正的 drift
  const getChangeTypeConfig = (action: string | undefined) => {
    switch (action) {
      case 'create': return { icon: '+', label: 'CREATE', className: styles.actionUnapplied };
      case 'update': return { icon: '~', label: 'UPDATE', className: styles.actionUpdate };
      case 'delete': return { icon: '−', label: 'DELETE', className: styles.actionDelete };
      case 'replace': return { icon: '↻', label: 'REPLACE', className: styles.actionDelete };
      default: return { icon: '?', label: (action || 'UNKNOWN').toUpperCase(), className: '' };
    }
  };

  // 计算变更的字段
  const computeChanges = (before: Record<string, unknown>, after: Record<string, unknown>) => {
    const changed: Array<{ key: string; before: unknown; after: unknown; type: 'add' | 'remove' | 'modify' }> = [];
    const allKeys = new Set([...Object.keys(before || {}), ...Object.keys(after || {})]);

    allKeys.forEach(key => {
      const beforeVal = before?.[key];
      const afterVal = after?.[key];
      const beforeEmpty = beforeVal === null || beforeVal === undefined;
      const afterEmpty = afterVal === null || afterVal === undefined;

      if (beforeEmpty && afterEmpty) return;

      if (JSON.stringify(beforeVal) !== JSON.stringify(afterVal)) {
        if (beforeEmpty && !afterEmpty) {
          changed.push({ key, before: beforeVal, after: afterVal, type: 'add' });
        } else if (!beforeEmpty && afterEmpty) {
          changed.push({ key, before: beforeVal, after: afterVal, type: 'remove' });
        } else {
          changed.push({ key, before: beforeVal, after: afterVal, type: 'modify' });
        }
      }
    });

    return changed;
  };

  if (loading) {
    return <div className={styles.loading}>Loading...</div>;
  }

  // 计算统计数据
  const statuses = resourceDriftStatuses || [];
  const driftedCount = statuses.filter(r => r.drift_status === 'drifted').length;
  const syncedCount = statuses.filter(r => r.drift_status === 'synced').length;
  const unappliedCount = statuses.filter(r => r.drift_status === 'unapplied').length;
  const totalCount = statuses.length;

  // 获取 drift 详情中的资源信息
  const driftResources = driftStatus?.drift_details?.resources || [];

  return (
    <div className={styles.healthTabContainer}>
      {/* Drift Configuration - Collapsible */}
      <div className={styles.section}>
        <div className={styles.driftConfigHeader}>
          <div className={styles.driftConfigTitle}>
            <span 
              className={styles.driftConfigToggle}
              onClick={() => setConfigExpanded(!configExpanded)}
            >
              {configExpanded ? '▼' : '▶'}
            </span>
            <h2 className={styles.sectionTitle}>Drift Detection Configuration</h2>
          </div>
          <div className={styles.checkButtonGroup}>
            {/* 上次检测信息 */}
            {driftStatus && (
              <div className={styles.lastCheckInfo}>
                <span className={styles.lastCheckTime}>
                  Last: {formatRelativeTime(driftStatus.last_check_at || null)}
                </span>
                <span className={`${styles.lastCheckStatus} ${styles[`status${driftStatus.check_status?.charAt(0).toUpperCase()}${driftStatus.check_status?.slice(1)}`]}`}>
                  {driftStatus.check_status === 'success' ? '✓' : 
                   driftStatus.check_status === 'failed' ? '✗' : 
                   driftStatus.check_status === 'skipped' ? '⊘' : '○'}
                </span>
              </div>
            )}
            {triggeringCheck ? (
              <>
                <div className={styles.checkingStatus}>
                  <svg className={styles.spinnerIcon} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeDasharray="31.4 31.4" />
                  </svg>
                  <span>Checking...</span>
                </div>
                <button 
                  className={styles.cancelCheckButton}
                  onClick={handleCancelCheck}
                >
                  Cancel
                </button>
              </>
            ) : (
              <button 
                className={styles.triggerCheckButton}
                onClick={handleTriggerCheck}
                disabled={triggeringCheck}
              >
                Run Check
              </button>
            )}
          </div>
        </div>
        {configExpanded && <DriftConfig workspaceId={workspaceId} />}
      </div>

      {/* Drift Status Overview - Only show when drift exists */}
      {driftStatus?.has_drift && driftedCount > 0 && (
        <div className={styles.section}>
          <div className={styles.sectionHeader}>
            <h2 className={styles.sectionTitle}>Drift Detection Status</h2>
            <span className={styles.driftLastCheck}>
              Last check: {formatRelativeTime(driftStatus?.last_check_at || null)}
            </span>
          </div>

          {/* 状态统计条 */}
          {totalCount > 0 && (
            <div className={styles.driftStatusBar}>
              {driftedCount > 0 && (
                <div 
                  className={styles.driftStatusBarDrift}
                  style={{ width: `${(driftedCount / totalCount) * 100}%` }}
                  title={`${driftedCount} drifted`}
                />
              )}
              {syncedCount > 0 && (
                <div 
                  className={styles.driftStatusBarSynced}
                  style={{ width: `${(syncedCount / totalCount) * 100}%` }}
                  title={`${syncedCount} synced`}
                />
              )}
              {unappliedCount > 0 && (
                <div 
                  className={styles.driftStatusBarUnapplied}
                  style={{ width: `${(unappliedCount / totalCount) * 100}%` }}
                  title={`${unappliedCount} unapplied`}
                />
              )}
            </div>
          )}

          {/* Stats and Quick Actions */}
          <div className={styles.driftStatusContent}>
            {/* 统计数字 */}
            <div className={styles.driftStatusStats}>
              <div className={styles.driftStatItem}>
                <span className={styles.driftStatValue} style={{ color: '#F59E0B' }}>{driftedCount}</span>
                <span className={styles.driftStatLabel}>Drifted</span>
              </div>
              <div className={styles.driftStatItem}>
                <span className={styles.driftStatValue} style={{ color: '#10B981' }}>{syncedCount}</span>
                <span className={styles.driftStatLabel}>Synced</span>
              </div>
              <div className={styles.driftStatItem}>
                <span className={styles.driftStatValue} style={{ color: '#6B7280' }}>{unappliedCount}</span>
                <span className={styles.driftStatLabel}>Unapplied</span>
              </div>
              <div className={styles.driftStatItem}>
                <span className={styles.driftStatValue}>{totalCount}</span>
                <span className={styles.driftStatLabel}>Total</span>
              </div>
            </div>

            {/* Summary */}
            <div className={styles.summaryBox}>
              <div className={styles.summaryText}>
                {driftedCount > 0 && (
                  <p>
                    <strong>{driftedCount}</strong> resource{driftedCount > 1 ? 's have' : ' has'} drifted. 
                    Run <code>terraform apply</code> to reconcile.
                  </p>
                )}
                {unappliedCount > 0 && (
                  <p>
                    <strong>{unappliedCount}</strong> new resource{unappliedCount > 1 ? 's' : ''} pending. 
                    Run <code>terraform apply</code> to create.
                  </p>
                )}
              </div>
            </div>
          </div>

          {/* Status Legend */}
          <div className={styles.statusLegend}>
            <div className={styles.legendItem}>
              <span className={styles.legendDot} style={{ backgroundColor: '#10B981' }}></span>
              <span className={styles.legendLabel}>Synced</span>
              <span className={styles.legendDesc}>Resource state matches code</span>
            </div>
            <div className={styles.legendItem}>
              <span className={styles.legendDot} style={{ backgroundColor: '#6B7280' }}></span>
              <span className={styles.legendLabel}>Unapplied</span>
              <span className={styles.legendDesc}>New resource pending creation</span>
            </div>
            <div className={styles.legendItem}>
              <span className={styles.legendDot} style={{ backgroundColor: '#F59E0B' }}></span>
              <span className={styles.legendLabel}>Drifted</span>
              <span className={styles.legendDesc}>Cloud state differs from code, needs attention</span>
            </div>
          </div>

          {/* AI Summary Area (Reserved, hidden for now) */}
          {/* 
          {driftedCount > 0 && (
            <div className={styles.aiSummaryPlaceholder}>
              <div className={styles.aiSummaryHeader}>
                <span className={styles.aiIcon}>🤖</span>
                <span className={styles.aiTitle}>AI Analysis</span>
              </div>
              <div className={styles.aiSummaryContent}>
                <span className={styles.aiPlaceholderText}>AI analysis coming soon...</span>
              </div>
            </div>
          )}
          */}
        </div>
      )}

      {/* Resource Drift List - Card Layout */}
      {driftStatus?.has_drift && (driftedCount > 0 || unappliedCount > 0) && (
        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Resources with Changes</h2>
          
          <div className={styles.resourceList}>
            {driftResources.filter(r => r.has_drift).map((resource) => {
              const isExpanded = expandedResources.has(resource.resource_id);
              const childrenCount = resource.drifted_children?.length || 0;
              
              // 判断资源是 drifted 还是 unapplied
              // 如果所有子资源都是 create 类型，则为 unapplied（灰色）
              // 如果有任何 update/delete/replace 类型，则为 drifted（黄色）
              const hasRealDrift = resource.drifted_children?.some(
                (child: any) => child.action === 'update' || child.action === 'delete' || child.action === 'replace'
              ) || false;
              
              const resourceStatus = hasRealDrift ? 'drifted' : 'unapplied';
              const resourceCardClass = hasRealDrift ? styles.resourceCardDrifted : styles.resourceCardUnapplied;
              const resourceIcon = hasRealDrift ? '⚠' : '○';
              const resourceIconClass = hasRealDrift ? styles.resourceIconDrifted : styles.resourceIconUnapplied;
              const badgeClass = hasRealDrift ? styles.driftBadge : styles.unappliedBadge;
              const badgeText = hasRealDrift ? 'DRIFTED' : 'UNAPPLIED';

              return (
                <div key={resource.resource_id} className={`${styles.resourceCard} ${resourceCardClass}`}>
                  <div className={styles.resourceHeader} onClick={() => toggleResource(resource.resource_id)}>
                    <div className={styles.resourceHeaderLeft}>
                      <span className={styles.expandIcon}>{isExpanded ? '▼' : '▶'}</span>
                      <span className={`${styles.resourceIcon} ${resourceIconClass}`}>{resourceIcon}</span>
                      <span className={styles.resourceName}>{resource.resource_name}</span>
                    </div>
                    <div className={styles.resourceHeaderRight}>
                      <span className={styles.childrenCount}>{childrenCount} changes</span>
                      <span className={badgeClass}>{badgeText}</span>
                    </div>
                  </div>

                  {isExpanded && resource.drifted_children && resource.drifted_children.length > 0 && (
                    <div className={styles.resourceBody}>
                      <div className={styles.childrenList}>
                        {resource.drifted_children.map((child: any, index: number) => {
                          // 后端返回的字段是 action 而不是 change_type
                          const config = getChangeTypeConfig(child.action);
                          // 后端返回的变更详情在 changes 对象中，每个 key 包含 before/after
                          const changesObj = child.changes || {};
                          const changesList = Object.entries(changesObj).map(([key, value]: [string, any]) => ({
                            key,
                            before: value?.before,
                            after: value?.after,
                            type: value?.before === null || value?.before === undefined ? 'add' as const :
                                  value?.after === null || value?.after === undefined ? 'remove' as const : 'modify' as const
                          }));

                          return (
                            <div key={index} className={`${styles.childCard} ${config.className}`}>
                              <div className={styles.childHeader}>
                                <span className={`${styles.actionIcon} ${config.className}`}>{config.icon}</span>
                                <span className={styles.childAddress}>{child.address}</span>
                                <span className={`${styles.actionBadge} ${config.className}`}>{config.label}</span>
                              </div>
                              
                              {changesList.length > 0 && (
                                <div className={styles.changesTable}>
                                  {changesList.map(({ key, before, after, type }) => (
                                    <div key={key} className={styles.changeRow}>
                                      <span className={`${styles.changeIcon} ${styles[`icon${type.charAt(0).toUpperCase() + type.slice(1)}`]}`}>
                                        {type === 'add' ? '+' : type === 'remove' ? '−' : '~'}
                                      </span>
                                      <span className={styles.changeKey}>{key} =</span>
                                      {type === 'add' ? (
                                        <span className={styles.valueCreate}>{formatValue(after)}</span>
                                      ) : type === 'remove' ? (
                                        <span className={styles.valueDelete}>{formatValue(before)}</span>
                                      ) : (
                                        <span className={styles.valueComparison}>
                                          <span className={styles.valueBefore}>{formatValue(before)}</span>
                                          <span className={styles.arrow}>→</span>
                                          <span className={styles.valueAfter}>{formatValue(after)}</span>
                                        </span>
                                      )}
                                    </div>
                                  ))}
                                </div>
                              )}
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};

export default HealthTab;
