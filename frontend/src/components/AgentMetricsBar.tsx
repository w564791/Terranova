import React from 'react';
import styles from './AgentMetricsBar.module.css';

interface AgentMetricsBarProps {
  label: string;
  value: number; // 0-100
  unit?: string;
}

const AgentMetricsBar: React.FC<AgentMetricsBarProps> = ({ label, value, unit = '%' }) => {
  // 确保value在0-100范围内
  const normalizedValue = Math.min(Math.max(value, 0), 100);
  
  // 根据使用率确定颜色
  const getColor = (val: number): string => {
    if (val < 70) return '#52c41a'; // 绿色
    if (val < 90) return '#faad14'; // 黄色
    return '#ff4d4f'; // 红色
  };

  const color = getColor(normalizedValue);

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <span className={styles.label}>{label}</span>
        <span className={styles.value} style={{ color }}>
          {normalizedValue.toFixed(1)}{unit}
        </span>
      </div>
      <div className={styles.barContainer}>
        <div
          className={styles.barFill}
          style={{
            width: `${normalizedValue}%`,
            backgroundColor: color,
          }}
        />
      </div>
    </div>
  );
};

export default AgentMetricsBar;
