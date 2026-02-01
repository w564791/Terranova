import React, { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import type { NodeProps } from '@xyflow/react';
import { Card, Typography, Tooltip } from 'antd';
import {
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  QuestionCircleOutlined,
} from '@ant-design/icons';
import styles from './ModuleNode.module.css';

const { Text } = Typography;

interface ModuleNodeData {
  type: string;
  instance_name: string;
  resource_name: string;
  is_linked: boolean;
  link_status: 'linked' | 'unlinked' | 'mismatch';
  module_source?: string;
  module_version?: string;
  config_complete: boolean;
  label?: string;
  [key: string]: unknown;
}

const ModuleNode: React.FC<NodeProps> = ({ data, selected }) => {
  const nodeData = data as ModuleNodeData;

  const getStatusIcon = () => {
    if (!nodeData.is_linked) {
      return (
        <Tooltip title="未关联平台 Module">
          <QuestionCircleOutlined style={{ color: '#999' }} />
        </Tooltip>
      );
    }
    if (nodeData.link_status === 'mismatch') {
      return (
        <Tooltip title="Module 版本不匹配">
          <ExclamationCircleOutlined style={{ color: '#faad14' }} />
        </Tooltip>
      );
    }
    if (nodeData.config_complete) {
      return (
        <Tooltip title="配置完整">
          <CheckCircleOutlined style={{ color: '#52c41a' }} />
        </Tooltip>
      );
    }
    return (
      <Tooltip title="配置不完整">
        <ExclamationCircleOutlined style={{ color: '#faad14' }} />
      </Tooltip>
    );
  };

  const getBorderStyle = () => {
    if (!nodeData.is_linked) {
      return { borderColor: '#d9d9d9', borderStyle: 'dashed' as const };
    }
    if (nodeData.config_complete) {
      return { borderColor: '#52c41a', borderStyle: 'solid' as const };
    }
    return { borderColor: '#faad14', borderStyle: 'solid' as const };
  };

  return (
    <div className={`${styles.nodeWrapper} ${selected ? styles.selected : ''}`}>
      {/* 四向连接点 - 使用 source 类型，React Flow 的 Loose 连接模式允许连接到任意 Handle */}
      
      {/* 左侧连接点 */}
      <Handle
        type="source"
        position={Position.Left}
        className={`${styles.handle} ${styles.handleLeft}`}
        id="left"
        isConnectable={true}
      />

      {/* 右侧连接点 */}
      <Handle
        type="source"
        position={Position.Right}
        className={`${styles.handle} ${styles.handleRight}`}
        id="right"
        isConnectable={true}
      />

      {/* 顶部连接点 */}
      <Handle
        type="source"
        position={Position.Top}
        className={`${styles.handle} ${styles.handleTop}`}
        id="top"
        isConnectable={true}
      />

      {/* 底部连接点 */}
      <Handle
        type="source"
        position={Position.Bottom}
        className={`${styles.handle} ${styles.handleBottom}`}
        id="bottom"
        isConnectable={true}
      />

      <Card
        size="small"
        className={styles.nodeCard}
        style={getBorderStyle()}
        styles={{ body: { padding: '4px 6px' } }}
      >
        <Tooltip title={`${nodeData.instance_name || nodeData.label}\n${nodeData.module_source || ''}`}>
          <div className={styles.nodeHeader}>
            <Text strong className={styles.nodeName}>
              {nodeData.instance_name || nodeData.label || 'Module'}
            </Text>
            {getStatusIcon()}
          </div>
        </Tooltip>
      </Card>
    </div>
  );
};

export default memo(ModuleNode);
