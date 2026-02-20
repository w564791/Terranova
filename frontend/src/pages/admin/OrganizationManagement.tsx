import React, { useState, useEffect } from 'react';
import { useToast } from '../../hooks/useToast';
import { iamService } from '../../services/iam';
import type { Organization, CreateOrganizationRequest, UpdateOrganizationRequest } from '../../services/iam';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './OrganizationManagement.module.css';

const OrganizationManagement: React.FC = () => {
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [editingOrg, setEditingOrg] = useState<Organization | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all');
  const [deleteConfirm, setDeleteConfirm] = useState<{
    show: boolean;
    org: Organization | null;
  }>({ show: false, org: null });
  const { showToast } = useToast();

  // Form state
  const [formData, setFormData] = useState({
    name: '',
    display_name: '',
    description: '',
    is_active: true,
  });

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  // Load organizations list
  const loadOrganizations = async () => {
    try {
      setLoading(true);
      const response = await iamService.listOrganizations();
      console.log('Organizations list response:', response);
      setOrganizations(response?.organizations || []);
    } catch (error: any) {
      console.error('Failed to load organizations:', error);
      showToast(error.response?.data?.error || error.message || 'Failed to load organizations', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadOrganizations();
  }, []);

  // Open add dialog
  const handleAdd = () => {
    setEditingOrg(null);
    setFormData({
      name: '',
      display_name: '',
      description: '',
      is_active: true,
    });
    setFormErrors({});
    setShowDialog(true);
  };

  // Open edit dialog
  const handleEdit = (org: Organization) => {
    setEditingOrg(org);
    setFormData({
      name: org.name,
      display_name: org.display_name,
      description: org.description || '',
      is_active: org.is_active,
    });
    setFormErrors({});
    setShowDialog(true);
  };

  // Validate form
  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    if (!formData.name.trim()) {
      errors.name = 'Organization ID is required';
    } else if (!/^[a-z0-9-]+$/.test(formData.name.trim())) {
      errors.name = 'Organization ID can only contain lowercase letters, numbers, and hyphens';
    }

    if (!formData.display_name.trim()) {
      errors.display_name = 'Display name is required';
    } else if (formData.display_name.trim().length < 2) {
      errors.display_name = 'Display name must be at least 2 characters';
    } else if (formData.display_name.trim().length > 100) {
      errors.display_name = 'Display name cannot exceed 100 characters';
    }

    if (formData.description && formData.description.length > 500) {
      errors.description = 'Description cannot exceed 500 characters';
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // Submit form
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    try {
      if (editingOrg) {
        // Update
        const updateData: UpdateOrganizationRequest = {
          display_name: formData.display_name,
          description: formData.description || undefined,
          is_active: formData.is_active,
        };
        await iamService.updateOrganization(editingOrg.id, updateData);
        showToast('Organization updated successfully', 'success');
      } else {
        // Create
        const createData: CreateOrganizationRequest = {
          name: formData.name,
          display_name: formData.display_name,
          description: formData.description || undefined,
        };
        await iamService.createOrganization(createData);
        showToast('Organization created successfully', 'success');
      }

      setShowDialog(false);
      loadOrganizations();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Operation failed', 'error');
    }
  };

  // Toggle organization status
  const handleToggleStatus = async (org: Organization) => {
    try {
      const updateData: UpdateOrganizationRequest = {
        display_name: org.display_name,
        description: org.description,
        is_active: !org.is_active,
      };
      await iamService.updateOrganization(org.id, updateData);
      showToast(`Organization ${!org.is_active ? 'enabled' : 'disabled'}`, 'success');
      loadOrganizations();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Operation failed', 'error');
    }
  };

  // Delete organization
  const handleDelete = (org: Organization) => {
    setDeleteConfirm({ show: true, org });
  };

  const confirmDelete = async () => {
    if (!deleteConfirm.org) return;

    try {
      await iamService.deleteOrganization(deleteConfirm.org.id);
      showToast('Organization deleted successfully', 'success');
      setDeleteConfirm({ show: false, org: null });
      loadOrganizations();
    } catch (error: any) {
      showToast(error.response?.data?.error || 'Delete failed', 'error');
    }
  };

  // Filter organizations
  const filteredOrganizations = organizations.filter((org) => {
    const matchesSearch = org.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      org.display_name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (org.description && org.description.toLowerCase().includes(searchTerm.toLowerCase()));
    
    const matchesStatus = statusFilter === 'all' ||
      (statusFilter === 'active' && org.is_active) ||
      (statusFilter === 'inactive' && !org.is_active);

    return matchesSearch && matchesStatus;
  });

  // Render status badge
  const renderStatusBadge = (org: Organization) => {
    if (org.is_active) {
      return <span className={`${styles.statusBadge} ${styles.active}`}>Active</span>;
    }
    return <span className={`${styles.statusBadge} ${styles.inactive}`}>Inactive</span>;
  };

  return (
    <div className={styles.container}>
      {/* Page header */}
      <div className={styles.header}>
        <h1 className={styles.title}>Organizations</h1>
        <p className={styles.description}>
          Manage organizations in the platform. Organizations are the top-level unit for permission management and can contain multiple projects and teams.
        </p>
      </div>

      {/* Action bar */}
      <div className={styles.actions}>
        <div className={styles.filters}>
          <input
            type="text"
            className={styles.searchInput}
            placeholder="Search organization name or description..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
          <select
            className={styles.filterSelect}
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as any)}
          >
            <option value="all">All Status</option>
            <option value="active">Enabled</option>
            <option value="inactive">Disabled</option>
          </select>
        </div>
        <button className={styles.addButton} onClick={handleAdd}>
          + Add Organization
        </button>
      </div>

      {/* Organizations list */}
      <div className={styles.organizationsList}>
        {loading ? (
          <div className={styles.loading}>Loading...</div>
        ) : filteredOrganizations.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>
              {searchTerm || statusFilter !== 'all' ? 'No matching organizations found' : 'No organizations'}
            </div>
            <div className={styles.emptyHint}>
              {searchTerm || statusFilter !== 'all' ? 'Try adjusting your search criteria' : 'Click "Add Organization" to create your first organization'}
            </div>
          </div>
        ) : (
          <table className={styles.organizationsTable}>
            <thead>
              <tr>
                <th>Organization Name</th>
                <th>Description</th>
                <th>Status</th>
                <th>Created</th>
                <th>Updated</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {filteredOrganizations.map((org) => (
                <tr key={org.id}>
                  <td className={styles.nameCell}>
                    <div>
                      <div className={styles.orgName}>{org.display_name}</div>
                      <div className={styles.orgId}>{org.name}</div>
                    </div>
                  </td>
                  <td className={styles.descriptionCell} title={org.description}>
                    {org.description || '-'}
                  </td>
                  <td>{renderStatusBadge(org)}</td>
                  <td className={styles.dateCell}>
                    {new Date(org.created_at).toLocaleDateString('en-US')}
                  </td>
                  <td className={styles.dateCell}>
                    {new Date(org.updated_at).toLocaleDateString('en-US')}
                  </td>
                  <td>
                    <div className={styles.actionButtons}>
                      <button className={styles.actionButton} onClick={() => handleEdit(org)}>
                        Edit
                      </button>
                      <button
                        className={`${styles.actionButton} ${org.is_active ? styles.disable : styles.enable}`}
                        onClick={() => handleToggleStatus(org)}
                      >
                        {org.is_active ? 'Disable' : 'Enable'}
                      </button>
                      <button
                        className={`${styles.actionButton} ${styles.delete}`}
                        onClick={() => handleDelete(org)}
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
      </div>

      {/* Add/Edit dialog */}
      {showDialog && (
        <div className={styles.dialog} onClick={() => setShowDialog(false)}>
          <div className={styles.dialogContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.dialogHeader}>
              <h2 className={styles.dialogTitle}>
                {editingOrg ? 'Edit Organization' : 'Add Organization'}
              </h2>
            </div>

            <form onSubmit={handleSubmit}>
              <div className={styles.dialogBody}>
                {/* Organization ID */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    Organization ID<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.name ? styles.error : ''}`}
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    placeholder="e.g., tech-dept"
                    disabled={!!editingOrg}
                  />
                  {formErrors.name && <span className={styles.errorText}>{formErrors.name}</span>}
                  {!formErrors.name && <span className={styles.hint}>Lowercase letters, numbers, and hyphens. Cannot be changed after creation.</span>}
                </div>

                {/* Display name */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    Display Name<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.display_name ? styles.error : ''}`}
                    value={formData.display_name}
                    onChange={(e) => setFormData({ ...formData, display_name: e.target.value })}
                    placeholder="e.g., Tech Department"
                  />
                  {formErrors.display_name && <span className={styles.errorText}>{formErrors.display_name}</span>}
                  {!formErrors.display_name && <span className={styles.hint}>2-100 characters</span>}
                </div>

                {/* Description */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>Description</label>
                  <textarea
                    className={`${styles.textarea} ${formErrors.description ? styles.error : ''}`}
                    value={formData.description}
                    onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                    placeholder="Brief description of the organization (optional)"
                    rows={3}
                  />
                  {formErrors.description && (
                    <span className={styles.errorText}>{formErrors.description}</span>
                  )}
                  {!formErrors.description && (
                    <span className={styles.hint}>Maximum 500 characters</span>
                  )}
                </div>

                {/* Enable status */}
                <div className={styles.checkbox}>
                  <input
                    type="checkbox"
                    id="is_active"
                    checked={formData.is_active}
                    onChange={(e) => setFormData({ ...formData, is_active: e.target.checked })}
                  />
                  <label htmlFor="is_active">Enable this organization</label>
                </div>
                <div className={styles.checkboxHint}>When disabled, all projects and teams under this organization will be inaccessible</div>
              </div>

              <div className={styles.dialogFooter}>
                <button
                  type="button"
                  className={`${styles.button} ${styles.secondary}`}
                  onClick={() => setShowDialog(false)}
                >
                  Cancel
                </button>
                <button type="submit" className={`${styles.button} ${styles.primary}`}>
                  {editingOrg ? 'Save' : 'Create'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete confirmation dialog */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="Confirm Delete Organization"
        message={deleteConfirm.org ? `Are you sure you want to delete the organization "${deleteConfirm.org.display_name}"? All projects, teams, and permission configurations under this organization will be deleted. This action cannot be undone!` : ''}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm({ show: false, org: null })}
        confirmText="Delete"
        cancelText="Cancel"
        type="danger"
      />
    </div>
  );
};

export default OrganizationManagement;
