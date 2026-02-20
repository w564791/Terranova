import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useToast } from '../../contexts/ToastContext';
import { iamService } from '../../services/iam';
import type { Team, TeamMember, PermissionGrant, PermissionDefinition, Organization, Project, PermissionLevel, ScopeType } from '../../services/iam';
import { workspaceService } from '../../services/workspaces';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './TeamDetail.module.css';

interface TeamToken {
  id: number;
  team_id: number;
  token_name: string;
  is_active: boolean;
  created_at: string;
  created_by: number;
  revoked_at?: string;
  revoked_by?: number;
  last_used_at?: string;
  expires_at?: string;
}

interface User {
  id: string;
  username: string;
  email: string;
}

interface MemberWithUser extends TeamMember {
  username?: string;
  email?: string;
}

const TeamDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  // 调试：确认showToast函数存在
  useEffect(() => {
    console.log('TeamDetail mounted, showToast:', typeof showToast);
  }, []);
  
  const [team, setTeam] = useState<Team | null>(null);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [tokens, setTokens] = useState<TeamToken[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [permissions, setPermissions] = useState<PermissionGrant[]>([]);
  const [roles, setRoles] = useState<any[]>([]);
  const [definitions, setDefinitions] = useState<PermissionDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingPermissions, setLoadingPermissions] = useState(false);
  const [activeTab, setActiveTab] = useState<'permissions' | 'roles'>('permissions');
  
  // 名称映射
  const [organizationNames, setOrganizationNames] = useState<Map<number, string>>(new Map());
  const [projectNames, setProjectNames] = useState<Map<number, string>>(new Map());
  const [workspaceNames, setWorkspaceNames] = useState<Map<string | number, string>>(new Map());
  
  // Token创建表单
  const [showTokenForm, setShowTokenForm] = useState(false);
  const [tokenName, setTokenName] = useState('');
  const [tokenNameError, setTokenNameError] = useState('');
  const [expiresIn, setExpiresIn] = useState<string>('30'); // 默认30天
  const [customDays, setCustomDays] = useState<number>(30);
  const [createdToken, setCreatedToken] = useState<string>('');
  
  // 成员添加表单
  const [showMemberForm, setShowMemberForm] = useState(false);
  const [selectedUserId, setSelectedUserId] = useState<string>('');
  const [memberRole, setMemberRole] = useState<'MEMBER' | 'MAINTAINER'>('MEMBER');
  const [userSearchTerm, setUserSearchTerm] = useState('');
  
  // 确认对话框
  const [revokeConfirm, setRevokeConfirm] = useState<{ show: boolean; token: TeamToken | null }>({
    show: false,
    token: null,
  });
  const [removeMemberConfirm, setRemoveMemberConfirm] = useState<{ show: boolean; member: TeamMember | null }>({
    show: false,
    member: null,
  });
  const [deleteTeamConfirm, setDeleteTeamConfirm] = useState(false);
  const [revokePermissionConfirm, setRevokePermissionConfirm] = useState<{ show: boolean; permission: PermissionGrant | null }>({
    show: false,
    permission: null,
  });
  const [revokeRoleConfirm, setRevokeRoleConfirm] = useState<{ show: boolean; role: any | null }>({
    show: false,
    role: null,
  });
  const [editingPermission, setEditingPermission] = useState<PermissionGrant | null>(null);
  const [savingPermission, setSavingPermission] = useState(false);

  // 加载团队详情
  const loadTeamDetail = async () => {
    if (!id) return;
    
    try {
      setLoading(true);
      const teamData = await iamService.getTeam(id);
      setTeam(teamData);
    } catch (error: any) {
      console.error('加载团队详情失败:', error);
      showToast(error.response?.data?.error || '加载团队详情失败', 'error');
      navigate('/iam/teams');
    } finally {
      setLoading(false);
    }
  };

  // 加载团队成员
  const loadMembers = async () => {
    if (!id) return;
    
    try {
      const response = await iamService.listTeamMembers(id);
      setMembers(response.members || []);
    } catch (error: any) {
      console.error('加载团队成员失败:', error);
      showToast(error.response?.data?.error || '加载团队成员失败', 'error');
    }
  };

  // 加载团队Tokens
  const loadTokens = async () => {
    if (!id) return;
    
    try {
      const response = await iamService.listTeamTokens(id);
      setTokens(response.tokens || []);
    } catch (error: any) {
      console.error('加载团队Tokens失败:', error);
      showToast(error.response?.data?.error || '加载团队Tokens失败', 'error');
    }
  };

  // 加载用户列表
  const loadUsers = async () => {
    try {
      const response = await iamService.listUsers({ limit: 100 });
      setUsers(response.users || []);
    } catch (error: any) {
      console.error('加载用户列表失败:', error);
    }
  };

  // 加载权限定义
  const loadDefinitions = async () => {
    try {
      const response = await iamService.listPermissionDefinitions();
      setDefinitions(response.definitions || []);
    } catch (error: any) {
      console.error('加载权限定义失败:', error);
    }
  };

  // 加载团队权限
  const loadTeamPermissions = async () => {
    if (!id || !team) return;
    
    try {
      setLoadingPermissions(true);
      
      // 使用新的API直接获取团队的所有权限
      const response = await fetch(`/api/v1/iam/teams/${id}/permissions`, {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });

      if (!response.ok) {
        throw new Error('Failed to fetch team permissions');
      }

      const data = await response.json();
      const teamPermissions = data.data || [];
      
      setPermissions(teamPermissions);

      // 收集需要获取名称的作用域ID
      const orgIds = new Set<number>();
      const projectIds = new Set<number>();
      const workspaceIds = new Set<string | number>();

      teamPermissions.forEach((perm: PermissionGrant) => {
        if (perm.scope_type === 'ORGANIZATION') {
          orgIds.add(perm.scope_id);
        } else if (perm.scope_type === 'PROJECT') {
          projectIds.add(perm.scope_id);
        } else if (perm.scope_type === 'WORKSPACE') {
          // 对于workspace，优先使用scope_id_str（语义化ID）
          const wsId = perm.scope_id_str || perm.scope_id;
          workspaceIds.add(wsId);
        }
      });

      // 并行获取所有需要的名称
      const orgNameMap = new Map<number, string>();
      const projNameMap = new Map<number, string>();
      const wsNameMap = new Map<string | number, string>();

      const namePromises: Promise<void>[] = [];

      // 获取组织名称
      orgIds.forEach(orgId => {
        namePromises.push(
          iamService.getOrganization(orgId)
            .then(org => {
              orgNameMap.set(orgId, org.display_name || org.name);
            })
            .catch(err => console.error(`获取组织 ${orgId} 失败:`, err))
        );
      });

      // 获取项目名称
      projectIds.forEach(projId => {
        namePromises.push(
          iamService.getProject(projId)
            .then(proj => {
              projNameMap.set(projId, proj.display_name || proj.name);
            })
            .catch(err => console.error(`获取项目 ${projId} 失败:`, err))
        );
      });

      // 获取工作空间名称
      workspaceIds.forEach(wsId => {
        namePromises.push(
          workspaceService.getWorkspace(wsId)
            .then(ws => {
              wsNameMap.set(wsId, ws.data.name);
            })
            .catch(err => console.error(`获取工作空间 ${wsId} 失败:`, err))
        );
      });

      // 等待所有名称获取完成
      await Promise.all(namePromises);

      // 更新名称映射状态
      setOrganizationNames(orgNameMap);
      setProjectNames(projNameMap);
      setWorkspaceNames(wsNameMap);
      
    } catch (error: any) {
      console.error('加载团队权限失败:', error);
      showToast(error.response?.data?.error || '加载团队权限失败', 'error');
    } finally {
      setLoadingPermissions(false);
    }
  };

  // 加载团队角色
  const loadTeamRoles = async () => {
    if (!id) return;
    
    try {
      const response = await fetch(`/api/v1/iam/teams/${id}/roles`, {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });
      if (response.ok) {
        const data = await response.json();
        setRoles(data.data || []);
      }
    } catch (error: any) {
      console.error('加载团队角色失败:', error);
    }
  };

  useEffect(() => {
    loadTeamDetail();
    loadMembers();
    loadTokens();
    loadUsers();
    loadDefinitions();
  }, [id]);

  useEffect(() => {
    if (team) {
      loadTeamPermissions();
      loadTeamRoles();
    }
  }, [team]);

  // 创建Token
  const handleCreateToken = async () => {
    if (!tokenName.trim()) {
      setTokenNameError('Token名称不能为空');
      return;
    }
    
    if (!/^[a-zA-Z0-9-_]+$/.test(tokenName.trim())) {
      setTokenNameError('Token名称只能包含字母、数字、连字符和下划线');
      return;
    }

    // 计算过期天数
    let expiresInDays = 0;
    if (expiresIn === 'custom') {
      expiresInDays = customDays;
    } else if (expiresIn !== 'never') {
      expiresInDays = parseInt(expiresIn);
    }

    try {
      const response = await iamService.createTeamToken(id!, tokenName.trim(), expiresInDays);
      setCreatedToken(response.token.token);
      setTokenName('');
      setTokenNameError('');
      setExpiresIn('30');
      setShowTokenForm(false);
      await loadTokens();
      showToast('Token创建成功，请立即复制', 'success');
    } catch (error: any) {
      setTokenNameError(error.response?.data?.error || '创建Token失败');
    }
  };

  // 获取用户名
  const getUserName = (userId: string | number): string => {
    const user = users.find(u => u.id === userId || u.id === userId.toString());
    return user ? user.username : `用户${userId}`;
  };

  // 复制Token
  const handleCopyToken = () => {
    navigator.clipboard.writeText(createdToken);
    showToast('Token已复制到剪贴板', 'success');
  };

  // 清除已创建的Token
  const handleClearCreatedToken = () => {
    setCreatedToken('');
  };

  // 吊销Token
  const handleRevokeToken = (token: TeamToken) => {
    setRevokeConfirm({ show: true, token });
  };

  const confirmRevokeToken = async () => {
    if (!revokeConfirm.token) return;

    try {
      await iamService.revokeTeamToken(id!, revokeConfirm.token.id);
      showToast('Token已吊销', 'success');
      setRevokeConfirm({ show: false, token: null });
      await loadTokens();
    } catch (error: any) {
      showToast(error.response?.data?.error || '吊销Token失败', 'error');
    }
  };

  // 添加成员
  const handleAddMember = async () => {
    if (!selectedUserId || selectedUserId === '') {
      showToast('请选择用户', 'error');
      return;
    }

    try {
      await iamService.addTeamMember(id!, {
        user_id: selectedUserId,
        role: memberRole,
      });
      showToast('成员添加成功', 'success');
      setShowMemberForm(false);
      setSelectedUserId('');
      setMemberRole('MEMBER');
      setUserSearchTerm('');
      await loadMembers();
    } catch (error: any) {
      showToast(error.response?.data?.error || '添加成员失败', 'error');
    }
  };

  // 移除成员
  const handleRemoveMember = (member: TeamMember) => {
    setRemoveMemberConfirm({ show: true, member });
  };

  const confirmRemoveMember = async () => {
    if (!removeMemberConfirm.member) return;

    try {
      await iamService.removeTeamMember(id!, removeMemberConfirm.member.user_id);
      showToast('成员移除成功', 'success');
      setRemoveMemberConfirm({ show: false, member: null });
      await loadMembers();
    } catch (error: any) {
      showToast(error.response?.data?.error || '移除成员失败', 'error');
    }
  };

  // 删除团队
  const handleDeleteTeam = () => {
    if (team?.is_system) {
      showToast('系统团队不能删除', 'error');
      return;
    }
    setDeleteTeamConfirm(true);
  };

  const confirmDeleteTeam = async () => {
    try {
      await iamService.deleteTeam(id!);
      showToast('团队删除成功', 'success');
      navigate('/iam/teams');
    } catch (error: any) {
      showToast(error.response?.data?.error || '删除团队失败', 'error');
    }
  };

  // 撤销权限
  const handleRevokePermission = (permission: PermissionGrant) => {
    setRevokePermissionConfirm({ show: true, permission });
  };

  const confirmRevokePermission = async () => {
    if (!revokePermissionConfirm.permission) return;

    try {
      await iamService.revokePermission(
        revokePermissionConfirm.permission.scope_type,
        revokePermissionConfirm.permission.id
      );
      showToast('权限撤销成功', 'success');
      setRevokePermissionConfirm({ show: false, permission: null });
      await loadTeamPermissions();
    } catch (error: any) {
      showToast(error.response?.data?.error || '撤销权限失败', 'error');
    }
  };

  // 编辑权限
  const handleEditPermission = (permission: PermissionGrant) => {
    setEditingPermission(permission);
  };

  // 保存权限编辑
  const handleSavePermissionEdit = async (newLevel: PermissionLevel) => {
    if (!editingPermission || savingPermission) return;

    try {
      setSavingPermission(true);
      
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
      await loadTeamPermissions();
    } catch (error: any) {
      showToast(error.response?.data?.error || '更新失败', 'error');
    } finally {
      setSavingPermission(false);
    }
  };

  // 撤销团队角色
  const handleRevokeRole = (role: any) => {
    setRevokeRoleConfirm({ show: true, role });
  };

  const confirmRevokeRole = async () => {
    if (!revokeRoleConfirm.role) return;

    try {
      const response = await fetch(
        `/api/v1/iam/teams/${id}/roles/${revokeRoleConfirm.role.id}`,
        {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('token')}`,
          },
        }
      );

      if (!response.ok) {
        throw new Error('Failed to revoke team role');
      }

      showToast('角色撤销成功', 'success');
      setRevokeRoleConfirm({ show: false, role: null });
      await loadTeamRoles();
    } catch (error: any) {
      showToast('撤销角色失败', 'error');
    }
  };

  // 获取权限定义名称
  const getPermissionName = (permissionId: number) => {
    const def = definitions.find((d) => d.id === permissionId);
    return def ? def.display_name : `权限 ${permissionId}`;
  };

  // 获取作用域显示名称
  const getScopeDisplayName = (scopeType: string, scopeID: number, scopeIDStr?: string) => {
    let name = '';
    switch (scopeType) {
      case 'ORGANIZATION':
        name = organizationNames.get(scopeID) || `组织 #${scopeID}`;
        break;
      case 'PROJECT':
        name = projectNames.get(scopeID) || `项目 #${scopeID}`;
        break;
      case 'WORKSPACE':
        // 对于workspace，优先使用scope_id_str查询名称
        const wsKey = scopeIDStr || scopeID;
        name = workspaceNames.get(wsKey) || `工作空间 #${scopeIDStr || scopeID}`;
        break;
      default:
        name = `${scopeType} #${scopeID}`;
    }
    return name;
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

  // 渲染角色徽章
  const renderRoleBadge = (role: string) => {
    if (role === 'MAINTAINER') {
      return <span className={`${styles.roleBadge} ${styles.maintainer}`}>Maintainer</span>;
    }
    return <span className={`${styles.roleBadge} ${styles.member}`}>Member</span>;
  };

  // 渲染Token状态徽章
  const renderTokenStatusBadge = (token: TeamToken) => {
    if (!token.is_active) {
      return <span className={`${styles.statusBadge} ${styles.revoked}`}>已吊销</span>;
    }
    if (token.expires_at && new Date(token.expires_at) < new Date()) {
      return <span className={`${styles.statusBadge} ${styles.expired}`}>已过期</span>;
    }
    return <span className={`${styles.statusBadge} ${styles.active}`}>有效</span>;
  };

  // 过滤用户列表
  const filteredUsers = users.filter(user => {
    const searchLower = userSearchTerm.toLowerCase();
    return (
      user.username.toLowerCase().includes(searchLower) ||
      user.email.toLowerCase().includes(searchLower) ||
      user.id.toString().includes(searchLower)
    );
  });

  if (loading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  if (!team) {
    return <div className={styles.error}>团队不存在</div>;
  }

  const activeTokenCount = tokens.filter(t => t.is_active).length;

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.header}>
        <button className={styles.backButton} onClick={() => navigate('/iam/teams')}>
          ← 返回团队列表
        </button>
        <div className={styles.headerContent}>
          <div>
            <h1 className={styles.title}>{team.display_name}</h1>
            <p className={styles.subtitle}>{team.name}</p>
          </div>
          {!team.is_system && (
            <button className={styles.deleteButton} onClick={handleDeleteTeam}>
              删除团队
            </button>
          )}
        </div>
      </div>

      {/* 团队信息卡片 */}
      <div className={styles.section}>
        <h2 className={styles.sectionTitle}>团队信息</h2>
        <div className={styles.infoCard}>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>团队ID:</span>
            <span className={styles.infoValue}>{team.id}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>团队标识:</span>
            <span className={styles.infoValue}>{team.name}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>显示名称:</span>
            <span className={styles.infoValue}>{team.display_name}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>描述:</span>
            <span className={styles.infoValue}>{team.description || '暂无描述'}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>创建时间:</span>
            <span className={styles.infoValue}>
              {new Date(team.created_at).toLocaleString('zh-CN')}
            </span>
          </div>
        </div>
      </div>

      {/* 团队成员 */}
      <div className={styles.section}>
        <div className={styles.sectionHeader}>
          <h2 className={styles.sectionTitle}>团队成员 ({members.length})</h2>
          <button
            className={styles.addButton}
            onClick={() => setShowMemberForm(!showMemberForm)}
          >
            {showMemberForm ? '取消' : '+ 添加成员'}
          </button>
        </div>

        {/* 添加成员表单 */}
        {showMemberForm && (
          <div className={styles.inlineForm}>
            <div className={styles.formRow}>
              <div className={styles.formField}>
                <label className={styles.formLabel}>选择用户</label>
                <div className={styles.selectWrapper}>
                  <input
                    type="text"
                    className={styles.searchInput}
                    placeholder="搜索用户名、邮箱或ID..."
                    value={userSearchTerm}
                    onChange={(e) => setUserSearchTerm(e.target.value)}
                  />
                  <select
                    className={styles.select}
                    value={selectedUserId}
                    onChange={(e) => setSelectedUserId(e.target.value)}
                    size={5}
                  >
                    <option value="">请选择用户</option>
                    {filteredUsers.map((user) => (
                      <option key={user.id} value={user.id}>
                        {user.username} ({user.email}) - ID: {user.id}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
              <div className={styles.formField}>
                <label className={styles.formLabel}>角色</label>
                <select
                  className={styles.input}
                  value={memberRole}
                  onChange={(e) => setMemberRole(e.target.value as 'MEMBER' | 'MAINTAINER')}
                >
                  <option value="MEMBER">Member</option>
                  <option value="MAINTAINER">Maintainer</option>
                </select>
              </div>
              <div className={styles.formActions}>
                <button
                  type="button"
                  className={styles.submitButton}
                  onClick={handleAddMember}
                >
                  添加
                </button>
              </div>
            </div>
          </div>
        )}

        {/* 成员列表 */}
        {members.length === 0 ? (
          <div className={styles.empty}>暂无成员</div>
        ) : (
          <div className={styles.membersList}>
            {members.map((member) => (
              <div key={member.id} className={styles.memberCard}>
                <div className={styles.memberInfo}>
                  <div className={styles.memberName}>{getUserName(member.user_id)}</div>
                  <div className={styles.memberUserId}>ID: {member.user_id}</div>
                </div>
                <div className={styles.memberActions}>
                  {renderRoleBadge(member.role)}
                  <button
                    className={styles.removeButton}
                    onClick={() => handleRemoveMember(member)}
                  >
                    移除
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 团队权限与角色 */}
      <div className={styles.section}>
        <div className={styles.sectionHeader}>
          <h2 className={styles.sectionTitle}>
            团队权限与角色 (权限: {permissions.length}, 角色: {roles.length})
          </h2>
          <button
            className={styles.addButton}
            onClick={() => navigate(`/iam/permissions/grant?principal_type=TEAM&principal_id=${id}`)}
          >
            + 新增授权
          </button>
        </div>

        {/* 标签页切换 */}
        <div className={styles.tabSelector}>
          <button
            className={`${styles.tabButton} ${activeTab === 'permissions' ? styles.active : ''}`}
            onClick={() => setActiveTab('permissions')}
          >
            直接授予的权限 ({permissions.length})
          </button>
          <button
            className={`${styles.tabButton} ${activeTab === 'roles' ? styles.active : ''}`}
            onClick={() => setActiveTab('roles')}
          >
            分配的角色 ({roles.length})
          </button>
        </div>

        <div className={styles.tokenHint}>
          {activeTab === 'permissions' 
            ? '团队权限会自动应用到所有团队成员。' 
            : '团队角色包含的权限会自动应用到所有团队成员。'}
        </div>

        {/* 权限标签页内容 */}
        {activeTab === 'permissions' && (
          <>
            {loadingPermissions ? (
              <div className={styles.loading}>加载权限中...</div>
            ) : permissions.length === 0 ? (
              <div className={styles.empty}>该团队暂无权限授予</div>
            ) : (
              <table className={styles.permissionsTable}>
                <thead>
                  <tr>
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
                  {permissions.map((permission) => (
                    <tr key={permission.id}>
                  <td>{renderScopeTypeBadge(permission.scope_type)}</td>
                  <td className={styles.scopeIdCell}>
                    {getScopeDisplayName(permission.scope_type, permission.scope_id, permission.scope_id_str)}
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
                          onClick={() => handleSavePermissionEdit(editingPermission.permission_level)}
                          disabled={savingPermission}
                        >
                          {savingPermission ? '保存中...' : '保存'}
                        </button>
                        <button
                          className={`${styles.actionButton} ${styles.cancel}`}
                          onClick={() => setEditingPermission(null)}
                          disabled={savingPermission}
                        >
                          取消
                        </button>
                      </div>
                    ) : (
                      <div className={styles.editActions}>
                        <button
                          className={`${styles.actionButton} ${styles.edit}`}
                          onClick={() => handleEditPermission(permission)}
                        >
                          编辑
                        </button>
                        <button
                          className={`${styles.actionButton} ${styles.revoke}`}
                          onClick={() => handleRevokePermission(permission)}
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
          </>
        )}

        {/* 角色标签页内容 */}
        {activeTab === 'roles' && (
          <>
            {loadingPermissions ? (
              <div className={styles.loading}>加载角色中...</div>
            ) : roles.length === 0 ? (
              <div className={styles.empty}>该团队暂无角色分配</div>
            ) : (
              <div className={styles.rolesList}>
                {roles.map((role) => (
                  <div key={role.id} className={styles.roleItem}>
                    <div className={styles.roleInfo}>
                      <div className={styles.roleHeader}>
                        <span className={styles.roleName}>{role.role_display_name}</span>
                        <span className={styles.roleScope}>
                          {renderScopeTypeBadge(role.scope_type)} {getScopeDisplayName(role.scope_type, role.scope_id)}
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
            )}
          </>
        )}
      </div>

      {/* 团队Tokens */}
      <div className={styles.section}>
        <div className={styles.sectionHeader}>
          <h2 className={styles.sectionTitle}>
            团队Tokens ({activeTokenCount}/2 有效)
          </h2>
          <button
            className={styles.addButton}
            onClick={() => {
              setShowTokenForm(!showTokenForm);
              setCreatedToken('');
            }}
            disabled={activeTokenCount >= 2 && !showTokenForm}
          >
            {showTokenForm ? '取消' : '+ 创建Token'}
          </button>
        </div>
        
        <div className={styles.tokenHint}>
          Token用于API认证，拥有团队的所有权限。每个团队最多2个有效Token。
        </div>

        {/* 创建Token表单 */}
        {showTokenForm && (
          <div className={styles.inlineForm}>
            <div className={styles.formRow}>
              <div className={styles.formField}>
                <label className={styles.formLabel}>Token名称</label>
                <input
                  type="text"
                  className={`${styles.input} ${tokenNameError ? styles.inputError : ''}`}
                  value={tokenName}
                  onChange={(e) => {
                    setTokenName(e.target.value);
                    setTokenNameError('');
                  }}
                  placeholder="例如：api-token-1"
                />
                {tokenNameError && <span className={styles.errorText}>{tokenNameError}</span>}
                {!tokenNameError && (
                  <span className={styles.hint}>字母、数字、连字符和下划线</span>
                )}
              </div>
              <div className={styles.formField}>
                <label className={styles.formLabel}>过期时间</label>
                <select
                  className={styles.input}
                  value={expiresIn}
                  onChange={(e) => setExpiresIn(e.target.value)}
                >
                  <option value="1">1天</option>
                  <option value="7">7天</option>
                  <option value="14">14天</option>
                  <option value="30">30天</option>
                  <option value="90">90天</option>
                  <option value="180">180天</option>
                  <option value="never">永不过期</option>
                  <option value="custom">自定义</option>
                </select>
                {expiresIn === 'custom' && (
                  <input
                    type="number"
                    className={styles.input}
                    value={customDays}
                    onChange={(e) => setCustomDays(Number(e.target.value))}
                    placeholder="输入天数"
                    min="1"
                  />
                )}
              </div>
              <div className={styles.formActions}>
                <button
                  type="button"
                  className={styles.submitButton}
                  onClick={handleCreateToken}
                >
                  创建
                </button>
              </div>
            </div>
          </div>
        )}

        {/* 显示创建的Token */}
        {createdToken && (
          <div className={styles.createdTokenBox}>
            <div className={styles.warningBox}>
              <strong> 重要提示：</strong>
              <p>此Token仅显示一次，请立即复制并妥善保管。</p>
            </div>
            <div className={styles.tokenDisplay}>
              <code className={styles.tokenCode}>{createdToken}</code>
              <button className={styles.copyButton} onClick={handleCopyToken}>
                复制
              </button>
              <button className={styles.clearButton} onClick={handleClearCreatedToken}>
                清除
              </button>
            </div>
          </div>
        )}

        {/* Token列表 */}
        {tokens.length === 0 ? (
          <div className={styles.empty}>暂无Token</div>
        ) : (
          <div className={styles.tokensList}>
            {tokens.map((token) => (
              <div key={token.id} className={styles.tokenCard}>
                <div className={styles.tokenHeader}>
                  <div className={styles.tokenName}>{token.token_name}</div>
                  {renderTokenStatusBadge(token)}
                </div>
                <div className={styles.tokenBody}>
                  <div className={styles.tokenInfo}>
                    <span className={styles.tokenLabel}>创建时间:</span>
                    <span className={styles.tokenValue}>
                      {new Date(token.created_at).toLocaleString('zh-CN')}
                    </span>
                  </div>
                  {token.last_used_at && (
                    <div className={styles.tokenInfo}>
                      <span className={styles.tokenLabel}>最后使用:</span>
                      <span className={styles.tokenValue}>
                        {new Date(token.last_used_at).toLocaleString('zh-CN')}
                      </span>
                    </div>
                  )}
                  <div className={styles.tokenInfo}>
                    <span className={styles.tokenLabel}>过期时间:</span>
                    <span className={styles.tokenValue}>
                      {token.expires_at 
                        ? new Date(token.expires_at).toLocaleString('zh-CN')
                        : '永不过期'}
                    </span>
                  </div>
                  {token.revoked_at && (
                    <div className={styles.tokenInfo}>
                      <span className={styles.tokenLabel}>吊销时间:</span>
                      <span className={styles.tokenValue}>
                        {new Date(token.revoked_at).toLocaleString('zh-CN')}
                      </span>
                    </div>
                  )}
                </div>
                {token.is_active && (
                  <div className={styles.tokenActions}>
                    <button
                      className={styles.revokeButton}
                      onClick={() => handleRevokeToken(token)}
                    >
                      吊销
                    </button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 吊销Token确认对话框 */}
      <ConfirmDialog
        isOpen={revokeConfirm.show}
        title="确认吊销Token"
        message={`确定要吊销Token "${revokeConfirm.token?.token_name}" 吗？吊销后将立即失效且无法恢复。`}
        onConfirm={confirmRevokeToken}
        onCancel={() => setRevokeConfirm({ show: false, token: null })}
        confirmText="吊销"
        cancelText="取消"
      />

      {/* 移除成员确认对话框 */}
      <ConfirmDialog
        isOpen={removeMemberConfirm.show}
        title="确认移除成员"
        message={`确定要移除用户 ${removeMemberConfirm.member?.user_id} 吗？`}
        onConfirm={confirmRemoveMember}
        onCancel={() => setRemoveMemberConfirm({ show: false, member: null })}
        confirmText="移除"
        cancelText="取消"
      />

      {/* 删除团队确认对话框 */}
      <ConfirmDialog
        isOpen={deleteTeamConfirm}
        title="确认删除团队"
        message={`确定要删除团队 "${team.display_name}" 吗？删除后，该团队的所有权限授予将被撤销，此操作不可恢复。`}
        onConfirm={confirmDeleteTeam}
        onCancel={() => setDeleteTeamConfirm(false)}
        confirmText="删除"
        cancelText="取消"
      />

      {/* 撤销权限确认对话框 */}
      <ConfirmDialog
        isOpen={revokePermissionConfirm.show}
        title="确认撤销权限"
        message="确定要撤销此权限吗？撤销后，团队成员将立即失去相应的访问权限。"
        onConfirm={confirmRevokePermission}
        onCancel={() => setRevokePermissionConfirm({ show: false, permission: null })}
        confirmText="撤销"
        cancelText="取消"
      />

      {/* 撤销角色确认对话框 */}
      <ConfirmDialog
        isOpen={revokeRoleConfirm.show}
        title="确认撤销角色"
        message={`确定要撤销角色"${revokeRoleConfirm.role?.role_display_name}"吗？撤销后，团队成员将失去该角色的所有权限。`}
        onConfirm={confirmRevokeRole}
        onCancel={() => setRevokeRoleConfirm({ show: false, role: null })}
        confirmText="撤销"
        cancelText="取消"
      />
    </div>
  );
};

export default TeamDetail;
