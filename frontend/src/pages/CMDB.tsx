import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Link, useSearchParams, useNavigate } from 'react-router-dom';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';
import api from '../services/api';
import cmdbService, { externalSourceService, getWorkspaceEmbeddingStatus, rebuildWorkspaceEmbedding, warmupEmbeddingCache, getEmbeddingCacheStats, getWarmupProgress, type EmbeddingStatus, type VectorSearchResponse, type EmbeddingCacheStats, type WarmupProgress } from '../services/cmdb';
import ConfirmDialog from '../components/ConfirmDialog';
import { useToast } from '../contexts/ToastContext';
import type {
  CMDBStats,
  ResourceSearchResult,
  ResourceTreeNode,
  WorkspaceResourceTree,
  ResourceTypeStat,
  WorkspaceResourceCount,
  SearchSuggestion,
  ExternalSourceResponse,
} from '../services/cmdb';
import ExternalSourcesTab from '../components/ExternalSourcesTab';
import styles from './CMDB.module.css';

// Copy to clipboard helper
const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
};

// Copyable value component
const CopyableValue: React.FC<{
  label: string;
  value: string | undefined | null;
  fieldKey: string;
  copiedField: string | null;
  onCopy: (value: string, field: string) => void;
}> = ({ label, value, fieldKey, copiedField, onCopy }) => {
  const hasValue = value !== undefined && value !== null && value !== '';
  
  return (
    <div className={styles.resourceDetailItem}>
      <span className={styles.detailLabel}>{label}:</span>
      <span 
        className={`${styles.detailValue} ${hasValue ? styles.copyable : ''}`}
        onClick={() => hasValue && onCopy(value!, fieldKey)}
        title={hasValue ? 'Click to copy' : undefined}
      >
        {hasValue ? value : '-'}
        {copiedField === fieldKey && <span className={styles.copiedToast}>Copied!</span>}
      </span>
    </div>
  );
};

// Resource details component with copy functionality
const ResourceDetails: React.FC<{ node: ResourceTreeNode }> = ({ node }) => {
  const [copiedField, setCopiedField] = useState<string | null>(null);

  const handleCopy = async (value: string, field: string) => {
    const success = await copyToClipboard(value);
    if (success) {
      setCopiedField(field);
      setTimeout(() => setCopiedField(null), 2000);
    }
  };

  // 收集所有非空字段
  const fields: { label: string; value: string | undefined; key: string }[] = [
    { label: 'Type', value: node.terraform_type, key: 'type' },
    { label: 'Name', value: node.terraform_name, key: 'tfname' },
    { label: 'Cloud ID', value: node.cloud_id, key: 'id' },
    { label: 'Cloud Name', value: node.cloud_name, key: 'name' },
    { label: 'ARN', value: node.cloud_arn, key: 'arn' },
    { label: 'Description', value: node.description, key: 'desc' },
    { label: 'Mode', value: node.mode, key: 'mode' },
    { label: 'Address', value: node.terraform_address, key: 'address' },
  ];

  return (
    <div className={styles.resourceDetails}>
      {fields.map(({ label, value, key }) => (
        <CopyableValue
          key={key}
          label={label}
          value={value}
          fieldKey={key}
          copiedField={copiedField}
          onCopy={handleCopy}
        />
      ))}
    </div>
  );
};

// TreeNode component with expand/collapse control
const TreeNode: React.FC<{
  node: ResourceTreeNode;
  level: number;
  expandAll?: boolean;
  workspaceId: string;
}> = ({ node, level, expandAll, workspaceId }) => {
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const hasChildren = node.children && node.children.length > 0;
  // 资源节点也可以展开显示详情
  const canExpand = hasChildren || node.type === 'resource';

  // Respond to expandAll changes
  useEffect(() => {
    if (expandAll !== undefined) {
      setExpanded(expandAll);
    }
  }, [expandAll]);

  // Only show jump link for root modules (level 0 and type module)
  const showJumpLink = node.type === 'module' && level === 0 && node.jump_url;

  // Handle copy cloud ID
  const handleCopyCloudId = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (node.cloud_id) {
      const success = await copyToClipboard(node.cloud_id);
      if (success) {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }
    }
  };

  return (
    <div className={level === 0 ? styles.treeNodeRoot : styles.treeNode}>
      <div
        className={styles.treeNodeHeader}
        onClick={() => canExpand && setExpanded(!expanded)}
        style={{ cursor: canExpand ? 'pointer' : 'default' }}
      >
        <span className={styles.expandIcon}>
          {canExpand ? (expanded ? '▼' : '▶') : '•'}
        </span>
        <span className={`${styles.nodeIcon} ${node.type === 'module' ? styles.moduleIcon : styles.resourceIcon}`}>
          {node.type === 'module' ? '[M]' : '[R]'}
        </span>
        <span className={styles.nodeName}>
          {node.type === 'module' ? node.name : `${node.terraform_type}.${node.terraform_name}`}
        </span>
        {node.type === 'module' && node.resource_count !== undefined && (
          <span className={styles.nodeCount}>({node.resource_count})</span>
        )}
        {node.type === 'resource' && node.cloud_id && (
          <span 
            className={`${styles.nodeCloudId} ${styles.copyable}`}
            onClick={handleCopyCloudId}
            title="Click to copy"
          >
            {node.cloud_id}
            {copied && <span className={styles.copiedToast}>Copied!</span>}
          </span>
        )}
        {showJumpLink && (
          <Link to={node.jump_url!} className={styles.jumpButton} onClick={(e) => e.stopPropagation()}>
            View →
          </Link>
        )}
      </div>
      {/* Resource details */}
      {node.type === 'resource' && expanded && (
        <ResourceDetails node={node} />
      )}
      {hasChildren && expanded && (
        <div className={styles.treeChildren}>
          {node.children!.map((child, index) => (
            <TreeNode 
              key={`${child.path || child.terraform_address}-${index}`} 
              node={child} 
              level={level + 1}
              expandAll={expandAll}
              workspaceId={workspaceId}
            />
          ))}
        </div>
      )}
    </div>
  );
};

// Format relative time
const formatRelativeTime = (dateString?: string): string => {
  if (!dateString) return 'Never';
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);
  
  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
};

