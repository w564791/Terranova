import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useToast } from '../../hooks/useToast';
import { iamService } from '../../services/iam';
import type {
  Team,
  TeamMember,
  Organization,
  CreateTeamRequest,
  AddTeamMemberRequest,
} from '../../services/iam';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './TeamManagement.module.css';

const TeamManagement: React.FC = () => {
  const navigate = useNavigate();
  const [teams, setTeams] = useState<Team[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [showMemberDialog, setShowMemberDialog] = useState(false);
  const [editingTeam, setEditingTeam] = useState<Team | null>(null);
  const [selectedTeam, setSelectedTeam] = useState<Team | null>(null);
  const [teamMembers, setTeamMembers] = useState<TeamMember[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedOrgId, setSelectedOrgId] = useState<number | 'all'>('all');
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; team: Team | null }>({
    show: false,
    team: null,
  });
  const [removeMemberConfirm, setRemoveMemberConfirm] = useState<{
    show: boolean;
    member: TeamMember | null;
  }>({
    show: false,
    member: null,
  });
  const { showToast } = useToast();

  // 表单状态
  const [formData, setFormData] = useState({
    org_id: 0,
    name: '',
    display_name: '',
    description: '',
  });

  const [memberFormData, setMemberFormData] = useState({
    user_id: 0,
    role: 'MEMBER' as 'MEMBER' | 'MAINTAINER',
  });

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  // 加载组织列表
  const loadOrganizations = async () => {
    try {
      const response = await iamService.listOrganizations(true);
      setOrganizations(response.organizations || []);
      if (response.organizations && response.organizations.length > 0) {
        setSelectedOrgId(response.organizations[0].id);
      }
    } catch (error: any) {
      console.error('加载组织列表失败:', error);
      showToast(error.response?.data?.error || '加载组织列表失败', 'error');
    }
  };

  // 加载团队列表
  const loadTeams = async (orgId: number) => {
    try {
      setLoading(true);
      const response = await iamService.listTeams(orgId);
      setTeams(response.teams || []);
    } catch (error: any) {
      console.error('加载团队列表失败:', error);
      showToast(error.response?.data?.error || '加载团队列表失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  // 加载团队成员
  const loadTeamMembers = async (teamId: number) => {
    try {
      const response = await iamService.listTeamMembers(teamId);
      setTeamMembers(response.members || []);
    } catch (error: any) {
      console.error('加载团队成员失败:', error);
      showToast(error.response?.data?.error || '加载团队成员失败', 'error');
    }
  };

  useEffect(() => {
    loadOrganizations();
  }, []);

  useEffect(() => {
    if (selectedOrgId !== 'all' && selectedOrgId > 0) {
      loadTeams(selectedOrgId);
    } else {
      setTeams([]);
      setLoading(false);
    }
  }, [selectedOrgId]);

  // 打开添加对话框
  const handleAdd = () => {
    if (selectedOrgId === 'all' || selectedOrgId === 0) {
      showToast('请先选择一个组织', 'error');
      return;
    }
    setEditingTeam(null);
    setFormData({
      org_id: selectedOrgId as number,
      name: '',
      display_name: '',
      description: '',
    });
    setFormErrors({});
    setShowDialog(true);
  };

  // 打开编辑对话框
  const handleEdit = (team: Team) => {
    setEditingTeam(team);
    setFormData({
      org_id: team.org_id,
      name: team.name,
      display_name: team.display_name,
      description: team.description || '',
    });
    setFormErrors({});
    setShowDialog(true);
  };

  // 打开成员管理对话框
  const handleManageMembers = async (team: Team) => {
    setSelectedTeam(team);
    await loadTeamMembers(team.id);
    setMemberFormData({
      user_id: 0,
      role: 'MEMBER',
    });
    setShowMemberDialog(true);
  };

  // 验证表单
  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    if (!formData.org_id || formData.org_id === 0) {
      errors.org_id = '请选择组织';
    }

    if (!formData.name.trim()) {
      errors.name = '团队标识不能为空';
    } else if (!/^[a-z0-9-]+$/.test(formData.name.trim())) {
      errors.name = '团队标识只能包含小写字母、数字和连字符';
    }

    if (!formData.display_name.trim()) {
      errors.display_name = '显示名称不能为空';
    } else if (formData.display_name.trim().length < 2) {
      errors.display_name = '显示名称至少2个字符';
    } else if (formData.display_name.trim().length > 100) {
      errors.display_name = '显示名称不能超过100个字符';
    }

    if (formData.description && formData.description.length > 500) {
      errors.description = '描述不能超过500个字符';
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // 提交表单
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    try {
      if (editingTeam) {
        // 注意：团队没有更新API，只能删除后重建
        showToast('团队信息创建后不可修改', 'error');
      } else {
        // 创建
        const createData: CreateTeamRequest = {
          org_id: formData.org_id,
          name: formData.name,
          display_name: formData.display_name,
          description: formData.description || undefined,
        };
        await iamService.createTeam(createData);
        showToast('团队创建成功', 'success');
      }

      setShowDialog(false);
      if (selectedOrgId !== 'all') {
        loadTeams(selectedOrgId as number);
      }
    } catch (error: any) {
      showToast(error.response?.data?.error || '操作失败', 'error');
    }
  };

  // 删除团队
  const handleDelete = (team: Team) => {
    if (team.is_system) {
      showToast('系统团队不能删除', 'error');
      return;
    }
    setDeleteConfirm({ show: true, team });
  };

  const confirmDelete = async () => {
    if (!deleteConfirm.team) return;

    try {
      await iamService.deleteTeam(deleteConfirm.team.id);
      showToast('团队删除成功', 'success');
      setDeleteConfirm({ show: false, team: null });
      if (selectedOrgId !== 'all') {
        loadTeams(selectedOrgId as number);
      }
    } catch (error: any) {
      showToast(error.response?.data?.error || '删除失败', 'error');
    }
  };

  // 添加成员
  const handleAddMember = async () => {
    if (!selectedTeam) return;

    if (!memberFormData.user_id || memberFormData.user_id === 0) {
      showToast('请输入用户ID', 'error');
      return;
    }

    try {
      const request: AddTeamMemberRequest = {
        user_id: memberFormData.user_id,
        role: memberFormData.role,
      };
      await iamService.addTeamMember(selectedTeam.id, request);
      showToast('成员添加成功', 'success');
      setMemberFormData({ user_id: 0, role: 'MEMBER' });
      await loadTeamMembers(selectedTeam.id);
    } catch (error: any) {
      showToast(error.response?.data?.error || '添加成员失败', 'error');
    }
  };

  // 移除成员
  const handleRemoveMember = (member: TeamMember) => {
    setRemoveMemberConfirm({ show: true, member });
  };

  const confirmRemoveMember = async () => {
    if (!removeMemberConfirm.member || !selectedTeam) return;

    try {
      await iamService.removeTeamMember(selectedTeam.id, removeMemberConfirm.member.user_id);
      showToast('成员移除成功', 'success');
      setRemoveMemberConfirm({ show: false, member: null });
      await loadTeamMembers(selectedTeam.id);
    } catch (error: any) {
      showToast(error.response?.data?.error || '移除成员失败', 'error');
    }
  };

  // 过滤团队
  const filteredTeams = teams.filter((team) => {
    const matchesSearch =
      team.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      team.display_name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (team.description && team.description.toLowerCase().includes(searchTerm.toLowerCase()));

    return matchesSearch;
  });

  // 渲染团队类型徽章
  const renderTeamTypeBadge = (team: Team) => {
    if (team.is_system) {
      return <span className={`${styles.typeBadge} ${styles.system}`}>System</span>;
    }
    return <span className={`${styles.typeBadge} ${styles.custom}`}>Custom</span>;
  };

  // 渲染角色徽章
  const renderRoleBadge = (role: string) => {
    if (role === 'MAINTAINER') {
      return <span className={`${styles.roleBadge} ${styles.maintainer}`}>Maintainer</span>;
    }
    return <span className={`${styles.roleBadge} ${styles.member}`}>Member</span>;
  };

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.header}>
        <h1 className={styles.title}>团队管理</h1>
        <p className={styles.description}>
          管理组织下的团队，团队用于组织用户并统一授予权限，简化权限管理。
        </p>
      </div>

      {/* 操作栏 */}
      <div className={styles.actions}>
        <div className={styles.filters}>
          <select
            className={styles.orgSelect}
            value={selectedOrgId}
            onChange={(e) => setSelectedOrgId(e.target.value === 'all' ? 'all' : Number(e.target.value))}
          >
            <option value="all">选择组织</option>
            {organizations.map((org) => (
              <option key={org.id} value={org.id}>
                {org.display_name}
              </option>
            ))}
          </select>
          <input
            type="text"
            className={styles.searchInput}
            placeholder="搜索团队名称或描述..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
        <button className={styles.addButton} onClick={handleAdd}>
          + 添加团队
        </button>
      </div>

      {/* 团队列表 */}
      <div className={styles.teamsList}>
        {selectedOrgId === 'all' ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>请选择一个组织</div>
            <div className={styles.emptyHint}>从上方下拉菜单中选择一个组织以查看其团队</div>
          </div>
        ) : loading ? (
          <div className={styles.loading}>加载中...</div>
        ) : filteredTeams.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>
              {searchTerm ? '没有找到匹配的团队' : '暂无团队'}
            </div>
            <div className={styles.emptyHint}>
              {searchTerm ? '尝试调整搜索条件' : '点击"添加团队"按钮创建第一个团队'}
            </div>
          </div>
        ) : (
          <div className={styles.teamsGrid}>
            {filteredTeams.map((team) => (
              <Link 
                key={team.id} 
                to={`/iam/teams/${team.id}`}
                className={styles.teamCard}
              >
                <div className={styles.cardHeader}>
                  <div className={styles.teamInfo}>
                    <div className={styles.teamName}>{team.display_name}</div>
                    <div className={styles.teamId}>{team.name}</div>
                  </div>
                  {renderTeamTypeBadge(team)}
                </div>

                <div className={styles.cardBody}>
                  <p className={styles.teamDescription}>{team.description || '暂无描述'}</p>
                </div>

                <div className={styles.cardFooter}>
                  <div className={styles.teamMeta}>
                    <span className={styles.metaItem}>
                      创建于 {new Date(team.created_at).toLocaleDateString('zh-CN')}
                    </span>
                  </div>
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>

      {/* 添加/编辑对话框 */}
      {showDialog && (
        <div className={styles.dialog} onClick={() => setShowDialog(false)}>
          <div className={styles.dialogContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.dialogHeader}>
              <h2 className={styles.dialogTitle}>{editingTeam ? '编辑团队' : '添加团队'}</h2>
            </div>

            <form onSubmit={handleSubmit}>
              <div className={styles.dialogBody}>
                {/* 所属组织 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    所属组织<span className={styles.required}>*</span>
                  </label>
                  <select
                    className={`${styles.input} ${formErrors.org_id ? styles.error : ''}`}
                    value={formData.org_id}
                    onChange={(e) => setFormData({ ...formData, org_id: Number(e.target.value) })}
                    disabled={!!editingTeam}
                  >
                    <option value={0}>请选择组织</option>
                    {organizations.map((org) => (
                      <option key={org.id} value={org.id}>
                        {org.display_name}
                      </option>
                    ))}
                  </select>
                  {formErrors.org_id && <span className={styles.errorText}>{formErrors.org_id}</span>}
                </div>

                {/* 团队标识 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    团队标识<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.name ? styles.error : ''}`}
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    placeholder="例如：developers"
                    disabled={!!editingTeam}
                  />
                  {formErrors.name && <span className={styles.errorText}>{formErrors.name}</span>}
                  {!formErrors.name && (
                    <span className={styles.hint}>小写字母、数字和连字符，创建后不可修改</span>
                  )}
                </div>

                {/* 显示名称 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    显示名称<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.display_name ? styles.error : ''}`}
                    value={formData.display_name}
                    onChange={(e) => setFormData({ ...formData, display_name: e.target.value })}
                    placeholder="例如：开发团队"
                  />
                  {formErrors.display_name && (
                    <span className={styles.errorText}>{formErrors.display_name}</span>
                  )}
                  {!formErrors.display_name && <span className={styles.hint}>2-100个字符</span>}
                </div>

                {/* 描述 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>描述</label>
                  <textarea
                    className={`${styles.textarea} ${formErrors.description ? styles.error : ''}`}
                    value={formData.description}
                    onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                    placeholder="团队的简要描述（可选）"
                    rows={3}
                  />
                  {formErrors.description && (
                    <span className={styles.errorText}>{formErrors.description}</span>
                  )}
                  {!formErrors.description && <span className={styles.hint}>最多500个字符</span>}
                </div>
              </div>

              <div className={styles.dialogFooter}>
                <button
                  type="button"
                  className={`${styles.button} ${styles.secondary}`}
                  onClick={() => setShowDialog(false)}
                >
                  取消
                </button>
                <button type="submit" className={`${styles.button} ${styles.primary}`}>
                  创建
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 成员管理对话框 */}
      {showMemberDialog && selectedTeam && (
        <div className={styles.dialog} onClick={() => setShowMemberDialog(false)}>
          <div
            className={`${styles.dialogContent} ${styles.wide}`}
            onClick={(e) => e.stopPropagation()}
          >
            <div className={styles.dialogHeader}>
              <h2 className={styles.dialogTitle}>
                管理团队成员 - {selectedTeam.display_name}
              </h2>
            </div>

            <div className={styles.dialogBody}>
              {/* 添加成员表单 */}
              <div className={styles.addMemberForm}>
                <div className={styles.formRow}>
                  <div className={styles.formGroup}>
                    <label className={styles.label}>用户ID</label>
                    <input
                      type="number"
                      className={styles.input}
                      value={memberFormData.user_id || ''}
                      onChange={(e) =>
                        setMemberFormData({ ...memberFormData, user_id: Number(e.target.value) })
                      }
                      placeholder="输入用户ID"
                    />
                  </div>
                  <div className={styles.formGroup}>
                    <label className={styles.label}>角色</label>
                    <select
                      className={styles.input}
                      value={memberFormData.role}
                      onChange={(e) =>
                        setMemberFormData({
                          ...memberFormData,
                          role: e.target.value as 'MEMBER' | 'MAINTAINER',
                        })
                      }
                    >
                      <option value="MEMBER">Member</option>
                      <option value="MAINTAINER">Maintainer</option>
                    </select>
                  </div>
                  <button
                    type="button"
                    className={`${styles.button} ${styles.primary}`}
                    onClick={handleAddMember}
                  >
                    添加
                  </button>
                </div>
              </div>

              {/* 成员列表 */}
              <div className={styles.membersList}>
                <h3 className={styles.membersTitle}>当前成员 ({teamMembers.length})</h3>
                {teamMembers.length === 0 ? (
                  <div className={styles.emptyMembers}>暂无成员</div>
                ) : (
                  <table className={styles.membersTable}>
                    <thead>
                      <tr>
                        <th>用户ID</th>
                        <th>角色</th>
                        <th>加入时间</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {teamMembers.map((member) => (
                        <tr key={member.id}>
                          <td>{member.user_id}</td>
                          <td>{renderRoleBadge(member.role)}</td>
                          <td className={styles.dateCell}>
                            {new Date(member.joined_at).toLocaleDateString('zh-CN')}
                          </td>
                          <td>
                            <button
                              className={`${styles.actionButton} ${styles.delete}`}
                              onClick={() => handleRemoveMember(member)}
                            >
                              移除
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </div>

            <div className={styles.dialogFooter}>
              <button
                type="button"
                className={`${styles.button} ${styles.secondary}`}
                onClick={() => setShowMemberDialog(false)}
              >
                关闭
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 删除确认对话框 */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="确认删除"
        message={`确定要删除团队 ${deleteConfirm.team?.display_name} 吗？删除后，该团队的所有权限授予将被撤销。`}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm({ show: false, team: null })}
        confirmText="删除"
        cancelText="取消"
      />

      {/* 移除成员确认对话框 */}
      <ConfirmDialog
        isOpen={removeMemberConfirm.show}
        title="确认移除"
        message={`确定要移除用户 ${removeMemberConfirm.member?.user_id} 吗？`}
        onConfirm={confirmRemoveMember}
        onCancel={() => setRemoveMemberConfirm({ show: false, member: null })}
        confirmText="移除"
        cancelText="取消"
      />
    </div>
  );
};

export default TeamManagement;
