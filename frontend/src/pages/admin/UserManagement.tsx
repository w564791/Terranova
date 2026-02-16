import React, { useState, useEffect } from 'react';
import { iamService } from '../../services/iam';
import { useToast } from '../../contexts/ToastContext';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './UserManagement.module.css';

const UserManagement: React.FC = () => {
  const { success: showSuccess, error: showError } = useToast();
  const [users, setUsers] = useState<any[]>([]);
  const [stats, setStats] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<boolean | undefined>(undefined);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [createForm, setCreateForm] = useState({
    username: '',
    email: '',
    password: '',
  });
  const [deleteConfirm, setDeleteConfirm] = useState<{
    show: boolean;
    user: any;
  }>({ show: false, user: null });

  useEffect(() => {
    loadStats();
    loadUsers();
  }, [statusFilter]);

  const loadStats = async () => {
    try {
      const data = await iamService.getUserStats();
      setStats(data);
    } catch (error: any) {
      console.error('加载统计失败:', error);
    }
  };

  const loadUsers = async () => {
    setLoading(true);
    try {
      const params: any = { limit: 50 };
      if (statusFilter !== undefined) params.is_active = statusFilter;
      if (search) params.search = search;

      const data = await iamService.listUsers(params);
      setUsers(data.users || []);
    } catch (error: any) {
      showError('加载用户失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = () => {
    loadUsers();
  };

  const handleToggleStatus = async (user: any) => {
    try {
      if (user.is_active) {
        await iamService.deactivateUser(user.id);
      } else {
        await iamService.activateUser(user.id);
      }
      showSuccess(`用户已${user.is_active ? '停用' : '激活'}`);
      loadUsers();
      loadStats();
    } catch (error: any) {
      showError('操作失败: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleCreateUser = async () => {
    if (!createForm.username || !createForm.email || !createForm.password) {
      showError('请填写所有必填字段');
      return;
    }

    try {
      await iamService.createUser(createForm);
      showSuccess('用户创建成功');
      setShowCreateModal(false);
      setCreateForm({ username: '', email: '', password: '' });
      loadUsers();
      loadStats();
    } catch (error: any) {
      showError('创建失败: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleDeleteUser = (user: any) => {
    setDeleteConfirm({ show: true, user });
  };

  const confirmDeleteUser = async () => {
    const { user } = deleteConfirm;
    try {
      await iamService.deleteUser(user.id);
      showSuccess('用户删除成功');
      loadUsers();
      loadStats();
    } catch (error: any) {
      showError('删除失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setDeleteConfirm({ show: false, user: null });
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1>用户管理</h1>
        <button onClick={() => setShowCreateModal(true)} className={styles.createButton}>
          新增用户
        </button>
      </div>

      {stats && (
        <div className={styles.statsBar}>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{stats.total}</div>
            <div className={styles.statLabel}>总用户数</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{stats.active}</div>
            <div className={styles.statLabel}>活跃用户</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{stats.inactive}</div>
            <div className={styles.statLabel}>停用用户</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{stats.system_admin_count}</div>
            <div className={styles.statLabel}>系统管理员</div>
          </div>
        </div>
      )}

      <div className={styles.filters}>
        <div className={styles.filterGroup}>
          <label>状态:</label>
          <select
            value={statusFilter === undefined ? 'all' : statusFilter ? 'active' : 'inactive'}
            onChange={(e) => {
              const val = e.target.value;
              setStatusFilter(val === 'all' ? undefined : val === 'active');
            }}
            className={styles.select}
          >
            <option value="all">全部</option>
            <option value="active">活跃</option>
            <option value="inactive">停用</option>
          </select>
        </div>

        <div className={styles.searchBox}>
          <input
            type="text"
            placeholder="搜索用户名或邮箱..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyPress={(e) => e.key === 'Enter' && handleSearch()}
            className={styles.searchInput}
          />
          <button onClick={handleSearch} className={styles.searchButton}>
            搜索
          </button>
        </div>
      </div>

      {loading && <div className={styles.loading}>加载中...</div>}

      {!loading && users.length === 0 && (
        <div className={styles.emptyState}>
          <p>暂无用户</p>
        </div>
      )}

      {!loading && users.length > 0 && (
        <div className={styles.tableContainer}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>ID</th>
                <th>用户名</th>
                <th>邮箱</th>
                <th>状态</th>
                <th>系统管理员</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>{user.id}</td>
                  <td className={styles.usernameCell}>{user.username}</td>
                  <td>{user.email}</td>
                  <td>
                    <span className={user.is_active ? styles.statusActive : styles.statusInactive}>
                      {user.is_active ? '活跃' : '停用'}
                    </span>
                  </td>
                  <td>
                    {user.is_system_admin && (
                      <span className={styles.statusActive}>是</span>
                    )}
                  </td>
                  <td>{new Date(user.created_at).toLocaleString()}</td>
                  <td className={styles.actions}>
                    <button
                      onClick={() => handleToggleStatus(user)}
                      className={user.is_active ? styles.deactivateButton : styles.activateButton}
                    >
                      {user.is_active ? '停用' : '激活'}
                    </button>
                    <button
                      onClick={() => handleDeleteUser(user)}
                      className={styles.deleteButton}
                    >
                      删除
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className={styles.info}>
        <h3>用户管理说明</h3>
        <ul>
          <li><strong>权限管理</strong>: 请在 IAM 权限管理页面中为用户分配角色和权限</li>
          <li><strong>状态管理</strong>: 可以激活或停用用户账号</li>
          <li><strong>搜索功能</strong>: 支持按用户名或邮箱搜索</li>
        </ul>
      </div>

      {/* 创建用户模态框 */}
      {showCreateModal && (
        <div className={styles.modal}>
          <div className={styles.modalContent}>
            <h2>新增用户</h2>
            <div className={styles.formGroup}>
              <label>用户名 *</label>
              <input
                type="text"
                value={createForm.username}
                onChange={(e) => setCreateForm({ ...createForm, username: e.target.value })}
                placeholder="请输入用户名"
              />
            </div>
            <div className={styles.formGroup}>
              <label>邮箱 *</label>
              <input
                type="email"
                value={createForm.email}
                onChange={(e) => setCreateForm({ ...createForm, email: e.target.value })}
                placeholder="请输入邮箱"
              />
            </div>
            <div className={styles.formGroup}>
              <label>密码 *</label>
              <input
                type="password"
                value={createForm.password}
                onChange={(e) => setCreateForm({ ...createForm, password: e.target.value })}
                placeholder="请输入密码（至少6位）"
              />
            </div>
            <div className={styles.modalActions}>
              <button onClick={handleCreateUser} className={styles.confirmButton}>
                创建
              </button>
              <button
                onClick={() => {
                  setShowCreateModal(false);
                  setCreateForm({ username: '', email: '', password: '' });
                }}
                className={styles.cancelButton}
              >
                取消
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 删除用户确认对话框 */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="确认删除用户"
        message={deleteConfirm.user ? `确定要删除用户 "${deleteConfirm.user.username}" 吗？此操作不可恢复。` : ''}
        confirmText="删除"
        cancelText="取消"
        type="danger"
        onConfirm={confirmDeleteUser}
        onCancel={() => setDeleteConfirm({ show: false, user: null })}
      />
    </div>
  );
};

export default UserManagement;
