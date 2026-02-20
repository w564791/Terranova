import React, { useState, useEffect } from 'react';
import { iamService } from '../../services/iam';
import type { Application, Organization, CreateApplicationRequest, UpdateApplicationRequest } from '../../services/iam';
import { useToast } from '../../contexts/ToastContext';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './ApplicationManagement.module.css';

const ApplicationManagement: React.FC = () => {
  const { success: showSuccess, error: showError } = useToast();
  const [applications, setApplications] = useState<Application[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [selectedOrg, setSelectedOrg] = useState<number>(0);
  const [loading, setLoading] = useState(false);
  const [showModal, setShowModal] = useState(false);
  const [showSecretModal, setShowSecretModal] = useState(false);
  const [secretInfo, setSecretInfo] = useState<{ appKey: string; appSecret: string; message: string } | null>(null);
  const [editingApp, setEditingApp] = useState<Application | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterActive, setFilterActive] = useState<boolean | undefined>(undefined);
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; id: number; name: string }>({ 
    show: false, id: 0, name: '' 
  });
  const [regenerateConfirm, setRegenerateConfirm] = useState<{ show: boolean; id: number; name: string }>({ 
    show: false, id: 0, name: '' 
  });

  const [formData, setFormData] = useState<CreateApplicationRequest>({
    org_id: 0,
    name: '',
    description: '',
    callback_urls: {},
    expires_at: undefined,
  });

  useEffect(() => {
    loadOrganizations();
  }, []);

  useEffect(() => {
    if (selectedOrg > 0) {
      loadApplications();
    }
  }, [selectedOrg, filterActive]);

  const loadOrganizations = async () => {
    try {
      const data = await iamService.listOrganizations(true);
      setOrganizations(data.organizations);
      if (data.organizations.length > 0) {
        setSelectedOrg(data.organizations[0].id);
      }
    } catch (error: any) {
      showError('加载组织列表失败: ' + (error.response?.data?.error || error.message));
    }
  };

  const loadApplications = async () => {
    if (selectedOrg === 0) return;
    setLoading(true);
    try {
      const data = await iamService.listApplications(selectedOrg, filterActive);
      setApplications(data.applications);
    } catch (error: any) {
      showError('加载应用列表失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = () => {
    setEditingApp(null);
    setFormData({
      org_id: selectedOrg,
      name: '',
      description: '',
      callback_urls: {},
      expires_at: undefined,
    });
    setShowModal(true);
  };

  const handleEdit = (app: Application) => {
    setEditingApp(app);
    setFormData({
      org_id: app.org_id,
      name: app.name,
      description: app.description,
      callback_urls: app.callback_urls,
      expires_at: app.expires_at,
    });
    setShowModal(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      if (editingApp) {
        const updateData: UpdateApplicationRequest = {
          name: formData.name,
          description: formData.description,
          callback_urls: formData.callback_urls,
          expires_at: formData.expires_at,
        };
        await iamService.updateApplication(editingApp.id, updateData);
        showSuccess('应用更新成功');
      } else {
        const result = await iamService.createApplication(formData);
        setSecretInfo({
          appKey: result.application.app_key,
          appSecret: result.app_secret,
          message: result.message,
        });
        setShowSecretModal(true);
      }
      setShowModal(false);
      loadApplications();
    } catch (error: any) {
      showError('操作失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = (id: number, name: string) => {
    setDeleteConfirm({ show: true, id, name });
  };

  const confirmDelete = async () => {
    const { id } = deleteConfirm;
    setLoading(true);
    try {
      await iamService.deleteApplication(id);
      showSuccess('应用删除成功');
      loadApplications();
    } catch (error: any) {
      showError('删除失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
      setDeleteConfirm({ show: false, id: 0, name: '' });
    }
  };

  const handleToggleActive = async (app: Application) => {
    setLoading(true);
    try {
      await iamService.updateApplication(app.id, { is_active: !app.is_active });
      showSuccess(`应用已${!app.is_active ? '启用' : '禁用'}`);
      loadApplications();
    } catch (error: any) {
      showError('操作失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  const handleRegenerateSecret = (id: number, name: string) => {
    setRegenerateConfirm({ show: true, id, name });
  };

  const confirmRegenerateSecret = async () => {
    const { id } = regenerateConfirm;
    setLoading(true);
    try {
      const result = await iamService.regenerateSecret(id);
      const app = applications.find(a => a.id === id);
      setSecretInfo({
        appKey: app?.app_key || '',
        appSecret: result.app_secret,
        message: result.message,
      });
      setShowSecretModal(true);
    } catch (error: any) {
      showError('重新生成密钥失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
      setRegenerateConfirm({ show: false, id: 0, name: '' });
    }
  };

  const filteredApplications = applications.filter(app =>
    app.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    app.description.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    showSuccess('已复制到剪贴板');
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1>应用管理</h1>
        <button onClick={handleCreate} className={styles.createButton} disabled={selectedOrg === 0}>
          + 创建应用
        </button>
      </div>

      <div className={styles.filters}>
        <div className={styles.filterGroup}>
          <label>组织:</label>
          <select
            value={selectedOrg}
            onChange={(e) => setSelectedOrg(Number(e.target.value))}
            className={styles.select}
          >
            <option value={0}>选择组织</option>
            {organizations.map(org => (
              <option key={org.id} value={org.id}>{org.display_name}</option>
            ))}
          </select>
        </div>

        <div className={styles.filterGroup}>
          <label>状态:</label>
          <select
            value={filterActive === undefined ? 'all' : filterActive ? 'active' : 'inactive'}
            onChange={(e) => {
              const val = e.target.value;
              setFilterActive(val === 'all' ? undefined : val === 'active');
            }}
            className={styles.select}
          >
            <option value="all">全部</option>
            <option value="active">启用</option>
            <option value="inactive">禁用</option>
          </select>
        </div>

        <div className={styles.searchBox}>
          <input
            type="text"
            placeholder="搜索应用名称或描述..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className={styles.searchInput}
          />
        </div>
      </div>

      {loading && <div className={styles.loading}>加载中...</div>}

      {!loading && selectedOrg === 0 && (
        <div className={styles.emptyState}>
          <p>请先选择一个组织</p>
        </div>
      )}

      {!loading && selectedOrg > 0 && filteredApplications.length === 0 && (
        <div className={styles.emptyState}>
          <p>暂无应用</p>
        </div>
      )}

      {!loading && filteredApplications.length > 0 && (
        <div className={styles.tableContainer}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>应用名称</th>
                <th>App Key</th>
                <th>描述</th>
                <th>状态</th>
                <th>最后使用</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {filteredApplications.map(app => (
                <tr key={app.id}>
                  <td className={styles.nameCell}>{app.name}</td>
                  <td className={styles.keyCell}>
                    <code>{app.app_key}</code>
                    <button
                      onClick={() => copyToClipboard(app.app_key)}
                      className={styles.copyButton}
                      title="复制"
                    >
                      复制
                    </button>
                  </td>
                  <td className={styles.descCell}>{app.description || '-'}</td>
                  <td>
                    <span className={app.is_active ? styles.statusActive : styles.statusInactive}>
                      {app.is_active ? '启用' : '禁用'}
                    </span>
                  </td>
                  <td>{app.last_used_at ? new Date(app.last_used_at).toLocaleString() : '从未使用'}</td>
                  <td>{new Date(app.created_at).toLocaleString()}</td>
                  <td className={styles.actions}>
                    <button onClick={() => handleEdit(app)} className={styles.editButton}>
                      编辑
                    </button>
                    <button
                      onClick={() => handleToggleActive(app)}
                      className={app.is_active ? styles.disableButton : styles.enableButton}
                    >
                      {app.is_active ? '禁用' : '启用'}
                    </button>
                    <button
                      onClick={() => handleRegenerateSecret(app.id, app.name)}
                      className={styles.regenerateButton}
                      disabled={!app.is_active}
                    >
                      重新生成密钥
                    </button>
                    <button
                      onClick={() => handleDelete(app.id, app.name)}
                      className={styles.deleteButton}
                    >
                      删除
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 创建/编辑模态框 */}
      {showModal && (
        <div className={styles.modal}>
          <div className={styles.modalContent}>
            <h2>{editingApp ? '编辑应用' : '创建应用'}</h2>
            <form onSubmit={handleSubmit}>
              <div className={styles.formGroup}>
                <label>应用名称 *</label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  required
                  placeholder="输入应用名称"
                />
              </div>

              <div className={styles.formGroup}>
                <label>描述</label>
                <textarea
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  placeholder="输入应用描述"
                  rows={3}
                />
              </div>

              <div className={styles.formGroup}>
                <label>过期时间</label>
                <input
                  type="datetime-local"
                  value={formData.expires_at ? formData.expires_at.slice(0, 16) : ''}
                  onChange={(e) => setFormData({ ...formData, expires_at: e.target.value ? e.target.value + ':00Z' : undefined })}
                />
              </div>

              <div className={styles.modalActions}>
                <button type="button" onClick={() => setShowModal(false)} className={styles.cancelButton}>
                  取消
                </button>
                <button type="submit" className={styles.submitButton} disabled={loading}>
                  {loading ? '处理中...' : editingApp ? '更新' : '创建'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 密钥显示模态框 */}
      {showSecretModal && secretInfo && (
        <div className={styles.modal}>
          <div className={styles.modalContent}>
            <h2>重要：保存应用密钥</h2>
            <div className={styles.secretWarning}>
              <p>{secretInfo.message}</p>
            </div>

            <div className={styles.secretInfo}>
              <div className={styles.secretItem}>
                <label>App Key:</label>
                <div className={styles.secretValue}>
                  <code>{secretInfo.appKey}</code>
                    <button onClick={() => copyToClipboard(secretInfo.appKey)} className={styles.copyButton}>
                      复制
                    </button>
                </div>
              </div>

              <div className={styles.secretItem}>
                <label>App Secret:</label>
                <div className={styles.secretValue}>
                  <code>{secretInfo.appSecret}</code>
                  <button onClick={() => copyToClipboard(secretInfo.appSecret)} className={styles.copyButton}>
                    复制
                  </button>
                </div>
              </div>
            </div>

            <div className={styles.modalActions}>
              <button
                onClick={() => {
                  setShowSecretModal(false);
                  setSecretInfo(null);
                }}
                className={styles.submitButton}
              >
                我已保存
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 删除确认对话框 */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="确认删除应用"
        message={`确定要删除应用 "${deleteConfirm.name}" 吗？此操作无法撤销。`}
        confirmText="删除"
        cancelText="取消"
        type="danger"
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm({ show: false, id: 0, name: '' })}
      />

      {/* 重新生成密钥确认对话框 */}
      <ConfirmDialog
        isOpen={regenerateConfirm.show}
        title="确认重新生成密钥"
        message={`确定要重新生成应用 "${regenerateConfirm.name}" 的密钥吗？旧密钥将立即失效。`}
        confirmText="重新生成"
        cancelText="取消"
        type="warning"
        onConfirm={confirmRegenerateSecret}
        onCancel={() => setRegenerateConfirm({ show: false, id: 0, name: '' })}
      />
    </div>
  );
};

export default ApplicationManagement;
