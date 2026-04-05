import React, { useState, useEffect, useCallback } from 'react';
import {
  getPlanSummary, getApplySummary,
  retryPlanSummary, retryApplySummary,
  confirmPlanSummary, bypassAIIncomplete,
  type PlanSummary, type ApplySummary,
} from '../services/ai';
import { reportSkillUsageByCapability } from '../services/aiForm';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';
import styles from './ExecuteSummary.module.css';

// 安全渲染：防止 AI 返回的对象被直接当 React child
const safeRender = (value: any): string => {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  if (typeof value === 'object') {
    // 尝试常见字段
    return value.description || value.text || value.message || value.summary ||
           value.label || value.name || value.field || value.impact ||
           JSON.stringify(value);
  }
  return String(value);
};

interface ExecuteSummaryProps {
  workspaceId: string;
  taskId: number;
  stage: 'plan' | 'apply';
  defaultExpanded?: boolean;
}

const ExecuteSummary: React.FC<ExecuteSummaryProps> = ({
  workspaceId,
  taskId,
  stage,
  defaultExpanded = true,
}) => {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [summary, setSummary] = useState<PlanSummary | ApplySummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [retrying, setRetrying] = useState(false);

  const fetchSummary = useCallback(async () => {
    try {
      setError(null);
      const result = stage === 'plan'
        ? await getPlanSummary(workspaceId, taskId)
        : await getApplySummary(workspaceId, taskId);
      setSummary(result);

      // 如果还在运行中，继续轮询
      if (result.status === 'running' || result.status === 'pending') {
        setTimeout(fetchSummary, 3000);
      }
    } catch (err: any) {
      // 注意：api 拦截器的 error 已被转为字符串（errorMessage），不是原始 error 对象
      // 404 = summary 还没生成，继续轮询
      const errStr = typeof err === 'string' ? err : (err?.message || '');
      if (errStr.includes('not found') || errStr.includes('404')) {
        setTimeout(fetchSummary, 5000);
      } else {
        setError('获取摘要失败');
      }
    } finally {
      setLoading(false);
    }
  }, [workspaceId, taskId, stage]);

  useEffect(() => {
    fetchSummary();
  }, [fetchSummary]);

  const handleRetry = async () => {
    try {
      setRetrying(true);
      setError(null);
      if (stage === 'plan') {
        await retryPlanSummary(workspaceId, taskId);
      } else {
        await retryApplySummary(workspaceId, taskId);
      }
      setSummary(null);
      setLoading(true);
      setTimeout(fetchSummary, 2000);
    } catch (err: any) {
      setError(typeof err === 'string' ? err : '重试失败');
    } finally {
      setRetrying(false);
    }
  };

  const getRiskColor = (level: string) => {
    switch (level) {
      case 'critical': return styles.riskCritical;
      case 'high': return styles.riskHigh;
      case 'medium': return styles.riskMedium;
      case 'low': return styles.riskLow;
      default: return '';
    }
  };

  const getRiskLabel = (level: string) => {
    switch (level) {
      case 'critical': return '严重';
      case 'high': return '高';
      case 'medium': return '中等';
      case 'low': return '低';
      default: return level;
    }
  };

  const stageLabel = stage === 'plan' ? '变更影响分析' : '执行结果分析';

  // 还在加载中且没有任何数据
  if (loading && !summary) {
    return (
      <div className={styles.container}>
        <div className={styles.header}>
          <span className={styles.headerTitle}>{stageLabel}</span>
          <div className={styles.loadingInline}>
            <div className={styles.spinner} />
            <span>分析中...</span>
          </div>
        </div>
      </div>
    );
  }

  // 没有数据（可能 AI 配置未开启）
  if (!summary && !loading && !error) {
    return null;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header} onClick={() => setExpanded(prev => !prev)}>
        <span className={styles.headerTitle}>{stageLabel}</span>

        {summary?.status === 'completed' && 'risk_level' in summary && (summary as PlanSummary).risk_level && (
          <span className={`${styles.riskBadge} ${getRiskColor((summary as PlanSummary).risk_level)}`}>
            风险: {getRiskLabel((summary as PlanSummary).risk_level)}
          </span>
        )}

        {summary?.status === 'running' && (
          <div className={styles.loadingInline}>
            <div className={styles.spinner} />
            <span>分析中...</span>
          </div>
        )}

        {summary?.status === 'failed' && (
          <span className={styles.failedBadge}>分析失败</span>
        )}

        <span className={styles.expandToggle}>{expanded ? '∧' : '∨'}</span>
      </div>

      {expanded && (
        <div className={styles.content}>
          {/* Running */}
          {summary?.status === 'running' && (
            <div className={styles.loading}>
              <div className={styles.spinner} />
              <span>AI 正在分析变更影响，请稍候...</span>
            </div>
          )}

          {/* Failed */}
          {summary?.status === 'failed' && (
            <div className={styles.errorBlock}>
              <div className={styles.errorMessage}>
                <span className={styles.errorIcon}>!</span>
                <span>{summary.error_message || '分析失败'}</span>
              </div>
              <button
                className={styles.retryButton}
                onClick={handleRetry}
                disabled={retrying}
              >
                {retrying ? '重试中...' : '重新分析'}
              </button>
            </div>
          )}

          {/* Error from fetch */}
          {error && !summary && (
            <div className={styles.errorBlock}>
              <span className={styles.errorIcon}>!</span>
              <span>{error}</span>
            </div>
          )}

          {/* Plan Summary Result */}
          {summary?.status === 'completed' && stage === 'plan' && (
            <PlanSummaryResult summary={summary as PlanSummary} getRiskColor={getRiskColor} getRiskLabel={getRiskLabel} />
          )}

          {/* AI Incomplete Warning */}
          {summary?.status === 'completed' && stage === 'plan' && (
            <AIIncompleteWarning
              summary={summary as PlanSummary}
              workspaceId={workspaceId}
              taskId={taskId}
              onBypassed={fetchSummary}
            />
          )}

          {/* Decision Confirmation */}
          {summary?.status === 'completed' && stage === 'plan' && (summary as PlanSummary).requires_confirmation && (
            <DecisionConfirmation
              summary={summary as PlanSummary}
              workspaceId={workspaceId}
              taskId={taskId}
              onConfirmed={fetchSummary}
            />
          )}

          {/* Apply Summary Result */}
          {summary?.status === 'completed' && stage === 'apply' && (
            <ApplySummaryResult summary={summary as ApplySummary} />
          )}

          {/* Duration */}
          {summary?.status === 'completed' && summary.duration > 0 && (
            <div className={styles.duration}>
              分析耗时: {(summary.duration / 1000).toFixed(1)}秒
            </div>
          )}
        </div>
      )}
    </div>
  );
};

