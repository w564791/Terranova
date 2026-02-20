import React, { useState, useEffect, Component, type ReactNode } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { processApiSchema } from '../utils/schemaTypeMapper';
import api from '../services/api';
import { FormPreview } from '../components/DynamicForm';
import type { FormSchema } from '../components/DynamicForm';
import { FormRenderer as OpenAPIFormRenderer } from '../components/OpenAPIFormRenderer';
import ConfirmDialog from '../components/ConfirmDialog';
import SplitButton from '../components/SplitButton';
import ResourceRunDialog from '../components/ResourceRunDialog';
import TopBar from '../components/TopBar';
import WorkspaceSidebar from '../components/WorkspaceSidebar';
import styles from './AddResources.module.css';

interface Resource {
  id: number;
  workspace_id: number;
  resource_type: string;
  resource_name: string;
  resource_id: string;
  current_version?: {
    id: number;
    version: number;
    tf_code: any;
    variables?: any;
    change_summary: string;
    created_at: string;
  };
  description?: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// 从 tf_code 中提取 module 版本信息
const extractModuleVersion = (tfCode: any): string | null => {
  if (!tfCode?.module) return null;
  
  const moduleKeys = Object.keys(tfCode.module);
  if (moduleKeys.length === 0) return null;
  
  const moduleKey = moduleKeys[0];
  const moduleArray = tfCode.module[moduleKey];
  
  if (Array.isArray(moduleArray) && moduleArray.length > 0) {
    return moduleArray[0].version || null;
  }
  
  return null;
};

type ViewMode = 'view' | 'compare';
type DataViewMode = 'form' | 'json';

interface Version {
  id: number;
  version: number;
  change_summary: string;
  created_at: string;
  is_latest: boolean;
  tf_code?: any;
}

interface DiffField {
  field: string;
  type: 'added' | 'removed' | 'modified' | 'unchanged';
  oldValue?: any;
  newValue?: any;
  expanded?: boolean;
}

const ViewResource: React.FC = () => {
  const { id, resourceId } = useParams<{ id: string; resourceId: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  
  const [resource, setResource] = useState<Resource | null>(null);
  const [schema, setSchema] = useState<any>(null); // 支持 v1 和 v2 schema
  const [rawSchema, setRawSchema] = useState<any>(null); // 原始 schema 数据（用于 ModuleFormRenderer）
  const [formData, setFormData] = useState<any>({});
  const [loading, setLoading] = useState(true);
  const [viewMode, setViewMode] = useState<ViewMode>('view');
  const [dataViewMode, setDataViewMode] = useState<DataViewMode>('form');
  const [versions, setVersions] = useState<Version[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
  const [displayData, setDisplayData] = useState<any>({});
  const [diffFields, setDiffFields] = useState<DiffField[]>([]);
  const [urlInitialized, setUrlInitialized] = useState(false);
  const [showRollbackDialog, setShowRollbackDialog] = useState(false);
  const [compareFromVersion, setCompareFromVersion] = useState<number | null>(null);
  const [compareToVersion, setCompareToVersion] = useState<number | null>(null);
  const [showRestoreDialog, setShowRestoreDialog] = useState(false);
  const [restoring, setRestoring] = useState(false);
  const [formRenderError, setFormRenderError] = useState(false);
  const [showRunDialog, setShowRunDialog] = useState(false);
  const [matchedModule, setMatchedModule] = useState<{ id: number; name: string } | null>(null);

  useEffect(() => {
    loadResource();
  }, [id, resourceId]);

  useEffect(() => {
    loadVersions();
  }, [id, resourceId]);

  useEffect(() => {
    // 从URL参数初始化状态
    if (resource && versions.length > 0 && !urlInitialized) {
      const urlVersion = searchParams.get('version');
      const urlMode = searchParams.get('mode') as ViewMode;
      const urlDataView = searchParams.get('view') as DataViewMode;
      
      if (urlVersion) {
        const versionNum = parseInt(urlVersion);
        if (versions.some(v => v.version === versionNum)) {
          setSelectedVersion(versionNum);
        } else {
          setSelectedVersion(resource.current_version?.version || null);
        }
      } else {
        setSelectedVersion(resource.current_version?.version || null);
      }
      
      if (urlMode === 'compare') {
        setViewMode('compare');
        // 如果是对比模式，触发对比
        const versionNum = parseInt(urlVersion || '0');
        if (versionNum && resource.current_version?.version) {
          handleCompareVersions(versionNum, resource.current_version.version);
        }
      } else {
        setViewMode('view');
      }
      
      if (urlDataView === 'json' || urlDataView === 'form') {
        setDataViewMode(urlDataView);
      }
      
      setUrlInitialized(true);
    }
  }, [resource, versions, urlInitialized]);

  useEffect(() => {
    // 当选择版本时，加载该版本的数据
    if (selectedVersion !== null && resource?.current_version?.version) {
      console.log('🔄 Version changed to:', selectedVersion);
      if (selectedVersion !== resource.current_version.version) {
        console.log('📥 Loading historical version data...');
        loadVersionData(selectedVersion);
      } else {
        console.log('📋 Using current version data');
        setDisplayData(formData);
      }
    }
  }, [selectedVersion, resource]);

  useEffect(() => {
    // 当formData更新时，如果是当前版本，更新displayData
    if (selectedVersion === resource?.current_version?.version && Object.keys(formData).length > 0) {
      console.log(' Updating displayData with formData:', formData);
      setDisplayData(formData);
    }
  }, [formData, selectedVersion, resource]);

  const loadResource = async () => {
    try {
      setLoading(true);
      
      // 1. 获取资源信息
      const resourceResponse: any = await api.get(`/workspaces/${id}/resources/${resourceId}`);
      const resourceData = resourceResponse.data?.resource || resourceResponse.resource || resourceResponse;
      setResource(resourceData);
      
      // 2. 从tf_code中提取module配置
      const tfCode = resourceData.current_version?.tf_code || {};
      
      let moduleConfig = null;
      let moduleSource = '';
      
      if (tfCode.module) {
        const moduleKeys = Object.keys(tfCode.module);
        if (moduleKeys.length > 0) {
          const moduleKey = moduleKeys[0];
          const moduleArray = tfCode.module[moduleKey];
          if (Array.isArray(moduleArray) && moduleArray.length > 0) {
            moduleConfig = moduleArray[0];
            moduleSource = moduleConfig.source;
          }
        }
      }
      
      if (!moduleSource) {
        showToast('无法获取Module信息', 'error');
        return;
      }
      
      // 3. 查找对应的module
      const modulesResponse = await api.get('/modules');
      const modules = modulesResponse.data.items || [];
      
      console.log('🔍 Looking for module with source:', moduleSource);
      console.log('📋 Available modules:', modules.map((m: any) => ({
        id: m.id,
        name: m.name,
        source: m.source,
        module_source: m.module_source
      })));
      
      const foundModule = modules.find((m: any) => 
        m.module_source === moduleSource || m.source === moduleSource
      );
      
      console.log(' Matched module:', foundModule);
      
      // 保存匹配的 module 信息
      if (foundModule) {
        setMatchedModule({ id: foundModule.id, name: foundModule.name });
      }
      
      if (!foundModule) {
        showToast('找不到对应的Module', 'error');
        console.error('❌ No module found for source:', moduleSource);
        return;
      }
      
      // 4. 加载module的schema
      const schemaResponse = await api.get(`/modules/${foundModule.id}/schemas`);
      
      let schemasData = [];
      if (schemaResponse.data.data) {
        schemasData = Array.isArray(schemaResponse.data.data) 
          ? schemaResponse.data.data 
          : [schemaResponse.data.data];
      } else if (Array.isArray(schemaResponse.data)) {
        schemasData = schemaResponse.data;
      }
      
      if (schemasData.length > 0) {
        let activeSchema = schemasData.find((s: any) => s.status === 'active') || schemasData[0];
        
        // 【优先使用 openapi_schema】如果存在 openapi_schema，优先使用它（v2 格式）
        let schemaToUse = activeSchema.openapi_schema || activeSchema.schema_data;
        
        // 解析字符串格式的 schema
        if (typeof schemaToUse === 'string') {
          try {
            schemaToUse = JSON.parse(schemaToUse);
          } catch (e) {
            console.error('Schema解析错误:', e);
            schemaToUse = {};
          }
        }
        
        console.log('📋 Schema source:', activeSchema.openapi_schema ? 'openapi_schema' : 'schema_data');
        console.log('📋 Schema version:', activeSchema.schema_version);
        console.log('📋 Has OpenAPI Schema:', !!activeSchema.openapi_schema);
        
        // 检查是否是 V2 Schema (OpenAPI 格式) - 与 AddResources.tsx 保持一致
        const isV2 = activeSchema.schema_version === 'v2' && !!activeSchema.openapi_schema;
        
        // 保存原始 schema 数据
        setRawSchema(activeSchema);
        
        if (isV2) {
          console.log('📊 Using V2 OpenAPI Schema');
          setSchema(activeSchema);
        } else {
          // V1 Schema 处理
          // 解析schema_data（如果是字符串）
          if (typeof activeSchema.schema_data === 'string') {
            try {
              activeSchema.schema_data = JSON.parse(activeSchema.schema_data);
            } catch (e) {
              console.error('Schema解析错误:', e);
              activeSchema.schema_data = {};
            }
          }
          
          // 使用processApiSchema处理类型转换
          const processedSchema = processApiSchema(activeSchema);
          console.log('📊 Processed V1 Schema:', processedSchema);
          
          setSchema(processedSchema);
        }
        
        // 5. 提取表单数据
        if (moduleConfig) {
          const { source, version, ...configData } = moduleConfig; // 排除 source 和 version
          console.log('📝 Extracted form data:', configData);
          setFormData(configData);
          // 直接设置 displayData，不等待 selectedVersion 的设置
          setDisplayData(configData);
        }
      } else {
        showToast('该Module暂无Schema定义', 'warning');
      }
    } catch (error: any) {
      console.error('加载资源失败:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleEdit = () => {
    navigate(`/workspaces/${id}/resources/${resourceId}/edit`);
  };

  const handleCloneAndEdit = () => {
    navigate(`/workspaces/${id}/resources/${resourceId}/edit?mode=clone`);
  };

  const handleBack = () => {
    navigate(`/workspaces/${id}?tab=resources`);
  };

  const loadVersions = async () => {
    try {
      const versionsResponse: any = await api.get(
        `/workspaces/${id}/resources/${resourceId}/versions`
      );
      const versionsData = versionsResponse.data?.versions || versionsResponse.versions || [];
      setVersions(versionsData);
    } catch (error: any) {
      console.error('加载版本列表失败:', error);
      showToast(extractErrorMessage(error), 'error');
    }
  };

  const extractModuleConfig = (tfCode: any): any => {
    // 尝试 module 或 modules（兼容不同的数据结构）
    const moduleData = tfCode?.module || tfCode?.modules;
    
    if (!moduleData) {
      console.warn(' No module/modules found in tf_code');
      return {};
    }
    
    const moduleKeys = Object.keys(moduleData);
    if (moduleKeys.length === 0) {
      console.warn(' Module data is empty');
      return {};
    }
    
    const moduleKey = moduleKeys[0];
    const moduleArray = moduleData[moduleKey];
    
    console.log('📝 Module key:', moduleKey);
    console.log('📝 Module array:', moduleArray);
    
    if (Array.isArray(moduleArray) && moduleArray.length > 0) {
      const { source, ...config } = moduleArray[0];
      console.log(' Successfully extracted config:', config);
      return config;
    }
    
    console.warn(' Module array is invalid');
    return {};
  };

  const loadVersionData = async (version: number) => {
    try {
      console.log(`🌐 Fetching version ${version} data...`);
      const versionResponse: any = await api.get(
        `/workspaces/${id}/resources/${resourceId}/versions/${version}`
      );
      
      console.log('📦 Full API response:', versionResponse);
      console.log('📦 versionResponse.data:', versionResponse.data);
      
      // 尝试多种可能的数据路径
      const versionDataResponse = versionResponse.data?.version || 
                                   versionResponse.data || 
                                   versionResponse.version ||
                                   versionResponse;
      
      console.log('📦 Version data:', versionDataResponse);
      console.log('📦 tf_code type:', typeof versionDataResponse.tf_code);
      console.log('📦 tf_code content:', versionDataResponse.tf_code);
      
      // 如果tf_code是字符串，需要先解析
      let tfCode = versionDataResponse.tf_code;
      if (typeof tfCode === 'string') {
        try {
          tfCode = JSON.parse(tfCode);
          console.log('📦 Parsed tf_code:', tfCode);
        } catch (e) {
          console.error('❌ Failed to parse tf_code:', e);
        }
      }
      
      if (!tfCode) {
        console.error('❌ tf_code is undefined or null!');
        console.error('❌ versionDataResponse keys:', Object.keys(versionDataResponse));
        showToast('无法获取版本数据', 'error');
        return;
      }
      
      const config = extractModuleConfig(tfCode);
      console.log('⚙️ Extracted config:', config);
      
      if (Object.keys(config).length > 0) {
        setDisplayData(config);
        console.log(' DisplayData updated with config');
      } else {
        console.error('❌ Extracted config is empty!');
      }
    } catch (error: any) {
      console.error('❌ 加载版本失败:', error);
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
    if (dataViewMode !== 'form') {
      newParams.set('view', dataViewMode);
    } else {
      newParams.delete('view');
    }
    setSearchParams(newParams, { replace: true });
    
    if (version !== resource?.current_version?.version) {
      showToast(`已切换到版本 v${version}`, 'info');
    }
  };

  const handleStartCompare = async () => {
    if (!selectedVersion || !resource?.current_version?.version) return;
    
    // 切换到对比模式
    setViewMode('compare');
    
    // 设置初始对比版本
    setCompareFromVersion(selectedVersion);
    setCompareToVersion(resource.current_version.version);
    
    // 更新URL参数
    const newParams = new URLSearchParams(searchParams);
    newParams.set('version', selectedVersion.toString());
    newParams.set('mode', 'compare');
    if (dataViewMode !== 'form') {
      newParams.set('view', dataViewMode);
    }
    setSearchParams(newParams, { replace: true });
    
    // 对比选中版本和当前版本
    await handleCompareVersions(selectedVersion, resource.current_version.version);
  };

  const handleRollbackVersion = () => {
    if (!selectedVersion || !resource?.current_version?.version) return;
    
    if (selectedVersion === resource.current_version.version) {
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
      const response: any = await api.post(
        `/workspaces/${id}/resources/${resourceId}/versions/${selectedVersion}/rollback`
      );
      
      const newVersion = response.data?.version || response.version || response;
      
      showToast(`成功回滚到版本 v${selectedVersion}，新版本号为 v${newVersion.version}`, 'success');
      
      // 重新加载资源数据
      await loadResource();
      await loadVersions();
      
      // 切换到新的当前版本
      setSelectedVersion(newVersion.version);
      
      // 清理URL参数
      const newParams = new URLSearchParams();
      setSearchParams(newParams, { replace: true });
    } catch (error: any) {
      console.error('版本回滚失败:', error);
      showToast(extractErrorMessage(error), 'error');
    }
  };
  
  const handleRestoreResource = () => {
    setShowRestoreDialog(true);
  };
  
  const confirmRestore = async () => {
    try {
      setRestoring(true);
      await api.post(`/workspaces/${id}/resources/${resourceId}/restore`);
      showToast('资源恢复成功', 'success');
      setShowRestoreDialog(false);
      
      // 重新加载资源数据
      await loadResource();
    } catch (error: any) {
      console.error('资源恢复失败:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setRestoring(false);
    }
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
        // 新增字段 - 保持完整JSON
        fields.push({ field: key, type: 'added', newValue, expanded: false });
      } else if (oldExists && !newExists) {
        // 删除字段 - 保持完整JSON
        fields.push({ field: key, type: 'removed', oldValue, expanded: false });
      } else if (JSON.stringify(oldValue) !== JSON.stringify(newValue)) {
        // 修改字段 - 保持完整JSON
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
      console.log(`🔀 Comparing versions: v${fromVer} → v${toVer}`);
      
      const [fromResponse, toResponse]: any[] = await Promise.all([
        api.get(`/workspaces/${id}/resources/${resourceId}/versions/${fromVer}`),
        api.get(`/workspaces/${id}/resources/${resourceId}/versions/${toVer}`)
      ]);
      
      console.log('📦 From response:', fromResponse);
      console.log('📦 To response:', toResponse);
      
      // 使用与loadVersionData相同的数据提取逻辑
      const fromData = fromResponse.data?.version || 
                       fromResponse.data || 
                       fromResponse.version ||
                       fromResponse;
      const toData = toResponse.data?.version || 
                     toResponse.data || 
                     toResponse.version ||
                     toResponse;
      
      console.log('📦 From data:', fromData);
      console.log('📦 To data:', toData);
      
      // 处理tf_code可能是字符串的情况
      let fromTfCode = fromData.tf_code;
      let toTfCode = toData.tf_code;
      
      if (typeof fromTfCode === 'string') {
        fromTfCode = JSON.parse(fromTfCode);
      }
      if (typeof toTfCode === 'string') {
        toTfCode = JSON.parse(toTfCode);
      }
      
      console.log('📦 From tf_code:', fromTfCode);
      console.log('📦 To tf_code:', toTfCode);
      
      if (!fromTfCode || !toTfCode) {
        console.error('❌ tf_code is missing!');
        showToast('无法获取版本数据', 'error');
        return;
      }
      
      const fromConfig = extractModuleConfig(fromTfCode);
      const toConfig = extractModuleConfig(toTfCode);
      
      console.log('⚙️ From config:', fromConfig);
      console.log('⚙️ To config:', toConfig);
      
      if (Object.keys(fromConfig).length === 0 && Object.keys(toConfig).length === 0) {
        console.error('❌ Both configs are empty!');
        showToast('无法提取配置数据', 'error');
        return;
      }
      
      const diff = calculateDiff(fromConfig, toConfig);
      console.log('📊 Diff fields:', diff);
      
      setDiffFields(diff);
      console.log(' Diff fields set, count:', diff.length);
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

  if (!resource || !schema) {
    return (
      <div className={styles.container}>
        <div className={styles.header}>
          <button onClick={handleBack} className={styles.backButton}>
            ← 返回Workspace
          </button>
          <h1 className={styles.title}>资源不存在或Schema未定义</h1>
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      {/* 左侧 Workspace 导航栏 - 使用共享组件 */}
      <WorkspaceSidebar
        workspaceId={id!}
        workspaceName={resource?.resource_name || 'Loading...'}
        activeTab="resources"
      />

      {/* 右侧主内容区 */}
      <div style={{ marginLeft: '256px', flex: 1, maxWidth: 'calc(100% - 256px)' }}>
        <TopBar title="工作空间" />
        <div className={styles.container} style={{ padding: '24px' }}>
          <div className={styles.header}>
            <div className={styles.headerLeft}>
              <button onClick={handleBack} className={styles.backButton}>
                ← 返回Workspace
              </button>
              <h1 className={styles.title}>查看资源</h1>
            </div>
        
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <span className={styles.resourceType}>{resource.resource_type}</span>
          <span className={styles.resourceName}>{resource.resource_name}</span>
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.configureStep}>
          {/* 查看模式 */}
          {viewMode === 'view' && (
            <>
              <div className={styles.resourceInfoCard}>
                <div className={styles.infoRow}>
                  <span className={styles.infoLabel}>资源ID:</span>
                  <span className={styles.infoValue}>{resource.resource_id}</span>
                </div>
                {/* Module 信息 */}
                {matchedModule && (
                  <div className={styles.infoRow}>
                    <span className={styles.infoLabel}>Module:</span>
                    <span className={styles.infoValue} style={{ 
                      display: 'inline-flex', 
                      alignItems: 'center', 
                      gap: '8px' 
                    }}>
                      <span
                        onClick={() => navigate(`/modules/${matchedModule.id}`)}
                        style={{
                          padding: '2px 10px',
                          background: 'linear-gradient(135deg, #3b82f6 0%, #1d4ed8 100%)',
                          color: 'white',
                          borderRadius: '4px',
                          fontSize: '12px',
                          fontWeight: 600,
                          cursor: 'pointer',
                          transition: 'all 0.2s',
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: '4px'
                        }}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.transform = 'translateY(-1px)';
                          e.currentTarget.style.boxShadow = '0 2px 8px rgba(59, 130, 246, 0.4)';
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.transform = 'translateY(0)';
                          e.currentTarget.style.boxShadow = 'none';
                        }}
                        title="点击查看 Module 详情"
                      >
                        {matchedModule.name}
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                          <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
                          <polyline points="15 3 21 3 21 9"></polyline>
                          <line x1="10" y1="14" x2="21" y2="3"></line>
                        </svg>
                      </span>
                    </span>
                  </div>
                )}
                {/* TF Module 版本信息 */}
                {extractModuleVersion(resource.current_version?.tf_code) && (
                  <div className={styles.infoRow}>
                    <span className={styles.infoLabel}>Module 版本:</span>
                    <span className={styles.infoValue} style={{ 
                      display: 'inline-flex', 
                      alignItems: 'center', 
                      gap: '8px' 
                    }}>
                      <span style={{
                        padding: '2px 8px',
                        background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
                        color: 'white',
                        borderRadius: '4px',
                        fontSize: '12px',
                        fontWeight: 600
                      }}>
                        {extractModuleVersion(resource.current_version?.tf_code)}
                      </span>
                    </span>
                  </div>
                )}
                <div className={styles.infoRow}>
                  <span className={styles.infoLabel}>创建时间:</span>
                  <span className={styles.infoValue}>{formatDate(resource.created_at)}</span>
                </div>
                <div className={styles.infoRow}>
                  <span className={styles.infoLabel}>更新时间:</span>
                  <span className={styles.infoValue}>{formatDate(resource.updated_at)}</span>
                </div>
                {resource.current_version?.change_summary && (
                  <div className={styles.infoRow}>
                    <span className={styles.infoLabel}>上次修改:</span>
                    <span className={styles.infoValue}>{resource.current_version.change_summary}</span>
                  </div>
                )}
              </div>

              {schema && (
                <div className={styles.resourceInfoCard}>
                  <h2 className={styles.stepTitle}>资源配置</h2>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
                    <div className={styles.viewToggle}>
                      <button
                        className={`${styles.viewButton} ${dataViewMode === 'form' ? styles.viewButtonActive : ''}`}
                        onClick={() => {
                          setDataViewMode('form');
                          setFormRenderError(false);
                          // 更新URL参数
                          const newParams = new URLSearchParams(searchParams);
                          newParams.delete('view');
                          setSearchParams(newParams, { replace: true });
                        }}
                        title={formRenderError ? '点击重新尝试表单视图' : '切换到表单视图'}
                      >
                        表单视图
                      </button>
                      <button
                        className={`${styles.viewButton} ${dataViewMode === 'json' ? styles.viewButtonActive : ''}`}
                        onClick={() => {
                          setDataViewMode('json');
                          // 更新URL参数
                          const newParams = new URLSearchParams(searchParams);
                          newParams.set('view', 'json');
                          setSearchParams(newParams, { replace: true });
                        }}
                      >
                        JSON视图
                      </button>
                    </div>
                    
                    {/* 右侧按钮组 - 固定宽度避免移动 */}
                    <div style={{ display: 'flex', alignItems: 'center', gap: '12px', minWidth: '280px', justifyContent: 'flex-end' }}>
                      {/* 版本选择下拉菜单 - 固定宽度，高度与按钮一致 */}
                      <select
                        value={selectedVersion || resource.current_version?.version || ''}
                        onChange={(e) => handleVersionChange(parseInt(e.target.value))}
                        style={{
                          padding: '10px 12px',
                          border: '1px solid var(--color-gray-300)',
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
                      
                      {/* 对比版本按钮 - 固定宽度占位，高度与编辑资源按钮一致 */}
                      <div style={{ width: '100px' }}>
                        {selectedVersion && selectedVersion !== resource.current_version?.version && (
                          <button
                            onClick={handleStartCompare}
                            className={styles.btnPrimary}
                            style={{ 
                              padding: '10px 16px',
                              fontSize: '14px',
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
                      background: '#fff3cd',
                      border: '1px solid #ffc107',
                      borderRadius: '6px',
                      color: '#856404',
                      marginBottom: '16px'
                    }}>
                       表单渲染失败，已自动切换到JSON视图
                    </div>
                  )}

                  <div className={styles.previewContent}>
                    {dataViewMode === 'form' && !formRenderError ? (
                      <ErrorBoundary
                        onError={() => {
                          setFormRenderError(true);
                          setDataViewMode('json');
                          showToast('表单渲染失败，已切换到JSON视图', 'warning');
                        }}
                      >
                        {/* 根据 schema 版本选择渲染器 - 与 AddResources.tsx 保持一致 */}
                        {rawSchema?.schema_version === 'v2' && rawSchema?.openapi_schema ? (
                          // 使用 key 强制在 displayData 变化时重新渲染组件
                          <OpenAPIFormRenderer
                            key={JSON.stringify(displayData)}
                            schema={rawSchema.openapi_schema}
                            initialValues={displayData}
                            onChange={() => {}}
                            readOnly={true}
                          />
                        ) : (
                          <FormPreview
                            schema={(schema as any).schema_data || schema}
                            values={displayData}
                            onClose={() => {}}
                            inline={true}
                            viewMode={dataViewMode}
                            onViewModeChange={setDataViewMode}
                          />
                        )}
                      </ErrorBoundary>
                    ) : (
                      <div style={{
                        background: '#f8f9fa',
                        border: '1px solid #dee2e6',
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
              )}
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
                  className={styles.btnSecondary}
                  style={{ padding: '8px 16px' }}
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
                background: 'var(--color-gray-50)',
                borderRadius: '8px'
              }}>
                <div style={{ flex: 1 }}>
                  <label style={{ 
                    fontSize: '13px', 
                    fontWeight: 500, 
                    marginBottom: '8px', 
                    display: 'block',
                    color: 'var(--color-gray-700)'
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
                      border: '1px solid var(--color-gray-300)',
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
                  color: 'var(--color-gray-400)',
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
                    color: 'var(--color-gray-700)'
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
                      border: '1px solid var(--color-gray-300)',
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

              {diffFields.length > 0 && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '1px', background: 'var(--color-gray-200)', borderRadius: '8px', overflow: 'hidden' }}>
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
                          {/* 左侧色块指示器 - 固定宽度 */}
                          <div style={{ 
                            width: '4px', 
                            height: '20px', 
                            borderRadius: '2px',
                            background: field.type === 'added' ? 'var(--color-green-500)' :
                                       field.type === 'removed' ? 'var(--color-red-500)' :
                                       field.type === 'modified' ? 'var(--color-yellow-500)' : 'var(--color-gray-300)',
                            flexShrink: 0
                          }} />
                          
                          {field.type === 'unchanged' && (
                            <span style={{ color: 'var(--color-gray-400)', width: '16px', flexShrink: 0 }}>
                              {field.expanded ? '▼' : '▶'}
                            </span>
                          )}
                          {field.type === 'modified' && (
                            <span style={{ color: 'var(--color-yellow-600)', width: '16px', flexShrink: 0 }}>~</span>
                          )}
                          {field.type === 'added' && (
                            <span style={{ color: 'var(--color-green-600)', width: '16px', flexShrink: 0 }}>+</span>
                          )}
                          {field.type === 'removed' && (
                            <span style={{ color: 'var(--color-red-600)', width: '16px', flexShrink: 0 }}>-</span>
                          )}
                          
                          <span style={{ 
                            fontFamily: 'monospace', 
                            fontWeight: 500,
                            color: field.field.includes('.') ? 'var(--color-gray-600)' : 'var(--color-gray-900)'
                          }}>
                            {field.field}:
                          </span>
                          
                          {field.type === 'unchanged' && !field.expanded && (
                            <span style={{ fontSize: '13px', color: 'var(--color-gray-500)' }}>
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
                            background: field.type === 'added' ? 'var(--color-green-100)' :
                                       field.type === 'removed' ? 'var(--color-red-100)' : 'var(--color-yellow-100)',
                            color: field.type === 'added' ? 'var(--color-green-700)' :
                                   field.type === 'removed' ? 'var(--color-red-700)' : 'var(--color-yellow-700)',
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
                              <div style={{ fontSize: '12px', color: 'var(--color-gray-600)', marginBottom: '4px', fontWeight: 500 }}>
                                删除的值：
                              </div>
                              <pre style={{ 
                                margin: 0, 
                                padding: '12px', 
                                background: 'var(--color-red-50)', 
                                borderRadius: '6px',
                                fontSize: '13px',
                                fontFamily: 'monospace',
                                color: 'var(--color-red-700)',
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
                              <div style={{ fontSize: '12px', color: 'var(--color-gray-600)', marginBottom: '4px', fontWeight: 500 }}>
                                新增的值：
                              </div>
                              <pre style={{ 
                                margin: 0, 
                                padding: '12px', 
                                background: 'var(--color-green-50)', 
                                borderRadius: '6px',
                                fontSize: '13px',
                                fontFamily: 'monospace',
                                color: 'var(--color-green-700)',
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
                                <div style={{ fontSize: '12px', color: 'var(--color-gray-600)', marginBottom: '4px', fontWeight: 500 }}>
                                  旧版本：
                                </div>
                                <pre style={{ 
                                  margin: 0, 
                                  padding: '12px', 
                                  background: 'var(--color-red-50)', 
                                  borderRadius: '6px',
                                  fontSize: '13px',
                                  fontFamily: 'monospace',
                                  color: 'var(--color-red-700)',
                                  border: '1px solid var(--color-red-200)',
                                  whiteSpace: 'pre-wrap',
                                  wordBreak: 'break-word'
                                }}>
                                  {formatValue(field.oldValue)}
                                </pre>
                              </div>
                              <div>
                                <div style={{ fontSize: '12px', color: 'var(--color-gray-600)', marginBottom: '4px', fontWeight: 500 }}>
                                  新版本：
                                </div>
                                <pre style={{ 
                                  margin: 0, 
                                  padding: '12px', 
                                  background: 'var(--color-green-50)', 
                                  borderRadius: '6px',
                                  fontSize: '13px',
                                  fontFamily: 'monospace',
                                  color: 'var(--color-green-700)',
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
                              <div style={{ fontSize: '12px', color: 'var(--color-gray-600)', marginBottom: '4px', fontWeight: 500 }}>
                                值：
                              </div>
                              <pre style={{ 
                                margin: 0, 
                                padding: '12px', 
                                background: 'var(--color-gray-50)', 
                                borderRadius: '6px',
                                fontSize: '13px',
                                fontFamily: 'monospace',
                                color: 'var(--color-gray-700)',
                                border: '1px solid var(--color-gray-200)',
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

      <div className={styles.footer}>
        <div className={styles.footerLeft}>
          {/* 可以添加其他操作按钮 */}
        </div>
        
        <div className={styles.footerRight}>
          <button onClick={handleBack} className={styles.btnCancel}>
            返回
          </button>
          
          {viewMode === 'view' && (
            <>
              {!resource.is_active ? (
                <button
                  onClick={handleRestoreResource}
                  className={styles.btnPrimary}
                >
                  恢复资源
                </button>
              ) : (
                <>
                  {selectedVersion && selectedVersion !== resource.current_version?.version ? (
                    <button
                      onClick={handleRollbackVersion}
                      className={styles.btnPrimary}
                    >
                      设置为当前版本
                    </button>
                  ) : (
                    <>
                      <button
                        onClick={() => setShowRunDialog(true)}
                        className={styles.btnSecondary}
                      >
                        运行该任务
                      </button>
                      <SplitButton
                        mainLabel="编辑资源"
                        mainOnClick={handleEdit}
                        menuItems={[
                          {
                            label: '克隆并编辑资源',
                            onClick: handleCloneAndEdit
                          }
                        ]}
                      />
                    </>
                  )}
                </>
              )}
            </>
          )}
        </div>
      </div>

      {/* 版本回滚确认对话框 */}
      <ConfirmDialog
        isOpen={showRollbackDialog}
        title="确认版本回滚"
        message={`确定要将资源回滚到版本 v${selectedVersion} 吗？\n\n这将创建一个新版本，内容为 v${selectedVersion} 的配置。`}
        confirmText="确认回滚"
        cancelText="取消"
        onConfirm={confirmRollback}
        onCancel={() => setShowRollbackDialog(false)}
        type="warning"
      />
      
      {/* 资源恢复确认对话框 */}
      <ConfirmDialog
        isOpen={showRestoreDialog}
        title="恢复资源"
        message={`确定要恢复资源 ${resource.resource_name} 吗？\n\n恢复后资源将重新变为可用状态。`}
        confirmText={restoring ? '恢复中...' : '确认恢复'}
        cancelText="取消"
        onConfirm={confirmRestore}
        onCancel={() => setShowRestoreDialog(false)}
      />
      
      {/* 资源运行对话框 */}
      <ResourceRunDialog
        isOpen={showRunDialog}
        workspaceId={id!}
        resourceName={resource.resource_name}
        resourceType={resource.resource_type}
        onClose={() => setShowRunDialog(false)}
        onSuccess={() => {
          setShowRunDialog(false);
        }}
      />
        </div>
      </div>
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

export default ViewResource;
