import React, { useState, useEffect } from 'react';
import { useToast } from '../hooks/useToast';
import { adminService } from '../services/admin';
import type { ProviderTemplate, CreateProviderTemplateRequest, UpdateProviderTemplateRequest } from '../services/admin';
import styles from './Admin.module.css';

const ProviderTemplatesAdmin: React.FC = () => {
  const [templates, setTemplates] = useState<ProviderTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editingTemplate, setEditingTemplate] = useState<ProviderTemplate | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<ProviderTemplate | null>(null);
  const { showToast } = useToast();

  const [formData, setFormData] = useState({
    name: '',
    type: '',
    source: '',
    config: '{}',
    version: '',
    constraint_op: '~>',
    enabled: true,
    description: '',
  });

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  const loadTemplates = async () => {
    try {
      setLoading(true);
      const response = await adminService.getProviderTemplates();
      setTemplates(response.items || []);
    } catch (error: any) {
      showToast(error.response?.data?.error || '加载Provider模板列表失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTemplates();
  }, []);

  const resetForm = () => {
    setFormData({
      name: '',
      type: '',
      source: '',
      config: '{}',
      version: '',
      constraint_op: '',
      enabled: true,
      description: '',
    });
    setFormErrors({});
  };

  const handleAdd = () => {
    setEditingTemplate(null);
    resetForm();
    setShowForm(true);
  };

  const handleEdit = (template: ProviderTemplate) => {
    setEditingTemplate(template);
    setFormData({
      name: template.name,
      type: template.type,
      source: template.source,
      config: JSON.stringify(template.config, null, 2),
      version: template.version,
      constraint_op: template.constraint_op ?? '',
      enabled: template.enabled,
      description: template.description,
    });
    setFormErrors({});
    setShowForm(true);
  };

  const handleCancel = () => {
    setShowForm(false);
    setEditingTemplate(null);
    resetForm();
  };

  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    if (!formData.name.trim()) {
      errors.name = '名称不能为空';
    }

    if (!formData.type.trim()) {
      errors.type = '类型不能为空';
    }

    if (!formData.source.trim()) {
      errors.source = 'Source不能为空';
    } else if (!formData.source.includes('/')) {
      errors.source = 'Source格式不正确（例如：hashicorp/aws）';
    }

    try {
      JSON.parse(formData.config);
    } catch {
      errors.config = 'Config必须是合法的JSON格式';
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    try {
      const parsedConfig = JSON.parse(formData.config);

      if (editingTemplate) {
        const updateData: UpdateProviderTemplateRequest = {
          name: formData.name,
          type: formData.type,
          source: formData.source,
          config: parsedConfig,
          version: formData.version,
          constraint_op: formData.constraint_op,
          enabled: formData.enabled,
          description: formData.description || undefined,
        };
        await adminService.updateProviderTemplate(editingTemplate.id, updateData);
        showToast('Provider模板更新成功', 'success');
      } else {
        const createData: CreateProviderTemplateRequest = {
          name: formData.name,
          type: formData.type,
          source: formData.source,
          config: parsedConfig,
          version: formData.version,
          constraint_op: formData.constraint_op,
          enabled: formData.enabled,
          description: formData.description || undefined,
        };
        await adminService.createProviderTemplate(createData);
        showToast('Provider模板创建成功', 'success');
      }

      setShowForm(false);
      setEditingTemplate(null);
      loadTemplates();
    } catch (error: any) {
      showToast(error.response?.data?.error || '操作失败', 'error');
    }
  };

  const handleSetDefault = async (template: ProviderTemplate) => {
    try {
      await adminService.setDefaultProviderTemplate(template.id);
      showToast(`模板 "${template.name}" 已设置为默认模板`, 'success');
      loadTemplates();
    } catch (error: any) {
      showToast(error.response?.data?.error || '设置默认模板失败', 'error');
    }
  };

  const handleDelete = (template: ProviderTemplate) => {
    setDeleteConfirm(template);
  };

  const confirmDelete = async () => {
    if (!deleteConfirm) return;

    try {
      await adminService.deleteProviderTemplate(deleteConfirm.id);
      showToast('Provider模板删除成功', 'success');
      setDeleteConfirm(null);
      loadTemplates();
    } catch (error: any) {
      showToast(error.response?.data?.error || '删除失败', 'error');
    }
  };

  const renderStatusBadge = (template: ProviderTemplate) => {
    if (template.enabled) {
      return <span className={`${styles.statusBadge} ${styles.enabled}`}>Enabled</span>;
    }
    return <span className={`${styles.statusBadge} ${styles.disabled}`}>Disabled</span>;
  };

  return (
    <>
      {/* 操作栏 */}
      <div className={styles.actions}>
        <div></div>
        {!showForm && (
          <button className={styles.addButton} onClick={handleAdd}>
            + 添加模板
          </button>
        )}
      </div>

      {/* 内联编辑表单 */}
      {showForm && (
        <div className={styles.inlineForm}>
          <div className={styles.inlineFormHeader}>
            <h3 className={styles.inlineFormTitle}>
              {editingTemplate ? '编辑Provider模板' : '添加Provider模板'}
            </h3>
            <button className={styles.inlineFormClose} onClick={handleCancel}>
              ×
            </button>
          </div>

          <form onSubmit={handleSubmit}>
            <div className={styles.inlineFormBody}>
              <div className={styles.inlineFormGrid}>
                {/* 名称 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    名称<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.name ? styles.error : ''}`}
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    placeholder="例如：AWS Default"
                  />
                  {formErrors.name && <span className={styles.errorText}>{formErrors.name}</span>}
                </div>

                {/* 类型 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    类型<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.type ? styles.error : ''}`}
                    value={formData.type}
                    onChange={(e) => setFormData({ ...formData, type: e.target.value })}
                    placeholder="aws, kubernetes, tencentcloud, etc."
                  />
                  {formErrors.type && <span className={styles.errorText}>{formErrors.type}</span>}
                </div>

                {/* Source */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    Source<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.source ? styles.error : ''}`}
                    value={formData.source}
                    onChange={(e) => setFormData({ ...formData, source: e.target.value })}
                    placeholder="hashicorp/aws"
                  />
                  {formErrors.source && <span className={styles.errorText}>{formErrors.source}</span>}
                  {!formErrors.source && <span className={styles.hint}>Terraform Registry 中的 Provider 路径</span>}
                </div>

                {/* 版本 + 约束 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>版本</label>
                  <div className={styles.inlineRow}>
                    <select
                      className={styles.select}
                      value={formData.constraint_op}
                      onChange={(e) => {
                        const val = e.target.value;
                        if (val === '') {
                          setFormData({ ...formData, constraint_op: '', version: '' });
                        } else {
                          setFormData({ ...formData, constraint_op: val });
                        }
                      }}
                    >
                      <option value="">不限制</option>
                      <option value="~>">~&gt;</option>
                      <option value=">=">&gt;=</option>
                      <option value=">">&gt;</option>
                      <option value="=">=</option>
                      <option value="<=">&lt;=</option>
                      <option value="<">&lt;</option>
                    </select>
                    {formData.constraint_op && (
                      <input
                        type="text"
                        className={styles.input}
                        value={formData.version}
                        onChange={(e) => setFormData({ ...formData, version: e.target.value })}
                        placeholder="6.0"
                      />
                    )}
                  </div>
                  <span className={styles.hint}>
                    {formData.constraint_op
                      ? `版本约束，例如：~> 6.0`
                      : '不限制版本，使用最新可用版本'}
                  </span>
                </div>

                {/* Config */}
                <div className={`${styles.formGroup} ${styles.inlineFormFull}`}>
                  <label className={styles.label}>
                    Config<span className={styles.required}>*</span>
                  </label>
                  <textarea
                    className={`${styles.textarea} ${formErrors.config ? styles.error : ''}`}
                    value={formData.config}
                    onChange={(e) => setFormData({ ...formData, config: e.target.value })}
                    rows={6}
                    placeholder='{"region": "us-east-1"}'
                  />
                  {formErrors.config && <span className={styles.errorText}>{formErrors.config}</span>}
                  {!formErrors.config && <span className={styles.hint}>JSON 格式的 Provider 配置参数</span>}
                  <div className={styles.securityNotice}>
                    <strong>Security:</strong> 请勿在此存放密钥、密码等敏感数据。敏感凭据应通过 Workspace 变量（Environment Variables）注入，
                    例如 AWS_ACCESS_KEY_ID、AWS_SECRET_ACCESS_KEY 等。此处仅配置 region、endpoint 等非敏感参数。
                  </div>
                </div>

                {/* 描述 */}
                <div className={`${styles.formGroup} ${styles.inlineFormFull}`}>
                  <label className={styles.label}>描述</label>
                  <textarea
                    className={styles.textarea}
                    value={formData.description}
                    onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                    rows={2}
                    placeholder="模板描述（可选）"
                  />
                </div>

                {/* 启用状态 */}
                <div className={styles.formGroup}>
                  <div className={styles.checkbox}>
                    <input
                      type="checkbox"
                      id="pt-enabled"
                      checked={formData.enabled}
                      onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
                    />
                    <label htmlFor="pt-enabled">启用此模板</label>
                  </div>
                  <div className={styles.checkboxHint}>启用后，此模板将在 Workspace 配置时可选</div>
                </div>
              </div>
            </div>

            <div className={styles.inlineFormFooter}>
              <button
                type="button"
                className={`${styles.button} ${styles.secondary}`}
                onClick={handleCancel}
              >
                取消
              </button>
              <button type="submit" className={`${styles.button} ${styles.primary}`}>
                {editingTemplate ? '保存' : '创建'}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* 模板列表 */}
      <div className={styles.versionsList}>
        {loading ? (
          <div className={styles.loading}>加载中...</div>
        ) : templates.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>暂无 Provider 模板</div>
            <div className={styles.emptyHint}>点击"添加模板"按钮创建第一个Provider模板</div>
          </div>
        ) : (
          <table className={styles.versionsTable}>
            <thead>
              <tr>
                <th>名称</th>
                <th>类型</th>
                <th>Source</th>
                <th>版本</th>
                <th>状态</th>
                <th>默认</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {templates.map((template) => (
                <tr key={template.id}>
                  <td>
                    <span style={{ fontWeight: 500 }}>{template.name}</span>
                  </td>
                  <td>
                    <span className={styles.typeBadge}>{template.type}</span>
                  </td>
                  <td>
                    <span className={styles.sourceCell}>{template.source}</span>
                  </td>
                  <td>
                    {template.version ? (
                      <span className={styles.versionConstraint}>
                        {template.constraint_op || '~>'} {template.version}
                      </span>
                    ) : (
                      <span style={{ color: 'var(--color-gray-400)' }}>-</span>
                    )}
                  </td>
                  <td>{renderStatusBadge(template)}</td>
                  <td>
                    {template.is_default && <span className={styles.defaultBadge}>DEFAULT</span>}
                  </td>
                  <td>
                    <div className={styles.actionButtons}>
                      {!template.is_default && template.enabled && (
                        <button
                          className={`${styles.actionButton} ${styles.setDefault}`}
                          onClick={() => handleSetDefault(template)}
                          title="设置为默认模板"
                        >
                          设为默认
                        </button>
                      )}
                      <button className={styles.actionButton} onClick={() => handleEdit(template)}>
                        编辑
                      </button>
                      <button
                        className={`${styles.actionButton} ${styles.delete}`}
                        onClick={() => handleDelete(template)}
                        disabled={template.is_default}
                        title={template.is_default ? '默认模板不能删除' : '删除模板'}
                      >
                        删除
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Toast风格删除确认 */}
      {deleteConfirm && (
        <div className={styles.toastConfirm}>
          <span className={styles.toastConfirmIcon}>!</span>
          <span className={styles.toastConfirmMessage}>
            确定删除模板 "{deleteConfirm.name}" ?
          </span>
          <div className={styles.toastConfirmActions}>
            <button
              className={`${styles.toastConfirmBtn} ${styles.toastConfirmBtnCancel}`}
              onClick={() => setDeleteConfirm(null)}
            >
              取消
            </button>
            <button
              className={`${styles.toastConfirmBtn} ${styles.toastConfirmBtnConfirm}`}
              onClick={confirmDelete}
            >
              删除
            </button>
          </div>
        </div>
      )}
    </>
  );
};

export default ProviderTemplatesAdmin;