// External resource node component
const ExternalResourceNode: React.FC<{
  resource: any;
}> = ({ resource }) => {
  const [copied, setCopied] = useState(false);

  const handleCopyCloudId = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (resource.cloud_resource_id) {
      const success = await copyToClipboard(resource.cloud_resource_id);
      if (success) {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }
    }
  };

  return (
    <div className={styles.treeNode}>
      <div className={styles.treeNodeHeader}>
        <span className={styles.expandIcon}>•</span>
        <span className={`${styles.nodeIcon} ${styles.resourceIcon}`}>[R]</span>
        <span className={styles.nodeName}>
          {resource.resource_type}.{resource.resource_name}
        </span>
        {resource.cloud_resource_id && (
          <span 
            className={`${styles.nodeCloudId} ${styles.copyable}`}
            onClick={handleCopyCloudId}
            title="Click to copy"
          >
            {resource.cloud_resource_id}
            {copied && <span className={styles.copiedToast}>Copied!</span>}
          </span>
        )}
      </div>
    </div>
  );
};

// External source node component
const ExternalSourceNode: React.FC<{
  source: ExternalSourceResponse;
}> = ({ source }) => {
  const [expanded, setExpanded] = useState(false);
  const [resources, setResources] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);

  // Load resources for this source
  const loadResources = useCallback(async () => {
    if (loaded) return;
    
    try {
      setLoading(true);
      // 使用cmdbService搜索
      const response = await cmdbService.searchResources(source.source_id, { limit: 100 });
      // 过滤出属于这个数据源的资源
      const filtered = response.results?.filter((r: any) => 
        r.terraform_address?.startsWith(`external.${source.source_id}.`)
      ) || [];
      setResources(filtered);
      setLoaded(true);
    } catch (error) {
      console.error('Failed to load resources:', error);
      setLoaded(true);
    } finally {
      setLoading(false);
    }
  }, [loaded, source.source_id]);

  useEffect(() => {
    if (expanded && !loaded) {
      loadResources();
    }
  }, [expanded, loaded, loadResources]);

  return (
    <div className={styles.treeNodeRoot}>
      <div
        className={styles.treeNodeHeader}
        onClick={() => setExpanded(!expanded)}
        style={{ cursor: 'pointer' }}
      >
        <span className={styles.expandIcon}>{expanded ? '▼' : '▶'}</span>
        <span className={`${styles.nodeIcon} ${styles.moduleIcon}`}>[S]</span>
        <span className={styles.nodeName}>{source.name}</span>
        <span className={styles.nodeCount}>({source.last_sync_count})</span>
        {source.cloud_provider && (
          <span className={styles.nodeCloudId} style={{ fontSize: '11px', opacity: 0.7 }}>
            {source.cloud_provider.toUpperCase()}
            {source.account_id && ` - ${source.account_id}`}
          </span>
        )}
      </div>
      
      {expanded && (
        <div className={styles.treeChildren}>
          {loading ? (
            <div className={styles.treeLoading}>Loading resources...</div>
          ) : resources.length === 0 ? (
            <div className={styles.treeEmpty}>No resources</div>
          ) : (
            resources.map((resource, index) => (
              <ExternalResourceNode 
                key={`${resource.terraform_address}-${index}`} 
                resource={resource} 
              />
            ))
          )}
        </div>
      )}
    </div>
  );
};

