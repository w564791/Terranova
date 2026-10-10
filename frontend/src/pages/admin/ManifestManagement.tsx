import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate, Link, useSearchParams } from 'react-router-dom';
import {
  Button,
  Select,
  Input,
  Popconfirm,
  Tooltip,
  Dropdown,
  Modal,
  Form,
  message,
  Radio,
  Tag,
  Empty,
  Spin,
} from 'antd';
import type { MenuProps } from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  RocketOutlined,
  ExportOutlined,
  MoreOutlined,
  SearchOutlined,
  EditFilled,
  GithubOutlined,
} from '@ant-design/icons';
import type {
  Manifest,
  ManifestCapabilities,
  ManifestSourceType,
  AvailableGitHubInstallation,
} from '../../services/manifestApi';
import {
  listManifests,
  deleteManifest,
  exportManifestZip,
  createManifest,
  listAvailableGitHubInstallations,
  checkOrgAdmin,
  gitRepoName,
  normalizeGitSubpath,
} from '../../services/manifestApi';
import { gitErrorCode, gitErrorMessage } from './ManifestEditorV2/bundleStatus';
import { GitHubAppConnectButton, GitHubRepoSelect } from './manifestGit/GitHubAppControls';
import { GITHUB_APP_CALLBACK_PARAMS, githubAppCallbackNotice } from './manifestGit/githubAppCallback';
import { iamService, setAuthOrgId } from '../../services/iam';
import { useToast } from '../../contexts/ToastContext';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './ManifestManagement.module.css';

interface Organization {
  id: number;
  name: string;
  display_name?: string;
}

type FilterType = 'all' | 'draft' | 'published' | 'archived';

interface CreateFormValues {
  name: string;
  description?: string;
  github_installation_id?: number;
  /** 仓库 full name "<owner>/<repo>" */
  git_repo?: string;
  git_subpath?: string;
}

/** 列表行的来源标记:Native / Git(悬停显示仓库) */
function SourceBadge({ manifest }: { manifest: Manifest }) {
  if (manifest.source_type === 'git') {
    const repo = gitRepoName(manifest.git_repo_url);
    const title = repo
      ? `Git 仓库：${repo}${manifest.git_subpath ? `（目录 ${manifest.git_subpath}）` : ''}`
      : 'Git 仓库';
    return (
      <Tooltip title={title}>
        <Tag icon={<GithubOutlined />} color="geekblue" style={{ marginInlineEnd: 0 }}>
          Git
        </Tag>
      </Tooltip>
    );
  }
  return (
    <Tooltip title="在线编辑">
      <Tag style={{ marginInlineEnd: 0 }}>Native</Tag>
    </Tooltip>
  );
}

const sourceCardStyle = (active: boolean): React.CSSProperties => ({
  flex: 1,
  height: 'auto',
  padding: '14px 16px',
  borderRadius: 8,
  lineHeight: 1.5,
  border: active ? '1px solid var(--brand, #1677ff)' : undefined,
});

