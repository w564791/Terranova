import { useState, useEffect } from 'react';
import { analyzeError, getTaskAnalysis, type ErrorAnalysis } from '../services/ai';
import styles from './AIErrorAnalysis.module.css';

interface AIErrorAnalysisProps {
  workspaceId: number | string;
  taskId: number;
  // 安全修复：移除 errorMessage、taskType、terraformVersion 参数
  // 这些信息现在从后端数据库获取，防止 prompt injection 攻击
}

const AIErrorAnalysis: React.FC<AIErrorAnalysisProps> = ({
  workspaceId,
  taskId,
}) => {
  const [expanded, setExpanded] = useState(false);
  const [analysis, setAnalysis] = useState<ErrorAnalysis | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [retryAfter, setRetryAfter] = useState(0);

  // 页面加载时自动获取已有分析结果
  useEffect(() => {
    loadExistingAnalysis();
  }, [taskId]);

  // QPS 限制倒计时
  useEffect(() => {
    if (retryAfter > 0) {
      const timer = setInterval(() => {
        setRetryAfter((prev) => {
          if (prev <= 1) {
            clearInterval(timer);
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
      return () => clearInterval(timer);
    }
  }, [retryAfter]);

  const loadExistingAnalysis = async () => {
    try {
      const result = await getTaskAnalysis(workspaceId, taskId);
      setAnalysis(result);
    } catch (err: any) {
      // 404 表示没有分析结果，这是正常的
      if (err.response?.status !== 404) {
        console.error('Failed to load existing analysis:', err);
      }
    }
  };

  const handleAnalyze = async () => {
    try {
      setLoading(true);
      setError(null);
      setExpanded(true);

      // 安全修复：只传入 task_id，其他信息从后端数据库获取
      const result = await analyzeError({
        task_id: taskId,
      });

      setAnalysis({
        id: 0,
        task_id: taskId,
        error_type: result.error_type,
        root_cause: result.root_cause,
        solutions: result.solutions,
        prevention: result.prevention,
        severity: result.severity,
        analysis_duration: result.analysis_duration,
        created_at: new Date().toISOString(),
      });
    } catch (err: any) {
      if (err.response?.status === 429) {
        const retrySeconds = err.response?.data?.data?.retry_after || 10;
        setRetryAfter(retrySeconds);
        setError(err.response?.data?.message || '请求过于频繁');
      } else {
        setError(err.response?.data?.message || '分析失败');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleReanalyze = () => {
    setAnalysis(null);
    handleAnalyze();
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'critical':
        return styles.severityCritical;
      case 'high':
        return styles.severityHigh;
      case 'medium':
        return styles.severityMedium;
      case 'low':
        return styles.severityLow;
      default:
        return '';
    }
  };

  const getSeverityLabel = (severity: string) => {
    switch (severity) {
      case 'critical':
        return '严重';
      case 'high':
        return '高';
      case 'medium':
        return '中等';
      case 'low':
        return '低';
      default:
        return severity;
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        {!analysis && !loading && retryAfter === 0 && (
          <button className={styles.analyzeButton} onClick={handleAnalyze}>
            AI 分析
          </button>
        )}

        {!analysis && !loading && retryAfter > 0 && (
          <button className={styles.analyzeButtonDisabled} disabled>
            AI 分析（{retryAfter}秒后可用）
          </button>
        )}

        {analysis && !loading && (
          <>
            <button
              className={styles.toggleButton}
              onClick={() => setExpanded(!expanded)}
            >
              AI 分析 {expanded ? '▼' : '▶'}
            </button>
            {retryAfter === 0 && (
              <button className={styles.reanalyzeButton} onClick={handleReanalyze}>
                重新分析
              </button>
            )}
            {retryAfter > 0 && (
              <span className={styles.retryHint}>
                {retryAfter}秒后可重新分析
              </span>
            )}
          </>
        )}
      </div>

      {expanded && (
        <div className={styles.content}>
          {loading && (
            <div className={styles.loading}>
              <div className={styles.spinner}></div>
              <span>分析中，请稍候...</span>
            </div>
          )}

          {error && (
            <div className={styles.error}>
              <span className={styles.errorIcon}>⚠</span>
              <span>{error}</span>
            </div>
          )}

          {analysis && !loading && (
            <div className={styles.result}>
              <div className={styles.resultHeader}>
                <span className={styles.resultTitle}>AI 分析结果</span>
                {analysis.analysis_duration && (
                  <span className={styles.duration}>
                    分析耗时: {(analysis.analysis_duration / 1000).toFixed(1)}秒
                  </span>
                )}
              </div>

              <div className={styles.section}>
                <div className={styles.sectionTitle}>错误类型</div>
                <div className={styles.sectionContent}>{analysis.error_type}</div>
              </div>

              <div className={styles.section}>
                <div className={styles.sectionTitle}>根本原因</div>
                <div className={styles.sectionContent}>{analysis.root_cause}</div>
              </div>

              <div className={styles.section}>
                <div className={styles.sectionTitle}>解决方案</div>
                <ol className={styles.solutionList}>
                  {analysis.solutions.map((solution, index) => (
                    <li key={index} className={styles.solutionItem}>
                      {solution}
                    </li>
                  ))}
                </ol>
              </div>

              <div className={styles.section}>
                <div className={styles.sectionTitle}>预防措施</div>
                <div className={styles.sectionContent}>{analysis.prevention}</div>
              </div>

              <div className={styles.section}>
                <div className={styles.sectionTitle}>严重程度</div>
                <div className={styles.sectionContent}>
                  <span className={`${styles.severityBadge} ${getSeverityColor(analysis.severity)}`}>
                    {getSeverityLabel(analysis.severity)}
                  </span>
                </div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default AIErrorAnalysis;
