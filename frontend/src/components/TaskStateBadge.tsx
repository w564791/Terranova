import React from 'react';
import styles from './TaskStateBadge.module.css';

export type TaskStatus = 'pending' | 'running' | 'success' | 'applied' | 'plan_completed' | 'failed' | 'cancelled';
export type TaskType = 'plan' | 'apply';

interface TaskStateBadgeProps {
  status: TaskStatus;
  type?: TaskType;
  size?: 'small' | 'medium' | 'large';
}

const statusConfig: Record<TaskStatus, { label: string; color: string; icon: string }> = {
  pending: {
    label: '等待中',
    color: 'teal',
    icon: '○',
  },
  running: {
    label: '执行中',
    color: 'blue',
    icon: '⟳',
  },
  success: {
    label: 'Planned',
    color: 'success',
    icon: '✓',
  },
  plan_completed: {
    label: 'Planned',
    color: 'success',
    icon: '✓',
  },
  applied: {
    label: 'Applied',
    color: 'success',
    icon: '✓',
  },
  failed: {
    label: 'Errored',
    color: 'error',
    icon: '✗',
  },
  cancelled: {
    label: '已取消',
    color: 'gray',
    icon: '⊘',
  },
};

const typeConfig: Record<TaskType, { label: string; color: string }> = {
  plan: {
    label: 'Plan',
    color: 'blue',
  },
  apply: {
    label: 'Apply',
    color: 'green',
  },
};

const TaskStateBadge: React.FC<TaskStateBadgeProps> = ({ 
  status, 
  type,
  size = 'medium' 
}) => {
  const config = statusConfig[status] || statusConfig.pending;

  return (
    <div className={styles.container}>
      {type && (
        <span className={`${styles.typeBadge} ${styles[typeConfig[type].color]} ${styles[size]}`}>
          {typeConfig[type].label}
        </span>
      )}
      <span 
        className={`${styles.statusBadge} ${styles[config.color]} ${styles[size]}`}
        title={config.label}
      >
        <span className={styles.icon}>{config.icon}</span>
        <span className={styles.label}>{config.label}</span>
      </span>
    </div>
  );
};

export default TaskStateBadge;
