import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../../contexts/ToastContext';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './RunTaskManagement.module.css';

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
  organization_id: string;
  team_id: string;
  workspace_count: number;
  created_at: string;
  updated_at: string;
}

const RunTaskManagement: React.FC = () => {
  const [runTasks, setRunTasks] = useState<RunTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; task: RunTask | null }>({
    show: false,
    task: null,
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

  const fetchRunTasks = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/v1/run-tasks', {
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        const data = await response.json();
        setRunTasks(data.run_tasks || []);
      } else {
        showToast('Failed to fetch run tasks', 'error');
      }
    } catch (error) {
      showToast('Failed to fetch run tasks', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRunTasks();
  }, []);

  const handleCreate = () => {
    navigate('/global/settings/run-tasks/create');
  };

  const handleEdit = (runTaskId: string) => {
    navigate(`/global/settings/run-tasks/${runTaskId}/edit`);
  };

  const handleDeleteClick = (task: RunTask) => {
    setDeleteConfirm({ show: true, task });
  };

  const handleDeleteConfirm = async () => {
    if (!deleteConfirm.task) return;
    
    setDeleting(true);
    try {
      const response = await fetch(`/api/v1/run-tasks/${deleteConfirm.task.run_task_id}`, {
        method: 'DELETE',
        headers: getAuthHeaders(),
      });
      if (response.ok) {
        showToast('Run task deleted successfully', 'success');
        setDeleteConfirm({ show: false, task: null });
        fetchRunTasks();
      } else {
        const data = await response.json();
        showToast(data.error || 'Failed to delete run task', 'error');
      }
    } catch (error) {
      showToast('Failed to delete run task', 'error');
    } finally {
      setDeleting(false);
    }
  };

  const handleDeleteCancel = () => {
    setDeleteConfirm({ show: false, task: null });
  };

  const handleTestConnection = async (runTaskId: string) => {
    setTesting(runTaskId);
    try {
      const response = await fetch(`/api/v1/run-tasks/${runTaskId}/test`, {
        method: 'POST',
        headers: getAuthHeaders(),
      });

      const data = await response.json();
      
      if (data.success) {
        if (data.hmac_configured && !data.hmac_verified) {
          showToast('Connection OK, but HMAC verification failed', 'warning');
        } else {
          const hmacStatus = data.hmac_configured ? '(HMAC verified ✓)' : '(No HMAC)';
          showToast(`Connection test passed! ${hmacStatus} (${data.duration_ms}ms)`, 'success');
        }
      } else {
        showToast(`Test failed: ${data.error || 'Unknown error'}`, 'error');
      }
    } catch (error) {
      showToast('Test failed: Network error', 'error');
    } finally {
      setTesting(null);
    }
  };

  const formatStages = (stages?: string) => {
    if (!stages) return '-';
    return stages.split(',').map(s => {
      switch (s) {
        case 'pre_plan': return 'Pre-plan';
        case 'post_plan': return 'Post-plan';
        case 'pre_apply': return 'Pre-apply';
        case 'post_apply': return 'Post-apply';
        default: return s;
      }
    }).join(', ');
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerContent}>
          <h1 className={styles.title}>Run Tasks</h1>
          <p className={styles.description}>
            Run tasks allow external services to pass or fail Terraform runs.
            Configure run tasks here and then assign them to workspaces.
          </p>
        </div>
        <div className={styles.headerActions}>
          <button className={styles.refreshButton} onClick={fetchRunTasks} disabled={loading}>
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
            Create Run Task
          </button>
        </div>
      </div>

      <div className={styles.content}>
        {loading ? (
          <div className={styles.loading}>Loading...</div>
        ) : runTasks.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyIcon}>
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
                <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
              </svg>
            </div>
            <h3>No Run Tasks</h3>
            <p>Create your first run task to integrate external services with Terraform runs.</p>
            <button className={styles.createButton} onClick={handleCreate}>
              Create Run Task
            </button>
          </div>
        ) : (
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Endpoint URL</th>
                  <th>HMAC</th>
                  <th>Workspaces</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {runTasks.map((task) => (
                  <tr key={task.run_task_id}>
                    <td>
                      <div className={styles.taskName}>
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                          <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
                          <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
                        </svg>
                        <span>{task.name}</span>
                        {task.is_global && <span className={styles.globalBadge}>Global</span>}
                      </div>
                      {task.is_global && task.global_stages && (
                        <div className={styles.taskMeta}>
                          Stages: {formatStages(task.global_stages)}
                        </div>
                      )}
                    </td>
                    <td>
                      <div className={styles.endpointUrl} title={task.endpoint_url}>
                        {task.endpoint_url}
                      </div>
                    </td>
                    <td>
                      {task.hmac_key_set ? (
                        <span className={styles.badgeSuccess}>Set</span>
                      ) : (
                        <span className={styles.badgeDefault}>Not Set</span>
                      )}
                    </td>
                    <td className={styles.centerCell}>{task.workspace_count}</td>
                    <td>
                      {task.enabled ? (
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
                          onClick={() => handleTestConnection(task.run_task_id)}
                          disabled={testing === task.run_task_id}
                          title="Test Connection"
                        >
                          {testing === task.run_task_id ? (
                            <span className={styles.spinner} />
                          ) : (
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                              <path d="M22 12h-4l-3 9L9 3l-3 9H2" />
                            </svg>
                          )}
                        </button>
                        <button
                          className={styles.actionButton}
                          onClick={() => handleEdit(task.run_task_id)}
                          title="Edit"
                        >
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                          </svg>
                        </button>
                        <button
                          className={`${styles.actionButton} ${styles.deleteButton}`}
                          onClick={() => handleDeleteClick(task)}
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
        title="Delete Run Task"
        type="danger"
        confirmText="Delete"
        cancelText="Cancel"
        onConfirm={handleDeleteConfirm}
        onCancel={handleDeleteCancel}
        loading={deleting}
      >
        <div className={styles.deleteDialogContent}>
          <p>
            Are you sure you want to delete the run task <strong>"{deleteConfirm.task?.name}"</strong>?
          </p>
          {deleteConfirm.task && deleteConfirm.task.workspace_count > 0 && (
            <div className={styles.deleteWarning}>
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                <line x1="12" y1="9" x2="12" y2="13" />
                <line x1="12" y1="17" x2="12.01" y2="17" />
              </svg>
              <span>
                This run task is currently assigned to <strong>{deleteConfirm.task.workspace_count}</strong> workspace(s).
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

export default RunTaskManagement;
