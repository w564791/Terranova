import React, { useState, useEffect, Component, type ReactNode } from 'react';
import { useParams, useNavigate, useSearchParams, Link } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { processApiSchema } from '../utils/schemaTypeMapper';
import api from '../services/api';
import DynamicForm, { FormPreview } from '../components/DynamicForm';
import { FormRenderer as OpenAPIFormRenderer } from '../components/OpenAPIFormRenderer';
import { 
  AITriggerButton, 
  AIInputPanel, 
  AIPreviewModal, 
  useAIConfigGenerator 
} from '../components/OpenAPIFormRenderer/AIFormAssistant';
import { JsonEditor } from '../components/DynamicForm/JsonEditor';
import DemoSelector from '../components/DemoSelector';
import ConfirmDialog from '../components/ConfirmDialog';
import TopBar from '../components/TopBar';
import { listVersions, type ModuleVersion } from '../services/moduleVersions';
import { moduleService, type AIPrompt } from '../services/modules';
import styles from './AddResources.module.css';

interface Module {
  id: number;
  name: string;
  description: string;
  provider: string;
  source: string;
  module_source?: string;
  source_type: string;
  version?: string;  // Terraform Registry module version
  ai_prompts?: AIPrompt[];  // AI 助手提示词
}

interface Schema {
  id: number;
  module_id: number;
  version: string;
  status: string;
  ai_generated: boolean;
  source_type: string;
  schema_data: Record<string, any>; // This is the actual schema object for v1
  schema_version?: string; // 'v1' or 'v2'
  openapi_schema?: any; // OpenAPI v3 schema for v2
  ui_config?: any;
  created_at: string;
  updated_at: string;
}

interface ResourceConfig {
  module_id: number;
  module_name: string;
  resource_name: string;
  resource_type: string;
  config: any;
  selected_version?: string;  // 用户选中的版本号
}

type Step = 'select' | 'configure' | 'preview';
type RunType = 'none' | 'plan' | 'plan_and_apply';