const ManifestManagement: React.FC = () => {
  const navigate = useNavigate();
  const toast = useToast();
  const [manifests, setManifests] = useState<Manifest[]>([]);
  const [capabilities, setCapabilities] = useState<ManifestCapabilities | null>(null);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [selectedOrgId, setSelectedOrgId] = useState<number | null>(null);
  const [filter, setFilter] = useState<FilterType>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deletingManifest, setDeletingManifest] = useState<Manifest | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const orgId = selectedOrgId?.toString() || '';
  const [createForm] = Form.useForm<CreateFormValues>();
  // 新建第一步:选择来源(创建后不可更改)
  const [createStep, setCreateStep] = useState<0 | 1>(0);
  const [createSource, setCreateSource] = useState<ManifestSourceType>('native');
  // git 来源:本组织已绑定的 GitHub App installation(MANIFESTS WRITE 可读;空 => 空状态)
  const [installations, setInstallations] = useState<AvailableGitHubInstallation[] | null>(null);
  const [installationsLoading, setInstallationsLoading] = useState(false);
  const [installationsError, setInstallationsError] = useState<string | null>(null);
  // "连接 GitHub App" 仅组织 ADMIN 可见(后端 connect 路由 RequirePermission(ORGANIZATION, ORGANIZATION, ADMIN))
  const [isOrgAdmin, setIsOrgAdmin] = useState(false);
  const selectedInstallationId = Form.useWatch('github_installation_id', createForm);

  const openCreate = () => {
    createForm.resetFields();
    setCreateStep(0);
    setCreateSource('native');
    setCreateOpen(true);
  };

  const loadInstallations = useCallback(async () => {
    if (!orgId) return;
    setInstallationsLoading(true);
    try {
      setInstallationsError(null);
      const [rows, admin] = await Promise.all([
        listAvailableGitHubInstallations(orgId).catch((err) => {
          setInstallationsError(gitErrorMessage(err) ?? (err as Error)?.message ?? '加载失败');
          return [] as AvailableGitHubInstallation[];
        }),
        checkOrgAdmin(orgId),
      ]);
      setInstallations(rows);
      setIsOrgAdmin(admin);
    } finally {
      setInstallationsLoading(false);
    }
  }, [orgId]);

  // GitHub App setup callback 回跳(?github_app=connected|requested|error&reason=...):提示一次后去掉参数
  const [searchParams, setSearchParams] = useSearchParams();
  const callbackHandledRef = useRef(false);
  useEffect(() => {
    const result = searchParams.get('github_app');
    if (!result) return;
    if (!callbackHandledRef.current) {
      callbackHandledRef.current = true;
      const notice = githubAppCallbackNotice(result, searchParams.get('reason'));
      if (notice) message.open({ type: notice.type, content: notice.text, key: 'github-app-callback', duration: 5 });
    }
    const next = new URLSearchParams(searchParams);
    GITHUB_APP_CALLBACK_PARAMS.forEach(k => next.delete(k));
    setSearchParams(next, { replace: true });
  }, [searchParams, setSearchParams]);

  // 加载组织列表
  useEffect(() => {
    const loadOrganizations = async () => {
      try {
        const response = await iamService.bootstrapActiveOrganization();
        setOrganizations(response.organizations || []);
        if (response.active_org_id != null) {
          setSelectedOrgId(response.active_org_id);
        }
      } catch (error) {
        console.error('加载组织列表失败:', error);
      }
    };
    loadOrganizations();
  }, []);


  const fetchManifests = useCallback(async () => {
    if (!orgId) return;
    setLoading(true);
    try {
      const params: any = { page, page_size: pageSize };
      if (filter !== 'all') {
        params.status = filter;
      }
      const response = await listManifests(orgId, params);
      let items = response.items || [];
      
      // 前端搜索过滤
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        items = items.filter(m => 
          m.name.toLowerCase().includes(query) ||
          m.description?.toLowerCase().includes(query) ||
          m.id.toLowerCase().includes(query)
        );
      }
      
      setManifests(items);
      setCapabilities(response.capabilities ?? null);
      setTotal(response.total || 0);
    } catch (error: any) {
      setCapabilities(null);
      toast.error('获取 Manifest 列表失败: ' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  }, [orgId, page, pageSize, filter, searchQuery, toast]);

  useEffect(() => {
    if (selectedOrgId) {
      fetchManifests();
    }
  }, [page, pageSize, selectedOrgId, filter, searchQuery]);

  const handleDelete = async (id: string) => {
    try {
      await deleteManifest(orgId, id);
      toast.success('删除成功');
      fetchManifests();
    } catch (error: any) {
      toast.error('删除失败: ' + (error.message || '未知错误'));
    }
  };

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      setCreating(true);
      const m = await createManifest(
        orgId,
        createSource === 'git'
          ? {
              name: values.name,
              description: values.description ?? '',
              source_type: 'git',
              github_installation_id: values.github_installation_id,
              git_repo: values.git_repo,
              // git 来源字段只在创建时发送(创建后不可变)
              git_subpath: normalizeGitSubpath(values.git_subpath).value || undefined,
            }
          : { name: values.name, description: values.description ?? '', source_type: 'native' }
      );
      message.success('已创建,正在跳转编辑器');
      setCreateOpen(false);
      createForm.resetFields();
      navigate(`/admin/manifests-v2/${m.id}/edit?org=${selectedOrgId}`);
    } catch (err: any) {
      const msg = typeof err === 'string' ? err : err?.message;
      // form validation 错误有 errorFields 字段, 不弹 message
      if (err?.errorFields) return;
      const gitMsg = gitErrorMessage(err);
      if (gitMsg) {
        // 子路径不合法:就地显示在子路径字段上
        if (gitErrorCode(err) === 'git_subpath_invalid') {
          createForm.setFields([{ name: 'git_subpath', errors: [gitMsg] }]);
          return;
        }
        message.error('创建失败：' + gitMsg);
        return;
      }
      if (msg) message.error('创建失败: ' + msg);
    } finally {
      setCreating(false);
    }
  };

  // 格式化相对时间
  const formatRelativeTime = (dateString: string | null) => {
    if (!dateString) return '从未';
    if (dateString.startsWith('0001-01-01')) return '从未';
    
    const date = new Date(dateString);
    const now = new Date();
    
    if (isNaN(date.getTime())) return '无效日期';
    
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 5) return '刚刚';
    if (diffMins < 60) return `${diffMins}分钟前`;
    if (diffHours < 24) return `${diffHours}小时前`;
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  };

  // 获取状态分类
  const getStatusCategory = (status: string): string => {
    switch (status) {
      case 'published':
        return 'success';
      case 'draft':
        return 'pending';
      case 'archived':
        return 'neutral';
      default:
        return 'neutral';
    }
  };

  // 获取状态显示文本
  const getStatusText = (status: string): string => {
    switch (status) {
      case 'published':
        return 'Published';
      case 'draft':
        return 'Draft';
      case 'archived':
        return 'Archived';
      default:
        return status;
    }
  };

  // 计算各状态数量
  const filterCounts = {
    all: total,
    draft: manifests.filter(m => m.status === 'draft').length,
    published: manifests.filter(m => m.status === 'published').length,
    archived: manifests.filter(m => m.status === 'archived').length,
  };

  const totalPages = Math.ceil(total / pageSize);

  // 新建入口按列表响应顶层 capabilities.can_write(列表为空时后端也返回);
  // 后端仍按 MANIFESTS WRITE 校验。
  const canCreate = capabilities?.can_write === true;

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.pageHeader}>
        <div className={styles.headerLeft}>
          <h1 className={styles.pageTitle}>Manifests</h1>
          {organizations.length > 1 && (
            <Select
              value={selectedOrgId}
              onChange={(value) => {
                setAuthOrgId(value);
                setSelectedOrgId(value);
              }}
              style={{ width: 200 }}
              options={organizations.map(org => ({
                value: org.id,
                label: org.display_name || org.name,
              }))}
            />
          )}
        </div>
        <div className={styles.headerRight}>
          {canCreate && (
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={openCreate}
              disabled={!selectedOrgId}
            >
              New Manifest
            </Button>
          )}
        </div>
      </div>

      {/* 过滤器栏 */}
      <div className={styles.filterSection}>
        <div className={styles.filterBar}>
          <button
            className={`${styles.filterButton} ${filter === 'all' ? styles.filterActive : ''}`}
            onClick={() => setFilter('all')}
          >
            All <span className={styles.filterCount}>{filterCounts.all}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'draft' ? styles.filterActive : ''}`}
            onClick={() => setFilter('draft')}
          >
            Draft <span className={styles.filterCount}>{filterCounts.draft}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'published' ? styles.filterActive : ''}`}
            onClick={() => setFilter('published')}
          >
            Published <span className={styles.filterCount}>{filterCounts.published}</span>
          </button>
          <button
            className={`${styles.filterButton} ${filter === 'archived' ? styles.filterActive : ''}`}
            onClick={() => setFilter('archived')}
          >
            Archived <span className={styles.filterCount}>{filterCounts.archived}</span>
          </button>
        </div>
        
        <div className={styles.searchBar}>
          <Input
            prefix={<SearchOutlined />}
            placeholder="Search by name, description, or ID"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            allowClear
            style={{ width: 300 }}
          />
        </div>
      </div>

      {/* Manifest 列表 */}
      <div className={styles.listSection}>
        {loading ? (
          <div className={styles.loading}>加载中...</div>
        ) : manifests.length === 0 ? (
          <div className={styles.emptyState}>
            <p className={styles.emptyText}>No manifests found</p>
            <p className={styles.emptyHint}>
              Create a new manifest to start building your infrastructure templates
            </p>
            {canCreate && (
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={openCreate}
                disabled={!selectedOrgId}
              >
                Create Manifest
              </Button>
            )}
          </div>
        ) : (
          <div className={styles.manifestList}>
            {manifests.map((manifest, index) => (
              <Link
                key={manifest.id}
                to={`/admin/manifests-v2/${manifest.id}/edit?org=${selectedOrgId}`}
                className={styles.manifestItem}
              >
                {/* 左侧状态指示条 */}
                <div className={`${styles.statusIndicator} ${styles[`indicator-${getStatusCategory(manifest.status)}`]}`}></div>
                
                {/* 主内容区 */}
                <div className={styles.manifestContent}>
                  {/* 第一行：名称 */}
                  <div className={styles.manifestTitleRow}>
                    <span className={styles.manifestName}>{manifest.name}</span>
                    <SourceBadge manifest={manifest} />
                    {manifest.deployment_count && manifest.deployment_count > 0 && (
                      <span className={styles.deploymentBadge}>
                        {manifest.deployment_count} deployments
                      </span>
                    )}
                  </div>
                  
                  {/* 第二行：元信息 */}
                  <div className={styles.manifestMetaRow}>
                    <span className={styles.manifestId}>{manifest.id}</span>
                    <span className={styles.metaSeparator}>|</span>
                    <span className={styles.manifestVersion}>
                      {/* version 字段本身已含 'v' 前缀(如 v1.0.5),不要再补 v */}
                      {manifest.latest_version?.version || 'draft'}
                    </span>
                    {manifest.description && (
                      <>
                        <span className={styles.metaSeparator}>|</span>
                        <span className={styles.manifestDescription}>
                          {manifest.description.length > 50 
                            ? manifest.description.substring(0, 50) + '...' 
                            : manifest.description}
                        </span>
                      </>
                    )}
                  </div>
                </div>
                
                {/* 右侧状态区 */}
                <div className={styles.manifestStatusArea}>
                  <span className={`${styles.statusBadge} ${styles[`statusBadge-${getStatusCategory(manifest.status)}`]}`}>
                    {manifest.status === 'published' ? '✓ ' : ''}
                    {getStatusText(manifest.status)}
                  </span>
                  <span className={styles.manifestTime}>
                    {formatRelativeTime(manifest.updated_at)}
                  </span>
                </div>
                
                {/* 操作按钮 */}
                <div className={styles.manifestActions} onClick={(e) => e.preventDefault()}>
                  <Dropdown
                    menu={{
                      items: [
                        // 编辑仅 can_write;删除仅 can_admin;部署仅 can_deploy(后端仍逐项校验)
                        manifest.can_write && {
                          key: 'edit',
                          icon: <EditOutlined />,
                          label: 'Edit',
                          onClick: () => navigate(`/admin/manifests-v2/${manifest.id}/edit?org=${selectedOrgId}`),
                        },
                        manifest.can_deploy && {
                          key: 'deploy',
                          icon: <RocketOutlined />,
                          label: 'Deploy',
                          // 新版部署在编辑器内的"部署到 Workspace"弹窗中完成,
                          // 这里直接进 v2 编辑器,用户在编辑器内点按钮触发部署
                          onClick: () => navigate(`/admin/manifests-v2/${manifest.id}/edit?org=${selectedOrgId}`),
                        },
                        {
                          key: 'export',
                          icon: <ExportOutlined />,
                          label: 'Export ZIP',
                          onClick: async () => {
                            try {
                              const blob = await exportManifestZip(orgId, manifest.id);
                              // 创建下载
                              const url = URL.createObjectURL(blob);
                              const a = document.createElement('a');
                              a.href = url;
                              a.download = `${manifest.name}-${manifest.latest_version?.version || 'draft'}.zip`;
                              document.body.appendChild(a);
                              a.click();
                              document.body.removeChild(a);
                              URL.revokeObjectURL(url);
                              toast.success('导出成功 (ZIP 包含 manifest.json 和 .tf 文件)');
                            } catch (error: any) {
                              toast.error('导出失败: ' + (error.message || '未知错误'));
                            }
                          },
                        },
                        manifest.can_admin && {
                          type: 'divider' as const,
                        },
                        manifest.can_admin && {
                          key: 'delete',
                          icon: <DeleteOutlined />,
                          label: 'Delete',
                          danger: true,
                          disabled: manifest.deployment_count !== undefined && manifest.deployment_count > 0,
                          onClick: () => {
                            if (manifest.deployment_count && manifest.deployment_count > 0) {
                              toast.warning('该 Manifest 有部署记录，请先删除部署');
                            } else {
                              setDeletingManifest(manifest);
                              setDeleteDialogOpen(true);
                            }
                          },
                        },
                      ].filter(Boolean) as MenuProps['items'],
                    }}
                    trigger={['click']}
                    placement="bottomRight"
                  >
                    <Button
                      type="text"
                      icon={<MoreOutlined />}
                      className={styles.moreButton}
                    />
                  </Dropdown>
                </div>
              </Link>
            ))}
          </div>
        )}

        {/* 分页 */}
        {total > 0 && (
          <div className={styles.paginationContainer}>
            <div className={styles.paginationLeft}>
              <div className={styles.paginationInfo}>
                Showing {Math.min((page - 1) * pageSize + 1, total)} to {Math.min(page * pageSize, total)} of {total} manifests
              </div>
              <div className={styles.pageSizeSelector}>
                <label className={styles.pageSizeLabel}>Per page:</label>
                <select 
                  value={pageSize} 
                  onChange={(e) => {
                    setPageSize(Number(e.target.value));
                    setPage(1);
                  }}
                  className={styles.pageSizeSelect}
                >
                  <option value={10}>10</option>
                  <option value={20}>20</option>
                  <option value={50}>50</option>
                  <option value={100}>100</option>
                </select>
              </div>
            </div>
            <div className={styles.paginationControls}>
              <button
                onClick={() => setPage(page - 1)}
                disabled={page === 1}
                className={styles.paginationButton}
              >
                ← Previous
              </button>
              <span className={styles.paginationPages}>
                Page {page} of {totalPages}
              </span>
              <button
                onClick={() => setPage(page + 1)}
                disabled={page >= totalPages}
                className={styles.paginationButton}
              >
                Next →
              </button>
            </div>
          </div>
        )}
      </div>
      {/* 删除确认弹窗 */}
      <ConfirmDialog
        isOpen={deleteDialogOpen}
        title="删除 Manifest"
        message={`确定要删除 Manifest "${deletingManifest?.name}" 吗？此操作不可恢复。`}
        confirmText="删除"
        cancelText="取消"
        type="danger"
        loading={deleting}
        onConfirm={async () => {
          if (!deletingManifest) return;
          setDeleting(true);
          try {
            await deleteManifest(orgId, deletingManifest.id);
            toast.success('删除成功');
            setDeleteDialogOpen(false);
            setDeletingManifest(null);
            fetchManifests();
          } catch (error: any) {
            toast.error('删除失败: ' + (error.message || '未知错误'));
          } finally {
            setDeleting(false);
          }
        }}
        onCancel={() => {
          setDeleteDialogOpen(false);
          setDeletingManifest(null);
        }}
      />
      {/* 创建 Manifest 弹窗:第一步选来源,第二步填写 */}
      <Modal
        title="新建 Manifest"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        destroyOnClose
        footer={
          createStep === 0
            ? [
                <Button key="cancel" onClick={() => setCreateOpen(false)}>
                  取消
                </Button>,
                <Button
                  key="next"
                  type="primary"
                  onClick={() => {
                    setCreateStep(1);
                    if (createSource === 'git') loadInstallations();
                  }}
                >
                  下一步
                </Button>,
              ]
            : [
                <Button key="back" onClick={() => setCreateStep(0)}>
                  上一步
                </Button>,
                <Button
                  key="ok"
                  type="primary"
                  loading={creating}
                  disabled={createSource === 'git' && (installationsLoading || !installations?.length)}
                  onClick={handleCreate}
                >
                  创建
                </Button>,
              ]
        }
      >
        {createStep === 0 ? (
          <>
            <p style={{ color: 'var(--ink-3)', marginBottom: 12 }}>选择内容来源（创建后不可更改）：</p>
            <Radio.Group
              value={createSource}
              onChange={(e) => setCreateSource(e.target.value)}
              style={{ display: 'flex', gap: 12, width: '100%' }}
            >
              <Radio.Button value="native" style={sourceCardStyle(createSource === 'native')}>
                <div style={{ fontWeight: 600 }}>
                  <EditFilled /> 在线编辑（native）
                </div>
                <div style={{ fontSize: 12, color: 'var(--ink-3)', whiteSpace: 'normal' }}>
                  在平台的 VS Code Web 编辑器里编写 .tf 文件并发布版本
                </div>
              </Radio.Button>
              <Radio.Button value="git" style={sourceCardStyle(createSource === 'git')}>
                <div style={{ fontWeight: 600 }}>
                  <GithubOutlined /> Git 仓库
                </div>
                <div style={{ fontSize: 12, color: 'var(--ink-3)', whiteSpace: 'normal' }}>
                  内容来自 GitHub 仓库（只读），发布时选择 commit
                </div>
              </Radio.Button>
            </Radio.Group>
          </>
        ) : (
          <>
            <p style={{ color: 'var(--ink-3)', marginBottom: 16 }}>
              来源：{createSource === 'git' ? 'Git 仓库' : '在线编辑（native）'}（创建后不可更改）。
              {createSource === 'git'
                ? '创建后可在编辑器中只读浏览仓库内容，发布时选择 commit。'
                : '创建后会自动跳转到 VS Code Web 编辑器,你可以在那里写 .tf 文件、发布版本、部署到 workspace。'}
            </p>
            {createSource === 'git' && installationsLoading ? (
              <div style={{ textAlign: 'center', padding: 24 }}>
                <Spin />
              </div>
            ) : createSource === 'git' && !installations?.length ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={
                  installationsError
                    ? `加载 GitHub App 安装失败：${installationsError}`
                    : isOrgAdmin
                      ? '尚未连接 GitHub App'
                      : '尚未连接 GitHub App，请联系组织管理员'
                }
              >
                {isOrgAdmin && <GitHubAppConnectButton orgId={orgId} type="primary" />}
              </Empty>
            ) : (
              <Form
                form={createForm}
                layout="vertical"
                preserve={false}
                initialValues={
                  createSource === 'git' && installations?.length === 1
                    ? { github_installation_id: installations[0].id }
                    : undefined
                }
              >
                <Form.Item
                  label="名称"
                  name="name"
                  rules={[
                    { required: true, message: '请输入名称' },
                    { max: 255, message: '不超过 255 字符' },
                  ]}
                >
                  <Input placeholder="例如: aws-vpc-stack" autoFocus />
                </Form.Item>
                <Form.Item
                  label="描述 (可选)"
                  name="description"
                  rules={[{ max: 1024, message: '不超过 1024 字符' }]}
                >
                  <Input.TextArea rows={3} maxLength={1024} showCount placeholder="这个 manifest 的用途简介" />
                </Form.Item>
                {createSource === 'git' && (
                  <>
                    <Form.Item label="GitHub 账户" required extra="仓库列表来自该账户下 GitHub App 已授权的仓库">
                      <div style={{ display: 'flex', gap: 8 }}>
                        <Form.Item
                          name="github_installation_id"
                          noStyle
                          rules={[{ required: true, message: '请选择 GitHub 账户' }]}
                        >
                          <Select
                            style={{ flex: 1 }}
                            placeholder="选择已连接的 GitHub 账户"
                            options={(installations ?? []).map(i => ({ value: i.id, label: i.account }))}
                            onChange={() => createForm.setFieldValue('git_repo', undefined)}
                          />
                        </Form.Item>
                        {isOrgAdmin && <GitHubAppConnectButton orgId={orgId} />}
                      </div>
                    </Form.Item>
                    <Form.Item label="仓库" name="git_repo" rules={[{ required: true, message: '请选择仓库' }]}>
                      <GitHubRepoSelect
                        orgId={orgId}
                        installationId={selectedInstallationId}
                      />
                    </Form.Item>
                    <Form.Item
                      label="子目录 (可选)"
                      name="git_subpath"
                      extra="仓库内作为 bundle 根的目录；留空表示仓库根目录。创建后不可修改"
                      rules={[
                        { max: 512, message: '不超过 512 字符' },
                        {
                          // 与后端 CleanSubpath + ValidatePath 一致;空 = 仓库根
                          validator: async (_, value?: string) => {
                            const { error } = normalizeGitSubpath(value);
                            if (error) throw new Error(error);
                          },
                        },
                      ]}
                    >
                      <Input placeholder="例如: stacks/vpc" />
                    </Form.Item>
                  </>
                )}
              </Form>
            )}
          </>
        )}
      </Modal>
    </div>
  );
};

export default ManifestManagement;
