import React, { useState, useEffect } from 'react';
import {
  Table,
  Button,
  Modal,
  Form,
  Select,
  Switch,
  message,
  Space,
  Popconfirm,
  Tag,
  Card,
  Empty,
  Checkbox,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  BellOutlined,
  SendOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

interface NotificationConfig {
  notification_id: string;
  name: string;
  description: string;
  notification_type: 'webhook' | 'lark_robot';
  endpoint_url: string;
  enabled: boolean;
  is_global?: boolean;
  global_events?: string;
}

interface GlobalNotification {
  notification_id: string;
  name: string;
  description: string;
  notification_type: 'webhook' | 'lark_robot';
  endpoint_url: string;
  enabled: boolean;
  is_global: boolean;
  global_events: string;
  created_at: string;
}

interface WorkspaceNotification {
  workspace_notification_id: string;
  workspace_id: string;
  notification_id: string;
  notification?: NotificationConfig;
  events: string;
  enabled: boolean;
  created_at: string;
}

interface Props {
  workspaceId: string;
}

const eventLabels: Record<string, string> = {
  task_created: 'Created',
  task_planning: 'Planning',
  task_planned: 'Planned',
  task_applying: 'Applying',
  task_completed: 'Completed',
  task_failed: 'Failed',
  task_cancelled: 'Cancelled',
  approval_required: 'Approval',
  drift_detected: 'Drift',
};

const WorkspaceNotificationConfig: React.FC<Props> = ({ workspaceId }) => {
  const [workspaceNotifications, setWorkspaceNotifications] = useState<WorkspaceNotification[]>([]);
  const [globalNotifications, setGlobalNotifications] = useState<GlobalNotification[]>([]);
  const [availableNotifications, setAvailableNotifications] = useState<NotificationConfig[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingNotification, setEditingNotification] = useState<WorkspaceNotification | null>(null);
  const [testing, setTesting] = useState<string | null>(null);
  const [form] = Form.useForm();

  const eventOptions = [
    { value: 'task_created', label: 'Task Created' },
    { value: 'task_planning', label: 'Task Planning' },
    { value: 'task_planned', label: 'Task Planned' },
    { value: 'task_applying', label: 'Task Applying' },
    { value: 'task_completed', label: 'Task Completed' },
    { value: 'task_failed', label: 'Task Failed' },
    { value: 'task_cancelled', label: 'Task Cancelled' },
    { value: 'approval_required', label: 'Approval Required' },
    { value: 'drift_detected', label: 'Drift Detected' },
  ];

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  const fetchWorkspaceNotifications = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/notifications`, {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        console.log('Workspace notifications API response:', data);
        setWorkspaceNotifications(data.workspace_notifications || []);
        setGlobalNotifications(data.global_notifications || []);
        console.log('Global notifications set:', data.global_notifications);
      } else {
        console.error('Failed to fetch workspace notifications:', response.status, response.statusText);
      }
    } catch (error) {
      console.error('Error fetching workspace notifications:', error);
      message.error('Failed to fetch workspace notifications');
    } finally {
      setLoading(false);
    }
  };

  const fetchAvailableNotifications = async () => {
    try {
      const response = await fetch('/api/v1/notifications', {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        // Filter out global notifications from available list (they are auto-applied)
        setAvailableNotifications((data.notifications || []).filter((n: NotificationConfig) => n.enabled && !n.is_global));
      }
    } catch (error) {
      console.error('Failed to fetch notifications');
    }
  };

  useEffect(() => {
    fetchWorkspaceNotifications();
    fetchAvailableNotifications();
  }, [workspaceId]);

  const handleAdd = () => {
    setEditingNotification(null);
    form.resetFields();
    form.setFieldsValue({
      events: ['task_completed', 'task_failed'],
      enabled: true,
    });
    setModalVisible(true);
  };

  const handleEdit = (notification: WorkspaceNotification) => {
    setEditingNotification(notification);
    form.setFieldsValue({
      notification_id: notification.notification_id,
      events: notification.events ? notification.events.split(',') : [],
      enabled: notification.enabled,
    });
    setModalVisible(true);
  };

  const handleDelete = async (id: string) => {
    try {
      const response = await fetch(`/api/v1/workspaces/${workspaceId}/notifications/${id}`, {
        method: 'DELETE',
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        message.success('Notification removed');
        fetchWorkspaceNotifications();
      } else {
        message.error('Failed to remove notification');
      }
    } catch (error) {
      message.error('Failed to remove notification');
    }
  };

  const handleTest = async (notificationId: string) => {
    setTesting(notificationId);
    try {
      const response = await fetch(`/api/v1/notifications/${notificationId}/test`, {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          event: 'task_completed',
          test_message: 'Test notification from workspace',
        }),
      });
      const data = await response.json();
      if (data.success) {
        message.success(`Test sent successfully! (${data.response_time_ms}ms)`);
      } else {
        message.error(`Test failed: ${data.error_message || 'Unknown error'}`);
      }
    } catch (error) {
      message.error('Test failed: Network error');
    } finally {
      setTesting(null);
    }
  };

  const handleSubmit = async (values: { notification_id: string; events: string[]; enabled: boolean }) => {
    try {
      const url = editingNotification
        ? `/api/v1/workspaces/${workspaceId}/notifications/${editingNotification.workspace_notification_id}`
        : `/api/v1/workspaces/${workspaceId}/notifications`;
      const method = editingNotification ? 'PUT' : 'POST';

      const response = await fetch(url, {
        method,
        headers: getAuthHeaders(),
        body: JSON.stringify({
          ...values,
          events: values.events.join(','),
        }),
      });

      if (response.ok) {
        message.success(editingNotification ? 'Notification updated' : 'Notification added');
        setModalVisible(false);
        fetchWorkspaceNotifications();
      } else {
        const data = await response.json();
        message.error(data.error || 'Failed to save');
      }
    } catch (error) {
      message.error('Failed to save');
    }
  };

  // 格式化事件显示
  const formatEvents = (events: string) => {
    if (!events) return '-';
    return events.split(',').map(e => eventLabels[e.trim()] || e).join(', ');
  };

  // Columns for global notifications table
  const globalColumns: ColumnsType<GlobalNotification> = [
    {
      title: 'Name',
      key: 'name',
      render: (_: unknown, record: GlobalNotification) => (
        <Space>
          <BellOutlined />
          <span style={{ fontWeight: 500 }}>{record.name}</span>
          <Tag color="blue">Global</Tag>
        </Space>
      ),
    },
    {
      title: 'Type',
      dataIndex: 'notification_type',
      key: 'notification_type',
      render: (type: string) => (
        <Tag color={type === 'webhook' ? 'cyan' : 'green'}>
          {type === 'webhook' ? 'Webhook' : 'Lark Robot'}
        </Tag>
      ),
    },
    {
      title: 'Events',
      dataIndex: 'global_events',
      key: 'global_events',
      render: (events: string) => formatEvents(events),
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
      width: 80,
      render: (_: unknown, record: GlobalNotification) => (
        <Button
          type="text"
          icon={<SendOutlined />}
          onClick={() => handleTest(record.notification_id)}
          loading={testing === record.notification_id}
          title="Test"
        />
      ),
    },
  ];

  const columns: ColumnsType<WorkspaceNotification> = [
    {
      title: 'Name',
      key: 'name',
      render: (_: unknown, record: WorkspaceNotification) => (
        <Space>
          <BellOutlined />
          <span style={{ fontWeight: 500 }}>{record.notification?.name || record.notification_id}</span>
        </Space>
      ),
    },
    {
      title: 'Type',
      key: 'notification_type',
      render: (_: unknown, record: WorkspaceNotification) => {
        const type = record.notification?.notification_type;
        return (
          <Tag color={type === 'webhook' ? 'cyan' : 'green'}>
            {type === 'webhook' ? 'Webhook' : 'Lark Robot'}
          </Tag>
        );
      },
    },
    {
      title: 'Events',
      dataIndex: 'events',
      key: 'events',
      render: (events: string) => formatEvents(events),
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
      width: 150,
      render: (_: unknown, record: WorkspaceNotification) => (
        <Space>
          <Button
            type="text"
            icon={<SendOutlined />}
            onClick={() => handleTest(record.notification_id)}
            loading={testing === record.notification_id}
            title="Test"
          />
          <Button type="text" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          <Popconfirm
            title="Remove this notification?"
            onConfirm={() => handleDelete(record.workspace_notification_id)}
          >
            <Button type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title="Notifications"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>
          Add Notification
        </Button>
      }
    >
      <p style={{ color: '#666', marginBottom: 16 }}>
        Configure notifications for this workspace. Notifications will be sent when workspace events occur.
      </p>

      {/* Global Notifications Section */}
      {globalNotifications.length > 0 && (
        <div style={{ marginBottom: 24 }}>
          <h4 style={{ marginBottom: 12, color: '#1890ff' }}>
            🌐 Global Notifications (Auto-applied to all workspaces)
          </h4>
          <Table
            columns={globalColumns}
            dataSource={globalNotifications}
            rowKey="notification_id"
            loading={loading}
            pagination={false}
            size="small"
            style={{ marginBottom: 16 }}
          />
        </div>
      )}

      {/* Workspace-specific Notifications Section */}
      <h4 style={{ marginBottom: 12 }}>
        📋 Workspace-specific Notifications
      </h4>
      {workspaceNotifications.length === 0 ? (
        <Empty description="No workspace-specific notifications configured" />
      ) : (
        <Table
          columns={columns}
          dataSource={workspaceNotifications}
          rowKey="workspace_notification_id"
          loading={loading}
          pagination={false}
        />
      )}

      <Modal
        title={editingNotification ? 'Edit Notification' : 'Add Notification'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={null}
        width={500}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="notification_id"
            label="Notification"
            rules={[{ required: true, message: 'Please select a notification' }]}
          >
            <Select
              placeholder="Select a notification"
              disabled={!!editingNotification}
              options={availableNotifications.map((n) => ({
                value: n.notification_id,
                label: `${n.name} (${n.notification_type === 'webhook' ? 'Webhook' : 'Lark Robot'})`,
              }))}
            />
          </Form.Item>

          <Form.Item
            name="events"
            label="Events to Trigger"
            rules={[{ required: true, message: 'Please select at least one event' }]}
          >
            <Checkbox.Group>
              <Space direction="vertical">
                {eventOptions.map((opt) => (
                  <Checkbox key={opt.value} value={opt.value}>
                    {opt.label}
                  </Checkbox>
                ))}
              </Space>
            </Checkbox.Group>
          </Form.Item>

          <Form.Item name="enabled" label="Enabled" valuePropName="checked">
            <Switch />
          </Form.Item>

          <Form.Item style={{ marginBottom: 0, textAlign: 'right' }}>
            <Space>
              <Button onClick={() => setModalVisible(false)}>Cancel</Button>
              <Button type="primary" htmlType="submit">
                {editingNotification ? 'Update' : 'Add'}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default WorkspaceNotificationConfig;
