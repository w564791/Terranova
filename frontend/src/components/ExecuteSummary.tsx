import React, { useState, useEffect, useCallback } from 'react';
import {
  getPlanSummary, getApplySummary,
  retryPlanSummary, retryApplySummary,
  type PlanSummary, type ApplySummary,
} from '../services/ai';
import styles from './ExecuteSummary.module.css';

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
          <span className={styles.headerIcon}>|</span>
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
                <span className={styles.errorIcon}>⚠</span>
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
              <span className={styles.errorIcon}>⚠</span>
              <span>{error}</span>
            </div>
          )}

          {/* Plan Summary Result */}
          {summary?.status === 'completed' && stage === 'plan' && (
            <PlanSummaryResult summary={summary as PlanSummary} getRiskColor={getRiskColor} getRiskLabel={getRiskLabel} />
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

      {/* 影响分析 — 摘要始终展示，详情默认折叠 */}
      {summary.impact_analysis && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>影响分析</div>
          <div className={styles.sectionContent}>
            {typeof summary.impact_analysis === 'object' && summary.impact_analysis.summary && (
              <p>{summary.impact_analysis.summary}</p>
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
                        {d.impact && <div className={styles.detailImpact}>{d.impact}</div>}
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
                  {r.impact && <div className={styles.affectedImpact}>{r.impact}</div>}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* 风险等级 — 始终展示 */}
      {summary.risk_level && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>风险等级</div>
          <div className={styles.sectionContent}>
            <span className={`${styles.riskBadge} ${getRiskColor(summary.risk_level)}`}>
              {getRiskLabel(summary.risk_level)}
            </span>
          </div>
        </div>
      )}
    </div>
  );
};

// ========== Apply Summary 子组件 ==========

const ApplySummaryResult: React.FC<{ summary: ApplySummary }> = ({ summary }) => {
  const [showResults, setShowResults] = useState(false);
  const [showAffected, setShowAffected] = useState(false);

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
              <p>{summary.impact_confirmation.predicted_vs_actual}</p>
            )}
            {summary.impact_confirmation.unexpected_changes &&
              Array.isArray(summary.impact_confirmation.unexpected_changes) &&
              summary.impact_confirmation.unexpected_changes.length > 0 && (
              <div className={styles.unexpectedChanges}>
                <div className={styles.unexpectedTitle}>意外变更:</div>
                <ul>
                  {summary.impact_confirmation.unexpected_changes.map((c: string, i: number) => (
                    <li key={i}>{c}</li>
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
            <div className={styles.resourceResultsList}>
              {summary.resource_results.map((r: any, i: number) => (
                <div key={i} className={styles.resourceResultItem}>
                  <div className={styles.detailHeader}>
                    <span className={styles.resourceAddress}>{r.address}</span>
                    <span className={`${styles.resourceAction} ${styles[`action${r.action}`]}`}>{r.action}</span>
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
                  {r.impact && <div className={styles.affectedImpact}>{r.impact}</div>}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default ExecuteSummary;
