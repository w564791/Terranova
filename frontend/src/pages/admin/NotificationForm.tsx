import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useToast } from '../../contexts/ToastContext';
import styles from './NotificationForm.module.css';

interface NotificationConfig {
  notification_id: string;
  name: string;
  description: string;
  notification_type: 'webhook' | 'lark_robot';
  endpoint_url: string;
  secret_set: boolean;
  enabled: boolean;
  is_global: boolean;
  global_events: string;
  retry_count: number;
  retry_interval_seconds: number;
  timeout_seconds: number;
}

interface FormData {
  name: string;
  description: string;
  notification_type: 'webhook' | 'lark_robot';
  endpoint_url: string;
  secret: string;
  enabled: boolean;
  is_global: boolean;
  global_events: string[];
  retry_count: number;
  retry_interval_seconds: number;
  timeout_seconds: number;
}

const NotificationForm: React.FC = () => {
  const { notificationId } = useParams<{ notificationId: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  const [formData, setFormData] = useState<FormData>({
    name: '',
    description: '',
    notification_type: 'webhook',
    endpoint_url: '',
    secret: '',
    enabled: true,
    is_global: false,
    global_events: ['task_completed', 'task_failed'],
    retry_count: 3,
    retry_interval_seconds: 30,
    timeout_seconds: 30,
  });
  const [loading, setLoading] = useState(false);
  const [initialLoading, setInitialLoading] = useState(!!notificationId);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  
  const isEdit = !!notificationId;

  const eventOptions = [
    { value: 'task_created', label: 'Task Created', desc: 'When a new task is created' },
    { value: 'task_planning', label: 'Task Planning', desc: 'When plan starts' },
    { value: 'task_planned', label: 'Task Planned', desc: 'When plan completes' },
    { value: 'task_applying', label: 'Task Applying', desc: 'When apply starts' },
    { value: 'task_completed', label: 'Task Completed', desc: 'When task completes successfully' },
    { value: 'task_failed', label: 'Task Failed', desc: 'When task fails' },
    { value: 'task_cancelled', label: 'Task Cancelled', desc: 'When task is cancelled' },
    { value: 'approval_required', label: 'Approval Required', desc: 'When approval is needed' },
    { value: 'drift_detected', label: 'Drift Detected', desc: 'When drift is detected' },
  ];

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  useEffect(() => {
    if (notificationId) {
      loadNotification();
    }
  }, [notificationId]);

  const loadNotification = async () => {
    if (!notificationId) return;
    
    try {
      setInitialLoading(true);
      const response = await fetch(`/api/v1/notifications/${notificationId}`, {
        headers: getAuthHeaders(),
      });
      
      if (response.ok) {
        const data: NotificationConfig = await response.json();
        const globalEvents = data.global_events ? data.global_events.split(',') : ['task_completed', 'task_failed'];
        setFormData({
          name: data.name,
          description: data.description || '',
          notification_type: data.notification_type,
          endpoint_url: data.endpoint_url,
          secret: '',
          enabled: data.enabled,
          is_global: data.is_global,
          global_events: globalEvents,
          retry_count: data.retry_count,
          retry_interval_seconds: data.retry_interval_seconds,
          timeout_seconds: data.timeout_seconds,
        });
      } else {
        showToast('Failed to load notification', 'error');
        navigate('/global/settings/notifications');
      }
    } catch (error) {
      showToast('Failed to load notification', 'error');
      navigate('/global/settings/notifications');
    } finally {
      setInitialLoading(false);
    }
  };

  const testNotification = async (): Promise<boolean> => {
    if (!formData.endpoint_url) {
      showToast('Please enter an endpoint URL first', 'error');
      return false;
    }

    setTesting(true);
    setTestResult(null);
    
    try {
      // 如果是编辑模式，使用已保存的配置测试
      if (isEdit && notificationId) {
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
          setTestResult({ success: true, message: `Test passed! (${data.response_time_ms}ms)` });
          return true;
        } else {
          setTestResult({ success: false, message: data.error_message || 'Test failed' });
          return false;
        }
      } else {
        // 新建模式，需要先保存才能测试
        showToast('Please save the notification first to test it', 'warning');
        return false;
      }
    } catch (error) {
      setTestResult({ success: false, message: 'Network error' });
      return false;
    } finally {
      setTesting(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent, andTest: boolean = false) => {
    e.preventDefault();

    // 验证必填字段
    if (!formData.name.trim()) {
      showToast('Name is required', 'error');
      return;
    }

    if (!/^[a-zA-Z0-9_-]+$/.test(formData.name)) {
      showToast('Name can only contain letters, numbers, dashes and underscores', 'error');
      return;
    }

    if (!formData.endpoint_url.trim()) {
      showToast('Endpoint URL is required', 'error');
      return;
    }

    try {
      new URL(formData.endpoint_url);
    } catch {
      showToast('Please enter a valid URL', 'error');
      return;
    }

    if (formData.is_global && formData.global_events.length === 0) {
      showToast('Please select at least one event for global notification', 'error');
      return;
    }

    // 保存
    const savedId = await saveNotification();
    
    // 如果需要测试且保存成功
    if (andTest && savedId) {
      setTesting(true);
      try {
        const response = await fetch(`/api/v1/notifications/${savedId}/test`, {
          method: 'POST',
          headers: getAuthHeaders(),
          body: JSON.stringify({
            event: 'task_completed',
            test_message: 'Test notification after saving',
          }),
        });

        const data = await response.json();
        
        if (data.success) {
          showToast(`Saved and test passed! (${data.response_time_ms}ms)`, 'success');
          navigate('/global/settings/notifications');
        } else {
          showToast(`Saved, but test failed: ${data.error_message || 'Unknown error'}`, 'warning');
          // 不跳转，让用户可以修改
        }
      } catch (error) {
        showToast('Saved, but test failed: Network error', 'warning');
      } finally {
        setTesting(false);
      }
    }
  };

  const saveNotification = async (): Promise<string | null> => {
    try {
      setLoading(true);
      
      const payload = {
        name: formData.name,
        description: formData.description || undefined,
        notification_type: formData.notification_type,
        endpoint_url: formData.endpoint_url,
        secret: formData.secret || undefined,
        enabled: formData.enabled,
        is_global: formData.is_global,
        global_events: formData.global_events.join(','),
        retry_count: formData.retry_count,
        retry_interval_seconds: formData.retry_interval_seconds,
        timeout_seconds: formData.timeout_seconds,
      };

      const url = isEdit
        ? `/api/v1/notifications/${notificationId}`
        : '/api/v1/notifications';
      const method = isEdit ? 'PUT' : 'POST';

      const response = await fetch(url, {
        method,
        headers: getAuthHeaders(),
        body: JSON.stringify(payload),
      });

      if (response.ok) {
        const data = await response.json();
        showToast(
          isEdit ? 'Notification updated successfully' : 'Notification created successfully',
          'success'
        );
        return data.notification_id;
      } else {
        const data = await response.json();
        showToast(data.error || 'Failed to save notification', 'error');
        return null;
      }
    } catch (error) {
      showToast('Failed to save notification', 'error');
      return null;
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    const savedId = await saveNotification();
    if (savedId) {
      navigate('/global/settings/notifications');
    }
  };

  const handleEventChange = (event: string, checked: boolean) => {
    if (checked) {
      setFormData({ ...formData, global_events: [...formData.global_events, event] });
    } else {
      setFormData({ ...formData, global_events: formData.global_events.filter(e => e !== event) });
    }
  };

  if (initialLoading) {
    return <div className={styles.loading}>Loading...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button className={styles.backButton} onClick={() => navigate('/global/settings/notifications')}>
          ← Back to Notifications
        </button>
        <h1>{isEdit ? 'Edit Notification' : 'Create Notification'}</h1>
        <p className={styles.subtitle}>
          {isEdit 
            ? 'Update the configuration for this notification.'
            : 'Configure a new notification to receive alerts for workspace events.'}
        </p>
      </div>

      <form onSubmit={handleSave} className={styles.form}>
        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Basic Information</h2>
          
          <div className={styles.formGroup}>
            <label htmlFor="name">Name *</label>
            <input
              id="name"
              type="text"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="e.g. ops-webhook, lark-alerts"
              required
            />
            <span className={styles.helpText}>
              Can only contain letters, numbers, dashes and underscores.
            </span>
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="description">Description</label>
            <textarea
              id="description"
              value={formData.description}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              placeholder="Optional description"
              rows={3}
            />
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="notification_type">Type *</label>
            <select
              id="notification_type"
              value={formData.notification_type}
              onChange={(e) => setFormData({ ...formData, notification_type: e.target.value as 'webhook' | 'lark_robot' })}
              disabled={isEdit}
              required
            >
              <option value="webhook">Webhook - HTTP POST to URL</option>
              <option value="lark_robot">Lark Robot - Feishu/Lark bot</option>
            </select>
            {isEdit && (
              <span className={styles.helpText}>
                Type cannot be changed after creation.
              </span>
            )}
          </div>
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Endpoint Configuration</h2>
          
          <div className={styles.formGroup}>
            <label htmlFor="endpoint_url">Endpoint URL *</label>
            <input
              id="endpoint_url"
              type="url"
              value={formData.endpoint_url}
              onChange={(e) => setFormData({ ...formData, endpoint_url: e.target.value })}
              placeholder={formData.notification_type === 'lark_robot' 
                ? 'https://open.larksuite.com/open-apis/bot/v2/hook/...'
                : 'https://example.com/webhook'}
              required
            />
            <span className={styles.helpText}>
              {formData.notification_type === 'lark_robot'
                ? 'Lark/Feishu bot webhook URL'
                : 'Notifications will POST to this URL'}
            </span>
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="secret">
              {formData.notification_type === 'lark_robot' ? 'Sign Secret' : 'HMAC Secret'}
            </label>
            <input
              id="secret"
              type="password"
              value={formData.secret}
              onChange={(e) => setFormData({ ...formData, secret: e.target.value })}
              placeholder={isEdit ? 'Leave empty to keep existing secret' : 'Optional secret key'}
            />
            <span className={styles.helpText}>
              {formData.notification_type === 'lark_robot'
                ? 'Lark bot signature verification key'
                : 'HMAC-SHA256 signing key for request verification'}
            </span>
          </div>

          {isEdit && (
            <div className={styles.testConnection}>
              <button
                type="button"
                className={styles.testButton}
                onClick={testNotification}
                disabled={testing || !formData.endpoint_url}
              >
                {testing ? 'Testing...' : 'Test Notification'}
              </button>
              {testResult && (
                <span className={testResult.success ? styles.testSuccess : styles.testError}>
                  {testResult.message}
                </span>
              )}
            </div>
          )}
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Retry Settings</h2>
          
          <div className={styles.formRow}>
            <div className={styles.formGroup}>
              <label htmlFor="retry_count">Retry Count</label>
              <input
                id="retry_count"
                type="number"
                min={0}
                max={10}
                value={formData.retry_count}
                onChange={(e) => setFormData({ ...formData, retry_count: parseInt(e.target.value) || 0 })}
              />
              <span className={styles.helpText}>
                Number of retries on failure (0-10)
              </span>
            </div>

            <div className={styles.formGroup}>
              <label htmlFor="retry_interval_seconds">Retry Interval (seconds)</label>
              <input
                id="retry_interval_seconds"
                type="number"
                min={5}
                max={300}
                value={formData.retry_interval_seconds}
                onChange={(e) => setFormData({ ...formData, retry_interval_seconds: parseInt(e.target.value) || 30 })}
              />
              <span className={styles.helpText}>
                Wait time between retries (5-300)
              </span>
            </div>

            <div className={styles.formGroup}>
              <label htmlFor="timeout_seconds">Timeout (seconds)</label>
              <input
                id="timeout_seconds"
                type="number"
                min={5}
                max={120}
                value={formData.timeout_seconds}
                onChange={(e) => setFormData({ ...formData, timeout_seconds: parseInt(e.target.value) || 30 })}
              />
              <span className={styles.helpText}>
                Request timeout (5-120)
              </span>
            </div>
          </div>
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Global Notification Settings</h2>
          
          <div className={styles.formGroup}>
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={formData.is_global}
                onChange={(e) => setFormData({ ...formData, is_global: e.target.checked })}
              />
              <span>Enable as Global Notification</span>
            </label>
            <span className={styles.helpText}>
              Global notifications are automatically applied to all workspaces.
            </span>
          </div>

          {formData.is_global && (
            <div className={styles.formGroup}>
              <label>Events to Trigger *</label>
              <div className={styles.checkboxGroup}>
                {eventOptions.map((opt) => (
                  <label key={opt.value} className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={formData.global_events.includes(opt.value)}
                      onChange={(e) => handleEventChange(opt.value, e.target.checked)}
                    />
                    <span>{opt.label}</span>
                    <span className={styles.eventDesc}>{opt.desc}</span>
                  </label>
                ))}
              </div>
            </div>
          )}
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Status</h2>
          
          <div className={styles.formGroup}>
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={formData.enabled}
                onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
              />
              <span>Enabled</span>
            </label>
            <span className={styles.helpText}>
              Disabled notifications will not be sent.
            </span>
          </div>
        </div>

        <div className={styles.formActions}>
          <button
            type="button"
            className={styles.cancelButton}
            onClick={() => navigate('/global/settings/notifications')}
            disabled={loading || testing}
          >
            Cancel
          </button>
          <button
            type="button"
            className={styles.saveTestButton}
            onClick={(e) => handleSubmit(e, true)}
            disabled={loading || testing}
          >
            {loading || testing ? 'Processing...' : 'Save & Test'}
          </button>
          <button
            type="submit"
            className={styles.submitButton}
            disabled={loading || testing}
          >
            {loading ? 'Saving...' : (isEdit ? 'Update' : 'Create')}
          </button>
        </div>
      </form>
    </div>
  );
};

export default NotificationForm;
