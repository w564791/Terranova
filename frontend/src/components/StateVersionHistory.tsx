import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Table, Tag, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { Resizable } from 'react-resizable';
import {
  stateAPI,
  formatFileSize,
} from '../services/state';
import type { StateVersion } from '../services/state';
import { useToast } from '../contexts/ToastContext';
import styles from './StateVersionHistory.module.css';
import 'react-resizable/css/styles.css';

interface StateVersionHistoryProps {
  workspaceId: string;
}

// 可调整大小的表头组件
const ResizableTitle = (props: any) => {
  const { onResize, width, ...restProps } = props;

  if (!width) {
    return <th {...restProps} />;
  }

  return (
    <Resizable
      width={width}
      height={0}
      handle={
        <span
          className={styles.resizeHandle}
          onClick={(e) => e.stopPropagation()}
        />
      }
      onResize={onResize}
      draggableOpts={{ enableUserSelectHack: false }}
    >
      <th {...restProps} />
    </Resizable>
  );
};

export const StateVersionHistory: React.FC<StateVersionHistoryProps> = ({ workspaceId }) => {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [versions, setVersions] = useState<StateVersion[]>([]);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [currentVersion, setCurrentVersion] = useState(0);
  const [pagination, setPagination] = useState({ current: 1, pageSize: 10 });
  
  // 列宽状态
  const [columnWidths, setColumnWidths] = useState({
    version: 120,
    source: 150,
    created_at: 180,
    created_by: 120,
    description: 200,
    size_bytes: 100,
  });

  // 加载版本列表
  const loadVersions = async (page: number = 1, pageSize: number = 10) => {
    setLoading(true);
    try {
      const offset = (page - 1) * pageSize;
      const response = await stateAPI.getStateVersions(workspaceId, pageSize, offset);
      setVersions(response.versions || []);
      setTotal(response.total || 0);
      setCurrentVersion(response.current_version || 0);
      setPagination({ current: page, pageSize });
    } catch (error: any) {
      const errMsg = typeof error === 'string' ? error : (error?.message || '未知错误');
      showToast('加载 State 版本历史失败: ' + errMsg, 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadVersions();
  }, [workspaceId]);

  // 点击行跳转到预览页
  const handleRowClick = (record: StateVersion) => {
    navigate(`/workspaces/${workspaceId}/states/${record.version}`);
  };

  // 处理列宽调整
  const handleResize = (key: string) => (_e: any, { size }: { size: { width: number } }) => {
    setColumnWidths((prev) => ({
      ...prev,
      [key]: size.width,
    }));
  };

  // 表格列定义
  const columns: ColumnsType<StateVersion> = [
    {
      title: 'Version',
      dataIndex: 'version',
      key: 'version',
      width: columnWidths.version,
      onHeaderCell: () => ({
        width: columnWidths.version,
        onResize: handleResize('version'),
      } as any),
      render: (version: number, record: StateVersion) => (
        <div>
          <span style={{ fontWeight: 'bold' }}>#{version}</span>
          {version === currentVersion && (
            <Tag color="green" style={{ marginLeft: 8 }}>
              Current
            </Tag>
          )}
        </div>
      ),
    },
    {
      title: 'Source',
      dataIndex: 'import_source',
      key: 'import_source',
      width: columnWidths.source,
      onHeaderCell: () => ({
        width: columnWidths.source,
        onResize: handleResize('source'),
      } as any),
      render: (source: string, record: StateVersion) => {
        if (record.is_rollback && record.rollback_from_version) {
          return (
            <Tooltip title={`从版本 #${record.rollback_from_version} 回滚`}>
              <Tag color="orange">Rollback</Tag>
            </Tooltip>
          );
        }
        if (record.is_imported) {
          return <Tag color="blue">Imported</Tag>;
        }
        return <Tag color="default">System</Tag>;
      },
    },
    {
      title: 'Created At',
      dataIndex: 'created_at',
      key: 'created_at',
      width: columnWidths.created_at,
      onHeaderCell: () => ({
        width: columnWidths.created_at,
        onResize: handleResize('created_at'),
      } as any),
      render: (date: string) => new Date(date).toLocaleString('zh-CN'),
    },
    {
      title: 'Created By',
      dataIndex: 'created_by_name',
      key: 'created_by_name',
      width: columnWidths.created_by,
      onHeaderCell: () => ({
        width: columnWidths.created_by,
        onResize: handleResize('created_by'),
      } as any),
      render: (name: string, record: StateVersion) => {
        // 使用后端返回的 created_by_name 字段
        const displayName = name || 'System';
        // 如果有原始用户 ID，显示 tooltip
        if (record.created_by && record.created_by !== displayName) {
          return (
            <Tooltip title={`User ID: ${record.created_by}`}>
              <span style={{ cursor: 'help' }}>{displayName}</span>
            </Tooltip>
          );
        }
        return displayName;
      },
    },
    {
      title: 'Description',
      dataIndex: 'description',
      key: 'description',
      width: columnWidths.description,
      ellipsis: true,
      onHeaderCell: () => ({
        width: columnWidths.description,
        onResize: handleResize('description'),
      } as any),
      render: (desc: string) => desc || '-',
    },
    {
      title: 'Size',
      dataIndex: 'size_bytes',
      key: 'size_bytes',
      width: columnWidths.size_bytes,
      onHeaderCell: () => ({
        width: columnWidths.size_bytes,
        onResize: handleResize('size_bytes'),
      } as any),
      render: (size: number) => formatFileSize(size),
    },
  ];

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h3>State Version History</h3>
        <div className={styles.stats}>
          <span>Total: {total} versions</span>
          <span style={{ marginLeft: 16 }}>Current: #{currentVersion}</span>
        </div>
      </div>

      <Table
        columns={columns}
        dataSource={versions}
        loading={loading}
        rowKey="id"
        components={{
          header: {
            cell: ResizableTitle,
          },
        }}
        pagination={{
          current: pagination.current,
          pageSize: pagination.pageSize,
          total: total,
          showSizeChanger: true,
          showTotal: (total) => `Total ${total} versions`,
          onChange: (page, pageSize) => loadVersions(page, pageSize),
        }}
        onRow={(record) => ({
          onClick: () => handleRowClick(record),
          style: { cursor: 'pointer' },
        })}
        scroll={{ x: 900 }}
      />
    </div>
  );
};