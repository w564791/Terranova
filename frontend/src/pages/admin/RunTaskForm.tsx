import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useToast } from '../../contexts/ToastContext';
import styles from './RunTaskForm.module.css';

interface RunTask {
  run_task_id: string;
  name: string;
  description: string;
  endpoint_url: string;
  hmac_key_set: boolean;
  enabled: boolean;
  is_global: boolean;
  global_stages?: string;
  global_enforcement_level?: string;
  timeout_seconds: number;
  max_run_seconds: number;
}

interface FormData {
  name: string;
  description: string;
  endpoint_url: string;
  hmac_key: string;
  enabled: boolean;
  is_global: boolean;
  global_stages: string[];
  global_enforcement_level: string;
  timeout_seconds: number;
  max_run_seconds: number;
}

const RunTaskForm: React.FC = () => {
  const { runTaskId } = useParams<{ runTaskId: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  const [formData, setFormData] = useState<FormData>({
    name: '',
    description: '',
    endpoint_url: '',
    hmac_key: '',
    enabled: true,
    is_global: false,
    global_stages: ['post_plan'],
    global_enforcement_level: 'advisory',
    timeout_seconds: 600,
    max_run_seconds: 3600,
  });
  const [loading, setLoading] = useState(false);
  const [initialLoading, setInitialLoading] = useState(!!runTaskId);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  
  const isEdit = !!runTaskId;

  const getAuthHeaders = () => {
    const token = localStorage.getItem('token');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    };
  };

  useEffect(() => {
    if (runTaskId) {
      loadRunTask();
    }
  }, [runTaskId]);

  const loadRunTask = async () => {
    if (!runTaskId) return;
    
    try {
      setInitialLoading(true);
      const response = await fetch(`/api/v1/run-tasks/${runTaskId}`, {
        headers: getAuthHeaders(),
      });
      
      if (response.ok) {
        const data: RunTask = await response.json();
        const globalStages = data.global_stages ? data.global_stages.split(',') : ['post_plan'];
        setFormData({
          name: data.name,
          description: data.description || '',
          endpoint_url: data.endpoint_url,
          hmac_key: '',
          enabled: data.enabled,
          is_global: data.is_global,
          global_stages: globalStages,
          global_enforcement_level: data.global_enforcement_level || 'advisory',
          timeout_seconds: data.timeout_seconds,
          max_run_seconds: data.max_run_seconds,
        });
      } else {
        showToast('Failed to load run task', 'error');
        navigate('/global/settings/run-tasks');
      }
    } catch (error) {
      showToast('Failed to load run task', 'error');
      navigate('/global/settings/run-tasks');
    } finally {
      setInitialLoading(false);
    }
  };

  const testConnection = async (): Promise<boolean> => {
    if (!formData.endpoint_url) {
      showToast('Please enter an endpoint URL first', 'error');
      return false;
    }

    setTesting(true);
    setTestResult(null);
    
    try {
      const testUrl = formData.endpoint_url.endsWith('/') 
        ? `${formData.endpoint_url}test` 
        : `${formData.endpoint_url}/test`;
      
      const response = await fetch('/api/v1/run-tasks/test', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          endpoint_url: testUrl,
          hmac_key: formData.hmac_key || '',
        }),
      });

      const data = await response.json();
      
      if (data.success) {
        if (data.hmac_configured && !data.hmac_verified) {
          setTestResult({ success: false, message: 'Connection OK, but HMAC verification failed' });
          return false;
        }
        const hmacStatus = data.hmac_configured ? '(HMAC verified ✓)' : '(No HMAC configured)';
        setTestResult({ success: true, message: `Connection test passed! ${hmacStatus}` });
        return true;
      } else {
        setTestResult({ success: false, message: data.error || 'Connection test failed' });
        return false;
      }
    } catch (error) {
      setTestResult({ success: false, message: 'Network error' });
      return false;
    } finally {
      setTesting(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
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

    if (formData.is_global && formData.global_stages.length === 0) {
      showToast('Please select at least one stage for global task', 'error');
      return;
    }

    // 测试连接
    const testPassed = await testConnection();
    if (!testPassed) {
      const confirmed = window.confirm('Connection test failed. Do you still want to save?');
      if (!confirmed) return;
    }

    // 保存
    await saveRunTask();
  };

  const saveRunTask = async () => {
    try {
      setLoading(true);
      
      const payload = {
        name: formData.name,
        description: formData.description || undefined,
        endpoint_url: formData.endpoint_url,
        hmac_key: formData.hmac_key || undefined,
        enabled: formData.enabled,
        is_global: formData.is_global,
        global_stages: formData.global_stages.join(','),
        global_enforcement_level: formData.global_enforcement_level,
        timeout_seconds: formData.timeout_seconds,
        max_run_seconds: formData.max_run_seconds,
      };

      const url = isEdit
        ? `/api/v1/run-tasks/${runTaskId}`
        : '/api/v1/run-tasks';
      const method = isEdit ? 'PUT' : 'POST';

      const response = await fetch(url, {
        method,
        headers: getAuthHeaders(),
        body: JSON.stringify(payload),
      });

      if (response.ok) {
        showToast(
          isEdit ? 'Run task updated successfully' : 'Run task created successfully',
          'success'
        );
        navigate('/global/settings/run-tasks');
      } else {
        const data = await response.json();
        showToast(data.error || 'Failed to save run task', 'error');
      }
    } catch (error) {
      showToast('Failed to save run task', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleStageChange = (stage: string, checked: boolean) => {
    if (checked) {
      setFormData({ ...formData, global_stages: [...formData.global_stages, stage] });
    } else {
      setFormData({ ...formData, global_stages: formData.global_stages.filter(s => s !== stage) });
    }
  };

  if (initialLoading) {
    return <div className={styles.loading}>Loading...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <button className={styles.backButton} onClick={() => navigate('/global/settings/run-tasks')}>
          ← Back to Run Tasks
        </button>
        <h1>{isEdit ? 'Edit Run Task' : 'Create Run Task'}</h1>
        <p className={styles.subtitle}>
          {isEdit 
            ? 'Update the configuration for this run task.'
            : 'Configure a new run task that can be triggered during Terraform runs.'}
        </p>
      </div>

      <form onSubmit={handleSubmit} className={styles.form}>
        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Basic Information</h2>
          
          <div className={styles.formGroup}>
            <label htmlFor="name">Name *</label>
            <input
              id="name"
              type="text"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="e.g. security-scan"
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
              placeholder="e.g. Security scanning service"
              rows={3}
            />
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
              placeholder="https://example.com/api/run-task"
              required
            />
            <span className={styles.helpText}>
              Run Tasks will POST to this URL.
            </span>
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="hmac_key">HMAC Key</label>
            <input
              id="hmac_key"
              type="password"
              value={formData.hmac_key}
              onChange={(e) => setFormData({ ...formData, hmac_key: e.target.value })}
              placeholder={isEdit ? 'Leave empty to keep existing key' : 'Enter HMAC key (optional)'}
            />
            <span className={styles.helpText}>
              A secret key that may be required by the service to verify request authenticity.
            </span>
          </div>

          <div className={styles.testConnection}>
            <button
              type="button"
              className={styles.testButton}
              onClick={testConnection}
              disabled={testing || !formData.endpoint_url}
            >
              {testing ? 'Testing...' : 'Test Connection'}
            </button>
            {testResult && (
              <span className={testResult.success ? styles.testSuccess : styles.testError}>
                {testResult.message}
              </span>
            )}
          </div>
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Timeout Settings</h2>
          
          <div className={styles.formRow}>
            <div className={styles.formGroup}>
              <label htmlFor="timeout_seconds">Timeout (seconds) *</label>
              <input
                id="timeout_seconds"
                type="number"
                min={60}
                max={600}
                value={formData.timeout_seconds}
                onChange={(e) => setFormData({ ...formData, timeout_seconds: parseInt(e.target.value) || 600 })}
                required
              />
              <span className={styles.helpText}>
                Time to wait for progress updates (60-600 seconds)
              </span>
            </div>

            <div className={styles.formGroup}>
              <label htmlFor="max_run_seconds">Max Run Time (seconds) *</label>
              <input
                id="max_run_seconds"
                type="number"
                min={600}
                max={3600}
                value={formData.max_run_seconds}
                onChange={(e) => setFormData({ ...formData, max_run_seconds: parseInt(e.target.value) || 3600 })}
                required
              />
              <span className={styles.helpText}>
                Maximum total run time (600-3600 seconds)
              </span>
            </div>
          </div>
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Global Task Settings</h2>
          
          <div className={styles.formGroup}>
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={formData.is_global}
                onChange={(e) => setFormData({ ...formData, is_global: e.target.checked })}
              />
              <span>Enable as Global Task</span>
            </label>
            <span className={styles.helpText}>
              Global tasks are automatically applied to all workspaces.
            </span>
          </div>

          {formData.is_global && (
            <>
              <div className={styles.formGroup}>
                <label>Run Stages *</label>
                <div className={styles.checkboxGroup}>
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={formData.global_stages.includes('pre_plan')}
                      onChange={(e) => handleStageChange('pre_plan', e.target.checked)}
                    />
                    <span>Pre-plan</span>
                    <span className={styles.stageDesc}>Before Terraform generates the plan</span>
                  </label>
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={formData.global_stages.includes('post_plan')}
                      onChange={(e) => handleStageChange('post_plan', e.target.checked)}
                    />
                    <span>Post-plan</span>
                    <span className={styles.stageDesc}>After Terraform creates the plan</span>
                  </label>
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={formData.global_stages.includes('pre_apply')}
                      onChange={(e) => handleStageChange('pre_apply', e.target.checked)}
                    />
                    <span>Pre-apply</span>
                    <span className={styles.stageDesc}>Before Terraform applies the plan</span>
                  </label>
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={formData.global_stages.includes('post_apply')}
                      onChange={(e) => handleStageChange('post_apply', e.target.checked)}
                    />
                    <span>Post-apply</span>
                    <span className={styles.stageDesc}>After Terraform applies the plan</span>
                  </label>
                </div>
              </div>

              <div className={styles.formGroup}>
                <label htmlFor="global_enforcement_level">Enforcement Level *</label>
                <select
                  id="global_enforcement_level"
                  value={formData.global_enforcement_level}
                  onChange={(e) => setFormData({ ...formData, global_enforcement_level: e.target.value })}
                  required
                >
                  <option value="advisory">Advisory - Failed run tasks produce a warning</option>
                  <option value="mandatory">Mandatory - Failed run tasks stop the run</option>
                </select>
              </div>
            </>
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
              Disabled run tasks will not be executed.
            </span>
          </div>
        </div>

        <div className={styles.formActions}>
          <button
            type="button"
            className={styles.cancelButton}
            onClick={() => navigate('/global/settings/run-tasks')}
            disabled={loading}
          >
            Cancel
          </button>
          <button
            type="submit"
            className={styles.submitButton}
            disabled={loading}
          >
            {loading ? 'Saving...' : (isEdit ? 'Update Run Task' : 'Create Run Task')}
          </button>
        </div>
      </form>
    </div>
  );
};

export default RunTaskForm;
