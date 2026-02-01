import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../../hooks/useToast';
import { iamService } from '../../services/iam';
import { workspaceService } from '../../services/workspaces';
import type {
  PermissionGrant,
  PermissionDefinition,
  PrincipalType,
  PermissionLevel,
} from '../../services/iam';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './PermissionManagement.module.css';

// 用户接口
interface User {
  id: number;
  username: string;
  email: string;
  role: string;
  is_active: boolean;
}

// 用户角色接口
interface UserRole {
  id: number;
  user_id: number;
  role_id: number;
  role_name: string;
  role_display_name: string;
  scope_type: string;
  scope_id: number;
  assigned_at: string;
  expires_at?: string;
  reason?: string;
}

// 用户权限聚合接口
interface UserPermissions {
  user: User;
  permissions: PermissionGrant[];
  roles: UserRole[];
  expanded: boolean;
}

const PermissionManagement: React.FC = () => {
  const navigate = useNavigate();
  const [userPermissions, setUserPermissions] = useState<UserPermissions[]>([]);
  const [definitions, setDefinitions] = useState<PermissionDefinition[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingRoles, setLoadingRoles] = useState(false);
  const [editingPermission, setEditingPermission] = useState<PermissionGrant | null>(null);
  const [selectedPermissions, setSelectedPermissions] = useState<Map<number, Set<number>>>(new Map());
  const [revokeConfirm, setRevokeConfirm] = useState<{
    show: boolean;
    permission: PermissionGrant | null;
  }>({
    show: false,
    permission: null,
  });
  const [batchRevokeConfirm, setBatchRevokeConfirm] = useState<{
    show: boolean;
    userId: number | null;
    permissionIds: number[];
  }>({
    show: false,
    userId: null,
    permissionIds: [],
  });
  const [revokeRoleConfirm, setRevokeRoleConfirm] = useState<{
    show: boolean;
    userRole: UserRole | null;
  }>({
    show: false,
    userRole: null,
  });
  const { showToast } = useToast();

  // 加载用户列表
  const loadUsers = async () => {
    try {
      const response = await iamService.listUsers({ is_active: true, limit: 1000 });
      setUsers(response.users || []);
      return response.users || [];
    } catch (error: any) {
      console.error('加载用户列表失败:', error);
      showToast(error.response?.data?.error || '加载用户列表失败', 'error');
      return [];
    }
  };

  // 加载权限定义
  const loadDefinitions = async () => {
    try {
      const response = await iamService.listPermissionDefinitions();
      setDefinitions(response.definitions || []);
    } catch (error: any) {
      console.error('加载权限定义失败:', error);
      showToast(error.response?.data?.error || '加载权限定义失败', 'error');
    }
  };

  // 加载所有用户的权限
  const loadAllUserPermissions = async (userList: User[]) => {
    try {
      setLoading(true);
      
      // 使用新的API并行获取每个用户的权限和角色
      const userPermissionsData: UserPermissions[] = await Promise.all(
        userList.map(async (user) => {
          let permissions: PermissionGrant[] = [];
          let roles: UserRole[] = [];
          
          try {
            // 使用新的API获取用户权限
            const permResponse = await fetch(`/api/v1/iam/users/${user.id}/permissions`, {
              headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`,
              },
            });
            if (permResponse.ok) {
              const permData = await permResponse.json();
              permissions = permData.data || [];
            }
          } catch (error) {
            console.error(`加载用户 ${user.id} 的权限失败:`, error);
          }

          try {
            // 获取用户角色
            const roleResponse = await fetch(`/api/v1/iam/users/${user.id}/roles`, {
              headers: {
                'Authorization': `Bearer ${localStorage.getItem('token')}`,
              },
            });
            if (roleResponse.ok) {
              const roleData = await roleResponse.json();
              roles = roleData.data || [];
            }
          } catch (error) {
            console.error(`加载用户 ${user.id} 的角色失败:`, error);
          }

          return {
            user,
            permissions,
            roles,
            expanded: false,
          };
        })
      );

      setUserPermissions(userPermissionsData);
    } catch (error: any) {
      console.error('加载用户权限失败:', error);
      showToast(error.response?.data?.error || '加载用户权限失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    const init = async () => {
      await loadDefinitions();
      const userList = await loadUsers();
      await loadAllUserPermissions(userList);
    };
    init();
  }, []);

  // 切换用户展开/折叠
  const toggleUserExpanded = (userId: number) => {
    setUserPermissions((prev) =>
      prev.map((up) =>
        up.user.id === userId ? { ...up, expanded: !up.expanded } : up
      )
    );
  };

  // 跳转到授权页面
  const handleGrantPermission = () => {
    navigate('/iam/permissions/grant');
  };

  // 编辑权限
  const handleEdit = (permission: PermissionGrant) => {
    setEditingPermission(permission);
  };

  // 保存编辑
  const handleSaveEdit = async (newLevel: PermissionLevel) => {
    if (!editingPermission) return;

    try {
      // 先撤销旧权限
      await iamService.revokePermission(editingPermission.scope_type, editingPermission.id);
      
      // 再授予新权限
      await iamService.grantPermission({
        scope_type: editingPermission.scope_type,
        scope_id: editingPermission.scope_id,
        principal_type: editingPermission.principal_type,
        principal_id: editingPermission.principal_id,
        permission_id: editingPermission.permission_id,
        permission_level: newLevel,
        expires_at: editingPermission.expires_at,
        reason: editingPermission.reason,
      });

      showToast('权限级别更新成功', 'success');
      setEditingPermission(null);
      
      // 重新加载数据
      const userList = await loadUsers();
      await loadAllUserPermissions(userList);
    } catch (error: any) {
      showToast(error.response?.data?.error || '更新失败', 'error');
    }
  };

  // 切换权限选择
  const togglePermissionSelection = (permissionId: number, userId: number) => {
    setSelectedPermissions((prev) => {
      const newMap = new Map(prev);
      const userPerms = newMap.get(userId) || new Set();
      
      if (userPerms.has(permissionId)) {
        userPerms.delete(permissionId);
      } else {
        userPerms.add(permissionId);
      }
      
      if (userPerms.size === 0) {
        newMap.delete(userId);
      } else {
        newMap.set(userId, userPerms);
      }
      
      return newMap;
    });
  };

  // 获取用户选中的权限数量
  const getSelectedCount = (userId: number) => {
    return selectedPermissions.get(userId)?.size || 0;
  };

  // 撤销单个权限
  const handleRevoke = (permission: PermissionGrant) => {
    setRevokeConfirm({ show: true, permission });
  };

  const confirmRevoke = async () => {
    if (!revokeConfirm.permission) return;

    try {
      await iamService.revokePermission(
        revokeConfirm.permission.scope_type,
        revokeConfirm.permission.id
      );
      showToast('权限撤销成功', 'success');
      setRevokeConfirm({ show: false, permission: null });
      
      // 重新加载数据，但保持展开状态
      const expandedUserIds = userPermissions
        .filter(up => up.expanded)
        .map(up => up.user.id);
      
      const userList = await loadUsers();
      await loadAllUserPermissions(userList);
      
      // 恢复展开状态
      setUserPermissions((prev) =>
        prev.map((up) => ({
          ...up,
          expanded: expandedUserIds.includes(up.user.id),
        }))
      );
    } catch (error: any) {
      showToast(error.response?.data?.error || '撤销失败', 'error');
    }
  };

  // 撤销用户角色
  const handleRevokeRole = (userRole: UserRole) => {
    setRevokeRoleConfirm({ show: true, userRole });
  };

  const confirmRevokeRole = async () => {
    if (!revokeRoleConfirm.userRole) return;

    try {
      const response = await fetch(
        `/api/v1/iam/users/${revokeRoleConfirm.userRole.user_id}/roles/${revokeRoleConfirm.userRole.id}`,
        {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('token')}`,
          },
        }
      );

      if (!response.ok) {
        throw new Error('Failed to revoke role');
      }

      showToast('角色撤销成功', 'success');
      setRevokeRoleConfirm({ show: false, userRole: null });
      
      // 重新加载数据，保持展开状态
      const expandedUserIds = userPermissions
        .filter(up => up.expanded)
        .map(up => up.user.id);
      
      const userList = await loadUsers();
      await loadAllUserPermissions(userList);
      
      // 恢复展开状态
      setUserPermissions((prev) =>
        prev.map((up) => ({
          ...up,
          expanded: expandedUserIds.includes(up.user.id),
        }))
      );
    } catch (error: any) {
      showToast('撤销角色失败', 'error');
    }
  };

  // 批量撤销选中的权限
  const handleBatchRevoke = (userId: number) => {
    const selectedPerms = selectedPermissions.get(userId);
    if (!selectedPerms || selectedPerms.size === 0) {
      showToast('请先勾选要撤销的权限', 'error');
      return;
    }

    const userPerms = userPermissions.find(up => up.user.id === userId);
    if (!userPerms) return;

    const permissionsToRevoke = userPerms.permissions.filter(p => selectedPerms.has(p.id));
    
    setBatchRevokeConfirm({ 
      show: true, 
      userId,
      permissionIds: Array.from(selectedPerms)
    });
  };

  const confirmBatchRevoke = async () => {
    if (!batchRevokeConfirm.userId || batchRevokeConfirm.permissionIds.length === 0) return;

    const userPerms = userPermissions.find(up => up.user.id === batchRevokeConfirm.userId);
    if (!userPerms) return;

    try {
      let successCount = 0;
      let failCount = 0;

      for (const permId of batchRevokeConfirm.permissionIds) {
        const permission = userPerms.permissions.find(p => p.id === permId);
        if (permission) {
          try {
            await iamService.revokePermission(permission.scope_type, permission.id);
            successCount++;
          } catch (error) {
            failCount++;
          }
        }
      }

      if (failCount > 0) {
        showToast(`撤销完成：成功 ${successCount} 个，失败 ${failCount} 个`, 'warning');
      } else {
        showToast(`成功撤销 ${successCount} 个权限`, 'success');
      }

      // 清除选中状态
      setSelectedPermissions((prev) => {
        const newMap = new Map(prev);
        newMap.delete(batchRevokeConfirm.userId!);
        return newMap;
      });

      setBatchRevokeConfirm({ show: false, userId: null, permissionIds: [] });
      
      // 重新加载数据，保持展开状态
      const expandedUserIds = userPermissions
        .filter(up => up.expanded)
        .map(up => up.user.id);
      
      const userList = await loadUsers();
      await loadAllUserPermissions(userList);
      
      // 恢复展开状态
      setUserPermissions((prev) =>
        prev.map((up) => ({
          ...up,
          expanded: expandedUserIds.includes(up.user.id),
        }))
      );
    } catch (error: any) {
      showToast('批量撤销失败', 'error');
    }
  };

  // 获取权限定义名称
  const getPermissionName = (permissionId: number) => {
    const def = definitions.find((d) => d.id === permissionId);
    return def ? def.display_name : `权限 ${permissionId}`;
  };

  // 获取作用域显示名称
  const getScopeDisplayName = (scopeType: string, scopeId: number) => {
    return `${scopeType} #${scopeId}`;
  };

  // 渲染权限级别徽章
  const renderLevelBadge = (level: PermissionLevel) => {
    const levelMap = {
      NONE: { class: styles.none, text: 'None' },
      READ: { class: styles.read, text: 'Read' },
      WRITE: { class: styles.write, text: 'Write' },
      ADMIN: { class: styles.admin, text: 'Admin' },
    };
    const config = levelMap[level] || levelMap.NONE;
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

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>权限管理</h1>
          <p className={styles.description}>
            查看和管理所有用户的权限授予，支持按用户聚合查看和细粒度的权限控制。
          </p>
        </div>
        <button className={styles.addButton} onClick={handleGrantPermission}>
          + 新增授权
        </button>
      </div>

      {/* 用户权限列表 */}
      <div className={styles.userList}>
        {loading ? (
          <div className={styles.loading}>加载中...</div>
        ) : userPermissions.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>暂无用户数据</div>
            <div className={styles.emptyHint}>系统中还没有用户或权限数据</div>
          </div>
        ) : (
          userPermissions.map((up) => (
            <div key={up.user.id} className={styles.userCard}>
              {/* 用户信息行 */}
              <div
                className={styles.userHeader}
                onClick={() => toggleUserExpanded(up.user.id)}
              >
                <div className={styles.userInfo}>
                  <div className={styles.userAvatar}>
                    {up.user.username.charAt(0).toUpperCase()}
                  </div>
                  <div className={styles.userDetails}>
                    <div className={styles.userName}>
                      {up.user.username}
                      {up.user.role === 'admin' && (
                        <span className={styles.roleTag}>超级管理员</span>
                      )}
                    </div>
                    <div className={styles.userEmail}>{up.user.email}</div>
                  </div>
                </div>
                <div className={styles.userStats}>
                  {up.roles.length > 0 && (
                    <span className={styles.roleCount}>
                      {up.roles.length} 个角色
                    </span>
                  )}
                  <span className={styles.permissionCount}>
                    {up.permissions.length} 个权限
                  </span>
                  {up.expanded && up.permissions.length > 0 && getSelectedCount(up.user.id) > 0 && (
                    <button
                      className={styles.batchRevokeButton}
                      onClick={(e) => {
                        e.stopPropagation();
                        handleBatchRevoke(up.user.id);
                      }}
                    >
                      批量撤销 ({getSelectedCount(up.user.id)})
                    </button>
                  )}
                  <span className={styles.expandIcon}>
                    {up.expanded ? '▼' : '▶'}
                  </span>
                </div>
              </div>

              {/* 权限详情（展开时显示） */}
              {up.expanded && (
                <div className={styles.permissionsDetail}>
                  {/* 用户角色部分 */}
                  <div className={styles.rolesSection}>
                    <div className={styles.sectionHeader}>
                      <h3 className={styles.sectionTitle}>分配的角色</h3>
                      <button
                        className={styles.addRoleButton}
                        onClick={(e) => {
                          e.stopPropagation();
                          navigate(`/iam/permissions/grant?user_id=${up.user.id}&type=role`);
                        }}
                      >
                        + 分配角色
                      </button>
                    </div>
                    {up.roles.length > 0 ? (
                      <div className={styles.rolesList}>
                        {up.roles.map((role) => (
                          <div key={role.id} className={styles.roleItem}>
                            <div className={styles.roleInfo}>
                              <div className={styles.roleHeader}>
                                <span className={styles.roleName}>{role.role_display_name}</span>
                                <span className={styles.roleScope}>
                                  {renderScopeTypeBadge(role.scope_type)} #{role.scope_id}
                                </span>
                              </div>
                              <div className={styles.roleTime}>
                                分配于 {new Date(role.assigned_at).toLocaleDateString('zh-CN')}
                              </div>
                            </div>
                            <button
                              className={styles.revokeRoleButton}
                              onClick={() => handleRevokeRole(role)}
                            >
                              撤销
                            </button>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className={styles.noRoles}>该用户暂无分配的角色</div>
                    )}
                  </div>

                  {/* 直接授予的权限 */}
                  <div className={styles.sectionHeader}>
                    <h3 className={styles.sectionTitle}>直接授予的权限</h3>
                    <button
                      className={styles.addPermissionButton}
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate(`/iam/permissions/grant?user_id=${up.user.id}&type=permission`);
                      }}
                    >
                      + 添加权限
                    </button>
                  </div>
                  {up.permissions.length === 0 ? (
                    <div className={styles.noPermissions}>该用户暂无直接授予的权限</div>
                  ) : (
                    <table className={styles.permissionsTable}>
                      <thead>
                        <tr>
                          <th style={{ width: '40px' }}>
                            <input
                              type="checkbox"
                              onChange={(e) => {
                                const allPermIds = up.permissions.map(p => p.id);
                                if (e.target.checked) {
                                  setSelectedPermissions((prev) => {
                                    const newMap = new Map(prev);
                                    newMap.set(up.user.id, new Set(allPermIds));
                                    return newMap;
                                  });
                                } else {
                                  setSelectedPermissions((prev) => {
                                    const newMap = new Map(prev);
                                    newMap.delete(up.user.id);
                                    return newMap;
                                  });
                                }
                              }}
                              checked={getSelectedCount(up.user.id) === up.permissions.length && up.permissions.length > 0}
                            />
                          </th>
                          <th>作用域</th>
                          <th>作用域ID</th>
                          <th>权限</th>
                          <th>级别</th>
                          <th>授予时间</th>
                          <th>过期时间</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {up.permissions.map((permission) => (
                          <tr key={permission.id}>
                            <td>
                              <input
                                type="checkbox"
                                checked={selectedPermissions.get(up.user.id)?.has(permission.id) || false}
                                onChange={() => togglePermissionSelection(permission.id, up.user.id)}
                                onClick={(e) => e.stopPropagation()}
                              />
                            </td>
                            <td>
                              {renderScopeTypeBadge(permission.scope_type)}
                            </td>
                            <td className={styles.scopeIdCell}>
                              {getScopeDisplayName(permission.scope_type, permission.scope_id)}
                            </td>
                            <td className={styles.permissionCell}>
                              {getPermissionName(permission.permission_id)}
                            </td>
                            <td>
                              {editingPermission?.id === permission.id ? (
                                <select
                                  className={styles.levelSelect}
                                  value={editingPermission.permission_level}
                                  onChange={(e) =>
                                    setEditingPermission({
                                      ...editingPermission,
                                      permission_level: e.target.value as PermissionLevel,
                                    })
                                  }
                                  autoFocus
                                >
                                  <option value="READ">Read</option>
                                  <option value="WRITE">Write</option>
                                  <option value="ADMIN">Admin</option>
                                </select>
                              ) : (
                                renderLevelBadge(permission.permission_level)
                              )}
                            </td>
                            <td className={styles.dateCell}>
                              {new Date(permission.granted_at).toLocaleDateString('zh-CN')}
                            </td>
                            <td className={styles.dateCell}>
                              {permission.expires_at
                                ? new Date(permission.expires_at).toLocaleDateString('zh-CN')
                                : '永久'}
                            </td>
                            <td>
                              {editingPermission?.id === permission.id ? (
                                <div className={styles.editActions}>
                                  <button
                                    className={`${styles.actionButton} ${styles.save}`}
                                    onClick={() => handleSaveEdit(editingPermission.permission_level)}
                                  >
                                    保存
                                  </button>
                                  <button
                                    className={`${styles.actionButton} ${styles.cancel}`}
                                    onClick={() => setEditingPermission(null)}
                                  >
                                    取消
                                  </button>
                                </div>
                              ) : (
                                <div className={styles.editActions}>
                                  <button
                                    className={`${styles.actionButton} ${styles.edit}`}
                                    onClick={() => handleEdit(permission)}
                                  >
                                    编辑
                                  </button>
                                  <button
                                    className={`${styles.actionButton} ${styles.revoke}`}
                                    onClick={() => handleRevoke(permission)}
                                  >
                                    撤销
                                  </button>
                                </div>
                              )}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                </div>
              )}
            </div>
          ))
        )}
      </div>

      {/* 撤销确认对话框 */}
      <ConfirmDialog
        isOpen={revokeConfirm.show}
        title="确认撤销"
        message={`确定要撤销此权限授予吗？撤销后，用户将立即失去相应的访问权限。`}
        onConfirm={confirmRevoke}
        onCancel={() => setRevokeConfirm({ show: false, permission: null })}
        confirmText="撤销"
        cancelText="取消"
      />

      {/* 批量撤销确认对话框 */}
      <ConfirmDialog
        isOpen={batchRevokeConfirm.show}
        title="确认批量撤销"
        message={`确定要撤销选中的 ${batchRevokeConfirm.permissionIds.length} 个权限吗？撤销后，用户将立即失去相应的访问权限。`}
        onConfirm={confirmBatchRevoke}
        onCancel={() => setBatchRevokeConfirm({ show: false, userId: null, permissionIds: [] })}
        confirmText="批量撤销"
        cancelText="取消"
      />

      {/* 撤销角色确认对话框 */}
      <ConfirmDialog
        isOpen={revokeRoleConfirm.show}
        title="确认撤销角色"
        message={`确定要撤销角色"${revokeRoleConfirm.userRole?.role_display_name}"吗？撤销后，用户将失去该角色的所有权限。`}
        onConfirm={confirmRevokeRole}
        onCancel={() => setRevokeRoleConfirm({ show: false, userRole: null })}
        confirmText="撤销"
        cancelText="取消"
      />
    </div>
  );
};

export default PermissionManagement;
