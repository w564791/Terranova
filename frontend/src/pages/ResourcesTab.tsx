import React, { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import api from '../services/api';
import ConfirmDialog from '../components/ConfirmDialog';
import { useWorkspaceManifestSummary } from '../hooks/useWorkspaceManifestSummary';
import styles from './ResourcesTab.module.css';

interface Resource {
  id: number;
  workspace_id: number;
  resource_type: string;
  resource_name: string;
  resource_id: string;
  current_version_id: number;
  is_active: boolean;
  description: string;
  tf_code: any;
  variables: any;
  created_by: number;
  created_at: string;
  updated_at: string;
  manifest_deployment_id?: string; // Manifest 部署 ID
  current_version?: {
    id: number;
    version: number;
    is_latest: boolean;
    change_summary?: string;
  };
}

interface ResourcesTabProps {
  workspaceId: string;
}

const ResourcesTab: React.FC<ResourcesTabProps> = ({ workspaceId }) => {
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  // 从URL读取状态
  const searchParams = new URLSearchParams(window.location.search);
  const pageFromUrl = parseInt(searchParams.get('page') || '1');
  const pageSizeFromUrl = parseInt(searchParams.get('pageSize') || '10');
  const searchFromUrl = searchParams.get('search') || '';
  const sortByFromUrl = searchParams.get('sortBy') || 'created_at';
  const sortOrderFromUrl = (searchParams.get('sortOrder') as 'asc' | 'desc') || 'desc';
  
  const [resources, setResources] = useState<Resource[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState(searchFromUrl);
  const [page, setPage] = useState(pageFromUrl);
  const [pageSize, setPageSize] = useState(pageSizeFromUrl);
  const [total, setTotal] = useState(0);
  const [sortBy, setSortBy] = useState(sortByFromUrl);
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>(sortOrderFromUrl);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [resourceToDelete, setResourceToDelete] = useState<Resource | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [includeInactive, setIncludeInactive] = useState(false);
  const [showDropdown, setShowDropdown] = useState(false);
  const [exporting, setExporting] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  // Manifest 软链接信息(workspace 装了 manifest 时用作徽章 / banner / 锁定按钮)
  const { summary: manifestSummary, loading: manifestSummaryLoading } = useWorkspaceManifestSummary(workspaceId);
  const isManifestManaged = manifestSummary?.has_manifest ?? false;

  const buildManifestEditorUrl = (resource?: Resource) => {
    if (manifestSummary?.manifest_id == null || manifestSummary?.org_id == null) return null;
    const params = new URLSearchParams({
      org: String(manifestSummary.org_id),
      subpath: manifestSummary.subpath ?? '',
    });
    if (manifestSummary.version_id) {
      params.set('version', manifestSummary.version_id);
    }
    if (resource?.resource_id) {
      params.set('resource', resource.resource_id);
    }
    return `/admin/manifests-v2/${manifestSummary.manifest_id}/edit?${params.toString()}`;
  };

  useEffect(() => {
    fetchResources();
  }, [workspaceId, page, pageSize, searchTerm, sortBy, sortOrder, includeInactive]);

  // 更新URL参数
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    params.set('tab', 'resources');
    params.set('page', page.toString());
    params.set('pageSize', pageSize.toString());
    params.set('sortBy', sortBy);
    params.set('sortOrder', sortOrder);
    if (searchTerm) {
      params.set('search', searchTerm);
    } else {
      params.delete('search');
    }
    navigate(`/workspaces/${workspaceId}?${params.toString()}`, { replace: true });
  }, [page, pageSize, searchTerm, sortBy, sortOrder, workspaceId, navigate]);

  // 当搜索或排序改变时，重置到第一页
  useEffect(() => {
    if (page !== 1) {
      setPage(1);
    }
  }, [searchTerm, sortBy, sortOrder]);

  const fetchResources = async () => {
    try {
      setLoading(true);
      
      // 构建查询参数
      const params = new URLSearchParams({
        page: page.toString(),
        page_size: pageSize.toString(),
        sort_by: sortBy,
        sort_order: sortOrder,
        include_inactive: includeInactive.toString(),
      });
      
      if (searchTerm) {
        params.append('search', searchTerm);
      }
      
      const response = await api.get(
        `/workspaces/${workspaceId}/resources?${params.toString()}`
      );
      
      const data = response.data || response;
      setResources(data.resources || []);
      setTotal(data.pagination?.total || 0);
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    } finally {
      setLoading(false);
    }
  };

  // 点击外部关闭下拉菜单
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setShowDropdown(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, []);

  const handleAddResource = () => {
    navigate(`/workspaces/${workspaceId}/add-resources`);
    setShowDropdown(false);
  };

  const handleExportHCL = async () => {
    try {
      setExporting(true);
      setShowDropdown(false);
      
      // 调用导出API - 由于响应拦截器返回response.data，这里直接就是blob或文本数据
      const response: any = await api.get(`/workspaces/${workspaceId}/resources/export/hcl`, {
        responseType: 'blob',
      });
      
      // response 已经是 blob 数据（因为响应拦截器返回了 response.data）
      // 如果是 Blob 类型直接使用，否则创建新的 Blob
      let blobData: Blob;
      if (response instanceof Blob) {
        blobData = response;
      } else if (typeof response === 'string') {
        blobData = new Blob([response], { type: 'text/plain;charset=utf-8' });
      } else {
        // 如果是其他类型，尝试转换为字符串
        blobData = new Blob([JSON.stringify(response)], { type: 'text/plain;charset=utf-8' });
      }
      
      // 创建下载链接
      const url = window.URL.createObjectURL(blobData);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${workspaceId}-resources.tf`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
      
      showToast('资源导出成功', 'success');
    } catch (error: any) {
      if (error.response?.status === 403) {
        showToast('权限不足：需要Workspace Admin权限才能导出资源', 'error');
      } else {
        const message = extractErrorMessage(error);
        showToast(message || '导出失败', 'error');
      }
    } finally {
      setExporting(false);
    }
  };

  const handleViewResource = (resource: Resource) => {
    // manifest 部署来源的资源:跳 manifest 编辑器并定位到对应 module/resource 块
    // (这类资源没有 tf_code,走 ViewResource 会报「无法获取Module信息」)。
    if (resource.manifest_deployment_id) {
      // summary 还在加载:不能 fallthrough 到会报错的 ViewResource,提示稍候
      if (manifestSummaryLoading) {
        showToast('Manifest 信息加载中,请稍候再试', 'info');
        return;
      }
      const manifestEditorUrl = buildManifestEditorUrl(resource);
      if (manifestEditorUrl && resource.resource_id) {
        navigate(manifestEditorUrl);
        return;
      }
      // 有 manifest 来源但拿不到跳转所需信息:明确提示,而非进会报错的详情页
      showToast('无法定位该资源对应的 Manifest 位置', 'error');
      return;
    }
    // 非 manifest 资源:走原查看页面
    navigate(`/workspaces/${workspaceId}/resources/${resource.id}`);
  };

  const handleDeleteResource = (resource: Resource) => {
    if (resource.manifest_deployment_id) {
      showToast('此资源由 Manifest 管理，请在 manifest 编辑器中操作', 'info');
      return;
    }
    setResourceToDelete(resource);
    setShowDeleteDialog(true);
  };

  const confirmDelete = async () => {
    if (!resourceToDelete) return;

    try {
      setDeleteLoading(true);
      await api.delete(`/workspaces/${workspaceId}/resources/${resourceToDelete.id}`);
      showToast('资源删除成功', 'success');
      setShowDeleteDialog(false);
      setResourceToDelete(null);
      fetchResources();
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(message, 'error');
    } finally {
      setDeleteLoading(false);
    }
  };

  const handleViewVersions = (resource: Resource) => {
    navigate(`/workspaces/${workspaceId}/resources/${resource.id}/versions`);
  };

  // 移除前端过滤，使用后端搜索

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  };

  return (
    <div className={styles.container}>
      {/* Header */}
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <h2 className={styles.title}>Resources</h2>
          <p className={styles.subtitle}>
            管理Workspace中的所有资源配置
          </p>
          {isManifestManaged && manifestSummary && (
            <div
              style={{
                marginTop: 8,
                padding: '8px 12px',
                background: '#1e3a5f',
                border: '1px solid #2d5a8f',
                borderRadius: 4,
                color: '#cce0ff',
                fontSize: 13,
                display: 'flex',
                alignItems: 'center',
                gap: 8,
              }}
            >
              <span>
                此 workspace 由 Manifest{' '}
                <strong style={{ color: 'var(--surface)' }}>
                  {manifestSummary.manifest_name ?? manifestSummary.manifest_id}
                </strong>{' '}
                @ <strong style={{ color: 'var(--surface)' }}>{manifestSummary.active_tag}</strong> 管理。
                资源变更请在 manifest 编辑器中统一修改。
              </span>
              <span style={{ flex: 1 }} />
              {manifestSummary.manifest_id && manifestSummary.org_id && (
                <button
                  style={{
                    background: 'var(--brand)',
                    color: 'var(--surface)',
                    border: 'none',
                    padding: '4px 12px',
                    borderRadius: 3,
                    cursor: 'pointer',
                    fontSize: 12,
                  }}
                  onClick={() => {
                    const manifestEditorUrl = buildManifestEditorUrl();
                    if (manifestEditorUrl) navigate(manifestEditorUrl);
                  }}
                >
                  打开编辑器
                </button>
              )}
            </div>
          )}
        </div>
        <div className={styles.headerRight}>
          <div className={styles.splitButtonContainer} ref={dropdownRef}>
            <button
              onClick={handleAddResource}
              className={styles.addButton}
              disabled={isManifestManaged}
              style={isManifestManaged ? { opacity: 0.5, cursor: 'not-allowed' } : undefined}
              title={
                isManifestManaged
                  ? '此 workspace 由 Manifest 管理,资源变更请到 manifest 编辑器统一修改'
                  : ''
              }
            >
              + Add Resources
            </button>
            <button 
              className={styles.dropdownToggle}
              onClick={() => setShowDropdown(!showDropdown)}
              aria-label="更多操作"
            >
              <span className={styles.dropdownArrow}>▼</span>
            </button>
            {showDropdown && (
              <div className={styles.dropdownMenu}>
                <button 
                  className={styles.dropdownItem}
                  onClick={handleExportHCL}
                  disabled={exporting}
                >
                  {exporting ? '导出中...' : '导出资源 (HCL)'}
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Filters */}
      <div className={styles.filters}>
        <div className={styles.searchBox}>
          <input
            type="text"
            placeholder="搜索资源名称、类型或描述..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className={styles.searchInput}
          />
        </div>
        <label className={styles.checkbox}>
          <input
            type="checkbox"
            checked={includeInactive}
            onChange={(e) => setIncludeInactive(e.target.checked)}
          />
          <span>显示已删除的资源</span>
        </label>
      </div>

      {/* Resources List */}
      {loading ? (
        <div className={styles.loading}>加载中...</div>
      ) : resources.length === 0 ? (
        <div className={styles.emptyState}>
          <h3 className={styles.emptyTitle}>
            {searchTerm ? '未找到匹配的资源' : '暂无资源'}
          </h3>
          <p className={styles.emptyDesc}>
            {searchTerm
              ? '尝试使用其他关键词搜索'
              : '点击"Add Resources"按钮添加第一个资源'}
          </p>
          {!searchTerm && (
            <button
              onClick={handleAddResource}
              className={styles.emptyButton}
              disabled={isManifestManaged}
              title={isManifestManaged ? '此 workspace 由 Manifest 管理,资源变更请到 manifest 编辑器' : undefined}
              style={isManifestManaged ? { opacity: 0.5, cursor: 'not-allowed' } : undefined}
            >
              + Add Resources
            </button>
          )}
        </div>
      ) : (
        <div className={styles.resourcesList}>
          <div className={styles.tableHeader}>
            <div className={styles.colName}>名称</div>
            <div className={styles.colType}>类型</div>
            <div className={styles.colVersion}>版本</div>
            <div className={styles.colStatus}>状态</div>
            <div className={styles.colUpdated}>创建时间</div>
            <div className={styles.colActions}>操作</div>
          </div>
          {resources.map((resource) => (
            <div 
              key={resource.id} 
              className={styles.resourceRow}
              onClick={() => handleViewResource(resource)}
              style={{ cursor: 'pointer' }}
            >
              <div className={styles.colName}>
                <div className={styles.resourceName}>
                  {resource.resource_name}
                  {resource.manifest_deployment_id && (
                    <span
                      className={styles.manifestBadge}
                      onClick={(e) => {
                        e.stopPropagation();
                        const manifestEditorUrl = buildManifestEditorUrl(resource);
                        if (manifestEditorUrl) navigate(manifestEditorUrl);
                      }}
                      style={{ cursor: manifestSummary?.manifest_id ? 'pointer' : 'default' }}
                      title="Manifest 管理，点击跳转到 manifest 编辑器"
                    >
                      Manifest
                    </span>
                  )}
                </div>
                {resource.current_version?.change_summary && (
                  <div className={styles.resourceDesc} title={resource.current_version.change_summary}>
                    上次修改: {resource.current_version.change_summary}
                  </div>
                )}
                {/* 移动端显示的元信息行 */}
                <div className={styles.resourceMobileMeta}>
                  <span>{resource.resource_type}</span>
                  <span className={styles.resourceMetaSeparator}>•</span>
                  <span>v{resource.current_version?.version || 1}.0</span>
                </div>
              </div>
              <div className={styles.colType}>
                {resource.resource_type}
              </div>
              <div className={styles.colVersion}>
                <span className={styles.versionNumber}>
                  {resource.current_version?.version || 1}.0
                </span>
                {resource.current_version?.is_latest && (
                  <span className={styles.defaultBadge}>DEFAULT</span>
                )}
              </div>
              <div className={styles.colStatus}>
                <span
                  className={`${styles.statusBadge} ${
                    resource.is_active ? styles.statusEnabled : styles.statusDeprecated
                  }`}
                >
                  {resource.is_active ? 'Enabled' : 'Deprecated'}
                </span>
              </div>
              <div className={styles.colUpdated}>
                {formatDate(resource.created_at)}
              </div>
              <div className={styles.colActions}>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    handleDeleteResource(resource);
                  }}
                  className={styles.btnDelete}
                  disabled={!resource.is_active || !!resource.manifest_deployment_id}
                  title={
                    resource.manifest_deployment_id
                      ? '此资源由 Manifest 管理，请在 manifest 编辑器中操作'
                      : undefined
                  }
                >
                  删除
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Pagination */}
      {total > 0 && (
        <div className={styles.paginationContainer}>
          <div className={styles.paginationLeft}>
            <div className={styles.paginationInfo}>
              Showing {Math.min((page - 1) * pageSize + 1, total)} to{' '}
              {Math.min(page * pageSize, total)} of {total} resources
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
              Page {page} of {Math.ceil(total / pageSize)}
            </span>
            <button
              onClick={() => setPage(page + 1)}
              disabled={page >= Math.ceil(total / pageSize)}
              className={styles.paginationButton}
            >
              Next →
            </button>
          </div>
        </div>
      )}

      {/* Delete Confirmation Dialog */}
      <ConfirmDialog
        isOpen={showDeleteDialog}
        title="删除资源"
        message={`确定要删除资源 ${resourceToDelete?.resource_name} 吗？\n\n此操作将标记资源为已删除状态，不会立即从数据库中删除。`}
        confirmText={deleteLoading ? '删除中...' : '删除'}
        cancelText="取消"
        onConfirm={confirmDelete}
        onCancel={() => {
          setShowDeleteDialog(false);
          setResourceToDelete(null);
        }}
        type="danger"
      />
    </div>
  );
};

export default ResourcesTab;
