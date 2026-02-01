import React, { useState, useEffect } from 'react';
import { secretsAPI, type Secret, type ResourceType, type SecretType, type CreateSecretResponse } from '../services/secrets';
import { useToast } from '../contexts/ToastContext';
import styles from './SecretsManager.module.css';

interface SecretsManagerProps {
  resourceType: ResourceType;
  resourceId: string;
}

const SecretsManager: React.FC<SecretsManagerProps> = ({ resourceType, resourceId }) => {
  const [secrets, setSecrets] = useState<Secret[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [createdSecret, setCreatedSecret] = useState<CreateSecretResponse | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; secret: Secret } | null>(null);
  const [editingSecret, setEditingSecret] = useState<Secret | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  
  // 表单状态
  const [formData, setFormData] = useState({
    key: '',
    value: '',
    secret_type: 'hcp' as SecretType,
    description: '',
    tags: '' as string,
  });

  const { showToast } = useToast();

  useEffect(() => {
    loadSecrets();
  }, [resourceType, resourceId]);

  const loadSecrets = async () => {
    try {
      setLoading(true);
      const data = await secretsAPI.list(resourceType, resourceId, true);
      setSecrets(data?.secrets || []);
    } catch (error: any) {
      console.error('Failed to load secrets:', error);
      // 只在非404错误时显示toast（404表示没有secrets，这是正常情况）
      if (error.response?.status !== 404) {
        showToast(error.response?.data?.error || 'Failed to load secrets', 'error');
      }
      // 设置为空数组
      setSecrets([]);
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = async () => {
    if (!formData.key.trim() || !formData.value.trim()) {
      showToast('Key and Value are required', 'error');
      return;
    }

    // 防止重复提交
    if (isSubmitting) {
      return;
    }

    try {
      setIsSubmitting(true);
      
      const tags = formData.tags
        .split(',')
        .map(t => t.trim())
        .filter(t => t.length > 0);

      await secretsAPI.create(resourceType, resourceId, {
        key: formData.key.trim(),
        value: formData.value.trim(),
        secret_type: formData.secret_type,
        description: formData.description.trim(),
        tags: tags.length > 0 ? tags : undefined,
      });

      // 显示成功消息
      showToast('Secret created successfully', 'success');
      
      // 立即关闭对话框并返回列表
      setCreatedSecret(null);
      setShowCreateDialog(false);
      setFormData({ key: '', value: '', secret_type: 'hcp', description: '', tags: '' });
      
      // 刷新列表（不阻塞UI，即使失败也不影响）
      loadSecrets().catch(err => console.error('Failed to reload secrets:', err));
    } catch (error: any) {
      console.error('Create secret error:', error);
      showToast(error.response?.data?.error || 'Failed to create secret', 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleEdit = (secret: Secret) => {
    setEditingSecret(secret);
    setFormData({
      key: secret.key,
      value: '',
      secret_type: secret.secret_type,
      description: secret.description || '',
      tags: secret.tags?.join(', ') || '',
    });
    setShowCreateDialog(true);
  };

  const handleUpdate = async () => {
    if (!editingSecret) return;

    // 防止重复提交
    if (isSubmitting) {
      return;
    }

    try {
      setIsSubmitting(true);
      
      const tags = formData.tags
        .split(',')
        .map(t => t.trim())
        .filter(t => t.length > 0);

      await secretsAPI.update(resourceType, resourceId, editingSecret.secret_id, {
        value: formData.value.trim() || undefined,
        description: formData.description.trim() || undefined,
        tags: tags.length > 0 ? tags : undefined,
      });

      showToast('Secret updated successfully', 'success');
      setEditingSecret(null);
      setShowCreateDialog(false);
      setFormData({ key: '', value: '', secret_type: 'hcp', description: '', tags: '' });
      await loadSecrets();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to update secret', 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleCancelEdit = () => {
    setEditingSecret(null);
    setShowCreateDialog(false);
    setFormData({ key: '', value: '', secret_type: 'hcp', description: '', tags: '' });
  };

  const handleDelete = async () => {
    if (!deleteConfirm) return;

    try {
      await secretsAPI.delete(resourceType, resourceId, deleteConfirm.secret.secret_id);
      showToast('Secret deleted successfully', 'success');
      setDeleteConfirm(null);
      loadSecrets();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to delete secret', 'error');
    }
  };

  const handleCopyValue = (value: string) => {
    navigator.clipboard.writeText(value);
    showToast('Value copied to clipboard', 'success');
  };

  const handleCloseCreateDialog = () => {
    setShowCreateDialog(false);
    setCreatedSecret(null);
    setEditingSecret(null);
    setFormData({ key: '', value: '', secret_type: 'hcp', description: '', tags: '' });
  };

  if (loading) {
    return <div className={styles.loading}>Loading secrets...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h3>Secrets ({secrets.length})</h3>
        <button 
          className={styles.addButton}
          onClick={() => {
            if (showCreateDialog) {
              handleCancelEdit();
            } else {
              setShowCreateDialog(true);
            }
          }}
        >
          {showCreateDialog ? 'Cancel' : '+ Create Secret'}
        </button>
      </div>

      {/* Create Secret Form */}
      {showCreateDialog && (
        <div className={styles.createForm}>
          {createdSecret ? (
            <div className={styles.secretCreated}>
              <h4>✓ Secret Created Successfully</h4>
              <p className={styles.warning}>
                 Please copy this value now. You won't be able to see it again!
              </p>
              <div className={styles.secretDisplay}>
                <div className={styles.secretField}>
                  <label>Key:</label>
                  <code>{createdSecret.key}</code>
                </div>
                <div className={styles.secretField}>
                  <label>Value:</label>
                  <div className={styles.valueDisplay}>
                    <code>{createdSecret.value}</code>
                    <button 
                      className={styles.copyButton}
                      onClick={() => handleCopyValue(createdSecret.value)}
                    >
                      Copy
                    </button>
                  </div>
                </div>
                {createdSecret.description && (
                  <div className={styles.secretField}>
                    <label>Description:</label>
                    <span>{createdSecret.description}</span>
                  </div>
                )}
              </div>
              <button 
                className={styles.doneButton}
                onClick={handleCloseCreateDialog}
              >
                Done
              </button>
            </div>
          ) : (
            <>
              <p className={styles.formHint}>
                {editingSecret 
                  ? 'Edit secret. You can update the value, description, and tags.' 
                  : 'Create an encrypted secret for this ' + resourceType.replace('_', ' ') + '. The value will be encrypted and stored securely. You can only view the value once after creation.'}
              </p>
              
              <div className={styles.formGroup}>
                <label htmlFor="secretType">Secret Type *</label>
                <select
                  id="secretType"
                  value={formData.secret_type}
                  onChange={(e) => setFormData({ ...formData, secret_type: e.target.value as SecretType })}
                  className={styles.input}
                  disabled={!!editingSecret}
                >
                  <option value="hcp">HCP (HashiCorp Cloud Platform)</option>
                </select>
              </div>

              <div className={styles.formGroup}>
                <label htmlFor="secretKey">Key *</label>
                <input
                  id="secretKey"
                  type="text"
                  value={formData.key}
                  onChange={(e) => setFormData({ ...formData, key: e.target.value })}
                  placeholder="e.g., HCP_SERVER_ADDRESS"
                  className={styles.input}
                  disabled={!!editingSecret}
                />
              </div>

              <div className={styles.formGroup}>
                <label htmlFor="secretValue">
                  Value {editingSecret ? '(optional - leave empty to keep current value)' : '*'}
                </label>
                <textarea
                  id="secretValue"
                  value={formData.value}
                  onChange={(e) => setFormData({ ...formData, value: e.target.value })}
                  placeholder={editingSecret ? "Enter new server token or leave empty to keep current" : "Enter the server token"}
                  className={styles.textarea}
                  rows={3}
                />
              </div>

              <div className={styles.formGroup}>
                <label htmlFor="secretDescription">Description</label>
                <input
                  id="secretDescription"
                  type="text"
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  placeholder="e.g., HCP Terraform server address"
                  className={styles.input}
                />
              </div>

              <div className={styles.formGroup}>
                <label htmlFor="secretTags">Tags (comma-separated)</label>
                <input
                  id="secretTags"
                  type="text"
                  value={formData.tags}
                  onChange={(e) => setFormData({ ...formData, tags: e.target.value })}
                  placeholder="e.g., production, hcp"
                  className={styles.input}
                />
              </div>

              <div className={styles.formActions}>
                <button
                  className={styles.createButton}
                  onClick={editingSecret ? handleUpdate : handleCreate}
                  disabled={isSubmitting || (editingSecret ? false : (!formData.key.trim() || !formData.value.trim()))}
                >
                  {isSubmitting ? 'Processing...' : (editingSecret ? 'Update Secret' : 'Create Secret')}
                </button>
              </div>
            </>
          )}
        </div>
      )}

      {/* Secrets List */}
      {!showCreateDialog && (
        <>
          {secrets.length === 0 ? (
            <div className={styles.emptyState}>
              <p>No secrets created yet</p>
              <p className={styles.hint}>Click "Create Secret" to add an encrypted secret</p>
            </div>
          ) : (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Type</th>
                  <th>Key</th>
                  <th>Description</th>
                  <th>Tags</th>
                  <th>Created</th>
                  <th>Last Used</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {secrets.map((secret) => (
                  <tr key={secret.secret_id}>
                    <td>
                      <span className={styles.typeTag}>{secret.secret_type.toUpperCase()}</span>
                    </td>
                    <td className={styles.keyCell}>
                      <code>{secret.key}</code>
                    </td>
                    <td>{secret.description || '-'}</td>
                    <td>
                      {secret.tags && secret.tags.length > 0 ? (
                        <div className={styles.tags}>
                          {secret.tags.map((tag, i) => (
                            <span key={i} className={styles.tag}>{tag}</span>
                          ))}
                        </div>
                      ) : '-'}
                    </td>
                    <td>{new Date(secret.created_at).toLocaleString()}</td>
                    <td>
                      {secret.last_used_at 
                        ? new Date(secret.last_used_at).toLocaleString()
                        : 'Never'}
                    </td>
                    <td>
                      <div style={{ display: 'flex', gap: '8px' }}>
                        <button
                          className={styles.editButton}
                          onClick={() => handleEdit(secret)}
                          title="Edit secret"
                        >
                          Edit
                        </button>
                        <button
                          className={styles.deleteButton}
                          onClick={() => setDeleteConfirm({ show: true, secret })}
                          title="Delete secret"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}

      {/* Delete Confirmation Dialog */}
      {deleteConfirm?.show && (
        <div className={styles.overlay}>
          <div className={styles.confirmDialog}>
            <div className={styles.dialogHeader}>
              <h3>Delete Secret</h3>
            </div>
            <div className={styles.dialogContent}>
              <p>Are you sure you want to delete the secret <strong>"{deleteConfirm.secret.key}"</strong>?</p>
              <p className={styles.warningText}>
                 This action cannot be undone. The secret value will be permanently deleted.
              </p>
            </div>
            <div className={styles.dialogActions}>
              <button
                className={styles.cancelButton}
                onClick={() => setDeleteConfirm(null)}
              >
                Cancel
              </button>
              <button
                className={styles.confirmButton}
                onClick={handleDelete}
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default SecretsManager;
