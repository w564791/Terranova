import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { variableSetService } from '../services/variableSets';
import { getProjects, type Project } from '../services/projects';
import { workspaceService, type Workspace } from '../services/workspaces';
import type { VariableSet } from '../services/variableSets';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './Admin.module.css';

interface PendingVar {
  key: string; value: string; description: string;
  variable_type: 'terraform' | 'environment';
  value_format: 'string' | 'hcl';
  sensitive: boolean;
}
interface PendingAssign {
  scope_type: 'project' | 'workspace';
  project_id?: number; workspace_id?: string;
  display: string;
}

const VariableSetsPage: React.FC = () => {
  const [varsets, setVarsets] = useState<VariableSet[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<VariableSet | null>(null);
  const { showToast } = useToast();
  const navigate = useNavigate();

  // Create form
  const [formData, setFormData] = useState({ name: '', description: '', scope: 'specific' as 'global' | 'specific' });
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  // Pending variables (collected before submit)
  const [pendingVars, setPendingVars] = useState<PendingVar[]>([]);
  const [varForm, setVarForm] = useState<PendingVar>({ key: '', value: '', description: '', variable_type: 'terraform', value_format: 'string', sensitive: false });
  const [showVarInput, setShowVarInput] = useState(false);

  // Pending assignments
  const [pendingAssigns, setPendingAssigns] = useState<PendingAssign[]>([]);
  const [assignForm, setAssignForm] = useState({ scope_type: 'workspace' as 'project' | 'workspace', project_id: '', workspace_id: '' });
  const [showAssignInput, setShowAssignInput] = useState(false);

  // Picker data
  const [projects, setProjects] = useState<Project[]>([]);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);

  const loadVarsets = async () => {
    try {
      setLoading(true);
      const response = await variableSetService.list();
      setVarsets(response.items || []);
    } catch (error: any) {
      showToast(error.response?.data?.error || '加载失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  const loadPickerData = async () => {
    try {
      const [pl, wr] = await Promise.all([getProjects().catch(() => []), workspaceService.getWorkspaces().catch(() => ({}))]);
      setProjects(pl || []);
      const raw = wr as any;
      const items = raw?.data?.items || raw?.items || raw?.data || raw || [];
      setWorkspaces(Array.isArray(items) ? items : []);
    } catch { /* ignore */ }
  };

  useEffect(() => { loadVarsets(); }, []);

  const resetForm = () => {
    setFormData({ name: '', description: '', scope: 'specific' });
    setFormErrors({});
    setPendingVars([]);
    setPendingAssigns([]);
    setShowVarInput(false);
    setShowAssignInput(false);
    setVarForm({ key: '', value: '', description: '', variable_type: 'terraform', value_format: 'string', sensitive: false });
  };

  const handleAdd = () => { resetForm(); setShowForm(true); loadPickerData(); };
  const handleCancel = () => { setShowForm(false); resetForm(); };

  // --- Add pending variable ---
  const addPendingVar = () => {
    if (!varForm.key.trim()) { showToast('Variable key is required', 'error'); return; }
    if (pendingVars.some(v => v.key === varForm.key.trim())) { showToast(`Key "${varForm.key}" already added`, 'error'); return; }
    setPendingVars([...pendingVars, { ...varForm, key: varForm.key.trim() }]);
    setVarForm({ key: '', value: '', description: '', variable_type: 'terraform', value_format: 'string', sensitive: false });
    setShowVarInput(false);
  };
  const removePendingVar = (idx: number) => setPendingVars(pendingVars.filter((_, i) => i !== idx));

  // --- Add pending assignment ---
  const addPendingAssign = () => {
    if (assignForm.scope_type === 'project') {
      if (!assignForm.project_id) { showToast('Please select a project', 'error'); return; }
      const pid = Number(assignForm.project_id);
      if (pendingAssigns.some(a => a.scope_type === 'project' && a.project_id === pid)) { showToast('Project already added', 'error'); return; }
      const proj = projects.find(p => p.id === pid);
      setPendingAssigns([...pendingAssigns, { scope_type: 'project', project_id: pid, display: proj?.display_name || proj?.name || `Project #${pid}` }]);
    } else {
      if (!assignForm.workspace_id) { showToast('Please select a workspace', 'error'); return; }
      if (pendingAssigns.some(a => a.scope_type === 'workspace' && a.workspace_id === assignForm.workspace_id)) { showToast('Workspace already added', 'error'); return; }
      const ws = workspaces.find(w => w.workspace_id === assignForm.workspace_id);
      setPendingAssigns([...pendingAssigns, { scope_type: 'workspace', workspace_id: assignForm.workspace_id, display: ws?.name || assignForm.workspace_id }]);
    }
    setAssignForm({ scope_type: 'workspace', project_id: '', workspace_id: '' });
    setShowAssignInput(false);
  };
  const removePendingAssign = (idx: number) => setPendingAssigns(pendingAssigns.filter((_, i) => i !== idx));

  // --- Submit all ---
  const handleSubmit = async () => {
    if (!formData.name.trim()) { setFormErrors({ name: 'required' }); return; }
    setSubmitting(true);
    try {
      // 1. Create varset
      const created = await variableSetService.create({ name: formData.name, description: formData.description, scope: formData.scope });
      const vid = created.varset_id;

      // 2. Create variables
      for (const v of pendingVars) {
        await variableSetService.createVariable(vid, { key: v.key, value: v.value, description: v.description, variable_type: v.variable_type, value_format: v.value_format, sensitive: v.sensitive });
      }

      // 3. Create assignments (only for specific scope)
      if (formData.scope === 'specific') {
        for (const a of pendingAssigns) {
          const data: any = { scope_type: a.scope_type };
          if (a.scope_type === 'project') data.project_id = a.project_id;
          else data.workspace_id = a.workspace_id;
          await variableSetService.createAssignment(vid, data);
        }
      }

      showToast(`Variable set "${formData.name}" created with ${pendingVars.length} variables and ${pendingAssigns.length} assignments`, 'success');
      setShowForm(false);
      resetForm();
      loadVarsets();
    } catch (error: any) {
      showToast(error.response?.data?.error || error || 'Create failed', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = (varset: VariableSet) => setDeleteConfirm(varset);
  const confirmDelete = async () => {
    if (!deleteConfirm) return;
    try {
      await variableSetService.delete(deleteConfirm.varset_id);
      showToast('Deleted', 'success');
      setDeleteConfirm(null);
      loadVarsets();
    } catch (error: any) { showToast(error.response?.data?.error || 'Delete failed', 'error'); }
  };

  const renderScopeBadge = (scope: string) => scope === 'global'
    ? <span className={styles.statusBadge} style={{ backgroundColor: '#EFF6FF', color: '#2563EB', border: '1px solid #93C5FD' }}>Global</span>
    : <span className={styles.statusBadge} style={{ backgroundColor: '#F0FDF4', color: '#16A34A', border: '1px solid #86EFAC' }}>Specific</span>;

  const formatDate = (d: string) => new Date(d).toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' });
  const btnStyle: React.CSSProperties = { padding: '5px 12px', border: '1px solid var(--color-gray-300)', background: 'var(--color-white)', color: 'var(--color-gray-700)', borderRadius: '6px', fontSize: '13px', fontWeight: 500, cursor: 'pointer' };
  const btnDeleteStyle: React.CSSProperties = { ...btnStyle, color: 'var(--color-red-500)', borderColor: 'var(--color-red-200)' };
  const btnSmall: React.CSSProperties = { ...btnStyle, padding: '3px 10px', fontSize: '12px' };

  return (
    <div className={styles.container} style={{ margin: '12px', background: '#f8fafc', border: '1px solid #e5e7eb', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.05)' }}>
      <div className={styles.header}>
        <h1 className={styles.title}>Variable Sets</h1>
        <p className={styles.description}>
          Variable Set allows grouping variables and applying them to multiple workspaces. Global applies to all workspaces automatically, Specific requires manual assignment.
        </p>
      </div>

      <div className={styles.actions}>
        <div></div>
        {!showForm && <button className={styles.addButton} onClick={handleAdd}>+ Create Variable Set</button>}
      </div>

      {/* ====== Inline Create Form ====== */}
      {showForm && (
        <div style={{ background: 'var(--color-white)', border: '1px solid #e5e7eb', borderRadius: '8px', padding: '20px', marginBottom: '20px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
            <h3 style={{ margin: 0, fontSize: '16px', fontWeight: 600 }}>Create Variable Set</h3>
            <button style={{ background: 'none', border: 'none', fontSize: '18px', cursor: 'pointer', color: 'var(--color-gray-500)' }} onClick={handleCancel}>x</button>
          </div>

          {/* Basic Info */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px', marginBottom: '20px' }}>
            <div>
              <label className={styles.label}>Name<span className={styles.required}>*</span></label>
              <input className={styles.input} value={formData.name} onChange={(e) => setFormData({ ...formData, name: e.target.value })} placeholder="e.g. Production AWS Credentials" />
              {formErrors.name && <span className={styles.errorText}>Name is required</span>}
            </div>
            <div>
              <label className={styles.label}>Scope<span className={styles.required}>*</span></label>
              <div style={{ display: 'flex', gap: '16px', marginTop: '6px' }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                  <input type="radio" checked={formData.scope === 'global'} onChange={() => setFormData({ ...formData, scope: 'global' })} /> Global
                </label>
                <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', fontSize: '14px' }}>
                  <input type="radio" checked={formData.scope === 'specific'} onChange={() => setFormData({ ...formData, scope: 'specific' })} /> Specific
                </label>
              </div>
            </div>
            <div style={{ gridColumn: '1 / -1' }}>
              <label className={styles.label}>Description</label>
              <textarea className={styles.textarea} value={formData.description} onChange={(e) => setFormData({ ...formData, description: e.target.value })} rows={2} placeholder="Optional" />
            </div>
          </div>

          {/* Variables Section */}
          <div style={{ borderTop: '1px solid var(--color-gray-200)', paddingTop: '16px', marginBottom: '20px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
              <h4 style={{ margin: 0, fontSize: '14px', fontWeight: 600, color: 'var(--color-gray-800)' }}>Variables ({pendingVars.length})</h4>
              {!showVarInput && <button style={btnSmall} onClick={() => setShowVarInput(true)}>+ Add</button>}
            </div>

            {/* Pending vars list */}
            {pendingVars.length > 0 && (
              <table className={styles.versionsTable} style={{ marginBottom: '8px' }}>
                <thead><tr><th>Key</th><th>Type</th><th>Format</th><th>Sensitive</th><th></th></tr></thead>
                <tbody>
                  {pendingVars.map((v, i) => (
                    <tr key={i}>
                      <td style={{ fontFamily: 'var(--font-mono)', fontWeight: 500 }}>{v.key}</td>
                      <td>{v.variable_type === 'terraform' ? 'Terraform' : 'Environment'}</td>
                      <td>{v.value_format}</td>
                      <td>{v.sensitive ? 'Yes' : 'No'}</td>
                      <td><button style={{ ...btnDeleteStyle, padding: '2px 8px', fontSize: '12px' }} onClick={() => removePendingVar(i)}>Remove</button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}

            {/* Add variable inline */}
            {showVarInput && (
              <div style={{ background: 'var(--color-gray-50)', border: '1px solid var(--color-gray-200)', borderRadius: '6px', padding: '12px', marginTop: '8px' }}>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '10px', marginBottom: '10px' }}>
                  <div>
                    <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)' }}>Key<span className={styles.required}>*</span></label>
                    <input className={styles.input} value={varForm.key} onChange={(e) => setVarForm({ ...varForm, key: e.target.value })} placeholder="AWS_ACCESS_KEY_ID" style={{ fontSize: '13px' }} />
                  </div>
                  <div>
                    <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)' }}>Type</label>
                    <select className={styles.input} value={varForm.variable_type} onChange={(e) => setVarForm({ ...varForm, variable_type: e.target.value as any })} style={{ fontSize: '13px' }}>
                      <option value="terraform">Terraform</option>
                      <option value="environment">Environment</option>
                    </select>
                  </div>
                  <div>
                    <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)' }}>Format</label>
                    <select className={styles.input} value={varForm.value_format} onChange={(e) => setVarForm({ ...varForm, value_format: e.target.value as any })} style={{ fontSize: '13px' }}>
                      <option value="string">String</option>
                      <option value="hcl">HCL</option>
                    </select>
                  </div>
                </div>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px', marginBottom: '10px' }}>
                  <div>
                    <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)' }}>Value</label>
                    <input className={styles.input} value={varForm.value} onChange={(e) => setVarForm({ ...varForm, value: e.target.value })} placeholder="Variable value" style={{ fontSize: '13px' }} />
                  </div>
                  <div>
                    <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)' }}>Description</label>
                    <input className={styles.input} value={varForm.description} onChange={(e) => setVarForm({ ...varForm, description: e.target.value })} placeholder="Optional" style={{ fontSize: '13px' }} />
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px', cursor: 'pointer' }}>
                    <input type="checkbox" checked={varForm.sensitive} onChange={(e) => setVarForm({ ...varForm, sensitive: e.target.checked })} /> Sensitive
                  </label>
                  <div style={{ display: 'flex', gap: '8px' }}>
                    <button style={btnSmall} onClick={() => setShowVarInput(false)}>Cancel</button>
                    <button style={{ ...btnSmall, background: 'var(--color-blue-500)', color: 'white', borderColor: 'var(--color-blue-500)' }} onClick={addPendingVar}>Add Variable</button>
                  </div>
                </div>
              </div>
            )}
          </div>

          {/* Assignments Section (only for specific) */}
          {formData.scope === 'specific' && (
            <div style={{ borderTop: '1px solid var(--color-gray-200)', paddingTop: '16px', marginBottom: '20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
                <h4 style={{ margin: 0, fontSize: '14px', fontWeight: 600, color: 'var(--color-gray-800)' }}>Assignments ({pendingAssigns.length})</h4>
                {!showAssignInput && <button style={btnSmall} onClick={() => setShowAssignInput(true)}>+ Add</button>}
              </div>

              {pendingAssigns.length > 0 && (
                <table className={styles.versionsTable} style={{ marginBottom: '8px' }}>
                  <thead><tr><th>Type</th><th>Target</th><th></th></tr></thead>
                  <tbody>
                    {pendingAssigns.map((a, i) => (
                      <tr key={i}>
                        <td>{a.scope_type === 'project' ? 'Project' : 'Workspace'}</td>
                        <td style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>{a.display}</td>
                        <td><button style={{ ...btnDeleteStyle, padding: '2px 8px', fontSize: '12px' }} onClick={() => removePendingAssign(i)}>Remove</button></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}

              {showAssignInput && (
                <div style={{ background: 'var(--color-gray-50)', border: '1px solid var(--color-gray-200)', borderRadius: '6px', padding: '12px', marginTop: '8px' }}>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                    <div>
                      <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)' }}>Type</label>
                      <select className={styles.input} value={assignForm.scope_type} onChange={(e) => setAssignForm({ ...assignForm, scope_type: e.target.value as any })} style={{ fontSize: '13px' }}>
                        <option value="workspace">Workspace</option>
                        <option value="project">Project</option>
                      </select>
                    </div>
                    <div>
                      <label style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-gray-600)' }}>
                        {assignForm.scope_type === 'project' ? 'Project' : 'Workspace'}<span className={styles.required}>*</span>
                      </label>
                      {assignForm.scope_type === 'project' ? (
                        <select className={styles.input} value={assignForm.project_id} onChange={(e) => setAssignForm({ ...assignForm, project_id: e.target.value })} style={{ fontSize: '13px' }}>
                          <option value="">-- Select --</option>
                          {projects.map(p => <option key={p.id} value={p.id}>{p.display_name || p.name}</option>)}
                        </select>
                      ) : (
                        <select className={styles.input} value={assignForm.workspace_id} onChange={(e) => setAssignForm({ ...assignForm, workspace_id: e.target.value })} style={{ fontSize: '13px' }}>
                          <option value="">-- Select --</option>
                          {workspaces.map(w => <option key={w.workspace_id || w.id} value={w.workspace_id || ''}>{w.name} ({w.workspace_id})</option>)}
                        </select>
                      )}
                    </div>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px', marginTop: '10px' }}>
                    <button style={btnSmall} onClick={() => setShowAssignInput(false)}>Cancel</button>
                    <button style={{ ...btnSmall, background: 'var(--color-blue-500)', color: 'white', borderColor: 'var(--color-blue-500)' }} onClick={addPendingAssign}>Add Assignment</button>
                  </div>
                </div>
              )}
            </div>
          )}

          {formData.scope === 'global' && (
            <div style={{ borderTop: '1px solid var(--color-gray-200)', paddingTop: '16px', marginBottom: '20px' }}>
              <div style={{ padding: '10px 14px', background: '#EFF6FF', border: '1px solid #93C5FD', borderRadius: '6px', color: '#1E40AF', fontSize: '13px' }}>
                Global scope: applies to all workspaces automatically.
              </div>
            </div>
          )}

          {/* Submit */}
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', borderTop: '1px solid var(--color-gray-200)', paddingTop: '16px' }}>
            <button className={`${styles.button} ${styles.secondary}`} onClick={handleCancel} disabled={submitting}>Cancel</button>
            <button className={`${styles.button} ${styles.primary}`} onClick={handleSubmit} disabled={submitting}>
              {submitting ? 'Creating...' : 'Create Variable Set'}
            </button>
          </div>
        </div>
      )}

      {/* ====== List ====== */}
      <div className={styles.versionsList} style={{ border: '1px solid #e5e7eb' }}>
        {loading ? (
          <div className={styles.loading}>Loading...</div>
        ) : varsets.length === 0 && !showForm ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>No variable sets</div>
            <div className={styles.emptyHint}>Click "Create Variable Set" to get started</div>
          </div>
        ) : varsets.length > 0 && (
          <table className={styles.versionsTable}>
            <thead><tr><th>Name</th><th>Scope</th><th>Variables</th><th>Assignments</th><th>Created</th><th>Actions</th></tr></thead>
            <tbody>
              {varsets.map((vs) => (
                <tr key={vs.varset_id} style={{ cursor: 'pointer' }} onClick={() => navigate(`/variable-sets/${vs.varset_id}`)}>
                  <td><span style={{ fontWeight: 500, color: 'var(--color-blue-600)' }}>{vs.name}</span></td>
                  <td>{renderScopeBadge(vs.scope)}</td>
                  <td><span style={{ color: 'var(--color-gray-600)' }}>{vs.variable_count ?? 0}</span></td>
                  <td><span style={{ color: 'var(--color-gray-600)' }}>{vs.scope === 'global' ? 'All' : (vs.assignment_count ?? 0)}</span></td>
                  <td><span style={{ color: 'var(--color-gray-500)', fontSize: '13px' }}>{formatDate(vs.created_at)}</span></td>
                  <td><button style={btnDeleteStyle} onClick={(e) => { e.stopPropagation(); handleDelete(vs); }}>Delete</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <ConfirmDialog isOpen={!!deleteConfirm} title={`Delete "${deleteConfirm?.name}"`} confirmText="Confirm" cancelText="Cancel" type="danger" onConfirm={confirmDelete} onCancel={() => setDeleteConfirm(null)}>
        <p style={{ margin: 0, color: 'var(--color-gray-700)', fontSize: '14px' }}>
          This will permanently remove <strong>{deleteConfirm?.name}</strong> and all its variables and assignments. This cannot be undone.
        </p>
      </ConfirmDialog>
    </div>
  );
};

export default VariableSetsPage;
