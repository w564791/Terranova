import React from 'react';
import { Tag, Tooltip } from 'antd';
import { CheckCircleOutlined, ExclamationCircleOutlined, QuestionCircleOutlined, ClockCircleOutlined } from '@ant-design/icons';
import type { ResourceDriftStatus } from '../../services/drift';

type DriftStatus = 'synced' | 'drifted' | 'unapplied' | 'unknown';

interface DriftStatusTagProps {
  status: DriftStatus;
  driftedCount?: number;
  lastCheckAt?: string | null;
  compact?: boolean;
}

const statusConfig: Record<DriftStatus, {
  color: string;
  bgColor: string;
  icon: React.ReactNode;
  text: string;
  description: string;
}> = {
  synced: {
    color: '#52c41a',
    bgColor: '#f6ffed',
    icon: <CheckCircleOutlined />,
    text: '已同步',
    description: '资源状态与配置一致',
  },
  drifted: {
    color: '#faad14',
    bgColor: '#fffbe6',
    icon: <ExclamationCircleOutlined />,
    text: '有漂移',
    description: '资源实际状态与配置不一致',
  },
  unapplied: {
    color: '#8c8c8c',
    bgColor: '#fafafa',
    icon: <ClockCircleOutlined />,
    text: '未应用',
    description: '资源配置已修改但尚未应用',
  },
  unknown: {
    color: '#d9d9d9',
    bgColor: '#fafafa',
    icon: <QuestionCircleOutlined />,
    text: '未知',
    description: '尚未进行 Drift 检测',
  },
};

const DriftStatusTag: React.FC<DriftStatusTagProps> = ({
  status,
  driftedCount,
  lastCheckAt,
  compact = false,
}) => {
  const config = statusConfig[status] || statusConfig.unknown;

  const tooltipContent = (
    <div>
      <div>{config.description}</div>
      {driftedCount !== undefined && driftedCount > 0 && (
        <div style={{ marginTop: 4 }}>漂移子资源数: {driftedCount}</div>
      )}
      {lastCheckAt && (
        <div style={{ marginTop: 4, fontSize: 12, color: '#999' }}>
          上次检测: {new Date(lastCheckAt).toLocaleString()}
        </div>
      )}
    </div>
  );

  if (compact) {
    return (
      <Tooltip title={tooltipContent}>
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: 20,
            height: 20,
            borderRadius: '50%',
            backgroundColor: config.bgColor,
            color: config.color,
            fontSize: 12,
          }}
        >
          {config.icon}
        </span>
      </Tooltip>
    );
  }

  return (
    <Tooltip title={tooltipContent}>
      <Tag
        icon={config.icon}
        color={config.color}
        style={{
          backgroundColor: config.bgColor,
          borderColor: config.color,
        }}
      >
        {config.text}
        {driftedCount !== undefined && driftedCount > 0 && ` (${driftedCount})`}
      </Tag>
    </Tooltip>
  );
};

// 从 ResourceDriftStatus 创建 DriftStatusTag 的辅助函数
export const createDriftStatusTag = (driftStatus: ResourceDriftStatus | null, compact = false) => {
  if (!driftStatus) {
    return <DriftStatusTag status="unknown" compact={compact} />;
  }

  return (
    <DriftStatusTag
      status={driftStatus.drift_status}
      driftedCount={driftStatus.drifted_children_count}
      lastCheckAt={driftStatus.last_check_at}
      compact={compact}
    />
  );
};

export default DriftStatusTag;
