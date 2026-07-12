import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Link, useSearchParams, useNavigate } from 'react-router-dom';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';
import api from '../services/api';
import cmdbService, { externalSourceService, getWorkspaceEmbeddingStatus, rebuildWorkspaceEmbedding, warmupEmbeddingCache, getEmbeddingCacheStats, getWarmupProgress, type EmbeddingStatus, type VectorSearchResponse, type EmbeddingCacheStats, type WarmupProgress, type SearchSummaryResult, type SearchSummaryProgressEvent } from '../services/cmdb';
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

/**
 * 清洗 AI「试试」建议：去掉「可查询XXX：」类说明前缀，只保留可回填搜索框的查询词。
 * 例: "可查询存储桶策略详情：test-ken-manifest policy" → "test-ken-manifest policy"
 */
const normalizeAISearchSuggestion = (raw: string): string => {
  let s = (raw || '').trim().replace(/^["'`「」『』“”]+|["'`「」『』“”]+$/g, '');
  if (!s) return '';

  // 中文说明 + 冒号 + 查询词 → 取右侧
  for (const sep of ['：', ':'] as const) {
    const i = s.lastIndexOf(sep);
    if (i < 0) continue;
    const left = s.slice(0, i).trim();
    const right = s.slice(i + sep.length).trim();
    if (right && /[\u4e00-\u9fff]/.test(left)) {
      s = right;
      break;
    }
  }

  const prefixes = [
    '试试搜索', '试试查询', '建议搜索', '建议查询', '可以搜索', '可以查询',
    '可搜索', '可查询', '请搜索', '请查询',
    '搜索：', '查询：', '搜索:', '查询:', '试试：', '试试:', '建议：', '建议:',
    '试试 ', '建议 ', '搜索 ', '查询 ',
  ];
  for (const p of prefixes) {
    if (s.startsWith(p)) {
      s = s.slice(p.length).trim();
    }
  }

  return s.slice(0, 80);
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
    { label: 'AI Summary', value: node.resource_summary, key: 'summary' },
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
  const [expanded, setExpanded] = useState(false);
  const [detail, setDetail] = useState<any>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [showAttributes, setShowAttributes] = useState(false);
  const [copied, setCopied] = useState(false);

  const handleCopy = async (e: React.MouseEvent, value: string) => {
    e.stopPropagation();
    const success = await copyToClipboard(value);
    if (success) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleExpand = async () => {
    const next = !expanded;
    setExpanded(next);
    if (next && !detail) {
      try {
        setDetailLoading(true);
        const addr = resource.terraform_address || `external.__external__.${resource.cloud_resource_id}`;
        const res = await cmdbService.getResourceDetail('__external__', addr);
        setDetail(res);
      } catch (err) {
        console.error('Failed to load resource detail:', err);
      } finally {
        setDetailLoading(false);
      }
    }
  };

  const data = detail || resource;

  return (
    <div className={styles.treeNode}>
      <div className={styles.treeNodeHeader} onClick={handleExpand} style={{ cursor: 'pointer' }}>
        <span className={styles.expandIcon}>{expanded ? '▼' : '▶'}</span>
        <span className={`${styles.nodeIcon} ${styles.resourceIcon}`}>[R]</span>
        <span className={styles.nodeName}>
          {resource.resource_type ? `${resource.resource_type}.${resource.resource_name}` : resource.resource_name || resource.cloud_resource_id}
        </span>
        {resource.cloud_resource_id && (
          <span
            className={`${styles.nodeCloudId} ${styles.copyable}`}
            onClick={(e) => handleCopy(e, resource.cloud_resource_id)}
            title="Click to copy"
          >
            {resource.cloud_resource_id}
            {copied && <span className={styles.copiedToast}>Copied!</span>}
          </span>
        )}
      </div>
      {expanded && (
        <div className={styles.resourceDetails} style={{ marginLeft: '32px', padding: '8px 0' }}>
          {detailLoading ? (
            <div style={{ fontSize: '12px', color: '#6b7280' }}>Loading...</div>
          ) : (
            <>
              {data.cloud_resource_name && (
                <CopyableValue label="Cloud Name" value={data.cloud_resource_name} fieldKey="name" copiedField={null} onCopy={() => {}} />
              )}
              {data.cloud_resource_arn && (
                <CopyableValue label="ARN" value={data.cloud_resource_arn} fieldKey="arn" copiedField={null} onCopy={() => {}} />
              )}
              {data.description && (
                <CopyableValue label="Description" value={data.description} fieldKey="desc" copiedField={null} onCopy={() => {}} />
              )}
              {data.terraform_address && (
                <CopyableValue label="Address" value={data.terraform_address} fieldKey="addr" copiedField={null} onCopy={() => {}} />
              )}
              {data.resource_summary && (
                <div style={{ marginTop: '4px' }}>
                  <span style={{ fontSize: '12px', color: '#6b7280', fontWeight: 500 }}>AI Summary:</span>
                  <div style={{ fontSize: '13px', color: '#374151', lineHeight: '1.5', marginTop: '2px', whiteSpace: 'pre-wrap' }}>
                    {data.resource_summary}
                  </div>
                </div>
              )}
              {data.attributes && typeof data.attributes === 'object' && Object.keys(data.attributes).length > 0 && (
                <div style={{ marginTop: '4px' }}>
                  <span
                    style={{ fontSize: '12px', color: '#3b82f6', cursor: 'pointer', userSelect: 'none' }}
                    onClick={() => setShowAttributes(!showAttributes)}
                  >
                    {showAttributes ? '∧' : '∨'} Attributes ({Object.keys(data.attributes).length} keys)
                  </span>
                  {showAttributes && (
                    <pre style={{ fontSize: '11px', background: '#f9fafb', border: '1px solid #e5e7eb', borderRadius: '4px', padding: '8px', marginTop: '4px', overflow: 'auto', maxHeight: '300px' }}>
                      {JSON.stringify(data.attributes, null, 2)}
                    </pre>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      )}
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

  // Load resources using search API (detail fetched on expand per resource)
  const loadResources = useCallback(async () => {
    if (loaded) return;

    try {
      setLoading(true);
      const response = await cmdbService.searchResources(source.source_id, { limit: 100 });
      const filtered = (response.results || []).filter((r: any) =>
        r.terraform_address?.startsWith(`external.${source.source_id}.`)
      );
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
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [rebuilding, setRebuilding] = useState(false);
  const toast = useToast();

  // 组件挂载时立即加载（不等 expand）
  const loadSources = useCallback(async () => {
    if (loaded) return;

    try {
      setLoading(true);
      const response = await externalSourceService.listExternalSources();
      setSources((response.sources || []).filter(s => s.is_enabled && s.last_sync_count > 0));
      setLoaded(true);
    } catch (error) {
      console.error('Failed to load external sources:', error);
      setLoaded(true);
    } finally {
      setLoading(false);
    }
  }, [loaded, toast]);

  useEffect(() => {
    loadSources();
  }, [loadSources]);

  const totalResources = sources.reduce((sum, s) => sum + s.last_sync_count, 0);

  // 加载中或没有数据时不渲染
  if (loading || (sources.length === 0 && loaded)) {
    return null;
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
        <div className={styles.workspaceActions} onClick={(e) => e.stopPropagation()}>
          <button
            className={styles.rebuildWorkspaceButton}
            onClick={async (e) => {
              e.stopPropagation();
              try {
                setRebuilding(true);
                await rebuildWorkspaceEmbedding('__external__');
                toast.success('External embedding rebuild started');
              } catch (err) {
                toast.error('Rebuild failed');
              } finally {
                setRebuilding(false);
              }
            }}
            disabled={rebuilding}
          >
            {rebuilding ? 'Rebuilding...' : 'Rebuild'}
          </button>
        </div>
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
    const remainingTasks = pending_tasks + processing_tasks;
    const estimatedMinutes = Math.ceil(remainingTasks * 5 / 60); // 每个资源约 5 秒

    return (
      <span
        className={styles.embeddingBadge}
        style={{ background: 'rgba(59, 130, 246, 0.15)', color: '#3b82f6' }}
        title={`Embedding: ${with_embedding}/${total_resources}\nPending: ${pending_tasks}, Processing: ${processing_tasks}\n预计: ${estimatedMinutes} 分钟`}
      >
        处理中 ({remainingTasks} 个任务待完成)
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
        message={`确定要重建 "${workspace.name}" 的所有 embedding 吗？重建期间现有 embedding 仍可搜索，新数据会逐条覆盖，可能需要较长时间。`}
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
  const isAdmin = user?.is_system_admin;
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
  const [actualSearchMethod, setActualSearchMethod] = useState<'vector' | 'keyword' | 'hybrid' | null>(null); // 实际使用的搜索方式
  const [fallbackReason, setFallbackReason] = useState<string | null>(null); // 降级原因

  // AI 搜索结果解读 + 筛查（cmdb_search_summary，SSE 进度）
  // 默认屏蔽列表直到 AI 完成；用户可「跳过」立刻看原始结果，AI 仍后台出 summary（跳过后不应用筛选）
  const [aiSummary, setAiSummary] = useState<SearchSummaryResult | null>(null);
  const [aiSummaryLoading, setAiSummaryLoading] = useState(false);
  const [aiSummaryError, setAiSummaryError] = useState<string | null>(null);
  const [aiSummaryStep, setAiSummaryStep] = useState<string>('');
  const [aiSummaryProgress, setAiSummaryProgress] = useState<SearchSummaryProgressEvent | null>(null);
  const [showDroppedResults, setShowDroppedResults] = useState(false);
  /** blocked=等 AI；revealed=可展示列表 */
  const [resultsGate, setResultsGate] = useState<'blocked' | 'revealed'>('revealed');
  /** 用户点了跳过：展示全部召回，不应用 AI dropped */
  const [skipAIFilter, setSkipAIFilter] = useState(false);
  const aiSummaryRequestRef = useRef(0);
  const aiSummaryAbortRef = useRef<AbortController | null>(null);

  // 是否对列表应用 AI 筛查
  const applyAIFilter = !skipAIFilter && !!aiSummary?.dropped?.length;

  // AI 筛查：按 dropped.index 拆分主列表 / 已剔除（跳过时不应用）
  const { keptResults, droppedResults, dropReasonByIndex } = useMemo(() => {
    const dropped = applyAIFilter ? (aiSummary?.dropped || []) : [];
    const reasonMap = new Map<number, string>();
    for (const d of dropped) {
      if (typeof d.index === 'number' && d.index >= 0) {
        reasonMap.set(d.index, d.reason || '相关度低');
      }
    }
    // 未应用筛选时：全部当 kept；仍记录 AI 原始 dropped 供 summary 文案（见 aiDroppedCount）
    if (reasonMap.size === 0) {
      return {
        keptResults: searchResults.map((r, i) => ({ result: r, originalIndex: i })),
        droppedResults: [] as { result: ResourceSearchResult; originalIndex: number; reason: string }[],
        dropReasonByIndex: reasonMap,
      };
    }
    const kept: { result: ResourceSearchResult; originalIndex: number }[] = [];
    const droppedList: { result: ResourceSearchResult; originalIndex: number; reason: string }[] = [];
    searchResults.forEach((r, i) => {
      if (reasonMap.has(i)) {
        droppedList.push({ result: r, originalIndex: i, reason: reasonMap.get(i)! });
      } else {
        kept.push({ result: r, originalIndex: i });
      }
    });
    return { keptResults: kept, droppedResults: droppedList, dropReasonByIndex: reasonMap };
  }, [searchResults, aiSummary, applyAIFilter]);

  const aiDroppedCount = aiSummary?.dropped?.length || 0;

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
  const [hasExternalSources, setHasExternalSources] = useState(false);

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
      
      // 并行加载workspace列表、资源数量、外部数据源
      const [wsResponse, countsResponse, extResponse] = await Promise.all([
        api.get('/workspaces'),
        cmdbService.getWorkspaceResourceCounts().catch(() => ({ counts: [] })),
        externalSourceService.listExternalSources().catch(() => ({ sources: null, total: 0 }))
      ]);

      // 检查是否有外部数据源
      const extSources = (extResponse as any).sources || [];
      setHasExternalSources(extSources.length > 0);
      
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
  const [expandedResultIndex, setExpandedResultIndex] = useState<number | null>(null);
  const [expandedSummaryIndex, setExpandedSummaryIndex] = useState<number | null>(null);
  const [expandedDetail, setExpandedDetail] = useState<any>(null);
  const [expandedDetailLoading, setExpandedDetailLoading] = useState(false);

  // 点卡片主体：有 jump_url 跳转，否则不响应
  const handleResultClick = (result: ResourceSearchResult) => {
    if (result.jump_url) {
      navigate(result.jump_url);
    }
  };

  // 点倒三角：展开/收起瀑布详情
  const handleToggleExpand = async (result: ResourceSearchResult, index: number) => {
    if (expandedResultIndex === index) {
      setExpandedResultIndex(null);
      setExpandedDetail(null);
      return;
    }

    setExpandedResultIndex(index);
    setExpandedDetail(null);
    try {
      setExpandedDetailLoading(true);
      const addr = result.terraform_address || `external.__external__.${result.cloud_resource_id}`;
      const detail = await cmdbService.getResourceDetail(result.workspace_id, addr);
      setExpandedDetail(detail);
    } catch (err) {
      console.error('Failed to load resource detail:', err);
    } finally {
      setExpandedDetailLoading(false);
    }
  };

  // 点击空白处收起
  useEffect(() => {
    if (expandedResultIndex === null) return;
    const handleClickOutside = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      if (!target.closest(`.${styles.resultItem}`) && !target.closest(`.${styles.resultItemClickable}`)) {
        setExpandedResultIndex(null);
        setExpandedDetail(null);
      }
    };
    document.addEventListener('click', handleClickOutside);
    return () => document.removeEventListener('click', handleClickOutside);
  }, [expandedResultIndex]);

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
        performSearch(value, 'auto');
      }, 600);
    }
  };

  // 跳过 AI 筛选：立刻展示原始召回，AI 继续跑，完成后只补 summary 不筛列表
  const handleSkipAIFilter = useCallback(() => {
    setSkipAIFilter(true);
    setResultsGate('revealed');
    setShowDroppedResults(false);
  }, []);

  // AI 解读（SSE）：默认屏蔽列表；完成/失败后放行。跳过时不应用筛选但仍写 summary
  const fetchAISummary = useCallback(async (query: string, results: ResourceSearchResult[]) => {
    const requestId = ++aiSummaryRequestRef.current;

    // 取消上一次未完成的 SSE
    if (aiSummaryAbortRef.current) {
      aiSummaryAbortRef.current.abort();
    }
    const abort = new AbortController();
    aiSummaryAbortRef.current = abort;

    setAiSummaryLoading(true);
    setAiSummaryError(null);
    setAiSummary(null);
    setAiSummaryStep('准备中…');
    setAiSummaryProgress(null);

    try {
      console.log('[CMDB] fetchAISummary SSE start', { query, count: results.length, requestId });
      const summary = await cmdbService.searchSummarySSE(
        query,
        results,
        (event) => {
          if (requestId !== aiSummaryRequestRef.current) return;
          setAiSummaryProgress(event);
          if (event.step_name) {
            setAiSummaryStep(
              event.message
                ? `${event.step_name}：${event.message}`
                : event.step_name
            );
          }
        },
        abort.signal
      );
      if (requestId !== aiSummaryRequestRef.current) return;
      console.log('[CMDB] fetchAISummary SSE complete', summary);
      setAiSummary(summary);
      setAiSummaryStep('完成');
      setAiSummaryError(null);
      // 未跳过则按筛选结果展示；已跳过则只补 summary，列表保持全量
      setResultsGate('revealed');
    } catch (err: unknown) {
      if (requestId !== aiSummaryRequestRef.current) return;
      if ((err as { name?: string })?.name === 'AbortError') {
        console.log('[CMDB] fetchAISummary aborted', requestId);
        return;
      }
      console.warn('[CMDB] AI search summary failed:', err);
      const message =
        (err as { response?: { data?: { message?: string } }; message?: string })?.response?.data?.message ||
        (err as { message?: string })?.message ||
        'AI 解读暂不可用';
      setAiSummaryError(message);
      setAiSummary(null);
      setAiSummaryStep('');
      // 失败时自动放行原始结果，避免一直卡住
      setResultsGate('revealed');
    } finally {
      if (requestId === aiSummaryRequestRef.current) {
        setAiSummaryLoading(false);
      }
    }
  }, []);

  // Perform search (extracted for reuse)
  const performSearch = async (query: string, source: 'manual' | 'auto' = 'manual') => {
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
      // 新搜索时作废上一次 AI 解读 / 筛查
      if (aiSummaryAbortRef.current) {
        aiSummaryAbortRef.current.abort();
        aiSummaryAbortRef.current = null;
      }
      aiSummaryRequestRef.current += 1;
      setAiSummary(null);
      setAiSummaryError(null);
      setAiSummaryLoading(false);
      setAiSummaryStep('');
      setAiSummaryProgress(null);
      setShowDroppedResults(false);
      setSkipAIFilter(false);
      setResultsGate('blocked'); // 默认屏蔽列表，等 AI 或用户跳过

      // 使用混合搜索（向量 + 关键词并行）
      const response = await cmdbService.vectorSearch(query, {
        resource_type: searchResourceType || undefined,
        limit: 50,
        source,
      });
      const results = response.results || [];
      setSearchResults(results);
      setActualSearchMethod(response.search_method);
      setFallbackReason(response.fallback_reason || null);

      // 零结果：无需筛选，直接展示空态；仍跑 AI 给改写建议
      if (results.length === 0) {
        setResultsGate('revealed');
      }

      setAiSummaryLoading(true);
      setAiSummaryStep('搜索完成，启动 AI 解读…');
      setSearchLoading(false);
      void fetchAISummary(query, results);
    } catch (err) {
      console.error('Search failed:', err);
      setSearchResults([]);
      setActualSearchMethod(null);
      setAiSummary(null);
      setAiSummaryError(null);
      setAiSummaryLoading(false);
      setAiSummaryStep('');
      setResultsGate('revealed');
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
                  isAdmin={isAdmin || false}
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
              <span style={{
                padding: '4px 12px',
                borderRadius: '4px',
                fontSize: '12px',
                background: 'rgba(59, 130, 246, 0.1)',
                color: '#3b82f6',
              }}>
                Hybrid Search
              </span>
            </div>
            <form className={styles.searchForm} onSubmit={handleSearch}>
              <div className={styles.searchInputWrapper}>
                <input
                  ref={searchInputRef}
                  type="text"
                  className={styles.searchInput}
                  placeholder="Enter resource ID, name or description to search..."
                  maxLength={120}
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
                        background: actualSearchMethod === 'keyword' ? 'rgba(34, 197, 94, 0.15)' : 'rgba(59, 130, 246, 0.15)',
                        color: actualSearchMethod === 'keyword' ? '#16a34a' : '#3b82f6',
                      }}
                      title={fallbackReason || undefined}
                    >
                      {actualSearchMethod === 'hybrid' ? 'Hybrid Search' : actualSearchMethod === 'vector' ? 'Vector Search' : 'Keyword Search'}
                      {fallbackReason && ' (fallback)'}
                    </span>
                  )}
                  {resultsGate === 'revealed' && (
                    <span className={styles.resultsCount}>
                      {applyAIFilter && droppedResults.length > 0 && !showDroppedResults
                        ? `显示 ${keptResults.length} / 共 ${searchResults.length} 条`
                        : `Found ${searchResults.length} results`}
                      {applyAIFilter && droppedResults.length > 0 && !showDroppedResults && (
                        <span className={styles.aiFilteredHint}> · AI 已筛掉 {droppedResults.length}</span>
                      )}
                      {skipAIFilter && searchResults.length > 0 && (
                        <span className={styles.aiFilteredHint}> · 已跳过筛选</span>
                      )}
                    </span>
                  )}
                  {resultsGate === 'blocked' && !searchLoading && (
                    <span className={styles.resultsCount}>
                      已召回 {searchResults.length} 条 · 等待 AI 筛查
                    </span>
                  )}
                </div>
              </div>

              {/* AI 结果解读 + 筛查卡片 */}
              <div className={styles.aiSummaryCard}>
                <div className={styles.aiSummaryHeader}>
                  <span className={styles.aiSummaryBadge}>AI 解读</span>
                  {searchLoading && (
                    <span className={styles.aiSummaryStatus}>等待搜索结果…</span>
                  )}
                  {!searchLoading && aiSummaryLoading && (
                    <span className={styles.aiSummaryStatus}>
                      {aiSummaryProgress
                        ? `步骤 ${aiSummaryProgress.step}/${aiSummaryProgress.total_steps}`
                        : '分析中…'}
                    </span>
                  )}
                  {!searchLoading && !aiSummaryLoading && aiSummary && (
                    <span className={styles.aiSummaryStatus}>
                      {skipAIFilter
                        ? `解读完成${aiDroppedCount > 0 ? `（建议剔除 ${aiDroppedCount} 条，未应用）` : ''}`
                        : `解读 + 相关性筛查${aiDroppedCount > 0 ? ` · 已剔除 ${aiDroppedCount} 条` : ''}`}
                    </span>
                  )}
                  {!searchLoading && !aiSummaryLoading && !aiSummary && !aiSummaryError && (
                    <span className={styles.aiSummaryStatus}>就绪</span>
                  )}
                </div>

                {/* 等待 AI：屏蔽列表时给出说明（跳过按钮在下方结果占位区） */}
                {!searchLoading && resultsGate === 'blocked' && (
                  <div className={styles.aiResultsGate}>
                    <p className={styles.aiResultsGateText}>
                      已召回 <strong>{searchResults.length}</strong> 条结果，正在 AI 筛查与解读。
                      完成后将展示筛选后的结果与摘要；也可跳过，先看全部原始结果。
                    </p>
                  </div>
                )}

                {/* SSE 步骤条 */}
                {(aiSummaryLoading || aiSummaryProgress) && (
                  <div className={styles.aiProgressSteps}>
                    {['准备上下文', 'AI 解读与筛查', '完成'].map((name, i) => {
                      const stepNum = i + 1;
                      const current = aiSummaryProgress?.step || (aiSummaryLoading ? 1 : 0);
                      const done = !!aiSummary || current > stepNum || (aiSummaryProgress?.type === 'complete');
                      const active = aiSummaryLoading && current === stepNum;
                      return (
                        <div
                          key={name}
                          className={`${styles.aiProgressStep} ${done ? styles.aiProgressStepDone : ''} ${active ? styles.aiProgressStepActive : ''}`}
                        >
                          <span className={styles.aiProgressDot}>{done ? '✓' : stepNum}</span>
                          <span>{name}</span>
                        </div>
                      );
                    })}
                  </div>
                )}

                {/* 列表已 blocked 时下方占位区有「结果已暂存…」，不再重复 loading 文案；仅跳过后仍等 summary 时显示 */}
                {aiSummaryLoading && !aiSummary && resultsGate === 'revealed' && (
                  <div className={styles.aiSummaryLoading}>
                    <div className={styles.spinner} style={{ width: 18, height: 18 }} />
                    <span>{aiSummaryStep || '正在生成结果总览…'}</span>
                  </div>
                )}
                {aiSummaryError && !aiSummaryLoading && (
                  <div className={styles.aiSummaryError}>
                    {aiSummaryError.includes('未找到') || aiSummaryError.includes('AI 配置')
                      ? 'AI 解读未配置：请在 AI Config 中添加 cmdb_search_summary 场景'
                      : `AI 解读失败：${aiSummaryError}`}
                    <button
                      type="button"
                      className={styles.aiScreeningToggle}
                      style={{ marginLeft: 8 }}
                      onClick={() => {
                        setResultsGate(searchResults.length > 0 ? 'blocked' : 'revealed');
                        setSkipAIFilter(false);
                        void fetchAISummary(searchQuery, searchResults);
                      }}
                    >
                      重试
                    </button>
                  </div>
                )}
                {aiSummary && (
                  <div className={styles.aiSummaryBody}>
                    <p className={styles.aiSummaryOverview}>{aiSummary.overview}</p>
                    {aiSummary.highlights && aiSummary.highlights.length > 0 && (
                      <ul className={styles.aiSummaryHighlights}>
                        {aiSummary.highlights.map((h, i) => (
                          <li key={`${h.name}-${i}`}>
                            <strong>{h.name}</strong>
                            {h.reason ? <span> — {h.reason}</span> : null}
                          </li>
                        ))}
                      </ul>
                    )}
                    {aiSummary.groups && aiSummary.groups.length > 0 && (
                      <div className={styles.aiSummaryGroups}>
                        {aiSummary.groups.map((g, i) => (
                          <span key={`${g.label}-${i}`} className={styles.aiSummaryGroupChip}>
                            {g.label}
                            {typeof g.count === 'number' ? ` (${g.count})` : ''}
                          </span>
                        ))}
                      </div>
                    )}
                    {applyAIFilter && droppedResults.length > 0 && (
                      <div className={styles.aiScreeningBar}>
                        <span className={styles.aiScreeningText}>
                          已自动隐藏 {droppedResults.length} 条低相关结果
                        </span>
                        <button
                          type="button"
                          className={styles.aiScreeningToggle}
                          onClick={() => setShowDroppedResults((v) => !v)}
                        >
                          {showDroppedResults ? '仅看相关结果' : '显示已剔除'}
                        </button>
                      </div>
                    )}
                    {skipAIFilter && aiDroppedCount > 0 && (
                      <div className={styles.aiScreeningBar}>
                        <span className={styles.aiScreeningText}>
                          你已跳过筛选，当前展示全部 {searchResults.length} 条；AI 建议剔除 {aiDroppedCount} 条
                        </span>
                        <button
                          type="button"
                          className={styles.aiScreeningToggle}
                          onClick={() => {
                            setSkipAIFilter(false);
                            setShowDroppedResults(false);
                          }}
                        >
                          应用 AI 筛选
                        </button>
                      </div>
                    )}
                    {aiSummary.suggestions && aiSummary.suggestions.length > 0 && (
                      <div className={styles.aiSummarySuggestions}>
                        <span className={styles.aiSummarySuggestionsLabel}>试试：</span>
                        {aiSummary.suggestions.map((s, i) => {
                          // 兜底：清洗「说明：查询词」，避免把引导语回填进搜索框
                          const query = normalizeAISearchSuggestion(s);
                          if (!query) return null;
                          return (
                            <button
                              key={`${query}-${i}`}
                              type="button"
                              className={styles.aiSummarySuggestionBtn}
                              title={query !== s ? `将搜索：${query}` : undefined}
                              onClick={() => {
                                setSearchQuery(query);
                                void performSearch(query, 'manual');
                              }}
                            >
                              {query}
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </div>
                )}
              </div>

              {/* 列表：blocked 时不展示（等 AI 或用户跳过） */}
              {searchLoading ? (
                <div className={styles.loading}>
                  <div className={styles.spinner}></div>
                </div>
              ) : resultsGate === 'blocked' ? (
                <div className={styles.resultsGatePlaceholder}>
                  <div className={styles.spinner} style={{ width: 22, height: 22 }} />
                  <p>结果已暂存，AI 筛查完成后展示</p>
                  <button type="button" className={styles.aiSkipButton} onClick={handleSkipAIFilter}>
                    跳过，直接看结果
                  </button>
                </div>
              ) : searchResults.length === 0 ? (
                <div className={styles.emptyState}>
                  <p className={styles.emptyText}>No matching resources found</p>
                </div>
              ) : !showDroppedResults && keptResults.length === 0 && droppedResults.length > 0 ? (
                <div className={styles.emptyState}>
                  <p className={styles.emptyText}>相关结果为空，AI 已剔除全部低相关命中</p>
                  <button
                    type="button"
                    className={styles.aiScreeningToggle}
                    style={{ marginTop: 12 }}
                    onClick={() => setShowDroppedResults(true)}
                  >
                    查看已剔除的 {droppedResults.length} 条
                  </button>
                </div>
              ) : (
                <div className={styles.resultsList}>
                  {(showDroppedResults
                    ? searchResults.map((result, originalIndex) => ({
                        result,
                        originalIndex,
                        dropped: dropReasonByIndex.has(originalIndex),
                        reason: dropReasonByIndex.get(originalIndex),
                      }))
                    : keptResults.map(({ result, originalIndex }) => ({
                        result,
                        originalIndex,
                        dropped: false,
                        reason: undefined as string | undefined,
                      }))
                  ).map(({ result, originalIndex, dropped, reason }) => (
                    <div
                      key={`${result.workspace_id}-${result.terraform_address}-${originalIndex}`}
                      className={`${result.jump_url ? styles.resultItemClickable : styles.resultItem} ${expandedResultIndex === originalIndex ? styles.resultItemExpanded : ''} ${dropped ? styles.resultItemDropped : ''}`}
                      onClick={() => handleResultClick(result)}
                      style={{ cursor: result.jump_url ? 'pointer' : 'default' }}
                    >
                      <div className={styles.resultHeader}>
                        <span className={styles.resourceType}>
                          {result.resource_type}
                        </span>
                        {dropped && (
                          <span className={styles.droppedBadge} title={reason || 'AI 判定相关度低'}>
                            已剔除{reason ? `：${reason}` : ''}
                          </span>
                        )}
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
                        {result.is_resource_deleted && (
                          <span
                            className={styles.deletedBadge}
                            title="平台资源已删除，Terraform 尚未 apply"
                          >
                            已删除
                          </span>
                        )}
                        <button
                          type="button"
                          className={styles.expandToggle}
                          onClick={(e) => {
                            e.stopPropagation();
                            handleToggleExpand(result, originalIndex);
                          }}
                          title={expandedResultIndex === originalIndex ? '点击收起' : '点击展开详情'}
                        >
                          {expandedResultIndex === originalIndex ? '▲' : '▼'}
                        </button>
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
                      {result.resource_summary && (
                        <div
                          className={`${styles.resourceSummaryPreview} ${expandedSummaryIndex === originalIndex ? styles.expanded : ''}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            setExpandedSummaryIndex(expandedSummaryIndex === originalIndex ? null : originalIndex);
                          }}
                          title={expandedSummaryIndex === originalIndex ? '点击折叠' : '点击展开摘要'}
                        >
                          {/* 跳过第一行（标题行，和卡片上的资源名/ID 重复） */}
                          {result.resource_summary.includes('\n')
                            ? result.resource_summary.substring(result.resource_summary.indexOf('\n') + 1).trim()
                            : result.resource_summary}
                        </div>
                      )}

                      {/* Inline detail panel (accordion) - 仅展示 summary 和 tags，不暴露内部字段 */}
                      {expandedResultIndex === originalIndex && (
                        <div className={styles.resultDetailPanel} onClick={(e) => e.stopPropagation()}>
                          {expandedDetailLoading ? (
                            <div className={styles.resultDetailLoading}>Loading...</div>
                          ) : expandedDetail ? (
                            <>
                              {expandedDetail.resource_summary && (
                                <div className={styles.resultDetailSection}>
                                  <div className={styles.resultDetailTitle}>Summary</div>
                                  <div className={styles.resultDetailSummary}>{expandedDetail.resource_summary}</div>
                                </div>
                              )}
                              {expandedDetail.tags && Object.keys(expandedDetail.tags).length > 0 && (
                                <div className={styles.resultDetailSection}>
                                  <div className={styles.resultDetailTitle}>Tags</div>
                                  <div className={styles.resultDetailGrid}>
                                    {Object.entries(expandedDetail.tags).map(([key, value]) => (
                                      <div key={key} className={styles.resultDetailRow}>
                                        <span className={styles.resultDetailKey}>{key}</span>
                                        <span className={styles.resultDetailValue}>{String(value)}</span>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              )}
                              {!expandedDetail.resource_summary && (!expandedDetail.tags || Object.keys(expandedDetail.tags).length === 0) && (
                                <div className={styles.resultDetailLoading}>No summary available</div>
                              )}
                            </>
                          ) : (
                            <div className={styles.resultDetailLoading}>No details available</div>
                          )}
                        </div>
                      )}
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
