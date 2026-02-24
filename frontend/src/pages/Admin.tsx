import React, { useState, useEffect } from 'react';
import { useToast } from '../hooks/useToast';
import { adminService, getEngineDisplayName, detectEngineTypeFromURL } from '../services/admin';
import type { TerraformVersion, CreateTerraformVersionRequest, UpdateTerraformVersionRequest, IaCEngineType } from '../services/admin';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './Admin.module.css';

const Admin: React.FC = () => {
  const [versions, setVersions] = useState<TerraformVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [editingVersion, setEditingVersion] = useState<TerraformVersion | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; version: TerraformVersion | null }>({
    show: false,
    version: null,
  });
  const { showToast } = useToast();

  // 表单状态
  const [formData, setFormData] = useState({
    version: '',
    download_url: '',
    checksum: '',
    enabled: true,
    deprecated: false,
  });

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  // 加载版本列表
  const loadVersions = async () => {
    try {
      setLoading(true);
      console.log('开始加载Terraform版本列表...');
      const response = await adminService.getTerraformVersions();
      console.log('API响应:', response);
      console.log('版本数量:', response.items?.length);
      setVersions(response.items || []);
    } catch (error: any) {
      console.error('加载版本列表失败:', error);
      showToast(error.response?.data?.error || '加载版本列表失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadVersions();
  }, []);

  // 打开添加对话框
  const handleAdd = () => {
    setEditingVersion(null);
    setFormData({
      version: '',
      download_url: '',
      checksum: '',
      enabled: true,
      deprecated: false,
    });
    setFormErrors({});
    setShowDialog(true);
  };

  // 打开编辑对话框
  const handleEdit = (version: TerraformVersion) => {
    setEditingVersion(version);
    setFormData({
      version: version.version,
      download_url: version.download_url,
      checksum: version.checksum,
      enabled: version.enabled,
      deprecated: version.deprecated,
    });
    setFormErrors({});
    setShowDialog(true);
  };

  // 验证表单
  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    if (!formData.version.trim()) {
      errors.version = '版本号不能为空';
    } else if (!/^\d+\.\d+\.\d+$/.test(formData.version.trim())) {
      errors.version = '版本号格式不正确（例如：1.5.0）';
    }

    if (!formData.download_url.trim()) {
      errors.download_url = '下载链接不能为空';
    } else if (!/^https?:\/\/.+/.test(formData.download_url.trim())) {
      errors.download_url = 'URL格式不正确';
    }

    if (!formData.checksum.trim()) {
      errors.checksum = 'Checksum不能为空';
    } else if (!/^[a-f0-9]{64}$/i.test(formData.checksum.trim())) {
      errors.checksum = 'Checksum必须是64位SHA256哈希值';
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // 提交表单
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    try {
      if (editingVersion) {
        // 更新
        const updateData: UpdateTerraformVersionRequest = {
          download_url: formData.download_url,
          checksum: formData.checksum,
          enabled: formData.enabled,
          deprecated: formData.deprecated,
        };
        await adminService.updateTerraformVersion(editingVersion.id, updateData);
        showToast('版本更新成功', 'success');
      } else {
        // 创建
        const createData: CreateTerraformVersionRequest = {
          version: formData.version,
          download_url: formData.download_url,
          checksum: formData.checksum,
          enabled: formData.enabled,
          deprecated: formData.deprecated,
        };
        await adminService.createTerraformVersion(createData);
        showToast('版本创建成功', 'success');
      }

      setShowDialog(false);
      loadVersions();
    } catch (error: any) {
      showToast(error.response?.data?.error || '操作失败', 'error');
    }
  };

  // 设置默认版本
  const handleSetDefault = async (version: TerraformVersion) => {
    try {
      await adminService.setDefaultVersion(version.id);
      showToast(`版本 ${version.version} 已设置为默认版本`, 'success');
      loadVersions();
    } catch (error: any) {
      showToast(error.response?.data?.error || '设置默认版本失败', 'error');
    }
  };

  // 删除版本
  const handleDelete = (version: TerraformVersion) => {
    setDeleteConfirm({ show: true, version });
  };

  const confirmDelete = async () => {
    if (!deleteConfirm.version) return;

    try {
      await adminService.deleteTerraformVersion(deleteConfirm.version.id);
      showToast('版本删除成功', 'success');
      setDeleteConfirm({ show: false, version: null });
      loadVersions();
    } catch (error: any) {
      showToast(error.response?.data?.error || '删除失败', 'error');
    }
  };

  // 渲染状态徽章
  const renderStatusBadge = (version: TerraformVersion) => {
    if (version.deprecated) {
      return <span className={`${styles.statusBadge} ${styles.deprecated}`}>Deprecated</span>;
    }
    if (version.enabled) {
      return <span className={`${styles.statusBadge} ${styles.enabled}`}>Enabled</span>;
    }
    return <span className={`${styles.statusBadge} ${styles.disabled}`}>Disabled</span>;
  };

  // 获取引擎类型徽章样式
  const getEngineBadgeClass = (engineType: IaCEngineType): string => {
    return engineType === 'opentofu' ? styles.opentofu : styles.terraform;
  };

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.header}>
        <h1 className={styles.title}>IaC Engine Versions</h1>
        <p className={styles.description}>
          管理平台支持的 IaC 引擎版本（Terraform / OpenTofu），配置下载链接和校验和以确保版本完整性。
          引擎类型会根据下载链接自动识别。
        </p>
      </div>

      {/* 操作栏 */}
      <div className={styles.actions}>
        <div></div>
        <button className={styles.addButton} onClick={handleAdd}>
          + 添加版本
        </button>
      </div>

      {/* 版本列表 */}
      <div className={styles.versionsList}>
        {loading ? (
          <div className={styles.loading}>加载中...</div>
        ) : versions.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>暂无 IaC 引擎版本</div>
            <div className={styles.emptyHint}>点击"添加版本"按钮创建第一个版本</div>
          </div>
        ) : (
          <table className={styles.versionsTable}>
            <thead>
              <tr>
                <th>引擎</th>
                <th>版本</th>
                <th>下载链接</th>
                <th>Checksum</th>
                <th>状态</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {versions.map((version) => {
                // 根据下载链接动态检测引擎类型（运行时推断，不存储在数据库）
                const engineType = detectEngineTypeFromURL(version.download_url);
                return (
                  <tr key={version.id}>
                    <td>
                      <span className={`${styles.engineBadge} ${getEngineBadgeClass(engineType)}`}>
                        {getEngineDisplayName(engineType)}
                      </span>
                    </td>
                    <td className={styles.versionCell}>
                      <span className={styles.versionNumber}>{version.version}</span>
                      {version.is_default && <span className={styles.defaultBadge}>DEFAULT</span>}
                    </td>
                    <td className={styles.urlCell} title={version.download_url}>
                      {version.download_url}
                    </td>
                    <td className={styles.checksumCell} title={version.checksum}>
                      {version.checksum.substring(0, 16)}...
                    </td>
                    <td>{renderStatusBadge(version)}</td>
                    <td>{new Date(version.created_at).toLocaleDateString('zh-CN')}</td>
                    <td>
                      <div className={styles.actionButtons}>
                        {!version.is_default && version.enabled && (
                          <button
                            className={`${styles.actionButton} ${styles.setDefault}`}
                            onClick={() => handleSetDefault(version)}
                            title="设置为默认版本"
                          >
                            设为默认
                          </button>
                        )}
                        <button className={styles.actionButton} onClick={() => handleEdit(version)}>
                          编辑
                        </button>
                        <button
                          className={`${styles.actionButton} ${styles.delete}`}
                          onClick={() => handleDelete(version)}
                          disabled={version.is_default}
                          title={version.is_default ? '默认版本不能删除' : '删除版本'}
                        >
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* 添加/编辑对话框 */}
      {showDialog && (
        <div className={styles.dialog} onClick={() => setShowDialog(false)}>
          <div className={styles.dialogContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.dialogHeader}>
              <h2 className={styles.dialogTitle}>
                {editingVersion ? '编辑Terraform版本' : '添加Terraform版本'}
              </h2>
            </div>

            <form onSubmit={handleSubmit}>
              <div className={styles.dialogBody}>
                {/* 版本号 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    版本号<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.version ? styles.error : ''}`}
                    value={formData.version}
                    onChange={(e) => setFormData({ ...formData, version: e.target.value })}
                    placeholder="例如：1.5.0"
                    disabled={!!editingVersion}
                  />
                  {formErrors.version && <span className={styles.errorText}>{formErrors.version}</span>}
                  {!formErrors.version && <span className={styles.hint}>语义化版本格式，例如：1.5.0</span>}
                </div>

                {/* 下载链接 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    下载链接<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.download_url ? styles.error : ''}`}
                    value={formData.download_url}
                    onChange={(e) => setFormData({ ...formData, download_url: e.target.value })}
                    placeholder="https://releases.hashicorp.com/terraform/..."
                  />
                  {formErrors.download_url && (
                    <span className={styles.errorText}>{formErrors.download_url}</span>
                  )}
                  {!formErrors.download_url && (
                    <span className={styles.hint}>官方Terraform发布URL</span>
                  )}
                </div>

                {/* Checksum */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    SHA256 Checksum<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.checksum ? styles.error : ''}`}
                    value={formData.checksum}
                    onChange={(e) => setFormData({ ...formData, checksum: e.target.value })}
                    placeholder="64位SHA256哈希值"
                  />
                  {formErrors.checksum && <span className={styles.errorText}>{formErrors.checksum}</span>}
                  {!formErrors.checksum && (
                    <span className={styles.hint}>用于验证下载文件的完整性</span>
                  )}
                </div>

                {/* 启用状态 */}
                <div className={styles.checkbox}>
                  <input
                    type="checkbox"
                    id="enabled"
                    checked={formData.enabled}
                    onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
                  />
                  <label htmlFor="enabled">启用此版本</label>
                </div>
                <div className={styles.checkboxHint}>启用后，此版本将在创建workspace时可选</div>

                {/* 弃用标记 */}
                <div className={styles.checkbox}>
                  <input
                    type="checkbox"
                    id="deprecated"
                    checked={formData.deprecated}
                    onChange={(e) => setFormData({ ...formData, deprecated: e.target.checked })}
                  />
                  <label htmlFor="deprecated">标记为已弃用</label>
                </div>
                <div className={styles.checkboxHint}>标记后将显示警告，但仍可使用</div>
              </div>

              <div className={styles.dialogFooter}>
                <button
                  type="button"
                  className={`${styles.button} ${styles.secondary}`}
                  onClick={() => setShowDialog(false)}
                >
                  取消
                </button>
                <button type="submit" className={`${styles.button} ${styles.primary}`}>
                  {editingVersion ? '保存' : '创建'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 删除确认对话框 */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="确认删除"
        message={`确定要删除版本 ${deleteConfirm.version?.version} 吗？如果有workspace正在使用此版本，删除将失败。`}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm({ show: false, version: null })}
        confirmText="删除"
        cancelText="取消"
      />

    </div>
  );
};

export default Admin;
