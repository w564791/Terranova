import React, { useState, useEffect, useRef, useCallback, useMemo, Component, type ReactNode } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { processApiSchema } from '../utils/schemaTypeMapper';
import api from '../services/api';
import DynamicForm, { type FormSchema, FormPreview } from '../components/DynamicForm';
import { FormRenderer as OpenAPIFormRenderer } from '../components/OpenAPIFormRenderer';
import { 
  AITriggerButton, 
  AIInputPanel, 
  AIPreviewModal, 
  useAIConfigGenerator 
} from '../components/OpenAPIFormRenderer/AIFormAssistant';
import { JsonEditor } from '../components/DynamicForm/JsonEditor';
import { 
  ResourceEditingService, 
  generateUUID,
  type EditorInfo,
  type DriftInfo 
} from '../services/resourceEditing';
import { websocketService } from '../services/websocket';
import EditingStatusBar from '../components/EditingStatusBar';
import DriftRecoveryDialog from '../components/DriftRecoveryDialog';
import TakeoverConfirmDialog from '../components/TakeoverConfirmDialog';
import TakeoverRequestDialog from '../components/TakeoverRequestDialog';
import TakeoverWaitingDialog from '../components/TakeoverWaitingDialog';
import SplitButton from '../components/SplitButton';
import ResourceRunDialog from '../components/ResourceRunDialog';
import TopBar from '../components/TopBar';
import WorkspaceSidebar from '../components/WorkspaceSidebar';
import { listVersions, getDefaultVersion, type ModuleVersion } from '../services/moduleVersions';
import { schemaV2Service } from '../services/schemaV2';
import type { WorkspaceResourceContext, WorkspaceResourceNode, RemoteDataNode } from '../components/OpenAPIFormRenderer/types';
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
  };
  description?: string;
  is_active: boolean;
}

type ViewMode = 'form' | 'json';

