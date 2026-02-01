import React, { useEffect, useRef, useState } from 'react';
import { useTerraformOutput } from '../hooks/useTerraformOutput';
import ConfirmDialog from './ConfirmDialog';
import styles from './TerraformOutputViewer.module.css';

interface Props {
  taskId: number;
  onStageChange?: (stage: string) => void; // 新增：通知父组件当前阶段变化
  currentTaskStage?: string; // 从父组件接收当前任务阶段（来自API）
}

const TerraformOutputViewer: React.FC<Props> = ({ taskId, onStageChange, currentTaskStage }) => {
  const { lines, isConnected, isCompleted, error } = useTerraformOutput(taskId);
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [taskStatus, setTaskStatus] = useState<string>('');
  const [planTaskId, setPlanTaskId] = useState<number | null>(null);
  const [showCancelDialog, setShowCancelDialog] = useState(false);
  const [filterStage, setFilterStage] = useState<string>('all');
  const [currentStage, setCurrentStage] = useState<string>('fetching');
  const [availableStages, setAvailableStages] = useState<Set<string>>(new Set(['fetching']));
  const [userSelectedAll, setUserSelectedAll] = useState(false); // 用户是否手动选择了"全部"

  // 获取任务状态
  useEffect(() => {
    const fetchTaskStatus = async () => {
      try {
        const pathParts = window.location.pathname.split('/');
        const workspaceId = pathParts[2];
        const response = await fetch(`http://localhost:8080/api/v1/workspaces/${workspaceId}/tasks/${taskId}`, {
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('token')}`
          }
        });
        const data = await response.json();
        const task = data.task || data;
        setTaskStatus(task.status);
        setPlanTaskId(task.plan_task_id || null);
      } catch (err) {
        console.error('Failed to fetch task status:', err);
      }
    };
    
    fetchTaskStatus();
    
    // 如果是waiting状态，定期检查
    const interval = setInterval(() => {
      if (taskStatus === 'waiting') {
        fetchTaskStatus();
      }
    }, 3000);
    
    return () => clearInterval(interval);
  }, [taskId, taskStatus]);

  // 自动滚动到底部
  useEffect(() => {
    if (autoScroll) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [lines, autoScroll]);

  // 检测当前阶段并自动切换tab，同时收集所有出现过的阶段
  useEffect(() => {
    // 收集所有出现过的阶段
    const stages = new Set<string>();
    
    for (const line of lines) {
      if (line.type === 'stage_marker' && line.status === 'begin') {
        const stage = line.stage?.toLowerCase() || '';
        stages.add(stage);
      }
    }
    
    setAvailableStages(stages);
    
    // 优先使用从API获取的currentTaskStage（与structured mode保持一致）
    // 如果没有提供，则从WebSocket日志中解析
    let latestActiveStage = currentTaskStage || 'fetching';
    
    // 如果没有从API获取到stage，则从WebSocket日志中解析
    if (!currentTaskStage) {
      for (const line of lines) {
        if (line.type === 'stage_marker' && line.status === 'begin') {
          const stage = line.stage?.toLowerCase() || '';
          latestActiveStage = stage;
        }
      }
    }
    
    // 使用最新的活跃阶段
    if (latestActiveStage !== currentStage) {
      console.log('[TerraformOutputViewer] Stage changed from', currentStage, 'to', latestActiveStage, '(from API:', !!currentTaskStage, ')');
      setCurrentStage(latestActiveStage);
      
      // 通知父组件阶段变化
      if (onStageChange) {
        onStageChange(latestActiveStage);
      }
      
      // 自动切换逻辑：
      // 1. 如果用户手动选择了"全部"，不自动切换
      // 2. 如果用户选择了具体阶段，自动跟随当前阶段切换
      // 3. 如果用户没有操作（filterStage === 'all' 且 !userSelectedAll），自动切换
      if (!userSelectedAll) {
        console.log('[TerraformOutputViewer] Auto-switching to stage:', latestActiveStage);
        setFilterStage(latestActiveStage);
      }
    }
  }, [lines, currentStage, userSelectedAll, onStageChange, currentTaskStage]);

  // 检测用户是否手动滚动
  const handleScroll = () => {
    if (!containerRef.current) return;
    
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    
    setAutoScroll(isAtBottom);
  };

  // 如果是waiting状态，显示等待提示
  if (taskStatus === 'waiting') {
    return (
      <div className={styles.container}>
        <div className={styles.header}>
          <span className={styles.title}>Terraform Output</span>
          <div className={styles.status}>
            <span className={styles.waiting}>
              <span className={styles.pulse}>●</span> Waiting
            </span>
          </div>
        </div>
        
        <div className={styles.waitingContainer}>
          <div className={styles.waitingIcon}>⏳</div>
          <h3 className={styles.waitingTitle}>Waiting for Plan to Complete</h3>
          <p className={styles.waitingDesc}>
            This Apply task is waiting for the associated Plan task to complete successfully.
          </p>
          {planTaskId && (
            <p className={styles.waitingLink}>
              <a href={`/workspaces/${window.location.pathname.split('/')[2]}/tasks/${planTaskId}`}>
                View Plan Task #{planTaskId} →
              </a>
            </p>
          )}
          <div className={styles.waitingActions}>
            <button 
              className={styles.cancelButton}
              onClick={() => setShowCancelDialog(true)}
            >
              Cancel Task
            </button>
          </div>
        </div>
        
        <ConfirmDialog
          isOpen={showCancelDialog}
          title="Cancel Task"
          message="This operation cannot be undone. Are you sure?"
          confirmText="Yes, confirm"
          cancelText="Cancel"
          onConfirm={async () => {
            try {
              const pathParts = window.location.pathname.split('/');
              const workspaceId = pathParts[2];
              await fetch(`http://localhost:8080/api/v1/workspaces/${workspaceId}/tasks/${taskId}/cancel`, {
                method: 'POST',
                headers: {
                  'Authorization': `Bearer ${localStorage.getItem('token')}`
                }
              });
              // 不刷新页面，只更新状态
              setTaskStatus('canceled');
              setShowCancelDialog(false);
            } catch (err) {
              console.error('Failed to cancel task:', err);
              setShowCancelDialog(false);
            }
          }}
          onCancel={() => setShowCancelDialog(false)}
          type="warning"
        />
      </div>
    );
  }

  // 获取阶段显示名称
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
    // 从taskStatus判断任务类型
    // 这里简化处理，实际应该从task信息获取
    // 暂时返回plan阶段的所有可能阶段
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
  };

  // 检查阶段是否有日志
  const hasStageLog = (stageName: string): boolean => {
    return availableStages.has(stageName);
  };

  // 过滤日志行
  const filteredLines = filterStage === 'all' ? lines : lines.filter((line, index) => {
    // 如果是stage_marker，总是显示
    if (line.type === 'stage_marker') return true;
    
    // 找到当前行所属的阶段
    let currentPhase = 'fetching'; // 默认阶段
    for (let i = index; i >= 0; i--) {
      if (lines[i].type === 'stage_marker' && lines[i].status === 'begin') {
        currentPhase = lines[i].stage?.toLowerCase() || 'fetching';
        break;
      }
    }
    
    return currentPhase === filterStage;
  });

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.tabs}>
          <button
            onClick={() => {
              setFilterStage('all');
              setUserSelectedAll(true); // 用户手动选择了"全部"，停止自动切换
              console.log('[TerraformOutputViewer] User selected "All", auto-switch disabled');
            }}
            className={filterStage === 'all' ? styles.active : ''}
          >
            全部
          </button>
          {getAllPossibleStages().map(stageName => {
            const hasLog = hasStageLog(stageName);
            return (
              <button
                key={stageName}
                className={`${filterStage === stageName ? styles.active : ''} ${!hasLog ? styles.disabled : ''}`}
                onClick={() => {
                  if (hasLog) {
                    setFilterStage(stageName);
                    setUserSelectedAll(false); // 用户选择了具体阶段，恢复自动切换
                    console.log('[TerraformOutputViewer] User selected stage:', stageName, ', auto-switch enabled');
                  }
                }}
                disabled={!hasLog}
                title={hasLog ? '' : '此阶段未执行'}
              >
                {getStageDisplayName(stageName)}
              </button>
            );
          })}
        </div>
        
        <div className={styles.headerRight}>
          <div className={styles.status}>
            {isConnected && !isCompleted && (
              <span className={styles.running}>
                <span className={styles.pulse}>●</span> Running
              </span>
            )}
            {isCompleted && (
              <span className={styles.completed}>✓ Completed</span>
            )}
            {!isConnected && !isCompleted && (
              <span className={styles.disconnected}>
                ○ {error || 'Connecting...'}
              </span>
            )}
            <span className={styles.lineCount}>
              {filterStage === 'all' ? lines.length : filteredLines.length} lines
            </span>
          </div>
        </div>
      </div>
      
      <div 
        ref={containerRef}
        className={styles.output}
        onScroll={handleScroll}
      >
        {filteredLines.map((msg, index) => {
          // 阶段标记特殊样式
          if (msg.type === 'stage_marker') {
            return (
              <div key={index} className={styles.stageMarker}>
                <span className={styles.stageIcon}>
                  {msg.status === 'begin' ? '▶' : '✓'}
                </span>
                <span className={styles.stageName}>{msg.stage}</span>
                <span className={styles.stageStatus}>{msg.status}</span>
                <span className={styles.stageTime}>
                  {msg.timestamp ? new Date(msg.timestamp).toLocaleTimeString() : ''}
                </span>
              </div>
            );
          }
          
          // 普通输出行
          return (
            <div 
              key={index} 
              className={`${styles.line} ${msg.type === 'error' ? styles.error : ''}`}
            >
              <span className={styles.lineNum}>{msg.line_num || index + 1}</span>
              <span className={styles.content}>{msg.line}</span>
            </div>
          );
        })}
        <div ref={bottomRef} />
      </div>
      
      {!autoScroll && (
        <button 
          className={styles.scrollButton}
          onClick={() => {
            setAutoScroll(true);
            bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
          }}
        >
          ↓ 滚动到底部
        </button>
      )}
    </div>
  );
};

export default TerraformOutputViewer;
