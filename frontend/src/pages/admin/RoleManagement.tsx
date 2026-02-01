import React, { useState, useEffect } from 'react';
import { useToast } from '../../hooks/useToast';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './RoleManagement.module.css';

// 角色接口
interface Role {
  id: number;
  name: string;
  display_name: string;
  description: string;
  is_system: boolean;
  is_active: boolean;
  created_at: string;
  policy_count?: number;
}

// 角色策略接口
interface RolePolicy {
  id: number;
  role_id: number;
  permission_id: string; // 业务语义ID
  permission_name: string;
  permission_display_name: string;
  resource_type: string;
  permission_level: string;
  scope_type: string;
}

// 权限定义接口
interface PermissionDefinition {
  id: string; // 业务语义ID
  name: string;
  display_name: string;
  resource_type: string;
  scope_level: string;
}

const RoleManagement: React.FC = () => {
  const [roles, setRoles] = useState<Role[]>([]);
  const [selectedRole, setSelectedRole] = useState<Role | null>(null);
  const [policies, setPolicies] = useState<RolePolicy[]>([]);
  const [permissions, setPermissions] = useState<PermissionDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingPolicies, setLoadingPolicies] = useState(false);
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [showAddPolicyForm, setShowAddPolicyForm] = useState(false);
  const [isAddingPolicy, setIsAddingPolicy] = useState(false);
  const [permissionSearch, setPermissionSearch] = useState('');
  const [editingPolicy, setEditingPolicy] = useState<RolePolicy | null>(null);
  const [removePolicyConfirm, setRemovePolicyConfirm] = useState<{
    show: boolean;
    policyId: number | null;
  }>({
    show: false,
    policyId: null,
  });
  const [createFormData, setCreateFormData] = useState({
    name: '',
    display_name: '',
    description: '',
  });
  const [policyFormData, setPolicyFormData] = useState({
    permission_id: '',
    permission_level: 'READ',
    scope_type: 'WORKSPACE',
  });
  const [submitting, setSubmitting] = useState(false);
  const [cloningRole, setCloningRole] = useState<Role | null>(null);
  const [cloneFormData, setCloneFormData] = useState({
    name: '',
    display_name: '',
    description: '',
  });
  const { showToast } = useToast();

  // 加载角色列表
  const loadRoles = async () => {
    try {
      setLoading(true);
      const response = await fetch('/api/v1/iam/roles', {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });

      if (!response.ok) {
        throw new Error('Failed to load roles');
      }

      const data = await response.json();
      setRoles(data.roles || []);
    } catch (error: any) {
      console.error('加载角色列表失败:', error);
      showToast('加载角色列表失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  // 加载角色详情
  const loadRoleDetails = async (roleId: number) => {
    try {
      setLoadingPolicies(true);
      const response = await fetch(`/api/v1/iam/roles/${roleId}`, {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });

      if (!response.ok) {
        throw new Error('Failed to load role details');
      }

      const data = await response.json();
      setPolicies(data.policies || []);
    } catch (error: any) {
      console.error('加载角色详情失败:', error);
      showToast('加载角色详情失败', 'error');
    } finally {
      setLoadingPolicies(false);
    }
  };

  // 加载权限定义列表
  const loadPermissions = async () => {
    try {
      const response = await fetch('/api/v1/iam/permissions/definitions', {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });

      if (!response.ok) {
        throw new Error('Failed to load permissions');
      }

      const data = await response.json();
      setPermissions(data.definitions || []);
    } catch (error: any) {
      console.error('加载权限定义失败:', error);
    }
  };

  useEffect(() => {
    loadRoles();
    loadPermissions();
  }, []);

  // 选择角色
  const handleSelectRole = (role: Role) => {
    setSelectedRole(role);
    loadRoleDetails(role.id);
  };

  // 创建自定义角色
  const handleCreateRole = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!createFormData.name || !createFormData.display_name) {
      showToast('请填写必填字段', 'error');
      return;
    }

    try {
      setSubmitting(true);
      const response = await fetch('/api/v1/iam/roles', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
        body: JSON.stringify(createFormData),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.message || 'Failed to create role');
      }

      showToast('角色创建成功', 'success');
      setShowCreateForm(false);
      setCreateFormData({ name: '', display_name: '', description: '' });
      loadRoles();
    } catch (error: any) {
      showToast(error.message || '创建角色失败', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  // 开始克隆角色
  const handleStartClone = (role: Role, e: React.MouseEvent) => {
    e.stopPropagation();
    setCloningRole(role);
    setCloneFormData({
      name: `${role.name}_copy`,
      display_name: `${role.display_name} (副本)`,
      description: role.description,
    });
  };

  // 取消克隆
  const handleCancelClone = () => {
    setCloningRole(null);
    setCloneFormData({ name: '', display_name: '', description: '' });
  };

  // 执行克隆
  const handleCloneRole = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!cloningRole || !cloneFormData.name || !cloneFormData.display_name) {
      showToast('请填写必填字段', 'error');
      return;
    }

    try {
      setSubmitting(true);
      const response = await fetch(`/api/v1/iam/roles/${cloningRole.id}/clone`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
        body: JSON.stringify(cloneFormData),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.message || 'Failed to clone role');
      }

      const result = await response.json();
      showToast(result.message || '角色克隆成功', 'success');
      handleCancelClone();
      loadRoles();
    } catch (error: any) {
      showToast(error.message || '克隆角色失败', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  // 为角色添加权限策略
  const handleAddPolicy = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!selectedRole || !policyFormData.permission_id) {
      showToast('请选择权限', 'error');
      return;
    }

    try {
      setSubmitting(true);
      const response = await fetch(`/api/v1/iam/roles/${selectedRole.id}/policies`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
        body: JSON.stringify(policyFormData),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.message || 'Failed to add policy');
      }

      showToast('权限策略添加成功', 'success');
      setIsAddingPolicy(false);
      setPolicyFormData({ permission_id: '', permission_level: 'READ', scope_type: 'WORKSPACE' });
      loadRoleDetails(selectedRole.id);
      loadRoles(); // 刷新角色列表以更新策略数量
    } catch (error: any) {
      showToast(error.message || '添加策略失败', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  // 删除权限策略
  const handleRemovePolicy = (policyId: number) => {
    setRemovePolicyConfirm({ show: true, policyId });
  };

  const confirmRemovePolicy = async () => {
    if (!selectedRole || !removePolicyConfirm.policyId) return;

    try {
      const response = await fetch(`/api/v1/iam/roles/${selectedRole.id}/policies/${removePolicyConfirm.policyId}`, {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });

      if (!response.ok) {
        throw new Error('Failed to remove policy');
      }

      showToast('权限策略删除成功', 'success');
      setRemovePolicyConfirm({ show: false, policyId: null });
      loadRoleDetails(selectedRole.id);
      loadRoles(); // 刷新角色列表以更新策略数量
    } catch (error: any) {
      showToast('删除策略失败', 'error');
    }
  };

  // 编辑权限级别
  const handleEditPolicy = (policy: RolePolicy) => {
    setEditingPolicy(policy);
  };

  const handleSaveEdit = async (newLevel: string) => {
    if (!selectedRole || !editingPolicy) return;

    try {
      // 先删除旧策略
      await fetch(`/api/v1/iam/roles/${selectedRole.id}/policies/${editingPolicy.id}`, {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });

      // 再添加新策略
      const response = await fetch(`/api/v1/iam/roles/${selectedRole.id}/policies`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
        body: JSON.stringify({
          permission_id: editingPolicy.permission_id,
          permission_level: newLevel,
          scope_type: editingPolicy.scope_type,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to update policy');
      }

      showToast('权限级别更新成功', 'success');
      setEditingPolicy(null);
      loadRoleDetails(selectedRole.id);
    } catch (error: any) {
      showToast('更新失败', 'error');
    }
  };

  // 渲染权限级别徽章
  const renderLevelBadge = (level: string) => {
    const levelMap: Record<string, { class: string; text: string }> = {
      NONE: { class: styles.none, text: 'None' },
      READ: { class: styles.read, text: 'Read' },
      WRITE: { class: styles.write, text: 'Write' },
      ADMIN: { class: styles.admin, text: 'Admin' },
    };
    const config = levelMap[level] || levelMap.READ;
    return <span className={`${styles.levelBadge} ${config.class}`}>{config.text}</span>;
  };

  // 渲染作用域类型徽章
  const renderScopeTypeBadge = (scopeType: string) => {
    const typeMap: Record<string, { class: string; text: string }> = {
      ORGANIZATION: { class: styles.organization, text: 'ORG' },
      PROJECT: { class: styles.project, text: 'PRJ' },
      WORKSPACE: { class: styles.workspace, text: 'WS' },
    };
    const config = typeMap[scopeType] || { class: styles.organization, text: scopeType };
    return <span className={`${styles.scopeBadge} ${config.class}`}>{config.text}</span>;
  };

  // 过滤权限列表（搜索）
  const filteredPermissions = permissions.filter((perm) => {
    if (!permissionSearch) return true;
    const search = permissionSearch.toLowerCase();
    return (
      perm.display_name.toLowerCase().includes(search) ||
      perm.name.toLowerCase().includes(search) ||
      perm.resource_type.toLowerCase().includes(search)
    );
  });

  // 按资源类型分组策略
  const groupedPolicies = policies.reduce((acc, policy) => {
    const key = `${policy.resource_type}-${policy.scope_type}`;
    if (!acc[key]) {
      acc[key] = {
        resource_type: policy.resource_type,
        scope_type: policy.scope_type,
        policies: [],
      };
    }
    acc[key].policies.push(policy);
    return acc;
  }, {} as Record<string, { resource_type: string; scope_type: string; policies: RolePolicy[] }>);

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>角色管理</h1>
          <p className={styles.description}>
            管理IAM角色和权限策略，支持系统预定义角色和自定义角色。
          </p>
        </div>
        <button 
          className={styles.createButton}
          onClick={() => setShowCreateForm(true)}
        >
          + 创建角色
        </button>
      </div>

      {/* 创建角色表单 */}
      {showCreateForm && (
        <div className={styles.createFormOverlay} onClick={() => setShowCreateForm(false)}>
          <div className={styles.createFormModal} onClick={(e) => e.stopPropagation()}>
            <div className={styles.modalHeader}>
              <h2 className={styles.modalTitle}>创建自定义角色</h2>
              <button 
                className={styles.closeButton}
                onClick={() => setShowCreateForm(false)}
              >
                ×
              </button>
            </div>
            <form onSubmit={handleCreateRole} className={styles.createForm}>
              <div className={styles.formGroup}>
                <label className={styles.label}>
                  角色名称<span className={styles.required}>*</span>
                </label>
                <input
                  type="text"
                  className={styles.input}
                  value={createFormData.name}
                  onChange={(e) => setCreateFormData({ ...createFormData, name: e.target.value })}
                  placeholder="例如：ops_engineer"
                  required
                />
                <span className={styles.hint}>英文名称，用于API调用</span>
              </div>
              <div className={styles.formGroup}>
                <label className={styles.label}>
                  显示名称<span className={styles.required}>*</span>
                </label>
                <input
                  type="text"
                  className={styles.input}
                  value={createFormData.display_name}
                  onChange={(e) => setCreateFormData({ ...createFormData, display_name: e.target.value })}
                  placeholder="例如：运维工程师"
                  required
                />
              </div>
              <div className={styles.formGroup}>
                <label className={styles.label}>描述</label>
                <textarea
                  className={styles.textarea}
                  value={createFormData.description}
                  onChange={(e) => setCreateFormData({ ...createFormData, description: e.target.value })}
                  placeholder="角色描述（可选）"
                  rows={3}
                />
              </div>
              <div className={styles.formActions}>
                <button
                  type="button"
                  className={`${styles.button} ${styles.secondary}`}
                  onClick={() => setShowCreateForm(false)}
                  disabled={submitting}
                >
                  取消
                </button>
                <button
                  type="submit"
                  className={`${styles.button} ${styles.primary}`}
                  disabled={submitting}
                >
                  {submitting ? '创建中...' : '创建角色'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}


      <div className={styles.content}>
        {/* 左侧：角色列表 */}
        <div className={styles.rolesList}>
          <div className={styles.rolesHeader}>
            <h2 className={styles.rolesTitle}>角色列表</h2>
            <span className={styles.rolesCount}>{roles.length} 个角色</span>
          </div>

          {loading ? (
            <div className={styles.loading}>加载中...</div>
          ) : (
            <div className={styles.rolesGrid}>
              {roles.map((role) => (
                <React.Fragment key={role.id}>
                  <div
                    className={`${styles.roleCard} ${
                      selectedRole?.id === role.id ? styles.selected : ''
                    } ${cloningRole?.id === role.id ? styles.cloning : ''}`}
                    onClick={() => handleSelectRole(role)}
                  >
                    <div className={styles.roleCardHeader}>
                      <div className={styles.roleCardTitle}>
                        {role.display_name}
                        {role.is_system && (
                          <span className={styles.systemBadge}>系统</span>
                        )}
                      </div>
                      <div className={styles.roleCardActions}>
                        <button
                          className={styles.cloneButton}
                          onClick={(e) => handleStartClone(role, e)}
                          title="克隆此角色"
                        >
                          克隆
                        </button>
                        <span className={styles.policyCount}>
                          {role.policy_count || 0} 个策略
                        </span>
                      </div>
                    </div>
                    <div className={styles.roleCardDesc}>{role.description}</div>
                  </div>
                  
                  {/* 内联克隆表单 */}
                  {cloningRole?.id === role.id && (
                    <div className={styles.cloneFormInline}>
                      <h4 className={styles.cloneFormTitle}>克隆角色: {role.display_name}</h4>
                      <form onSubmit={handleCloneRole} className={styles.cloneForm}>
                        <div className={styles.formGroup}>
                          <label className={styles.label}>
                            角色名称<span className={styles.required}>*</span>
                          </label>
                          <input
                            type="text"
                            className={styles.input}
                            value={cloneFormData.name}
                            onChange={(e) => setCloneFormData({ ...cloneFormData, name: e.target.value })}
                            placeholder="例如：ops_engineer_copy"
                            required
                            autoFocus
                          />
                        </div>
                        <div className={styles.formGroup}>
                          <label className={styles.label}>
                            显示名称<span className={styles.required}>*</span>
                          </label>
                          <input
                            type="text"
                            className={styles.input}
                            value={cloneFormData.display_name}
                            onChange={(e) => setCloneFormData({ ...cloneFormData, display_name: e.target.value })}
                            placeholder="例如：运维工程师 (副本)"
                            required
                          />
                        </div>
                        <div className={styles.formGroup}>
                          <label className={styles.label}>描述</label>
                          <textarea
                            className={styles.textarea}
                            value={cloneFormData.description}
                            onChange={(e) => setCloneFormData({ ...cloneFormData, description: e.target.value })}
                            placeholder="角色描述（可选）"
                            rows={2}
                          />
                        </div>
                        <div className={styles.cloneFormActions}>
                          <button
                            type="button"
                            className={`${styles.button} ${styles.secondary}`}
                            onClick={handleCancelClone}
                            disabled={submitting}
                          >
                            取消
                          </button>
                          <button
                            type="submit"
                            className={`${styles.button} ${styles.primary}`}
                            disabled={submitting}
                          >
                            {submitting ? '克隆中...' : '克隆角色'}
                          </button>
                        </div>
                      </form>
                    </div>
                  )}
                </React.Fragment>
              ))}
            </div>
          )}
        </div>

        {/* 右侧：角色详情 */}
        <div className={styles.roleDetail}>
          {!selectedRole ? (
            <div className={styles.emptyDetail}>
              <div className={styles.emptyText}>选择一个角色查看详情</div>
              <div className={styles.emptyHint}>点击左侧角色卡片查看权限策略</div>
            </div>
          ) : (
            <>
              <div className={styles.detailHeader}>
                <div>
                  <h2 className={styles.detailTitle}>
                    {selectedRole.display_name}
                    {selectedRole.is_system && (
                      <span className={styles.systemBadge}>系统角色</span>
                    )}
                  </h2>
                  <p className={styles.detailDesc}>{selectedRole.description}</p>
                </div>
              </div>

              <div className={styles.policiesSection}>
                <h3 className={styles.policiesTitle}>权限策略</h3>
                {loadingPolicies ? (
                  <div className={styles.loading}>加载中...</div>
                ) : policies.length === 0 && !isAddingPolicy ? (
                  <div className={styles.noPolicies}>
                    <div className={styles.noPoliciesText}>该角色暂无权限策略</div>
                    {!selectedRole.is_system && (
                      <button
                        className={styles.addPolicyButtonCenter}
                        onClick={() => setIsAddingPolicy(true)}
                      >
                        + 添加权限
                      </button>
                    )}
                  </div>
                ) : (
                  <>
                    <div className={styles.policiesGrid}>
                      {Object.entries(groupedPolicies).map(([key, group]) => (
                        <div key={key} className={styles.policyGroup}>
                          <div className={styles.policyGroupHeader}>
                            <span className={styles.resourceType}>{group.resource_type}</span>
                            {renderScopeTypeBadge(group.scope_type)}
                          </div>
                        <div className={styles.policyItems}>
                          {group.policies.map((policy) => (
                            <div key={policy.id} className={styles.policyItem}>
                              <span className={styles.policyName}>
                                {policy.permission_display_name || policy.permission_name || '未知权限'}
                              </span>
                              <div className={styles.policyActions}>
                                {editingPolicy?.id === policy.id ? (
                                  <>
                                    <select
                                      className={styles.levelSelect}
                                      value={editingPolicy.permission_level}
                                      onChange={(e) => setEditingPolicy({ ...editingPolicy, permission_level: e.target.value })}
                                      autoFocus
                                    >
                                      <option value="READ">Read</option>
                                      <option value="WRITE">Write</option>
                                      <option value="ADMIN">Admin</option>
                                    </select>
                                    <button
                                      className={styles.saveButton}
                                      onClick={() => handleSaveEdit(editingPolicy.permission_level)}
                                    >
                                      保存
                                    </button>
                                    <button
                                      className={styles.cancelButton}
                                      onClick={() => setEditingPolicy(null)}
                                    >
                                      取消
                                    </button>
                                  </>
                                ) : (
                                  <>
                                    <span onClick={() => !selectedRole.is_system && handleEditPolicy(policy)} style={{ cursor: !selectedRole.is_system ? 'pointer' : 'default' }}>
                                      {renderLevelBadge(policy.permission_level)}
                                    </span>
                                    {!selectedRole.is_system && (
                                      <button
                                        className={styles.removePolicyButton}
                                        onClick={() => handleRemovePolicy(policy.id)}
                                        title="删除此策略"
                                      >
                                        ×
                                      </button>
                                    )}
                                  </>
                                )}
                              </div>
                            </div>
                          ))}
                        </div>
                        </div>
                      ))}
                    </div>
                    {!selectedRole.is_system && !isAddingPolicy && (
                      <div className={styles.addPolicyButtonContainer}>
                        <button
                          className={styles.addPolicyButton}
                          onClick={() => setIsAddingPolicy(true)}
                        >
                          + 添加更多权限
                        </button>
                      </div>
                    )}
                  </>
                )}

                {/* 内联添加权限表单 */}
                {isAddingPolicy && !selectedRole.is_system && (
                  <div className={styles.inlineForm}>
                    <h4 className={styles.inlineFormTitle}>添加权限策略</h4>
                    <form onSubmit={handleAddPolicy} className={styles.inlineFormContent}>
                      <div className={styles.inlineFormRow}>
                        <div className={styles.formGroup}>
                          <label className={styles.label}>
                            权限<span className={styles.required}>*</span>
                          </label>
                          <input
                            type="text"
                            className={styles.input}
                            placeholder="搜索权限..."
                            value={permissionSearch}
                            onChange={(e) => setPermissionSearch(e.target.value)}
                          />
                          <select
                            className={styles.input}
                            value={policyFormData.permission_id}
                            onChange={(e) => setPolicyFormData({ ...policyFormData, permission_id: e.target.value })}
                            required
                            size={Math.min(filteredPermissions.length + 1, 8)}
                          >
                            <option value="">请选择权限</option>
                            {filteredPermissions.map((perm) => (
                              <option key={perm.id} value={perm.id}>
                                {perm.display_name} ({perm.resource_type})
                              </option>
                            ))}
                          </select>
                          {permissionSearch && (
                            <span className={styles.hint}>
                              找到 {filteredPermissions.length} 个匹配的权限
                            </span>
                          )}
                        </div>
                        <div className={styles.formGroup}>
                          <label className={styles.label}>
                            权限级别<span className={styles.required}>*</span>
                          </label>
                          <select
                            className={styles.input}
                            value={policyFormData.permission_level}
                            onChange={(e) => setPolicyFormData({ ...policyFormData, permission_level: e.target.value })}
                            required
                          >
                            <option value="READ">Read</option>
                            <option value="WRITE">Write</option>
                            <option value="ADMIN">Admin</option>
                          </select>
                        </div>
                        <div className={styles.formGroup}>
                          <label className={styles.label}>
                            作用域类型<span className={styles.required}>*</span>
                          </label>
                          <select
                            className={styles.input}
                            value={policyFormData.scope_type}
                            onChange={(e) => setPolicyFormData({ ...policyFormData, scope_type: e.target.value })}
                            required
                          >
                            <option value="ORGANIZATION">组织</option>
                            <option value="PROJECT">项目</option>
                            <option value="WORKSPACE">工作空间</option>
                          </select>
                        </div>
                      </div>
                      <div className={styles.inlineFormActions}>
                        <button
                          type="button"
                          className={`${styles.button} ${styles.secondary}`}
                          onClick={() => {
                            setIsAddingPolicy(false);
                            setPolicyFormData({ permission_id: '', permission_level: 'READ', scope_type: 'WORKSPACE' });
                          }}
                          disabled={submitting}
                        >
                          取消
                        </button>
                        <button
                          type="submit"
                          className={`${styles.button} ${styles.primary}`}
                          disabled={submitting || !policyFormData.permission_id}
                        >
                          {submitting ? '添加中...' : '添加策略'}
                        </button>
                      </div>
                    </form>
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      {/* 删除策略确认对话框 */}
      <ConfirmDialog
        isOpen={removePolicyConfirm.show}
        title="确认删除"
        message="确定要删除此权限策略吗？删除后，拥有此角色的用户将失去相应的权限。"
        onConfirm={confirmRemovePolicy}
        onCancel={() => setRemovePolicyConfirm({ show: false, policyId: null })}
        confirmText="删除"
        cancelText="取消"
      />
    </div>
  );
};

export default RoleManagement;
