import React, { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { variableSetService } from '../services/variableSets';
import type { VariableSet, VarsetVariable, VarsetAssignment } from '../services/variableSets';
import { getProjects, type Project } from '../services/projects';
import { workspaceService, type Workspace } from '../services/workspaces';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './Admin.module.css';

const VariableSetDetail: React.FC = () => {
  const { varsetId } = useParams<{ varsetId: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();

  const [varset, setVarset] = useState<VariableSet | null>(null);
  const [variables, setVariables] = useState<VarsetVariable[]>([]);
  const [assignments, setAssignments] = useState<VarsetAssignment[]>([]);
  const [loading, setLoading] = useState(true);

  // Edit varset info
  const [editingInfo, setEditingInfo] = useState(false);
  const [infoForm, setInfoForm] = useState({ name: '', description: '' });

  // Variable form
  const [showVarForm, setShowVarForm] = useState(false);
  const [editingVar, setEditingVar] = useState<VarsetVariable | null>(null);
  const [varFormData, setVarFormData] = useState({
    key: '', value: '', description: '',
    variable_type: 'terraform' as 'terraform' | 'environment',
    value_format: 'string' as 'string' | 'hcl',
    sensitive: false,
  });

  // Variable delete
  const [deleteVarDialog, setDeleteVarDialog] = useState(false);
  const [varToDelete, setVarToDelete] = useState<VarsetVariable | null>(null);

  // Assignment form
  const [showAssignForm, setShowAssignForm] = useState(false);
  const [assignFormData, setAssignFormData] = useState({
    scope_type: 'workspace' as 'project' | 'workspace',
    project_id: '' as string | number,
    workspace_id: '',
  });

  // Assignment delete
  const [deleteAssignDialog, setDeleteAssignDialog] = useState(false);
  const [assignToDelete, setAssignToDelete] = useState<VarsetAssignment | null>(null);

  // Picker data
  const [projects, setProjects] = useState<Project[]>([]);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [pickerLoading, setPickerLoading] = useState(false);

  const loadData = useCallback(async () => {
    if (!varsetId) return;
    try {
      setLoading(true);
      const [vsData, varsData, assignData, projectList, wsResponse] = await Promise.all([
        variableSetService.get(varsetId),
        variableSetService.listVariables(varsetId),
        variableSetService.listAssignments(varsetId),
        getProjects().catch(() => []),
        workspaceService.getWorkspaces().catch(() => ({ data: [] })),
      ]);
      setVarset(vsData);
      setVariables(Array.isArray(varsData) ? varsData : []);
      setAssignments(assignData.items || []);
      setProjects(projectList || []);
      const wsRaw = wsResponse as any;
      const wsItems = wsRaw?.data?.items || wsRaw?.items || wsRaw?.data || wsRaw || [];
      setWorkspaces(Array.isArray(wsItems) ? wsItems : []);
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Failed to load variable set', 'error');
    } finally {
      setLoading(false);
    }
  }, [varsetId, showToast]);

  useEffect(() => { loadData(); }, [loadData]);

  // --- Info edit ---
  const startEditInfo = () => {
    if (!varset) return;
    setInfoForm({ name: varset.name, description: varset.description || '' });
    setEditingInfo(true);
  };
  const cancelEditInfo = () => setEditingInfo(false);
  const saveInfo = async () => {
    if (!varsetId) return;
    try {
      await variableSetService.update(varsetId, { name: infoForm.name, description: infoForm.description });
      showToast('更新成功', 'success');
      setEditingInfo(false);
      loadData();
    } catch (error: any) {
      showToast(error.response?.data?.error || '更新失败', 'error');
    }
  };

  // --- Variable handlers ---
  const resetVarForm = () => setVarFormData({ key: '', value: '', description: '', variable_type: 'terraform', value_format: 'string', sensitive: false });

  const handleAddVar = () => { setEditingVar(null); resetVarForm(); setShowVarForm(true); };
  const handleEditVar = (v: VarsetVariable) => {
    setEditingVar(v);
    setVarFormData({ key: v.key, value: v.sensitive ? '' : v.value, description: v.description, variable_type: v.variable_type, value_format: v.value_format, sensitive: v.sensitive });
    setShowVarForm(true);
  };
  const handleCancelVar = () => { setShowVarForm(false); setEditingVar(null); resetVarForm(); };

  const handleSubmitVar = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!varsetId || !varFormData.key.trim()) { showToast('Key is required', 'error'); return; }
    try {
      if (editingVar) {
        await variableSetService.updateVariable(varsetId, editingVar.variable_id, { value: varFormData.value, description: varFormData.description, sensitive: varFormData.sensitive });
        showToast('Variable updated', 'success');
      } else {
        await variableSetService.createVariable(varsetId, { key: varFormData.key, value: varFormData.value, description: varFormData.description, variable_type: varFormData.variable_type, value_format: varFormData.value_format, sensitive: varFormData.sensitive });
        showToast('Variable created', 'success');
      }
      handleCancelVar();
      loadData();
    } catch (error: any) { showToast(error.response?.data?.error || 'Operation failed', 'error'); }
  };

  const handleDeleteVarClick = (v: VarsetVariable) => { setVarToDelete(v); setDeleteVarDialog(true); };
  const handleDeleteVarConfirm = async () => {
    if (!varsetId || !varToDelete) return;
    try {
      await variableSetService.deleteVariable(varsetId, varToDelete.variable_id);
      showToast('Variable deleted', 'success');
      setDeleteVarDialog(false); setVarToDelete(null); loadData();
    } catch (error: any) { showToast(error.response?.data?.error || 'Delete failed', 'error'); }
  };

  // --- Assignment handlers ---
  const handleAddAssign = () => {
    setAssignFormData({ scope_type: 'workspace', project_id: '', workspace_id: '' });
    setShowAssignForm(true);
    // Refresh picker data
    setPickerLoading(true);
    Promise.all([getProjects().catch(() => []), workspaceService.getWorkspaces().catch(() => ({ data: [] }))])
      .then(([pl, wr]) => {
        setProjects(pl || []);
        const raw = wr as any;
        setWorkspaces(Array.isArray(raw?.data?.items || raw?.items || raw?.data || raw) ? (raw?.data?.items || raw?.items || raw?.data || raw) : []);
      })
      .finally(() => setPickerLoading(false));
  };
  const handleCancelAssign = () => setShowAssignForm(false);

  const handleSubmitAssign = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!varsetId) return;
    const data: any = { scope_type: assignFormData.scope_type };
    if (assignFormData.scope_type === 'project') {
      if (!assignFormData.project_id) { showToast('请选择 Project', 'error'); return; }
      data.project_id = Number(assignFormData.project_id);
    } else {
      if (!assignFormData.workspace_id) { showToast('请选择 Workspace', 'error'); return; }
      data.workspace_id = assignFormData.workspace_id;
    }
    try {
      await variableSetService.createAssignment(varsetId, data);
      showToast('Assignment created', 'success');
      setShowAssignForm(false); loadData();
    } catch (error: any) { showToast(error.response?.data?.error || 'Operation failed', 'error'); }
  };

  const handleDeleteAssignClick = (a: VarsetAssignment) => { setAssignToDelete(a); setDeleteAssignDialog(true); };
  const handleDeleteAssignConfirm = async () => {
    if (!varsetId || !assignToDelete) return;
    try {
      await variableSetService.deleteAssignment(varsetId, assignToDelete.id);
      showToast('Assignment removed', 'success');
      setDeleteAssignDialog(false); setAssignToDelete(null); loadData();
    } catch (error: any) { showToast(error.response?.data?.error || 'Delete failed', 'error'); }
  };

  // --- Helpers ---
  const renderScopeBadge = (scope: string) => scope === 'global'
    ? <span className={styles.statusBadge} style={{ backgroundColor: '#EFF6FF', color: '#2563EB', border: '1px solid #93C5FD' }}>Global</span>
    : <span className={styles.statusBadge} style={{ backgroundColor: '#F0FDF4', color: '#16A34A', border: '1px solid #86EFAC' }}>Specific</span>;

  const formatDate = (dateStr: string) => new Date(dateStr).toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });

  const btnStyle: React.CSSProperties = { padding: '5px 12px', border: '1px solid var(--color-gray-300)', background: 'var(--color-white)', color: 'var(--color-gray-700)', borderRadius: '6px', fontSize: '13px', fontWeight: 500, cursor: 'pointer' };
  const btnDeleteStyle: React.CSSProperties = { ...btnStyle, color: 'var(--color-red-500)', borderColor: 'var(--color-red-200)' };

  if (loading && !varset) return <div className={styles.container}><div className={styles.loading}>Loading...</div></div>;
  if (!varset) return <div className={styles.container}><div className={styles.empty}><div className={styles.emptyText}>Variable set not found</div></div></div>;

  return (
    <div className={styles.container}>
      {/* Header */}
      <div className={styles.header}>
        <div style={{ marginBottom: '8px' }}>
          <span style={{ color: 'var(--color-blue-600)', cursor: 'pointer', fontSize: '14px' }} onClick={() => navigate('/variable-sets')}>
            &larr; Back to Variable Sets
          </span>
        </div>
        {editingInfo ? (
          <div className={styles.inlineForm} style={{ marginBottom: '16px' }}>
            <div className={styles.inlineFormBody}>
              <div className={styles.inlineFormGrid}>
                <div className={styles.formGroup}>
                  <label className={styles.label}>名称</label>
                  <input className={styles.input} value={infoForm.name} onChange={(e) => setInfoForm({ ...infoForm, name: e.target.value })} />
                </div>
                <div className={`${styles.formGroup} ${styles.inlineFormFull}`}>
                  <label className={styles.label}>描述</label>
                  <textarea className={styles.textarea} value={infoForm.description} onChange={(e) => setInfoForm({ ...infoForm, description: e.target.value })} rows={2} />
                </div>
              </div>
            </div>
            <div className={styles.inlineFormFooter}>
              <button className={`${styles.button} ${styles.secondary}`} onClick={cancelEditInfo}>取消</button>
              <button className={`${styles.button} ${styles.primary}`} onClick={saveInfo}>保存</button>
            </div>
          </div>
        ) : (
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <h1 className={styles.title} style={{ marginBottom: 0 }}>{varset.name}</h1>
            {renderScopeBadge(varset.scope)}
            <button style={btnStyle} onClick={startEditInfo}>编辑</button>
          </div>
        )}
        {!editingInfo && varset.description && <p className={styles.description}>{varset.description}</p>}
      </div>

      {/* ====== Variables Section ====== */}
      <div style={{ marginBottom: '32px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
          <h2 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--color-gray-900)', margin: 0 }}>Variables ({variables.length})</h2>
          {!showVarForm && <button className={styles.addButton} onClick={handleAddVar}>+ Add variable</button>}
        </div>

        {/* Variable list */}
        <div className={styles.versionsList}>
          {variables.length === 0 && !showVarForm ? (
            <div className={styles.empty}>
              <div className={styles.emptyText}>No variables yet</div>
              <div className={styles.emptyHint}>Add variables to this variable set</div>
            </div>
          ) : variables.length > 0 && (
            <table className={styles.versionsTable}>
              <thead><tr><th>Key</th><th>Value</th><th>Type</th><th>Format</th><th>Description</th><th>Actions</th></tr></thead>
              <tbody>
                {variables.map((v) => (
                  <tr key={v.id}>
                    <td style={{ fontWeight: 500, fontFamily: 'var(--font-mono)' }}>{v.key}</td>
                    <td style={{ fontFamily: 'var(--font-mono)', color: v.sensitive ? 'var(--color-gray-400)' : undefined }}>
                      {v.sensitive ? '-- sensitive --' : (v.value || <span style={{ color: 'var(--color-gray-400)' }}>(empty)</span>)}
                    </td>
                    <td><span className={styles.typeBadge}>{v.variable_type === 'terraform' ? 'Terraform' : 'Environment'}</span></td>
                    <td>{v.value_format === 'hcl' ? <span className={styles.typeBadge} style={{ background: '#FEF3C7', color: '#92400E', border: '1px solid #FCD34D' }}>HCL</span> : <span style={{ color: 'var(--color-gray-500)', fontSize: '13px' }}>string</span>}</td>
                    <td style={{ color: 'var(--color-gray-600)', fontSize: '13px', maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis' }}>{v.description || '-'}</td>
                    <td>
                      <div style={{ display: 'flex', gap: '6px' }}>
                        <button style={btnStyle} onClick={() => handleEditVar(v)}>编辑</button>
                        <button style={btnDeleteStyle} onClick={() => handleDeleteVarClick(v)}>删除</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        {/* Variable form */}
        {showVarForm && (
          <div className={styles.inlineForm} style={{ marginTop: '12px' }}>
            <div className={styles.inlineFormHeader}>
              <h3 className={styles.inlineFormTitle}>{editingVar ? `Edit: ${editingVar.key}` : 'Add variable'}</h3>
              <button className={styles.inlineFormClose} onClick={handleCancelVar}>x</button>
            </div>
            <form onSubmit={handleSubmitVar}>
              <div className={styles.inlineFormBody}>
                <div className={styles.inlineFormGrid}>
                  {/* Type */}
                  <div className={styles.formGroup}>
                    <label className={styles.label}>Type</label>
                    {editingVar ? (
                      <div style={{ padding: '8px 0', fontSize: '14px' }}>
                        <span className={styles.typeBadge}>{varFormData.variable_type === 'terraform' ? 'Terraform' : 'Environment'}</span>
                        <span style={{ color: 'var(--color-gray-400)', fontSize: '12px', marginLeft: '8px' }}>(immutable)</span>
                      </div>
                    ) : (
                      <div style={{ display: 'flex', gap: '16px', marginTop: '4px' }}>
                        <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                          <input type="radio" value="terraform" checked={varFormData.variable_type === 'terraform'} onChange={() => setVarFormData({ ...varFormData, variable_type: 'terraform' })} /> Terraform
                        </label>
                        <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                          <input type="radio" value="environment" checked={varFormData.variable_type === 'environment'} onChange={() => setVarFormData({ ...varFormData, variable_type: 'environment' })} /> Environment
                        </label>
                      </div>
                    )}
                  </div>
                  {/* Format */}
                  <div className={styles.formGroup}>
                    <label className={styles.label}>Format</label>
                    {editingVar ? (
                      <div style={{ padding: '8px 0', fontSize: '14px' }}>{varFormData.value_format === 'hcl' ? 'HCL' : 'String'} <span style={{ color: 'var(--color-gray-400)', fontSize: '12px' }}>(immutable)</span></div>
                    ) : (
                      <div style={{ display: 'flex', gap: '16px', marginTop: '4px' }}>
                        <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                          <input type="radio" value="string" checked={varFormData.value_format === 'string'} onChange={() => setVarFormData({ ...varFormData, value_format: 'string' })} /> String
                        </label>
                        <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                          <input type="radio" value="hcl" checked={varFormData.value_format === 'hcl'} onChange={() => setVarFormData({ ...varFormData, value_format: 'hcl' })} /> HCL
                        </label>
                      </div>
                    )}
                  </div>
                  {/* Key */}
                  <div className={styles.formGroup}>
                    <label className={styles.label}>Key<span className={styles.required}>*</span></label>
                    {editingVar ? (
                      <div style={{ padding: '8px 12px', background: 'var(--color-gray-50)', borderRadius: '6px', fontSize: '14px', fontFamily: 'var(--font-mono)', border: '1px solid var(--color-gray-200)' }}>{varFormData.key}</div>
                    ) : (
                      <input className={styles.input} value={varFormData.key} onChange={(e) => setVarFormData({ ...varFormData, key: e.target.value })} placeholder="e.g. AWS_ACCESS_KEY_ID" />
                    )}
                  </div>
                  {/* Sensitive */}
                  <div className={styles.formGroup}>
                    <label className={styles.label}>Sensitive</label>
                    <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: editingVar?.sensitive ? 'not-allowed' : 'pointer', fontSize: '14px', marginTop: '4px' }}>
                      <input type="checkbox" checked={varFormData.sensitive} onChange={(e) => setVarFormData({ ...varFormData, sensitive: e.target.checked })} disabled={editingVar?.sensitive === true} style={{ width: '16px', height: '16px' }} />
                      Mark as sensitive
                      {editingVar?.sensitive && <span style={{ color: 'var(--color-gray-400)', fontSize: '12px' }}>(cannot be disabled)</span>}
                    </label>
                  </div>
                  {/* Value */}
                  <div className={`${styles.formGroup} ${styles.inlineFormFull}`}>
                    <label className={styles.label}>Value</label>
                    <textarea className={styles.textarea} value={varFormData.value} onChange={(e) => setVarFormData({ ...varFormData, value: e.target.value })} placeholder={editingVar?.sensitive ? 'Enter new sensitive value' : 'Variable value'} rows={3} />
                  </div>
                  {/* Description */}
                  <div className={`${styles.formGroup} ${styles.inlineFormFull}`}>
                    <label className={styles.label}>Description</label>
                    <textarea className={styles.textarea} value={varFormData.description} onChange={(e) => setVarFormData({ ...varFormData, description: e.target.value })} placeholder="Optional" rows={2} />
                  </div>
                </div>
              </div>
              <div className={styles.inlineFormFooter}>
                <button type="button" className={`${styles.button} ${styles.secondary}`} onClick={handleCancelVar}>Cancel</button>
                <button type="submit" className={`${styles.button} ${styles.primary}`}>{editingVar ? 'Update' : 'Add'}</button>
              </div>
            </form>
          </div>
        )}
      </div>

      {/* ====== Assignments Section ====== */}
      {varset.scope === 'global' ? (
        <div style={{ padding: '16px 20px', background: '#EFF6FF', border: '1px solid #93C5FD', borderRadius: '8px', color: '#1E40AF', fontSize: '14px', lineHeight: '1.6' }}>
          This variable set applies to all workspaces globally. No manual assignments needed.
        </div>
      ) : (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
            <h2 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--color-gray-900)', margin: 0 }}>Assignments ({assignments.length})</h2>
            {!showAssignForm && <button className={styles.addButton} onClick={handleAddAssign}>+ Add assignment</button>}
          </div>

          <div className={styles.versionsList}>
            {assignments.length === 0 && !showAssignForm ? (
              <div className={styles.empty}>
                <div className={styles.emptyText}>No assignments yet</div>
                <div className={styles.emptyHint}>Assign this variable set to projects or workspaces</div>
              </div>
            ) : assignments.length > 0 && (
              <table className={styles.versionsTable}>
                <thead><tr><th>Type</th><th>Target</th><th>Attached At</th><th>Actions</th></tr></thead>
                <tbody>
                  {assignments.map((a) => (
                    <tr key={a.id}>
                      <td>
                        {a.scope_type === 'project'
                          ? <span className={styles.statusBadge} style={{ backgroundColor: '#FEF3C7', color: '#92400E', border: '1px solid #FCD34D' }}>Project</span>
                          : <span className={styles.statusBadge} style={{ backgroundColor: '#EDE9FE', color: '#6D28D9', border: '1px solid #C4B5FD' }}>Workspace</span>}
                      </td>
                      <td style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                        {a.scope_type === 'project'
                          ? (projects.find(p => p.id === a.project_id)?.display_name || projects.find(p => p.id === a.project_id)?.name || `Project #${a.project_id}`)
                          : (workspaces.find(w => w.workspace_id === a.workspace_id)?.name || a.workspace_id || '-')}
                      </td>
                      <td style={{ color: 'var(--color-gray-500)', fontSize: '13px' }}>{a.attached_at ? formatDate(a.attached_at) : '-'}</td>
                      <td><button style={btnDeleteStyle} onClick={() => handleDeleteAssignClick(a)}>Remove</button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {/* Assignment form */}
          {showAssignForm && (
            <div className={styles.inlineForm} style={{ marginTop: '12px' }}>
              <div className={styles.inlineFormHeader}>
                <h3 className={styles.inlineFormTitle}>Add assignment</h3>
                <button className={styles.inlineFormClose} onClick={handleCancelAssign}>x</button>
              </div>
              <form onSubmit={handleSubmitAssign}>
                <div className={styles.inlineFormBody}>
                  <div className={styles.inlineFormGrid}>
                    <div className={styles.formGroup}>
                      <label className={styles.label}>Scope type</label>
                      <div style={{ display: 'flex', gap: '16px', marginTop: '4px' }}>
                        <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                          <input type="radio" value="project" checked={assignFormData.scope_type === 'project'} onChange={() => setAssignFormData({ ...assignFormData, scope_type: 'project' })} /> Project
                        </label>
                        <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                          <input type="radio" value="workspace" checked={assignFormData.scope_type === 'workspace'} onChange={() => setAssignFormData({ ...assignFormData, scope_type: 'workspace' })} /> Workspace
                        </label>
                      </div>
                    </div>
                    <div className={styles.formGroup}>
                      {pickerLoading ? (
                        <div style={{ padding: '8px 0', color: 'var(--color-gray-500)', fontSize: '14px' }}>Loading...</div>
                      ) : assignFormData.scope_type === 'project' ? (
                        <>
                          <label className={styles.label}>Project<span className={styles.required}>*</span></label>
                          <select className={styles.input} value={assignFormData.project_id} onChange={(e) => setAssignFormData({ ...assignFormData, project_id: e.target.value })}>
                            <option value="">-- Select Project --</option>
                            {projects.map((p) => <option key={p.id} value={p.id}>{p.display_name || p.name} (ID: {p.id})</option>)}
                          </select>
                        </>
                      ) : (
                        <>
                          <label className={styles.label}>Workspace<span className={styles.required}>*</span></label>
                          <select className={styles.input} value={assignFormData.workspace_id} onChange={(e) => setAssignFormData({ ...assignFormData, workspace_id: e.target.value })}>
                            <option value="">-- Select Workspace --</option>
                            {workspaces.map((w) => <option key={w.workspace_id || w.id} value={w.workspace_id || ''}>{w.name} ({w.workspace_id})</option>)}
                          </select>
                        </>
                      )}
                    </div>
                  </div>
                </div>
                <div className={styles.inlineFormFooter}>
                  <button type="button" className={`${styles.button} ${styles.secondary}`} onClick={handleCancelAssign}>Cancel</button>
                  <button type="submit" className={`${styles.button} ${styles.primary}`}>Add</button>
                </div>
              </form>
            </div>
          )}
        </div>
      )}

      {/* Delete variable dialog */}
      <ConfirmDialog isOpen={deleteVarDialog} title={`Delete variable "${varToDelete?.key}"`} confirmText="Yes, delete" cancelText="Cancel" type="danger" onConfirm={handleDeleteVarConfirm} onCancel={() => { setDeleteVarDialog(false); setVarToDelete(null); }}>
        <p style={{ margin: 0, color: 'var(--color-gray-700)', fontSize: '14px', lineHeight: '1.5' }}>
          Deleting <strong>{varToDelete?.key}</strong> will permanently remove it. This <strong>cannot be undone</strong>.
        </p>
      </ConfirmDialog>

      {/* Delete assignment dialog */}
      <ConfirmDialog isOpen={deleteAssignDialog} title="Remove assignment" confirmText="Yes, remove" cancelText="Cancel" type="danger" onConfirm={handleDeleteAssignConfirm} onCancel={() => { setDeleteAssignDialog(false); setAssignToDelete(null); }}>
        <p style={{ margin: 0, color: 'var(--color-gray-700)', fontSize: '14px', lineHeight: '1.5' }}>
          Remove this {assignToDelete?.scope_type} assignment? The variable set will no longer apply to this target.
        </p>
      </ConfirmDialog>
    </div>
  );
};

export default VariableSetDetail;
