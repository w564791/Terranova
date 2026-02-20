import React, { useState, useEffect } from 'react';
import { useToast } from '../../hooks/useToast';
import { iamService } from '../../services/iam';
import type {
  Project,
  Organization,
  CreateProjectRequest,
  UpdateProjectRequest,
} from '../../services/iam';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './ProjectManagement.module.css';

const ProjectManagement: React.FC = () => {
  const [projects, setProjects] = useState<Project[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [editingProject, setEditingProject] = useState<Project | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedOrgId, setSelectedOrgId] = useState<number | 'all'>('all');
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all');
  const [deleteConfirm, setDeleteConfirm] = useState<{ show: boolean; project: Project | null }>({
    show: false,
    project: null,
  });
  const { showToast } = useToast();

  // 表单状态
  const [formData, setFormData] = useState({
    org_id: 0,
    name: '',
    display_name: '',
    description: '',
    is_active: true,
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

  // 加载项目列表
  const loadProjects = async (orgId: number) => {
    try {
      setLoading(true);
      const response = await iamService.listProjects(orgId);
      setProjects(response.projects || []);
    } catch (error: any) {
      console.error('加载项目列表失败:', error);
      showToast(error.response?.data?.error || '加载项目列表失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadOrganizations();
  }, []);

  useEffect(() => {
    if (selectedOrgId !== 'all' && selectedOrgId > 0) {
      loadProjects(selectedOrgId);
    } else {
      setProjects([]);
      setLoading(false);
    }
  }, [selectedOrgId]);

  // 打开添加对话框
  const handleAdd = () => {
    if (selectedOrgId === 'all' || selectedOrgId === 0) {
      showToast('请先选择一个组织', 'error');
      return;
    }
    setEditingProject(null);
    setFormData({
      org_id: selectedOrgId as number,
      name: '',
      display_name: '',
      description: '',
      is_active: true,
    });
    setFormErrors({});
    setShowDialog(true);
  };

  // 打开编辑对话框
  const handleEdit = (project: Project) => {
    setEditingProject(project);
    setFormData({
      org_id: project.org_id,
      name: project.name,
      display_name: project.display_name,
      description: project.description || '',
      is_active: project.is_active,
    });
    setFormErrors({});
    setShowDialog(true);
  };

  // 验证表单
  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    if (!formData.org_id || formData.org_id === 0) {
      errors.org_id = '请选择组织';
    }

    if (!formData.name.trim()) {
      errors.name = '项目标识不能为空';
    } else if (!/^[a-z0-9-]+$/.test(formData.name.trim())) {
      errors.name = '项目标识只能包含小写字母、数字和连字符';
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
      if (editingProject) {
        // 更新
        const updateData: UpdateProjectRequest = {
          display_name: formData.display_name,
          description: formData.description || undefined,
          is_active: formData.is_active,
        };
        await iamService.updateProject(editingProject.id, updateData);
        showToast('项目更新成功', 'success');
      } else {
        // 创建
        const createData: CreateProjectRequest = {
          org_id: formData.org_id,
          name: formData.name,
          display_name: formData.display_name,
          description: formData.description || undefined,
        };
        await iamService.createProject(createData);
        showToast('项目创建成功', 'success');
      }

      setShowDialog(false);
      if (selectedOrgId !== 'all') {
        loadProjects(selectedOrgId as number);
      }
    } catch (error: any) {
      showToast(error.response?.data?.error || '操作失败', 'error');
    }
  };

  // 切换项目状态
  const handleToggleStatus = async (project: Project) => {
    try {
      const updateData: UpdateProjectRequest = {
        display_name: project.display_name,
        description: project.description,
        is_active: !project.is_active,
      };
      await iamService.updateProject(project.id, updateData);
      showToast(`项目已${!project.is_active ? '启用' : '停用'}`, 'success');
      if (selectedOrgId !== 'all') {
        loadProjects(selectedOrgId as number);
      }
    } catch (error: any) {
      showToast(error.response?.data?.error || '操作失败', 'error');
    }
  };

  // 删除项目
  const handleDelete = (project: Project) => {
    setDeleteConfirm({ show: true, project });
  };

  const confirmDelete = async () => {
    if (!deleteConfirm.project) return;

    try {
      await iamService.deleteProject(deleteConfirm.project.id);
      showToast('项目删除成功', 'success');
      setDeleteConfirm({ show: false, project: null });
      if (selectedOrgId !== 'all') {
        loadProjects(selectedOrgId as number);
      }
    } catch (error: any) {
      showToast(error.response?.data?.error || '删除失败', 'error');
    }
  };

  // 过滤项目
  const filteredProjects = projects.filter((project) => {
    const matchesSearch =
      project.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      project.display_name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (project.description && project.description.toLowerCase().includes(searchTerm.toLowerCase()));

    const matchesStatus =
      statusFilter === 'all' ||
      (statusFilter === 'active' && project.is_active) ||
      (statusFilter === 'inactive' && !project.is_active);

    return matchesSearch && matchesStatus;
  });

  // 渲染状态徽章
  const renderStatusBadge = (project: Project) => {
    if (project.is_default) {
      return <span className={`${styles.statusBadge} ${styles.default}`}>Default</span>;
    }
    if (project.is_active) {
      return <span className={`${styles.statusBadge} ${styles.active}`}>Active</span>;
    }
    return <span className={`${styles.statusBadge} ${styles.inactive}`}>Inactive</span>;
  };

  // 获取组织名称
  const getOrgName = (orgId: number) => {
    const org = organizations.find((o) => o.id === orgId);
    return org ? org.display_name : `组织 ${orgId}`;
  };

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.header}>
        <h1 className={styles.title}>项目管理</h1>
        <p className={styles.description}>
          管理组织下的项目，项目是工作空间的容器，用于组织和管理相关的基础设施资源。
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
            placeholder="搜索项目名称或描述..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
          <select
            className={styles.filterSelect}
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as any)}
          >
            <option value="all">全部状态</option>
            <option value="active">已启用</option>
            <option value="inactive">已停用</option>
          </select>
        </div>
        <button className={styles.addButton} onClick={handleAdd}>
          + 添加项目
        </button>
      </div>

      {/* 项目列表 */}
      <div className={styles.projectsList}>
        {selectedOrgId === 'all' ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>请选择一个组织</div>
            <div className={styles.emptyHint}>从上方下拉菜单中选择一个组织以查看其项目</div>
          </div>
        ) : loading ? (
          <div className={styles.loading}>加载中...</div>
        ) : filteredProjects.length === 0 ? (
          <div className={styles.empty}>
            <div className={styles.emptyText}>
              {searchTerm || statusFilter !== 'all' ? '没有找到匹配的项目' : '暂无项目'}
            </div>
            <div className={styles.emptyHint}>
              {searchTerm || statusFilter !== 'all'
                ? '尝试调整搜索条件'
                : '点击"添加项目"按钮创建第一个项目'}
            </div>
          </div>
        ) : (
          <table className={styles.projectsTable}>
            <thead>
              <tr>
                <th>项目名称</th>
                <th>描述</th>
                <th>状态</th>
                <th>创建时间</th>
                <th>更新时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {filteredProjects.map((project) => (
                <tr key={project.id}>
                  <td className={styles.nameCell}>
                    <div>
                      <div className={styles.projectName}>{project.display_name}</div>
                      <div className={styles.projectId}>{project.name}</div>
                    </div>
                  </td>
                  <td className={styles.descriptionCell} title={project.description}>
                    {project.description || '-'}
                  </td>
                  <td>{renderStatusBadge(project)}</td>
                  <td className={styles.dateCell}>
                    {new Date(project.created_at).toLocaleDateString('zh-CN')}
                  </td>
                  <td className={styles.dateCell}>
                    {new Date(project.updated_at).toLocaleDateString('zh-CN')}
                  </td>
                  <td>
                    <div className={styles.actionButtons}>
                      <button className={styles.actionButton} onClick={() => handleEdit(project)}>
                        编辑
                      </button>
                      <button
                        className={`${styles.actionButton} ${
                          project.is_active ? styles.disable : styles.enable
                        }`}
                        onClick={() => handleToggleStatus(project)}
                        disabled={project.is_default}
                        title={project.is_default ? '默认项目不能停用' : ''}
                      >
                        {project.is_active ? '停用' : '启用'}
                      </button>
                      <button
                        className={`${styles.actionButton} ${styles.delete}`}
                        onClick={() => handleDelete(project)}
                        disabled={project.is_default}
                        title={project.is_default ? '默认项目不能删除' : ''}
                      >
                        删除
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* 添加/编辑对话框 */}
      {showDialog && (
        <div className={styles.dialog} onClick={() => setShowDialog(false)}>
          <div className={styles.dialogContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.dialogHeader}>
              <h2 className={styles.dialogTitle}>{editingProject ? '编辑项目' : '添加项目'}</h2>
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
                    disabled={!!editingProject}
                  >
                    <option value={0}>请选择组织</option>
                    {organizations.map((org) => (
                      <option key={org.id} value={org.id}>
                        {org.display_name}
                      </option>
                    ))}
                  </select>
                  {formErrors.org_id && <span className={styles.errorText}>{formErrors.org_id}</span>}
                  {!formErrors.org_id && editingProject && (
                    <span className={styles.hint}>创建后不可修改</span>
                  )}
                </div>

                {/* 项目标识 */}
                <div className={styles.formGroup}>
                  <label className={styles.label}>
                    项目标识<span className={styles.required}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${formErrors.name ? styles.error : ''}`}
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    placeholder="例如：infrastructure"
                    disabled={!!editingProject}
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
                    placeholder="例如：基础设施项目"
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
                    placeholder="项目的简要描述（可选）"
                    rows={3}
                  />
                  {formErrors.description && (
                    <span className={styles.errorText}>{formErrors.description}</span>
                  )}
                  {!formErrors.description && <span className={styles.hint}>最多500个字符</span>}
                </div>

                {/* 启用状态 */}
                {!editingProject && (
                  <>
                    <div className={styles.checkbox}>
                      <input
                        type="checkbox"
                        id="is_active"
                        checked={formData.is_active}
                        onChange={(e) => setFormData({ ...formData, is_active: e.target.checked })}
                      />
                      <label htmlFor="is_active">启用此项目</label>
                    </div>
                    <div className={styles.checkboxHint}>停用后，该项目下的所有工作空间将无法访问</div>
                  </>
                )}
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
                  {editingProject ? '保存' : '创建'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 删除确认对话框 */}
      <ConfirmDialog
        isOpen={deleteConfirm.show}
        title="确认删除"
        message={`确定要删除项目 ${deleteConfirm.project?.display_name} 吗？如果项目下有工作空间，删除将失败。`}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm({ show: false, project: null })}
        confirmText="删除"
        cancelText="取消"
      />
    </div>
  );
};

export default ProjectManagement;
