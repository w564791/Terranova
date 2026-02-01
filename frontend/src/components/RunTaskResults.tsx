import React, { useState, useEffect } from 'react';
import api from '../services/api';
import styles from './RunTaskResults.module.css';

interface RunTaskOutcome {
  id: number;
  outcome_id: string;
  description: string;
  body?: string;
  url?: string;
  tags?: Record<string, Array<{ label: string; level: string }>>;
}

interface RunTaskResult {
  id: number;
  result_id: string;
  run_task_id?: string;
  run_task_name?: string;
  stage: string;
  enforcement_level: string;
  status: string;
  message: string;
  url?: string;
  started_at?: string;
  completed_at?: string;
  outcomes?: RunTaskOutcome[];
  is_global?: boolean;
}

interface Props {
  workspaceId: string;
  taskId: string | number;
  stage?: 'pre_plan' | 'post_plan' | 'pre_apply' | 'post_apply';
}

const stageLabels: Record<string, string> = {
  pre_plan: 'Pre-plan checks',
  post_plan: 'Post-plan checks',
  pre_apply: 'Pre-apply checks',
  post_apply: 'Post-apply checks',
};

const statusIcons: Record<string, { icon: string; color: string }> = {
  pending: { icon: '○', color: '#6b7280' },
  running: { icon: '◐', color: '#3b82f6' },
  passed: { icon: '✓', color: '#10b981' },
  failed: { icon: '✗', color: '#ef4444' },
  error: { icon: '!', color: '#ef4444' },
  timeout: { icon: '⏱', color: '#f59e0b' },
  skipped: { icon: '⊘', color: '#6b7280' },
};

