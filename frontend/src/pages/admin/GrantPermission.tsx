import React, { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useToast } from '../../hooks/useToast';
import { iamService } from '../../services/iam';
import { workspaceService } from '../../services/workspaces';
import type {
  PermissionDefinition,
  Organization,
  Project,
  ScopeType,
  PrincipalType,
  PermissionLevel,
  BatchGrantPermissionRequest,
  BatchGrantPermissionItem,
} from '../../services/iam';
import styles from './GrantPermission.module.css';

// 角色接口
interface Role {
  id: number;
  name: string;
  display_name: string;
  is_system: boolean;
  policy_count: number;
}

// 用户接口
interface User {
  id: number;
  username: string;
  email: string;
  role: string;
  is_active: boolean;
}

// 工作空间接口
interface Workspace {
  id: number;
  workspace_id: string;
  name: string;
  description: string;
}

// 团队接口
interface Team {
  id: string | number;
  name: string;
  display_name: string;
}

// 应用接口
interface Application {
  id: number;
  name: string;
}

const GrantPermission: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { showToast } = useToast();

  // 从URL参数读取预设值
  const urlPrincipalType = searchParams.get('principal_type') as PrincipalType | null;
  const urlPrincipalId = searchParams.get('principal_id');
  const urlGrantType = searchParams.get('type'); // 'permission' or 'role'

  const [definitions, setDefinitions] = useState<PermissionDefinition[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [applications, setApplications] = useState<Application[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  
  const [loadingScopes, setLoadingScopes] = useState(false);
  const [loadingPrincipals, setLoadingPrincipals] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  // 默认：从团队页面跳转时默认为授予权限，除非URL明确指定type=role
  const [grantType, setGrantType] = useState<'permission' | 'role'>(
    urlGrantType === 'role' ? 'role' : 'permission'
  );
  const [selectedRoleIds, setSelectedRoleIds] = useState<Set<number>>(new Set());

  // 判断是否从特定上下文进入（如团队详情页）
  const isContextLocked = urlPrincipalType && urlPrincipalId;

  // 授权表单状态
  const [grantFormData, setGrantFormData] = useState({
    scope_type: 'ORGANIZATION' as ScopeType,
    scope_id: 0 as number | string, // 支持数字 ID 和语义化 ID
    principal_type: (urlPrincipalType || 'USER') as PrincipalType,
    principal_id: urlPrincipalId || 0,
    expires_at: '',
    reason: '',
  });

  // 选中的权限（用于多选）- 使用语义ID作为key
  const [selectedPermissions, setSelectedPermissions] = useState<Map<string, PermissionLevel>>(
    new Map()
  );

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});
  const [conflictWarning, setConflictWarning] = useState<string>('');

  // 加载组织列表
  const loadOrganizations = async () => {
    try {
      const response = await iamService.listOrganizations(true);
      setOrganizations(response.organizations || []);
      if (response.organizations && response.organizations.length > 0) {
        setGrantFormData((prev) => ({ ...prev, scope_id: response.organizations[0].id }));
      }
    } catch (error: any) {
      console.error('加载组织列表失败:', error);
      showToast(error.response?.data?.error || '加载组织列表失败', 'error');
    }
  };

  // 加载项目列表
  const loadProjects = async (orgId: number) => {
    try {
      setLoadingScopes(true);
      const response = await iamService.listProjects(orgId);
      setProjects(response.projects || []);
    } catch (error: any) {
      console.error('加载项目列表失败:', error);
      showToast(error.response?.data?.error || '加载项目列表失败', 'error');
    } finally {
      setLoadingScopes(false);
    }
  };

  // 加载工作空间列表
  const loadWorkspaces = async () => {
    try {
      setLoadingScopes(true);
      const response = await workspaceService.getWorkspaces();
      let workspaceData: Workspace[] = [];
      if (response.data) {
        if ('items' in response.data && Array.isArray(response.data.items)) {
          workspaceData = response.data.items;
        } else if (Array.isArray(response.data)) {
          workspaceData = response.data;
        }
      }
      setWorkspaces(workspaceData);
    } catch (error: any) {
      console.error('加载工作空间列表失败:', error);
      showToast(error.response?.data?.error || '加载工作空间列表失败', 'error');
    } finally {
      setLoadingScopes(false);
    }
  };

  // 加载用户列表
  const loadUsers = async () => {
    try {
      setLoadingPrincipals(true);
      const response = await iamService.listUsers({ is_active: true, limit: 1000 });
      setUsers(response.users || []);
    } catch (error: any) {
      console.error('加载用户列表失败:', error);
      showToast(error.response?.data?.error || '加载用户列表失败', 'error');
    } finally {
      setLoadingPrincipals(false);
    }
  };

  // 加载团队列表
  const loadTeams = async (orgId: number) => {
    try {
      setLoadingPrincipals(true);
      const response = await iamService.listTeams(orgId);
      setTeams(response.teams || []);
    } catch (error: any) {
      console.error('加载团队列表失败:', error);
      showToast(error.response?.data?.error || '加载团队列表失败', 'error');
    } finally {
      setLoadingPrincipals(false);
    }
  };

  // 加载应用列表
  const loadApplications = async (orgId: number) => {
    try {
      setLoadingPrincipals(true);
      const response = await iamService.listApplications(orgId, true);
      setApplications(response.applications || []);
    } catch (error: any) {
      console.error('加载应用列表失败:', error);
      showToast(error.response?.data?.error || '加载应用列表失败', 'error');
    } finally {
      setLoadingPrincipals(false);
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

  // 加载角色列表
  const loadRoles = async () => {
    try {
      const response = await fetch('/api/v1/iam/roles', {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });
      if (response.ok) {
        const data = await response.json();
        setRoles(data.roles || []);
      }
    } catch (error: any) {
      console.error('加载角色列表失败:', error);
    }
  };

  useEffect(() => {
    loadOrganizations();
    loadDefinitions();
    loadUsers();
    loadRoles();
  }, []);

  // 当作用域类型改变时，加载相应的作用域列表
  useEffect(() => {
    if (grantFormData.scope_type === 'PROJECT' && organizations.length > 0) {
      loadProjects(organizations[0].id);
    } else if (grantFormData.scope_type === 'WORKSPACE') {
      loadWorkspaces();
    }
  }, [grantFormData.scope_type, organizations]);

  // 当主体类型改变时，加载相应的主体列表
  useEffect(() => {
    if (grantFormData.principal_type === 'TEAM' && organizations.length > 0) {
      loadTeams(organizations[0].id);
    } else if (grantFormData.principal_type === 'APPLICATION' && organizations.length > 0) {
      loadApplications(organizations[0].id);
    }
  }, [grantFormData.principal_type, organizations]);

  // 切换权限选择
  const togglePermission = (permissionId: string, level: PermissionLevel) => {
    setSelectedPermissions((prev) => {
      const newMap = new Map(prev);
      if (newMap.has(permissionId) && newMap.get(permissionId) === level) {
        newMap.delete(permissionId);
      } else {
        newMap.set(permissionId, level);
      }
      return newMap;
    });
  };

  // 验证授权表单
  const validateGrantForm = (): boolean => {
    const errors: Record<string, string> = {};

    if (!grantFormData.scope_id || grantFormData.scope_id === 0) {
      errors.scope_id = '请选择作用域';
    }

    if (!grantFormData.principal_id || grantFormData.principal_id === 0) {
      errors.principal_id = '请选择主体';
    }

    if (selectedPermissions.size === 0) {
      errors.permissions = '请至少选择一个权限';
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // 提交授权表单
  const handleGrantSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validateGrantForm()) {
      return;
    }

    try {
      setSubmitting(true);

      const permissionsArray: BatchGrantPermissionItem[] = Array.from(
        selectedPermissions.entries()
      ).map(([permission_id, permission_level]) => ({
        permission_id,
        permission_level,
      }));

      const request: BatchGrantPermissionRequest = {
        scope_type: grantFormData.scope_type,
        scope_id: grantFormData.scope_id,
        principal_type: grantFormData.principal_type,
        principal_id: grantFormData.principal_id,
        permissions: permissionsArray,
        expires_at: grantFormData.expires_at || undefined,
        reason: grantFormData.reason || undefined,
      };

      const result = await iamService.batchGrantPermissions(request);

      if (result.failed_count > 0) {
        // 使用结构化的conflicts数组
        const conflicts = (result as any).conflicts || [];
        
        if (conflicts.length > 0) {
          // 直接使用后端返回的结构化数据
          const detailsText = conflicts.map((c: any) => 
            `${c.permission_name}(当前级别: ${c.existing_level})`
          ).join(', ');
          
          // 设置冲突警告，显示在按钮左侧，包含成功和失败数量
          setConflictWarning(`成功 ${result.success_count} 个，失败 ${result.failed_count} 个。失败原因：以下权限已存在 - ${detailsText}。如需修改权限级别，请先撤销现有权限`);
          
          showToast(
            `部分权限已存在：成功 ${result.success_count} 个，跳过 ${result.failed_count} 个已存在的权限`,
            'warning'
          );
          
          // 不要立即跳转，让用户看到冲突信息
          return;
        } else {
          showToast(
            `授权完成：成功 ${result.success_count} 个，失败 ${result.failed_count} 个`,
            'warning'
          );
        }
      } else {
        showToast(`成功授予 ${result.success_count} 个权限`, 'success');
      }
      
      // 清除冲突警告
      setConflictWarning('');

      // 返回相应页面
      if (urlPrincipalType === 'TEAM' && urlPrincipalId) {
        navigate(`/iam/teams/${urlPrincipalId}`);
      } else {
        navigate('/iam/permissions');
      }
    } catch (error: any) {
      console.log('捕获到错误:', error);
      
      // axios拦截器已经提取了error.response.data，所以error就是响应数据
      const conflicts = error.conflicts || [];
      
      console.log('提取的conflicts数组:', conflicts);
      
      if (conflicts.length > 0) {
        // 直接使用后端返回的结构化数据
        const detailsText = conflicts.map((c: any) => 
          `${c.permission_name}(当前级别: ${c.existing_level})`
        ).join(', ');
        
        console.log('设置conflictWarning为:', detailsText);
        
        // 设置冲突警告，显示在按钮左侧，包含成功和失败数量
        const successCount = error.success_count || 0;
        const failedCount = error.failed_count || conflicts.length;
        setConflictWarning(`成功 ${successCount} 个，失败 ${failedCount} 个。失败原因：以下权限已存在 - ${detailsText}。如需修改权限级别，请先撤销现有权限`);
        
        showToast(
          `权限已存在：该主体已拥有这些权限。如需修改权限级别，请先撤销现有权限。`,
          'warning'
        );
        
        // 不要跳转，让用户看到冲突信息
        return;
      } else {
        const errorMsg = error.error || error.message || '授权失败';
        showToast(errorMsg, 'error');
      }
    } finally {
      setSubmitting(false);
    }
  };

  // 取消并返回
  const handleCancel = () => {
    // 如果是从团队详情页进入，返回团队详情页
    if (urlPrincipalType === 'TEAM' && urlPrincipalId) {
      navigate(`/iam/teams/${urlPrincipalId}`);
    } else {
      navigate('/iam/permissions');
    }
  };

  // 根据作用域类型和主体类型过滤权限定义
  const getFilteredDefinitions = () => {
    let filtered = definitions;

    if (grantFormData.scope_type === 'ORGANIZATION' && grantFormData.principal_type === 'USER') {
      const allowedResourceTypes = ['ORGANIZATION', 'PROJECTS', 'WORKSPACES', 'MODULES'];
      filtered = definitions.filter((def) => allowedResourceTypes.includes(def.resource_type));
    } else if (grantFormData.scope_type === 'WORKSPACE' && grantFormData.principal_type === 'USER') {
      const allowedResourceTypes = [
        'TASK_DATA_ACCESS',
        'WORKSPACE_EXECUTION',
        'WORKSPACE_STATE',
        'WORKSPACE_VARIABLES',
        'WORKSPACE_RESOURCES',
        'WORKSPACE_MANAGEMENT',
      ];
      filtered = definitions.filter((def) => allowedResourceTypes.includes(def.resource_type));
    }

    return filtered;
  };

  // 获取权限级别详细说明
  const getPermissionLevelDetails = (resourceType: string) => {
    const details: Record<string, { read: string; write: string; admin: string }> = {
      ORGANIZATION: {
        read: '查看组织信息、设置、配置（全只读）',
        write: '修改组织设置、更新配置',
        admin: '创建/删除组织、完全管理组织',
      },
      PROJECTS: {
        read: '查看所有项目列表和详情',
        write: '修改项目信息、更新项目配置',
        admin: '创建/删除项目、完全管理项目',
      },
      WORKSPACES: {
        read: '查看所有工作空间列表和详情',
        write: '暂未实现',
        admin: '暂未实现',
      },
      MODULES: {
        read: '查看模块列表和详情（所有GET请求）',
        write: '查看、创建、更新模块（GET/POST/PUT请求）',
        admin: '完全管理模块（所有操作包括DELETE）',
      },
      TASK_DATA_ACCESS: {
        read: '查看任务数据、执行历史',
        write: '导出任务数据、生成报告',
        admin: '完全访问所有任务数据、数据管理',
      },
      WORKSPACE_EXECUTION: {
        read: '查看任务、日志、资源变更、评论',
        write: '创建Plan任务（含plan+apply）、添加评论',
        admin: '确认执行Apply、取消任务、更新资源状态、锁定工作空间',
      },
      WORKSPACE_STATE: {
        read: '查看State版本、State内容',
        write: '创建State快照',
        admin: '回滚State、删除State版本',
      },
      WORKSPACE_VARIABLES: {
        read: '查看变量列表（敏感变量值隐藏）',
        write: '创建/更新变量',
        admin: '删除变量、查看敏感变量值',
      },
      WORKSPACE_RESOURCES: {
        read: '查看资源列表和详情',
        write: '创建、编辑、导入、部署资源、管理快照、编辑会话、Drift管理',
        admin: '删除资源、删除快照、删除Drift',
      },
      WORKSPACE_MANAGEMENT: {
        read: '查看工作空间所有数据（任务、变量、State、资源、快照等）- 统一只读权限',
        write: 'READ权限 + 创建Plan任务、管理变量、管理资源、回滚State、锁定/解锁工作空间',
        admin: 'WRITE权限 + 取消任务、确认Apply、删除State版本、删除工作空间 - 完全控制',
      },
    };
    return details[resourceType];
  };

  // 按资源类型分组权限定义
  const groupedDefinitions = getFilteredDefinitions().reduce((acc, def) => {
    if (!acc[def.resource_type]) {
      acc[def.resource_type] = [];
    }
    acc[def.resource_type].push(def);
    return acc;
  }, {} as Record<string, PermissionDefinition[]>);

  // 切换角色选择
  const toggleRole = (roleId: number) => {
    setSelectedRoleIds((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(roleId)) {
        newSet.delete(roleId);
      } else {
        newSet.add(roleId);
      }
      return newSet;
    });
  };

  // 分配角色（支持用户和团队）
  const handleAssignRole = async () => {
    if (!grantFormData.principal_id || selectedRoleIds.size === 0) {
      showToast('请选择主体和角色', 'error');
      return;
    }

    try {
      setSubmitting(true);
      let successCount = 0;
      let failCount = 0;

      // 根据主体类型选择API端点
      const apiPath = grantFormData.principal_type === 'TEAM' 
        ? `/api/v1/iam/teams/${grantFormData.principal_id}/roles`
        : `/api/v1/iam/users/${grantFormData.principal_id}/roles`;

      // 为主体分配每个选中的角色
      for (const roleId of selectedRoleIds) {
        try {
          const requestBody: any = {
            role_id: roleId,
            scope_type: grantFormData.scope_type,
            scope_id: grantFormData.scope_id,
          };
          
          if (grantFormData.expires_at) {
            requestBody.expires_at = grantFormData.expires_at;
          }
          
          if (grantFormData.reason) {
            requestBody.reason = grantFormData.reason;
          }
          
          const response = await fetch(apiPath, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${localStorage.getItem('token')}`,
            },
            body: JSON.stringify(requestBody),
          });

          if (response.ok) {
            successCount++;
          } else {
            const errorData = await response.json();
            // 如果是409冲突（已存在），不算失败
            if (response.status === 409) {
              console.log(`角色 ${roleId} 已分配，跳过`);
            } else {
              failCount++;
              console.error(`分配失败:`, errorData);
            }
          }
        } catch (error) {
          failCount++;
          console.error('分配角色出错:', error);
        }
      }

      if (failCount > 0) {
        showToast(`角色分配完成：成功 ${successCount} 个，失败 ${failCount} 个`, 'warning');
      } else {
        showToast(`成功分配 ${successCount} 个角色`, 'success');
      }

      // 返回相应页面
      if (urlPrincipalType === 'TEAM' && urlPrincipalId) {
        navigate(`/iam/teams/${urlPrincipalId}`);
      } else {
        navigate('/iam/permissions');
      }
    } catch (error: any) {
      showToast(error.message || '角色分配失败', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>新增授权</h1>
          <p className={styles.description}>
            为用户、团队或应用授予权限或分配角色。
          </p>
        </div>
      </div>

      {/* 授权类型选择 */}
      <div className={styles.typeSelector}>
        <button
          type="button"
          className={`${styles.typeButton} ${grantType === 'permission' ? styles.active : ''}`}
          onClick={() => setGrantType('permission')}
        >
          授予权限
        </button>
        <button
          type="button"
          className={`${styles.typeButton} ${grantType === 'role' ? styles.active : ''}`}
          onClick={() => setGrantType('role')}
        >
          分配角色
        </button>
      </div>
      {urlPrincipalType === 'TEAM' && (
        <div className={styles.hint} style={{ marginTop: '8px', color: '#0066cc', fontSize: '14px', background: '#e7f3ff', padding: '12px', borderRadius: '4px' }}>
          💡 提示：{grantType === 'permission' ? '为团队授予权限后，团队的所有成员将自动继承这些权限。' : '为团队分配角色后，团队的所有成员将自动继承角色包含的权限。'}
        </div>
      )}

      {/* 授权表单 */}
      <form onSubmit={grantType === 'permission' ? handleGrantSubmit : (e) => { e.preventDefault(); handleAssignRole(); }} className={styles.grantForm}>
        {/* 基本信息 */}
        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>基本信息</h2>
          <div className={styles.formRow}>
            {/* 作用域类型 */}
            <div className={styles.formGroup}>
              <label className={styles.label}>
                作用域类型<span className={styles.required}>*</span>
              </label>
              <select
                className={styles.input}
                value={grantFormData.scope_type}
                onChange={(e) => {
                  const newScopeType = e.target.value as ScopeType;
                  setGrantFormData({
                    ...grantFormData,
                    scope_type: newScopeType,
                    scope_id: 0,
                  });
                }}
              >
                <option value="ORGANIZATION">组织</option>
                <option value="PROJECT">项目</option>
                <option value="WORKSPACE">工作空间</option>
              </select>
            </div>

            {/* 作用域选择 */}
            <div className={styles.formGroup}>
              <label className={styles.label}>
                {grantFormData.scope_type === 'ORGANIZATION'
                  ? '组织'
                  : grantFormData.scope_type === 'PROJECT'
                  ? '项目'
                  : '工作空间'}
                <span className={styles.required}>*</span>
              </label>
              <select
                className={`${styles.input} ${formErrors.scope_id ? styles.error : ''}`}
                value={grantFormData.scope_id || ''}
                onChange={(e) => {
                  const value = e.target.value;
                  // 如果是 workspace，保持字符串；否则转换为数字
                  const scopeId = grantFormData.scope_type === 'WORKSPACE' ? value : Number(value);
                  setGrantFormData({ ...grantFormData, scope_id: scopeId });
                }}
                disabled={loadingScopes}
              >
                <option value="">
                  {loadingScopes
                    ? '加载中...'
                    : `请选择${
                        grantFormData.scope_type === 'ORGANIZATION'
                          ? '组织'
                          : grantFormData.scope_type === 'PROJECT'
                          ? '项目'
                          : '工作空间'
                      }`}
                </option>
                {grantFormData.scope_type === 'ORGANIZATION' &&
                  organizations.map((org) => (
                    <option key={org.id} value={org.id}>
                      {org.display_name} ({org.name})
                    </option>
                  ))}
                {grantFormData.scope_type === 'PROJECT' &&
                  projects.map((project) => (
                    <option key={project.id} value={project.id}>
                      {project.display_name} ({project.name})
                    </option>
                  ))}
                {grantFormData.scope_type === 'WORKSPACE' &&
                  workspaces.map((workspace) => (
                    <option key={workspace.workspace_id} value={workspace.workspace_id}>
                      {workspace.name}
                    </option>
                  ))}
              </select>
              {formErrors.scope_id && (
                <span className={styles.errorText}>{formErrors.scope_id}</span>
              )}
            </div>

            {/* 主体类型 */}
            <div className={styles.formGroup}>
              <label className={styles.label}>
                主体类型<span className={styles.required}>*</span>
              </label>
              {isContextLocked ? (
                <input
                  type="text"
                  className={styles.input}
                  value={
                    grantFormData.principal_type === 'USER'
                      ? '用户'
                      : grantFormData.principal_type === 'TEAM'
                      ? '团队'
                      : '应用'
                  }
                  disabled
                />
              ) : (
                <select
                  className={styles.input}
                  value={grantFormData.principal_type}
                  onChange={(e) => {
                    const newPrincipalType = e.target.value as PrincipalType;
                    setGrantFormData({
                      ...grantFormData,
                      principal_type: newPrincipalType,
                      principal_id: 0,
                    });
                  }}
                >
                  <option value="USER">用户</option>
                  <option value="TEAM">团队</option>
                  <option value="APPLICATION">应用</option>
                </select>
              )}
            </div>

            {/* 主体选择 */}
            <div className={styles.formGroup}>
              <label className={styles.label}>
                {grantFormData.principal_type === 'USER'
                  ? '用户'
                  : grantFormData.principal_type === 'TEAM'
                  ? '团队'
                  : '应用'}
                <span className={styles.required}>*</span>
              </label>
              {isContextLocked ? (
                <input
                  type="text"
                  className={styles.input}
                  value={
                    grantFormData.principal_type === 'USER'
                      ? users.find(u => u.id === grantFormData.principal_id)?.username || `用户 #${grantFormData.principal_id}`
                      : grantFormData.principal_type === 'TEAM'
                      ? teams.find(t => t.id === grantFormData.principal_id)?.display_name || `团队 #${grantFormData.principal_id}`
                      : applications.find(a => a.id === grantFormData.principal_id)?.name || `应用 #${grantFormData.principal_id}`
                  }
                  disabled
                />
              ) : (
                <select
                  className={`${styles.input} ${formErrors.principal_id ? styles.error : ''}`}
                  value={grantFormData.principal_id || ''}
                  onChange={(e) =>
                    setGrantFormData({ ...grantFormData, principal_id: e.target.value })
                  }
                  disabled={loadingPrincipals}
                >
                  <option value="">
                    {loadingPrincipals
                      ? '加载中...'
                      : `请选择${
                          grantFormData.principal_type === 'USER'
                            ? '用户'
                            : grantFormData.principal_type === 'TEAM'
                            ? '团队'
                            : '应用'
                        }`}
                  </option>
                  {grantFormData.principal_type === 'USER' &&
                    users.map((user) => (
                      <option key={user.id} value={user.id}>
                        {user.username} ({user.email})
                      </option>
                    ))}
                  {grantFormData.principal_type === 'TEAM' &&
                    teams.map((team) => (
                      <option key={team.id} value={team.id}>
                        {team.display_name} ({team.name})
                      </option>
                    ))}
                  {grantFormData.principal_type === 'APPLICATION' &&
                    applications.map((app) => (
                      <option key={app.id} value={app.id}>
                        {app.name}
                      </option>
                    ))}
                </select>
              )}
              {formErrors.principal_id && (
                <span className={styles.errorText}>{formErrors.principal_id}</span>
              )}
            </div>
          </div>

          <div className={styles.formRow}>
            {/* 过期时间 */}
            <div className={styles.formGroup}>
              <label className={styles.label}>过期时间</label>
              <input
                type="datetime-local"
                className={styles.input}
                value={grantFormData.expires_at}
                onChange={(e) =>
                  setGrantFormData({ ...grantFormData, expires_at: e.target.value })
                }
              />
              <span className={styles.hint}>留空表示永久有效</span>
            </div>

            {/* 原因 */}
            <div className={styles.formGroup}>
              <label className={styles.label}>原因</label>
              <input
                type="text"
                className={styles.input}
                value={grantFormData.reason}
                onChange={(e) => setGrantFormData({ ...grantFormData, reason: e.target.value })}
                placeholder="授权原因（可选）"
              />
            </div>
          </div>
        </div>

        {/* 权限选择区域或角色选择 */}
        {grantType === 'permission' ? (
          <div className={styles.section}>
            <div className={styles.permissionsSectionHeader}>
              <h2 className={styles.sectionTitle}>
                选择权限<span className={styles.required}>*</span>
              </h2>
              <span className={styles.selectedCount}>
                已选择 {selectedPermissions.size} 个权限
              </span>
            </div>
            {formErrors.permissions && (
              <span className={styles.errorText}>{formErrors.permissions}</span>
            )}

            <div className={styles.permissionsGrid}>
              {Object.entries(groupedDefinitions).map(([resourceType, defs]) => (
                <div key={resourceType} className={styles.permissionGroup}>
                  <h3 className={styles.permissionGroupTitle}>{resourceType}</h3>
                  <div className={styles.permissionItems}>
                    {defs.map((def) => (
                      <div key={def.id} className={styles.permissionItem}>
                        <div className={styles.permissionInfo}>
                          <span className={styles.permissionName}>{def.display_name}</span>
                          <span className={styles.permissionDesc}>{def.description}</span>
                          {(() => {
                            const details = getPermissionLevelDetails(def.resource_type);
                            if (details) {
                              return (
                                <div className={styles.permissionLevelDetails}>
                                  <div className={styles.levelDetail}>
                                    <strong>READ:</strong> {details.read}
                                  </div>
                                  <div className={styles.levelDetail}>
                                    <strong>WRITE:</strong> {details.write}
                                  </div>
                                  <div className={styles.levelDetail}>
                                    <strong>ADMIN:</strong> {details.admin}
                                  </div>
                                </div>
                              );
                            }
                            return null;
                          })()}
                        </div>
                        <div className={styles.permissionLevels}>
                          {(['READ', 'WRITE', 'ADMIN'] as PermissionLevel[]).map((level) => (
                            <label
                              key={level}
                              className={`${styles.levelCheckbox} ${
                                selectedPermissions.get(def.id.toString()) === level ? styles.checked : ''
                              }`}
                            >
                              <input
                                type="checkbox"
                                checked={selectedPermissions.get(def.id.toString()) === level}
                                onChange={() => togglePermission(def.id.toString(), level)}
                              />
                              <span className={styles.levelLabel}>{level}</span>
                            </label>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div className={styles.section}>
            <h2 className={styles.sectionTitle}>
              选择角色<span className={styles.required}>*</span>
            </h2>
            <div className={styles.rolesGridHeader}>
              <span className={styles.selectedRoleCount}>
                已选择 {selectedRoleIds.size} 个角色
              </span>
            </div>
            <div className={styles.rolesGrid}>
              {roles.map((role) => (
                <label
                  key={role.id}
                  className={`${styles.roleCard} ${selectedRoleIds.has(role.id) ? styles.selected : ''}`}
                >
                  <input
                    type="checkbox"
                    checked={selectedRoleIds.has(role.id)}
                    onChange={() => toggleRole(role.id)}
                    className={styles.roleCheckbox}
                  />
                  <div className={styles.roleCardContent}>
                    <div className={styles.roleCardHeader}>
                      <span className={styles.roleCardTitle}>
                        {role.display_name}
                        {role.is_system && <span className={styles.systemBadge}>系统</span>}
                      </span>
                      <span className={styles.rolePolicyCount}>{role.policy_count} 个策略</span>
                    </div>
                  </div>
                </label>
              ))}
            </div>
          </div>
        )}

        {/* 表单按钮 */}
        <div className={styles.formActions}>
          {grantType === 'permission' && conflictWarning && (
            <div style={{ 
              flex: 1, 
              marginRight: '16px', 
              padding: '12px 16px', 
              background: '#f8d7da', 
              border: '1px solid #dc3545', 
              borderRadius: '6px',
              fontSize: '14px',
              color: '#721c24'
            }}>
               <strong>授权结果：</strong>{conflictWarning}
            </div>
          )}
          <button
            type="button"
            className={`${styles.button} ${styles.secondary}`}
            onClick={handleCancel}
            disabled={submitting}
          >
            取消
          </button>
          <button
            type="submit"
            className={`${styles.button} ${styles.primary}`}
            disabled={submitting || (grantType === 'permission' ? selectedPermissions.size === 0 : selectedRoleIds.size === 0)}
          >
            {submitting ? (grantType === 'permission' ? '授予中...' : '分配中...') : (grantType === 'permission' ? `授予 ${selectedPermissions.size} 个权限` : `分配 ${selectedRoleIds.size} 个角色`)}
          </button>
        </div>
      </form>
    </div>
  );
};

export default GrantPermission;
