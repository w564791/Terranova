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
  const [projects, setProjects] = useState<Project[]>([]);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);

  // Edit info
  const [editingInfo, setEditingInfo] = useState(false);
  const [infoForm, setInfoForm] = useState({ name: '', description: '' });

  // Add variable
  const [showVarInput, setShowVarInput] = useState(false);
  const [editingVar, setEditingVar] = useState<VarsetVariable | null>(null);
  const [varForm, setVarForm] = useState({ key: '', value: '', description: '', variable_type: 'terraform' as 'terraform' | 'environment', value_format: 'string' as 'string' | 'hcl', sensitive: false });

  // Delete
  const [deleteVarDialog, setDeleteVarDialog] = useState(false);
  const [varToDelete, setVarToDelete] = useState<VarsetVariable | null>(null);
  const [deleteAssignDialog, setDeleteAssignDialog] = useState(false);
  const [assignToDelete, setAssignToDelete] = useState<VarsetAssignment | null>(null);

  const loadData = useCallback(async () => {
    if (!varsetId) return;
    try {
      setLoading(true);
      const [vsData, varsData, assignData, pl, wr] = await Promise.all([
        variableSetService.get(varsetId),
        variableSetService.listVariables(varsetId),
        variableSetService.listAssignments(varsetId),
        getProjects().catch(() => []),
        workspaceService.getWorkspaces().catch(() => ({})),
      ]);
      setVarset(vsData);
      setVariables(Array.isArray(varsData) ? varsData : []);
      setAssignments(assignData.items || []);
      setProjects(pl || []);
      const raw = wr as any;
      const items = raw?.data?.items || raw?.items || raw?.data || raw || [];
      setWorkspaces(Array.isArray(items) ? items : []);
    } catch (e: any) { showToast(e.response?.data?.error || 'Load failed', 'error'); }
    finally { setLoading(false); }
  }, [varsetId, showToast]);

  useEffect(() => { loadData(); }, [loadData]);

  // --- Info ---
  const startEdit = () => { if (varset) { setInfoForm({ name: varset.name, description: varset.description || '' }); setEditingInfo(true); } };
  const saveInfo = async () => {
    if (!varsetId) return;
    try { await variableSetService.update(varsetId, infoForm); showToast('Updated', 'success'); setEditingInfo(false); loadData(); }
    catch (e: any) { showToast(e.response?.data?.error || 'Failed', 'error'); }
  };

  // --- Variables ---
  const resetVarForm = () => setVarForm({ key: '', value: '', description: '', variable_type: 'terraform', value_format: 'string', sensitive: false });
  const handleAddVar = () => { setEditingVar(null); resetVarForm(); setShowVarInput(true); };
  const handleEditVar = (v: VarsetVariable) => {
    setEditingVar(v);
    setVarForm({ key: v.key, value: v.sensitive ? '' : v.value, description: v.description, variable_type: v.variable_type, value_format: v.value_format, sensitive: v.sensitive });
    setShowVarInput(true);
  };
  const submitVar = async () => {
    if (!varsetId) return;
    if (!editingVar && !varForm.key.trim()) { showToast('Key required', 'error'); return; }
    try {
      if (editingVar) {
        await variableSetService.updateVariable(varsetId, editingVar.variable_id, { value: varForm.value, description: varForm.description, sensitive: varForm.sensitive });
        showToast('Variable updated', 'success');
      } else {
        await variableSetService.createVariable(varsetId, varForm);
        showToast('Variable added', 'success');
      }
      setShowVarInput(false); setEditingVar(null); resetVarForm(); loadData();
    } catch (e: any) { showToast(e.response?.data?.error || 'Failed', 'error'); }
  };
  const confirmDeleteVar = async () => {
    if (!varsetId || !varToDelete) return;
    try { await variableSetService.deleteVariable(varsetId, varToDelete.variable_id); showToast('Deleted', 'success'); setDeleteVarDialog(false); setVarToDelete(null); loadData(); }
    catch (e: any) { showToast(e.response?.data?.error || 'Failed', 'error'); }
  };

  // --- Assignments ---
  const addAssignment = async (scopeType: 'project' | 'workspace', id: number | string) => {
    if (!varsetId) return;
    try {
      const data: any = { scope_type: scopeType };
      if (scopeType === 'project') data.project_id = id;
      else data.workspace_id = id;
      await variableSetService.createAssignment(varsetId, data);
      showToast('Assignment added', 'success'); loadData();
    } catch (e: any) { showToast(e.response?.data?.error || 'Failed', 'error'); }
  };
  const confirmDeleteAssign = async () => {
    if (!varsetId || !assignToDelete) return;
    try { await variableSetService.deleteAssignment(varsetId, assignToDelete.id); showToast('Removed', 'success'); setDeleteAssignDialog(false); setAssignToDelete(null); loadData(); }
    catch (e: any) { showToast(e.response?.data?.error || 'Failed', 'error'); }
  };

  // --- Styles ---
  const sectionTitle: React.CSSProperties = { fontSize: '18px', fontWeight: 700, color: '#1a1a1a', margin: '0 0 8px 0' };
  const subText: React.CSSProperties = { fontSize: '13px', color: '#6b7280', lineHeight: '1.5' };
  const inputStyle: React.CSSProperties = { width: '100%', padding: '10px 12px', border: '1px solid #d1d5db', borderRadius: '6px', fontSize: '14px', boxSizing: 'border-box' as const };
  const fieldLabel: React.CSSProperties = { fontSize: '15px', fontWeight: 600, color: '#1a1a1a', display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '6px' };
  const cardBox: React.CSSProperties = { border: '1px solid #e5e7eb', borderRadius: '8px', padding: '20px', marginTop: '12px' };
  const tagStyle: React.CSSProperties = { display: 'inline-flex', alignItems: 'center', gap: '4px', padding: '4px 10px', background: '#f3f4f6', borderRadius: '4px', fontSize: '13px' };
  const outerStyle: React.CSSProperties = { margin: '12px', background: '#f8fafc', border: '1px solid #e5e7eb', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.05)' };

  if (loading && !varset) return <div className={styles.container} style={outerStyle}><div className={styles.loading}>Loading...</div></div>;
  if (!varset) return <div className={styles.container} style={outerStyle}><div className={styles.empty}><div className={styles.emptyText}>Variable set not found</div></div></div>;

  const assignedProjectIds = assignments.filter(a => a.scope_type === 'project').map(a => a.project_id!);
  const assignedWorkspaceIds = assignments.filter(a => a.scope_type === 'workspace').map(a => a.workspace_id!);

  return (
    <div className={styles.container} style={outerStyle}>
      {/* Back link */}
      <div style={{ marginBottom: '16px' }}>
        <span style={{ color: '#3b82f6', cursor: 'pointer', fontSize: '14px' }} onClick={() => navigate('/variable-sets')}>
          &larr; Back to Variable Sets
        </span>
      </div>

      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: '8px', padding: '32px' }}>

        {/* Name + Description */}
        {editingInfo ? (
          <div style={{ marginBottom: '32px' }}>
            <div style={{ marginBottom: '16px' }}>
              <div style={fieldLabel}>Name</div>
              <input style={inputStyle} value={infoForm.name} onChange={e => setInfoForm({ ...infoForm, name: e.target.value })} />
            </div>
            <div style={{ marginBottom: '16px' }}>
              <div style={fieldLabel}>Description</div>
              <textarea style={{ ...inputStyle, minHeight: '80px', resize: 'vertical' as const }} value={infoForm.description} onChange={e => setInfoForm({ ...infoForm, description: e.target.value })} />
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <button style={{ padding: '8px 18px', border: 'none', background: '#3b82f6', color: '#fff', borderRadius: '6px', fontSize: '14px', fontWeight: 500, cursor: 'pointer' }} onClick={saveInfo}>Save</button>
              <button style={{ padding: '8px 18px', border: '1px solid #d1d5db', background: '#fff', borderRadius: '6px', fontSize: '14px', cursor: 'pointer' }} onClick={() => setEditingInfo(false)}>Cancel</button>
            </div>
          </div>
        ) : (
          <div style={{ marginBottom: '32px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '4px' }}>
                  <h1 style={{ fontSize: '24px', fontWeight: 700, color: '#1a1a1a', margin: 0 }}>{varset.name}</h1>
                  {varset.scope === 'global'
                    ? <span style={{ fontSize: '12px', padding: '3px 10px', background: '#EFF6FF', color: '#2563EB', border: '1px solid #93C5FD', borderRadius: '12px', fontWeight: 500 }}>Global</span>
                    : <span style={{ fontSize: '12px', padding: '3px 10px', background: '#F0FDF4', color: '#16A34A', border: '1px solid #86EFAC', borderRadius: '12px', fontWeight: 500 }}>Specific</span>}
                </div>
                {varset.description && <p style={{ ...subText, margin: '4px 0 0 0' }}>{varset.description}</p>}
              </div>
              <button style={{ padding: '8px 20px', background: '#fff', color: '#374151', border: '1px solid #d1d5db', borderRadius: '6px', fontWeight: 500, fontSize: '14px', cursor: 'pointer' }} onClick={startEdit}>Edit</button>
            </div>
          </div>
        )}

        {/* Variable set scope */}
        <div style={{ marginBottom: '32px' }}>
          <h2 style={sectionTitle}>Variable set scope</h2>
          {varset.scope === 'global' ? (
            <div style={{ padding: '12px 16px', background: '#EFF6FF', border: '1px solid #93C5FD', borderRadius: '6px', color: '#1E40AF', fontSize: '14px', lineHeight: '1.6' }}>
              Apply to all projects and workspaces. All current and future workspaces in this organization will access this variable set.
            </div>
          ) : (
            <div style={cardBox}>
              {/* Projects */}
              <div style={{ marginBottom: '20px' }}>
                <div style={{ fontWeight: 600, fontSize: '14px', color: '#1a1a1a', marginBottom: '4px' }}>Apply to projects</div>
                <div style={{ ...subText, marginBottom: '8px' }}>All current and future workspaces in the selected projects will access this variable set.</div>
                <select style={inputStyle} value="" onChange={e => { const pid = Number(e.target.value); if (pid) addAssignment('project', pid); }}>
                  <option value="">Select projects</option>
                  {projects.filter(p => !assignedProjectIds.includes(p.id)).map(p => <option key={p.id} value={p.id}>{p.display_name || p.name}</option>)}
                </select>
                {assignedProjectIds.length > 0 && (
                  <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginTop: '8px' }}>
                    {assignments.filter(a => a.scope_type === 'project').map(a => {
                      const p = projects.find(x => x.id === a.project_id);
                      return (
                        <span key={a.id} style={tagStyle}>
                          {p?.display_name || p?.name || `Project #${a.project_id}`}
                          <span style={{ cursor: 'pointer', color: '#9ca3af', fontWeight: 700 }} onClick={() => { setAssignToDelete(a); setDeleteAssignDialog(true); }}>x</span>
                        </span>
                      );
                    })}
                  </div>
                )}
              </div>

              {/* Workspaces */}
              <div>
                <div style={{ fontWeight: 600, fontSize: '14px', color: '#1a1a1a', marginBottom: '4px' }}>Apply to workspaces</div>
                <div style={{ ...subText, marginBottom: '8px' }}>Only the selected workspaces will access this variable set.</div>
                <select style={inputStyle} value="" onChange={e => { const wid = e.target.value; if (wid) addAssignment('workspace', wid); }}>
                  <option value="">Select workspaces</option>
                  {workspaces.filter(w => !assignedWorkspaceIds.includes(w.workspace_id || '')).map(w => <option key={w.workspace_id || w.id} value={w.workspace_id || ''}>{w.name}</option>)}
                </select>
                {assignedWorkspaceIds.length > 0 && (
                  <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginTop: '8px' }}>
                    {assignments.filter(a => a.scope_type === 'workspace').map(a => {
                      const w = workspaces.find(x => x.workspace_id === a.workspace_id);
                      return (
                        <span key={a.id} style={tagStyle}>
                          {w?.name || a.workspace_id}
                          <span style={{ cursor: 'pointer', color: '#9ca3af', fontWeight: 700 }} onClick={() => { setAssignToDelete(a); setDeleteAssignDialog(true); }}>x</span>
                        </span>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        {/* Variables */}
        <div style={{ marginBottom: '32px' }}>
          <h2 style={sectionTitle}>Variables</h2>
          <p style={{ ...subText, marginBottom: '16px' }}>
            You can add any number of Terraform and Environment variables. Terraform will use these variables for all plan and apply operations.
          </p>

          {/* Variable table */}
          {variables.length > 0 && (
            <div style={{ border: '1px solid #e5e7eb', borderRadius: '8px', overflow: 'hidden', marginBottom: '12px' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ background: '#f9fafb' }}>
                    <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: '11px', fontWeight: 600, color: '#6b7280', textTransform: 'uppercase' as const, letterSpacing: '0.05em' }}>Key</th>
                    <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: '11px', fontWeight: 600, color: '#6b7280', textTransform: 'uppercase' as const, letterSpacing: '0.05em' }}>Value</th>
                    <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: '11px', fontWeight: 600, color: '#6b7280', textTransform: 'uppercase' as const, letterSpacing: '0.05em' }}>Category</th>
                    <th style={{ width: '120px' }}></th>
                  </tr>
                </thead>
                <tbody>
                  {variables.map(v => (
                    <tr key={v.id} style={{ borderTop: '1px solid #e5e7eb' }}>
                      <td style={{ padding: '12px 16px' }}>
                        <span style={{ fontWeight: 500, fontSize: '14px' }}>{v.key}</span>
                        {v.sensitive && <span style={{ marginLeft: '8px', fontSize: '11px', padding: '2px 8px', background: '#FEF3C7', color: '#92400E', borderRadius: '4px', fontWeight: 600 }}>SENSITIVE</span>}
                      </td>
                      <td style={{ padding: '12px 16px', color: '#6b7280', fontSize: '14px' }}>
                        {v.sensitive ? '-- sensitive value --' : (v.value || <span style={{ color: '#d1d5db' }}>(empty)</span>)}
                      </td>
                      <td style={{ padding: '12px 16px', fontSize: '14px', color: '#374151' }}>
                        {v.variable_type === 'terraform' ? 'Terraform' : 'Environment'}
                      </td>
                      <td style={{ padding: '12px 8px' }}>
                        <div style={{ display: 'flex', gap: '6px' }}>
                          <button style={{ padding: '3px 10px', border: '1px solid #d1d5db', background: '#fff', borderRadius: '4px', fontSize: '12px', cursor: 'pointer' }} onClick={() => handleEditVar(v)}>Edit</button>
                          <button style={{ padding: '3px 10px', border: '1px solid #fca5a5', background: '#fff', color: '#ef4444', borderRadius: '4px', fontSize: '12px', cursor: 'pointer' }} onClick={() => { setVarToDelete(v); setDeleteVarDialog(true); }}>Delete</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Add/Edit variable form */}
          {showVarInput ? (
            <div style={cardBox}>
              <div style={{ fontSize: '15px', fontWeight: 600, color: '#1a1a1a', marginBottom: '16px' }}>
                {editingVar ? `Edit variable: ${editingVar.key}` : 'Add new variable'}
              </div>

              {/* Type radio */}
              {!editingVar && (
                <div style={{ marginBottom: '20px' }}>
                  <label style={{ display: 'flex', gap: '10px', cursor: 'pointer', marginBottom: '12px' }}>
                    <input type="radio" checked={varForm.variable_type === 'terraform'} onChange={() => setVarForm({ ...varForm, variable_type: 'terraform' })} style={{ marginTop: '3px' }} />
                    <div>
                      <div style={{ fontWeight: 600, fontSize: '14px' }}>Terraform variable</div>
                      <div style={subText}>These variables should match the declarations in your configuration. Click the HCL box to use interpolation or set a non-string value.</div>
                    </div>
                  </label>
                  <label style={{ display: 'flex', gap: '10px', cursor: 'pointer' }}>
                    <input type="radio" checked={varForm.variable_type === 'environment'} onChange={() => setVarForm({ ...varForm, variable_type: 'environment' })} style={{ marginTop: '3px' }} />
                    <div>
                      <div style={{ fontWeight: 600, fontSize: '14px' }}>Environment variable</div>
                      <div style={subText}>These variables are available in the Terraform runtime environment.</div>
                    </div>
                  </label>
                </div>
              )}

              {editingVar && (
                <div style={{ marginBottom: '12px', padding: '8px 12px', background: '#f9fafb', borderRadius: '6px', fontSize: '13px', color: '#6b7280' }}>
                  Type: <strong>{varForm.variable_type === 'terraform' ? 'Terraform' : 'Environment'}</strong> | Format: <strong>{varForm.value_format}</strong> (immutable)
                </div>
              )}

              {/* Key + Value + HCL + Sensitive */}
              <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-end', marginBottom: '16px' }}>
                <div style={{ flex: 1 }}>
                  <label style={{ fontSize: '13px', fontWeight: 600, color: '#374151', marginBottom: '4px', display: 'block' }}>Key</label>
                  {editingVar ? (
                    <div style={{ padding: '10px 12px', background: '#f9fafb', border: '1px solid #e5e7eb', borderRadius: '6px', fontSize: '14px', fontFamily: 'var(--font-mono)' }}>{varForm.key}</div>
                  ) : (
                    <input style={inputStyle} value={varForm.key} onChange={e => setVarForm({ ...varForm, key: e.target.value })} placeholder="key" />
                  )}
                </div>
                <div style={{ flex: 1.5 }}>
                  <label style={{ fontSize: '13px', fontWeight: 600, color: '#374151', marginBottom: '4px', display: 'block' }}>Value</label>
                  <input style={inputStyle} type={varForm.sensitive ? 'password' : 'text'} value={varForm.value} onChange={e => setVarForm({ ...varForm, value: e.target.value })} placeholder={editingVar?.sensitive ? 'Enter new value' : 'value'} />
                </div>
                {!editingVar && (
                  <>
                    <label style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '13px', cursor: 'pointer', whiteSpace: 'nowrap', paddingBottom: '10px' }}>
                      <input type="checkbox" checked={varForm.value_format === 'hcl'} onChange={e => setVarForm({ ...varForm, value_format: e.target.checked ? 'hcl' : 'string' })} /> HCL
                    </label>
                    <label style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '13px', cursor: 'pointer', whiteSpace: 'nowrap', paddingBottom: '10px' }}>
                      <input type="checkbox" checked={varForm.sensitive} onChange={e => setVarForm({ ...varForm, sensitive: e.target.checked })} /> Sensitive
                    </label>
                  </>
                )}
                {editingVar && !editingVar.sensitive && (
                  <label style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '13px', cursor: 'pointer', whiteSpace: 'nowrap', paddingBottom: '10px' }}>
                    <input type="checkbox" checked={varForm.sensitive} onChange={e => setVarForm({ ...varForm, sensitive: e.target.checked })} /> Sensitive
                  </label>
                )}
              </div>

              {/* Description */}
              <div style={{ marginBottom: '16px' }}>
                <label style={{ fontSize: '13px', fontWeight: 600, color: '#374151', marginBottom: '4px', display: 'block' }}>Description (Optional)</label>
                <textarea style={{ ...inputStyle, minHeight: '60px', resize: 'vertical' as const }} value={varForm.description} onChange={e => setVarForm({ ...varForm, description: e.target.value })} placeholder="description (optional)" />
              </div>

              <div style={{ display: 'flex', gap: '8px' }}>
                <button style={{ padding: '8px 18px', border: 'none', background: '#3b82f6', color: '#fff', borderRadius: '6px', fontSize: '14px', fontWeight: 500, cursor: 'pointer' }} onClick={submitVar}>
                  {editingVar ? 'Update variable' : 'Add variable'}
                </button>
                <button style={{ padding: '8px 18px', border: '1px solid #d1d5db', background: '#fff', borderRadius: '6px', fontSize: '14px', cursor: 'pointer' }} onClick={() => { setShowVarInput(false); setEditingVar(null); }}>Cancel</button>
              </div>
            </div>
          ) : (
            <button style={{ padding: '10px 20px', border: '1px solid #d1d5db', background: '#fff', borderRadius: '6px', fontSize: '14px', fontWeight: 500, cursor: 'pointer', color: '#374151' }} onClick={handleAddVar}>
              + Add variable
            </button>
          )}
        </div>
      </div>

      {/* Dialogs */}
      <ConfirmDialog isOpen={deleteVarDialog} title={`Delete "${varToDelete?.key}"`} confirmText="Confirm" cancelText="Cancel" type="danger" onConfirm={confirmDeleteVar} onCancel={() => { setDeleteVarDialog(false); setVarToDelete(null); }}>
        <p style={{ margin: 0, color: '#374151', fontSize: '14px' }}>Permanently delete <strong>{varToDelete?.key}</strong>. This cannot be undone.</p>
      </ConfirmDialog>
      <ConfirmDialog isOpen={deleteAssignDialog} title="Remove assignment" confirmText="Confirm" cancelText="Cancel" type="danger" onConfirm={confirmDeleteAssign} onCancel={() => { setDeleteAssignDialog(false); setAssignToDelete(null); }}>
        <p style={{ margin: 0, color: '#374151', fontSize: '14px' }}>Remove this {assignToDelete?.scope_type} assignment?</p>
      </ConfirmDialog>
    </div>
  );
};

export default VariableSetDetail;