const RunTaskResults: React.FC<Props> = ({ workspaceId, taskId, stage }) => {
  const [results, setResults] = useState<RunTaskResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedStages, setExpandedStages] = useState<Set<string>>(new Set());
  const [expandedResults, setExpandedResults] = useState<Set<string>>(new Set());

  const fetchResults = async () => {
    try {
      const data: any = await api.get(
        `/workspaces/${workspaceId}/tasks/${taskId}/run-task-results`
      );
      setResults(data.run_task_results || []);
    } catch (error) {
      console.error('Failed to fetch run task results:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchResults();
    const interval = setInterval(() => {
      if (results.some((r) => r.status === 'running' || r.status === 'pending')) {
        fetchResults();
      }
    }, 5000);
    return () => clearInterval(interval);
  }, [workspaceId, taskId]);

  const toggleStage = (stageKey: string) => {
    setExpandedStages((prev) => {
      const next = new Set(prev);
      if (next.has(stageKey)) {
        next.delete(stageKey);
      } else {
        next.add(stageKey);
      }
      return next;
    });
  };

  const toggleResult = (resultId: string) => {
    setExpandedResults((prev) => {
      const next = new Set(prev);
      if (next.has(resultId)) {
        next.delete(resultId);
      } else {
        next.add(resultId);
      }
      return next;
    });
  };

  const formatTime = (dateString?: string) => {
    if (!dateString) return '';
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins} minutes ago`;
    if (diffHours < 24) return `${diffHours} hours ago`;
    return date.toLocaleString();
  };

  if (loading) {
    return null;
  }

  // Filter by stage if specified
  const filteredResults = stage
    ? results.filter((r) => r.stage === stage)
    : results;

  if (filteredResults.length === 0) {
    return null;
  }

  // Group results by stage
  const groupedResults = filteredResults.reduce((acc, result) => {
    const s = result.stage;
    if (!acc[s]) acc[s] = [];
    acc[s].push(result);
    return acc;
  }, {} as Record<string, RunTaskResult[]>);

  const stageOrder = ['pre_plan', 'post_plan', 'pre_apply', 'post_apply'];

  // Calculate summary for each stage
  const getStageSummary = (stageResults: RunTaskResult[]) => {
    const passed = stageResults.filter((r) => r.status === 'passed').length;
    const failed = stageResults.filter((r) => r.status === 'failed').length;
    const total = stageResults.length;
    return { passed, failed, total };
  };

  // Get overall status for a stage
  const getStageStatus = (stageResults: RunTaskResult[]) => {
    if (stageResults.some((r) => r.status === 'running')) return 'running';
    if (stageResults.some((r) => r.status === 'pending')) return 'pending';
    if (stageResults.some((r) => r.status === 'failed' && r.enforcement_level === 'mandatory')) return 'failed';
    if (stageResults.every((r) => r.status === 'passed' || r.status === 'skipped')) return 'passed';
    return 'warning';
  };

  return (
    <>
      {stageOrder.map((s) => {
        const stageResults = groupedResults[s];
        if (!stageResults || stageResults.length === 0) return null;

        const summary = getStageSummary(stageResults);
        const stageStatus = getStageStatus(stageResults);
        const statusConfig = statusIcons[stageStatus] || statusIcons.pending;
        const isStageExpanded = expandedStages.has(s);

        return (
          <div key={s} className={`${styles.stageCard} ${styles[`card${stageStatus}`]}`}>
            <div 
              className={styles.stageHeader}
              onClick={() => toggleStage(s)}
            >
              <div className={styles.stageIcon} style={{ color: statusConfig.color }}>
                {statusConfig.icon}
              </div>
              <div className={styles.stageInfo}>
                <span className={styles.stageTitle}>{stageLabels[s]}</span>
                <span className={styles.stageTime}>
                  {stageResults[0]?.completed_at
                    ? formatTime(stageResults[0].completed_at)
                    : stageResults[0]?.started_at
                    ? formatTime(stageResults[0].started_at)
                    : ''}
                </span>
              </div>
              <div className={styles.stageSummary}>
                {summary.passed > 0 && (
                  <span className={styles.summaryPassed}>{summary.passed} passed</span>
                )}
                {summary.failed > 0 && (
                  <span className={styles.summaryFailed}>, {summary.failed} failed</span>
                )}
              </div>
              <span className={styles.expandIcon}>{isStageExpanded ? '∧' : '∨'}</span>
            </div>

            {isStageExpanded && (
              <div className={styles.resultsList}>
                {stageResults.map((result) => {
                  const config = statusIcons[result.status] || statusIcons.pending;
                  const isResultExpanded = expandedResults.has(result.result_id);

                  return (
                    <div key={result.result_id} className={styles.resultItem}>
                      <div
                        className={styles.resultHeader}
                        onClick={() => toggleResult(result.result_id)}
                      >
                        <span className={styles.resultExpandIcon}>{isResultExpanded ? '▼' : '▶'}</span>
                        <span className={styles.resultIcon} style={{ color: config.color }}>
                          {config.icon}
                        </span>
                        <span className={styles.resultName}>
                          {result.run_task_name || result.result_id}
                        </span>
                        <span
                          className={styles.resultStatus}
                          style={{ color: config.color }}
                        >
                          {result.status.charAt(0).toUpperCase() + result.status.slice(1)}
                        </span>
                        {result.is_global && (
                          <span className={styles.globalBadge}>Global</span>
                        )}
                      </div>

                      {isResultExpanded && (
                        <div className={styles.resultDetails}>
                          {result.message && (
                            <p className={styles.resultMessage}>{result.message}</p>
                          )}
                          {result.url && (
                            <a
                              href={result.url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className={styles.resultLink}
                            >
                              View Details →
                            </a>
                          )}
                          {result.outcomes && result.outcomes.length > 0 && (
                            <div className={styles.outcomes}>
                              <h4 className={styles.outcomesTitle}>Outcomes</h4>
                              {result.outcomes.map((outcome) => (
                                <div key={outcome.id} className={styles.outcomeItem}>
                                  <div className={styles.outcomeHeader}>
                                    <span className={styles.outcomeId}>{outcome.outcome_id}</span>
                                    <span className={styles.outcomeDesc}>{outcome.description}</span>
                                  </div>
                                  {outcome.body && (
                                    <div className={styles.outcomeBody}>
                                      <pre>{outcome.body}</pre>
                                    </div>
                                  )}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
    </>
  );
};

export default RunTaskResults;
