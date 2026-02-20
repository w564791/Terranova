import React, { useState } from 'react';
import styles from './StageProgress.module.css';

interface Stage {
  name: string;
  displayName: string;
  status: 'completed' | 'running' | 'error' | 'pending';
}

interface Props {
  taskType: 'plan' | 'apply' | 'plan_and_apply';
  taskStatus: string;
  currentStage?: string;
  errorStage?: string;
  onViewModeChange?: (mode: 'plan' | 'apply') => void; // 新增：通知父组件viewMode变化
}

const StageProgress: React.FC<Props> = ({ taskType, taskStatus, currentStage, errorStage, onViewModeChange }) => {
  // 对于plan_and_apply类型，允许用户切换查看plan或apply阶段
  const [viewMode, setViewMode] = useState<'plan' | 'apply'>('plan');
  
  // 当viewMode改变时，通知父组件
  const handleViewModeChange = (mode: 'plan' | 'apply') => {
    setViewMode(mode);
    if (onViewModeChange) {
      onViewModeChange(mode);
    }
  };
  // 定义Plan阶段
  const planStages: Stage[] = [
    { name: 'fetching', displayName: 'Fetching', status: 'pending' },
    { name: 'init', displayName: 'Init', status: 'pending' },
    { name: 'planning', displayName: 'Planning', status: 'pending' },
    { name: 'saving_plan', displayName: 'Saving Plan', status: 'pending' },
  ];

  // 定义Apply阶段
  const applyStages: Stage[] = [
    { name: 'fetching', displayName: 'Fetching', status: 'pending' },
    { name: 'init', displayName: 'Init', status: 'pending' },
    { name: 'restoring_plan', displayName: 'Restoring Plan', status: 'pending' },
    { name: 'applying', displayName: 'Applying', status: 'pending' },
    { name: 'saving_state', displayName: 'Saving State', status: 'pending' },
  ];

  // 根据任务类型选择阶段
  let stages = taskType === 'apply' ? applyStages : planStages;
  
  // 对于plan_and_apply，根据viewMode决定显示哪些阶段
  if (taskType === 'plan_and_apply') {
    stages = viewMode === 'plan' ? planStages : applyStages;
  }

  // 更新阶段状态
  stages = stages.map(stage => {
    // 如果有错误阶段，该阶段及之后的都标记为error或pending
    if (errorStage) {
      if (stage.name === errorStage) {
        return { ...stage, status: 'error' as const };
      }
      // 检查是否在错误阶段之前
      const errorIndex = stages.findIndex(s => s.name === errorStage);
      const currentIndex = stages.findIndex(s => s.name === stage.name);
      if (currentIndex < errorIndex) {
        return { ...stage, status: 'completed' as const };
      }
      return { ...stage, status: 'pending' as const };
    }

    // 如果是当前阶段
    if (currentStage && stage.name === currentStage) {
      return { ...stage, status: 'running' as const };
    }

    // 根据任务状态判断
    if (taskStatus === 'success') {
      return { ...stage, status: 'completed' as const };
    }

    if (taskStatus === 'failed') {
      // 如果任务失败，需要根据当前阶段判断哪些已完成
      if (currentStage) {
        const currentIndex = stages.findIndex(s => s.name === currentStage);
        const stageIndex = stages.findIndex(s => s.name === stage.name);
        if (stageIndex < currentIndex) {
          return { ...stage, status: 'completed' as const };
        } else if (stageIndex === currentIndex) {
          return { ...stage, status: 'error' as const };
        }
      }
      return { ...stage, status: 'pending' as const };
    }

    if (taskStatus === 'running') {
      // 运行中，需要根据当前阶段判断
      if (currentStage) {
        const currentIndex = stages.findIndex(s => s.name === currentStage);
        const stageIndex = stages.findIndex(s => s.name === stage.name);
        if (stageIndex < currentIndex) {
          return { ...stage, status: 'completed' as const };
        } else if (stageIndex === currentIndex) {
          return { ...stage, status: 'running' as const };
        }
      }
      return { ...stage, status: 'pending' as const };
    }

    if (taskStatus === 'plan_completed') {
      // Plan完成，所有Plan阶段都完成
      return { ...stage, status: 'completed' as const };
    }

    if (taskStatus === 'applied') {
      // Apply完成，所有阶段都完成
      return { ...stage, status: 'completed' as const };
    }

    return stage;
  });

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'completed':
        return '✓';
      case 'running':
        return null; // 使用CSS绘制的旋转箭头
      case 'error':
        return '✗';
      default:
        return '○';
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.progressWrapper}>
        {/* 左侧箭头 - 固定在最左侧 */}
        {taskType === 'plan_and_apply' ? (
          <button
            className={styles.arrowButton}
            onClick={() => handleViewModeChange('plan')}
            disabled={viewMode === 'plan'}
            title="切换到Plan阶段"
          >
            ←
          </button>
        ) : (
          <div className={styles.arrowPlaceholder}></div>
        )}
        
        <div className={styles.stageList}>
          {stages.map((stage, index) => (
            <div key={stage.name} className={styles.stageItem}>
              <div className={`${styles.stageIcon} ${styles[stage.status]}`}>
                {stage.status === 'running' ? (
                  <div className={styles.spinner}></div>
                ) : (
                  getStatusIcon(stage.status)
                )}
              </div>
              <div className={styles.stageContent}>
                <div className={styles.stageName}>{stage.displayName}</div>
              </div>
              {index < stages.length - 1 && (
                <div className={`${styles.connector} ${
                  stage.status === 'completed' ? styles.connectorCompleted : ''
                }`} />
              )}
            </div>
          ))}
        </div>
        
        {/* 右侧箭头 - 固定在最右侧 */}
        {taskType === 'plan_and_apply' ? (
          <button
            className={styles.arrowButton}
            onClick={() => handleViewModeChange('apply')}
            disabled={viewMode === 'apply'}
            title="切换到Apply阶段"
          >
            →
          </button>
        ) : (
          <div className={styles.arrowPlaceholder}></div>
        )}
      </div>
    </div>
  );
};

export default StageProgress;
