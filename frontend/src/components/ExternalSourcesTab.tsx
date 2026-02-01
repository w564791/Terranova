import React, { useState, useEffect, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { externalSourceService, getExternalSourceEmbeddingStatus, rebuildExternalSourceEmbedding } from '../services/cmdb';
import type {
  ExternalSourceResponse,
  CreateExternalSourceRequest,
  UpdateExternalSourceRequest,
  AuthHeaderInput,
  SyncLogResponse,
  EmbeddingStatus,
} from '../services/cmdb';
import styles from './ExternalSourcesTab.module.css';

// 格式化相对时间
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

// 状态徽章组件
const StatusBadge: React.FC<{ status?: string }> = ({ status }) => {
  if (!status) return <span className={styles.statusBadge}>-</span>;
  
  const statusClass = {
    success: styles.statusSuccess,
    failed: styles.statusFailed,
    running: styles.statusRunning,
  }[status] || '';
  
  return (
    <span className={`${styles.statusBadge} ${statusClass}`}>
      {status}
    </span>
  );
};

// Header编辑组件
const HeaderEditor: React.FC<{
  headers: AuthHeaderInput[];
  onChange: (headers: AuthHeaderInput[]) => void;
  existingHeaders?: { key: string; has_value: boolean }[];
}> = ({ headers, onChange, existingHeaders }) => {
  const addHeader = () => {
    onChange([...headers, { key: '', value: '' }]);
  };

  const removeHeader = (index: number) => {
    onChange(headers.filter((_, i) => i !== index));
  };

  const updateHeader = (index: number, field: 'key' | 'value', value: string) => {
    const newHeaders = [...headers];
    newHeaders[index] = { ...newHeaders[index], [field]: value };
    onChange(newHeaders);
  };

  // 检查是否已有值
  const hasExistingValue = (key: string) => {
    return existingHeaders?.find(h => h.key === key)?.has_value || false;
  };

  return (
    <div className={styles.headerEditor}>
      <label className={styles.label}>认证Headers</label>
      {headers.map((header, index) => (
        <div key={index} className={styles.headerRow}>
          <input
            type="text"
            className={styles.headerKeyInput}
            placeholder="Header Key (e.g., X-API-Key)"
            value={header.key}
            onChange={(e) => updateHeader(index, 'key', e.target.value)}
          />
          <div className={styles.headerValueWrapper}>
            <input
              type="password"
              className={styles.headerValueInput}
              placeholder={hasExistingValue(header.key) ? '••••••••（已设置，留空保持不变）' : 'Header Value'}
              value={header.value || ''}
              onChange={(e) => updateHeader(index, 'value', e.target.value)}
            />
            {hasExistingValue(header.key) && !header.value && (
              <span className={styles.hasValueIndicator}>已设置</span>
            )}
          </div>
          <button
            type="button"
            className={styles.removeHeaderButton}
            onClick={() => removeHeader(index)}
          >
            ×
          </button>
        </div>
      ))}
      <button type="button" className={styles.addHeaderButton} onClick={addHeader}>
        + 添加Header
      </button>
      <p className={styles.headerHint}>
        💡 Header值将加密存储，无法查看。如需修改请输入新值。
      </p>
    </div>
  );
};

// 字段映射编辑组件
const FieldMappingEditor: React.FC<{
  mapping: Record<string, string>;
  onChange: (mapping: Record<string, string>) => void;
}> = ({ mapping, onChange }) => {
  const fields = [
    { key: 'resource_type', label: '资源类型', placeholder: '$.type' },
    { key: 'resource_name', label: '资源名称', placeholder: '$.name' },
    { key: 'cloud_resource_id', label: '云资源ID', placeholder: '$.id' },
    { key: 'cloud_resource_name', label: '云资源名称', placeholder: '$.displayName' },
    { key: 'cloud_resource_arn', label: 'ARN', placeholder: '$.arn' },
    { key: 'description', label: '描述', placeholder: '$.description' },
    { key: 'tags', label: '标签', placeholder: '$.tags' },
  ];

  const updateField = (key: string, value: string) => {
    onChange({ ...mapping, [key]: value });
  };

  return (
    <div className={styles.fieldMappingEditor}>
      <label className={styles.label}>字段映射（JSONPath格式）</label>
      <div className={styles.fieldMappingGrid}>
        {fields.map((field) => (
          <div key={field.key} className={styles.fieldMappingRow}>
            <label className={styles.fieldLabel}>{field.label}</label>
            <input
              type="text"
              className={styles.fieldInput}
              placeholder={field.placeholder}
              value={mapping[field.key] || ''}
              onChange={(e) => updateField(field.key, e.target.value)}
            />
          </div>
        ))}
      </div>
    </div>
  );
};

// 同步日志组件
const SyncLogsModal: React.FC<{
  sourceId: string;
  sourceName: string;
  onClose: () => void;
}> = ({ sourceId, sourceName, onClose }) => {
  const [logs, setLogs] = useState<SyncLogResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const toast = useToast();

  useEffect(() => {
    const loadLogs = async () => {
      try {
        setLoading(true);
        const response = await externalSourceService.getSyncLogs(sourceId, 20);
        setLogs(response.logs || []);
      } catch (error) {
        console.error('Failed to load sync logs:', error);
        toast.error('加载同步日志失败');
      } finally {
        setLoading(false);
      }
    };
    loadLogs();
  }, [sourceId, toast]);

  return (
    <div className={styles.modalOverlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.modalHeader}>
          <h3>同步日志 - {sourceName}</h3>
          <button className={styles.closeButton} onClick={onClose}>×</button>
        </div>
        <div className={styles.modalContent}>
          {loading ? (
            <div className={styles.loading}>加载中...</div>
          ) : logs.length === 0 ? (
            <div className={styles.emptyLogs}>暂无同步日志</div>
          ) : (
            <table className={styles.logsTable}>
              <thead>
                <tr>
                  <th>开始时间</th>
                  <th>状态</th>
                  <th>新增</th>
                  <th>更新</th>
                  <th>删除</th>
                  <th>耗时</th>
                  <th>错误信息</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((log) => {
                  const duration = log.completed_at
                    ? Math.round((new Date(log.completed_at).getTime() - new Date(log.started_at).getTime()) / 1000)
                    : '-';
                  return (
                    <tr key={log.id}>
                      <td>{new Date(log.started_at).toLocaleString()}</td>
                      <td><StatusBadge status={log.status} /></td>
                      <td className={styles.countAdded}>+{log.resources_added}</td>
                      <td className={styles.countUpdated}>~{log.resources_updated}</td>
                      <td className={styles.countDeleted}>-{log.resources_deleted}</td>
                      <td>{duration}s</td>
                      <td className={styles.errorMessage}>{log.error_message || '-'}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
};

// 创建/编辑表单组件
const SourceForm: React.FC<{
  source?: ExternalSourceResponse;
  onSave: (data: CreateExternalSourceRequest | UpdateExternalSourceRequest) => Promise<void>;
  onCancel: () => void;
  saving: boolean;
}> = ({ source, onSave, onCancel, saving }) => {
  const [formData, setFormData] = useState<CreateExternalSourceRequest>({
    name: source?.name || '',
    description: source?.description || '',
    api_endpoint: source?.api_endpoint || '',
    http_method: (source?.http_method as 'GET' | 'POST') || 'GET',
    request_body: source?.request_body || '',
    auth_headers: source?.auth_headers?.map(h => ({ key: h.key, value: '' })) || [],
    response_path: source?.response_path || '',
    field_mapping: source?.field_mapping || {},
    primary_key_field: source?.primary_key_field || '',
    cloud_provider: source?.cloud_provider || '',
    account_id: source?.account_id || '',
    account_name: source?.account_name || '',
    region: source?.region || '',
    sync_interval_minutes: source?.sync_interval_minutes || 60,
    resource_type_filter: source?.resource_type_filter || '',
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // 验证必填字段
    if (!formData.name.trim()) {
      return;
    }
    if (!formData.api_endpoint.trim()) {
      return;
    }
    if (!formData.primary_key_field.trim()) {
      return;
    }

    // 过滤掉空的headers
    const filteredData = {
      ...formData,
      auth_headers: formData.auth_headers?.filter(h => h.key.trim()) || [],
    };

    await onSave(filteredData);
  };

  return (
    <form className={styles.form} onSubmit={handleSubmit}>
      <div className={styles.formSection}>
        <h4 className={styles.sectionTitle}>基本信息</h4>
        
        <div className={styles.formGroup}>
          <label className={styles.label}>名称 *</label>
          <input
            type="text"
            className={styles.input}
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            placeholder="例如：AWS CMDB - Production"
            required
          />
        </div>

        <div className={styles.formGroup}>
          <label className={styles.label}>描述</label>
          <textarea
            className={styles.textarea}
            value={formData.description}
            onChange={(e) => setFormData({ ...formData, description: e.target.value })}
            placeholder="数据源描述"
            rows={2}
          />
        </div>
      </div>

      <div className={styles.formSection}>
        <h4 className={styles.sectionTitle}>API配置</h4>
        
        <div className={styles.formRow}>
          <div className={styles.formGroup}>
            <label className={styles.label}>HTTP方法</label>
            <select
              className={styles.select}
              value={formData.http_method}
              onChange={(e) => setFormData({ ...formData, http_method: e.target.value as 'GET' | 'POST' })}
            >
              <option value="GET">GET</option>
              <option value="POST">POST</option>
            </select>
          </div>
          
          <div className={styles.formGroupFlex}>
            <label className={styles.label}>API端点 *</label>
            <input
              type="url"
              className={styles.input}
              value={formData.api_endpoint}
              onChange={(e) => setFormData({ ...formData, api_endpoint: e.target.value })}
              placeholder="https://cmdb.example.com/api/v1/resources"
              required
            />
          </div>
        </div>

        {formData.http_method === 'POST' && (
          <div className={styles.formGroup}>
            <label className={styles.label}>请求体</label>
            <textarea
              className={styles.textarea}
              value={formData.request_body}
              onChange={(e) => setFormData({ ...formData, request_body: e.target.value })}
              placeholder='{"filter": "active"}'
              rows={3}
            />
          </div>
        )}

        <HeaderEditor
          headers={formData.auth_headers || []}
          onChange={(headers) => setFormData({ ...formData, auth_headers: headers })}
          existingHeaders={source?.auth_headers}
        />

        <div className={styles.formGroup}>
          <label className={styles.label}>响应数据路径</label>
          <input
            type="text"
            className={styles.input}
            value={formData.response_path}
            onChange={(e) => setFormData({ ...formData, response_path: e.target.value })}
            placeholder="$.data.items（留空表示使用整个响应）"
          />
        </div>
      </div>

      <div className={styles.formSection}>
        <h4 className={styles.sectionTitle}>数据映射</h4>
        
        <div className={styles.formGroup}>
          <label className={styles.label}>主键字段 *</label>
          <input
            type="text"
            className={styles.input}
            value={formData.primary_key_field}
            onChange={(e) => setFormData({ ...formData, primary_key_field: e.target.value })}
            placeholder="$.id（用于唯一标识资源）"
            required
          />
          <p className={styles.hint}>主键用于增量同步时判断资源是否已存在</p>
        </div>

        <FieldMappingEditor
          mapping={formData.field_mapping || {}}
          onChange={(mapping) => setFormData({ ...formData, field_mapping: mapping })}
        />
      </div>

      <div className={styles.formSection}>
        <h4 className={styles.sectionTitle}>云环境配置</h4>
        
        <div className={styles.formRow}>
          <div className={styles.formGroup}>
            <label className={styles.label}>云提供商</label>
            <select
              className={styles.select}
              value={formData.cloud_provider}
              onChange={(e) => setFormData({ ...formData, cloud_provider: e.target.value })}
            >
              <option value="">选择云提供商</option>
              <option value="aws">AWS</option>
              <option value="azure">Azure</option>
              <option value="gcp">GCP</option>
              <option value="aliyun">阿里云</option>
              <option value="other">其他</option>
            </select>
          </div>
          
          <div className={styles.formGroup}>
            <label className={styles.label}>账户ID</label>
            <input
              type="text"
              className={styles.input}
              value={formData.account_id}
              onChange={(e) => setFormData({ ...formData, account_id: e.target.value })}
              placeholder="123456789012"
            />
          </div>
        </div>

        <div className={styles.formRow}>
          <div className={styles.formGroup}>
            <label className={styles.label}>账户名称</label>
            <input
              type="text"
              className={styles.input}
              value={formData.account_name}
              onChange={(e) => setFormData({ ...formData, account_name: e.target.value })}
              placeholder="Production Account"
            />
          </div>
          
          <div className={styles.formGroup}>
            <label className={styles.label}>区域</label>
            <input
              type="text"
              className={styles.input}
              value={formData.region}
              onChange={(e) => setFormData({ ...formData, region: e.target.value })}
              placeholder="us-east-1"
            />
          </div>
        </div>
      </div>

      <div className={styles.formSection}>
        <h4 className={styles.sectionTitle}>同步配置</h4>
        
        <div className={styles.formRow}>
          <div className={styles.formGroup}>
            <label className={styles.label}>同步间隔（分钟）</label>
            <input
              type="number"
              className={styles.input}
              value={formData.sync_interval_minutes}
              onChange={(e) => setFormData({ ...formData, sync_interval_minutes: parseInt(e.target.value) || 0 })}
              min={0}
              placeholder="60（0表示手动同步）"
            />
            <p className={styles.hint}>设置为0表示仅手动同步</p>
          </div>
          
          <div className={styles.formGroup}>
            <label className={styles.label}>资源类型过滤</label>
            <input
              type="text"
              className={styles.input}
              value={formData.resource_type_filter}
              onChange={(e) => setFormData({ ...formData, resource_type_filter: e.target.value })}
              placeholder="aws_security_group（可选）"
            />
          </div>
        </div>
      </div>

      <div className={styles.formActions}>
        <button type="button" className={styles.cancelButton} onClick={onCancel}>
          取消
        </button>
        <button type="submit" className={styles.saveButton} disabled={saving}>
          {saving ? '保存中...' : (source ? '更新' : '创建')}
        </button>
      </div>
    </form>
  );
};

// Embedding 状态徽章组件
const EmbeddingStatusBadge: React.FC<{ status: EmbeddingStatus | null; loading: boolean }> = ({ status, loading }) => {
  if (loading) {
    return <span style={{ background: 'rgba(156, 163, 175, 0.2)', color: '#6b7280', padding: '2px 8px', borderRadius: '4px', fontSize: '12px' }}>...</span>;
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
    const actualProgress = total_resources > 0 ? (with_embedding / total_resources) * 100 : 0;
    const remainingTasks = pending_tasks + processing_tasks;
    const estimatedMinutes = Math.ceil(remainingTasks * 5 / 60);
    
    // 区分：正在处理 vs 等待队列
    const isProcessing = processing_tasks > 0;
    const statusText = isProcessing 
      ? `处理中 ${actualProgress.toFixed(0)}% (${with_embedding}/${total_resources})`
      : `队列中 (${pending_tasks} 个任务等待)`;
    const bgColor = isProcessing ? 'rgba(59, 130, 246, 0.15)' : 'rgba(156, 163, 175, 0.15)';
    const textColor = isProcessing ? '#3b82f6' : '#6b7280';
    
    return (
      <span 
        style={{ background: bgColor, color: textColor, padding: '2px 8px', borderRadius: '4px', fontSize: '12px' }}
        title={`Embedding: ${with_embedding}/${total_resources}\n等待中: ${pending_tasks}, 处理中: ${processing_tasks}\n预计: ${estimatedMinutes > 0 ? estimatedMinutes + ' 分钟' : '不到 1 分钟'}`}
      >
        {statusText}
      </span>
    );
  }

  // 全部完成
  if (with_embedding === total_resources && with_embedding > 0) {
    return (
      <span 
        style={{ background: 'rgba(34, 197, 94, 0.15)', color: '#16a34a', padding: '2px 8px', borderRadius: '4px', fontSize: '12px' }}
        title={`All ${total_resources} resources have embeddings`}
      >
        Vector Ready
      </span>
    );
  }

  // 部分完成（没有 pending 任务，但 embedding 不完整）
  if (with_embedding > 0 && with_embedding < total_resources) {
    const progress = (with_embedding / total_resources) * 100;
    return (
      <span 
        style={{ background: 'rgba(234, 179, 8, 0.15)', color: '#ca8a04', padding: '2px 8px', borderRadius: '4px', fontSize: '12px' }}
        title={`Embedding: ${with_embedding}/${total_resources} (${progress.toFixed(0)}%)\n点击"重建 Embedding"生成剩余的向量`}
      >
        Embedding {progress.toFixed(0)}%
      </span>
    );
  }

  // 没有 embedding（需要重建）
  if (with_embedding === 0 && total_resources > 0) {
    return (
      <span 
        style={{ background: 'rgba(239, 68, 68, 0.15)', color: '#dc2626', padding: '2px 8px', borderRadius: '4px', fontSize: '12px' }}
        title={`${total_resources} 个资源没有 embedding\n点击"重建 Embedding"生成向量`}
      >
        需要重建
      </span>
    );
  }

  return null;
};

// 主组件
const ExternalSourcesTab: React.FC = () => {
  const toast = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  const [sources, setSources] = useState<ExternalSourceResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editingSource, setEditingSource] = useState<ExternalSourceResponse | undefined>();
  const [saving, setSaving] = useState(false);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [showLogsFor, setShowLogsFor] = useState<{ id: string; name: string } | null>(null);
  const [embeddingStatus, setEmbeddingStatus] = useState<EmbeddingStatus | null>(null);
  const [embeddingLoading, setEmbeddingLoading] = useState(false);

  // 从URL读取状态
  useEffect(() => {
    const action = searchParams.get('action');
    const sourceId = searchParams.get('source_id');
    
    if (action === 'create') {
      setShowForm(true);
      setEditingSource(undefined);
    } else if (action === 'edit' && sourceId) {
      // 等待sources加载完成后再设置编辑状态
      if (sources.length > 0) {
        const source = sources.find(s => s.source_id === sourceId);
        if (source) {
          setEditingSource(source);
          setShowForm(true);
        }
      }
    }
  }, [searchParams, sources]);

  // 加载 embedding 状态
  const loadEmbeddingStatus = useCallback(async () => {
    try {
      setEmbeddingLoading(true);
      const status = await getExternalSourceEmbeddingStatus();
      setEmbeddingStatus(status);
      return status;
    } catch (error) {
      console.error('Failed to load embedding status:', error);
      setEmbeddingStatus(null);
      return null;
    } finally {
      setEmbeddingLoading(false);
    }
  }, []);

  // 加载数据源列表
  const loadSources = useCallback(async () => {
    try {
      setLoading(true);
      const response = await externalSourceService.listExternalSources();
      setSources(response.sources || []);
    } catch (error) {
      console.error('Failed to load external sources:', error);
      toast.error('加载外部数据源列表失败');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    loadSources();
    loadEmbeddingStatus();
  }, [loadSources, loadEmbeddingStatus]);

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

  // 创建数据源
  const handleCreate = async (data: CreateExternalSourceRequest | UpdateExternalSourceRequest) => {
    try {
      setSaving(true);
      await externalSourceService.createExternalSource(data as CreateExternalSourceRequest);
      toast.success('外部数据源创建成功');
      setShowForm(false);
      setSearchParams({}); // 清除URL参数
      await loadSources();
    } catch (error: any) {
      console.error('Failed to create external source:', error);
      toast.error(error?.response?.data?.error || '创建外部数据源失败');
    } finally {
      setSaving(false);
    }
  };

  // 更新数据源
  const handleUpdate = async (data: CreateExternalSourceRequest | UpdateExternalSourceRequest) => {
    if (!editingSource) return;
    
    try {
      setSaving(true);
      await externalSourceService.updateExternalSource(editingSource.source_id, data as UpdateExternalSourceRequest);
      toast.success('外部数据源更新成功');
      setShowForm(false);
      setEditingSource(undefined);
      setSearchParams({}); // 清除URL参数
      await loadSources();
    } catch (error: any) {
      console.error('Failed to update external source:', error);
      toast.error(error?.response?.data?.error || '更新外部数据源失败');
    } finally {
      setSaving(false);
    }
  };

  // 删除数据源
  const handleDelete = async (source: ExternalSourceResponse) => {
    if (!window.confirm(`确定要删除数据源 "${source.name}" 吗？\n\n这将同时删除所有同步的资源数据。`)) {
      return;
    }

    try {
      await externalSourceService.deleteExternalSource(source.source_id);
      toast.success('外部数据源删除成功');
      await loadSources();
    } catch (error: any) {
      console.error('Failed to delete external source:', error);
      toast.error(error?.response?.data?.error || '删除外部数据源失败');
    }
  };

  // 同步数据源
  const handleSync = async (source: ExternalSourceResponse) => {
    try {
      setSyncingId(source.source_id);
      toast.info(`开始同步 "${source.name}"...`);
      await externalSourceService.syncExternalSource(source.source_id);
      toast.success(`"${source.name}" 同步完成`);
      await loadSources();
      // 延迟刷新 embedding 状态（因为 embedding 是异步生成的）
      setTimeout(() => {
        loadEmbeddingStatus();
      }, 2000);
    } catch (error: any) {
      console.error('Failed to sync external source:', error);
      toast.error(error?.response?.data?.error || `同步 "${source.name}" 失败`);
    } finally {
      setSyncingId(null);
    }
  };

  // 测试连接
  const handleTestConnection = async (source: ExternalSourceResponse) => {
    try {
      setTestingId(source.source_id);
      const result = await externalSourceService.testConnection(source.source_id);
      if (result.success) {
        toast.success(`连接成功！发现 ${result.sample_count} 个资源`);
      } else {
        toast.error(`连接失败: ${result.message}`);
      }
    } catch (error: any) {
      console.error('Failed to test connection:', error);
      toast.error(error?.response?.data?.error || '测试连接失败');
    } finally {
      setTestingId(null);
    }
  };

  // 切换启用状态
  const handleToggleEnabled = async (source: ExternalSourceResponse) => {
    try {
      await externalSourceService.updateExternalSource(source.source_id, {
        is_enabled: !source.is_enabled,
      });
      toast.success(source.is_enabled ? '已禁用数据源' : '已启用数据源');
      await loadSources();
    } catch (error: any) {
      console.error('Failed to toggle enabled:', error);
      toast.error('更新状态失败');
    }
  };

  // 编辑数据源
  const handleEdit = (source: ExternalSourceResponse) => {
    setEditingSource(source);
    setShowForm(true);
    setSearchParams({ action: 'edit', source_id: source.source_id });
  };

  // 取消编辑
  const handleCancel = () => {
    setShowForm(false);
    setEditingSource(undefined);
    setSearchParams({}); // 清除URL参数
  };

  // 显示创建表单
  const handleShowCreateForm = () => {
    setShowForm(true);
    setEditingSource(undefined);
    setSearchParams({ action: 'create' });
  };

  if (loading) {
    return (
      <div className={styles.loading}>
        <div className={styles.spinner}></div>
        <span>加载中...</span>
      </div>
    );
  }

  if (showForm) {
    return (
      <div className={styles.formContainer}>
        <h3 className={styles.formTitle}>
          {editingSource ? '编辑外部数据源' : '创建外部数据源'}
        </h3>
        <SourceForm
          source={editingSource}
          onSave={editingSource ? handleUpdate : handleCreate}
          onCancel={handleCancel}
          saving={saving}
        />
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <h3 className={styles.title}>外部数据源</h3>
          <EmbeddingStatusBadge status={embeddingStatus} loading={embeddingLoading} />
          {embeddingStatus && embeddingStatus.total_resources > 0 && (
            <button
              className={styles.actionButton}
              onClick={async () => {
                try {
                  toast.info('开始重建 Embedding...');
                  await rebuildExternalSourceEmbedding();
                  toast.success('Embedding 重建任务已创建');
                  loadEmbeddingStatus();
                } catch (error: any) {
                  toast.error(error?.response?.data?.message || '重建 Embedding 失败');
                }
              }}
              disabled={embeddingLoading || (embeddingStatus.pending_tasks > 0 || embeddingStatus.processing_tasks > 0)}
              title="重建所有外部数据源的 Embedding（全量重建）"
              style={{ fontSize: '12px', padding: '4px 8px' }}
            >
              {embeddingStatus.pending_tasks > 0 || embeddingStatus.processing_tasks > 0 ? '重建中...' : '重建 Embedding'}
            </button>
          )}
        </div>
        <button className={styles.createButton} onClick={handleShowCreateForm}>
          + 添加数据源
        </button>
      </div>

      {sources.length === 0 ? (
        <div className={styles.emptyState}>
          <p>暂无外部数据源</p>
          <p className={styles.emptyHint}>
            添加外部CMDB数据源，同步第三方系统的资源数据
          </p>
        </div>
      ) : (
        <div className={styles.sourcesList}>
          {sources.map((source) => (
            <div key={source.source_id} className={styles.sourceCard}>
              <div className={styles.sourceHeader}>
                <div className={styles.sourceInfo}>
                  <h4 className={styles.sourceName}>{source.name}</h4>
                  <span className={`${styles.enabledBadge} ${source.is_enabled ? styles.enabled : styles.disabled}`}>
                    {source.is_enabled ? '已启用' : '已禁用'}
                  </span>
                </div>
                <div className={styles.sourceActions}>
                  <button
                    className={styles.actionButton}
                    onClick={() => handleTestConnection(source)}
                    disabled={testingId === source.source_id}
                    title="测试连接"
                  >
                    {testingId === source.source_id ? '测试中...' : '测试'}
                  </button>
                  <button
                    className={styles.actionButton}
                    onClick={() => handleSync(source)}
                    disabled={syncingId === source.source_id || !source.is_enabled || !!(embeddingStatus && (embeddingStatus.pending_tasks > 0 || embeddingStatus.processing_tasks > 0))}
                    title={embeddingStatus && (embeddingStatus.pending_tasks > 0 || embeddingStatus.processing_tasks > 0) ? 'Embedding 生成中...' : '同步数据'}
                  >
                    {syncingId === source.source_id ? '同步中...' : '同步'}
                  </button>
                  {embeddingStatus && (embeddingStatus.pending_tasks > 0 || embeddingStatus.processing_tasks > 0) && (
                    <button
                      className={styles.actionButton}
                      onClick={() => handleSync(source)}
                      disabled={syncingId === source.source_id || !source.is_enabled}
                      title="重新同步（会重新生成 embedding）"
                      style={{ background: 'rgba(234, 179, 8, 0.15)', color: '#ca8a04' }}
                    >
                      重新同步
                    </button>
                  )}
                  <button
                    className={styles.actionButton}
                    onClick={() => setShowLogsFor({ id: source.source_id, name: source.name })}
                    title="查看日志"
                  >
                    日志
                  </button>
                  <button
                    className={styles.actionButton}
                    onClick={() => handleEdit(source)}
                    title="编辑"
                  >
                    编辑
                  </button>
                  <button
                    className={`${styles.actionButton} ${styles.deleteButton}`}
                    onClick={() => handleDelete(source)}
                    title="删除"
                  >
                    删除
                  </button>
                </div>
              </div>

              <div className={styles.sourceDetails}>
                <div className={styles.detailRow}>
                  <span className={styles.detailLabel}>API端点:</span>
                  <span className={styles.detailValue}>{source.api_endpoint}</span>
                </div>
                {source.cloud_provider && (
                  <div className={styles.detailRow}>
                    <span className={styles.detailLabel}>云提供商:</span>
                    <span className={styles.detailValue}>
                      {source.cloud_provider.toUpperCase()}
                      {source.account_id && ` (${source.account_id})`}
                    </span>
                  </div>
                )}
                <div className={styles.detailRow}>
                  <span className={styles.detailLabel}>主键字段:</span>
                  <span className={styles.detailValue}>{source.primary_key_field}</span>
                </div>
              </div>

              <div className={styles.sourceMeta}>
                <div className={styles.metaItem}>
                  <span className={styles.metaLabel}>最后同步:</span>
                  <span className={styles.metaValue}>
                    {formatRelativeTime(source.last_sync_at)}
                  </span>
                </div>
                <div className={styles.metaItem}>
                  <span className={styles.metaLabel}>状态:</span>
                  <StatusBadge status={source.last_sync_status} />
                </div>
                <div className={styles.metaItem}>
                  <span className={styles.metaLabel}>资源数:</span>
                  <span className={styles.metaValue}>{source.last_sync_count}</span>
                </div>
                <div className={styles.metaItem}>
                  <span className={styles.metaLabel}>同步间隔:</span>
                  <span className={styles.metaValue}>
                    {source.sync_interval_minutes > 0 ? `${source.sync_interval_minutes}分钟` : '手动'}
                  </span>
                </div>
                <button
                  className={styles.toggleButton}
                  onClick={() => handleToggleEnabled(source)}
                >
                  {source.is_enabled ? '禁用' : '启用'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {showLogsFor && (
        <SyncLogsModal
          sourceId={showLogsFor.id}
          sourceName={showLogsFor.name}
          onClose={() => setShowLogsFor(null)}
        />
      )}
    </div>
  );
};

export default ExternalSourcesTab;