// ========== Plan Summary 子组件 ==========

const PlanSummaryResult: React.FC<{
  summary: PlanSummary;
  getRiskColor: (level: string) => string;
  getRiskLabel: (level: string) => string;
}> = ({ summary, getRiskColor, getRiskLabel }) => {
  const [showDetails, setShowDetails] = useState(false);
  const [showAffected, setShowAffected] = useState(false);
  const [showThinking, setShowThinking] = useState(false);

  const detailsCount = summary.impact_analysis?.details?.length || 0;
  const affectedCount = summary.affected_resources?.length || 0;

  return (
    <div className={styles.result}>
      {/* 变更概述 — 始终展示 */}
      {summary.changes_overview && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>变更概述</div>
          <div className={styles.sectionContent}>{summary.changes_overview}</div>
        </div>
      )}

      {/* Deterministic Risk Score */}
      {summary.risk_score_value !== undefined && summary.risk_score_breakdown && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>
            Risk Score
            <span className={`${styles.riskBadge} ${getRiskColor(summary.risk_score_breakdown.risk_level)}`} style={{ marginLeft: 8 }}>
              {summary.risk_score_value.toFixed(1)} / 100
            </span>
            {summary.risk_score_breakdown.near_threshold && (
              <span className={styles.nearThresholdTag}>Near Threshold</span>
            )}
            {summary.risk_score_breakdown.divergence_alert && (
              <span className={styles.divergenceTag}>
                AI/Go Divergence (AI: {getRiskLabel(summary.risk_score_breakdown.ai_risk_level || '')}, Go: {getRiskLabel(summary.risk_score_breakdown.risk_level)})
              </span>
            )}
          </div>
          <div className={styles.sectionContent}>
            <div className={styles.scoreBar}>
              <div
                className={styles.scoreBarFill}
                style={{
                  width: `${summary.risk_score_value}%`,
                  backgroundColor: summary.risk_score_color === 'green' ? '#10b981' :
                    summary.risk_score_color === 'yellow' ? '#f59e0b' :
                    summary.risk_score_color === 'orange' ? '#f97316' : '#ef4444'
                }}
              />
            </div>
            <div className={styles.scoreDetails}>
              <span>Base Deduction: {summary.risk_score_breakdown.base_deduction}</span>
              <span>Env Multiplier: x{summary.risk_score_breakdown.env_multiplier}</span>
              {summary.risk_score_breakdown.combo_multiplier_applied && (
                <span>Combo: {summary.risk_score_breakdown.combo_detail}</span>
              )}
            </div>
            {summary.risk_score_breakdown.deductions.length > 0 && (
              <div className={styles.deductionList}>
                {summary.risk_score_breakdown.deductions.map((d, i) => (
                  <div key={i} className={styles.deductionItem}>
                    <span className={styles.deductionCategory}>{d.category}</span>
                    <span className={styles.deductionPoints}>{d.points}</span>
                    <span className={styles.deductionReason}>{d.item}: {d.reason}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* 影响分析 — 摘要始终展示，详情默认折叠 */}
      {summary.impact_analysis && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>影响分析</div>
          <div className={styles.sectionContent}>
            {typeof summary.impact_analysis === 'object' && summary.impact_analysis.summary && (
              <p>{safeRender(summary.impact_analysis.summary)}</p>
            )}
            {summary.impact_analysis.details && Array.isArray(summary.impact_analysis.details) && detailsCount > 0 && (
              <>
                <div className={styles.collapseToggle} onClick={() => setShowDetails(!showDetails)}>
                  {showDetails ? '∧' : '∨'} 资源变更详情（{detailsCount} 项）
                </div>
                {showDetails && (
                  <div className={styles.detailsList}>
                    {summary.impact_analysis.details.map((d: any, i: number) => (
                      <div key={i} className={styles.detailItem}>
                        <div className={styles.detailHeader}>
                          <span className={styles.detailResource}>{d.resource}</span>
                          <span className={styles.detailAction}>{d.action}</span>
                          {d.dependencies_affected > 0 && (
                            <span className={styles.detailDeps}>影响 {d.dependencies_affected} 个依赖</span>
                          )}
                        </div>
                        {d.impact && <div className={styles.detailImpact}>{safeRender(d.impact)}</div>}
                      </div>
                    ))}
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      )}

      {/* 受影响资源 — 默认折叠 */}
      {summary.affected_resources && Array.isArray(summary.affected_resources) && affectedCount > 0 && (
        <div className={styles.section}>
          <div className={styles.collapseToggle} onClick={() => setShowAffected(!showAffected)}>
            {showAffected ? '∧' : '∨'} 受影响的依赖资源（{affectedCount} 项）
          </div>
          {showAffected && (
            <div className={styles.affectedList}>
              {summary.affected_resources.map((r: any, i: number) => (
                <div key={i} className={styles.affectedItem}>
                  <div className={styles.affectedHeader}>
                    <span className={styles.affectedAddress}>{r.address}</span>
                    <span className={styles.affectedType}>{r.type}</span>
                  </div>
                  {r.impact && <div className={styles.affectedImpact}>{safeRender(r.impact)}</div>}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* AI Thinking 内容 — 默认折叠 */}
      {summary.thinking_content && Array.isArray(summary.thinking_content) && summary.thinking_content.length > 0 && (
        <div className={styles.section}>
          <div className={styles.collapseToggle} onClick={() => setShowThinking(!showThinking)}>
            {showThinking ? '∧' : '∨'} AI Thinking（{summary.thinking_content.length} 轮）
          </div>
          {showThinking && (
            <div className={styles.detailsList}>
              {summary.thinking_content.map((t: string, i: number) => (
                <div key={i} className={styles.detailItem}>
                  <div className={styles.detailHeader}>
                    <span className={styles.detailResource}>Round {i + 1}</span>
                  </div>
                  <div className={styles.detailImpact} style={{ whiteSpace: 'pre-wrap', fontSize: 12, color: '#666' }}>{t}</div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* 风险等级已在 header badge 中展示，不再重复 */}
    </div>
  );
};

// ========== Apply Summary 子组件 ==========

const ApplySummaryResult: React.FC<{ summary: ApplySummary }> = ({ summary }) => {
  const [showResults, setShowResults] = useState(false);
  const [showAffected, setShowAffected] = useState(false);
  const [showThinking, setShowThinking] = useState(false);

  const resultsCount = summary.resource_results?.length || 0;
  const affectedCount = summary.affected_resources?.length || 0;

  return (
    <div className={styles.result}>
      {/* 执行总结 — 始终展示 */}
      {summary.execution_summary && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>执行总结</div>
          <div className={styles.sectionContent}>{summary.execution_summary}</div>
        </div>
      )}

      {/* 预测对比 — 始终展示 */}
      {summary.impact_confirmation && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>预测对比</div>
          <div className={styles.sectionContent}>
            {summary.impact_confirmation.predicted_vs_actual && (
              <p>{safeRender(summary.impact_confirmation.predicted_vs_actual)}</p>
            )}
            {summary.impact_confirmation.unexpected_changes &&
              Array.isArray(summary.impact_confirmation.unexpected_changes) &&
              summary.impact_confirmation.unexpected_changes.length > 0 && (
              <div className={styles.unexpectedChanges}>
                <div className={styles.unexpectedTitle}>意外变更:</div>
                <ul>
                  {summary.impact_confirmation.unexpected_changes.map((c: any, i: number) => (
                    <li key={i}>{typeof c === 'string' ? c : (c.description || c.field || JSON.stringify(c))}</li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        </div>
      )}

      {/* 资源执行结果 — 默认折叠 */}
      {summary.resource_results && Array.isArray(summary.resource_results) && resultsCount > 0 && (
        <div className={styles.section}>
          <div className={styles.collapseToggle} onClick={() => setShowResults(!showResults)}>
            {showResults ? '∧' : '∨'} 资源执行结果（{resultsCount} 项）
          </div>
          {showResults && (
            <div className={styles.detailsList}>
              {summary.resource_results.map((r: any, i: number) => (
                <div key={i} className={styles.detailItem}>
                  <div className={styles.detailHeader}>
                    <span className={styles.detailResource}>{r.address}</span>
                    <span className={styles.detailAction}>{r.action}</span>
                    <span className={`${styles.resourceStatus} ${r.status === 'success' ? styles.statusSuccess : styles.statusFailed}`}>
                      {r.status}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* 受影响资源 — 默认折叠 */}
      {summary.affected_resources && Array.isArray(summary.affected_resources) && affectedCount > 0 && (
        <div className={styles.section}>
          <div className={styles.collapseToggle} onClick={() => setShowAffected(!showAffected)}>
            {showAffected ? '∧' : '∨'} 受影响的依赖资源（{affectedCount} 项）
          </div>
          {showAffected && (
            <div className={styles.affectedList}>
              {summary.affected_resources.map((r: any, i: number) => (
                <div key={i} className={styles.affectedItem}>
                  <div className={styles.affectedHeader}>
                    <span className={styles.affectedAddress}>{r.address}</span>
                    <span className={styles.affectedType}>{r.type}</span>
                  </div>
                  {r.impact && <div className={styles.affectedImpact}>{safeRender(r.impact)}</div>}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* AI Thinking 内容 — 默认折叠 */}
      {summary.thinking_content && Array.isArray(summary.thinking_content) && summary.thinking_content.length > 0 && (
        <div className={styles.section}>
          <div className={styles.collapseToggle} onClick={() => setShowThinking(!showThinking)}>
            {showThinking ? '∧' : '∨'} AI Thinking（{summary.thinking_content.length} 轮）
          </div>
          {showThinking && (
            <div className={styles.detailsList}>
              {summary.thinking_content.map((t: string, i: number) => (
                <div key={i} className={styles.detailItem}>
                  <div className={styles.detailHeader}>
                    <span className={styles.detailResource}>Round {i + 1}</span>
                  </div>
                  <div className={styles.detailImpact} style={{ whiteSpace: 'pre-wrap', fontSize: 12, color: '#666' }}>{t}</div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

// ========== Decision Confirmation 子组件 ==========

const DecisionConfirmation: React.FC<{
  summary: PlanSummary;
  workspaceId: string;
  taskId: number;
  onConfirmed: () => void;
}> = ({ summary, workspaceId, taskId, onConfirmed }) => {
  const [checkedCodes, setCheckedCodes] = useState<Set<string>>(new Set());
  const [note, setNote] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  // 分离风险确认项和 ABORT
  const allActions = summary.decision_actions || [];
  const riskActions = allActions.filter(a => a.code !== 'ABORT');
  const abortAction = allActions.find(a => a.code === 'ABORT');
  const allChecked = riskActions.length > 0 && riskActions.every(a => checkedCodes.has(a.code));

  // 已确认状态
  if (summary.user_decision_code) {
    // 兼容新旧格式：新格式用逗号分隔多个 code
    const confirmedCodes = summary.user_decision_code.split(',');
    const confirmedLabels = confirmedCodes
      .map(code => summary.decision_actions?.find(a => a.code === code)?.label || code)
      .filter(Boolean);
    return (
      <div className={styles.decisionBox} style={{ borderColor: '#10b981' }}>
        <div className={styles.decisionHeader}>
          <span className={styles.decisionTitle}>{summary.decision_title || summary.decision_scenario || 'Risk Decision'}</span>
          <span className={styles.decisionConfirmed}>已确认</span>
        </div>
        <div className={styles.decisionResult}>
          {confirmedLabels.map((label, i) => (
            <div key={i} style={{ fontSize: '13px', color: '#374151', marginBottom: '2px' }}>✓ {label}</div>
          ))}
          <div style={{ fontSize: '12px', color: '#6b7280', marginTop: '6px' }}>
            {summary.user_decision_by} 于 {summary.user_decision_at ? new Date(summary.user_decision_at).toLocaleString() : ''} 确认
          </div>
          {summary.user_decision_note && (
            <div style={{ fontSize: '13px', color: '#374151', marginTop: '4px' }}>备注: {summary.user_decision_note}</div>
          )}
        </div>
      </div>
    );
  }

  const handleConfirm = async () => {
    if (!allChecked) return;
    try {
      setSubmitting(true);
      setError('');
      const decisionCode = Array.from(checkedCodes).join(',');
      await confirmPlanSummary(workspaceId, taskId, decisionCode, note);
      // 上报 action，评分由全局 FeedbackBanner 组件处理
      reportSkillUsageByCapability('plan_summary', 'accepted', taskId);
      onConfirmed();
    } catch (err: any) {
      setError(typeof err === 'string' ? err : '提交失败');
    } finally {
      setSubmitting(false);
    }
  };

  const handleAbort = async () => {
    try {
      setSubmitting(true);
      setError('');
      await confirmPlanSummary(workspaceId, taskId, 'ABORT', note);
      reportSkillUsageByCapability('plan_summary', 'aborted', taskId);
      onConfirmed();
    } catch (err: any) {
      setError(typeof err === 'string' ? err : '提交失败');
    } finally {
      setSubmitting(false);
    }
  };

  const toggleCheck = (code: string) => {
    setCheckedCodes(prev => {
      const next = new Set(prev);
      if (next.has(code)) next.delete(code);
      else next.add(code);
      return next;
    });
    setError('');
  };

  // V4: 使用 AI 生成的 decision_title，V3 fallback 到 scenario 映射
  const scenarioTitles: Record<string, string> = {
    SECURITY_GROUP_CHANGE: 'Security Group Change Confirmation',
    RESOURCE_DELETION: 'Resource Deletion Confirmation',
    IAM_PERMISSION_CHANGE: 'IAM Permission Change Confirmation',
    NETWORK_CORE_CHANGE: 'Core Network Change Confirmation',
  };

  const title = summary.decision_title
    || scenarioTitles[summary.decision_scenario || '']
    || 'Risk Decision Confirmation';

  const riskHighlights = summary.risk_highlights || [];

  return (
    <div className={styles.decisionBox}>
      <div className={styles.decisionHeader}>
        <span className={styles.decisionTitle}>{title}</span>
        <span className={styles.decisionRequired}>Action Required</span>
      </div>
      <div className={styles.decisionBody}>
        {riskHighlights.length > 0 && (
          <ul className={styles.riskHighlights}>
            {riskHighlights.map((highlight, i) => (
              <li key={i}>{highlight}</li>
            ))}
          </ul>
        )}
        {riskHighlights.length === 0 && (
          <p className={styles.decisionPrompt}>AI 判断此变更需要人工确认后才能执行 Apply：</p>
        )}
        <div className={styles.decisionOptions}>
          {riskActions.map((action) => (
            <label key={action.code} className={`${styles.decisionOption} ${checkedCodes.has(action.code) ? styles.decisionOptionChecked : ''}`}>
              <input
                type="checkbox"
                value={action.code}
                checked={checkedCodes.has(action.code)}
                onChange={() => toggleCheck(action.code)}
              />
              <span>{action.label}</span>
            </label>
          ))}
        </div>
        <textarea
          className={styles.decisionNote}
          placeholder="补充说明（可选）"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          rows={2}
        />
        {error && <div style={{ color: '#dc2626', fontSize: '13px', marginTop: '4px' }}>{error}</div>}
        <div className={styles.decisionButtonGroup}>
          <button
            className={styles.decisionAbort}
            onClick={handleAbort}
            disabled={submitting}
          >
            {abortAction?.label || '终止本次变更'}
          </button>
          <button
            className={styles.decisionSubmit}
            onClick={handleConfirm}
            disabled={submitting || !allChecked}
            title={allChecked ? '' : '请先确认所有风险项'}
          >
            {submitting ? '提交中...' : '已确认风险'}
          </button>
        </div>
      </div>
    </div>
  );
};

const AIIncompleteWarning: React.FC<{
  summary: PlanSummary;
  workspaceId: string;
  taskId: number;
  onBypassed: () => void;
}> = ({ summary, workspaceId, taskId, onBypassed }) => {
  const { user } = useSelector((state: RootState) => state.auth);
  const isAdmin = user?.is_system_admin;
  const [bypassReason, setBypassReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  if (!summary.ai_analysis_incomplete) return null;

  // Already bypassed
  if (summary.bypassed_by) {
    return (
      <div className={styles.aiIncompleteWarning} style={{ borderColor: '#f59e0b' }}>
        <div className={styles.warningHeader}>AI Analysis Incomplete (Bypassed)</div>
        <div className={styles.warningBody}>
          <p>AI analysis did not complete successfully. Admin {summary.bypassed_by} bypassed at {summary.bypassed_at ? new Date(summary.bypassed_at).toLocaleString() : ''}.</p>
          {summary.bypass_reason && <p>Reason: {summary.bypass_reason}</p>}
        </div>
      </div>
    );
  }

  const handleBypass = async () => {
    if (!bypassReason.trim()) return;
    try {
      setSubmitting(true);
      setError('');
      await bypassAIIncomplete(workspaceId, taskId, bypassReason);
      onBypassed();
    } catch (err: any) {
      setError(typeof err === 'string' ? err : 'Bypass failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className={styles.aiIncompleteWarning}>
      <div className={styles.warningHeader}>AI Analysis Incomplete</div>
      <div className={styles.warningBody}>
        <p>AI analysis did not complete successfully. Operations are blocked until an admin reviews and bypasses this check.</p>
        {isAdmin && (
          <div className={styles.bypassForm}>
            <textarea
              className={styles.decisionNote}
              placeholder="Bypass reason (required)"
              value={bypassReason}
              onChange={(e) => setBypassReason(e.target.value)}
              rows={2}
            />
            {error && <div style={{ color: '#dc2626', fontSize: '13px', marginTop: '4px' }}>{error}</div>}
            <button
              className={styles.bypassButton}
              onClick={handleBypass}
              disabled={submitting || !bypassReason.trim()}
            >
              {submitting ? 'Processing...' : 'Force Bypass'}
            </button>
          </div>
        )}
      </div>
    </div>
  );
};

export default ExecuteSummary;
