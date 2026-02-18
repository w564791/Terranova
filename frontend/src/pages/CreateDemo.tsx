import React, { useState, useEffect, Component, type ReactNode } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import api from '../services/api';
import { FormRenderer as OpenAPIFormRenderer } from '../components/OpenAPIFormRenderer';
import { 
  AITriggerButton, 
  AIInputPanel, 
  AIPreviewModal, 
  useAIConfigGenerator 
} from '../components/OpenAPIFormRenderer/AIFormAssistant';
import { JsonEditor } from '../components/DynamicForm/JsonEditor';
import { moduleDemoService } from '../services/moduleDemos';
import { schemaV2Service } from '../services/schemaV2';
import { getVersion, type ModuleVersion } from '../services/moduleVersions';
import styles from './AddResources.module.css';

interface Module {
  id: number;
  name: string;
  description: string;
  provider: string;
  source: string;
  module_source?: string;
  source_type: string;
  version?: string;
}

const CreateDemo: React.FC = () => {
  const { moduleId } = useParams<{ moduleId: string }>();
  const [searchParams] = useSearchParams();
  const versionId = searchParams.get('version_id');
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  const [module, setModule] = useState<Module | null>(null);
  const [currentVersion, setCurrentVersion] = useState<ModuleVersion | null>(null);
  const [schema, setSchema] = useState<any>(null);
  const [formData, setFormData] = useState<Record<string, any>>({});
  const [demoName, setDemoName] = useState('');
  const [description, setDescription] = useState('');
  const [usageNotes, setUsageNotes] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [nameError, setNameError] = useState('');
  const [viewMode, setViewMode] = useState<'form' | 'json'>('form');
  const [formRenderError, setFormRenderError] = useState(false);
  const [noSchema, setNoSchema] = useState(false);

  // 从 Schema 中提取默认值
  const extractSchemaDefaults = React.useCallback((schemaData: any): Record<string, unknown> => {
    if (!schemaData) return {};
    
    const properties = schemaData?.components?.schemas?.ModuleInput?.properties || {};
    const defaults: Record<string, unknown> = {};
    
    Object.entries(properties).forEach(([name, prop]: [string, any]) => {
      if (Object.prototype.hasOwnProperty.call(prop, 'default')) {
        defaults[name] = prop.default;
      }
    });
    
    return defaults;
  }, []);

  // 从 Schema 中提取必填字段列表
  const extractRequiredFields = React.useCallback((schemaData: any): string[] => {
    if (!schemaData) return [];
    return schemaData?.components?.schemas?.ModuleInput?.required || [];
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
    const defaults = extractSchemaDefaults(schema);
    // 使用深度合并，确保嵌套对象（如 tags）中用户新增的字段也能被包含
    return deepMergeForDisplay(defaults, formData);
  }, [schema, formData, extractSchemaDefaults]);

  // 过滤掉 Schema 默认值和空值，只保留用户实际修改的数据和必填字段
  // 用于 JSON 视图显示和提交时使用
  const filterSchemaDefaultsAndEmpty = React.useCallback((
    data: Record<string, unknown>, 
    schemaDefaults: Record<string, unknown>,
    requiredFields: string[] = []
  ): Record<string, unknown> => {
    const result: Record<string, unknown> = {};
    
    // 首先添加所有必填字段（即使值为空）
    requiredFields.forEach(key => {
      const value = data[key];
      // 必填字段始终保留，即使值为 undefined/null/空字符串
      result[key] = value !== undefined ? value : '';
    });
    
    Object.keys(data).forEach(key => {
      const value = data[key];
      const defaultValue = schemaDefaults[key];
      const isRequired = requiredFields.includes(key);
      
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

  // 用于 JSON 视图显示和提交的数据（过滤掉默认值和空值，但保留必填字段）
  const filteredFormData = React.useMemo(() => {
    const defaults = extractSchemaDefaults(schema);
    const required = extractRequiredFields(schema);
    return filterSchemaDefaultsAndEmpty(formData, defaults, required);
  }, [schema, formData, extractSchemaDefaults, extractRequiredFields, filterSchemaDefaultsAndEmpty]);

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

  // AI 助手 Hook - 使用合并后的数据，应用时使用智能合并
  const ai = useAIConfigGenerator({
    moduleId: moduleId ? parseInt(moduleId) : 0,
    currentFormData: mergedFormData,
    onGenerate: (config) => {
      // 使用智能合并：以 mergedFormData 为基础（包含用户新增的字段），用 AI 数据补充空值
      // 注意：这里不能使用 prev，因为 prev 可能不包含用户在表单中新增的字段
      const merged = smartMerge(mergedFormData, config);
      setFormData(merged);
    },
  });

  useEffect(() => {
    if (moduleId) {
      loadModule();
      loadVersionAndSchema();
    }
  }, [moduleId, versionId]);

  const loadModule = async () => {
    try {
      const response = await api.get(`/modules/${moduleId}`);
      setModule(response.data);
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
    }
  };

  const loadVersionAndSchema = async () => {
    try {
      setLoading(true);
      setNoSchema(false);
      
      // 如果有 version_id，先获取版本信息
      if (versionId) {
        try {
          const version = await getVersion(parseInt(moduleId!), versionId);
          setCurrentVersion(version);
          
          // 如果该版本没有 Schema，显示提示
          if (!version.active_schema_id) {
            setNoSchema(true);
            setViewMode('json');
            return;
          }
        } catch (error) {
          console.warn('Failed to load version:', error);
        }
      }
      
      // 获取 Schema（传递 version_id）
      const schemaData = await schemaV2Service.getSchemaV2(parseInt(moduleId!), versionId || undefined);
      if (schemaData?.openapi_schema) {
        setSchema(schemaData.openapi_schema);
      } else {
        setNoSchema(true);
        setViewMode('json');
      }
    } catch (error: any) {
      console.warn('Failed to load schema:', error);
      setNoSchema(true);
      setViewMode('json');
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async () => {
    if (!demoName.trim()) {
      setNameError('请输入 Demo 名称');
      showToast('请输入 Demo 名称', 'warning');
      return;
    }

    try {
      setSaving(true);
      
      await moduleDemoService.createDemo(parseInt(moduleId!), {
        name: demoName.trim(),
        description: description.trim(),
        usage_notes: usageNotes.trim(),
        config_data: filteredFormData,  // 使用过滤后的数据（不包含 Schema 默认值和空值）
        version_id: versionId || undefined,  // 关联到当前版本
      });

      showToast('Demo 创建成功', 'success');
      // 导航回 Demo 列表页，并带上版本参数
      navigate(`/modules/${moduleId}/demos${versionId ? `?version_id=${versionId}` : ''}`);
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = () => {
    navigate(`/modules/${moduleId}`);
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.content} style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '400px' }}>
          <div>加载中...</div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <button onClick={handleCancel} className={styles.backButton}>
            ← 返回 Module
          </button>
          <h1 className={styles.title}>创建 Demo</h1>
        </div>
        
        <div className={styles.steps}>
          <div className={`${styles.stepIndicator} ${styles.stepActive}`}>
            配置 Demo
          </div>
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.configureStep}>
          <h2 className={styles.stepTitle}>
            为 {module?.name || 'Module'} 创建 Demo 配置
          </h2>
          <p className={styles.stepDesc}>
            Demo 是预设的配置模板，可以在创建资源时快速应用
          </p>
          
          {/* Demo 基本信息 */}
          <div style={{ marginBottom: '24px', padding: '16px', background: '#f8f9fa', borderRadius: '8px' }}>
            <div className={styles.formGroup} style={{ marginBottom: '16px' }}>
              <label className={styles.label}>
                Demo 名称 <span style={{ color: 'var(--color-red-500)' }}>*</span>
              </label>
              <input
                type="text"
                className={`${styles.input} ${nameError ? styles.inputError : ''}`}
                value={demoName}
                onChange={(e) => {
                  setDemoName(e.target.value);
                  setNameError('');
                }}
                placeholder="例如：Production Config、Minimal Setup"
              />
              {nameError && (
                <div className={styles.errorMessage}>{nameError}</div>
              )}
            </div>
            
            <div className={styles.formGroup} style={{ marginBottom: '16px' }}>
              <label className={styles.label}>描述</label>
              <textarea
                className={styles.input}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="描述这个 Demo 的用途..."
                rows={2}
                style={{ resize: 'vertical' }}
              />
            </div>
            
            <div className={styles.formGroup}>
              <label className={styles.label}>使用说明</label>
              <textarea
                className={styles.input}
                value={usageNotes}
                onChange={(e) => setUsageNotes(e.target.value)}
                placeholder="如何使用这个配置..."
                rows={2}
                style={{ resize: 'vertical' }}
              />
            </div>
          </div>
          
          {/* 配置表单 */}
          <div className={styles.dynamicFormContainer}>
            <div className={styles.formDescription} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <p style={{ margin: 0 }}>
                  配置参数
                  {schema && (
                    <span style={{ 
                      marginLeft: '8px', 
                      padding: '2px 8px', 
                      background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
                      color: 'white',
                      borderRadius: '4px',
                      fontSize: '12px',
                      fontWeight: 600
                    }}>
                      OpenAPI v3
                    </span>
                  )}
                </p>
                {/* AI 助手按钮 - 和标签在同一行 */}
                {schema && moduleId && (
                  <AITriggerButton
                    expanded={ai.expanded}
                    onClick={() => ai.setExpanded(!ai.expanded)}
                  />
                )}
              </div>
              
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                {/* 视图切换 */}
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
            {ai.expanded && schema && moduleId && (
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
                <span> 表单渲染失败，已自动切换到JSON视图。编辑完成后可点击"表单视图"按钮重新尝试。</span>
              </div>
            )}
            
            {viewMode === 'form' && !formRenderError && schema ? (
              <ErrorBoundary
                onError={() => {
                  setFormRenderError(true);
                  setViewMode('json');
                  showToast('表单渲染失败，已切换到JSON视图', 'warning');
                }}
              >
                <OpenAPIFormRenderer
                  schema={schema}
                  initialValues={formData}
                  onChange={setFormData}
                />
              </ErrorBoundary>
            ) : (
              <JsonEditor
                value={JSON.stringify(filteredFormData, null, 2)}
                onChange={(value) => {
                  try {
                    const parsed = JSON.parse(value);
                    setFormData(parsed);
                  } catch (e) {
                    console.error('Invalid JSON:', e);
                  }
                }}
                minHeight={300}
                maxHeight={600}
              />
            )}
          </div>
        </div>
      </div>

      <div className={styles.footer}>
        <div className={styles.footerLeft}>
        </div>
        
        <div className={styles.footerRight}>
          <button onClick={handleCancel} className={styles.btnCancel}>
            取消
          </button>
          
          <button
            onClick={handleSubmit}
            className={styles.btnPrimary}
            disabled={saving}
          >
            {saving ? '创建中...' : '创建 Demo'}
          </button>
        </div>
      </div>

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

export default CreateDemo;
