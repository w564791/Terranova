import React, { useState, useEffect } from 'react';
import {
  Table,
  Button,
  Select,
  Switch,
  Space,
  Tag,
  Card,
  Empty,
  Alert,
  message,
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  LinkOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { useToast } from '../contexts/ToastContext';
import ConfirmDialog from './ConfirmDialog';

interface RunTrigger {
  id: number;
  source_workspace_id: string;
  target_workspace_id: string;
  enabled: boolean;
  trigger_condition: string;
  created_at: string;
  source_workspace?: {
    workspace_id: string;
    name: string;
    auto_apply: boolean;
  };
}

interface Workspace {
  workspace_id: string;
  name: string;
  auto_apply: boolean;
}

interface Props {
  workspaceId: string;
  workspaceAutoApply?: boolean;
}

const WorkspaceRunTriggerConfig: React.FC<Props> = ({ workspaceId, workspaceAutoApply }) => {
  const { success, error: showError, warning } = useToast();
  const [triggers, setTriggers] = useState<RunTrigger[]>([]);
  const [availableSources, setAvailableSources] = useState<Workspace[]>([]);
  const [loading, setLoading] = useState(false);
  
  // 内联添加表单状态
  const [showAddForm, setShowAddForm] = useState(false);
  const [selectedSource, setSelectedSource] = useState<string | undefined>(undefined);
  const [addEnabled, setAddEnabled] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  
  // 删除确认对话框状态
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deletingTrigger, setDeletingTrigger] = useState<RunTrigger | null>(null);
  const [deleting, setDeleting] = useState(false);

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  // 获取允许触发当前 workspace 的 triggers（当前 workspace 作为 target）
  const fetchTriggers = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/run-triggers/inbound`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        setTriggers(data.inbound_triggers || []);
      } else {
        console.error('Failed to fetch run triggers');
      }
    } catch (error) {
      console.error('Error fetching run triggers:', error);
      message.error('Failed to fetch run triggers');
    } finally {
      setLoading(false);
    }
  };

  // 获取可以作为触发源的 workspace 列表
  const fetchAvailableSources = async () => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/run-triggers/available-sources`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        setAvailableSources(data.available_workspaces || []);
      }
    } catch (error) {
      console.error('Failed to fetch available sources');
    }
  };

  useEffect(() => {
    fetchTriggers();
    fetchAvailableSources();
  }, [workspaceId]);

  const handleAdd = async () => {
    if (!selectedSource) {
      message.error('Please select a source workspace');
      return;
    }

    setSubmitting(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/run-triggers/inbound`, {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          source_workspace_id: selectedSource,
          enabled: addEnabled,
        }),
      });

      const data = await response.json();
      
      if (response.ok) {
        success('Run trigger created');
        if (data.warning) {
          warning(data.warning);
        }
        setShowAddForm(false);
        setSelectedSource(undefined);
        setAddEnabled(true);
        fetchTriggers();
        fetchAvailableSources();
      } else {
        showError(data.error || 'Failed to create run trigger');
      }
    } catch (error) {
      showError('Failed to create run trigger');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeleteClick = (trigger: RunTrigger) => {
    setDeletingTrigger(trigger);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingTrigger) return;

    setDeleting(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/run-triggers/${deletingTrigger.id}`, {
        method: 'DELETE',
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        success('Run trigger deleted');
        setDeleteDialogOpen(false);
        setDeletingTrigger(null);
        fetchTriggers();
        fetchAvailableSources();
      } else {
        showError('Failed to delete run trigger');
      }
    } catch (error) {
      showError('Failed to delete run trigger');
    } finally {
      setDeleting(false);
    }
  };

  const handleToggle = async (id: number, enabled: boolean) => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/run-triggers/${id}`, {
        method: 'PUT',
        headers: getAuthHeaders(),
        body: JSON.stringify({ enabled }),
      });
      if (response.ok) {
        success(`Run trigger ${enabled ? 'enabled' : 'disabled'}`);
        fetchTriggers();
      } else {
        showError('Failed to update run trigger');
      }
    } catch (error) {
      showError('Failed to update run trigger');
    }
  };

  const columns: ColumnsType<RunTrigger> = [
    {
      title: 'Source Workspace',
      key: 'source',
      render: (_: unknown, record: RunTrigger) => (
        <Space>
          <LinkOutlined />
          <span style={{ fontWeight: 500 }}>
            {record.source_workspace?.name || record.source_workspace_id}
          </span>
        </Space>
      ),
    },
    {
      title: 'Trigger Condition',
      dataIndex: 'trigger_condition',
      key: 'trigger_condition',
      render: (condition: string) => (
        <Tag color="blue">
          {condition === 'apply_success' ? 'After Apply Success' : condition}
        </Tag>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean, record: RunTrigger) => (
        <Switch
          checked={enabled}
          onChange={(checked) => handleToggle(record.id, checked)}
          checkedChildren="Enabled"
          unCheckedChildren="Disabled"
        />
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 100,
      render: (_: unknown, record: RunTrigger) => (
        <Button 
          type="text" 
          danger 
          icon={<DeleteOutlined />} 
          onClick={() => handleDeleteClick(record)}
        />
      ),
    },
  ];

  return (
    <>
      <Card
        title="Run Triggers"
        extra={
          <Button 
            type="primary" 
            icon={<PlusOutlined />} 
            onClick={() => setShowAddForm(true)}
            disabled={showAddForm}
          >
            Add Trigger
          </Button>
        }
      >
        <p style={{ color: '#666', marginBottom: 16 }}>
          Configure which workspaces are allowed to trigger this workspace.
          When a source workspace's apply completes successfully, it will automatically start a Plan+Apply task in this workspace.
        </p>

        {/* Auto Apply 警告 */}
        {workspaceAutoApply && triggers.length > 0 && (
          <Alert
            type="warning"
            showIcon
            icon={<WarningOutlined />}
            message="Auto Apply Warning"
            description={
              <div>
                <p>This workspace has <strong>Auto Apply</strong> enabled.</p>
                <p>
                  <strong>Risk:</strong> When triggered by other workspaces, this workspace will automatically apply changes
                  without manual confirmation. This could lead to unintended infrastructure changes.
                </p>
              </div>
            }
            style={{ marginBottom: 16 }}
          />
        )}

        {/* 内联添加表单 */}
        {showAddForm && (
          <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
            <h4 style={{ marginBottom: 16 }}>Allow Workspace to Trigger</h4>
            
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', marginBottom: 8, fontWeight: 500 }}>
                Source Workspace *
              </label>
              <Select
                style={{ width: '100%' }}
                placeholder="Select a workspace..."
                value={selectedSource}
                onChange={(value) => setSelectedSource(value)}
                showSearch
                optionFilterProp="children"
                options={availableSources.map((ws) => ({
                  value: ws.workspace_id,
                  label: ws.name,
                }))}
              />
              <p style={{ marginTop: 6, fontSize: 12, color: '#999' }}>
                Select the workspace that will be allowed to trigger this workspace.
              </p>
            </div>

            {/* 当前 workspace 有 auto_apply 时的警告 */}
            {workspaceAutoApply && selectedSource && (
              <Alert
                type="warning"
                showIcon
                message="Auto Apply Warning"
                description="This workspace has Auto Apply enabled. Triggered tasks will automatically apply changes without manual confirmation."
                style={{ marginBottom: 16 }}
              />
            )}

            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', marginBottom: 8, fontWeight: 500 }}>
                Enabled
              </label>
              <Switch
                checked={addEnabled}
                onChange={(checked) => setAddEnabled(checked)}
                checkedChildren="Yes"
                unCheckedChildren="No"
              />
            </div>

            <Space>
              <Button
                onClick={() => {
                  setShowAddForm(false);
                  setSelectedSource(undefined);
                  setAddEnabled(true);
                }}
              >
                Cancel
              </Button>
              <Button
                type="primary"
                onClick={handleAdd}
                disabled={!selectedSource || submitting}
                loading={submitting}
              >
                Add Trigger
              </Button>
            </Space>
          </Card>
        )}

        {/* 触发器列表 */}
        {triggers.length === 0 && !showAddForm ? (
          <Empty description="No workspaces are allowed to trigger this workspace" />
        ) : triggers.length > 0 ? (
          <Table
            columns={columns}
            dataSource={triggers}
            rowKey="id"
            loading={loading}
            pagination={false}
          />
        ) : null}
      </Card>

      {/* 删除确认对话框 */}
      <ConfirmDialog
        isOpen={deleteDialogOpen}
        title="Delete Run Trigger"
        message={`Are you sure you want to remove the trigger permission for workspace "${deletingTrigger?.source_workspace?.name || deletingTrigger?.source_workspace_id}"? This action cannot be undone.`}
        confirmText="Delete"
        cancelText="Cancel"
        onConfirm={handleDeleteConfirm}
        onCancel={() => {
          setDeleteDialogOpen(false);
          setDeletingTrigger(null);
        }}
        loading={deleting}
      />
    </>
  );
};

export default WorkspaceRunTriggerConfig;