const EditResource: React.FC = () => {
  const { id, resourceId } = useParams<{ id: string; resourceId: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [searchParams] = useSearchParams();
  
  const [resource, setResource] = useState<Resource | null>(null);
  const [schema, setSchema] = useState<FormSchema | null>(null);
  const [rawSchema, setRawSchema] = useState<any>(null); // 原始 schema 数据（用于 ModuleFormRenderer）
  const [formData, setFormData] = useState<any>({});
  const [changeSummary, setChangeSummary] = useState('');
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [changeSummaryError, setChangeSummaryError] = useState('');
  const [viewMode, setViewMode] = useState<ViewMode>('form');
  const [formRenderError, setFormRenderError] = useState(false);
  const [initialFieldsToShow, setInitialFieldsToShow] = useState<string[]>([]);
  const [isCloneMode, setIsCloneMode] = useState(false);
  const [moduleSource, setModuleSource] = useState('');
  const changeSummaryRef = React.useRef<HTMLInputElement>(null);
  
  // Module 版本相关状态
  const [matchedModuleId, setMatchedModuleId] = useState<number | null>(null);
  const [moduleVersions, setModuleVersions] = useState<ModuleVersion[]>([]);
  const [selectedVersionId, setSelectedVersionId] = useState<string>('');
  const [loadingVersionSchema, setLoadingVersionSchema] = useState(false);
  
  // Workspace 资源引用上下文
  const [workspaceResourceContext, setWorkspaceResourceContext] = useState<WorkspaceResourceContext | null>(null);

  // 从 Schema 中提取默认值
  const extractSchemaDefaults = useCallback((schema: any): Record<string, unknown> => {
    if (!schema?.openapi_schema) return {};
    
    const properties = schema.openapi_schema?.components?.schemas?.ModuleInput?.properties || {};
    const defaults: Record<string, unknown> = {};
    
    Object.entries(properties).forEach(([name, prop]: [string, any]) => {
      if (Object.prototype.hasOwnProperty.call(prop, 'default')) {
        defaults[name] = prop.default;
      }
    });
    
    return defaults;
  }, []);

  // 深度合并函数：用于合并 Schema 默认值和用户数据
  const deepMergeForDisplay = useCallback((defaults: Record<string, unknown>, userData: Record<string, unknown>): Record<string, unknown> => {
    const result = { ...defaults };
    
    Object.keys(userData).forEach(key => {
      const userValue = userData[key];
      const defaultValue = result[key];
      
      if (
        userValue && typeof userValue === 'object' && !Array.isArray(userValue) &&
        defaultValue && typeof defaultValue === 'object' && !Array.isArray(defaultValue)
      ) {
        result[key] = deepMergeForDisplay(defaultValue as Record<string, unknown>, userValue as Record<string, unknown>);
      } else {
        result[key] = userValue;
      }
    });
    
    return result;
  }, []);

  // 从 Schema 中提取默认值并与 formData 深度合并
  const mergedFormData = useMemo(() => {
    const defaults = extractSchemaDefaults(rawSchema);
    return deepMergeForDisplay(defaults, formData);
  }, [rawSchema, formData, extractSchemaDefaults, deepMergeForDisplay]);

  // 过滤掉对象中的空字符串值
  const filterEmptyStrings = useCallback((obj: Record<string, unknown>): Record<string, unknown> => {
    const result: Record<string, unknown> = {};
    
    Object.keys(obj).forEach(key => {
      const value = obj[key];
      
      if (value === '') {
        return;
      }
      
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        const filtered = filterEmptyStrings(value as Record<string, unknown>);
        if (Object.keys(filtered).length > 0) {
          result[key] = filtered;
        }
      } else {
        result[key] = value;
      }
    });
    
    return result;
  }, []);

  // 智能合并函数：AI 数据优先，用户数据作为补充
  const smartMerge = useCallback((userData: Record<string, unknown>, aiData: Record<string, unknown>): Record<string, unknown> => {
    const result = { ...userData };
    
    Object.keys(aiData).forEach(key => {
      const aiValue = aiData[key];
      const userValue = result[key];
      
      if (aiValue === '') {
        return;
      }
      
      if (aiValue && typeof aiValue === 'object' && !Array.isArray(aiValue)) {
        const filteredAiValue = filterEmptyStrings(aiValue as Record<string, unknown>);
        
        if (Object.keys(filteredAiValue).length === 0) {
          return;
        }
        
        if (userValue && typeof userValue === 'object' && !Array.isArray(userValue)) {
          result[key] = smartMerge(userValue as Record<string, unknown>, filteredAiValue);
        } else {
          result[key] = filteredAiValue;
        }
        return;
      }
      
      result[key] = aiValue;
    });
    
    return result;
  }, [filterEmptyStrings]);

  // AI 助手 Hook
  const ai = useAIConfigGenerator({
    moduleId: matchedModuleId || 0,
    workspaceId: id,
    currentFormData: mergedFormData,
    onGenerate: (config: Record<string, unknown>) => {
      const merged = smartMerge(mergedFormData, config);
      setFormData(merged);
    },
  });

  // 编辑协作状态 - 每个窗口独立的session_id（不使用sessionStorage共享）
  const [sessionId] = useState(() => {
    const newId = generateUUID();
    // console.log('🆕 生成新的session ID:', newId);
    return newId;
  });
  const [otherEditors, setOtherEditors] = useState<EditorInfo[]>([]);
  const [hasVersionConflict, setHasVersionConflict] = useState(false);
  const [editingDisabled, setEditingDisabled] = useState(false);
  const [showDriftDialog, setShowDriftDialog] = useState(false);
  const [driftToRecover, setDriftToRecover] = useState<DriftInfo | null>(null);
  const [showTakeoverDialog, setShowTakeoverDialog] = useState(false);
  const [sessionToTakeover, setSessionToTakeover] = useState<EditorInfo | null>(null);
  const [hasShownTakeoverWarning, setHasShownTakeoverWarning] = useState(false);
  const [hasUserEdited, setHasUserEdited] = useState(false);
  const [showEditorsDialog, setShowEditorsDialog] = useState(false);
  const [showRunDialog, setShowRunDialog] = useState(false);
  const [savedResourceName, setSavedResourceName] = useState('');
  
  // WebSocket接管请求状态
  const [showTakeoverRequestDialog, setShowTakeoverRequestDialog] = useState(false);
  const [takeoverRequest, setTakeoverRequest] = useState<any>(null);
  const [showTakeoverWaitingDialog, setShowTakeoverWaitingDialog] = useState(false);
  const [waitingForTakeoverRequestId, setWaitingForTakeoverRequestId] = useState<number | null>(null);

  const heartbeatTimerRef = useRef<number | null>(null);
  const statusPollTimerRef = useRef<number | null>(null);
  const driftSaveTimerRef = useRef<number | null>(null);
  const initialFormDataRef = useRef<any>(null);
  const hasApprovedTakeoverRef = useRef<boolean>(false); // 标记是否已同意接管
  const takenOverSessionIdRef = useRef<string | null>(null); // 记录被接管的session_id
  const hasAutoTakenOverRef = useRef<boolean>(false); // 标记是否已自动接管，防止重复执行
  const hasSubmittedRef = useRef<boolean>(false); // 标记是否已提交，防止cleanup重复删除锁
  
  // 使用ref存储最新状态，解决WebSocket事件处理函数的闭包问题
  const stateRef = useRef({
    sessionToTakeover: null as EditorInfo | null,
    driftToRecover: null as DriftInfo | null,
  });
  
  // 更新stateRef
  useEffect(() => {
    stateRef.current = {
      sessionToTakeover,
      driftToRecover,
    };
  }, [sessionToTakeover, driftToRecover]);

  useEffect(() => {
    // Check if we're in clone mode
    const mode = searchParams.get('mode');
    if (mode === 'clone') {
      setIsCloneMode(true);
    }
  }, [searchParams]);
  
  // 刷新确认弹窗 - 防止用户意外刷新丢失编辑内容
  useEffect(() => {
    if (isCloneMode) return;
    
    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      // 只有在用户有编辑内容时才提示
      if (hasUserEdited) {
        e.preventDefault();
        // 现代浏览器会显示标准提示，不会显示自定义消息
        e.returnValue = '您有未保存的更改，确定要离开吗？';
        return e.returnValue;
      }
    };
    
    window.addEventListener('beforeunload', handleBeforeUnload);
    
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  }, [hasUserEdited, isCloneMode]);

  useEffect(() => {
    const loadResource = async () => {
      try {
        setLoading(true);
        
        // 1. 获取资源信息
        const resourceResponse: any = await api.get(`/workspaces/${id}/resources/${resourceId}`);
        const resourceData = resourceResponse.data?.resource || resourceResponse.resource || resourceResponse;
        setResource(resourceData);
        
        // 2. 从tf_code中提取module配置
        const tfCode = resourceData.current_version?.tf_code || {};
        console.log('Resource TF Code:', tfCode);
        
        let moduleConfig = null;
        let extractedModuleSource = '';
        
        if (tfCode.module) {
          const moduleKeys = Object.keys(tfCode.module);
          if (moduleKeys.length > 0) {
            const moduleKey = moduleKeys[0];
            const moduleArray = tfCode.module[moduleKey];
            if (Array.isArray(moduleArray) && moduleArray.length > 0) {
              moduleConfig = moduleArray[0];
              extractedModuleSource = moduleConfig.source;
            }
          }
        }
        
        setModuleSource(extractedModuleSource);
        
        if (!extractedModuleSource) {
          showToast('无法获取Module信息', 'error');
          return;
        }
        
        // 3. 查找对应的module
        const modulesResponse = await api.get('/modules');
        const modules = modulesResponse.data.items || [];
        
        const matchedModule = modules.find((m: any) => 
          m.module_source === extractedModuleSource || m.source === extractedModuleSource
        );
        
        if (!matchedModule) {
          showToast('找不到对应的Module', 'error');
          return;
        }
        
        // 保存匹配的 Module ID
        setMatchedModuleId(matchedModule.id);
        
        // 4. 加载 Module 版本列表
        try {
          const versionsRes = await listVersions(matchedModule.id);
          const versionItems = versionsRes.items || [];
          setModuleVersions(versionItems);
          
          // 设置默认选中的版本
          const defaultVersion = versionItems.find((v: ModuleVersion) => v.is_default);
          if (defaultVersion) {
            setSelectedVersionId(defaultVersion.id);
          } else if (versionItems.length > 0) {
            setSelectedVersionId(versionItems[0].id);
          }
        } catch (error) {
          console.warn('加载版本列表失败:', error);
        }
        
        // 5. 加载module的schema（使用默认版本）
        const schemaResponse = await api.get(`/modules/${matchedModule.id}/schemas`);
        console.log('Schema API Response:', schemaResponse.data);
        
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
          
          console.log('📊 Active Schema:', activeSchema);
          console.log('📊 Schema Version:', activeSchema.schema_version);
          console.log('📊 Has OpenAPI Schema:', !!activeSchema.openapi_schema);
          
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
            const { source, ...configData } = moduleConfig;
            console.log('📝 Extracted form data:', configData);
            
            // 找出所有有值的字段
            const fieldsWithValues = Object.keys(configData).filter(key => {
              const value = configData[key];
              if (value === null || value === undefined || value === '') return false;
              if (Array.isArray(value) && value.length === 0) return false;
              if (typeof value === 'object' && Object.keys(value).length === 0) return false;
              return true;
            });
            
            console.log('🔑 Fields with values:', fieldsWithValues);
            console.log('📊 Will set initialFieldsToShow to:', fieldsWithValues);
            setInitialFieldsToShow(fieldsWithValues);
            setFormData(configData);
            // 保存初始formData用于比较
            initialFormDataRef.current = JSON.parse(JSON.stringify(configData));
            console.log(' State updated - initialFieldsToShow:', fieldsWithValues);
          }
        } else {
          showToast('该Module暂无Schema定义', 'warning');
        }
        
        // 6. 加载 workspace 中其他资源的 available-outputs（用于资源引用）
        let otherResources: WorkspaceResourceNode[] = [];
        try {
          const availableOutputsResponse: any = await api.get(`/workspaces/${id}/available-outputs`);
          const availableResources = availableOutputsResponse.resources || [];
          
          // 过滤掉当前资源，转换为 WorkspaceResourceNode 格式
          otherResources = availableResources
            .filter((r: any) => r.resourceId !== resourceData.resource_id)
            .map((r: any) => ({
              id: r.resourceId,
              resource_name: r.resourceName,
              resource_type: r.resourceType,
              // 构建 tf_module_key：格式为 {resource_type}_{resource_name}
              // 例如：AWS_network-policy_outpu
              tf_module_key: `${r.resourceType}_${r.resourceName}`,
              module_id: r.moduleId,
              module_source: r.moduleName,
              outputs: r.outputs?.map((o: any) => ({
                name: o.name,
                type: o.type,
                description: o.description,
              })) || [],
            }));
          
          console.log(`📊 Loaded ${otherResources.length} other resources for reference`);
        } catch (error) {
          console.warn('加载 available-outputs 失败:', error);
          // 不影响主流程，只是引用功能不可用
        }
        
        // 7. 加载 workspace 的 remote data 配置（用于引用其他 workspace 的 outputs）
        let remoteDataList: RemoteDataNode[] = [];
        try {
          const remoteDataResponse: any = await api.get(`/workspaces/${id}/remote-data`);
          const remoteDataItems = remoteDataResponse.remote_data || [];
          
          // 转换为 RemoteDataNode 格式
          remoteDataList = await Promise.all(
            remoteDataItems.map(async (item: any) => {
              // 获取源 workspace 的 outputs
              let availableOutputs: any[] = [];
              try {
                const outputsResponse: any = await api.get(
                  `/workspaces/${id}/remote-data/source-outputs?source_workspace_id=${item.source_workspace_id}`
                );
                availableOutputs = (outputsResponse.outputs || []).map((o: any) => ({
                  key: o.key,
                  type: o.type,
                  sensitive: o.sensitive,
                  value: o.value,
                }));
              } catch (err) {
                console.warn(`获取 remote data ${item.data_name} 的 outputs 失败:`, err);
              }
              
              return {
                remote_data_id: item.remote_data_id,
                data_name: item.data_name,
                source_workspace_id: item.source_workspace_id,
                source_workspace_name: item.source_workspace_name,
                description: item.description,
                available_outputs: availableOutputs,
              };
            })
          );
          
          console.log(`📊 Loaded ${remoteDataList.length} remote data references`);
        } catch (error) {
          console.warn('加载 remote-data 失败:', error);
          // 不影响主流程，只是远程数据引用功能不可用
        }
        
        // 设置 workspace 资源引用上下文（包含本地资源和远程数据）
        if (otherResources.length > 0 || remoteDataList.length > 0) {
          setWorkspaceResourceContext({
            workspaceId: id!,
            currentResourceId: resourceData.resource_id,
            resources: otherResources,
            remoteData: remoteDataList.length > 0 ? remoteDataList : undefined,
          });
        }
      } catch (error: any) {
        console.error('加载资源失败:', error);
        showToast(extractErrorMessage(error), 'error');
      } finally {
        setLoading(false);
      }
    };
    
    loadResource();
  }, [id, resourceId]);

  // 编辑协作生命周期
  useEffect(() => {
    if (!id || !resourceId || isCloneMode) return;
    
    // 连接WebSocket
    websocketService.connect(sessionId);
    // console.log('🔌 WebSocket连接已建立');
    
    // 监听WebSocket连接状态
    let wsConnected = false;
    const checkWSConnection = () => {
      const isConnected = websocketService.isConnected();
      if (isConnected !== wsConnected) {
        wsConnected = isConnected;
        console.log(`🔌 WebSocket状态变化: ${isConnected ? '已连接' : '已断开'}`);
      }
      return isConnected;
    };
    
    // 监听接管请求（被接管方）
    const handleTakeoverRequest = (data: any) => {
      console.log('🔔 WebSocket收到接管请求:', data);
      setTakeoverRequest(data);
      setShowTakeoverRequestDialog(true);
    };
    
    // 监听接管结果（接管方）- 使用stateRef解决闭包问题
    const handleTakeoverApproved = (data: any) => {
      console.log(' WebSocket收到接管批准通知');
      setShowTakeoverWaitingDialog(false);
      setWaitingForTakeoverRequestId(null);
      showToast('接管成功', 'success');
      
      // 使用stateRef获取最新状态，解决闭包问题
      const currentSessionToTakeover = stateRef.current.sessionToTakeover;
      const currentDriftToRecover = stateRef.current.driftToRecover;
      
      // 记录被接管的session_id
      if (currentSessionToTakeover) {
        takenOverSessionIdRef.current = currentSessionToTakeover.session_id;
        console.log('📝 记录被接管的session_id:', currentSessionToTakeover.session_id);
      }
      
      // 重置状态
      setEditingDisabled(false);
      setHasShownTakeoverWarning(false);
      setOtherEditors([]);
      setSessionToTakeover(null);
      setShowTakeoverDialog(false);
      
      // 如果有drift，显示恢复对话框
      if (currentDriftToRecover) {
        setShowDriftDialog(true);
      }
    };
    
    const handleTakeoverRejected = () => {
      console.log('❌ WebSocket收到接管拒绝通知');
      setShowTakeoverWaitingDialog(false);
      setWaitingForTakeoverRequestId(null);
      showToast('对方拒绝了接管请求', 'warning');
      
      // 清理并返回
      const storageKey = `editing_session_${id}_${resourceId}`;
      sessionStorage.removeItem(storageKey);
      navigate(`/workspaces/${id}/resources/${resourceId}`);
    };
    
    const handleForceTakeover = () => {
      console.log(' WebSocket收到强制接管通知');
      showToast('您的编辑会话已被强制接管', 'warning');
      
      // 停止所有定时器
      if (heartbeatTimerRef.current) {
        clearInterval(heartbeatTimerRef.current);
        heartbeatTimerRef.current = null;
      }
      if (statusPollTimerRef.current) {
        clearInterval(statusPollTimerRef.current);
        statusPollTimerRef.current = null;
      }
      
      // 清理并返回
      const storageKey = `editing_session_${id}_${resourceId}`;
      sessionStorage.removeItem(storageKey);
      navigate(`/workspaces/${id}/resources/${resourceId}`);
    };
    
    // 注册事件监听
    websocketService.on('takeover_request', handleTakeoverRequest);
    websocketService.on('takeover_approved', handleTakeoverApproved);
    websocketService.on('takeover_rejected', handleTakeoverRejected);
    websocketService.on('force_takeover', handleForceTakeover);
    
    const initEditing = async () => {
      try {
        if (!id || !resourceId) {
          console.error('Missing workspace or resource ID');
          return;
        }
        
        const response = await ResourceEditingService.startEditing(
          id, // 直接使用字符串ID，支持语义化ID
          Number(resourceId),
          sessionId
        );
        
        console.log('🔒 编辑会话已启动:', response);
        console.log('📊 Other editors:', response.other_editors);
        console.log('🆔 Current session ID:', sessionId);
        
        setOtherEditors(response.other_editors);
        
        // 检查是否有其他编辑者
        if (response.other_editors.length > 0) {
          // 有其他编辑者
          const firstEditor = response.other_editors[0];
          console.log('🔔 检测到其他编辑者:');
          console.log('  - 当前session:', sessionId);
          console.log('  - 其他编辑者session:', firstEditor.session_id);
          console.log('  - 是否同一用户:', firstEditor.is_same_user);
          
          // 确保不是自己的当前session
          if (firstEditor.session_id === sessionId) {
            console.error('❌ 错误：检测到的其他编辑者是自己！');
            return;
          }
          
          // 如果有drift,暂存起来
          if (response.has_drift && response.drift) {
            setDriftToRecover(response.drift);
            setHasVersionConflict(response.has_version_conflict);
          }
          
          // 无论是同一用户还是不同用户，都显示接管确认对话框
          // 让用户明确知道有其他窗口正在编辑，并确认是否接管
          setSessionToTakeover(firstEditor);
          setShowTakeoverDialog(true);
          console.log(' 显示接管对话框，is_same_user:', firstEditor.is_same_user);
        } else if (response.has_drift && response.drift) {
          // 没有其他窗口,直接显示drift恢复对话框
          setDriftToRecover(response.drift);
          setShowDriftDialog(true);
          setHasVersionConflict(response.has_version_conflict);
        }
        
        // 启动心跳 - 5秒一次，保持编辑锁活跃
        if (id && resourceId) {
          heartbeatTimerRef.current = window.setInterval(async () => {
            try {
              await ResourceEditingService.heartbeat(id, Number(resourceId), sessionId);
            } catch (error) {
              // 心跳失败说明锁已被删除或接管,静默停止所有定时器
              // console.log('⏸️ 心跳失败，锁可能已被删除或接管');
              if (heartbeatTimerRef.current) {
                clearInterval(heartbeatTimerRef.current);
                heartbeatTimerRef.current = null;
              }
              if (statusPollTimerRef.current) {
                clearInterval(statusPollTimerRef.current);
                statusPollTimerRef.current = null;
              }
            }
          }, 5000); // 5秒一次
        }
        
        // 启动状态轮询作为降级方案（仅在WebSocket断开时使用）
        const MAX_CONSECUTIVE_FAILURES = 3;
        let consecutiveFailures = 0;
        
        if (id && resourceId) {
          statusPollTimerRef.current = window.setInterval(async () => {
            // 只在WebSocket断开时执行轮询
            const wsConnected = checkWSConnection();
            if (wsConnected) {
              // console.log('⏭️ WebSocket已连接，跳过HTTP轮询');
              return;
            }
            
            // console.log('🔄 WebSocket断开，使用HTTP轮询降级');
            
            try {
              const status = await ResourceEditingService.getEditingStatus(
                id,
                Number(resourceId),
                sessionId
              );
            
            consecutiveFailures = 0;
            
            const filteredEditors = status.editors.filter(e => !e.is_current_session);
            setOtherEditors(filteredEditors);
            
            const currentSession = status.editors.find(e => e.is_current_session);
            
            if (status.editors.length > 0 && !currentSession && !editingDisabled && !hasShownTakeoverWarning && heartbeatTimerRef.current && !takenOverSessionIdRef.current) {
              console.warn(' 未找到当前session,可能被接管');
              setEditingDisabled(true);
              setHasShownTakeoverWarning(true);
              showToast('编辑已被其他窗口接管', 'warning');
              if (statusPollTimerRef.current) {
                clearInterval(statusPollTimerRef.current);
                statusPollTimerRef.current = null;
              }
            }
            
            // 检查pending请求
            try {
              const pendingRequests: any = await api.get(
                `/workspaces/${id}/resources/${resourceId}/editing/pending-requests?target_session=${sessionId}`
              );
              
              const requests = pendingRequests.requests || [];
              if (requests.length > 0) {
                const request = requests[0];
                console.log('🔔 HTTP轮询检测到接管请求:', request);
                setTakeoverRequest(request);
                setShowTakeoverRequestDialog(true);
              }
            } catch (error) {
              console.error('检查接管请求失败:', error);
            }
          } catch (error) {
            console.error('状态轮询失败:', error);
            consecutiveFailures++;
            
            if (consecutiveFailures >= MAX_CONSECUTIVE_FAILURES && !editingDisabled && !hasShownTakeoverWarning) {
              console.warn(' 连续多次状态轮询失败');
              setEditingDisabled(true);
              setHasShownTakeoverWarning(true);
              showToast('编辑会话已断开,请刷新页面重新编辑', 'warning');
              if (statusPollTimerRef.current) {
                clearInterval(statusPollTimerRef.current);
                statusPollTimerRef.current = null;
              }
            }
          }
          }, 3000);
        }
        
      } catch (error) {
        console.error('初始化编辑会话失败:', error);
      }
    };
    
    initEditing();
    
    return () => {
      // console.log('🧹 清理编辑会话...');
      
      // 断开WebSocket
      websocketService.disconnect();
      console.log(' WebSocket已断开');
      
      // 清理定时器
      if (heartbeatTimerRef.current) {
        clearInterval(heartbeatTimerRef.current);
        heartbeatTimerRef.current = null;
        console.log(' 心跳定时器已清理');
      }
      if (statusPollTimerRef.current) {
        clearInterval(statusPollTimerRef.current);
        statusPollTimerRef.current = null;
        console.log(' 状态轮询定时器已清理');
      }
      if (driftSaveTimerRef.current) {
        clearTimeout(driftSaveTimerRef.current);
        driftSaveTimerRef.current = null;
        console.log(' 草稿保存定时器已清理');
      }
      
      // 页面卸载时立即保存一次草稿(只在有编辑时)
      if (id && resourceId && hasUserEdited && formData && Object.keys(formData).length > 0) {
        console.log('💾 页面卸载,保存草稿...');
        ResourceEditingService.saveDrift(
          id,
          Number(resourceId),
          sessionId,
          { formData, changeSummary }
        ).catch(console.error);
      }
      
      // 结束编辑会话（如果已同意接管或已提交，则跳过）
      if (id && resourceId && !hasApprovedTakeoverRef.current && !hasSubmittedRef.current) {
        console.log('🔚 结束编辑会话...');
        ResourceEditingService.endEditing(
          id,
          Number(resourceId),
          sessionId
        ).catch(console.error);
      } else if (hasApprovedTakeoverRef.current) {
        console.log('⏭️ 已同意接管，跳过endEditing');
      } else if (hasSubmittedRef.current) {
        console.log('⏭️ 已提交，跳过endEditing');
      }
    };
  }, [id, resourceId, isCloneMode, sessionId]);

  // 独立的接管请求状态轮询（解决闭包问题）
  useEffect(() => {
    if (!waitingForTakeoverRequestId || !id || !resourceId) return;
    
    console.log('🔄 启动接管请求状态轮询，request_id:', waitingForTakeoverRequestId);
    
    const pollTimer = window.setInterval(async () => {
      try {
        console.log('🔍 轮询检查请求状态，request_id:', waitingForTakeoverRequestId);
        
        const requestStatus: any = await api.get(
          `/workspaces/${id}/resources/${resourceId}/editing/request-status/${waitingForTakeoverRequestId}`
        );
        
        console.log('🔍 请求状态响应:', requestStatus);
        console.log('🔍 当前状态:', requestStatus.status);
        
        if (requestStatus.status === 'approved') {
          console.log(' 接管被批准');
          setShowTakeoverWaitingDialog(false);
          setWaitingForTakeoverRequestId(null);
          showToast('接管成功', 'success');
          
          // 记录被接管的session_id，用于过滤状态轮询结果
          if (sessionToTakeover) {
            takenOverSessionIdRef.current = sessionToTakeover.session_id;
            console.log('📝 记录被接管的session_id:', sessionToTakeover.session_id);
          }
          
          // 重置被接管的状态标志，允许继续编辑
          setEditingDisabled(false);
          setHasShownTakeoverWarning(false);
          
          // 不刷新页面，直接清理状态并继续编辑
          setOtherEditors([]);
          setSessionToTakeover(null);
          setShowTakeoverDialog(false);
          
          // 如果有drift，显示恢复对话框
          if (driftToRecover) {
            setShowDriftDialog(true);
          }
        } else if (requestStatus.status === 'rejected') {
          console.log('❌ 接管被拒绝');
          setShowTakeoverWaitingDialog(false);
          setWaitingForTakeoverRequestId(null);
          showToast('对方拒绝了接管请求', 'warning');
          
          // 清理sessionStorage并返回资源查看页面
          const storageKey = `editing_session_${id}_${resourceId}`;
          sessionStorage.removeItem(storageKey);
          navigate(`/workspaces/${id}/resources/${resourceId}`);
        } else if (requestStatus.status === 'expired') {
          console.log('⏰ 接管请求超时');
          setShowTakeoverWaitingDialog(false);
          setWaitingForTakeoverRequestId(null);
          showToast('接管请求已超时', 'warning');
          
          // 清理sessionStorage并返回资源查看页面
          const storageKey = `editing_session_${id}_${resourceId}`;
          sessionStorage.removeItem(storageKey);
          navigate(`/workspaces/${id}/resources/${resourceId}`);
        } else {
          console.log('⏳ 请求仍在pending状态');
        }
      } catch (error) {
        console.error('检查请求状态失败:', error);
      }
    }, 2000); // 2秒轮询一次，更快响应
    
    return () => {
      console.log('🧹 清理接管请求状态轮询');
      clearInterval(pollTimer);
    };
  }, [waitingForTakeoverRequestId, id, resourceId, showToast, sessionToTakeover, driftToRecover, navigate]);

  // 检测用户是否编辑了内容
  useEffect(() => {
    if (!initialFormDataRef.current || loading) return;
    
    // 比较当前formData和初始formData
    const hasChanged = JSON.stringify(formData) !== JSON.stringify(initialFormDataRef.current);
    if (hasChanged && !hasUserEdited) {
      setHasUserEdited(true);
      console.log('✏️ 检测到用户编辑');
    }
  }, [formData, loading, hasUserEdited]);

  // 版本切换时重新加载 Schema
  const handleVersionChange = async (newVersionId: string) => {
    if (!matchedModuleId || newVersionId === selectedVersionId) return;
    
    setSelectedVersionId(newVersionId);
    setLoadingVersionSchema(true);
    
    try {
      // 使用 schemaV2Service 获取指定版本的 Schema
      const schemaData = await schemaV2Service.getSchemaV2(matchedModuleId, newVersionId);
      
      if (schemaData?.openapi_schema) {
        setRawSchema(schemaData);
        setSchema(schemaData as any);
        showToast('已切换到新版本的 Schema', 'success');
      } else {
        showToast('该版本没有可用的 Schema', 'warning');
        setViewMode('json');
      }
    } catch (error) {
      console.error('加载版本 Schema 失败:', error);
      showToast('加载版本 Schema 失败', 'error');
    } finally {
      setLoadingVersionSchema(false);
    }
  };

  // 草稿自动保存
  useEffect(() => {
    // 跳过克隆模式和初始加载
    if (!id || !resourceId || isCloneMode || loading) return;
    
    // 只有在formData有内容时才保存
    if (!formData || Object.keys(formData).length === 0) return;
    
    // 只有用户编辑过才保存草稿
    if (!hasUserEdited && !changeSummary) return;
    
    if (driftSaveTimerRef.current) {
      clearTimeout(driftSaveTimerRef.current);
    }
    
    driftSaveTimerRef.current = window.setTimeout(async () => {
      try {
        if (!id || !resourceId) {
          console.error('Missing workspace or resource ID for drift save');
          return;
        }
        
        console.log('💾 自动保存草稿:', { formData, changeSummary });
        await ResourceEditingService.saveDrift(
          id,
          Number(resourceId),
          sessionId,
          { formData, changeSummary }
        );
        console.log(' 草稿保存成功');
        showToast('草稿已自动保存', 'success');
      } catch (error: any) {
        console.error('保存草稿失败:', error);
        const errorMsg = error?.response?.data?.error || error?.message || '未知错误';
        showToast(`保存草稿失败: ${errorMsg}`, 'error');
      }
    }, 2000); // 改为2秒防抖，减少API调用频率
  }, [formData, changeSummary, id, resourceId, isCloneMode, sessionId, loading, hasUserEdited]);

  const handleSubmit = async (shouldRunAfter: boolean = false) => {
    console.log('🚀 EditResource handleSubmit 开始');
    console.log('📝 shouldRunAfter:', shouldRunAfter);
    console.log('📝 isCloneMode:', isCloneMode);
    
    // 验证变更摘要
    if (!changeSummary.trim()) {
      setChangeSummaryError('请输入变更摘要');
      showToast('请输入变更摘要', 'warning');
      // 自动滚动到摘要输入框并聚焦
      changeSummaryRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
      changeSummaryRef.current?.focus();
      return;
    }
    
    try {
      setSubmitting(true);
      
      let finalResourceName = '';
      
      // 获取选中版本的版本号（用于写入 tf_code）
      let moduleVersionStr = '';
      if (selectedVersionId && moduleVersions.length > 0) {
        const selectedVersion = moduleVersions.find(v => v.id === selectedVersionId);
        if (selectedVersion?.version) {
          moduleVersionStr = selectedVersion.version;
          console.log(`📦 Using selected version: ${moduleVersionStr}`);
        }
      }
      
      // 如果没有选中版本，尝试获取默认版本
      if (!moduleVersionStr && matchedModuleId) {
        try {
          const defaultVersion = await getDefaultVersion(matchedModuleId);
          if (defaultVersion?.version) {
            moduleVersionStr = defaultVersion.version;
            console.log(`📦 Using default version: ${moduleVersionStr}`);
          }
        } catch (error) {
          console.warn('Failed to get default version:', error);
        }
      }
      
      if (isCloneMode) {
        // 克隆模式：创建新资源
        // 生成唯一的资源名称（使用短时间戳，只取后6位）
        const timestamp = Date.now().toString().slice(-6);
        
        // 如果原名称已经包含_clone，移除所有_clone后缀，只保留基础名称
        let baseName = resource?.resource_name || '';
        baseName = baseName.replace(/_clone(_\d+)?/g, '');
        
        const newResourceName = `${baseName}_clone_${timestamp}`;
        finalResourceName = newResourceName;
        
        // 构建 module 配置
        const moduleConfig: Record<string, any> = {
          source: moduleSource,
          ...formData
        };
        
        // 添加版本信息
        if (moduleVersionStr) {
          moduleConfig.version = moduleVersionStr;
          console.log(`📦 Adding version ${moduleVersionStr} to cloned resource`);
        }
        
        // 创建资源
        const newTFCode = {
          module: {
            [`${resource?.resource_type}_${newResourceName}`]: [moduleConfig]
          }
        };
        
        try {
          await api.post(`/workspaces/${id}/resources`, {
            resource_type: resource?.resource_type,
            resource_name: newResourceName,
            tf_code: newTFCode,
            variables: resource?.current_version?.variables || {},
            change_summary: changeSummary.trim()
          });
          
          showToast('资源克隆成功', 'success');
          
          if (shouldRunAfter) {
            // 保存资源名称并打开运行对话框
            setSavedResourceName(newResourceName);
            setShowRunDialog(true);
          } else {
            navigate(`/workspaces/${id}?tab=resources`);
          }
        } catch (createError) {
          showToast('资源克隆失败', 'error');
          throw createError;
        }
      } else {
        // 编辑模式：更新现有资源
        finalResourceName = resource?.resource_name || '';
        
        // 构建 module 配置
        const editModuleConfig: Record<string, any> = {
          source: moduleSource,
          ...formData
        };
        
        // 添加版本信息
        if (moduleVersionStr) {
          editModuleConfig.version = moduleVersionStr;
          console.log(`📦 Adding version ${moduleVersionStr} to edited resource`);
        }
        
        // 更新资源
        const updatedTFCode = {
          module: {
            [`${resource?.resource_type}_${resource?.resource_name}`]: [editModuleConfig]
          }
        };
        
        try {
          await api.put(`/workspaces/${id}/resources/${resourceId}`, {
            tf_code: updatedTFCode,
            variables: resource?.current_version?.variables || {},
            change_summary: changeSummary.trim()
          });
          
          showToast('资源更新成功', 'success');
          
          // 设置标志，防止cleanup重复删除
          hasSubmittedRef.current = true;
          
          // 立即结束编辑会话，删除锁
          try {
            await ResourceEditingService.endEditing(id!, Number(resourceId), sessionId);
            console.log(' 提交成功后已结束编辑会话');
          } catch (error) {
            console.error('结束编辑会话失败:', error);
          }
          
          // 清理sessionStorage
          const storageKey = `editing_session_${id}_${resourceId}`;
          sessionStorage.removeItem(storageKey);
          
          if (shouldRunAfter) {
            // 保存资源名称并打开运行对话框
            setSavedResourceName(finalResourceName);
            setShowRunDialog(true);
          } else {
            navigate(`/workspaces/${id}?tab=resources`);
          }
        } catch (updateError) {
          showToast('资源更新失败', 'error');
          throw updateError;
        }
      }
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
      // 保留用户输入
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = () => {
    console.log('🚪 取消编辑，清理所有定时器');
    
    // 先停止所有定时器，避免在导航过程中触发弹窗
    if (heartbeatTimerRef.current) {
      clearInterval(heartbeatTimerRef.current);
      heartbeatTimerRef.current = null;
    }
    if (statusPollTimerRef.current) {
      clearInterval(statusPollTimerRef.current);
      statusPollTimerRef.current = null;
    }
    if (driftSaveTimerRef.current) {
      clearTimeout(driftSaveTimerRef.current);
      driftSaveTimerRef.current = null;
    }
    
    // 清理sessionStorage中的session_id
    const storageKey = `editing_session_${id}_${resourceId}`;
    sessionStorage.removeItem(storageKey);
    console.log('🗑️ 已清理sessionStorage');
    
    // 返回到资源查看页面，而不是资源列表
    navigate(`/workspaces/${id}/resources/${resourceId}`);
  };

  const handleChangeSummaryChange = (value: string) => {
    setChangeSummary(value);
    if (changeSummaryError && value.trim()) {
      setChangeSummaryError('');
    }
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
          <button onClick={handleCancel} className={styles.backButton}>
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
          {/* 编辑状态栏 */}
          {!isCloneMode && (
            <EditingStatusBar
              otherEditors={otherEditors}
              hasVersionConflict={hasVersionConflict}
              isDisabled={editingDisabled}
              onShowDetails={() => setShowEditorsDialog(true)}
            />
          )}

      {/* 编辑者详情对话框 */}
      {showEditorsDialog && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0,0,0,0.5)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 1000
        }}>
          <div style={{
            background: 'white',
            borderRadius: '12px',
            padding: '24px',
            maxWidth: '500px',
            width: '90%'
          }}>
            <h3 style={{ margin: '0 0 16px 0' }}>正在编辑的用户</h3>
            <div style={{ marginBottom: '20px' }}>
              {otherEditors.map((editor, index) => (
                <div key={index} style={{
                  padding: '12px',
                  background: '#f9fafb',
                  borderRadius: '8px',
                  marginBottom: '8px'
                }}>
                  <div style={{ fontWeight: 500, marginBottom: '4px' }}>
                    {editor.user_name} {editor.is_same_user && '(您)'}
                  </div>
                  <div style={{ fontSize: '13px', color: '#6b7280' }}>
                    会话ID: {editor.session_id.substring(0, 8)}...
                  </div>
                  <div style={{ fontSize: '13px', color: '#6b7280' }}>
                    最后活动: {editor.time_since_heartbeat}秒前
                  </div>
                </div>
              ))}
            </div>
            <button
              onClick={() => setShowEditorsDialog(false)}
              style={{
                padding: '10px 20px',
                background: '#3b82f6',
                color: 'white',
                border: 'none',
                borderRadius: '6px',
                cursor: 'pointer',
                width: '100%'
              }}
            >
              关闭
            </button>
          </div>
        </div>
      )}

      {/* 草稿恢复对话框 */}
      {showDriftDialog && driftToRecover && (
        <DriftRecoveryDialog
          drift={driftToRecover}
          hasVersionConflict={hasVersionConflict}
          resourceId={resource?.resource_id}
          resourceName={resource?.resource_name}
          onRecover={() => {
            setFormData(driftToRecover.drift_content.formData);
            setChangeSummary(driftToRecover.drift_content.changeSummary);
            setShowDriftDialog(false);
            setHasUserEdited(true);
          }}
          onDiscard={async () => {
            try {
              // 删除drift需要按user_id删除,不是按session_id
              const response = await api.delete(
                `/workspaces/${id}/resources/${resourceId}/drift`,
                { 
                  params: { session_id: driftToRecover.session_id || sessionId },
                  data: { user_id: driftToRecover.user_id }
                }
              );
              setShowDriftDialog(false);
              setDriftToRecover(null);
              showToast('草稿已删除', 'success');
            } catch (error: any) {
              console.error('删除草稿失败:', error);
              const errorMsg = error?.response?.data?.error || error?.message || '删除失败';
              showToast(`删除草稿失败: ${errorMsg}`, 'error');
            }
          }}
          onCancel={() => setShowDriftDialog(false)}
        />
      )}

      {/* 接管确认对话框（接管方） */}
      {showTakeoverDialog && sessionToTakeover && (
        <TakeoverConfirmDialog
          otherSession={sessionToTakeover}
          onConfirm={async (forceTakeover: boolean) => {
            console.log('🚀 点击了接管按钮, forceTakeover:', forceTakeover);
            console.log('🚀 target_session_id:', sessionToTakeover.session_id);
            
            try {
              if (forceTakeover) {
                // 强制接管：直接调用TakeoverEditing，不需要等待确认
                console.log('🚀 开始强制接管...');
                
                await api.post(
                  `/workspaces/${id}/resources/${resourceId}/editing/force-takeover`,
                  { 
                    target_session_id: sessionToTakeover.session_id,
                    requester_session_id: sessionId  // 传递当前编辑session_id
                  }
                );
                
                console.log(' 强制接管成功');
                setShowTakeoverDialog(false);
                showToast('强制接管成功', 'success');
                
                // 记录被接管的session_id
                takenOverSessionIdRef.current = sessionToTakeover.session_id;
                
                // 重置状态
                setEditingDisabled(false);
                setHasShownTakeoverWarning(false);
                setOtherEditors([]);
                setSessionToTakeover(null);
                
                // 如果有drift，显示恢复对话框
                if (driftToRecover) {
                  setShowDriftDialog(true);
                }
              } else {
                // 普通接管：发送请求，等待对方确认
                console.log('🚀 开始发送接管请求...');
                
                const response = await api.post(
                  `/workspaces/${id}/resources/${resourceId}/editing/takeover-request`,
                  { 
                    target_session_id: sessionToTakeover.session_id,
                    requester_session_id: sessionId  // 传递当前编辑session_id
                  }
                );
              
                console.log('接管请求响应:', response);
                console.log('response类型:', typeof response);
                console.log('response.request_id:', (response as any)?.request_id);
                
                // 注意：axios拦截器返回response.data，所以response直接就是数据
                const requestId = (response as any)?.request_id;
                
                if (requestId) {
                  setWaitingForTakeoverRequestId(requestId);
                  setShowTakeoverDialog(false);
                  setShowTakeoverWaitingDialog(true);
                  console.log(' 接管请求已发送，request_id:', requestId);
                  console.log(' waitingForTakeoverRequestId已设置为:', requestId);
                } else {
                  console.error('响应格式错误，response:', response);
                  console.error('requestId:', requestId);
                  showToast('接管请求响应格式错误', 'error');
                }
              }
            } catch (error: any) {
              console.error('接管请求失败:', error);
              const errorMsg = error?.response?.data?.error || error?.message || '发送接管请求失败';
              
              // 如果目标session已过期，提示刷新页面
              if (errorMsg.includes('不存在或已过期')) {
                showToast('对方已离开编辑，请刷新页面重新编辑', 'info');
                setShowTakeoverDialog(false);
                // 延迟刷新，让用户看到提示
                setTimeout(() => {
                  window.location.reload();
                }, 1500);
              } else {
                showToast(errorMsg, 'error');
                setShowTakeoverDialog(false);
              }
            }
          }}
          onCancel={() => {
            setShowTakeoverDialog(false);
            // 取消接管时，清理 sessionStorage 并返回资源查看页面
            const storageKey = `editing_session_${id}_${resourceId}`;
            sessionStorage.removeItem(storageKey);
            navigate(`/workspaces/${id}/resources/${resourceId}`);
          }}
        />
      )}

      {/* 接管请求对话框（被接管方） */}
      {showTakeoverRequestDialog && takeoverRequest && (
        <TakeoverRequestDialog
          request={takeoverRequest}
          onApprove={async () => {
            try {
              // 设置标志，防止cleanup中调用endEditing
              hasApprovedTakeoverRef.current = true;
              
              // 先停止心跳，防止被接管后心跳重新创建锁
              if (heartbeatTimerRef.current) {
                clearInterval(heartbeatTimerRef.current);
                heartbeatTimerRef.current = null;
              }
              if (statusPollTimerRef.current) {
                clearInterval(statusPollTimerRef.current);
                statusPollTimerRef.current = null;
              }
              
              await api.post(
                `/workspaces/${id}/resources/${resourceId}/editing/takeover-response`,
                { request_id: takeoverRequest.id, approved: true }
              );
              
              setShowTakeoverRequestDialog(false);
              showToast('已同意接管', 'info');
              
              // 清理并返回资源查看页面
              const storageKey = `editing_session_${id}_${resourceId}`;
              sessionStorage.removeItem(storageKey);
              navigate(`/workspaces/${id}/resources/${resourceId}`);
            } catch (error: any) {
              showToast('响应接管请求失败', 'error');
            }
          }}
          onReject={async () => {
            try {
              await api.post(
                `/workspaces/${id}/resources/${resourceId}/editing/takeover-response`,
                { request_id: takeoverRequest.id, approved: false }
              );
              
              setShowTakeoverRequestDialog(false);
              showToast('已拒绝接管', 'info');
            } catch (error: any) {
              showToast('响应接管请求失败', 'error');
            }
          }}
        />
      )}

      {/* 接管等待对话框（接管方） */}
      {showTakeoverWaitingDialog && sessionToTakeover && (
        <TakeoverWaitingDialog
          targetUserName={sessionToTakeover.user_name}
          isSameUser={sessionToTakeover.is_same_user}
          onCancel={() => {
            setShowTakeoverWaitingDialog(false);
            setWaitingForTakeoverRequestId(null);
            // 取消请求后返回资源查看页面
            const storageKey = `editing_session_${id}_${resourceId}`;
            sessionStorage.removeItem(storageKey);
            navigate(`/workspaces/${id}/resources/${resourceId}`);
          }}
        />
      )}

      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <button onClick={handleCancel} className={styles.backButton}>
            ← 返回Workspace
          </button>
          <h1 className={styles.title}>{isCloneMode ? '克隆资源' : '编辑资源'}</h1>
        </div>
        
        <div className={styles.resourceInfo}>
          <span className={styles.resourceType}>{resource.resource_type}</span>
          <span className={styles.resourceSeparator}>·</span>
          <span className={styles.resourceName}>
            {isCloneMode ? `${resource.resource_name.replace(/_clone(_\d+)?/g, '')}_clone_[...]` : resource.resource_name}
          </span>
          {!isCloneMode && (
            <>
              <span className={styles.resourceSeparator}>·</span>
              <span className={styles.versionInfo}>v{resource.current_version?.version || 1}</span>
            </>
          )}
          {isCloneMode && (
            <>
              <span className={styles.resourceSeparator}>·</span>
              <span style={{ 
                padding: '4px 8px', 
                background: 'var(--color-blue-100)', 
                color: 'var(--color-blue-700)',
                borderRadius: '4px',
                fontSize: '12px',
                fontWeight: 500
              }}>
                克隆模式
              </span>
            </>
          )}
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.configureStep}>
          <h2 className={styles.stepTitle}>修改配置</h2>
          
          {schema && (
            <div className={styles.dynamicFormContainer}>
              <div className={styles.formDescription} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '12px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                  <span style={{ fontSize: '14px', color: '#333', fontWeight: 500 }}>
                    基于Module Schema自动生成的配置表单
                  </span>
                  {rawSchema?.schema_version === 'v2' && rawSchema?.openapi_schema && (
                    <span style={{ 
                      padding: '2px 8px', 
                      background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
                      color: 'white',
                      borderRadius: '4px',
                      fontSize: '11px',
                      fontWeight: 600
                    }}>
                      OpenAPI v3
                    </span>
                  )}
                  {/* AI 助手按钮 - 和标签在同一行 */}
                  {rawSchema?.schema_version === 'v2' && rawSchema?.openapi_schema && matchedModuleId && (
                    <AITriggerButton
                      expanded={ai.expanded}
                      onClick={() => ai.setExpanded(!ai.expanded)}
                    />
                  )}
                </div>
                
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                  {/* Module 版本选择器 - 始终显示（如果有版本） */}
                  {moduleVersions.length > 0 && (
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span style={{ fontSize: '13px', color: '#64748b' }}>TF Module:</span>
                      <select
                        value={selectedVersionId}
                        onChange={(e) => handleVersionChange(e.target.value)}
                        disabled={loadingVersionSchema || moduleVersions.length <= 1}
                        style={{
                          padding: '4px 8px',
                          border: '1px solid #e2e8f0',
                          borderRadius: '6px',
                          fontSize: '13px',
                          color: '#334155',
                          background: 'white',
                          cursor: loadingVersionSchema || moduleVersions.length <= 1 ? 'default' : 'pointer',
                          minWidth: '120px'
                        }}
                      >
                        {moduleVersions.map(v => (
                          <option key={v.id} value={v.id}>
                            {v.version} {v.is_default ? '(默认)' : ''}
                          </option>
                        ))}
                      </select>
                      {loadingVersionSchema && (
                        <span style={{ fontSize: '12px', color: '#94a3b8' }}>加载中...</span>
                      )}
                    </div>
                  )}
                  
                  {/* 视图切换按钮 */}
                  <div className={styles.viewToggle}>
                    <button
                      className={`${styles.viewButton} ${viewMode === 'form' ? styles.viewButtonActive : ''}`}
                      onClick={() => {
                        setViewMode('form');
                        setFormRenderError(false);
                      }}
                      title={formRenderError ? '点击重新尝试表单视图' : '切换到表单视图'}
                    >
                      表单视图
                    </button>
                    <button
                      className={`${styles.viewButton} ${viewMode === 'json' ? styles.viewButtonActive : ''}`}
                      onClick={() => setViewMode('json')}
                    >
                      JSON视图
                    </button>
                  </div>
                </div>
              </div>
              
              {/* AI 输入面板 - 贯穿式显示在标题栏下方 */}
              {ai.expanded && rawSchema?.schema_version === 'v2' && rawSchema?.openapi_schema && matchedModuleId && (
                <AIInputPanel
                  description={ai.description}
                  onDescriptionChange={ai.setDescription}
                  onGenerate={ai.handleGenerate}
                  onClose={() => ai.setExpanded(false)}
                  loading={ai.loading}
                  generateMode={ai.generateMode}
                  hasCurrentData={ai.hasCurrentData}
                  hasGeneratedConfig={ai.hasGeneratedConfig}
                  onPreview={ai.openPreview}
                  cmdbMode={ai.cmdbMode}
                  onCmdbModeChange={ai.setCmdbMode}
                  progress={ai.progress}
                />
              )}
              
              {formRenderError && viewMode === 'json' && (
                <div style={{
                  padding: '12px 16px',
                  background: '#fff3cd',
                  border: '1px solid #ffc107',
                  borderRadius: '6px',
                  color: '#856404',
                  marginBottom: '16px',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center'
                }}>
                  <span>⚠️ 表单渲染失败，已自动切换到JSON视图。编辑完成后可点击"表单视图"按钮重新尝试。</span>
                </div>
              )}
              
              {/* 根据 viewMode 和 schema 版本选择渲染器 */}
              {viewMode === 'form' && !formRenderError ? (
                <ErrorBoundary
                  onError={() => {
                    setFormRenderError(true);
                    setViewMode('json');
                    showToast('表单渲染失败，已切换到JSON视图', 'warning');
                  }}
                >
                  {rawSchema?.schema_version === 'v2' && rawSchema?.openapi_schema ? (
                    <OpenAPIFormRenderer
                      schema={rawSchema.openapi_schema}
                      initialValues={formData}
                      onChange={setFormData}
                      workspaceResource={workspaceResourceContext || undefined}
                    />
                  ) : (
                    <DynamicForm
                      schema={(schema as any).schema_data || schema}
                      values={formData}
                      onChange={setFormData}
                      initialFieldsToShow={initialFieldsToShow}
                    />
                  )}
                </ErrorBoundary>
              ) : (
                <JsonEditor
                  value={JSON.stringify(formData, null, 2)}
                  onChange={(value) => {
                    try {
                      const parsed = JSON.parse(value);
                      setFormData(parsed);
                    } catch (e) {
                      // JSON格式错误时不更新formData
                      console.error('Invalid JSON:', e);
                    }
                  }}
                  minHeight={300}
                  maxHeight={600}
                />
              )}
            </div>
          )}
          
          {/* 变更摘要 */}
          <div className={styles.changeSummarySection}>
            <label className={styles.changeSummaryLabel}>
              变更摘要 <span className={styles.required}>*</span>
            </label>
            <input
              ref={changeSummaryRef}
              type="text"
              placeholder="描述本次修改的内容，例如：更新bucket配置、启用版本控制等"
              value={changeSummary}
              onChange={(e) => handleChangeSummaryChange(e.target.value)}
              className={`${styles.changeSummaryInput} ${changeSummaryError ? styles.inputError : ''}`}
            />
            {changeSummaryError && (
              <div className={styles.errorMessage}>{changeSummaryError}</div>
            )}
            <div className={styles.changeSummaryHint}>
              变更摘要将记录在版本历史中，帮助团队了解每次修改的目的
            </div>
          </div>
        </div>
      </div>

      <div className={styles.footer}>
        <div className={styles.footerLeft}>
          {/* 可以添加预览按钮 */}
        </div>
        
        <div className={styles.footerRight}>
          <button onClick={handleCancel} className={styles.btnCancel}>
            取消
          </button>
          
          {isCloneMode ? (
            <SplitButton
              mainLabel="克隆资源"
              mainOnClick={() => handleSubmit(false)}
              menuItems={[
                {
                  label: '克隆并运行该任务',
                  onClick: () => handleSubmit(true)
                }
              ]}
              disabled={submitting}
            />
          ) : (
            <SplitButton
              mainLabel="保存修改"
              mainOnClick={() => handleSubmit(false)}
              menuItems={[
                {
                  label: '保存并运行该任务',
                  onClick: () => handleSubmit(true)
                }
              ]}
              disabled={submitting}
            />
          )}
        </div>
      </div>
      
      {/* 资源运行对话框 */}
      {savedResourceName && resource && (
        <ResourceRunDialog
          isOpen={showRunDialog}
          workspaceId={id!}
          resourceName={savedResourceName}
          resourceType={resource.resource_type}
          onClose={() => {
            setShowRunDialog(false);
            setSavedResourceName('');
            // 关闭对话框时返回资源列表
            navigate(`/workspaces/${id}?tab=resources`);
          }}
          onSuccess={() => {
            setShowRunDialog(false);
            setSavedResourceName('');
            // 运行成功会自动跳转到任务详情页，这里不需要额外处理
          }}
        />
      )}

      {/* AI 预览弹窗 - 使用 mergedConfig 显示合并后的完整数据 */}
      <AIPreviewModal
        open={ai.previewOpen}
        onClose={() => ai.setPreviewOpen(false)}
        onApply={ai.handleApplyConfig}
        onRecheck={() => ai.handleGenerate('refine')}
        generatedConfig={ai.mergedConfig || ai.generatedConfig}
        placeholders={ai.placeholders}
        emptyFields={ai.emptyFields}
        renderConfigValue={ai.renderConfigValue}
        mode={ai.generateMode}
        loading={ai.loading}
        blockMessage={ai.blockMessage}
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

export default EditResource;
