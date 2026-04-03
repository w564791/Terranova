import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { variableSetService } from '../services/variableSets';
import type { VariableSet } from '../services/variableSets';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './Admin.module.css';

const VariableSetsPage: React.FC = () => {
  const [varsets, setVarsets] = useState<VariableSet[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<VariableSet | null>(null);
  const { showToast } = useToast();
  const navigate = useNavigate();

  const [formData, setFormData] = useState({
    name: '',
    description: '',
    scope: 'specific' as 'global' | 'specific',
  });

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  const loadVarsets = async () => {
    try {
      setLoading(true);
      const response = await variableSetService.list();
      setVarsets(response.items || []);
    } catch (error: any) {
      showToast(error.response?.data?.error || '加载变量集列表失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadVarsets();
  }, []);

  const resetForm = () => {
    setFormData({ name: '', description: '', scope: 'specific' });
    setFormErrors({});
  };

  const handleAdd = () => {
    resetForm();
    setShowForm(true);
  };

  const handleCancel = () => {
    setShowForm(false);
    resetForm();
  };

  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};
    if (!formData.name.trim()) {
      errors.name = '名称不能为空';
    }
    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validateForm()) return;

    try {
      const created = await variableSetService.create({
        name: formData.name,
        description: formData.description,
        scope: formData.scope,
      });
      showToast('变量集创建成功，请继续添加变量和分配', 'success');
      navigate(`/variable-sets/${created.varset_id}`);
    } catch (error: any) {
      showToast(error.response?.data?.error || '创建失败', 'error');
    }
  };

  const handleDelete = (varset: VariableSet) => {
    setDeleteConfirm(varset);
  };

  const confirmDelete = async () => {
    if (!deleteConfirm) return;
    try {
      await variableSetService.delete(deleteConfirm.varset_id);
      showToast('变量集删除成功', 'success');
      setDeleteConfirm(null);
      loadVarsets();
    } catch (error: any) {
      showToast(error.response?.data?.error || '删除失败', 'error');
    }
  };

  const renderScopeBadge = (scope: string) => {
    if (scope === 'global') {
      return (
        <span className={styles.statusBadge} style={{ backgroundColor: '#EFF6FF', color: '#2563EB', border: '1px solid #93C5FD' }}>
          Global
        </span>
      );
    }
    return (
      <span className={styles.statusBadge} style={{ backgroundColor: '#F0FDF4', color: '#16A34A', border: '1px solid #86EFAC' }}>
        Specific
      </span>
    );
  };

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
  };

  // 按钮样式（带边框）
  const btnStyle: React.CSSProperties = {
    padding: '5px 12px',
    border: '1px solid var(--color-gray-300)',
    background: 'var(--color-white)',
    color: 'var(--color-gray-700)',
    borderRadius: '6px',
    fontSize: '13px',
    fontWeight: 500,
    cursor: 'pointer',
  };
  const btnDeleteStyle: React.CSSProperties = {
    ...btnStyle,
    color: 'var(--color-red-500)',
    borderColor: 'var(--color-red-200)',
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>Variable Sets</h1>
        <p className={styles.description}>
          变量集允许将变量分组并应用到多个 Workspace。Global 变量集自动应用于所有 Workspace，Specific 变量集需要手动分配。
        </p>
      </div>

      {/* 操作栏 */}
      <div className={styles.actions}>
        <div></div>
        {!showForm && (
          <button className={styles.addButton} onClick={handleAdd}>
            + 创建变量集
          </button>
        )}
      </div>

      {/* 创建表单（仅创建，编辑在详情页） */}
      {showForm && (
        <div className={styles.inlineForm}>
          <div className={styles.inlineFormHeader}>
            <h3 className={styles.inlineFormTitle}>创建变量集</h3>
            <button className={styles.inlineFormClose} onClick={handleCancel}>
              x
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
                    placeholder="例如：Production AWS Credentials"
                  />
                  {formErrors.name && <span className={styles.errorText}>{formErrors.name}</span>}
                </div>

                {/* Scope */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    作用域<span className={styles.required}>*</span>
                  </label>
                  <div style={{ display: 'flex', gap: '16px', marginTop: '4px' }}>
                    <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                      <input
                        type="radio"
                        name="scope"
                        value="global"
                        checked={formData.scope === 'global'}
                        onChange={() => setFormData({ ...formData, scope: 'global' })}
                      />
                      Global
                    </label>
                    <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                      <input
                        type="radio"
                        name="scope"
                        value="specific"
                        checked={formData.scope === 'specific'}
                        onChange={() => setFormData({ ...formData, scope: 'specific' })}
                      />
                      Specific
                    </label>
                  </div>
                  <span className={styles.hint}>
                    Global 变量集自动应用到所有 Workspace，Specific 需要手动分配
                  </span>
                </div>

                {/* 描述 */}
                <div className={`${styles.formGroup} ${styles.inlineFormFull}`}>
                  <label className={styles.label}>描述</label>
                  <textarea
                    className={styles.textarea}
                    value={formData.description}
                    onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                    rows={2}
                    placeholder="变量集描述（可选）"
                  />
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
                创建
              </button>
            </div>
          </form>
        </div>
      )}

      {/* 变量集列表 */}
      <div className={styles.versionsList} style={{ border: '1px solid #e5e7eb' }}>
        {loading ? (
          <div className={styles.loading}>加载中...</div>
        ) : varsets.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>暂无变量集</div>
            <div className={styles.emptyHint}>点击"创建变量集"按钮创建第一个变量集</div>
          </div>
        ) : (
          <table className={styles.versionsTable}>
            <thead>
              <tr>
                <th>名称</th>
                <th>作用域</th>
                <th>变量数</th>
                <th>分配数</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {varsets.map((varset) => (
                <tr
                  key={varset.varset_id}
                  style={{ cursor: 'pointer' }}
                  onClick={() => navigate(`/variable-sets/${varset.varset_id}`)}
                >
                  <td>
                    <span style={{ fontWeight: 500, color: 'var(--color-blue-600)' }}>
                      {varset.name}
                    </span>
                  </td>
                  <td>{renderScopeBadge(varset.scope)}</td>
                  <td>
                    <span style={{ color: 'var(--color-gray-600)' }}>
                      {varset.variable_count ?? 0}
                    </span>
                  </td>
                  <td>
                    <span style={{ color: 'var(--color-gray-600)' }}>
                      {varset.scope === 'global' ? 'All' : (varset.assignment_count ?? 0)}
                    </span>
                  </td>
                  <td>
                    <span style={{ color: 'var(--color-gray-500)', fontSize: '13px' }}>
                      {formatDate(varset.created_at)}
                    </span>
                  </td>
                  <td>
                    <button
                      style={btnDeleteStyle}
                      onClick={(e) => { e.stopPropagation(); handleDelete(varset); }}
                    >
                      删除
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* 删除确认弹窗 */}
      <ConfirmDialog
        isOpen={!!deleteConfirm}
        title={`删除变量集 "${deleteConfirm?.name}"`}
        confirmText="确认删除"
        cancelText="取消"
        type="danger"
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm(null)}
      >
        <div style={{ marginBottom: '16px' }}>
          <p style={{ margin: '0 0 12px 0', color: 'var(--color-gray-700)', fontSize: '14px', lineHeight: '1.5' }}>
            删除变量集 <strong>{deleteConfirm?.name}</strong> 将永久移除其所有变量和分配关系。
          </p>
          <p style={{ margin: 0, color: 'var(--color-gray-700)', fontSize: '14px', lineHeight: '1.5' }}>
            此操作<strong>不可撤销</strong>，确定要继续吗？
          </p>
        </div>
      </ConfirmDialog>
    </div>
  );
};

export default VariableSetsPage;
