import React, { useState, useEffect, useCallback } from 'react';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import api from '../services/api';
import { variableSetService } from '../services/variableSets';
import type { EffectiveVariable } from '../services/variableSets';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './WorkspaceDetail.module.css';

interface Variable {
  id: number;
  variable_id: string;  // 变量语义化ID
  key: string;
  value: string;
  version: number;  // 版本号
  description: string;
  variable_type: 'terraform' | 'environment';
  value_format: 'string' | 'hcl';
  sensitive: boolean;
  // is_deleted 不从API返回（内部实现细节）
  created_at: string;
  updated_at: string;
}

interface VariablesTabProps {
  workspaceId: string;
}

const VariablesTab: React.FC<VariablesTabProps> = ({ workspaceId }) => {
  const { showToast } = useToast();
  const [variables, setVariables] = useState<Variable[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddForm, setShowAddForm] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [showMenu, setShowMenu] = useState<number | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [variableToDelete, setVariableToDelete] = useState<Variable | null>(null);
  const [formData, setFormData] = useState({
    key: '',
    value: '',
    description: '',
    variable_type: 'terraform' as 'terraform' | 'environment',
    sensitive: false,
    hcl: false,
    value_format: 'string' as 'string' | 'hcl'
  });

  // Effective variables state
  const [effectiveVars, setEffectiveVars] = useState<EffectiveVariable[]>([]);
  const [effectiveLoading, setEffectiveLoading] = useState(false);

  useEffect(() => {
    fetchVariables();
    fetchEffectiveVariables();
  }, []);

  const fetchEffectiveVariables = useCallback(async () => {
    try {
      setEffectiveLoading(true);
      const data = await variableSetService.getEffectiveVariables(workspaceId);
      // Sort: active first, then overridden
      const sorted = [...(data || [])].sort((a, b) => {
        if (a.is_overridden === b.is_overridden) {
          return a.key.localeCompare(b.key);
        }
        return a.is_overridden ? 1 : -1;
      });
      setEffectiveVars(sorted);
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    } finally {
      setEffectiveLoading(false);
    }
  }, [workspaceId, showToast]);

  // Refresh effective variables after workspace variable changes
  const refreshEffective = () => {
    fetchEffectiveVariables();
  };

  const formatSource = (ev: EffectiveVariable): string => {
    let name = ev.source_name;
    if (ev.scope_level === 'global') {
      name += ' (Glob)';
    } else if (ev.scope_level === 'project') {
      name += ' (Proj)';
    }
    return name;
  };

  const fetchVariables = async () => {
    try {
      setLoading(true);
      const response = await api.get(`/workspaces/${workspaceId}/variables`);
      setVariables(response.data || []);
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleEdit = (variable: Variable) => {
    setEditingId(variable.id);
    setShowAddForm(false);
    setShowMenu(null);
    setFormData({
      key: variable.key,
      value: variable.value,
      description: variable.description,
      variable_type: variable.variable_type,
      sensitive: variable.sensitive,
      hcl: variable.value_format === 'hcl',
      value_format: variable.value_format
    });
  };

  const handleDeleteClick = (variable: Variable) => {
    setShowMenu(null);
    setVariableToDelete(variable);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!variableToDelete) return;

    try {
      // 使用 variable_id 而不是数字 id
      const varId = variableToDelete.variable_id || variableToDelete.id;
      await api.delete(`/workspaces/${workspaceId}/variables/${varId}`);
      showToast('变量删除成功', 'success');
      setDeleteDialogOpen(false);
      setVariableToDelete(null);
      fetchVariables();
      refreshEffective();
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    }
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setVariableToDelete(null);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!formData.key.trim()) {
      showToast('请输入变量名', 'error');
      return;
    }

    try {
      // 构建符合后端API的请求数据
      const requestData: any = {
        key: formData.key,
        value: formData.value,
        description: formData.description,
        variable_type: formData.variable_type,
        sensitive: formData.sensitive,
        value_format: formData.hcl ? 'hcl' : 'string'
      };
      
      if (editingId) {
        // 更新变量：必须包含当前版本号
        const currentVariable = variables.find(v => v.id === editingId);
        if (!currentVariable) {
          showToast('变量不存在', 'error');
          return;
        }
        
        requestData.version = currentVariable.version;  // 添加版本号
        
        try {
          // 使用 variable_id 而不是数字 id
          const currentVariable = variables.find(v => v.id === editingId);
          const varId = currentVariable?.variable_id || editingId;
          const response = await api.put(`/workspaces/${workspaceId}/variables/${varId}`, requestData);
          
          // 显示版本变更信息
          if (response.data?.version_info) {
            showToast(
              `变量更新成功 (版本 ${response.data.version_info.old_version} → ${response.data.version_info.new_version})`,
              'success'
            );
          } else {
            showToast('变量更新成功', 'success');
          }
          setEditingId(null);
        } catch (error: any) {
          // 处理版本冲突
          if (error.response?.status === 409) {
            showToast('版本冲突：变量已被其他用户修改，请刷新后重试', 'error');
            // 自动刷新变量列表
            await fetchVariables();
            refreshEffective();
          } else {
            throw error;
          }
          return;
        }
      } else {
        // 创建变量
        await api.post(`/workspaces/${workspaceId}/variables`, requestData);
        showToast('变量创建成功', 'success');
        setShowAddForm(false);
      }
      
      setFormData({
        key: '',
        value: '',
        description: '',
        variable_type: 'terraform',
        sensitive: false,
        hcl: false,
        value_format: 'string'
      });
      fetchVariables();
      refreshEffective();
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    }
  };

  const handleCancel = () => {
    setShowAddForm(false);
    setEditingId(null);
    setFormData({
      key: '',
      value: '',
      description: '',
      variable_type: 'terraform',
      sensitive: false,
      hcl: false,
      value_format: 'string'
    });
  };

  return (
    <div className={styles.variablesContainer}>
      {/* Sensitive variables info */}
      <div className={styles.section}>
        <h3 className={styles.infoTitle}>Sensitive variables</h3>
        <p className={styles.infoText}>
          <a href="#" className={styles.infoLink}>Sensitive</a> variables are never shown in the UI or API, and can't be edited. They may appear in Terraform logs if your configuration is designed to output them. To change a sensitive variable, delete and replace it.
        </p>
      </div>

      {/* Workspace variables header */}
      <div className={styles.section}>
        <h3 className={styles.variablesSectionTitle}>Workspace variables ({variables.length})</h3>
        <p className={styles.infoText}>
          Variables defined within a workspace always overwrite variables from variable sets that have the same type and the same key. Learn more about <a href="#" className={styles.infoLink}>variable set precedence</a>.
        </p>

        {/* Variable table header */}
        <div className={styles.variablesTableHeader}>
          <div>Key</div>
          <div>Value</div>
          <div>Category</div>
          <div></div>
        </div>

        {/* Variables list */}
        {variables.map((variable) => (
          <React.Fragment key={variable.id}>
            {editingId === variable.id ? (
              // 编辑表单
              <div className={styles.variableEditRow}>
                <form onSubmit={handleSubmit} className={styles.addVariableForm}>
                  <div className={styles.formSection}>
                    <h4 className={styles.formSectionTitle}>Edit variable</h4>
                    <div className={styles.radioGroup}>
                      <label className={styles.radioLabel}>
                        <input
                          type="radio"
                          value="terraform"
                          checked={formData.variable_type === 'terraform'}
                          onChange={(e) => setFormData({ ...formData, variable_type: e.target.value as 'terraform' | 'environment', hcl: false })}
                        />
                        <div>
                          <strong>Terraform variable</strong>
                          <p>These variables should match the declarations in your configuration. Click the HCL box to use interpolation or set a non-string value.</p>
                        </div>
                      </label>
                      <label className={styles.radioLabel}>
                        <input
                          type="radio"
                          value="environment"
                          checked={formData.variable_type === 'environment'}
                          onChange={(e) => setFormData({ ...formData, variable_type: e.target.value as 'terraform' | 'environment', hcl: false })}
                        />
                        <div>
                          <strong>Environment variable</strong>
                          <p>These variables are available in the Terraform runtime environment.</p>
                        </div>
                      </label>
                    </div>
                  </div>

                  <div className={styles.formRow}>
                    <div className={styles.formGroup}>
                      <label className={styles.formLabel}>Key</label>
                      <input
                        type="text"
                        value={formData.key}
                        onChange={(e) => setFormData({ ...formData, key: e.target.value })}
                        className={styles.formInput}
                        placeholder="key"
                        required
                      />
                    </div>
                    <div className={styles.formGroup}>
                      <label className={styles.formLabel}>Value</label>
                      <input
                        type="text"
                        value={formData.value}
                        onChange={(e) => setFormData({ ...formData, value: e.target.value })}
                        className={styles.formInput}
                        placeholder="value"
                        required
                      />
                    </div>
                    <div className={styles.formCheckboxes}>
                      {formData.variable_type === 'terraform' && (
                        <label className={styles.checkboxLabel}>
                          <input
                            type="checkbox"
                            checked={formData.hcl}
                            onChange={(e) => setFormData({ ...formData, hcl: e.target.checked })}
                          />
                          <span>HCL</span>
                        </label>
                      )}
                      <label className={styles.checkboxLabel}>
                        <input
                          type="checkbox"
                          checked={formData.sensitive}
                          onChange={(e) => setFormData({ ...formData, sensitive: e.target.checked })}
                          disabled={editingId !== null && formData.sensitive}
                        />
                        <span>Sensitive</span>
                        {editingId !== null && formData.sensitive && (
                          <span className={styles.disabledHint}>(cannot be disabled)</span>
                        )}
                      </label>
                    </div>
                  </div>

                  <div className={styles.formGroup}>
                    <label className={styles.formLabel}>Description (Optional)</label>
                    <textarea
                      value={formData.description}
                      onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                      className={styles.formTextarea}
                      placeholder="description (optional)"
                      rows={2}
                    />
                  </div>

                  <div className={styles.formActions}>
                    <button type="submit" className={styles.primaryButton}>
                      Update variable
                    </button>
                    <button type="button" onClick={handleCancel} className={styles.cancelButton}>
                      Cancel
                    </button>
                  </div>
                </form>
              </div>
            ) : (
              // 变量行
              <div className={styles.variableRow} id={`var-${variable.variable_id}`}>
                <div className={styles.variableKeyCell}>
                  <div className={styles.variableKey}>
                    {variable.key}
                    {variable.value_format === 'hcl' && (
                      <span className={styles.hclBadge}>HCL</span>
                    )}
                    {variable.sensitive && (
                      <span className={styles.sensitiveBadge}>Sensitive</span>
                    )}
                  </div>
                  {variable.description && (
                    <div className={styles.variableDescription}>
                      {variable.description}
                    </div>
                  )}
                </div>
                <div className={styles.variableValue}>
                  {variable.sensitive ? '-- sensitive value --' : variable.value}
                </div>
                <div className={styles.variableCategory}>
                  {variable.variable_type === 'terraform' ? 'Terraform' : 'Environment'}
                </div>
                <div className={styles.variableActions}>
                  <div className={styles.menuContainer}>
                    <button 
                      onClick={() => setShowMenu(showMenu === variable.id ? null : variable.id)} 
                      className={styles.deleteButton}
                    >
                      ⋯
                    </button>
                    {showMenu === variable.id && (
                      <div className={styles.dropdownMenu}>
                        <button onClick={() => handleEdit(variable)} className={styles.menuItem}>
                          Edit variable
                        </button>
                        <button onClick={() => handleDeleteClick(variable)} className={styles.menuItemDanger}>
                          Delete
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            )}
          </React.Fragment>
        ))}

        {/* Add form */}
        {showAddForm && !editingId ? (
          <form onSubmit={handleSubmit} className={styles.addVariableForm}>
            <div className={styles.formSection}>
              <h4 className={styles.formSectionTitle}>Add new variable</h4>
              <div className={styles.radioGroup}>
                <label className={styles.radioLabel}>
                  <input
                    type="radio"
                    value="terraform"
                    checked={formData.variable_type === 'terraform'}
                    onChange={(e) => setFormData({ ...formData, variable_type: e.target.value as 'terraform' | 'environment', hcl: false })}
                  />
                  <div>
                    <strong>Terraform variable</strong>
                    <p>These variables should match the declarations in your configuration. Click the HCL box to use interpolation or set a non-string value.</p>
                  </div>
                </label>
                <label className={styles.radioLabel}>
                  <input
                    type="radio"
                    value="environment"
                    checked={formData.variable_type === 'environment'}
                    onChange={(e) => setFormData({ ...formData, variable_type: e.target.value as 'terraform' | 'environment', hcl: false })}
                  />
                  <div>
                    <strong>Environment variable</strong>
                    <p>These variables are available in the Terraform runtime environment.</p>
                  </div>
                </label>
              </div>
            </div>

            <div className={styles.formRow}>
              <div className={styles.formGroup}>
                <label className={styles.formLabel}>Key</label>
                <input
                  type="text"
                  value={formData.key}
                  onChange={(e) => setFormData({ ...formData, key: e.target.value })}
                  className={styles.formInput}
                  placeholder="key"
                  required
                />
              </div>
              <div className={styles.formGroup}>
                <label className={styles.formLabel}>Value</label>
                <input
                  type="text"
                  value={formData.value}
                  onChange={(e) => setFormData({ ...formData, value: e.target.value })}
                  className={styles.formInput}
                  placeholder="value"
                  required
                />
              </div>
              <div className={styles.formCheckboxes}>
                {formData.variable_type === 'terraform' && (
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={formData.hcl}
                      onChange={(e) => setFormData({ ...formData, hcl: e.target.checked })}
                    />
                    <span>HCL</span>
                  </label>
                )}
                <label className={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={formData.sensitive}
                    onChange={(e) => setFormData({ ...formData, sensitive: e.target.checked })}
                  />
                  <span>Sensitive</span>
                </label>
              </div>
            </div>

            <div className={styles.formGroup}>
              <label className={styles.formLabel}>Description (Optional)</label>
              <textarea
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                className={styles.formTextarea}
                placeholder="description (optional)"
                rows={2}
              />
            </div>

            <div className={styles.formActions}>
              <button type="submit" className={styles.primaryButton}>
                Add variable
              </button>
              <button type="button" onClick={handleCancel} className={styles.cancelButton}>
                Cancel
              </button>
            </div>
          </form>
        ) : !editingId && (
          <button onClick={() => setShowAddForm(true)} className={styles.addVariableButton}>
            + Add variable
          </button>
        )}
      </div>

      {loading && (
        <div className={styles.section}>
          <div className={styles.loading}>加载中...</div>
        </div>
      )}

      {/* Effective Variables Section */}
      <div className={styles.section}>
        <h3 className={styles.sectionTitle} style={{ marginBottom: '12px' }}>
          Variable Set Variables
        </h3>
        <div>
            {effectiveLoading ? (
              <div className={styles.loading}>Loading effective variables...</div>
            ) : effectiveVars.filter(ev => ev.source_type === 'varset').length === 0 ? (
              <p className={styles.infoText}>No variable set variables attached to this workspace.</p>
            ) : (
              <>
                <p className={styles.infoText} style={{ marginBottom: '12px' }}>
                  以下为通过 Variable Set 注入的变量。被 Workspace 自身变量覆盖的会标记为 Overridden。
                </p>
                {/* Effective variables table header */}
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: '2fr 2fr 1fr 1.5fr 100px',
                  gap: 'var(--spacing-md)',
                  padding: 'var(--spacing-sm) var(--spacing-md)',
                  background: 'var(--color-gray-50)',
                  border: '1px solid var(--color-gray-200)',
                  borderRadius: 'var(--radius-md) var(--radius-md) 0 0',
                  fontSize: '12px',
                  fontWeight: 600,
                  color: 'var(--color-gray-600)',
                  textTransform: 'uppercase' as const,
                }}>
                  <div>Key</div>
                  <div>Value</div>
                  <div>Type</div>
                  <div>Source</div>
                  <div>Status</div>
                </div>

                {/* Effective variables rows */}
                {effectiveVars.filter(ev => ev.source_type === 'varset').map((ev) => (
                  <div
                    key={`${ev.variable_id}-${ev.source_id}`}
                    id={`var-${ev.variable_id}`}
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '2fr 2fr 1fr 1.5fr 100px',
                      gap: 'var(--spacing-md)',
                      padding: 'var(--spacing-md)',
                      border: '1px solid var(--color-gray-200)',
                      borderTop: 'none',
                      background: 'var(--color-white)',
                      alignItems: 'center',
                      textDecoration: ev.is_overridden ? 'line-through' : 'none',
                      opacity: ev.is_overridden ? 0.6 : 1,
                    }}
                  >
                    <div style={{ fontWeight: 500, color: 'var(--color-gray-900)', fontSize: '14px' }}>
                      {ev.key}
                    </div>
                    <div style={{ color: 'var(--color-gray-700)', fontSize: '13px', wordBreak: 'break-all' }}>
                      {ev.is_overridden && ev.overridden_by ? (
                        <>
                          <a
                            href={`#var-${ev.overridden_by.variable_id}`}
                            style={{ color: 'var(--color-blue-600)', textDecoration: 'none', fontWeight: 500 }}
                            onClick={(e) => {
                              e.preventDefault();
                              const el = document.getElementById(`var-${ev.overridden_by!.variable_id}`);
                              if (el) {
                                el.scrollIntoView({ behavior: 'smooth', block: 'center' });
                                el.style.background = '#DBEAFE';
                                setTimeout(() => { el.style.background = ''; }, 2000);
                              }
                            }}
                          >
                            Overridden
                          </a>
                          {' '}{ev.key}
                        </>
                      ) : ev.sensitive ? (
                        <span style={{ fontStyle: 'italic', color: 'var(--color-gray-500)' }}>-- sensitive --</span>
                      ) : (
                        ev.value
                      )}
                    </div>
                    <div style={{ color: 'var(--color-gray-600)', fontSize: '13px' }}>
                      {ev.variable_type === 'terraform' ? 'Terraform' : 'Environment'}
                    </div>
                    <div style={{ color: 'var(--color-gray-600)', fontSize: '13px' }}>
                      {formatSource(ev)}
                    </div>
                    <div>
                      {ev.is_overridden ? (
                        <span style={{
                          background: 'var(--color-gray-100)',
                          color: 'var(--color-gray-600)',
                          padding: '3px 10px',
                          borderRadius: '12px',
                          fontSize: '12px',
                          fontWeight: 500,
                        }}>
                          Overridden
                        </span>
                      ) : (
                        <span style={{
                          background: '#D1FAE5',
                          color: '#065F46',
                          padding: '3px 10px',
                          borderRadius: '12px',
                          fontSize: '12px',
                          fontWeight: 500,
                        }}>
                          Active
                        </span>
                      )}
                    </div>
                  </div>
                ))}
              </>
            )}
          </div>
      </div>

      {/* Delete Confirmation Dialog */}
      <ConfirmDialog
        isOpen={deleteDialogOpen}
        title={`Delete ${variableToDelete?.key} variable`}
        confirmText="Yes, delete variable"
        cancelText="Cancel"
        onConfirm={handleDeleteConfirm}
        onCancel={handleDeleteCancel}
      >
        <div style={{ marginBottom: '16px' }}>
          <p style={{ margin: '0 0 12px 0', color: 'var(--color-gray-700)', fontSize: '14px', lineHeight: '1.5' }}>
            Deleting the <strong>{variableToDelete?.key}</strong> variable will permanently remove its value, and it will no longer be used in future runs.
          </p>
          <p style={{ margin: 0, color: 'var(--color-gray-700)', fontSize: '14px', lineHeight: '1.5' }}>
            This operation <strong>cannot be undone</strong>. Are you sure?
          </p>
        </div>
      </ConfirmDialog>
    </div>
  );
};

export default VariablesTab;
