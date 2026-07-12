import React, { useState, useEffect, Component, type ReactNode } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { moduleDemoService, type ModuleDemo, type ModuleDemoVersion } from '../services/moduleDemos';
import { ModuleFormRenderer } from '../components/ModuleFormRenderer';
import HCLView from '../components/HCLView/HCLView';
import { schemaV2Service, type OpenAPISchema } from '../services/schemaV2';
import { useUIVersion } from '../hooks/useUIVersion';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './AddResources.module.css'; // 复用 AddResources 的样式

type ViewMode = 'view' | 'compare';

interface DiffField {
  field: string;
  type: 'added' | 'removed' | 'modified' | 'unchanged';
  oldValue?: any;
  newValue?: any;
  expanded?: boolean;
}

const DemoDetail: React.FC = () => {
  const { moduleId, demoId } = useParams<{ moduleId: string; demoId: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  const { isV3 } = useUIVersion();
  
  const [demo, setDemo] = useState<ModuleDemo | null>(null);
  const [schema, setSchema] = useState<OpenAPISchema | null>(null);
  const [schemaLoading, setSchemaLoading] = useState(true);
  const [dataViewMode, setDataViewMode] = useState<'form' | 'json'>('form');
  const [loading, setLoading] = useState(true);
  const [versions, setVersions] = useState<ModuleDemoVersion[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
  const [displayData, setDisplayData] = useState<any>({});
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [viewMode, setViewMode] = useState<ViewMode>('view');
  const [compareFromVersion, setCompareFromVersion] = useState<number | null>(null);
  const [compareToVersion, setCompareToVersion] = useState<number | null>(null);
  const [diffFields, setDiffFields] = useState<DiffField[]>([]);
  const [urlInitialized, setUrlInitialized] = useState(false);
  const [showRollbackDialog, setShowRollbackDialog] = useState(false);
  const [formRenderError, setFormRenderError] = useState(false);
  
  // 从 URL 参数获取初始 group（FormRenderer 内部的 tab）
  const getInitialGroup = (): string | undefined => {
    return searchParams.get('group') || undefined;
  };
  const [activeGroup, setActiveGroup] = useState<string | undefined>(getInitialGroup());

  useEffect(() => {
    loadDemo();
    loadVersions();
    loadSchema();
  }, [moduleId, demoId]);

  useEffect(() => {
    if (demo && selectedVersion === null && demo.current_version) {
      setSelectedVersion(demo.current_version.version);
      setDisplayData(demo.current_version.config_data);
    }
  }, [demo]);

  useEffect(() => {
    // 从URL参数初始化状态
    if (demo && versions.length > 0 && !urlInitialized) {
      const urlVersion = searchParams.get('version');
      const urlMode = searchParams.get('mode') as ViewMode;
      
      if (urlVersion) {
        const versionNum = parseInt(urlVersion);
        if (versions.some(v => v.version === versionNum)) {
          setSelectedVersion(versionNum);
        } else {
          setSelectedVersion(demo.current_version?.version || null);
        }
      } else {
        setSelectedVersion(demo.current_version?.version || null);
      }
      
      if (urlMode === 'compare') {
        setViewMode('compare');
        // 如果是对比模式，触发对比
        const versionNum = parseInt(urlVersion || '0');
        if (versionNum && demo.current_version?.version) {
          handleCompareVersions(versionNum, demo.current_version.version);
        }
      } else {
        setViewMode('view');
      }
      
      setUrlInitialized(true);
    }
  }, [demo, versions, urlInitialized]);

  const loadDemo = async () => {
    try {
      setLoading(true);
      const data = await moduleDemoService.getDemoById(parseInt(demoId!));
      setDemo(data);
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
      navigate(`/modules/${moduleId}`);
    } finally {
      setLoading(false);
    }
  };

  const loadVersions = async () => {
    try {
      const data = await moduleDemoService.getVersions(parseInt(demoId!));
      setVersions(data);
    } catch (error: any) {
      console.error('加载版本列表失败:', error);
    }
  };

  const loadSchema = async () => {
    try {
      setSchemaLoading(true);
      const schemaData = await schemaV2Service.getSchemaV2(parseInt(moduleId!));
      if (schemaData?.openapi_schema) {
        setSchema(schemaData.openapi_schema);
      }
    } catch (error: any) {
      console.warn('Failed to load schema:', error);
      // Schema 加载失败时使用 JSON 视图
      setDataViewMode('json');
    } finally {
      setSchemaLoading(false);
    }
  };

  const loadVersionData = async (version: number) => {
    try {
      const versionData = await moduleDemoService.getVersionById(
        versions.find(v => v.version === version)?.id || 0
      );
      setDisplayData(versionData.config_data);
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  const handleVersionChange = (version: number) => {
    setSelectedVersion(version);
    
    // 更新URL参数
    const newParams = new URLSearchParams(searchParams);
    newParams.set('version', version.toString());
    if (viewMode !== 'view') {
      newParams.set('mode', viewMode);
    } else {
      newParams.delete('mode');
    }
    setSearchParams(newParams, { replace: true });
    
    if (version !== demo?.current_version?.version) {
      loadVersionData(version);
    } else {
      setDisplayData(demo?.current_version?.config_data || {});
    }
  };

  const handleStartCompare = async () => {
    if (!selectedVersion || !demo?.current_version?.version) return;
    
    // 切换到对比模式
    setViewMode('compare');
    
    // 设置初始对比版本
    setCompareFromVersion(selectedVersion);
    setCompareToVersion(demo.current_version.version);
    
    // 更新URL参数
    const newParams = new URLSearchParams(searchParams);
    newParams.set('version', selectedVersion.toString());
    newParams.set('mode', 'compare');
    setSearchParams(newParams, { replace: true });
    
    // 对比选中版本和当前版本
    await handleCompareVersions(selectedVersion, demo.current_version.version);
  };

  const calculateDiff = (oldConfig: any, newConfig: any): DiffField[] => {
    const fields: DiffField[] = [];
    const allKeys = new Set([...Object.keys(oldConfig), ...Object.keys(newConfig)]);
    
    allKeys.forEach(key => {
      const oldValue = oldConfig[key];
      const newValue = newConfig[key];
      
      const oldExists = key in oldConfig;
      const newExists = key in newConfig;
      
      if (!oldExists && newExists) {
        // 新增字段
        fields.push({ field: key, type: 'added', newValue, expanded: false });
      } else if (oldExists && !newExists) {
        // 删除字段
        fields.push({ field: key, type: 'removed', oldValue, expanded: false });
      } else if (JSON.stringify(oldValue) !== JSON.stringify(newValue)) {
        // 修改字段
        fields.push({ field: key, type: 'modified', oldValue, newValue, expanded: false });
      } else {
        // 未变更字段
        fields.push({ field: key, type: 'unchanged', oldValue, newValue, expanded: false });
      }
    });
    
    return fields;
  };

  const handleCompareVersions = async (fromVer: number, toVer: number) => {
    try {
      console.log(`🔀 Comparing demo versions: v${fromVer} → v${toVer}`);
      
      const [fromVersion, toVersion] = await Promise.all([
        moduleDemoService.getVersionById(
          versions.find(v => v.version === fromVer)?.id || 0
        ),
        moduleDemoService.getVersionById(
          versions.find(v => v.version === toVer)?.id || 0
        )
      ]);
      
      const fromConfig = fromVersion.config_data || {};
      const toConfig = toVersion.config_data || {};
      
      const diff = calculateDiff(fromConfig, toConfig);
      console.log('📊 Diff fields:', diff);
      
      setDiffFields(diff);
    } catch (error: any) {
      console.error('❌ 对比版本失败:', error);
      showToast(extractErrorMessage(error), 'error');
    }
  };

  const toggleFieldExpansion = (index: number) => {
    setDiffFields(prev => prev.map((field, i) => 
      i === index ? { ...field, expanded: !field.expanded } : field
    ));
  };

  const formatValue = (value: any): string => {
    if (value === null || value === undefined) return '';
    if (typeof value === 'object') {
      return JSON.stringify(value, null, 2);
    }
    return String(value);
  };

  const handleEdit = () => {
    navigate(`/modules/${moduleId}/demos/${demoId}/edit`);
  };

  const handleDelete = async () => {
    try {
      await moduleDemoService.deleteDemo(parseInt(demoId!));
      showToast('Demo 删除成功', 'success');
      navigate(`/modules/${moduleId}`);
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  const handleRollbackVersion = () => {
    if (!selectedVersion || !demo?.current_version?.version) return;
    
    if (selectedVersion === demo.current_version.version) {
      showToast('当前已是最新版本', 'info');
      return;
    }
    
    // 显示确认对话框
    setShowRollbackDialog(true);
  };

  const confirmRollback = async () => {
    setShowRollbackDialog(false);
    
    if (!selectedVersion) return;
    
    try {
      // 找到选中版本的ID
      const versionToRollback = versions.find(v => v.version === selectedVersion);
      if (!versionToRollback) {
        showToast('找不到指定版本', 'error');
        return;
      }
      
      const result = await moduleDemoService.rollbackToVersion(
        parseInt(demoId!),
        versionToRollback.id
      );
      
      showToast(`成功回滚到版本 v${selectedVersion}，新版本号为 v${result.current_version?.version}`, 'success');
      
      // 重新加载数据
      await loadDemo();
      await loadVersions();
      
      // 切换到新的当前版本
      if (result.current_version?.version) {
        setSelectedVersion(result.current_version.version);
      }
      
      // 清理URL参数
      const newParams = new URLSearchParams();
      setSearchParams(newParams, { replace: true });
    } catch (error: any) {
      console.error('版本回滚失败:', error);
      showToast(extractErrorMessage(error), 'error');
    }
  };

  // 处理 FormRenderer 内部 group 切换
  const handleGroupChange = (groupId: string) => {
    setActiveGroup(groupId);
    
    // 更新 URL 参数
    const newParams = new URLSearchParams(searchParams);
    newParams.set('group', groupId);
    setSearchParams(newParams, { replace: true });
  };

  const handleBack = () => {
    navigate(`/modules/${moduleId}`);
  };

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

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.header}>
          <h1 className={styles.title}>加载中...</h1>
        </div>
      </div>
    );
  }

  if (!demo) {
    return (
      <div className={styles.container}>
        <div className={styles.header}>
          <button 
            onClick={handleBack}
            style={{
              padding: '8px 16px',
              background: 'var(--bg)',
              border: '1px solid var(--line-2)',
              borderRadius: '6px',
              cursor: 'pointer',
              fontSize: '14px',
              color: 'var(--ink-2)'
            }}
          >
            ← 返回模块
          </button>
          <h1 className={styles.title}>Demo 不存在</h1>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {/* 只在非对比模式下显示header */}
      {viewMode !== 'compare' && (
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <h1 className={styles.title}>查看 Demo</h1>
          </div>
          
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <span className={styles.resourceType}>{demo.name}</span>
            <span className={styles.resourceName}>v{demo.current_version?.version || 1}</span>
          </div>
        </div>
      )}

      <div className={styles.content}>
        <div className={styles.configureStep}>
          {/* 查看模式 */}
          {viewMode === 'view' && (
            <>
              {/* Demo 信息卡片 */}
              <div className={styles.resourceInfoCard}>
            <div className={styles.infoRow}>
              <span className={styles.infoLabel}>Demo 名称:</span>
              <span className={styles.infoValue}>{demo.name}</span>
            </div>
            {demo.description && (
              <div className={styles.infoRow}>
                <span className={styles.infoLabel}>描述:</span>
                <span className={styles.infoValue}>{demo.description}</span>
              </div>
            )}
            {demo.usage_notes && (
              <div className={styles.infoRow}>
                <span className={styles.infoLabel}>使用说明:</span>
                <span className={styles.infoValue}>{demo.usage_notes}</span>
              </div>
            )}
            <div className={styles.infoRow}>
              <span className={styles.infoLabel}>创建时间:</span>
              <span className={styles.infoValue}>{formatDate(demo.created_at)}</span>
            </div>
            <div className={styles.infoRow}>
              <span className={styles.infoLabel}>更新时间:</span>
              <span className={styles.infoValue}>{formatDate(demo.updated_at)}</span>
            </div>
            {demo.current_version?.change_summary && (
              <div className={styles.infoRow}>
                <span className={styles.infoLabel}>上次修改:</span>
                <span className={styles.infoValue}>{demo.current_version.change_summary}</span>
              </div>
            )}
          </div>

              <div className={styles.resourceInfoCard}>
                <h2 className={styles.stepTitle}>配置数据</h2>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
                  <div className={styles.viewToggle}>
                    <button
                      className={`${styles.viewButton} ${dataViewMode === 'form' ? styles.viewButtonActive : ''}`}
                      onClick={() => setDataViewMode('form')}
                    >
                      表单视图
                    </button>
                    <button
                      className={`${styles.viewButton} ${dataViewMode === 'json' ? styles.viewButtonActive : ''}`}
                      onClick={() => setDataViewMode('json')}
                    >
                      {isV3 ? 'HCL 视图' : 'JSON视图'}
                    </button>
                  </div>
                  
                  {/* 右侧按钮组 */}
                  <div style={{ display: 'flex', alignItems: 'center', gap: '12px', minWidth: '280px', justifyContent: 'flex-end' }}>
                    {/* 版本选择 */}
                    <select
                      value={selectedVersion || ''}
                      onChange={(e) => handleVersionChange(parseInt(e.target.value))}
                      style={{
                        padding: '10px 12px',
                        border: '1px solid var(--line-2)',
                        borderRadius: '6px',
                        fontSize: '14px',
                        background: 'white',
                        cursor: 'pointer',
                        minWidth: '150px',
                        height: '40px'
                      }}
                    >
                      {versions.map((v) => (
                        <option key={v.id} value={v.version}>
                          v{v.version} {v.is_latest ? '(当前)' : ''}
                        </option>
                      ))}
                    </select>
                    
                    {/* 对比版本按钮 */}
                    <div style={{ width: '100px' }}>
                      {selectedVersion && selectedVersion !== demo?.current_version?.version && (
                        <button
                          onClick={handleStartCompare}
                          style={{
                            padding: '10px 16px',
                            background: 'var(--brand)',
                            color: 'white',
                            border: 'none',
                            borderRadius: '6px',
                            cursor: 'pointer',
                            fontSize: '14px',
                            fontWeight: 500,
                            width: '100%',
                            height: '40px'
                          }}
                        >
                          对比版本
                        </button>
                      )}
                    </div>
                  </div>
                </div>

                {formRenderError && dataViewMode === 'json' && (
                  <div style={{
                    padding: '12px 16px',
                    background: 'var(--amber-soft)',
                    border: '1px solid var(--amber)',
                    borderRadius: '6px',
                    color: 'var(--amber-hover)',
                    marginBottom: '16px'
                  }}>
                     表单渲染失败，已自动切换到JSON视图
                  </div>
                )}

                <div>
                  {schemaLoading ? (
                    <div style={{ padding: '20px', textAlign: 'center', color: 'var(--ink-3)' }}>
                      加载 Schema 中...
                    </div>
                  ) : dataViewMode === 'form' && !formRenderError ? (
                    schema ? (
                      <ErrorBoundary
                        onError={() => {
                          setFormRenderError(true);
                          setDataViewMode('json');
                          showToast('表单渲染失败，已切换到JSON视图', 'warning');
                        }}
                      >
                        <div style={{ 
                          border: '1px solid var(--line-2)', 
                          borderRadius: '8px', 
                          padding: '16px',
                          background: 'var(--bg)'
                        }}>
                          <ModuleFormRenderer
                            schema={schema}
                            initialValues={displayData}
                            readOnly={true}
                            showVersionBadge={false}
                            activeGroupId={activeGroup}
                            onGroupChange={handleGroupChange}
                          />
                        </div>
                      </ErrorBoundary>
                    ) : (
                      <div style={{ 
                        textAlign: 'center', 
                        padding: '40px', 
                        background: 'var(--amber-soft)',
                        borderRadius: '6px',
                        color: 'var(--amber-hover)'
                      }}>
                        <p>该模块暂无 Schema 定义</p>
                        <p style={{ fontSize: '14px', marginTop: '8px' }}>
                          请切换到 JSON 视图查看配置
                        </p>
                      </div>
                    )
                  ) : isV3 ? (
                    <HCLView
                      data={displayData}
                      moduleName={demo?.name?.toLowerCase().replace(/[^a-z0-9_-]/g, '_') || 'demo'}
                    />
                  ) : (
                    <div style={{
                      background: 'var(--bg)',
                      border: '1px solid var(--line-2)',
                      borderRadius: '6px',
                      padding: '16px',
                      maxHeight: '600px',
                      overflow: 'auto'
                    }}>
                      <pre style={{
                        margin: 0,
                        fontFamily: 'Monaco, Menlo, Consolas, monospace',
                        fontSize: '13px',
                        lineHeight: '1.5',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word'
                      }}>
                        {JSON.stringify(displayData, null, 2)}
                      </pre>
                    </div>
                  )}
                </div>
              </div>
            </>
          )}

          {/* 版本对比视图 */}
          {viewMode === 'compare' && (
            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
                <h2 className={styles.stepTitle} style={{ margin: 0 }}>版本对比</h2>
                <button
                  onClick={() => {
                    setViewMode('view');
                    setCompareFromVersion(null);
                    setCompareToVersion(null);
                    // 更新URL参数
                    const newParams = new URLSearchParams(searchParams);
                    newParams.delete('mode');
                    setSearchParams(newParams, { replace: true });
                  }}
                  style={{
                    padding: '8px 16px',
                    background: 'var(--bg)',
                    color: 'var(--ink-2)',
                    border: '1px solid var(--line-2)',
                    borderRadius: '6px',
                    cursor: 'pointer',
                    fontSize: '14px',
                    fontWeight: 500
                  }}
                >
                  返回查看
                </button>
              </div>
              
              {/* 版本选择器 */}
              <div style={{ 
                display: 'flex', 
                gap: '16px', 
                marginBottom: '20px', 
                alignItems: 'center',
                padding: '16px',
                background: 'var(--bg)',
                borderRadius: '8px'
              }}>
                <div style={{ flex: 1 }}>
                  <label style={{ 
                    fontSize: '13px', 
                    fontWeight: 500, 
                    marginBottom: '8px', 
                    display: 'block',
                    color: 'var(--ink)'
                  }}>
                    From (旧版本):
                  </label>
                  <select
                    value={compareFromVersion || ''}
                    onChange={(e) => {
                      const from = parseInt(e.target.value);
                      setCompareFromVersion(from);
                      if (compareToVersion) {
                        handleCompareVersions(from, compareToVersion);
                      }
                    }}
                    style={{
                      padding: '10px 12px',
                      border: '1px solid var(--line-2)',
                      borderRadius: '6px',
                      fontSize: '14px',
                      background: 'white',
                      cursor: 'pointer',
                      width: '100%',
                      height: '40px'
                    }}
                  >
                    <option value="">选择版本</option>
                    {versions.map((v) => (
                      <option key={v.id} value={v.version}>
                        v{v.version} {v.change_summary ? `- ${v.change_summary}` : ''}
                      </option>
                    ))}
                  </select>
                </div>
                
                <div style={{ 
                  fontSize: '24px', 
                  color: 'var(--ink-faint)',
                  marginTop: '24px'
                }}>
                  →
                </div>
                
                <div style={{ flex: 1 }}>
                  <label style={{ 
                    fontSize: '13px', 
                    fontWeight: 500, 
                    marginBottom: '8px', 
                    display: 'block',
                    color: 'var(--ink)'
                  }}>
                    To (新版本):
                  </label>
                  <select
                    value={compareToVersion || ''}
                    onChange={(e) => {
                      const to = parseInt(e.target.value);
                      setCompareToVersion(to);
                      if (compareFromVersion) {
                        handleCompareVersions(compareFromVersion, to);
                      }
                    }}
                    style={{
                      padding: '10px 12px',
                      border: '1px solid var(--line-2)',
                      borderRadius: '6px',
                      fontSize: '14px',
                      background: 'white',
                      cursor: 'pointer',
                      width: '100%',
                      height: '40px'
                    }}
                  >
                    <option value="">选择版本</option>
                    {versions.map((v) => (
                      <option key={v.id} value={v.version}>
                        v{v.version} {v.is_latest ? '(当前)' : ''} {v.change_summary ? `- ${v.change_summary}` : ''}
                      </option>
                    ))}
                  </select>
                </div>
              </div>

              {/* 差异显示 */}
              {diffFields.length > 0 && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '1px', background: 'var(--line)', borderRadius: '8px', overflow: 'hidden' }}>
                  {diffFields.map((field, index) => (
                    <div key={field.field} style={{ background: 'white' }}>
                      <div
                        style={{
                          padding: '12px 16px',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          cursor: field.type === 'unchanged' ? 'pointer' : 'default'
                        }}
                        onClick={() => field.type === 'unchanged' && toggleFieldExpansion(index)}
                      >
                        <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flex: 1 }}>
                          {/* 左侧色块指示器 */}
                          <div style={{ 
                            width: '4px', 
                            height: '20px', 
                            borderRadius: '2px',
                            background: field.type === 'added' ? 'var(--green)' :
                                       field.type === 'removed' ? 'var(--red)' :
                                       field.type === 'modified' ? 'var(--color-yellow-500)' : 'var(--line-2)',
                            flexShrink: 0
                          }} />
                          
                          {field.type === 'unchanged' && (
                            <span style={{ color: 'var(--ink-faint)', width: '16px', flexShrink: 0 }}>
                              {field.expanded ? '▼' : '▶'}
                            </span>
                          )}
                          {field.type === 'modified' && (
                            <span style={{ color: 'var(--color-yellow-600)', width: '16px', flexShrink: 0 }}>~</span>
                          )}
                          {field.type === 'added' && (
                            <span style={{ color: 'var(--green-hover)', width: '16px', flexShrink: 0 }}>+</span>
                          )}
                          {field.type === 'removed' && (
                            <span style={{ color: 'var(--red-hover)', width: '16px', flexShrink: 0 }}>-</span>
                          )}
                          
                          <span style={{ 
                            fontFamily: 'monospace', 
                            fontWeight: 500,
                            color: 'var(--ink)'
                          }}>
                            {field.field}:
                          </span>
                          
                          {field.type === 'unchanged' && !field.expanded && (
                            <span style={{ fontSize: '13px', color: 'var(--ink-3)' }}>
                              ··· 1 unchanged attribute hidden
                            </span>
                          )}
                        </div>
                        {field.type !== 'unchanged' && (
                          <span style={{
                            padding: '2px 8px',
                            borderRadius: '4px',
                            fontSize: '11px',
                            fontWeight: 600,
                            background: field.type === 'added' ? 'var(--green-soft)' :
                                       field.type === 'removed' ? 'var(--red-soft)' : 'var(--color-yellow-100)',
                            color: field.type === 'added' ? 'var(--green-active)' :
                                   field.type === 'removed' ? 'var(--red-active)' : 'var(--color-yellow-700)',
                            flexShrink: 0
                          }}>
                            {field.type}
                          </span>
                        )}
                      </div>
                      {(field.type !== 'unchanged' || field.expanded) && (
                        <div style={{ padding: '0 16px 12px 48px' }}>
                          {field.type === 'removed' && (
                            <div>
                              <div style={{ fontSize: '12px', color: 'var(--ink-2)', marginBottom: '4px', fontWeight: 500 }}>
                                删除的值：
                              </div>
                              <pre style={{ 
                                margin: 0, 
                                padding: '12px', 
                                background: 'var(--red-soft)', 
                                borderRadius: '6px',
                                fontSize: '13px',
                                fontFamily: 'monospace',
                                color: 'var(--red-active)',
                                border: '1px solid var(--color-red-200)',
                                whiteSpace: 'pre-wrap',
                                wordBreak: 'break-word'
                              }}>
                                {formatValue(field.oldValue)}
                              </pre>
                            </div>
                          )}
                          {field.type === 'added' && (
                            <div>
                              <div style={{ fontSize: '12px', color: 'var(--ink-2)', marginBottom: '4px', fontWeight: 500 }}>
                                新增的值：
                              </div>
                              <pre style={{ 
                                margin: 0, 
                                padding: '12px', 
                                background: 'var(--green-soft)', 
                                borderRadius: '6px',
                                fontSize: '13px',
                                fontFamily: 'monospace',
                                color: 'var(--green-active)',
                                border: '1px solid var(--color-green-200)',
                                whiteSpace: 'pre-wrap',
                                wordBreak: 'break-word'
                              }}>
                                {formatValue(field.newValue)}
                              </pre>
                            </div>
                          )}
                          {field.type === 'modified' && (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                              <div>
                                <div style={{ fontSize: '12px', color: 'var(--ink-2)', marginBottom: '4px', fontWeight: 500 }}>
                                  旧版本：
                                </div>
                                <pre style={{ 
                                  margin: 0, 
                                  padding: '12px', 
                                  background: 'var(--red-soft)', 
                                  borderRadius: '6px',
                                  fontSize: '13px',
                                  fontFamily: 'monospace',
                                  color: 'var(--red-active)',
                                  border: '1px solid var(--color-red-200)',
                                  whiteSpace: 'pre-wrap',
                                  wordBreak: 'break-word'
                                }}>
                                  {formatValue(field.oldValue)}
                                </pre>
                              </div>
                              <div>
                                <div style={{ fontSize: '12px', color: 'var(--ink-2)', marginBottom: '4px', fontWeight: 500 }}>
                                  新版本：
                                </div>
                                <pre style={{ 
                                  margin: 0, 
                                  padding: '12px', 
                                  background: 'var(--green-soft)', 
                                  borderRadius: '6px',
                                  fontSize: '13px',
                                  fontFamily: 'monospace',
                                  color: 'var(--green-active)',
                                  border: '1px solid var(--color-green-200)',
                                  whiteSpace: 'pre-wrap',
                                  wordBreak: 'break-word'
                                }}>
                                  {formatValue(field.newValue)}
                                </pre>
                              </div>
                            </div>
                          )}
                          {field.type === 'unchanged' && field.expanded && (
                            <div>
                              <div style={{ fontSize: '12px', color: 'var(--ink-2)', marginBottom: '4px', fontWeight: 500 }}>
                                值：
                              </div>
                              <pre style={{ 
                                margin: 0, 
                                padding: '12px', 
                                background: 'var(--bg)', 
                                borderRadius: '6px',
                                fontSize: '13px',
                                fontFamily: 'monospace',
                                color: 'var(--ink)',
                                border: '1px solid var(--line)',
                                whiteSpace: 'pre-wrap',
                                wordBreak: 'break-word'
                              }}>
                                {formatValue(field.oldValue)}
                              </pre>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* 只在非对比模式下显示footer */}
      {viewMode !== 'compare' && (
        <div className={styles.footer}>
          <div className={styles.footerLeft}>
            <button
              onClick={() => setShowDeleteDialog(true)}
              style={{
                padding: '10px 20px',
                background: 'var(--red)',
                color: 'white',
                border: 'none',
                borderRadius: '6px',
                cursor: 'pointer',
                fontSize: '14px',
                fontWeight: 500
              }}
            >
              删除 Demo
            </button>
          </div>
          
          <div className={styles.footerRight}>
            <button 
              onClick={handleBack}
              style={{
                padding: '10px 20px',
                background: 'var(--bg)',
                color: 'var(--ink-2)',
                border: '1px solid var(--line-2)',
                borderRadius: '6px',
                cursor: 'pointer',
                fontSize: '14px',
                fontWeight: 500,
                marginRight: '12px'
              }}
            >
              返回
            </button>
            
            {/* 根据当前查看的版本显示不同的按钮 */}
            {selectedVersion && selectedVersion !== demo?.current_version?.version ? (
              <button 
                onClick={handleRollbackVersion}
                style={{
                  padding: '10px 20px',
                  background: 'var(--green)',
                  color: 'white',
                  border: 'none',
                  borderRadius: '6px',
                  cursor: 'pointer',
                  fontSize: '14px',
                  fontWeight: 500
                }}
              >
                设置为当前版本
              </button>
            ) : (
              <button 
                onClick={handleEdit}
                style={{
                  padding: '10px 20px',
                  background: 'var(--brand)',
                  color: 'white',
                  border: 'none',
                  borderRadius: '6px',
                  cursor: 'pointer',
                  fontSize: '14px',
                  fontWeight: 500
                }}
              >
                编辑 Demo
              </button>
            )}
          </div>
        </div>
      )}

      <ConfirmDialog
        isOpen={showDeleteDialog}
        title="删除 Demo"
        message={`确定要删除 Demo "${demo.name}" 吗？`}
        confirmText="确认删除"
        cancelText="取消"
        onConfirm={handleDelete}
        onCancel={() => setShowDeleteDialog(false)}
        type="danger"
      />

      <ConfirmDialog
        isOpen={showRollbackDialog}
        title="确认版本回滚"
        message={`确定要将 Demo 回滚到版本 v${selectedVersion} 吗？\n\n这将创建一个新版本，内容为 v${selectedVersion} 的配置。`}
        confirmText="确认回滚"
        cancelText="取消"
        onConfirm={confirmRollback}
        onCancel={() => setShowRollbackDialog(false)}
        type="warning"
      />
    </div>
  );
};

// 错误边界组件
class ErrorBoundary extends Component<
  { children: ReactNode; onError: () => void },
  { hasError: boolean }
> {
  constructor(props: { children: ReactNode; onError: () => void }) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  componentDidCatch(error: Error, errorInfo: any) {
    console.error('Form render error:', error, errorInfo);
    this.props.onError();
  }

  render() {
    if (this.state.hasError) {
      return null;
    }
    return this.props.children;
  }
}

export default DemoDetail;