const AddResources: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { showToast } = useToast();
  
  // 从 URL 参数获取 module 和版本（特殊入口，用于指定非默认版本）
  const urlModuleParam = searchParams.get('module');
  const urlVersionParam = searchParams.get('version');
  
  const [step, setStep] = useState<Step>('select');
  const [modules, setModules] = useState<Module[]>([]);
  const [selectedModules, setSelectedModules] = useState<number[]>([]);
  const [currentModuleIndex, setCurrentModuleIndex] = useState(0);
  const [resourceConfigs, setResourceConfigs] = useState<ResourceConfig[]>([]);
  const [currentSchema, setCurrentSchema] = useState<Schema | null>(null);
  const [formData, setFormData] = useState<any>({});
  const [resourceName, setResourceName] = useState('');
  const [loading, setLoading] = useState(false);
  const [runType, setRunType] = useState<RunType>('plan');
  const [existingResources, setExistingResources] = useState<string[]>([]);
  const [nameError, setNameError] = useState('');
  const [viewMode, setViewMode] = useState<'form' | 'json'>('form');
  const [showDemoConfirmDialog, setShowDemoConfirmDialog] = useState(false);
  const [pendingDemoData, setPendingDemoData] = useState<any>(null);
  const [pendingDemoName, setPendingDemoName] = useState<string>('');
  const [initialFieldsToShow, setInitialFieldsToShow] = useState<string[]>([]);
  const [formRenderError, setFormRenderError] = useState(false);
  const [configViewMode, setConfigViewMode] = useState<'form' | 'json'>('form');
  const [previewRenderError, setPreviewRenderError] = useState(false);
  
  // Module 版本相关状态
  const [moduleVersions, setModuleVersions] = useState<ModuleVersion[]>([]);
  const [selectedVersionId, setSelectedVersionId] = useState<string>('');
  const [loadingVersions, setLoadingVersions] = useState(false);

  // 获取当前 Module
  const currentModule = modules.find(m => m.id === selectedModules[currentModuleIndex]);

  // 从 Schema 中提取默认值
  const extractSchemaDefaults = React.useCallback((schema: any): Record<string, unknown> => {
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
  // 策略：用户数据优先，但对于嵌套对象需要深度合并
  const deepMergeForDisplay = (defaults: Record<string, unknown>, userData: Record<string, unknown>): Record<string, unknown> => {
    const result = { ...defaults };
    
    Object.keys(userData).forEach(key => {
      const userValue = userData[key];
      const defaultValue = result[key];
      
      // 如果两个值都是对象（非数组），则深度合并
      if (
        userValue && typeof userValue === 'object' && !Array.isArray(userValue) &&
        defaultValue && typeof defaultValue === 'object' && !Array.isArray(defaultValue)
      ) {
        result[key] = deepMergeForDisplay(defaultValue as Record<string, unknown>, userValue as Record<string, unknown>);
      } else {
        // 否则用户数据覆盖默认值
        result[key] = userValue;
      }
    });
    
    return result;
  };

  // 从 Schema 中提取默认值并与 formData 深度合并
  // 这样 AI 助手可以看到完整的表单数据（包括默认值和用户新增的字段）
  const mergedFormData = React.useMemo(() => {
    const defaults = extractSchemaDefaults(currentSchema);
    // 使用深度合并，确保嵌套对象（如 tags）中用户新增的字段也能被包含
    return deepMergeForDisplay(defaults, formData);
  }, [currentSchema, formData, extractSchemaDefaults]);

  // 过滤掉 Schema 默认值和空值，只保留用户实际修改的数据和必填字段
  // 用于 JSON 视图显示和提交时使用
  const filterSchemaDefaultsAndEmpty = React.useCallback((
    data: Record<string, unknown>, 
    schemaDefaults: Record<string, unknown>,
    requiredFieldsList: string[] = []
  ): Record<string, unknown> => {
    const result: Record<string, unknown> = {};
    
    // 首先添加所有必填字段（即使值为空）
    requiredFieldsList.forEach(key => {
      const value = data[key];
      // 必填字段始终保留，即使值为 undefined/null/空字符串
      result[key] = value !== undefined ? value : '';
    });
    
    Object.keys(data).forEach(key => {
      const value = data[key];
      const defaultValue = schemaDefaults[key];
      const isRequired = requiredFieldsList.includes(key);
      
      // 必填字段已经在上面处理过了
      if (isRequired) return;
      
      // 跳过 null 和 undefined
      if (value === null || value === undefined) return;
      
      // 跳过空字符串
      if (value === '') return;
      
      // 跳过空数组
      if (Array.isArray(value) && value.length === 0) return;
      
      // 处理对象（非数组）
      if (typeof value === 'object' && !Array.isArray(value)) {
        // 递归过滤嵌套对象
        const nestedDefault = (defaultValue && typeof defaultValue === 'object' && !Array.isArray(defaultValue)) 
          ? defaultValue as Record<string, unknown>
          : {};
        const filtered = filterSchemaDefaultsAndEmpty(value as Record<string, unknown>, nestedDefault, []);
        // 跳过空对象
        if (Object.keys(filtered).length > 0) {
          result[key] = filtered;
        }
        return;
      }
      
      // 跳过与默认值完全相同的值
      if (defaultValue !== undefined && JSON.stringify(value) === JSON.stringify(defaultValue)) {
        return;
      }
      
      // 保留用户修改的值
      result[key] = value;
    });
    
    return result;
  }, []);

  // 智能合并函数：AI 数据优先，用户数据作为补充
  // 策略：
  // 1. AI 明确提供的值应该覆盖默认值（这是用户期望的行为）
  // 2. 用户手动添加的字段（不在 AI 数据中）应该保留
  // 3. 对于嵌套对象（如 tags），递归应用相同策略
  // 4. 过滤掉 AI 生成的空字符串值（AI 不应该生成空字符串）
  const smartMerge = (userData: Record<string, unknown>, aiData: Record<string, unknown>): Record<string, unknown> => {
    // 以用户数据为基础（保留用户手动添加的字段）
    const result = { ...userData };
    
    // 遍历 AI 生成的数据，AI 的值优先
    Object.keys(aiData).forEach(key => {
      const aiValue = aiData[key];
      const userValue = result[key];
      
      // 过滤掉 AI 生成的空字符串值（AI 不应该生成空字符串）
      if (aiValue === '') {
        return;
      }
      
      // 如果 AI 的值是对象，需要特殊处理
      if (aiValue && typeof aiValue === 'object' && !Array.isArray(aiValue)) {
        // 过滤掉对象中的空字符串
        const filteredAiValue = filterEmptyStrings(aiValue as Record<string, unknown>);
        
        // 如果过滤后的对象为空，跳过
        if (Object.keys(filteredAiValue).length === 0) {
          return;
        }
        
        // 如果用户数据中也有这个字段且是对象，递归合并
        if (userValue && typeof userValue === 'object' && !Array.isArray(userValue)) {
          result[key] = smartMerge(userValue as Record<string, unknown>, filteredAiValue);
        } else {
          // 否则直接使用 AI 的值
          result[key] = filteredAiValue;
        }
        return;
      }
      
      // 对于非对象值，AI 的值直接覆盖用户数据
      // 这是关键修改：AI 明确提供的值应该覆盖默认值
      result[key] = aiValue;
    });
    
    return result;
  };

  // 过滤掉对象中的空字符串值
  const filterEmptyStrings = (obj: Record<string, unknown>): Record<string, unknown> => {
    const result: Record<string, unknown> = {};
    
    Object.keys(obj).forEach(key => {
      const value = obj[key];
      
      // 跳过空字符串
      if (value === '') {
        return;
      }
      
      // 递归处理嵌套对象
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
  };

  // 从 Schema 中提取必填字段列表
  const extractRequiredFields = React.useCallback((schema: any): string[] => {
    if (!schema?.openapi_schema) return [];
    return schema.openapi_schema?.components?.schemas?.ModuleInput?.required || [];
  }, []);

  // 获取当前 Schema 的必填字段列表
  const requiredFields = React.useMemo(() => {
    return extractRequiredFields(currentSchema);
  }, [currentSchema, extractRequiredFields]);

  // 用于 JSON 视图显示和提交的数据（过滤掉默认值和空值，但保留必填字段）
  const filteredFormDataForSubmit = React.useMemo(() => {
    const defaults = extractSchemaDefaults(currentSchema);
    return filterSchemaDefaultsAndEmpty(formData, defaults, requiredFields);
  }, [currentSchema, formData, extractSchemaDefaults, filterSchemaDefaultsAndEmpty, requiredFields]);

  // 过滤掉对象中的所有空值（空字符串、空数组、空对象、null、undefined）
  // 用于 JSON 视图显示和提交时过滤无意义的空值
  // 但必填字段（requiredFields）即使为空也要保留
  const filterEmptyValues = (obj: Record<string, unknown>, requiredKeys: string[] = []): Record<string, unknown> => {
    const result: Record<string, unknown> = {};
    
    Object.keys(obj).forEach(key => {
      const value = obj[key];
      const isRequired = requiredKeys.includes(key);
      
      // 必填字段始终保留（即使为空）
      if (isRequired) {
        result[key] = value;
        return;
      }
      
      // 跳过 null 和 undefined
      if (value === null || value === undefined) {
        return;
      }
      
      // 跳过空字符串
      if (value === '') {
        return;
      }
      
      // 跳过空数组
      if (Array.isArray(value) && value.length === 0) {
        return;
      }
      
      // 处理对象（非数组）
      if (typeof value === 'object' && !Array.isArray(value)) {
        const filtered = filterEmptyValues(value as Record<string, unknown>, []);
        // 跳过空对象
        if (Object.keys(filtered).length > 0) {
          result[key] = filtered;
        }
        return;
      }
      
      // 保留其他值（包括 false、0 等有效值）
      result[key] = value;
    });
    
    return result;
  };

  // 用于 JSON 视图显示的数据（过滤掉空值，但保留必填字段）
  const filteredFormDataForDisplay = React.useMemo(() => {
    return filterEmptyValues(mergedFormData, requiredFields);
  }, [mergedFormData, requiredFields]);

  // AI 助手 Hook - 使用合并后的数据，应用时使用智能合并
  const ai = useAIConfigGenerator({
    moduleId: currentModule?.id || 0,
    workspaceId: id,
    currentFormData: mergedFormData,
    onGenerate: (config: Record<string, unknown>) => {
      // 使用智能合并：以 mergedFormData 为基础（包含用户新增的字段），用 AI 数据补充空值
      // 注意：这里不能使用 prev，因为 prev 可能不包含用户在表单中新增的字段
      const merged = smartMerge(mergedFormData, config);
      setFormData(merged);
    },
  });

  useEffect(() => {
    loadModules();
    loadExistingResources();
  }, []);

  const loadExistingResources = async () => {
    try {
      const response: any = await api.get(`/workspaces/${id}/resources`);
      // 处理不同的响应格式
      let resources: any[] = [];
      if (response.data?.resources) {
        resources = response.data.resources;
      } else if (response.resources) {
        resources = response.resources;
      } else if (Array.isArray(response.data)) {
        resources = response.data;
      } else if (Array.isArray(response)) {
        resources = response;
      }
      // 保存完整的resource_id列表（格式：resource_type.resource_name）
      const resourceIds = resources.map((r: any) => r.resource_id);
      setExistingResources(resourceIds);
    } catch (error: any) {
      console.error('Failed to load existing resources:', error);
      // 不显示错误提示，因为这不是关键功能
    }
  };

  const loadModules = async () => {
    try {
      const response = await api.get('/modules');
      console.log('📦 Modules API Response:', response.data);
      
      // 处理不同的响应格式
      let modulesData: Module[] = [];
      if (response.data.items) {
        modulesData = response.data.items;
      } else if (Array.isArray(response.data)) {
        modulesData = response.data;
      } else if (response.data.data && Array.isArray(response.data.data)) {
        modulesData = response.data.data;
      }
      
      console.log('📦 Loaded modules:', modulesData.map(m => ({ id: m.id, name: m.name, version: m.version })));
      setModules(modulesData);
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  const handleModuleSelect = (moduleId: number) => {
    setSelectedModules(prev => {
      if (prev.includes(moduleId)) {
        return prev.filter(id => id !== moduleId);
      } else {
        return [...prev, moduleId];
      }
    });
  };

  const handleNext = async () => {
    if (step === 'select') {
      if (selectedModules.length === 0) {
        showToast('请至少选择一个Module', 'warning');
        return;
      }
      
      // 加载第一个Module的Schema
      await loadModuleSchema(selectedModules[0]);
      setStep('configure');
      setCurrentModuleIndex(0);
    } else if (step === 'configure') {
      // 保存当前配置（如果失败，保留用户输入）
      const saved = saveCurrentConfig();
      if (!saved) {
        return; // 保留用户输入，不进入下一步
      }
      
      // 检查是否还有更多Module需要配置
      if (currentModuleIndex < selectedModules.length - 1) {
        // 加载下一个Module
        setCurrentModuleIndex(currentModuleIndex + 1);
        await loadModuleSchema(selectedModules[currentModuleIndex + 1]);
        setFormData({});
        setResourceName('');
        setNameError('');
        setInitialFieldsToShow([]);
        setFormRenderError(false);
        setConfigViewMode('form');
      } else {
        // 所有Module配置完成，进入预览
        setStep('preview');
      }
    }
  };

  const loadModuleSchema = async (moduleId: number) => {
    try {
      setLoading(true);
      
      // 加载 Module 版本列表
      setLoadingVersions(true);
      try {
        const versionsRes = await listVersions(moduleId);
        const versionItems = versionsRes.items || [];
        setModuleVersions(versionItems);
        
        // 检查 URL 参数是否指定了版本（特殊入口）
        // 只有当 URL 中的 module 参数与当前 module 匹配时，才使用 URL 中的 version 参数
        let versionToSelect: ModuleVersion | undefined;
        
        const urlModuleMatches = urlModuleParam && String(moduleId) === urlModuleParam;
        
        if (urlVersionParam && urlModuleMatches) {
          // 通过版本号查找（URL 参数是版本号字符串，如 "1.0.0"）
          versionToSelect = versionItems.find((v: ModuleVersion) => v.version === urlVersionParam);
          if (versionToSelect) {
            console.log(`📦 URL version param found: ${urlVersionParam} for module ${moduleId} (ID: ${versionToSelect.id})`);
          } else {
            console.warn(`📦 URL version param "${urlVersionParam}" not found for module ${moduleId}, falling back to default`);
          }
        } else if (urlVersionParam && !urlModuleMatches) {
          console.log(`📦 URL version param "${urlVersionParam}" ignored (module mismatch: URL=${urlModuleParam}, current=${moduleId})`);
        }
        
        // 如果 URL 没有指定版本或版本不存在，使用默认版本
        if (!versionToSelect) {
          versionToSelect = versionItems.find((v: ModuleVersion) => v.is_default);
          if (!versionToSelect && versionItems.length > 0) {
            versionToSelect = versionItems[0];
          }
        }
        
        if (versionToSelect) {
          setSelectedVersionId(versionToSelect.id);
          console.log(`📦 Version selected: ${versionToSelect.version} (ID: ${versionToSelect.id})`);
          
          // 更新 URL 参数，显示当前使用的 module 和版本
          const newParams = new URLSearchParams(searchParams);
          newParams.set('module', String(moduleId));
          newParams.set('version', versionToSelect.version);
          setSearchParams(newParams, { replace: true });
        } else {
          setSelectedVersionId('');
          console.log('📦 No versions available');
          
          // 即使没有版本，也更新 module 参数
          const newParams = new URLSearchParams(searchParams);
          newParams.set('module', String(moduleId));
          newParams.delete('version');
          setSearchParams(newParams, { replace: true });
        }
      } catch (versionError) {
        console.warn('加载版本列表失败:', versionError);
        setModuleVersions([]);
        setSelectedVersionId('');
      } finally {
        setLoadingVersions(false);
      }
      
      // 加载模块的 AI 提示词
      try {
        const promptsRes = await moduleService.getModulePrompts(moduleId);
        const promptsData = promptsRes.data?.items || [];
        console.log('💡 Module prompts loaded:', promptsData);
        
        // 更新 modules 状态中对应模块的 ai_prompts
        setModules(prev => prev.map(m => 
          m.id === moduleId ? { ...m, ai_prompts: promptsData } : m
        ));
      } catch (promptsError) {
        console.warn('加载模块提示词失败:', promptsError);
        // 不影响主流程
      }
      
      const response = await api.get(`/modules/${moduleId}/schemas`);
      console.log('📊 Schema API Response:', response.data);
      
      // 处理不同的响应格式
      let schemasData = [];
      if (response.data.data) {
        schemasData = Array.isArray(response.data.data) ? response.data.data : [response.data.data];
      } else if (Array.isArray(response.data)) {
        schemasData = response.data;
      }
      
      console.log('📊 Schemas Data:', schemasData);
      
      if (schemasData.length > 0) {
        // 选择第一个active状态的schema或第一个schema
        let activeSchema = schemasData.find((s: any) => s.status === 'active') || schemasData[0];
        
        console.log('📊 Active Schema:', activeSchema);
        console.log('📊 Schema Version:', activeSchema.schema_version);
        console.log('📊 Has OpenAPI Schema:', !!activeSchema.openapi_schema);
        
        // 检查是否是 V2 Schema (OpenAPI 格式)
        if (activeSchema.schema_version === 'v2' && activeSchema.openapi_schema) {
          console.log('📊 Using V2 OpenAPI Schema');
          setCurrentSchema(activeSchema);
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
          
          setCurrentSchema(processedSchema);
        }
      } else {
        showToast('该Module暂无Schema定义', 'warning');
      }
    } catch (error: any) {
      console.error('加载Schema失败:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setLoading(false);
    }
  };

  // 判断是否是 V2 Schema
  const isV2Schema = (schema: Schema | null): boolean => {
    return schema?.schema_version === 'v2' && !!schema?.openapi_schema;
  };

  const checkResourceNameExists = (resourceType: string, name: string): boolean => {
    // 构建完整的resource_id（格式：resource_type.resource_name）
    const resourceId = `${resourceType}.${name}`;
    
    // 检查是否与现有资源重名（比较完整的resource_id）
    if (existingResources.includes(resourceId)) {
      return true;
    }
    // 检查是否与本次添加的资源重名
    if (resourceConfigs.some(c => c.resource_name === name && c.resource_type === resourceType)) {
      return true;
    }
    return false;
  };

  const saveCurrentConfig = () => {
    // 优先使用独立的资源名称输入框的值，回退到表单中的name字段
    const nameToUse = resourceName.trim() || (formData.name && formData.name.trim());
    
    if (!nameToUse) {
      setNameError('请输入资源名称');
      showToast('请输入资源名称', 'warning');
      return false;
    }

    const module = modules.find(m => m.id === selectedModules[currentModuleIndex]);
    if (!module) return false;

    const resourceType = `${module.provider}_${module.name}`;

    // 检查名称是否已存在（传入resource_type和name）
    if (checkResourceNameExists(resourceType, nameToUse)) {
      setNameError(`资源 "${resourceType}.${nameToUse}" 已存在，请使用其他名称`);
      showToast(`资源 "${resourceType}.${nameToUse}" 已存在`, 'error');
      return false;
    }

    // 获取选中版本的版本号
    let selectedVersionStr = '';
    if (selectedVersionId && moduleVersions.length > 0) {
      const selectedVersion = moduleVersions.find(v => v.id === selectedVersionId);
      if (selectedVersion?.version) {
        selectedVersionStr = selectedVersion.version;
        console.log(`📦 Saving config with version: ${selectedVersionStr}`);
      }
    }
    
    // 使用过滤后的数据（过滤掉 Schema 默认值和空值）
    // 这样提交时不会带上无意义的空值和默认值
    const config: ResourceConfig = {
      module_id: module.id,
      module_name: module.name,
      resource_name: nameToUse,
      resource_type: resourceType,
      config: filteredFormDataForSubmit,  // 使用过滤后的数据（不包含默认值和空值）
      selected_version: selectedVersionStr  // 保存选中的版本号
    };

    setResourceConfigs(prev => [...prev, config]);
    setNameError('');
    return true;
  };

  const handleSubmit = async () => {
    try {
      setLoading(true);
      
      // 0. 如果需要执行任务，先设置 TF_CLI_ARGS 变量
      if (runType !== 'none' && resourceConfigs.length > 0) {
        // 构建所有新添加资源的 target 列表
        const targetArgs = resourceConfigs.map(config => {
          // module 名称格式：{resource_type}_{resource_name}
          // resource_type 格式：{provider}_{module_name}
          // 最终格式：{provider}_{module_name}_{resource_name}
          const moduleName = `${config.resource_type}_${config.resource_name}`;
          return `--target=module.${moduleName}`;
        }).join(' ');
        
        console.log('🎯 设置 TF_CLI_ARGS:', targetArgs);
        
        try {
          // 尝试获取现有变量
          const variablesResponse: any = await api.get(`/workspaces/${id}/variables`);
          const variables = variablesResponse.data?.data || variablesResponse.data || [];
          
          const existingVar = variables.find((v: any) => v.key === 'TF_CLI_ARGS');
          
          if (existingVar) {
            // 更新现有变量
            await api.put(`/workspaces/${id}/variables/${existingVar.id}`, {
              version: existingVar.version,
              key: 'TF_CLI_ARGS',
              value: targetArgs,
              category: 'env',
              variable_type: 'environment',
              sensitive: false,
              description: 'Auto-generated for resource-specific run'
            });
            console.log('✅ TF_CLI_ARGS 变量已更新');
          } else {
            // 创建新变量
            try {
              await api.post(`/workspaces/${id}/variables`, {
                key: 'TF_CLI_ARGS',
                value: targetArgs,
                category: 'env',
                variable_type: 'environment',
                sensitive: false,
                description: 'Auto-generated for resource-specific run'
              });
              console.log('✅ TF_CLI_ARGS 变量已创建');
            } catch (createError: any) {
              // 如果创建失败是因为变量已存在，尝试重新获取并更新
              const errorMessage = createError?.response?.data?.message || createError?.message || '';
              if (errorMessage.includes('已存在') || errorMessage.includes('exist')) {
                const retryResponse: any = await api.get(`/workspaces/${id}/variables`);
                const retryVariables = retryResponse.data?.data || retryResponse.data || [];
                const retryExistingVar = retryVariables.find((v: any) => v.key === 'TF_CLI_ARGS');
                
                if (retryExistingVar) {
                  await api.put(`/workspaces/${id}/variables/${retryExistingVar.id}`, {
                    version: retryExistingVar.version,
                    key: 'TF_CLI_ARGS',
                    value: targetArgs,
                    category: 'env',
                    variable_type: 'environment',
                    sensitive: false,
                    description: 'Auto-generated for resource-specific run'
                  });
                  console.log('✅ TF_CLI_ARGS 变量已更新（重试成功）');
                }
              } else {
                throw createError;
              }
            }
          }
        } catch (varError) {
          console.error('设置 TF_CLI_ARGS 变量失败:', varError);
          showToast('设置运行参数失败', 'error');
          return;
        }
      }
      
      // 1. 批量创建资源
      for (const config of resourceConfigs) {
        // 获取module信息以获取source（使用Module表中的source字段）
        const module = modules.find(m => m.id === config.module_id);
        if (!module) {
          showToast(`找不到Module ID ${config.module_id}`, 'error');
          continue;
        }
        
        // 构建 module 配置
        const moduleConfig: Record<string, any> = {
          source: module.module_source || module.source,  // 优先使用module_source，回退到source
          ...config.config
        };
        
        // 使用配置步骤中用户选中的版本
        if (config.selected_version) {
          moduleConfig.version = config.selected_version;
          console.log(`📦 Adding version ${config.selected_version} to module ${module.name} (from user selection)`);
        } else if (module.version) {
          // 回退到 Module 表的 version 字段
          moduleConfig.version = module.version;
          console.log(`📦 Adding version ${module.version} to module ${module.name} (from Module table)`);
        } else {
          console.log(`⚠️ Module ${module.name} has no version configured`);
        }
        
        const tfCode = {
          module: {
            [`${config.resource_type}_${config.resource_name}`]: [moduleConfig]
          }
        };
        console.log('📦 Generated tf_code:', JSON.stringify(tfCode, null, 2));
        
        await api.post(`/workspaces/${id}/resources`, {
          resource_type: config.resource_type,
          resource_name: config.resource_name,
          tf_code: tfCode,
          description: `从Module ${config.module_name} 创建`
        });
      }
      
      showToast(`成功添加 ${resourceConfigs.length} 个资源`, 'success');
      
      // 2. 根据runType创建任务
      if (runType === 'none') {
        // 仅添加资源，不创建任务
        showToast('资源已添加，未创建任务', 'info');
        // 跳转到resources标签页
        navigate(`/workspaces/${id}?tab=resources`);
      } else if (runType === 'plan') {
        // 创建Plan任务
        const response: any = await api.post(`/workspaces/${id}/tasks/plan`, {
          description: `添加 ${resourceConfigs.length} 个资源后执行Plan`,
          run_type: 'plan'
        });
        const taskId = response.data?.task?.id || response.task?.id;
        showToast('Plan任务创建成功', 'success');
        // 跳转到任务详情页或runs标签页
        if (taskId) {
          navigate(`/workspaces/${id}/tasks/${taskId}`);
        } else {
          navigate(`/workspaces/${id}?tab=runs`);
        }
      } else if (runType === 'plan_and_apply') {
        // 创建Plan+Apply任务（使用run_type参数）
        const response: any = await api.post(`/workspaces/${id}/tasks/plan`, {
          description: `添加 ${resourceConfigs.length} 个资源后执行Plan+Apply`,
          run_type: 'plan_and_apply'
        });
        const taskId = response.data?.task?.id || response.task?.id;
        showToast('Plan+Apply任务创建成功', 'success');
        // 跳转到任务详情页或runs标签页
        if (taskId) {
          navigate(`/workspaces/${id}/tasks/${taskId}`);
        } else {
          navigate(`/workspaces/${id}?tab=runs`);
        }
      }
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleBack = () => {
    if (step === 'configure' && currentModuleIndex > 0) {
      // 从当前Module返回到上一个Module
      setCurrentModuleIndex(currentModuleIndex - 1);
      loadModuleSchema(selectedModules[currentModuleIndex - 1]);
      // 清空表单数据
      setFormData({});
      setNameError('');
    } else if (step === 'configure') {
      // 从第一个Module返回到选择页面
      setStep('select');
      setFormData({});
      setNameError('');
      setInitialFieldsToShow([]);
    } else if (step === 'preview') {
      // 从预览返回到配置页面
      // 移除最后一个已保存的配置
      const lastConfig = resourceConfigs[resourceConfigs.length - 1];
      setResourceConfigs(prev => prev.slice(0, -1));
      
      // 恢复表单数据
      if (lastConfig) {
        setFormData(lastConfig.config);
      }
      
      // 返回到最后一个Module的配置页面
      setStep('configure');
      setCurrentModuleIndex(selectedModules.length - 1);
      loadModuleSchema(selectedModules[selectedModules.length - 1]);
      setNameError('');
    }
  };

  const handleSelectDemo = (demoData: any, demoName: string) => {
    // 检查是否有表单数据
    const hasData = Object.keys(formData).length > 0;
    
    if (hasData) {
      // 显示确认对话框
      setPendingDemoData(demoData);
      setPendingDemoName(demoName);
      setShowDemoConfirmDialog(true);
    } else {
      // 直接应用Demo数据
      applyDemoData(demoData, demoName);
    }
  };

  const applyDemoData = (demoData: any, demoName: string) => {
    setFormData(demoData);
    setFormRenderError(false);
    
    // 获取Demo数据中所有有值的字段名
    const fieldsWithValues = Object.keys(demoData).filter(key => {
      const value = demoData[key];
      // 排除空值、空数组、空对象
      if (value === null || value === undefined || value === '') return false;
      if (Array.isArray(value) && value.length === 0) return false;
      if (typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0) return false;
      return true;
    });
    
    console.log('📋 Fields with values from demo:', fieldsWithValues);
    setInitialFieldsToShow(fieldsWithValues);
    
    showToast(`已应用Demo "${demoName}" 的配置`, 'success');
  };

  const confirmApplyDemo = () => {
    if (pendingDemoData) {
      applyDemoData(pendingDemoData, pendingDemoName);
    }
    setShowDemoConfirmDialog(false);
    setPendingDemoData(null);
    setPendingDemoName('');
  };

  const handleCancel = () => {
    navigate(`/workspaces/${id}`);
  };

  const renderStepContent = () => {
    switch (step) {
      case 'select':
        return (
          <div className={styles.selectStep}>
            <h2 className={styles.stepTitle}>选择Module</h2>
            <p className={styles.stepDesc}>从Module库中选择要添加的资源</p>
            
            <div className={styles.searchBar}>
              <input
                type="text"
                placeholder="搜索Module..."
                className={styles.searchInput}
              />
            </div>

            <div className={styles.moduleList}>
              {modules.map(module => (
                <label
                  key={module.id}
                  className={`${styles.moduleCard} ${
                    selectedModules.includes(module.id) ? styles.moduleCardSelected : ''
                  }`}
                >
                  <input
                    type="checkbox"
                    checked={selectedModules.includes(module.id)}
                    onChange={() => handleModuleSelect(module.id)}
                  />
                  <div className={styles.moduleInfo}>
                    <div className={styles.moduleName}>{module.name}</div>
                    <div className={styles.moduleDesc}>{module.description}</div>
                    <div className={styles.moduleProvider}>Provider: {module.provider}</div>
                  </div>
                </label>
              ))}
            </div>
          </div>
        );

      case 'configure':
        const currentModule = modules.find(m => m.id === selectedModules[currentModuleIndex]);
        
        return (
          <div className={styles.configureStep}>
            <h2 className={styles.stepTitle}>
              配置: {currentModule?.name} ({currentModuleIndex + 1}/{selectedModules.length})
            </h2>
            
            {currentSchema && (currentSchema.schema_data || isV2Schema(currentSchema)) ? (
              <div className={styles.dynamicFormContainer}>
                {/* 资源名称输入框 - 放在表单容器内以保持对齐 */}
                <div className={styles.resourceNameSection}>
                  <label className={styles.label}>
                    资源名称 <span style={{ color: 'var(--color-red-500)' }}>*</span>
                  </label>
                  <input
                    type="text"
                    className={`${styles.input} ${nameError ? styles.inputError : ''}`}
                    value={resourceName}
                    onChange={(e) => {
                      setResourceName(e.target.value);
                      setNameError('');
                    }}
                    placeholder="请输入资源名称，例如：my-bucket"
                  />
                  {nameError && (
                    <div className={styles.errorMessage}>{nameError}</div>
                  )}
                  <div className={styles.hint}>
                    资源名称将用于标识此资源，格式为：{currentModule?.provider}_{currentModule?.name}.{resourceName || '<资源名称>'}
                  </div>
                </div>
                
                {/* Module 版本信息（只读显示） */}
                {moduleVersions.length > 0 && selectedVersionId && (
                  <div className={styles.resourceNameSection} style={{ marginTop: '16px' }}>
                    <label className={styles.label}>
                      TF Module 版本
                    </label>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                      {loadingVersions ? (
                        <span style={{ fontSize: '13px', color: '#94a3b8' }}>加载中...</span>
                      ) : (
                        <>
                          <span style={{
                            padding: '8px 12px',
                            border: '1px solid #e2e8f0',
                            borderRadius: '6px',
                            fontSize: '14px',
                            color: '#334155',
                            background: '#f8fafc',
                            minWidth: '120px',
                            display: 'inline-block'
                          }}>
                            {moduleVersions.find(v => v.id === selectedVersionId)?.version || '-'}
                            {moduleVersions.find(v => v.id === selectedVersionId)?.is_default && (
                              <span style={{ marginLeft: '8px', color: '#64748b', fontSize: '12px' }}>(默认)</span>
                            )}
                          </span>
                          <span style={{ 
                            fontSize: '13px', 
                            color: '#10b981',
                            display: 'flex',
                            alignItems: 'center',
                            gap: '4px'
                          }}>
                            ✓ 此版本将被写入资源配置
                          </span>
                        </>
                      )}
                    </div>
                    <div className={styles.hint}>
                      默认使用 Module 的默认版本。如需使用其他版本，请在 URL 中添加 <code style={{ background: '#f1f5f9', padding: '2px 6px', borderRadius: '4px' }}>?version=x.x.x</code> 参数
                    </div>
                  </div>
                )}
                
                <div className={styles.formDescription} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '12px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                    <span style={{ fontSize: '14px', color: '#333', fontWeight: 500 }}>
                      基于Module Schema自动生成的配置表单
                    </span>
                    {isV2Schema(currentSchema) && (
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
                    {isV2Schema(currentSchema) && currentModule && (
                      <AITriggerButton
                        expanded={ai.expanded}
                        onClick={() => ai.setExpanded(!ai.expanded)}
                      />
                    )}
                  </div>
                  
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                    {/* 视图切换 */}
                    <div className={styles.viewToggle}>
                      <button
                        className={`${styles.viewButton} ${configViewMode === 'form' ? styles.viewButtonActive : ''}`}
                        onClick={() => {
                          setConfigViewMode('form');
                          setFormRenderError(false);
                        }}
                        title={formRenderError ? '点击重新尝试表单视图' : '切换到表单视图'}
                      >
                        表单视图
                      </button>
                      <button
                        className={`${styles.viewButton} ${configViewMode === 'json' ? styles.viewButtonActive : ''}`}
                        onClick={() => setConfigViewMode('json')}
                      >
                        JSON视图
                      </button>
                    </div>
                    
                    {currentModule && (
                      <DemoSelector
                        moduleId={currentModule.id}
                        onSelectDemo={handleSelectDemo}
                        hasFormData={Object.keys(formData).length > 0}
                      />
                    )}
                  </div>
                </div>
                
                {/* AI 输入面板 - 贯穿式显示在标题栏下方 */}
                {ai.expanded && isV2Schema(currentSchema) && currentModule && (
                  <>
                    {console.log('[AddResources] currentModule:', currentModule)}
                    {console.log('[AddResources] ai_prompts:', currentModule.ai_prompts)}
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
                      prompts={currentModule.ai_prompts}
                      progress={ai.progress}
                      finalProgress={ai.finalProgress}
                    />
                  </>
                )}
                
                {formRenderError && configViewMode === 'json' && (
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
                    <span> 表单渲染失败，已自动切换到JSON视图。编辑完成后可点击"表单视图"按钮重新尝试。</span>
                  </div>
                )}
                
                {configViewMode === 'form' && !formRenderError ? (
                  <ErrorBoundary
                    onError={() => {
                      setFormRenderError(true);
                      setConfigViewMode('json');
                      showToast('表单渲染失败，已切换到JSON视图', 'warning');
                    }}
                  >
                    {isV2Schema(currentSchema) ? (
                      <OpenAPIFormRenderer
                        schema={currentSchema.openapi_schema}
                        initialValues={formData}
                        onChange={setFormData}
                      />
                    ) : (
                      <DynamicForm
                        schema={currentSchema.schema_data}
                        values={formData}
                        onChange={setFormData}
                        initialFieldsToShow={initialFieldsToShow}
                      />
                    )}
                  </ErrorBoundary>
                ) : (
                  <JsonEditor
                    value={JSON.stringify(filteredFormDataForSubmit, null, 2)}
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
            ) : currentSchema ? (
              <div className={styles.notice}>
                该Module暂无Schema定义，请先在Module管理页面生成Schema
              </div>
            ) : (
              <div className={styles.notice}>
                加载Schema中...
              </div>
            )}
          </div>
        );

      case 'preview':
        return (
          <div className={styles.previewStep}>
            <h2 className={styles.stepTitle}>预览资源</h2>
            <p className={styles.stepDesc}>确认要添加的资源配置</p>

            {/* 内嵌FormPreview内容 */}
            {currentSchema && resourceConfigs.length > 0 && (
              <div className={styles.previewContainer}>
                {/* 视图切换按钮 */}
                <div className={styles.viewToggle}>
                  <button
                    className={`${styles.viewButton} ${viewMode === 'form' ? styles.viewButtonActive : ''}`}
                    onClick={() => {
                      setViewMode('form');
                      setPreviewRenderError(false);
                    }}
                    title={previewRenderError ? '点击重新尝试表单视图' : '切换到表单视图'}
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

                {previewRenderError && viewMode === 'json' && (
                  <div style={{
                    padding: '12px 16px',
                    background: '#fff3cd',
                    border: '1px solid #ffc107',
                    borderRadius: '6px',
                    color: '#856404',
                    marginBottom: '16px'
                  }}>
                     表单预览渲染失败，已自动切换到JSON视图
                  </div>
                )}

                {/* 资源列表 */}
                <div className={styles.resourcesList}>
                  {resourceConfigs.map((config, index) => (
                    <div key={index} className={styles.resourcePreview}>
                      <div className={styles.resourceHeader}>
                        <span className={styles.resourceType}>{config.resource_type}</span>
                        <span className={styles.resourceName}>{config.resource_name}</span>
                      </div>
                      
                      {/* 使用FormPreview组件的inline模式 */}
                      <div className={styles.previewContent}>
                        {viewMode === 'form' && !previewRenderError ? (
                          <ErrorBoundary
                            onError={() => {
                              setPreviewRenderError(true);
                              setViewMode('json');
                              showToast('预览渲染失败，已切换到JSON视图', 'warning');
                            }}
                          >
                            {isV2Schema(currentSchema) ? (
                              <OpenAPIFormRenderer
                                schema={currentSchema.openapi_schema}
                                initialValues={config.config}
                                onChange={() => {}}
                                readOnly={true}
                              />
                            ) : (
                              <FormPreview
                                schema={currentSchema.schema_data}
                                values={config.config}
                                onClose={() => {}}
                                inline={true}
                                viewMode={viewMode}
                                onViewModeChange={setViewMode}
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
                              {JSON.stringify(config.config, null, 2)}
                            </pre>
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className={styles.runTypeSelection}>
              <h3 className={styles.selectionTitle}>选择执行类型</h3>
              <div className={styles.runTypeOptions}>
                <label className={`${styles.runTypeOption} ${runType === 'none' ? styles.runTypeSelected : ''}`}>
                  <input
                    type="radio"
                    name="runType"
                    value="none"
                    checked={runType === 'none'}
                    onChange={() => setRunType('none')}
                  />
                  <div>
                    <div className={styles.runTypeTitle}>仅添加资源</div>
                    <div className={styles.runTypeDesc}>只保存资源配置，不执行任何任务</div>
                  </div>
                </label>
                
                <label className={`${styles.runTypeOption} ${runType === 'plan' ? styles.runTypeSelected : ''}`}>
                  <input
                    type="radio"
                    name="runType"
                    value="plan"
                    checked={runType === 'plan'}
                    onChange={() => setRunType('plan')}
                  />
                  <div>
                    <div className={styles.runTypeTitle}>Plan only</div>
                    <div className={styles.runTypeDesc}>预览变更，不执行Apply</div>
                  </div>
                </label>
                
                <label className={`${styles.runTypeOption} ${runType === 'plan_and_apply' ? styles.runTypeSelected : ''}`}>
                  <input
                    type="radio"
                    name="runType"
                    value="plan_and_apply"
                    checked={runType === 'plan_and_apply'}
                    onChange={() => setRunType('plan_and_apply')}
                  />
                  <div>
                    <div className={styles.runTypeTitle}>Plan and Apply</div>
                    <div className={styles.runTypeDesc}>执行Plan后自动Apply（根据Apply Method设置）</div>
                  </div>
                </label>
              </div>
            </div>
          </div>
        );

      default:
        return null;
    }
  };

  // 导航菜单项
  const navItems = [
    { id: 'overview', label: 'Overview' },
    { id: 'runs', label: 'Runs' },
    { id: 'states', label: 'States' },
    { id: 'resources', label: 'Resources' },
    { id: 'variables', label: 'Variables' },
    { id: 'outputs', label: 'Outputs' },
    { id: 'health', label: 'Health' },
  ];

  return (
    <div className={styles.workspaceLayout}>
      {/* 左侧导航栏 */}
      <aside className={styles.workspaceSidebar}>
        <div className={styles.workspaceHeader}>
          <button onClick={() => navigate('/workspaces')} className={styles.sidebarBackButton}>
            ← Workspaces
          </button>
          <h1 className={styles.workspaceTitle}>添加资源</h1>
        </div>

        {/* 导航菜单 */}
        <nav className={styles.workspaceNav}>
          {navItems.map((item) => (
            <Link
              key={item.id}
              to={`/workspaces/${id}?tab=${item.id}`}
              className={`${styles.navItem} ${item.id === 'resources' ? styles.navItemActive : ''}`}
            >
              <span className={styles.navLabel}>{item.label}</span>
            </Link>
          ))}
        </nav>
      </aside>

      {/* 右侧主内容区 */}
      <main className={styles.workspaceMain}>
        <TopBar title="添加资源" />
        
        <div className={styles.container}>
          <div className={styles.header}>
            <div className={styles.headerLeft}>
              <button onClick={handleCancel} className={styles.backButton}>
                ← 返回Workspace
              </button>
              <h1 className={styles.title}>添加资源</h1>
            </div>
            
            <div className={styles.steps}>
              <div className={`${styles.stepIndicator} ${step === 'select' ? styles.stepActive : styles.stepCompleted}`}>
                1. 选择Module
              </div>
              <div className={`${styles.stepIndicator} ${step === 'configure' ? styles.stepActive : step === 'preview' ? styles.stepCompleted : ''}`}>
                2. 配置资源
              </div>
              <div className={`${styles.stepIndicator} ${step === 'preview' ? styles.stepActive : ''}`}>
                3. 预览提交
              </div>
            </div>
          </div>

          <div className={styles.content}>
            {renderStepContent()}
          </div>

          <div className={styles.footer}>
            <div className={styles.footerLeft}>
              {step !== 'select' && (
                <button onClick={handleBack} className={styles.btnSecondary}>
                  上一步
                </button>
              )}
            </div>
            
            <div className={styles.footerRight}>
              <button onClick={handleCancel} className={styles.btnCancel}>
                取消
              </button>
              
              {step === 'configure' && (
                <button onClick={() => {
                  saveCurrentConfig();
                  setStep('preview');
                }} className={styles.btnSecondary}>
                  跳过并预览
                </button>
              )}
              
              {step === 'preview' ? (
                <button
                  onClick={handleSubmit}
                  className={styles.btnPrimary}
                  disabled={loading}
                >
                  {loading ? '提交中...' : (
                    runType === 'none' ? '仅添加资源' :
                    runType === 'plan' ? '添加并执行Plan' :
                    '添加并执行Plan+Apply'
                  )}
                </button>
              ) : (
                <button
                  onClick={handleNext}
                  className={styles.btnPrimary}
                  disabled={loading || (step === 'select' && selectedModules.length === 0)}
                >
                  {step === 'configure' && currentModuleIndex < selectedModules.length - 1
                    ? '下一个'
                    : '下一步'}
                </button>
              )}
            </div>
          </div>
        </div>
      </main>

      <ConfirmDialog
        isOpen={showDemoConfirmDialog}
        title="确认使用Demo配置"
        message="选择Demo将覆盖当前已填写的表单数据，是否继续？"
        confirmText="确认使用"
        cancelText="取消"
        onConfirm={confirmApplyDemo}
        onCancel={() => {
          setShowDemoConfirmDialog(false);
          setPendingDemoData(null);
          setPendingDemoName('');
        }}
        type="warning"
      />

      {/* AI 预览弹窗 - 使用 mergedConfig 显示合并后的完整数据 */}
      <AIPreviewModal
        open={ai.previewOpen}
        onClose={() => ai.setPreviewOpen(false)}
        onApply={ai.handleApplyConfig}
        onRecheck={() => ai.handleGenerate('refine')}
        onRegenerate={(userSelections) => ai.handleGenerateWithSelections(userSelections)}
        generatedConfig={ai.mergedConfig || ai.generatedConfig}
        placeholders={ai.placeholders}
        emptyFields={ai.emptyFields}
        renderConfigValue={ai.renderConfigValue}
        mode={ai.generateMode}
        loading={ai.loading}
        blockMessage={ai.blockMessage}
        userDescription={ai.description}
        cmdbLookups={ai.cmdbLookups}
        warnings={ai.warnings}
        needSelection={ai.needSelection}
        progress={ai.progress}
        finalProgress={ai.finalProgress}
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

export default AddResources;
