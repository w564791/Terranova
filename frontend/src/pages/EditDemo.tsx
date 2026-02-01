import React, { useState, useEffect, useRef } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { moduleDemoService, type ModuleDemo } from '../services/moduleDemos';
import { ModuleFormRenderer } from '../components/ModuleFormRenderer';
import { schemaV2Service, type OpenAPISchema } from '../services/schemaV2';
import { JsonEditor } from '../components/DynamicForm/JsonEditor';
import api from '../services/api';
import styles from './AddResources.module.css';

const EditDemo: React.FC = () => {
  const { moduleId, demoId } = useParams<{ moduleId: string; demoId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { showToast } = useToast();
  const changeSummaryRef = useRef<HTMLInputElement>(null);
  
  // 从 URL 参数获取初始 group（FormRenderer 内部的 tab）
  const getInitialGroup = (): string | undefined => {
    return searchParams.get('group') || undefined;
  };
  const [activeGroup, setActiveGroup] = useState<string | undefined>(getInitialGroup());
  
  // 从 URL 参数获取初始编辑器模式
  const getInitialEditorMode = (): boolean => {
    return searchParams.get('editor') === 'json';
  };
  
  const [demo, setDemo] = useState<ModuleDemo | null>(null);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [usageNotes, setUsageNotes] = useState('');
  const [configData, setConfigData] = useState<Record<string, unknown>>({});
  const [changeSummary, setChangeSummary] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  
  // Schema 相关状态
  const [schema, setSchema] = useState<OpenAPISchema | null>(null);
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [useJsonEditor, setUseJsonEditor] = useState(getInitialEditorMode());

  // 从 Schema 中提取默认值
  const extractSchemaDefaults = React.useCallback((schemaData: OpenAPISchema | null): Record<string, unknown> => {
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
  const extractRequiredFields = React.useCallback((schemaData: OpenAPISchema | null): string[] => {
    if (!schemaData) return [];
    return schemaData?.components?.schemas?.ModuleInput?.required || [];
  }, []);

  // 获取当前 Schema 的必填字段列表
  const requiredFields = React.useMemo(() => {
    return extractRequiredFields(schema);
  }, [schema, extractRequiredFields]);

  // 过滤掉 Schema 默认值和空值，只保留用户实际修改的数据和必填字段
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

  // 用于 JSON 视图显示和提交的数据（过滤掉默认值和空值，但保留必填字段）
  const filteredConfigData = React.useMemo(() => {
    const defaults = extractSchemaDefaults(schema);
    return filterSchemaDefaultsAndEmpty(configData, defaults, requiredFields);
  }, [schema, configData, extractSchemaDefaults, filterSchemaDefaultsAndEmpty, requiredFields]);

  useEffect(() => {
    loadDemo();
    loadSchema();
  }, [moduleId, demoId]);

  const loadDemo = async () => {
    try {
      setLoading(true);
      const data = await moduleDemoService.getDemoById(parseInt(demoId!));
      setDemo(data);
      setName(data.name);
      setDescription(data.description || '');
      setUsageNotes(data.usage_notes || '');
      setConfigData(data.current_version?.config_data || {});
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
      navigate(`/modules/${moduleId}`);
    } finally {
      setLoading(false);
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
      // Schema 加载失败时使用 JSON 编辑器
      setUseJsonEditor(true);
    } finally {
      setSchemaLoading(false);
    }
  };

  const handleSave = async () => {
    if (!name.trim()) {
      showToast('请输入Demo名称', 'warning');
      return;
    }

    if (!changeSummary.trim()) {
      showToast('请输入变更说明', 'warning');
      // 滚动到变更说明输入框
      changeSummaryRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
      changeSummaryRef.current?.focus();
      return;
    }

    try {
      setSaving(true);
      await moduleDemoService.updateDemo(parseInt(demoId!), {
        name,
        description,
        usage_notes: usageNotes,
        config_data: filteredConfigData,  // 使用过滤后的数据（不包含 Schema 默认值和空值）
        change_summary: changeSummary
      });
      
      showToast('Demo 更新成功', 'success');
      navigate(`/modules/${moduleId}/demos/${demoId}`);
    } catch (error: any) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = () => {
    navigate(`/modules/${moduleId}/demos/${demoId}`);
  };

  // 处理表单值变化
  const handleFormChange = (values: Record<string, unknown>) => {
    setConfigData(values);
  };

  // 处理 FormRenderer 内部 group 切换
  const handleGroupChange = (groupId: string) => {
    setActiveGroup(groupId);
    
    // 更新 URL 参数
    const newParams = new URLSearchParams(searchParams);
    newParams.set('group', groupId);
    setSearchParams(newParams, { replace: true });
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
          <h1 className={styles.title}>Demo 不存在</h1>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <button 
            onClick={handleCancel}
            style={{
              padding: '8px 16px',
              background: '#f8f9fa',
              border: '1px solid #dee2e6',
              borderRadius: '6px',
              cursor: 'pointer',
              fontSize: '14px',
              color: '#495057'
            }}
          >
            ← 返回
          </button>
          <h1 className={styles.title}>编辑 Demo</h1>
        </div>
        <div className={styles.headerRight}>
          <button
            onClick={() => {
              const newValue = !useJsonEditor;
              setUseJsonEditor(newValue);
              
              // 更新 URL 参数
              const newParams = new URLSearchParams(searchParams);
              if (newValue) {
                newParams.set('editor', 'json');
              } else {
                newParams.delete('editor');
              }
              setSearchParams(newParams, { replace: true });
            }}
            style={{
              padding: '6px 12px',
              background: useJsonEditor ? '#1890ff' : '#f8f9fa',
              border: '1px solid #dee2e6',
              borderRadius: '6px',
              cursor: 'pointer',
              fontSize: '13px',
              color: useJsonEditor ? '#fff' : '#495057'
            }}
          >
            {useJsonEditor ? '使用表单编辑' : '使用 JSON 编辑'}
          </button>
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.configureStep}>
          <div className={styles.dynamicFormContainer}>
            <div className={styles.formGroup}>
              <label className={styles.label}>Demo 名称 *</label>
              <input
                type="text"
                className={styles.input}
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="输入Demo名称"
              />
            </div>

            <div className={styles.formGroup}>
              <label className={styles.label}>描述</label>
              <input
                type="text"
                className={styles.input}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="输入描述信息"
              />
            </div>

            <div className={styles.formGroup}>
              <label className={styles.label}>使用说明</label>
              <input
                type="text"
                className={styles.input}
                value={usageNotes}
                onChange={(e) => setUsageNotes(e.target.value)}
                placeholder="输入使用说明"
              />
            </div>

            <div className={styles.formGroup}>
              <label className={styles.label}>配置数据</label>
              {schemaLoading ? (
                <div style={{ padding: '20px', textAlign: 'center', color: '#8c8c8c' }}>
                  加载 Schema 中...
                </div>
              ) : useJsonEditor || !schema ? (
                <JsonEditor
                  value={JSON.stringify(filteredConfigData, null, 2)}
                  onChange={(value) => {
                    try {
                      const parsed = JSON.parse(value);
                      setConfigData(parsed);
                    } catch (e) {
                      console.error('Invalid JSON:', e);
                    }
                  }}
                  minHeight={300}
                  maxHeight={600}
                />
              ) : (
                <div style={{ 
                  border: '1px solid #d9d9d9', 
                  borderRadius: '8px', 
                  padding: '16px',
                  background: '#fafafa'
                }}>
                  <ModuleFormRenderer
                    schema={schema}
                    initialValues={configData}
                    onChange={handleFormChange}
                    showVersionBadge={false}
                    activeGroupId={activeGroup}
                    onGroupChange={handleGroupChange}
                  />
                </div>
              )}
            </div>

            <div className={styles.formGroup}>
              <label className={styles.label}>变更说明 *</label>
              <input
                ref={changeSummaryRef}
                type="text"
                className={styles.input}
                value={changeSummary}
                onChange={(e) => setChangeSummary(e.target.value)}
                placeholder="描述本次变更的内容"
              />
              <p className={styles.hint}>
                说明本次修改的内容，将记录在版本历史中
              </p>
            </div>
          </div>
        </div>
      </div>

      <div className={styles.footer}>
        <div className={styles.footerLeft}></div>
        
        <div className={styles.footerRight}>
          <button 
            onClick={handleCancel}
            className={styles.btnCancel}
          >
            取消
          </button>
          <button 
            onClick={handleSave}
            className={styles.btnPrimary}
            disabled={saving}
          >
            {saving ? '保存中...' : '保存'}
          </button>
        </div>
      </div>
    </div>
  );
};

export default EditDemo;
