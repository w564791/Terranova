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

const VariableSetsPage: React.FC = () => {
  const [varsets, setVarsets] = useState<VariableSet[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<VariableSet | null>(null);
  const { showToast } = useToast();
  const navigate = useNavigate();

  // Form state
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [scope, setScope] = useState<'global' | 'specific'>('specific');
  const [selectedProjects, setSelectedProjects] = useState<number[]>([]);
  const [selectedWorkspaces, setSelectedWorkspaces] = useState<string[]>([]);
  const [pendingVars, setPendingVars] = useState<PendingVar[]>([]);
  const [showVarInput, setShowVarInput] = useState(false);
  const [varForm, setVarForm] = useState<PendingVar>({ key: '', value: '', description: '', variable_type: 'terraform', value_format: 'string', sensitive: false });

  // Picker data
  const [projects, setProjects] = useState<Project[]>([]);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);

  const loadVarsets = async () => {
    try { setLoading(true); const r = await variableSetService.list(); setVarsets(r.items || []); }
    catch (e: any) { showToast(e.response?.data?.error || 'Load failed', 'error'); }
    finally { setLoading(false); }
  };

  const loadPickerData = async () => {
    const [pl, wr] = await Promise.all([getProjects().catch(() => []), workspaceService.getWorkspaces().catch(() => ({}))]);
    setProjects(pl || []);
    const raw = wr as any;
    const items = raw?.data?.items || raw?.items || raw?.data || raw || [];
    setWorkspaces(Array.isArray(items) ? items : []);
  };

  useEffect(() => { loadVarsets(); }, []);

  const resetForm = () => {
    setName(''); setDescription(''); setScope('specific');
    setSelectedProjects([]); setSelectedWorkspaces([]);
    setPendingVars([]); setShowVarInput(false);
    setVarForm({ key: '', value: '', description: '', variable_type: 'terraform', value_format: 'string', sensitive: false });
  };

  const handleAdd = () => { resetForm(); setShowForm(true); loadPickerData(); };
  const handleCancel = () => { setShowForm(false); resetForm(); };

  // Add variable to pending list
  const addVar = () => {
    if (!varForm.key.trim()) { showToast('Key is required', 'error'); return; }
    if (pendingVars.some(v => v.key === varForm.key.trim())) { showToast(`"${varForm.key}" already added`, 'error'); return; }
    setPendingVars([...pendingVars, { ...varForm, key: varForm.key.trim() }]);
    setVarForm({ key: '', value: '', description: '', variable_type: varForm.variable_type, value_format: 'string', sensitive: false });
    setShowVarInput(false);
  };

  // Submit: create varset + variables + assignments
  const handleSubmit = async () => {
    if (!name.trim()) { showToast('Name is required', 'error'); return; }
    setSubmitting(true);
    try {
      const created = await variableSetService.create({ name: name.trim(), description, scope });
      const vid = created.varset_id;
      for (const v of pendingVars) {
        await variableSetService.createVariable(vid, v);
      }
      if (scope === 'specific') {
        for (const pid of selectedProjects) {
          await variableSetService.createAssignment(vid, { scope_type: 'project', project_id: pid });
        }
        for (const wsid of selectedWorkspaces) {
          await variableSetService.createAssignment(vid, { scope_type: 'workspace', workspace_id: wsid });
        }
      }
      showToast('Variable set created', 'success');
      setShowForm(false); resetForm(); loadVarsets();
    } catch (e: any) { showToast(e.response?.data?.error || e || 'Failed', 'error'); }
    finally { setSubmitting(false); }
  };

  const confirmDelete = async () => {
    if (!deleteConfirm) return;
    try { await variableSetService.delete(deleteConfirm.varset_id); showToast('Deleted', 'success'); setDeleteConfirm(null); loadVarsets(); }
    catch (e: any) { showToast(e.response?.data?.error || 'Failed', 'error'); }
  };

  const scopeBadge = (s: string) => s === 'global'
    ? <span className={styles.statusBadge} style={{ backgroundColor: 'var(--brand-soft)', color: 'var(--brand-ink)', border: '1px solid var(--brand-300)' }}>Global</span>
    : <span className={styles.statusBadge} style={{ backgroundColor: 'var(--green-soft)', color: 'var(--green-hover)', border: '1px solid var(--green-line)' }}>Specific</span>;

  const fmtDate = (d: string) => new Date(d).toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' });

  const btnDel: React.CSSProperties = { padding: '5px 12px', border: '1px solid var(--color-red-200)', background: 'var(--surface)', color: 'var(--red)', borderRadius: '6px', fontSize: '13px', fontWeight: 500, cursor: 'pointer' };
  const sectionTitle: React.CSSProperties = { fontSize: '18px', fontWeight: 700, color: 'var(--ink)', margin: '0 0 12px 0' };
  const subText: React.CSSProperties = { fontSize: '13px', color: 'var(--ink-2)', lineHeight: '1.5' };
  const fieldLabel: React.CSSProperties = { fontSize: '15px', fontWeight: 600, color: 'var(--ink)', display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '6px' };
  const inputStyle: React.CSSProperties = { width: '100%', padding: '10px 12px', border: '1px solid var(--line-2)', borderRadius: '6px', fontSize: '14px', boxSizing: 'border-box' as const };
  const cardBox: React.CSSProperties = { border: '1px solid var(--line)', borderRadius: '8px', padding: '20px', marginTop: '12px' };

  return (
    <div className={styles.container} style={{ margin: '12px', background: 'var(--bg)', border: '1px solid var(--line)', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.05)' }}>
      <div className={styles.header}>
        <h1 className={styles.title}>Variable Sets</h1>
        <p className={styles.description}>Variable sets let you reuse the same variables across multiple workspaces.</p>
      </div>

      <div className={styles.actions}>
        <div></div>
        {!showForm && <button className={styles.addButton} onClick={handleAdd}>+ Create variable set</button>}
      </div>

      {/* ====== Create Form ====== */}
      {showForm && (
        <div style={{ background: 'var(--surface)', border: '1px solid var(--line)', borderRadius: '8px', padding: '32px', marginBottom: '24px' }}>

          {/* Name */}
          <div style={{ marginBottom: '24px' }}>
            <div style={fieldLabel}>Name <span style={{ fontSize: '11px', padding: '2px 8px', background: 'var(--surface-2)', color: 'var(--ink-2)', borderRadius: '4px', fontWeight: 500 }}>Required</span></div>
            <input style={inputStyle} value={name} onChange={e => setName(e.target.value)} placeholder="" />
          </div>

          {/* Description */}
          <div style={{ marginBottom: '32px' }}>
            <div style={fieldLabel}>Description</div>
            <textarea style={{ ...inputStyle, minHeight: '100px', resize: 'vertical' as const }} value={description} onChange={e => setDescription(e.target.value)} />
          </div>

          {/* Variable set scope */}
          <div style={{ marginBottom: '32px' }}>
            <h2 style={sectionTitle}>Variable set scope</h2>

            <label style={{ display: 'flex', gap: '12px', cursor: 'pointer', marginBottom: '16px' }}>
              <input type="radio" name="scope" checked={scope === 'global'} onChange={() => setScope('global')} style={{ marginTop: '3px' }} />
              <div>
                <div style={{ fontSize: '15px', fontWeight: 500, color: 'var(--ink)' }}>Apply to all projects and workspaces</div>
                <div style={subText}>All current and future workspaces in this organization will access this variable set.</div>
              </div>
            </label>

            <label style={{ display: 'flex', gap: '12px', cursor: 'pointer' }}>
              <input type="radio" name="scope" checked={scope === 'specific'} onChange={() => setScope('specific')} style={{ marginTop: '3px' }} />
              <div style={{ fontSize: '15px', fontWeight: 500, color: 'var(--ink)' }}>Apply to specific projects and workspaces</div>
            </label>

            {scope === 'specific' && (
              <div style={cardBox}>
                {/* Apply to projects */}
                <div style={{ marginBottom: '20px' }}>
                  <div style={{ fontWeight: 600, fontSize: '14px', color: 'var(--ink)', marginBottom: '4px' }}>Apply to projects</div>
                  <div style={{ ...subText, marginBottom: '8px' }}>All current and future workspaces in the selected projects will access this variable set.</div>
                  <select
                    style={inputStyle}
                    value=""
                    onChange={e => {
                      const pid = Number(e.target.value);
                      if (pid && !selectedProjects.includes(pid)) setSelectedProjects([...selectedProjects, pid]);
                    }}
                  >
                    <option value="">Select projects</option>
                    {projects.filter(p => !selectedProjects.includes(p.id)).map(p => (
                      <option key={p.id} value={p.id}>{p.display_name || p.name}</option>
                    ))}
                  </select>
                  {selectedProjects.length > 0 && (
                    <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginTop: '8px' }}>
                      {selectedProjects.map(pid => {
                        const p = projects.find(x => x.id === pid);
                        return (
                          <span key={pid} style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', padding: '4px 10px', background: 'var(--surface-2)', borderRadius: '4px', fontSize: '13px' }}>
                            {p?.display_name || p?.name || `#${pid}`}
                            <span style={{ cursor: 'pointer', color: 'var(--ink-faint)', fontWeight: 700 }} onClick={() => setSelectedProjects(selectedProjects.filter(x => x !== pid))}>x</span>
                          </span>
                        );
                      })}
                    </div>
                  )}
                </div>

                {/* Apply to workspaces */}
                <div>
                  <div style={{ fontWeight: 600, fontSize: '14px', color: 'var(--ink)', marginBottom: '4px' }}>Apply to workspaces</div>
                  <div style={{ ...subText, marginBottom: '8px' }}>Only the selected workspaces will access this variable set.</div>
                  <select
                    style={inputStyle}
                    value=""
                    onChange={e => {
                      const wid = e.target.value;
                      if (wid && !selectedWorkspaces.includes(wid)) setSelectedWorkspaces([...selectedWorkspaces, wid]);
                    }}
                  >
                    <option value="">Select workspaces</option>
                    {workspaces.filter(w => !selectedWorkspaces.includes(w.workspace_id || '')).map(w => (
                      <option key={w.workspace_id || w.id} value={w.workspace_id || ''}>{w.name}</option>
                    ))}
                  </select>
                  {selectedWorkspaces.length > 0 && (
                    <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginTop: '8px' }}>
                      {selectedWorkspaces.map(wid => {
                        const w = workspaces.find(x => x.workspace_id === wid);
                        return (
                          <span key={wid} style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', padding: '4px 10px', background: 'var(--surface-2)', borderRadius: '4px', fontSize: '13px' }}>
                            {w?.name || wid}
                            <span style={{ cursor: 'pointer', color: 'var(--ink-faint)', fontWeight: 700 }} onClick={() => setSelectedWorkspaces(selectedWorkspaces.filter(x => x !== wid))}>x</span>
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

            {/* Pending vars table */}
            {pendingVars.length > 0 && (
              <div style={{ border: '1px solid var(--line)', borderRadius: '8px', overflow: 'hidden', marginBottom: '12px' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ background: 'var(--bg)' }}>
                      <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: '11px', fontWeight: 600, color: 'var(--ink-2)', textTransform: 'uppercase' as const, letterSpacing: '0.05em' }}>Key</th>
                      <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: '11px', fontWeight: 600, color: 'var(--ink-2)', textTransform: 'uppercase' as const, letterSpacing: '0.05em' }}>Value</th>
                      <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: '11px', fontWeight: 600, color: 'var(--ink-2)', textTransform: 'uppercase' as const, letterSpacing: '0.05em' }}>Category</th>
                      <th style={{ width: '40px' }}></th>
                    </tr>
                  </thead>
                  <tbody>
                    {pendingVars.map((v, i) => (
                      <tr key={i} style={{ borderTop: '1px solid var(--line)' }}>
                        <td style={{ padding: '12px 16px' }}>
                          <span style={{ fontWeight: 500, fontSize: '14px' }}>{v.key}</span>
                          {v.sensitive && <span style={{ marginLeft: '8px', fontSize: '11px', padding: '2px 8px', background: 'var(--amber-soft)', color: 'var(--amber-hover)', borderRadius: '4px', fontWeight: 600 }}>SENSITIVE</span>}
                        </td>
                        <td style={{ padding: '12px 16px', color: 'var(--ink-2)', fontSize: '14px' }}>
                          {v.sensitive ? '-- sensitive value --' : (v.value || <span style={{ color: 'var(--line-2)' }}>(empty)</span>)}
                        </td>
                        <td style={{ padding: '12px 16px', fontSize: '14px', color: 'var(--ink)' }}>
                          {v.variable_type === 'terraform' ? 'Terraform' : 'Environment'}
                        </td>
                        <td style={{ padding: '12px 8px', textAlign: 'center' }}>
                          <span style={{ cursor: 'pointer', color: 'var(--ink-faint)', fontSize: '16px' }} onClick={() => setPendingVars(pendingVars.filter((_, j) => j !== i))}>x</span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {/* Inline add variable */}
            {showVarInput ? (
              <div style={{ ...cardBox, marginTop: '0' }}>
                <div style={{ fontSize: '15px', fontWeight: 600, color: 'var(--ink)', marginBottom: '16px' }}>Add new variable</div>

                {/* Type radio */}
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

                {/* Key + Value + HCL + Sensitive on one row */}
                <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-end', marginBottom: '16px' }}>
                  <div style={{ flex: 1 }}>
                    <label style={{ fontSize: '13px', fontWeight: 600, color: 'var(--ink)', marginBottom: '4px', display: 'block' }}>Key</label>
                    <input style={inputStyle} value={varForm.key} onChange={e => setVarForm({ ...varForm, key: e.target.value })} placeholder="key" />
                  </div>
                  <div style={{ flex: 1.5 }}>
                    <label style={{ fontSize: '13px', fontWeight: 600, color: 'var(--ink)', marginBottom: '4px', display: 'block' }}>Value</label>
                    <input style={inputStyle} type={varForm.sensitive ? 'password' : 'text'} value={varForm.value} onChange={e => setVarForm({ ...varForm, value: e.target.value })} placeholder="value" />
                  </div>
                  <label style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '13px', cursor: 'pointer', whiteSpace: 'nowrap', paddingBottom: '10px' }}>
                    <input type="checkbox" checked={varForm.value_format === 'hcl'} onChange={e => setVarForm({ ...varForm, value_format: e.target.checked ? 'hcl' : 'string' })} /> HCL
                  </label>
                  <label style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '13px', cursor: 'pointer', whiteSpace: 'nowrap', paddingBottom: '10px' }}>
                    <input type="checkbox" checked={varForm.sensitive} onChange={e => setVarForm({ ...varForm, sensitive: e.target.checked })} /> Sensitive
                  </label>
                </div>

                {/* Description */}
                <div style={{ marginBottom: '16px' }}>
                  <label style={{ fontSize: '13px', fontWeight: 600, color: 'var(--ink)', marginBottom: '4px', display: 'block' }}>Description (Optional)</label>
                  <textarea style={{ ...inputStyle, minHeight: '60px', resize: 'vertical' as const }} value={varForm.description} onChange={e => setVarForm({ ...varForm, description: e.target.value })} placeholder="description (optional)" />
                </div>

                {/* Buttons */}
                <div style={{ display: 'flex', gap: '8px' }}>
                  <button style={{ padding: '8px 18px', border: 'none', background: 'var(--brand)', color: 'var(--surface)', borderRadius: '6px', fontSize: '14px', fontWeight: 500, cursor: 'pointer' }} onClick={addVar}>Add variable</button>
                  <button style={{ padding: '8px 18px', border: '1px solid var(--line-2)', background: 'var(--surface)', borderRadius: '6px', fontSize: '14px', cursor: 'pointer' }} onClick={() => setShowVarInput(false)}>Cancel</button>
                </div>
              </div>
            ) : (
              <button
                style={{ padding: '10px 20px', border: '1px solid var(--line-2)', background: 'var(--surface)', borderRadius: '6px', fontSize: '14px', fontWeight: 500, cursor: 'pointer', color: 'var(--ink)' }}
                onClick={() => setShowVarInput(true)}
              >
                + Add variable
              </button>
            )}
          </div>

          {/* Submit */}
          <div style={{ display: 'flex', gap: '12px' }}>
            <button
              style={{ padding: '12px 24px', border: 'none', background: 'var(--brand)', color: 'var(--surface)', borderRadius: '6px', fontSize: '15px', fontWeight: 600, cursor: submitting ? 'not-allowed' : 'pointer', opacity: submitting ? 0.7 : 1 }}
              onClick={handleSubmit}
              disabled={submitting}
            >
              {submitting ? 'Creating...' : 'Create variable set'}
            </button>
            <button
              style={{ padding: '12px 24px', border: '1px solid var(--line-2)', background: 'var(--surface)', color: 'var(--ink)', borderRadius: '6px', fontSize: '15px', fontWeight: 500, cursor: 'pointer' }}
              onClick={handleCancel}
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* ====== List ====== */}
      <div className={styles.versionsList} style={{ border: '1px solid var(--line)' }}>
        {loading ? (
          <div className={styles.loading}>Loading...</div>
        ) : varsets.length === 0 && !showForm ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>No variable sets</div>
            <div className={styles.emptyHint}>Click "Create variable set" to get started</div>
          </div>
        ) : varsets.length > 0 && (
          <table className={styles.versionsTable}>
            <thead><tr><th>Name</th><th>Scope</th><th>Variables</th><th>Assignments</th><th>Created</th><th>Actions</th></tr></thead>
            <tbody>
              {varsets.map(vs => (
                <tr key={vs.varset_id} style={{ cursor: 'pointer' }} onClick={() => navigate(`/variable-sets/${vs.varset_id}`)}>
                  <td><span style={{ fontWeight: 500, color: 'var(--brand-ink)' }}>{vs.name}</span></td>
                  <td>{scopeBadge(vs.scope)}</td>
                  <td>{vs.variable_count ?? 0}</td>
                  <td>{vs.scope === 'global' ? 'All' : (vs.assignment_count ?? 0)}</td>
                  <td><span style={{ color: 'var(--ink-faint)', fontSize: '13px' }}>{fmtDate(vs.created_at)}</span></td>
                  <td><button style={btnDel} onClick={e => { e.stopPropagation(); setDeleteConfirm(vs); }}>Delete</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <ConfirmDialog isOpen={!!deleteConfirm} title={`Delete "${deleteConfirm?.name}"`} confirmText="Confirm" cancelText="Cancel" type="danger" onConfirm={confirmDelete} onCancel={() => setDeleteConfirm(null)}>
        <p style={{ margin: 0, color: 'var(--ink)', fontSize: '14px' }}>This will permanently remove <strong>{deleteConfirm?.name}</strong> and all its variables and assignments.</p>
      </ConfirmDialog>
    </div>
  );
};

export default VariableSetsPage;
