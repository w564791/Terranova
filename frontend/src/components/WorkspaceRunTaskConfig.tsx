import React, { useState, useEffect } from 'react';
import {
  Table,
  Button,
  Modal,
  Form,
  Select,
  Radio,
  Switch,
  message,
  Space,
  Popconfirm,
  Tag,
  Card,
  Empty,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  LinkOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

interface RunTask {
  run_task_id: string;
  name: string;
  description: string;
  endpoint_url: string;
  enabled: boolean;
  is_global?: boolean;
}

interface GlobalRunTask {
  run_task_id: string;
  name: string;
  description: string;
  endpoint_url: string;
  enabled: boolean;
  is_global: boolean;
  global_stages: string;
  global_enforcement_level: string;
  timeout_seconds: number;
  max_run_seconds: number;
  created_at: string;
}

interface WorkspaceRunTask {
  workspace_run_task_id: string;
  workspace_id: string;
  run_task_id: string;
  run_task?: RunTask;
  stage: string;
  enforcement_level: string;
  enabled: boolean;
  created_at: string;
}

interface Props {
  workspaceId: string;
}

const stageLabels: Record<string, string> = {
  pre_plan: 'Pre-plan',
  post_plan: 'Post-plan',
  pre_apply: 'Pre-apply',
  post_apply: 'Post-apply',
};

const enforcementLabels: Record<string, { label: string; color: string }> = {
  advisory: { label: 'Advisory', color: 'blue' },
  mandatory: { label: 'Mandatory', color: 'red' },
};

const WorkspaceRunTaskConfig: React.FC<Props> = ({ workspaceId }) => {
  const [workspaceRunTasks, setWorkspaceRunTasks] = useState<WorkspaceRunTask[]>([]);
  const [globalRunTasks, setGlobalRunTasks] = useState<GlobalRunTask[]>([]);
  const [availableRunTasks, setAvailableRunTasks] = useState<RunTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingTask, setEditingTask] = useState<WorkspaceRunTask | null>(null);
  const [form] = Form.useForm();

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  const fetchWorkspaceRunTasks = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/run-tasks`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        setWorkspaceRunTasks(data.workspace_run_tasks || []);
        setGlobalRunTasks(data.global_run_tasks || []);
      }
    } catch (error) {
      message.error('Failed to fetch workspace run tasks');
    } finally {
      setLoading(false);
    }
  };

  const fetchAvailableRunTasks = async () => {
    try {
      const response = await fetch('/api/v1/run-tasks', {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        // Filter out global run tasks from available list (they are auto-applied)
        setAvailableRunTasks((data.run_tasks || []).filter((t: RunTask) => t.enabled && !t.is_global));
      }
    } catch (error) {
      console.error('Failed to fetch run tasks');
    }
  };

  useEffect(() => {
    fetchWorkspaceRunTasks();
    fetchAvailableRunTasks();
  }, [workspaceId]);

  const handleAdd = () => {
    setEditingTask(null);
    form.resetFields();
    form.setFieldsValue({
      stage: 'post_plan',
      enforcement_level: 'advisory',
      enabled: true,
    });
    setModalVisible(true);
  };

  const handleEdit = (task: WorkspaceRunTask) => {
    setEditingTask(task);
    form.setFieldsValue({
      run_task_id: task.run_task_id,
      stage: task.stage,
      enforcement_level: task.enforcement_level,
      enabled: task.enabled,
    });
    setModalVisible(true);
  };

  const handleDelete = async (id: string) => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/run-tasks/${id}`, {
        method: 'DELETE',
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        message.success('Run task removed');
        fetchWorkspaceRunTasks();
      } else {
        message.error('Failed to remove run task');
      }
    } catch (error) {
      message.error('Failed to remove run task');
    }
  };

  const handleSubmit = async (values: { run_task_id: string; stage: string; enforcement_level: string; enabled: boolean }) => {
    try {
      const url = editingTask
        ? `/api/v1/workspaces/${workspaceId}/run-tasks/${editingTask.workspace_run_task_id}`
        : `/api/v1/workspaces/${workspaceId}/run-tasks`;
      const method = editingTask ? 'PUT' : 'POST';

      const response = await fetch(url, {
        method,
        headers: getAuthHeaders(),
        body: JSON.stringify(values),
      });

      if (response.ok) {
        message.success(editingTask ? 'Run task updated' : 'Run task added');
        setModalVisible(false);
        fetchWorkspaceRunTasks();
      } else {
        const data = await response.json();
        message.error(data.error || 'Failed to save');
      }
    } catch (error) {
      message.error('Failed to save');
    }
  };

  // 格式化阶段显示
  const formatStages = (stages: string) => {
    if (!stages) return '-';
    const stageMap: Record<string, string> = {
      pre_plan: 'Pre-plan',
      post_plan: 'Post-plan',
      pre_apply: 'Pre-apply',
      post_apply: 'Post-apply',
    };
    return stages.split(',').map(s => stageMap[s.trim()] || s).join(', ');
  };

  // Columns for global run tasks table
  const globalColumns: ColumnsType<GlobalRunTask> = [
    {
      title: 'Task Name',
      key: 'name',
      render: (_: unknown, record: GlobalRunTask) => (
        <Space>
          <LinkOutlined />
          <span style={{ fontWeight: 500 }}>{record.name}</span>
          <Tag color="blue">Global</Tag>
        </Space>
      ),
    },
    {
      title: 'Stages',
      dataIndex: 'global_stages',
      key: 'global_stages',
      render: (stages: string) => formatStages(stages),
    },
    {
      title: 'Enforcement',
      dataIndex: 'global_enforcement_level',
      key: 'global_enforcement_level',
      render: (level: string) => {
        const config = enforcementLabels[level];
        return config ? <Tag color={config.color}>{config.label}</Tag> : level || 'Advisory';
      },
    },
    {
      title: 'Status',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'green' : 'default'}>{enabled ? 'Enabled' : 'Disabled'}</Tag>
      ),
    },
  ];

  const columns: ColumnsType<WorkspaceRunTask> = [
    {
      title: 'Task Name',
      key: 'name',
      render: (_: unknown, record: WorkspaceRunTask) => (
        <Space>
          <LinkOutlined />
          <span style={{ fontWeight: 500 }}>{record.run_task?.name || record.run_task_id}</span>
        </Space>
      ),
    },
    {
      title: 'Stage',
      dataIndex: 'stage',
      key: 'stage',
      render: (stage: string) => stageLabels[stage] || stage,
    },
    {
      title: 'Enforcement',
      dataIndex: 'enforcement_level',
      key: 'enforcement_level',
      render: (level: string) => {
        const config = enforcementLabels[level];
        return config ? <Tag color={config.color}>{config.label}</Tag> : level;
      },
    },
    {
      title: 'Status',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'green' : 'default'}>{enabled ? 'Enabled' : 'Disabled'}</Tag>
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 120,
      render: (_: unknown, record: WorkspaceRunTask) => (
        <Space>
          <Button type="text" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          <Popconfirm
            title="Remove this run task?"
            onConfirm={() => handleDelete(record.workspace_run_task_id)}
          >
            <Button type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title="Run Tasks"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>
          Add Run Task
        </Button>
      }
    >
      <p style={{ color: '#666', marginBottom: 16 }}>
        Configure run tasks for this workspace. Run tasks allow external services to pass or fail Terraform runs.
      </p>

      {/* Global Run Tasks Section */}
      {globalRunTasks.length > 0 && (
        <div style={{ marginBottom: 24 }}>
          <h4 style={{ marginBottom: 12, color: '#1890ff' }}>
            🌐 Global Run Tasks (Auto-applied to all workspaces)
          </h4>
          <Table
            columns={globalColumns}
            dataSource={globalRunTasks}
            rowKey="run_task_id"
            loading={loading}
            pagination={false}
            size="small"
            style={{ marginBottom: 16 }}
          />
        </div>
      )}

      {/* Workspace-specific Run Tasks Section */}
      <h4 style={{ marginBottom: 12 }}>
        📋 Workspace-specific Run Tasks
      </h4>
      {workspaceRunTasks.length === 0 ? (
        <Empty description="No workspace-specific run tasks configured" />
      ) : (
        <Table
          columns={columns}
          dataSource={workspaceRunTasks}
          rowKey="workspace_run_task_id"
          loading={loading}
          pagination={false}
        />
      )}

      <Modal
        title={editingTask ? 'Edit Run Task' : 'Add Run Task'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={null}
        width={500}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="run_task_id"
            label="Run Task"
            rules={[{ required: true, message: 'Please select a run task' }]}
          >
            <Select
              placeholder="Select a run task"
              disabled={!!editingTask}
              options={availableRunTasks.map((t) => ({
                value: t.run_task_id,
                label: t.name,
              }))}
            />
          </Form.Item>

          <Form.Item
            name="stage"
            label="Run Stage"
            rules={[{ required: true }]}
          >
            <Radio.Group>
              <Space direction="vertical">
                <Radio value="pre_plan">Pre-plan - Before Terraform generates the plan</Radio>
                <Radio value="post_plan">Post-plan - After Terraform creates the plan</Radio>
                <Radio value="pre_apply">Pre-apply - Before Terraform applies the plan</Radio>
                <Radio value="post_apply">Post-apply - After Terraform applies the plan</Radio>
              </Space>
            </Radio.Group>
          </Form.Item>

          <Form.Item
            name="enforcement_level"
            label="Enforcement Level"
            rules={[{ required: true }]}
          >
            <Radio.Group>
              <Space direction="vertical">
                <Radio value="advisory">Advisory - Failed run tasks produce a warning</Radio>
                <Radio value="mandatory">Mandatory - Failed run tasks stop the run</Radio>
              </Space>
            </Radio.Group>
          </Form.Item>

          <Form.Item name="enabled" label="Enabled" valuePropName="checked">
            <Switch />
          </Form.Item>

          <Form.Item style={{ marginBottom: 0, textAlign: 'right' }}>
            <Space>
              <Button onClick={() => setModalVisible(false)}>Cancel</Button>
              <Button type="primary" htmlType="submit">
                {editingTask ? 'Update' : 'Add'}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default WorkspaceRunTaskConfig;