// External sources tree view component
const ExternalSourcesTreeView: React.FC = () => {
  const [expanded, setExpanded] = useState(false);
  const [sources, setSources] = useState<ExternalSourceResponse[]>([]);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const toast = useToast();

  // Load external sources
  const loadSources = useCallback(async () => {
    if (loaded) return;
    
    try {
      setLoading(true);
      const response = await externalSourceService.listExternalSources();
      setSources(response.sources.filter(s => s.is_enabled && s.last_sync_count > 0));
      setLoaded(true);
    } catch (error) {
      console.error('Failed to load external sources:', error);
      setLoaded(true);
    } finally {
      setLoading(false);
    }
  }, [loaded, toast]);

  useEffect(() => {
    if (expanded && !loaded) {
      loadSources();
    }
  }, [expanded, loaded, loadSources]);

  const totalResources = sources.reduce((sum, s) => sum + s.last_sync_count, 0);

  if (sources.length === 0 && loaded) {
    return null; // 没有外部数据源时不显示
  }

  return (
    <div className={styles.workspaceTreeContainer}>
      <div
        className={styles.workspaceTreeHeader}
        onClick={() => setExpanded(!expanded)}
        style={{ cursor: 'pointer' }}
      >
        <span className={styles.expandIcon}>{expanded ? '▼' : '▶'}</span>
        <span className={styles.workspaceIcon}>[E]</span>
        <span className={styles.workspaceName}>External CMDB Sources</span>
        <span className={styles.resourceCountBadge}>
          {totalResources} resources
        </span>
        <span className={styles.lastSyncedBadge} style={{ background: 'rgba(59, 130, 246, 0.1)', color: '#3b82f6' }}>
          External Data
        </span>
      </div>
      
      {expanded && (
        <div className={styles.workspaceTreeContent}>
          {loading ? (
            <div className={styles.treeLoading}>Loading...</div>
          ) : sources.length === 0 ? (
            <div className={styles.treeEmpty}>
              No external sources configured
            </div>
          ) : (
            <div className={styles.treeContainer}>
              {sources.map((source) => (
                <ExternalSourceNode key={source.source_id} source={source} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

// Embedding 状态徽章组件
const EmbeddingStatusBadge: React.FC<{ status: EmbeddingStatus | null; loading: boolean }> = ({ status, loading }) => {
  if (loading) {
    return <span className={styles.embeddingBadge} style={{ background: 'rgba(156, 163, 175, 0.2)', color: '#6b7280' }}>...</span>;
  }
  
  if (!status) {
    return null;
  }

  const { total_resources, with_embedding, pending_tasks, processing_tasks } = status;
  
  // 没有资源
  if (total_resources === 0) {
    return null;
  }

  // 正在处理中（有 pending 或 processing 任务）
  if (processing_tasks > 0 || pending_tasks > 0) {
    // 计算实际进度：已完成的 embedding 数量 / 总资源数量
    const actualProgress = total_resources > 0 ? (with_embedding / total_resources) * 100 : 0;
    const remainingTasks = pending_tasks + processing_tasks;
    const estimatedMinutes = Math.ceil(remainingTasks * 5 / 60); // 每个资源约 5 秒
    
    return (
      <span 
        className={styles.embeddingBadge} 
        style={{ background: 'rgba(59, 130, 246, 0.15)', color: '#3b82f6' }}
        title={`Embedding: ${with_embedding}/${total_resources}\nPending: ${pending_tasks}, Processing: ${processing_tasks}\n预计: ${estimatedMinutes} 分钟`}
      >
        Embedding {actualProgress.toFixed(0)}% ({with_embedding}/{total_resources})
      </span>
    );
  }

  // 全部完成（所有资源都有 embedding）
  if (with_embedding === total_resources && with_embedding > 0) {
    return (
      <span 
        className={styles.embeddingBadge} 
        style={{ background: 'rgba(34, 197, 94, 0.15)', color: '#16a34a' }}
        title={`All ${total_resources} resources have embeddings`}
      >
        Vector Ready
      </span>
    );
  }

  // 部分完成（有 embedding 但没有正在处理的任务）
  if (with_embedding > 0 && with_embedding < total_resources) {
    const progress = (with_embedding / total_resources) * 100;
    return (
      <span 
        className={styles.embeddingBadge} 
        style={{ background: 'rgba(234, 179, 8, 0.15)', color: '#ca8a04' }}
        title={`Embedding: ${with_embedding}/${total_resources} (${progress.toFixed(0)}%)\nSync to generate remaining embeddings`}
      >
        Embedding {progress.toFixed(0)}%
      </span>
    );
  }

  // 没有 embedding 且没有正在处理的任务 - 不显示任何徽章
  // 这样没有 sync 过的 workspace 就不会显示 0%
  return null;
};

// Workspace resource tree component
const WorkspaceTree: React.FC<{
  workspace: { workspace_id: string; name: string };
  initialResourceCount?: number;
  lastSyncedAt?: string;
  isAdmin: boolean;
  onSyncSuccess: () => void;
  onSyncError: (error: string) => void;
}> = ({ workspace, initialResourceCount, lastSyncedAt, isAdmin, onSyncSuccess, onSyncError }) => {
  const [expanded, setExpanded] = useState(false);
  const [expandAllNodes, setExpandAllNodes] = useState<boolean | undefined>(undefined);
  const [resourceTree, setResourceTree] = useState<WorkspaceResourceTree | null>(null);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [rebuilding, setRebuilding] = useState(false);
  const [showRebuildConfirm, setShowRebuildConfirm] = useState(false);
  const [embeddingStatus, setEmbeddingStatus] = useState<EmbeddingStatus | null>(null);
  const [embeddingLoading, setEmbeddingLoading] = useState(false);

  // Load embedding status
  const loadEmbeddingStatus = useCallback(async () => {
    try {
      setEmbeddingLoading(true);
      const status = await getWorkspaceEmbeddingStatus(workspace.workspace_id);
      setEmbeddingStatus(status);
      return status;
    } catch (error) {
      console.error('Failed to load embedding status:', error);
      setEmbeddingStatus(null);
      return null;
    } finally {
      setEmbeddingLoading(false);
    }
  }, [workspace.workspace_id]);

  // Auto-refresh embedding status when processing
  useEffect(() => {
    if (!embeddingStatus) return;
    
    const { pending_tasks, processing_tasks } = embeddingStatus;
    
    // 如果有正在处理的任务，每 3 秒刷新一次
    if (pending_tasks > 0 || processing_tasks > 0) {
      const timer = setInterval(async () => {
        const newStatus = await loadEmbeddingStatus();
        // 如果处理完成，停止轮询
        if (newStatus && newStatus.pending_tasks === 0 && newStatus.processing_tasks === 0) {
          clearInterval(timer);
        }
      }, 3000);
      
      return () => clearInterval(timer);
    }
  }, [embeddingStatus, loadEmbeddingStatus]);

  // Load resource tree
  const loadResourceTree = useCallback(async () => {
    if (loaded) return;
    
    try {
      setLoading(true);
      const tree = await cmdbService.getWorkspaceResourceTree(workspace.workspace_id);
      setResourceTree(tree);
      setLoaded(true);
      // 加载 embedding 状态
      loadEmbeddingStatus();
    } catch (error) {
      console.error('Failed to load resource tree:', error);
      setResourceTree(null);
      setLoaded(true);
    } finally {
      setLoading(false);
    }
  }, [workspace.workspace_id, loaded, loadEmbeddingStatus]);

  // Load embedding status on mount (不需要展开)
  useEffect(() => {
    loadEmbeddingStatus();
  }, [loadEmbeddingStatus]);

  // Load data when expanded
  useEffect(() => {
    if (expanded && !loaded) {
      loadResourceTree();
    }
  }, [expanded, loaded, loadResourceTree]);

  // Sync this workspace
  const handleSync = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      setSyncing(true);
      await cmdbService.syncWorkspace(workspace.workspace_id);
      setLoaded(false);
      await loadResourceTree();
      // 延迟刷新 embedding 状态（因为 embedding 是异步生成的）
      setTimeout(() => {
        loadEmbeddingStatus();
      }, 2000);
      onSyncSuccess();
    } catch (error) {
      console.error('Sync failed:', error);
      onSyncError(`Failed to sync workspace ${workspace.name}`);
    } finally {
      setSyncing(false);
    }
  };

  // Show rebuild confirmation dialog
  const handleRebuildClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    setShowRebuildConfirm(true);
  };

  // Rebuild embedding for this workspace (full rebuild)
  const handleRebuildConfirm = async () => {
    try {
      setRebuilding(true);
      setShowRebuildConfirm(false);
      await rebuildWorkspaceEmbedding(workspace.workspace_id);
      // 延迟刷新 embedding 状态
      setTimeout(() => {
        loadEmbeddingStatus();
      }, 2000);
      onSyncSuccess();
    } catch (error) {
      console.error('Rebuild failed:', error);
      onSyncError(`Failed to rebuild embedding for workspace ${workspace.name}`);
    } finally {
      setRebuilding(false);
    }
  };

  // Expand all nodes
  const handleExpandAll = (e: React.MouseEvent) => {
    e.stopPropagation();
    setExpandAllNodes(true);
  };

  // Collapse all nodes
  const handleCollapseAll = (e: React.MouseEvent) => {
    e.stopPropagation();
    setExpandAllNodes(false);
  };

  return (
    <div className={styles.workspaceTreeContainer}>
      <div
        className={styles.workspaceTreeHeader}
        onClick={() => setExpanded(!expanded)}
      >
        <span className={styles.expandIcon}>{expanded ? '▼' : '▶'}</span>
        <span className={styles.workspaceIcon}>[W]</span>
        <span className={styles.workspaceName}>{workspace.name}</span>
        <span className={styles.workspaceId}>({workspace.workspace_id})</span>
        {(resourceTree || initialResourceCount !== undefined) && (
          <span className={styles.resourceCountBadge}>
            {resourceTree ? resourceTree.total_resources : initialResourceCount} resources
          </span>
        )}
        {lastSyncedAt && (
          <span className={styles.lastSyncedBadge} title={new Date(lastSyncedAt).toLocaleString()}>
            Synced: {formatRelativeTime(lastSyncedAt)}
          </span>
        )}
        <EmbeddingStatusBadge status={embeddingStatus} loading={embeddingLoading} />
        <div className={styles.workspaceActions} onClick={(e) => e.stopPropagation()}>
          {expanded && resourceTree && resourceTree.tree && resourceTree.tree.length > 0 && (
            <>
              <button className={styles.expandCollapseButton} onClick={handleExpandAll}>
                Expand All
              </button>
              <button className={styles.expandCollapseButton} onClick={handleCollapseAll}>
                Collapse All
              </button>
            </>
          )}
          {isAdmin && (
            <>
              <button
                className={styles.syncWorkspaceButton}
                onClick={handleSync}
                disabled={syncing || rebuilding}
              >
                {syncing ? 'Syncing...' : 'Sync'}
              </button>
              <button
                className={styles.rebuildWorkspaceButton}
                onClick={handleRebuildClick}
                disabled={syncing || rebuilding}
                title="清空并重新生成所有 embedding"
              >
                {rebuilding ? 'Rebuilding...' : 'Rebuild'}
              </button>
            </>
          )}
        </div>
      </div>
      
      {/* Rebuild Confirmation Dialog */}
      <ConfirmDialog
        isOpen={showRebuildConfirm}
        title="重建 Embedding 索引"
        message={`确定要重建 "${workspace.name}" 的所有 embedding 吗？这将清空现有的 embedding 数据并重新生成，可能需要较长时间。`}
        confirmText="确认重建"
        cancelText="取消"
        type="warning"
        onConfirm={handleRebuildConfirm}
        onCancel={() => setShowRebuildConfirm(false)}
        loading={rebuilding}
      />

      {expanded && (
        <div className={styles.workspaceTreeContent}>
          {loading ? (
            <div className={styles.treeLoading}>Loading...</div>
          ) : resourceTree && resourceTree.tree && resourceTree.tree.length > 0 ? (
            <div className={styles.treeContainer}>
              {resourceTree.tree.map((node, index) => (
                <TreeNode 
                  key={`${node.path || node.terraform_address}-${index}`} 
                  node={node} 
                  level={0}
                  expandAll={expandAllNodes}
                  workspaceId={workspace.workspace_id}
                />
              ))}
            </div>
          ) : (
            <div className={styles.treeEmpty}>
              No resource index
              {isAdmin && (
                <button
                  className={styles.syncInlineButton}
                  onClick={handleSync}
                  disabled={syncing}
                >
                  {syncing ? 'Syncing...' : 'Click to sync'}
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

const CMDB: React.FC = () => {
  const { user } = useSelector((state: RootState) => state.auth);
  const isAdmin = user?.role === 'admin';
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const toast = useToast();

  // Get initial state from URL
  const initialTab = (searchParams.get('tab') as 'tree' | 'search' | 'external') || 'tree';
  const initialQuery = searchParams.get('q') || '';
  const initialType = searchParams.get('type') || '';

  // State
  const [activeTab, setActiveTab] = useState<'tree' | 'search' | 'external'>(initialTab);
  const [stats, setStats] = useState<CMDBStats | null>(null);
  const [statsLoading, setStatsLoading] = useState(true);

  // Search state
  const [searchQuery, setSearchQuery] = useState(initialQuery);
  const [searchResourceType, setSearchResourceType] = useState(initialType);
  const [searchResults, setSearchResults] = useState<ResourceSearchResult[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [hasSearched, setHasSearched] = useState(!!initialQuery);
  const [searchMode, setSearchMode] = useState<'vector' | 'keyword'>('vector'); // 默认使用 vector 搜索
  const [actualSearchMethod, setActualSearchMethod] = useState<'vector' | 'keyword' | null>(null); // 实际使用的搜索方式
  const [fallbackReason, setFallbackReason] = useState<string | null>(null); // 降级原因

  // Autocomplete state
  const [suggestions, setSuggestions] = useState<SearchSuggestion[]>([]);
  const [suggestionsLoading, setSuggestionsLoading] = useState(false);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [selectedSuggestionIndex, setSelectedSuggestionIndex] = useState(-1);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const suggestionsRef = useRef<HTMLDivElement>(null);
  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const autoSearchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Workspace list
  const [workspaces, setWorkspaces] = useState<{ workspace_id: string; name: string }[]>([]);
  const [workspacesLoading, setWorkspacesLoading] = useState(true);
  const [workspaceResourceData, setWorkspaceResourceData] = useState<Map<string, WorkspaceResourceCount>>(new Map());

  // Sync state
  const [syncing, setSyncing] = useState(false);

  // Cache warmup state
  const [warming, setWarming] = useState(false);
  const [cacheStats, setCacheStats] = useState<EmbeddingCacheStats | null>(null);
  const [warmupProgress, setWarmupProgress] = useState<WarmupProgress | null>(null);

  // Update URL when tab changes
  const handleTabChange = (tab: 'tree' | 'search' | 'external') => {
    setActiveTab(tab);
    const newParams = new URLSearchParams(searchParams);
    newParams.set('tab', tab);
    if (tab !== 'search') {
      newParams.delete('q');
      newParams.delete('type');
    }
    setSearchParams(newParams);
  };

  // Load stats
  const loadStats = useCallback(async () => {
    try {
      setStatsLoading(true);
      const data = await cmdbService.getStats();
      setStats(data);
    } catch (err) {
      console.error('Failed to load CMDB stats:', err);
    } finally {
      setStatsLoading(false);
    }
  }, []);

  // Load workspace list and resource counts
  const loadWorkspaces = useCallback(async () => {
    try {
      setWorkspacesLoading(true);
      
      // 并行加载workspace列表和资源数量
      const [wsResponse, countsResponse] = await Promise.all([
        api.get('/workspaces'),
        cmdbService.getWorkspaceResourceCounts().catch(() => ({ counts: [] }))
      ]);
      
      // 解析workspace列表
      let wsList: any[] = [];
      const response: any = wsResponse;
      if (response?.data?.items) {
        wsList = response.data.items;
      } else if (response?.items) {
        wsList = response.items;
      } else if (response?.data?.workspaces) {
        wsList = response.data.workspaces;
      } else if (response?.workspaces) {
        wsList = response.workspaces;
      } else if (Array.isArray(response?.data)) {
        wsList = response.data;
      } else if (Array.isArray(response)) {
        wsList = response;
      }
      setWorkspaces(wsList);
      
      // 构建资源数据映射（包含数量和同步时间）
      const dataMap = new Map<string, WorkspaceResourceCount>();
      if (countsResponse?.counts) {
        countsResponse.counts.forEach((c: WorkspaceResourceCount) => {
          dataMap.set(c.workspace_id, c);
        });
      }
      setWorkspaceResourceData(dataMap);
    } catch (err) {
      console.error('Failed to load workspaces:', err);
    } finally {
      setWorkspacesLoading(false);
    }
  }, []);

  // Load cache stats
  const loadCacheStats = useCallback(async () => {
    if (!isAdmin) return;
    try {
      const stats = await getEmbeddingCacheStats();
      setCacheStats(stats);
    } catch (err) {
      console.error('Failed to load cache stats:', err);
    }
  }, [isAdmin]);

  useEffect(() => {
    loadStats();
    loadWorkspaces();
    loadCacheStats();
  }, [loadStats, loadWorkspaces, loadCacheStats]);

  // Auto search if query in URL
  useEffect(() => {
    if (initialQuery && activeTab === 'search') {
      handleSearch();
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Search resources (form submit handler)
  const handleSearch = async (e?: React.FormEvent) => {
    e?.preventDefault();
    // 清除自动搜索定时器
    if (autoSearchTimerRef.current) {
      clearTimeout(autoSearchTimerRef.current);
    }
    performSearch(searchQuery);
  };

  // Handle search result click - navigate directly
  const handleResultClick = (result: ResourceSearchResult) => {
    // 外部数据源不跳转
    if (!result.jump_url) {
      return;
    }
    navigate(result.jump_url);
  };

  // Sync all workspaces
  const handleSyncAll = async () => {
    if (!isAdmin) return;
    
    try {
      setSyncing(true);
      toast.info('Starting sync for all workspaces...');
      await cmdbService.syncAllWorkspaces();
      // 后台异步执行，显示成功消息
      toast.success('Sync task started. Data will be updated in the background.');
      // 延迟刷新统计数据
      setTimeout(async () => {
        await loadStats();
      }, 3000);
    } catch (err) {
      console.error('Sync failed:', err);
      toast.error('Failed to start sync task. Please try again.');
    } finally {
      setSyncing(false);
    }
  };

  // Workspace sync callbacks
  const handleWorkspaceSyncSuccess = () => {
    toast.success('Workspace synced successfully');
    loadStats();
  };

  const handleWorkspaceSyncError = (errorMsg: string) => {
    toast.error(errorMsg);
  };

  // Fetch search suggestions with debounce
  const fetchSuggestions = useCallback(async (query: string) => {
    if (query.length < 2) {
      setSuggestions([]);
      setShowSuggestions(false);
      return;
    }

    try {
      setSuggestionsLoading(true);
      const response = await cmdbService.getSearchSuggestions(query, 10);
      setSuggestions(response.suggestions || []);
      setShowSuggestions(true);
      setSelectedSuggestionIndex(-1);
    } catch (err) {
      console.error('Failed to fetch suggestions:', err);
      setSuggestions([]);
    } finally {
      setSuggestionsLoading(false);
    }
  }, []);

  // Handle search input change with debounce
  const handleSearchInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setSearchQuery(value);

    // Clear previous timers
    if (debounceTimerRef.current) {
      clearTimeout(debounceTimerRef.current);
    }
    if (autoSearchTimerRef.current) {
      clearTimeout(autoSearchTimerRef.current);
    }

    // Set new debounce timer for suggestions
    debounceTimerRef.current = setTimeout(() => {
      fetchSuggestions(value);
    }, 300);

    // Set auto-search timer (longer delay for actual search)
    if (value.trim().length >= 2) {
      autoSearchTimerRef.current = setTimeout(() => {
        // 自动触发搜索
        performSearch(value);
      }, 600);
    }
  };

  // Perform search (extracted for reuse)
  const performSearch = async (query: string) => {
    if (!query.trim()) return;

    // Update URL with search params
    const newParams = new URLSearchParams();
    newParams.set('tab', 'search');
    newParams.set('q', query);
    if (searchResourceType) {
      newParams.set('type', searchResourceType);
    }
    setSearchParams(newParams);

    try {
      setSearchLoading(true);
      setHasSearched(true);
      setActualSearchMethod(null);
      setFallbackReason(null);
      setShowSuggestions(false); // 搜索时隐藏建议

      if (searchMode === 'vector') {
        // 使用 vector 搜索（支持自动降级）
        const response = await cmdbService.vectorSearch(query, {
          resource_type: searchResourceType || undefined,
          limit: 50,
        });
        setSearchResults(response.results || []);
        setActualSearchMethod(response.search_method);
        setFallbackReason(response.fallback_reason || null);
      } else {
        // 使用关键字搜索
        const response = await cmdbService.searchResources(query, {
          resource_type: searchResourceType || undefined,
          limit: 50,
        });
        setSearchResults(response.results || []);
        setActualSearchMethod('keyword');
        setFallbackReason(null);
      }
    } catch (err) {
      console.error('Search failed:', err);
      setSearchResults([]);
      setActualSearchMethod(null);
    } finally {
      setSearchLoading(false);
    }
  };

  // Handle suggestion selection
  const handleSuggestionSelect = (suggestion: SearchSuggestion) => {
    setSearchQuery(suggestion.value);
    setShowSuggestions(false);
    setSuggestions([]);
    // Trigger search immediately
    setTimeout(() => {
      searchInputRef.current?.form?.requestSubmit();
    }, 0);
  };

  // Handle keyboard navigation in suggestions
  const handleSearchKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (!showSuggestions || suggestions.length === 0) return;

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setSelectedSuggestionIndex(prev => 
          prev < suggestions.length - 1 ? prev + 1 : prev
        );
        break;
      case 'ArrowUp':
        e.preventDefault();
        setSelectedSuggestionIndex(prev => prev > 0 ? prev - 1 : -1);
        break;
      case 'Enter':
        if (selectedSuggestionIndex >= 0) {
          e.preventDefault();
          handleSuggestionSelect(suggestions[selectedSuggestionIndex]);
        }
        break;
      case 'Escape':
        setShowSuggestions(false);
        setSelectedSuggestionIndex(-1);
        break;
    }
  };

  // Handle input focus
  const handleSearchFocus = () => {
    if (suggestions.length > 0) {
      setShowSuggestions(true);
    }
  };

  // Handle input blur - delay to allow click on suggestion
  const handleSearchBlur = () => {
    setTimeout(() => {
      setShowSuggestions(false);
    }, 200);
  };

  // Get suggestion type label and style
  const getSuggestionTypeInfo = (type: string) => {
    switch (type) {
      case 'id':
        return { label: 'ID', className: styles.suggestionTypeId };
      case 'arn':
        return { label: 'ARN', className: styles.suggestionTypeArn };
      case 'name':
        return { label: 'Name', className: styles.suggestionTypeName };
      case 'description':
        return { label: 'Desc', className: styles.suggestionTypeDescription };
      default:
        return { label: type, className: '' };
    }
  };

  // Cleanup timers on unmount
  useEffect(() => {
    return () => {
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current);
      }
      if (autoSearchTimerRef.current) {
        clearTimeout(autoSearchTimerRef.current);
      }
    };
  }, []);

  return (
    <div className={styles.container}>
      {/* Page header */}
      <div className={styles.header}>
        <h1 className={styles.title}>CMDB Resource Index</h1>
        {isAdmin && (
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            {/* 缓存统计徽章 */}
            {cacheStats && cacheStats.total_count > 0 && (
              <span
                style={{
                  padding: '4px 10px',
                  borderRadius: '4px',
                  fontSize: '12px',
                  background: 'rgba(34, 197, 94, 0.15)',
                  color: '#16a34a',
                }}
                title={`缓存关键词: ${cacheStats.total_count}\n总命中次数: ${cacheStats.total_hits}\n平均命中: ${cacheStats.avg_hit_count?.toFixed(1) || 0}`}
              >
                Cache: {cacheStats.total_count} keywords
              </span>
            )}
            {/* 预热进度显示 */}
            {warmupProgress && warmupProgress.is_running && (
              <span
                style={{
                  padding: '4px 10px',
                  borderRadius: '4px',
                  fontSize: '12px',
                  background: 'rgba(59, 130, 246, 0.15)',
                  color: '#3b82f6',
                }}
                title={`内部: ${warmupProgress.internal_count}, 外部: ${warmupProgress.external_count}, 静态: ${warmupProgress.static_count}`}
              >
                Warming: {warmupProgress.processed_count}/{warmupProgress.total_keywords} ({Math.round((warmupProgress.processed_count / warmupProgress.total_keywords) * 100)}%)
              </span>
            )}
            {/* 预热按钮 */}
            <button
              className={styles.syncButton}
              onClick={async () => {
                try {
                  setWarming(true);
                  toast.info('Starting cache warmup...');
                  await warmupEmbeddingCache(false);
                  toast.success('Cache warmup started. Running in background.');
                  // 开始轮询进度
                  const pollProgress = async () => {
                    try {
                      const progress = await getWarmupProgress();
                      setWarmupProgress(progress);
                      if (progress.is_running) {
                        setTimeout(pollProgress, 2000);
                      } else {
                        // 完成后刷新缓存统计
                        const stats = await getEmbeddingCacheStats();
                        setCacheStats(stats);
                        setWarming(false);
                        toast.success(`Warmup completed: ${progress.new_count} new, ${progress.cached_count} cached, ${progress.failed_count} failed`);
                      }
                    } catch (e) {
                      console.error('Failed to poll progress:', e);
                      setWarming(false);
                    }
                  };
                  setTimeout(pollProgress, 1000);
                } catch (err) {
                  console.error('Warmup failed:', err);
                  toast.error('Failed to start cache warmup.');
                  setWarming(false);
                }
              }}
              disabled={warming || (warmupProgress?.is_running ?? false)}
              style={{ background: 'rgba(168, 85, 247, 0.15)', color: '#9333ea' }}
              title="预热 Embedding 缓存，加速向量搜索"
            >
              {warming || warmupProgress?.is_running ? 'Warming...' : 'Warmup'}
            </button>
            {/* 强制重新预热按钮 */}
            <button
              className={styles.syncButton}
              onClick={async () => {
                try {
                  setWarming(true);
                  toast.info('Starting force warmup (regenerating all)...');
                  await warmupEmbeddingCache(true);
                  toast.success('Force warmup started. Running in background.');
                  // 开始轮询进度
                  const pollProgress = async () => {
                    try {
                      const progress = await getWarmupProgress();
                      setWarmupProgress(progress);
                      if (progress.is_running) {
                        setTimeout(pollProgress, 2000);
                      } else {
                        const stats = await getEmbeddingCacheStats();
                        setCacheStats(stats);
                        setWarming(false);
                        toast.success(`Force warmup completed: ${progress.new_count} regenerated, ${progress.failed_count} failed`);
                      }
                    } catch (e) {
                      console.error('Failed to poll progress:', e);
                      setWarming(false);
                    }
                  };
                  setTimeout(pollProgress, 1000);
                } catch (err) {
                  console.error('Force warmup failed:', err);
                  toast.error('Failed to start force warmup.');
                  setWarming(false);
                }
              }}
              disabled={warming || (warmupProgress?.is_running ?? false)}
              style={{ background: 'rgba(239, 68, 68, 0.15)', color: '#dc2626' }}
              title="强制重新生成所有 Embedding 缓存"
            >
              Force Warmup
            </button>
            {/* 同步按钮 */}
            <button
              className={styles.syncButton}
              onClick={handleSyncAll}
              disabled={syncing}
            >
              {syncing ? 'Starting...' : 'Sync All'}
            </button>
          </div>
        )}
      </div>

      {/* Stats cards */}
      <div className={styles.statsGrid}>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Workspaces</div>
          <div className={styles.statValue}>
            {statsLoading ? '-' : stats?.total_workspaces || 0}
          </div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Managed Resources</div>
          <div className={styles.statValue}>
            {statsLoading ? '-' : stats?.total_resources || 0}
          </div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Modules</div>
          <div className={styles.statValue}>
            {statsLoading ? '-' : stats?.total_modules || 0}
          </div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Resource Types</div>
          <div className={styles.statValue}>
            {statsLoading ? '-' : stats?.resource_type_stats?.length || 0}
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className={styles.tabs}>
        <button
          className={`${styles.tab} ${activeTab === 'tree' ? styles.tabActive : ''}`}
          onClick={() => handleTabChange('tree')}
        >
          Resource Tree
        </button>
        <button
          className={`${styles.tab} ${activeTab === 'search' ? styles.tabActive : ''}`}
          onClick={() => handleTabChange('search')}
        >
          Search
        </button>
        {isAdmin && (
          <button
            className={`${styles.tab} ${activeTab === 'external' ? styles.tabActive : ''}`}
            onClick={() => handleTabChange('external')}
          >
            External Sources
          </button>
        )}
      </div>

      {/* Resource tree tab */}
      {activeTab === 'tree' && (
        <div className={styles.treeSection}>
          <div className={styles.treeSectionHeader}>
            <h3 className={styles.treeTitle}>All Workspace Resource Trees</h3>
            <span className={styles.workspaceCount}>
              {workspaces.length} Workspaces
            </span>
          </div>

          {workspacesLoading ? (
            <div className={styles.loading}>
              <div className={styles.spinner}></div>
            </div>
          ) : workspaces.length === 0 ? (
            <div className={styles.emptyState}>
              <p className={styles.emptyText}>No Workspaces</p>
            </div>
          ) : (
            <div className={styles.workspacesList}>
              {workspaces.map((ws) => (
                <WorkspaceTree
                  key={ws.workspace_id}
                  workspace={ws}
                  initialResourceCount={workspaceResourceData.get(ws.workspace_id)?.resource_count}
                  lastSyncedAt={workspaceResourceData.get(ws.workspace_id)?.last_synced_at}
                  isAdmin={isAdmin}
                  onSyncSuccess={handleWorkspaceSyncSuccess}
                  onSyncError={handleWorkspaceSyncError}
                />
              ))}
              
              {/* 外部数据源树（与workspace平级） */}
              <ExternalSourcesTreeView />
            </div>
          )}
        </div>
      )}

      {/* Search tab */}
      {activeTab === 'search' && (
        <>
          <div className={styles.searchSection}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
              <h3 className={styles.searchTitle} style={{ margin: 0 }}>Search Resources</h3>
              <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                <span style={{ fontSize: '13px', color: '#6b7280' }}>Search Mode:</span>
                <button
                  type="button"
                  onClick={() => setSearchMode('vector')}
                  style={{
                    padding: '4px 12px',
                    borderRadius: '4px',
                    border: 'none',
                    fontSize: '13px',
                    cursor: 'pointer',
                    background: searchMode === 'vector' ? 'rgba(59, 130, 246, 0.15)' : 'rgba(156, 163, 175, 0.1)',
                    color: searchMode === 'vector' ? '#3b82f6' : '#6b7280',
                    fontWeight: searchMode === 'vector' ? 500 : 400,
                  }}
                  title="AI 语义搜索（支持自然语言）"
                >
                  Vector
                </button>
                <button
                  type="button"
                  onClick={() => setSearchMode('keyword')}
                  style={{
                    padding: '4px 12px',
                    borderRadius: '4px',
                    border: 'none',
                    fontSize: '13px',
                    cursor: 'pointer',
                    background: searchMode === 'keyword' ? 'rgba(34, 197, 94, 0.15)' : 'rgba(156, 163, 175, 0.1)',
                    color: searchMode === 'keyword' ? '#16a34a' : '#6b7280',
                    fontWeight: searchMode === 'keyword' ? 500 : 400,
                  }}
                  title="精确关键字搜索"
                >
                  Keyword
                </button>
              </div>
            </div>
            <form className={styles.searchForm} onSubmit={handleSearch}>
              <div className={styles.searchInputWrapper}>
                <input
                  ref={searchInputRef}
                  type="text"
                  className={styles.searchInput}
                  placeholder="Enter resource ID, name or description to search..."
                  value={searchQuery}
                  onChange={handleSearchInputChange}
                  onKeyDown={handleSearchKeyDown}
                  onFocus={handleSearchFocus}
                  onBlur={handleSearchBlur}
                  autoComplete="off"
                />
                {/* Suggestions dropdown */}
                {showSuggestions && (
                  <div ref={suggestionsRef} className={styles.suggestionsDropdown}>
                    {suggestionsLoading ? (
                      <div className={styles.suggestionsLoading}>Loading suggestions...</div>
                    ) : suggestions.length === 0 ? (
                      <div className={styles.suggestionsEmpty}>No suggestions found</div>
                    ) : (
                      suggestions.map((suggestion, index) => {
                        const typeInfo = getSuggestionTypeInfo(suggestion.type);
                        return (
                          <div
                            key={`${suggestion.type}-${suggestion.value}-${index}`}
                            className={`${styles.suggestionItem} ${index === selectedSuggestionIndex ? styles.suggestionItemActive : ''}`}
                            onClick={() => handleSuggestionSelect(suggestion)}
                            onMouseEnter={() => setSelectedSuggestionIndex(index)}
                          >
                            <span className={`${styles.suggestionType} ${typeInfo.className}`}>
                              {typeInfo.label}
                            </span>
                            <div className={styles.suggestionContent}>
                              <div className={styles.suggestionLabel}>{suggestion.label}</div>
                              <div className={styles.suggestionMeta}>
                                <span className={styles.suggestionResourceType}>{suggestion.resource_type}</span>
                                {suggestion.is_external && (
                                  <span className={styles.suggestionExternalBadge}>External</span>
                                )}
                              </div>
                            </div>
                          </div>
                        );
                      })
                    )}
                  </div>
                )}
              </div>
              <select
                className={styles.filterSelect}
                value={searchResourceType}
                onChange={(e) => setSearchResourceType(e.target.value)}
              >
                <option value="">All Resource Types</option>
                {stats?.resource_type_stats?.map((stat: ResourceTypeStat) => (
                  <option key={stat.resource_type} value={stat.resource_type}>
                    {stat.resource_type} ({stat.count})
                  </option>
                ))}
              </select>
              <button
                type="submit"
                className={styles.searchButton}
                disabled={searchLoading || !searchQuery.trim()}
              >
                {searchLoading ? 'Searching...' : 'Search'}
              </button>
            </form>
          </div>

          {/* Search results */}
          {hasSearched && (
            <div className={styles.resultsSection}>
              <div className={styles.resultsHeader}>
                <h3 className={styles.resultsTitle}>Search Results</h3>
                <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                  {actualSearchMethod && (
                    <span 
                      style={{
                        padding: '2px 8px',
                        borderRadius: '4px',
                        fontSize: '12px',
                        background: actualSearchMethod === 'vector' ? 'rgba(59, 130, 246, 0.15)' : 'rgba(34, 197, 94, 0.15)',
                        color: actualSearchMethod === 'vector' ? '#3b82f6' : '#16a34a',
                      }}
                      title={fallbackReason || undefined}
                    >
                      {actualSearchMethod === 'vector' ? 'Vector Search' : 'Keyword Search'}
                      {fallbackReason && ' (fallback)'}
                    </span>
                  )}
                  <span className={styles.resultsCount}>
                    Found {searchResults.length} results
                  </span>
                </div>
              </div>

              {searchLoading ? (
                <div className={styles.loading}>
                  <div className={styles.spinner}></div>
                </div>
              ) : searchResults.length === 0 ? (
                <div className={styles.emptyState}>
                  <p className={styles.emptyText}>No matching resources found</p>
                </div>
              ) : (
                <div className={styles.resultsList}>
                  {searchResults.map((result, index) => (
                    <div 
                      key={`${result.workspace_id}-${result.terraform_address}-${index}`} 
                      className={result.jump_url ? styles.resultItemClickable : styles.resultItem}
                      onClick={() => handleResultClick(result)}
                      style={{ cursor: result.jump_url ? 'pointer' : 'default' }}
                    >
                      <div className={styles.resultHeader}>
                        <span className={styles.resourceType}>
                          {result.resource_type}
                        </span>
                        {result.source_type === 'external' ? (
                          <span className={styles.externalBadge}>
                            {result.external_source_name || 'External'}
                            {result.cloud_provider && ` (${result.cloud_provider.toUpperCase()})`}
                          </span>
                        ) : (
                          <span className={styles.workspaceBadge}>
                            {result.workspace_name || result.workspace_id}
                          </span>
                        )}
                      </div>
                      <h4 className={styles.resourceName}>
                        {result.cloud_resource_name || result.resource_name}
                      </h4>
                      <div className={styles.cloudInfo}>
                        {result.cloud_resource_id && (
                          <span className={styles.cloudId}>ID: {result.cloud_resource_id}</span>
                        )}
                        {result.description && (
                          <span className={styles.cloudName}>{result.description}</span>
                        )}
                      </div>
                      {result.cloud_resource_arn && (
                        <div className={styles.arnInfo}>
                          <span className={styles.arnLabel}>ARN:</span>
                          <span className={styles.arnValue} title={result.cloud_resource_arn}>
                            {result.cloud_resource_arn}
                          </span>
                        </div>
                      )}
                      {result.source_type === 'external' && (result.cloud_account_id || result.cloud_region) && (
                        <div className={styles.externalInfo}>
                          {result.cloud_account_id && (
                            <span className={styles.accountInfo}>
                              Account: {result.cloud_account_name || result.cloud_account_id}
                            </span>
                          )}
                          {result.cloud_region && (
                            <span className={styles.regionInfo}>Region: {result.cloud_region}</span>
                          )}
                        </div>
                      )}
                      <div className={styles.terraformAddress}>
                        {result.terraform_address}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Resource type stats */}
          {!hasSearched && stats?.resource_type_stats && stats.resource_type_stats.length > 0 && (
            <div className={styles.resultsSection}>
              <h3 className={styles.resultsTitle}>Resource Type Distribution</h3>
              <div className={styles.typeStats}>
                {stats.resource_type_stats.map((stat: ResourceTypeStat) => (
                  <div key={stat.resource_type} className={styles.typeStat}>
                    <span className={styles.typeStatName}>
                      {stat.resource_type}
                    </span>
                    <span className={styles.typeStatCount}>{stat.count}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}

      {/* External sources tab */}
      {activeTab === 'external' && isAdmin && (
        <div className={styles.treeSection}>
          <ExternalSourcesTab />
        </div>
      )}
    </div>
  );
};

export default CMDB;
