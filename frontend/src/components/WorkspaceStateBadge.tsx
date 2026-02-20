import React from 'react';
import styles from './WorkspaceStateBadge.module.css';

export type WorkspaceState = 
  | 'created'
  | 'planning'
  | 'plan_done'
  | 'waiting_apply'
  | 'applying'
  | 'completed'
  | 'failed';

interface WorkspaceStateBadgeProps {
  state: WorkspaceState;
  size?: 'small' | 'medium' | 'large';
}

const stateConfig: Record<WorkspaceState, { label: string; color: string; icon: string }> = {
  created: {
    label: '已创建',
    color: 'gray',
    icon: '○',
  },
  planning: {
    label: 'Planning',
    color: 'blue',
    icon: '⟳',
  },
  plan_done: {
    label: 'Plan完成',
    color: 'green',
    icon: '✓',
  },
  waiting_apply: {
    label: '等待Apply',
    color: 'orange',
    icon: '⏸',
  },
  applying: {
    label: 'Applying',
    color: 'blue',
    icon: '⟳',
  },
  completed: {
    label: '已完成',
    color: 'success',
    icon: '✓',
  },
  failed: {
    label: '失败',
    color: 'error',
    icon: '✗',
  },
};

const WorkspaceStateBadge: React.FC<WorkspaceStateBadgeProps> = ({ 
  state, 
  size = 'medium' 
}) => {
  const config = stateConfig[state] || stateConfig.created;

  return (
    <span 
      className={`${styles.badge} ${styles[config.color]} ${styles[size]}`}
      title={config.label}
    >
      <span className={styles.icon}>{config.icon}</span>
      <span className={styles.label}>{config.label}</span>
    </span>
  );
};

export default WorkspaceStateBadge;
