import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../../contexts/ToastContext';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './NotificationManagement.module.css';

interface NotificationConfig {
  id: number;
  notification_id: string;
  name: string;
  description: string;
  notification_type: 'webhook' | 'lark_robot';
  endpoint_url: string;
  secret_set: boolean;
  enabled: boolean;
  is_global: boolean;
  global_events: string;
  workspace_count: number;
  created_at: string;
  updated_at: string;
}

const NotificationManagement: React.FC = () => {
  const [notifications, setNotifications] = useState<NotificationConfig[]>([]);
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; notification: NotificationConfig | null }>({
    show: false,
    notification: null,
  });
  const [deleting, setDeleting] = useState(false);
  const navigate = useNavigate();
  const { showToast } = useToast();

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  const fetchNotifications = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/notifications', {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        setNotifications(data.notifications || []);
      } else {
        showToast('Failed to fetch notifications', 'error');
      }
    } catch (error) {
      showToast('Failed to fetch notifications', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchNotifications();
  }, []);

  const handleCreate = () => {
    navigate('/global/settings/notifications/create');
  };

  const handleEdit = (notificationId: string) => {
    navigate(`/global/settings/notifications/${notificationId}/edit`);
  };

  const handleDeleteClick = (notification: NotificationConfig) => {
    setDeleteConfirm({ show: true, notification });
  };

  const handleDeleteConfirm = async () => {
    if (!deleteConfirm.notification) return;
    
    setDeleting(true);
    try {
      const response = await fetch(`/api/v1/notifications/${deleteConfirm.notification.notification_id}`, {
        method: 'DELETE',
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        showToast('Notification deleted successfully', 'success');
        setDeleteConfirm({ show: false, notification: null });
        fetchNotifications();
      } else {
        const data = await response.json();
        showToast(data.error || 'Failed to delete notification', 'error');
      }
    } catch (error) {
      showToast('Failed to delete notification', 'error');
    } finally {
      setDeleting(false);
    }
  };

  const handleDeleteCancel = () => {
    setDeleteConfirm({ show: false, notification: null });
  };

  const handleTestNotification = async (notificationId: string) => {
    setTesting(notificationId);
    try {
      const response = await fetch(`/api/v1/notifications/${notificationId}/test`, {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          event: 'task_completed',
          test_message: 'Test notification from IaC Platform',
        }),
      });

      const data = await response.json();
      
      if (data.success) {
        showToast(`Test notification sent successfully! (${data.response_time_ms}ms)`, 'success');
      } else {
        showToast(`Test failed: ${data.error_message || 'Unknown error'}`, 'error');
      }
    } catch (error) {
      showToast('Test failed: Network error', 'error');
    } finally {
      setTesting(null);
    }
  };

  const formatEvents = (events?: string) => {
    if (!events) return '-';
    return events.split(',').map(e => {
      switch (e) {
        case 'task_created': return 'Created';
        case 'task_completed': return 'Completed';
        case 'task_failed': return 'Failed';
        case 'task_cancelled': return 'Cancelled';
        case 'approval_required': return 'Approval';
        case 'drift_detected': return 'Drift';
        default: return e;
      }
    }).join(', ');
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerContent}>
          <h1 className={styles.title}>Notifications</h1>
          <p className={styles.description}>
            Configure webhook and Lark robot notifications for workspace events.
            Create notifications here and then assign them to workspaces.
          </p>
        </div>
        <div className={styles.headerActions}>
          <button className={styles.refreshButton} onClick={fetchNotifications} disabled={loading}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
            </svg>
            Refresh
          </button>
          <button className={styles.createButton} onClick={handleCreate}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            Create Notification
          </button>
        </div>
      </div>

      <div className={styles.content}>
        {loading ? (
          <div className={styles.loading}>Loading...</div>
        ) : notifications.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyIcon}>
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
                <path d="M13.73 21a2 2 0 0 1-3.46 0" />
              </svg>
            </div>
            <h3>No Notifications</h3>
            <p>Create your first notification to receive alerts for workspace events.</p>
            <button className={styles.createButton} onClick={handleCreate}>
              Create Notification
            </button>
          </div>
        ) : (
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Endpoint URL</th>
                  <th>Secret</th>
                  <th>Workspaces</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {notifications.map((notification) => (
                  <tr key={notification.notification_id}>
                    <td>
                      <div className={styles.notificationName}>
                        {notification.notification_type === 'webhook' ? (
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
                            <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
                          </svg>
                        ) : (
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                          </svg>
                        )}
                        <span>{notification.name}</span>
                        {notification.is_global && <span className={styles.globalBadge}>Global</span>}
                      </div>
                      {notification.is_global && notification.global_events && (
                        <div className={styles.notificationMeta}>
                          Events: {formatEvents(notification.global_events)}
                        </div>
                      )}
                    </td>
                    <td>
                      <span className={notification.notification_type === 'webhook' ? styles.badgeBlue : styles.badgeGreen}>
                        {notification.notification_type === 'webhook' ? 'Webhook' : 'Lark Robot'}
                      </span>
                    </td>
                    <td>
                      <div className={styles.endpointUrl} title={notification.endpoint_url}>
                        {notification.endpoint_url}
                      </div>
                    </td>
                    <td>
                      {notification.secret_set ? (
                        <span className={styles.badgeSuccess}>Set</span>
                      ) : (
                        <span className={styles.badgeDefault}>Not Set</span>
                      )}
                    </td>
                    <td className={styles.centerCell}>
                      {notification.is_global ? (
                        <span className={styles.badgeGold}>All</span>
                      ) : (
                        notification.workspace_count
                      )}
                    </td>
                    <td>
                      {notification.enabled ? (
                        <span className={styles.statusEnabled}>
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
                            <polyline points="22 4 12 14.01 9 11.01" />
                          </svg>
                          Enabled
                        </span>
                      ) : (
                        <span className={styles.statusDisabled}>
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <circle cx="12" cy="12" r="10" />
                            <line x1="15" y1="9" x2="9" y2="15" />
                            <line x1="9" y1="9" x2="15" y2="15" />
                          </svg>
                          Disabled
                        </span>
                      )}
                    </td>
                    <td>
                      <div className={styles.actions}>
                        <button
                          className={styles.actionButton}
                          onClick={() => handleTestNotification(notification.notification_id)}
                          disabled={testing === notification.notification_id}
                          title="Test Notification"
                        >
                          {testing === notification.notification_id ? (
                            <span className={styles.spinner} />
                          ) : (
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                              <line x1="22" y1="2" x2="11" y2="13" />
                              <polygon points="22 2 15 22 11 13 2 9 22 2" />
                            </svg>
                          )}
                        </button>
                        <button
                          className={styles.actionButton}
                          onClick={() => handleEdit(notification.notification_id)}
                          title="Edit"
                        >
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                          </svg>
                        </button>
                        <button
                          className={`${styles.actionButton} ${styles.deleteButton}`}
                          onClick={() => handleDeleteClick(notification)}
                          title="Delete"
                        >
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <polyline points="3 6 5 6 21 6" />
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                          </svg>
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Delete Confirmation Dialog */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="Delete Notification"
        type="danger"
        confirmText="Delete"
        cancelText="Cancel"
        onConfirm={handleDeleteConfirm}
        onCancel={handleDeleteCancel}
        loading={deleting}
      >
        <div className={styles.deleteDialogContent}>
          <p>
            Are you sure you want to delete the notification <strong>"{deleteConfirm.notification?.name}"</strong>?
          </p>
          {deleteConfirm.notification && deleteConfirm.notification.workspace_count > 0 && (
            <div className={styles.deleteWarning}>
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                <line x1="12" y1="9" x2="12" y2="13" />
                <line x1="12" y1="17" x2="12.01" y2="17" />
              </svg>
              <span>
                This notification is currently assigned to <strong>{deleteConfirm.notification.workspace_count}</strong> workspace(s).
                Deleting it will remove the association from all workspaces.
              </span>
            </div>
          )}
          <p className={styles.deleteNote}>This action cannot be undone.</p>
        </div>
      </ConfirmDialog>
    </div>
  );
};

export default NotificationManagement;
