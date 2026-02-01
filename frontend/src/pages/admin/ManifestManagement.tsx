import React, { useState, useEffect, useCallback } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import {
  Button,
  Select,
  Input,
  Popconfirm,
  Tooltip,
  Dropdown,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  RocketOutlined,
  ExportOutlined,
  ImportOutlined,
  MoreOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import type { Manifest } from '../../services/manifestApi';
import { listManifests, deleteManifest, exportManifestZip } from '../../services/manifestApi';
import { iamService } from '../../services/iam';
import { useToast } from '../../contexts/ToastContext';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './ManifestManagement.module.css';

interface Organization {
  id: number;
  name: string;
  display_name?: string;
}

type FilterType = 'all' | 'draft' | 'published' | 'archived';

const ManifestManagement: React.FC = () => {
  const navigate = useNavigate();
  const toast = useToast();
  const [manifests, setManifests] = useState<Manifest[]>([]);
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

  // 加载组织列表
  useEffect(() => {
    const loadOrganizations = async () => {
      try {
        const response = await iamService.listOrganizations(true);
        setOrganizations(response.organizations || []);
        if (response.organizations && response.organizations.length > 0) {
          setSelectedOrgId(response.organizations[0].id);
        }
      } catch (error) {
        console.error('加载组织列表失败:', error);
      }
    };
    loadOrganizations();
  }, []);

  const orgId = selectedOrgId?.toString() || '';

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
      setTotal(response.total || 0);
    } catch (error: any) {
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

  return (
    <div className={styles.container}>
      {/* 页面头部 */}
      <div className={styles.pageHeader}>
        <div className={styles.headerLeft}>
          <h1 className={styles.pageTitle}>Manifests</h1>
          {organizations.length > 1 && (
            <Select
              value={selectedOrgId}
              onChange={(value) => setSelectedOrgId(value)}
              style={{ width: 200 }}
              options={organizations.map(org => ({
                value: org.id,
                label: org.display_name || org.name,
              }))}
            />
          )}
        </div>
        <div className={styles.headerRight}>
          <Button
            icon={<ImportOutlined />}
            onClick={() => navigate(`/admin/manifests/new?org=${selectedOrgId}&tab=import`)}
            disabled={!selectedOrgId}
          >
            Import HCL
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => navigate(`/admin/manifests/new?org=${selectedOrgId}`)}
            disabled={!selectedOrgId}
          >
            New Manifest
          </Button>
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
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => navigate(`/admin/manifests/new?org=${selectedOrgId}`)}
              disabled={!selectedOrgId}
            >
              Create Manifest
            </Button>
          </div>
        ) : (
          <div className={styles.manifestList}>
            {manifests.map((manifest, index) => (
              <Link
                key={manifest.id}
                to={`/admin/manifests/${manifest.id}/edit?org=${selectedOrgId}`}
                className={styles.manifestItem}
              >
                {/* 左侧状态指示条 */}
                <div className={`${styles.statusIndicator} ${styles[`indicator-${getStatusCategory(manifest.status)}`]}`}></div>
                
                {/* 主内容区 */}
                <div className={styles.manifestContent}>
                  {/* 第一行：名称 */}
                  <div className={styles.manifestTitleRow}>
                    <span className={styles.manifestName}>{manifest.name}</span>
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
                      v{manifest.latest_version?.version || 'draft'}
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
                        {
                          key: 'edit',
                          icon: <EditOutlined />,
                          label: 'Edit',
                          onClick: () => navigate(`/admin/manifests/${manifest.id}/edit?org=${selectedOrgId}`),
                        },
                        {
                          key: 'deploy',
                          icon: <RocketOutlined />,
                          label: 'Deploy',
                          onClick: () => navigate(`/admin/manifests/${manifest.id}/deploy?org=${selectedOrgId}`),
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
                        {
                          type: 'divider',
                        },
                        {
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
                      ],
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
    </div>
  );
};

export default ManifestManagement;
