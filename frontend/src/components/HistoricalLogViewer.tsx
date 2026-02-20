import React, { useState, useEffect } from 'react';
import api from '../services/api';
import styles from './HistoricalLogViewer.module.css';

interface Props {
  taskId: number;
}

const HistoricalLogViewer: React.FC<Props> = ({ taskId }) => {
  const [logs, setLogs] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [logType, setLogType] = useState<'all' | 'plan' | 'apply'>('all');

  useEffect(() => {
    fetchLogs();
  }, [taskId, logType]);

  const fetchLogs = async () => {
    setLoading(true);
    setError(null);
    
    try {
      // 使用fetch直接调用，因为需要text格式
      const response = await fetch(
        `http://localhost:8080/api/v1/tasks/${taskId}/logs?type=${logType}&format=text`
      );
      
      if (!response.ok) {
        throw new Error('Failed to fetch logs');
      }
      
      const text = await response.text();
      setLogs(text);
    } catch (err: any) {
      setError(err.message || 'Failed to fetch logs');
    } finally {
      setLoading(false);
    }
  };

  const handleDownload = () => {
    window.open(`http://localhost:8080/api/v1/tasks/${taskId}/logs/download?type=${logType}`, '_blank');
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>
          <div className={styles.spinner}></div>
          <span>加载日志中...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          <span>❌ 加载失败: {error}</span>
          <button onClick={fetchLogs} className={styles.retryBtn}>
            重试
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.tabs}>
          <button
            className={logType === 'all' ? styles.active : ''}
            onClick={() => setLogType('all')}
          >
            全部
          </button>
          <button
            className={logType === 'plan' ? styles.active : ''}
            onClick={() => setLogType('plan')}
          >
            Plan
          </button>
          <button
            className={logType === 'apply' ? styles.active : ''}
            onClick={() => setLogType('apply')}
          >
            Apply
          </button>
        </div>
        <button className={styles.downloadBtn} onClick={handleDownload}>
          ⬇ 下载日志
        </button>
      </div>
      
      <div className={styles.logContent}>
        <pre>{logs}</pre>
      </div>
    </div>
  );
};

export default HistoricalLogViewer;
