import React, { useState, useEffect } from 'react';
import api from '../services/api';
import styles from './StageLogViewer.module.css';

interface Props {
  taskId: number;
  taskType: 'plan' | 'apply';
}

interface Stage {
  name: string;
  displayName: string;
  logs: string;
  startTime?: string;
  endTime?: string;
}

const StageLogViewer: React.FC<Props> = ({ taskId, taskType }) => {
  const [stages, setStages] = useState<Stage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedStage, setSelectedStage] = useState<string>('all');

  useEffect(() => {
    fetchAndParseLogs();
  }, [taskId, taskType]);

  const fetchAndParseLogs = async () => {
    setLoading(true);
    setError(null);
    
    try {
      // 从URL获取workspaceId
      const pathParts = window.location.pathname.split('/');
      const workspaceId = pathParts[2];
      
      // 获取任务详情
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}`);
      const task = data.task || data;
      
      // 根据taskType获取对应的日志
      let logText = '';
      if (taskType === 'plan') {
        logText = task.plan_output || '';
      } else if (taskType === 'apply') {
        logText = task.apply_output || '';
      }
      
      console.log('[StageLogViewer] Task type:', taskType, 'Log length:', logText.length);
      
      if (!logText) {
        console.log('[StageLogViewer] No logs found, task:', task);
        // 如果任务被取消且没有日志，说明是在pending状态取消的
        if (task.status === 'cancelled') {
          setError('任务在执行前被取消，未生成日志');
        } else {
          setError('No logs available for this task');
        }
        setStages([]);
        return;
      }
      
      const parsedStages = parseStages(logText);
      console.log('[StageLogViewer] Parsed stages:', parsedStages.length, parsedStages.map(s => s.name));
      setStages(parsedStages);
    } catch (err: any) {
      console.error('[StageLogViewer] Error:', err);
      setError(err.message || 'Failed to fetch logs');
    } finally {
      setLoading(false);
    }
  };

  const parseStages = (logText: string): Stage[] => {
    const stages: Stage[] = [];
    const lines = logText.split('\n');
    
    let currentStage: Stage | null = null;
    let currentLogs: string[] = [];
    
    // 跳过 "=== PLAN OUTPUT ===" 或 "=== APPLY OUTPUT ===" 标题
    let startIndex = 0;
    if (lines[0]?.includes('=== PLAN OUTPUT ===') || lines[0]?.includes('=== APPLY OUTPUT ===')) {
      startIndex = 1;
    }
    
    for (let i = startIndex; i < lines.length; i++) {
      const line = lines[i];
      
      // 匹配阶段开始标记
      const beginMatch = line.match(/^========== (\w+) BEGIN at (.+) ==========$/);
      if (beginMatch) {
        // 保存上一个阶段
        if (currentStage) {
          currentStage.logs = currentLogs.join('\n');
          stages.push(currentStage);
        }
        
        // 创建新阶段
        const stageName = beginMatch[1].toLowerCase();
        currentStage = {
          name: stageName,
          displayName: getStageDisplayName(stageName),
          logs: '',
          startTime: beginMatch[2],
        };
        currentLogs = [line]; // 包含BEGIN标记
        continue;
      }
      
      // 匹配阶段结束标记
      const endMatch = line.match(/^========== (\w+) END at (.+) ==========$/);
      if (endMatch && currentStage) {
        currentLogs.push(line); // 包含END标记
        currentStage.endTime = endMatch[2];
        currentStage.logs = currentLogs.join('\n');
        stages.push(currentStage);
        currentStage = null;
        currentLogs = [];
        continue;
      }
      
      // 普通日志行
      if (currentStage) {
        currentLogs.push(line);
      }
    }
    
    // 处理最后一个阶段（如果没有END标记）
    if (currentStage && currentLogs.length > 0) {
      currentStage.logs = currentLogs.join('\n');
      stages.push(currentStage);
    }
    
    return stages;
  };

  const getStageDisplayName = (stageName: string): string => {
    const nameMap: Record<string, string> = {
      'pending': 'Pending',
      'fetching': 'Fetching',
      'pre_plan': 'Pre-Plan',
      'init': 'Init',
      'planning': 'Planning',
      'post_plan': 'Post-Plan',
      'cost_estimation': 'Cost Estimation',
      'policy_check': 'Policy Check',
      'pre_apply': 'Pre-Apply',
      'restoring_plan': 'Restoring Plan',
      'applying': 'Applying',
      'post_apply': 'Post-Apply',
      'saving_plan': 'Saving Plan',
      'saving_state': 'Saving State',
    };
    return nameMap[stageName] || stageName;
  };

  // 获取所有可能的阶段（根据任务类型）
  const getAllPossibleStages = (): string[] => {
    if (taskType === 'plan') {
      return [
        'pending',
        'fetching',
        'pre_plan',
        'init',
        'planning',
        'post_plan',
        'cost_estimation',
        'policy_check',
        'saving_plan',
      ];
    } else {
      return [
        'pending',
        'fetching',
        'init',
        'restoring_plan',
        'pre_apply',
        'applying',
        'post_apply',
        'saving_state',
      ];
    }
  };

  // 检查阶段是否有日志
  const hasStageLog = (stageName: string): boolean => {
    return stages.some(s => s.name === stageName);
  };

  const getDisplayLogs = (): string => {
    if (selectedStage === 'all') {
      return stages.map(s => s.logs).join('\n\n');
    }
    
    const stage = stages.find(s => s.name === selectedStage);
    return stage ? stage.logs : '';
  };

  const handleDownload = () => {
    window.open(
      `http://localhost:8080/api/v1/tasks/${taskId}/logs/download?type=${taskType}`,
      '_blank'
    );
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
    // 判断是否是取消任务的情况
    const isCancelled = error.includes('任务已取消');
    
    return (
      <div className={styles.container}>
        <div className={isCancelled ? styles.info : styles.error}>
          <span>{isCancelled ? '' : '❌ '}{error}</span>
          {!isCancelled && (
            <button onClick={fetchAndParseLogs} className={styles.retryBtn}>
              重试
            </button>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.tabs}>
          <button
            className={selectedStage === 'all' ? styles.active : ''}
            onClick={() => setSelectedStage('all')}
          >
            全部
          </button>
          {getAllPossibleStages().map(stageName => {
            const hasLog = hasStageLog(stageName);
            return (
              <button
                key={stageName}
                className={`${selectedStage === stageName ? styles.active : ''} ${!hasLog ? styles.disabled : ''}`}
                onClick={() => hasLog && setSelectedStage(stageName)}
                disabled={!hasLog}
                title={hasLog ? '' : '此阶段未执行'}
              >
                {getStageDisplayName(stageName)}
              </button>
            );
          })}
        </div>
        <button className={styles.downloadBtn} onClick={handleDownload}>
          ⬇ 下载日志
        </button>
      </div>
      
      <div className={styles.logContent}>
        <pre>{getDisplayLogs()}</pre>
      </div>
      
      {selectedStage !== 'all' && (
        <div className={styles.stageInfo}>
          {stages.find(s => s.name === selectedStage)?.startTime && (
            <span>
              开始: {stages.find(s => s.name === selectedStage)?.startTime}
            </span>
          )}
          {stages.find(s => s.name === selectedStage)?.endTime && (
            <span>
              结束: {stages.find(s => s.name === selectedStage)?.endTime}
            </span>
          )}
        </div>
      )}
    </div>
  );
};

export default StageLogViewer;
