import React, { useState, useEffect, useRef } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import DynamicForm, { type FormSchema, FormPreview } from '../components/DynamicForm';
import { FormRenderer as OpenAPIFormRenderer } from '../components/OpenAPIFormRenderer';
import { OpenAPISchemaEditor } from '../components/OpenAPISchemaEditor';
import { JsonDiff } from '../components/JsonDiff';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { processApiSchema } from '../utils/schemaTypeMapper';
import type { OpenAPISchema } from '../services/schemaV2';
import api from '../services/api';
import styles from './SchemaManagement.module.css';

interface Schema {
  id: number;
  module_id: number;
  version: string;
  status: string;
  ai_generated: boolean;
  source_type: 'json_import' | 'tf_parse' | 'ai_generate';
  schema_data: FormSchema;
  schema_version?: string;
  openapi_schema?: any;
  ui_config?: any;
  variables_tf?: string;
  created_at: string;
  updated_at: string;
}

interface Module {
  id: number;
  name: string;
  provider: string;
  version: string;
}

const SchemaManagement: React.FC = () => {
  const { moduleId } = useParams<{ moduleId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { showToast } = useToast();
  const [module, setModule] = useState<Module | null>(null);
  const [schemas, setSchemas] = useState<Schema[]>([]);
  const [activeSchema, setActiveSchema] = useState<Schema | null>(null);
  const [formValues, setFormValues] = useState<Record<string, any>>({});
  const [loading, setLoading] = useState(true);
  const [showPreview, setShowPreview] = useState(false);
  
  // 从 URL 参数获取初始 tab
  const getInitialTab = (): 'form' | 'json' | 'outputs' | 'versions' => {
    const tabParam = searchParams.get('tab');
    if (tabParam === 'json' || tabParam === 'outputs' || tabParam === 'versions') {
      return tabParam;
    }
    return 'form';
  };
  const [activeTab, setActiveTab] = useState<'form' | 'json' | 'outputs' | 'versions'>(getInitialTab());
  
  // 从 URL 参数获取初始 group（FormRenderer 内部的 tab）
  const getInitialGroup = (): string | undefined => {
    return searchParams.get('group') || undefined;
  };
  const [activeGroup, setActiveGroup] = useState<string | undefined>(getInitialGroup());
  const [jsonString, setJsonString] = useState('');
  const [jsonError, setJsonError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  
  // Schema 编辑器状态
  const [showSchemaEditor, setShowSchemaEditor] = useState(false);
  const [pendingOpenAPISchema, setPendingOpenAPISchema] = useState<OpenAPISchema | null>(null);
  const [pendingVariablesTf, setPendingVariablesTf] = useState<string>('');
  
  // 从 URL 参数获取初始编辑状态
  const getInitialEditMode = (): boolean => {
    return searchParams.get('edit') === 'true';
  };

  // 进入/退出编辑模式时更新 URL
  const enterEditMode = (schema: OpenAPISchema, variablesTf: string) => {
    setPendingOpenAPISchema(schema);
    setPendingVariablesTf(variablesTf);
    setShowSchemaEditor(true);
    
    // 更新 URL 参数
    searchParams.set('edit', 'true');
    setSearchParams(searchParams, { replace: true });
  };

  const exitEditMode = () => {
    setShowSchemaEditor(false);
    setPendingOpenAPISchema(null);
    setPendingVariablesTf('');
    
    // 移除 URL 参数
    searchParams.delete('edit');
    setSearchParams(searchParams, { replace: true });
  };

  // 版本对比状态
  const [showDiffModal, setShowDiffModal] = useState(false);
  const [diffOldVersion, setDiffOldVersion] = useState<Schema | null>(null);
  const [diffNewVersion, setDiffNewVersion] = useState<Schema | null>(null);

  // 判断是否是 V2 Schema
  const isV2Schema = (schema: Schema | null): boolean => {
    return schema?.schema_version === 'v2' && !!schema?.openapi_schema;
  };

  // 获取 Schema 的 JSON 数据用于对比
  const getSchemaJson = (schema: Schema): any => {
    if (isV2Schema(schema)) {
      return schema.openapi_schema;
    }
    return schema.schema_data;
  };

  useEffect(() => {
    const fetchModuleAndSchemas = async () => {
      try {
        const moduleResponse = await api.get(`/modules/${moduleId}`);
        setModule(moduleResponse.data);

        const versionId = searchParams.get('version_id');
        const response = await api.get(`/modules/${moduleId}/schemas`, {
          params: versionId ? { version_id: versionId } : undefined,
        });
        const schemasData = Array.isArray(response.data) ? response.data : [];

        // 按版本排序（最新的在前）
        const sortedSchemas = schemasData.sort((a: Schema, b: Schema) => {
          return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
        });
        setSchemas(sortedSchemas);

        const activeSchemaData = sortedSchemas.find((s: Schema) => s.status === 'active') || sortedSchemas[0];
        if (activeSchemaData) {
          if (activeSchemaData.schema_version === 'v2' && activeSchemaData.openapi_schema) {
            setActiveSchema(activeSchemaData);

            // 如果 URL 参数指示编辑模式，自动进入编辑状态
            if (getInitialEditMode()) {
              setPendingOpenAPISchema(activeSchemaData.openapi_schema);
              setPendingVariablesTf(activeSchemaData.variables_tf || '');
              setShowSchemaEditor(true);
            }
          } else {
            let parsedSchemaData = activeSchemaData.schema_data;
            if (typeof activeSchemaData.schema_data === 'string') {
              try {
                parsedSchemaData = JSON.parse(activeSchemaData.schema_data);
              } catch (e) {
                parsedSchemaData = {};
              }
            }
            const processedSchema = processApiSchema({
              ...activeSchemaData,
              schema_data: parsedSchemaData
            });
            setActiveSchema(processedSchema);
          }
        }
      } catch (error) {
        const message = extractErrorMessage(error);
        showToast(message, 'error');
      } finally {
        setLoading(false);
      }
    };

    if (moduleId) {
      fetchModuleAndSchemas();
    }
  }, [moduleId, searchParams, showToast]);

  // 处理 TF 文件上传（支持多文件：variables.tf + outputs.tf）
  const handleTfFileUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files;
    if (!files || files.length === 0) return;

    // 验证所有文件都是 .tf 文件
    for (let i = 0; i < files.length; i++) {
      if (!files[i].name.endsWith('.tf')) {
        showToast('请只上传 .tf 文件', 'error');
        return;
      }
    }

    setUploading(true);
    try {
      let variablesTf = '';
      let outputsTf = '';

      // 读取所有选中的文件
      for (let i = 0; i < files.length; i++) {
        const file = files[i];
        const content = await file.text();
        const fileName = file.name.toLowerCase();

        if (fileName.includes('variable') || fileName === 'variables.tf') {
          variablesTf += content + '\n';
        } else if (fileName.includes('output') || fileName === 'outputs.tf') {
          outputsTf += content + '\n';
        } else {
          // 默认当作 variables.tf 处理
          variablesTf += content + '\n';
        }
      }

      if (!variablesTf.trim() && !outputsTf.trim()) {
        showToast('未找到有效的 TF 文件内容', 'error');
        return;
      }
      
      const parseResponse = await api.post('/modules/parse-tf-v2', {
        variables_tf: variablesTf || undefined,
        outputs_tf: outputsTf || undefined,
        module_name: module?.name || 'Module',
        module_version: module?.version || '1.0.0',
        provider: module?.provider || 'aws'
      });

      const responseData = parseResponse as any;

      if (!responseData) {
        throw new Error('解析响应为空');
      }
      
      const openapi_schema = responseData.openapi_schema || 
                             responseData.OpenAPISchema ||
                             responseData.schema;
      
      if (!openapi_schema) {
        throw new Error(`解析响应中缺少 openapi_schema 字段`);
      }

      setPendingOpenAPISchema(openapi_schema);
      setPendingVariablesTf(variablesTf);
      setShowSchemaEditor(true);
      
      const fieldCount = Object.keys(openapi_schema.components?.schemas?.ModuleInput?.properties || {}).length;
      const outputCount = openapi_schema['x-iac-platform']?.outputs?.items?.length || 0;
      
      let message = `TF 文件解析成功！共 ${fieldCount} 个变量`;
      if (outputCount > 0) {
        message += `，${outputCount} 个输出`;
      }
      showToast(message, 'success');
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(`解析失败: ${message}`, 'error');
    } finally {
      setUploading(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    }
  };

  // Schema 编辑器保存回调
  const handleSchemaEditorSave = async (editedSchema: OpenAPISchema) => {
    try {
      setUploading(true);
      
      const versionId = searchParams.get('version_id');
      const createResponse = await api.post(`/modules/${moduleId}/schemas/v2`, {
        openapi_schema: editedSchema,
        variables_tf: pendingVariablesTf,
        version: generateNextVersion(),
        status: 'active'
      }, {
        params: versionId ? { version_id: versionId } : undefined,
      });

      const createdSchema = createResponse as any;

      if (createdSchema && createdSchema.id) {
        const newSchema = {
          ...createdSchema,
          schema_version: 'v2',
          openapi_schema: editedSchema
        };
        setActiveSchema(newSchema);
        setSchemas(prev => [newSchema, ...prev]);
        exitEditMode();
        showToast('Schema 保存成功！', 'success');
      } else {
        throw new Error('创建 Schema 失败');
      }
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(`保存失败: ${message}`, 'error');
    } finally {
      setUploading(false);
    }
  };

  const handleSchemaEditorCancel = () => {
    exitEditMode();
  };

  const generateNextVersion = (): string => {
    if (schemas.length === 0) return '1';
    
    // 找出最大的版本号
    let maxVersion = 0;
    for (const schema of schemas) {
      // 支持 "1", "2", "3" 或 "1.0.0", "1.0.1" 格式
      const versionStr = schema.version || '0';
      const parts = versionStr.split('.');
      const majorVersion = parseInt(parts[0], 10) || 0;
      if (majorVersion > maxVersion) {
        maxVersion = majorVersion;
      }
    }
    
    return String(maxVersion + 1);
  };

  const handleFormSubmit = () => {
    console.log('表单数据:', formValues);
    showToast('配置已生成！查看控制台输出。', 'success');
  };

  const handleTabChange = (tab: 'form' | 'json' | 'outputs' | 'versions') => {
    if (tab === 'json' && activeTab === 'form') {
      setJsonString(JSON.stringify(formValues, null, 2));
      setJsonError(null);
    } else if (tab === 'form' && activeTab === 'json') {
      try {
        const parsed = JSON.parse(jsonString);
        setFormValues(parsed);
        setJsonError(null);
      } catch (error: any) {
        setJsonError(`JSON 格式错误: ${error.message}`);
        showToast('JSON 格式错误，请修正后再切换', 'error');
        return;
      }
    }
    setActiveTab(tab);
    
    // 更新 URL 参数
    if (tab === 'form') {
      searchParams.delete('tab');
    } else {
      searchParams.set('tab', tab);
    }
    setSearchParams(searchParams, { replace: true });
  };

  // 处理 FormRenderer 内部 group 切换
  const handleGroupChange = (groupId: string) => {
    setActiveGroup(groupId);
    
    // 更新 URL 参数
    searchParams.set('group', groupId);
    setSearchParams(searchParams, { replace: true });
  };

  // 从 Schema 中提取 outputs 定义
  interface ModuleOutput {
    name: string;
    alias?: string;
    type: string;
    description?: string;
    sensitive?: boolean;
    valueExpression?: string;
  }

  const extractOutputsFromSchema = (schema: any): ModuleOutput[] => {
    const outputs: ModuleOutput[] = [];

    if (!schema) return outputs;

    // 方式1: 从 x-iac-platform.outputs.items 提取
    const iacPlatform = schema['x-iac-platform'];
    if (iacPlatform?.outputs?.items) {
      for (const item of iacPlatform.outputs.items) {
        outputs.push({
          name: item.name || '',
          alias: item.alias,
          type: item.type || 'string',
          description: item.description,
          sensitive: item.sensitive,
          valueExpression: item.valueExpression,
        });
      }
    }

    // 方式2: 从 components.schemas.ModuleOutput.properties 提取
    if (outputs.length === 0) {
      const moduleOutput = schema.components?.schemas?.ModuleOutput;
      if (moduleOutput?.properties) {
        for (const [name, prop] of Object.entries(moduleOutput.properties)) {
          const propObj = prop as any;
          outputs.push({
            name,
            alias: propObj['x-alias'],
            type: propObj.type || 'string',
            description: propObj.description,
            sensitive: propObj['x-sensitive'],
            valueExpression: propObj['x-value-expression'],
          });
        }
      }
    }

    return outputs;
  };

  // 渲染 Outputs 列表
  const renderOutputsList = () => {
    if (!activeSchema) return null;

    const schema = isV2Schema(activeSchema) ? activeSchema.openapi_schema : null;
    const outputs = extractOutputsFromSchema(schema);

    return (
      <div className={styles.outputsList}>
        <div className={styles.outputsHeader}>
          <h3>模块输出 (Outputs)</h3>
          <span className={styles.outputsCount}>{outputs.length} 个输出</span>
        </div>
        
        {outputs.length === 0 ? (
          <div className={styles.emptyOutputs}>
            <p>此模块暂无定义 Outputs</p>
            <p style={{ fontSize: '12px', color: '#999' }}>
              提示：使用 tf2openapi 工具解析 outputs.tf 文件可以生成 Outputs 定义
            </p>
          </div>
        ) : (
          <table className={styles.outputsTable}>
            <thead>
              <tr>
                <th>输出名</th>
                <th>类型</th>
                <th>描述</th>
                <th>属性</th>
              </tr>
            </thead>
            <tbody>
              {outputs.map(output => (
                <tr key={output.name}>
                  <td>
                    <code className={styles.outputName}>{output.name}</code>
                    {output.alias && (
                      <span className={styles.outputAlias}>({output.alias})</span>
                    )}
                  </td>
                  <td>
                    <span className={styles.outputType}>{output.type}</span>
                  </td>
                  <td>{output.description || '-'}</td>
                  <td>
                    {output.sensitive && (
                      <span className={styles.sensitiveTag}>Sensitive</span>
                    )}
                    {output.valueExpression && (
                      <code className={styles.valueExpression}>
                        {output.valueExpression}
                      </code>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    );
  };

  const formatJSON = () => {
    try {
      const parsed = JSON.parse(jsonString);
      setJsonString(JSON.stringify(parsed, null, 2));
      setJsonError(null);
      showToast('JSON 格式化成功', 'success');
    } catch (error: any) {
      setJsonError(`JSON 格式错误: ${error.message}`);
      showToast('JSON 格式错误，无法格式化', 'error');
    }
  };

  const copyJSON = async () => {
    try {
      await navigator.clipboard.writeText(jsonString);
      showToast('已复制到剪贴板', 'success');
    } catch (error) {
      showToast('复制失败', 'error');
    }
  };

  // 打开版本对比
  const openVersionDiff = (oldSchema: Schema, newSchema: Schema) => {
    setDiffOldVersion(oldSchema);
    setDiffNewVersion(newSchema);
    setShowDiffModal(true);
  };

  // 默认对比：当前版本与上一个版本
  const openDefaultDiff = () => {
    if (schemas.length < 2) {
      showToast('至少需要两个版本才能对比', 'warning');
      return;
    }
    const currentIndex = schemas.findIndex(s => s.id === activeSchema?.id);
    if (currentIndex === -1 || currentIndex >= schemas.length - 1) {
      openVersionDiff(schemas[1], schemas[0]);
    } else {
      openVersionDiff(schemas[currentIndex + 1], schemas[currentIndex]);
    }
  };

  // 切换到指定版本（仅预览）
  const switchToVersion = (schema: Schema) => {
    if (schema.schema_version === 'v2' && schema.openapi_schema) {
      setActiveSchema(schema);
    } else {
      let parsedSchemaData = schema.schema_data;
      if (typeof schema.schema_data === 'string') {
        try {
          parsedSchemaData = JSON.parse(schema.schema_data);
        } catch (e) {
          parsedSchemaData = {};
        }
      }
      const processedSchema = processApiSchema({
        ...schema,
        schema_data: parsedSchemaData
      });
      setActiveSchema(processedSchema);
    }
    setFormValues({});
    showToast(`已切换到版本 ${schema.version}（预览模式）`, 'success');
  };

  // 设置活跃版本（调用后端 API）
  const setActiveVersion = async (schema: Schema) => {
    try {
      await api.post(`/modules/${moduleId}/schemas/${schema.id}/activate`);
      
      // 更新本地状态
      setSchemas(prev => prev.map(s => ({
        ...s,
        status: s.id === schema.id ? 'active' : 'inactive'
      })));
      
      // 切换到该版本
      switchToVersion(schema);
      showToast(`已将版本 ${schema.version} 设置为活跃版本`, 'success');
    } catch (error) {
      const message = extractErrorMessage(error);
      showToast(`设置活跃版本失败: ${message}`, 'error');
    }
  };

  const renderForm = () => {
    if (!activeSchema) return null;

    if (isV2Schema(activeSchema)) {
      return (
        <OpenAPIFormRenderer
          schema={activeSchema.openapi_schema}
          initialValues={formValues}
          onChange={setFormValues}
          activeGroupId={activeGroup}
          onGroupChange={handleGroupChange}
        />
      );
    } else {
      return (
        <DynamicForm
          schema={activeSchema.schema_data}
          values={formValues}
          onChange={setFormValues}
        />
      );
    }
  };

  // 渲染版本列表
  const renderVersionList = () => {
    return (
      <div className={styles.versionList}>
        <div className={styles.versionHeader}>
          <h3>版本历史</h3>
          <button 
            onClick={openDefaultDiff}
            className={styles.diffButton}
            disabled={schemas.length < 2}
          >
            📊 对比版本
          </button>
        </div>
        
        {schemas.length === 0 ? (
          <div className={styles.emptyVersions}>暂无版本记录</div>
        ) : (
          <div className={styles.versionItems}>
            {schemas.map((schema, index) => (
              <div 
                key={schema.id} 
                className={`${styles.versionItem} ${schema.id === activeSchema?.id ? styles.active : ''}`}
              >
                <div className={styles.versionInfo}>
                  <div className={styles.versionMain}>
                    <span className={styles.versionNumber}>v{schema.version}</span>
                    {schema.status === 'active' && (
                      <span className={styles.activeTag}>当前</span>
                    )}
                  </div>
                  <div className={styles.versionMeta}>
                    <span>{new Date(schema.created_at).toLocaleString()}</span>
                  </div>
                </div>
                <div className={styles.versionActions}>
                  {schema.id !== activeSchema?.id && (
                    <button 
                      onClick={() => switchToVersion(schema)}
                      className={styles.switchButton}
                    >
                      查看
                    </button>
                  )}
                  {index < schemas.length - 1 && (
                    <button 
                      onClick={() => openVersionDiff(schemas[index + 1], schema)}
                      className={styles.compareButton}
                      title="与上一版本对比"
                    >
                      对比
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    );
  };

  // 如果正在显示 Schema 编辑器
  if (showSchemaEditor && pendingOpenAPISchema) {
    return (
      <div className={styles.container}>
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <button onClick={handleSchemaEditorCancel} className={styles.backButton}>
              ← 返回
            </button>
            <h1 className={styles.title}>
              {module ? `${module.name} - Schema 编辑器` : 'Schema 编辑器'}
            </h1>
          </div>
        </div>
        
        <div className={styles.contentFull}>
          <OpenAPISchemaEditor
            schema={pendingOpenAPISchema}
            onSave={handleSchemaEditorSave}
            onCancel={handleSchemaEditorCancel}
            title={`编辑 ${module?.name || 'Module'} Schema`}
          />
        </div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>加载中...</div>
      </div>
    );
  }

  if (!activeSchema) {
    return (
      <div className={styles.container}>
        <div className={styles.empty}>
          <h2>暂无 Schema</h2>
          <p>该模块还没有配置 Schema，请上传 variables.tf 文件来创建：</p>
          <div className={styles.emptyActions}>
            <input
              type="file"
              accept=".tf,.hcl,text/plain"
              multiple
              onChange={handleTfFileUpload}
              ref={fileInputRef}
              style={{ display: 'none' }}
            />
            <button 
              onClick={() => fileInputRef.current?.click()} 
              className={styles.createButton}
              disabled={uploading}
            >
              {uploading ? '解析中...' : '📄 上传 TF 文件（支持多选）'}
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <button onClick={() => navigate(-1)} className={styles.backButton}>
            ← 返回
          </button>
          <h1 className={styles.title}>
            {module ? `${module.name}` : 'Schema 管理'}
          </h1>
          <div className={styles.schemaInfo}>
            <span className={styles.version}>v{activeSchema.version}</span>
            <span className={`${styles.status} ${styles[activeSchema.status]}`}>
              {activeSchema.status}
            </span>
            {isV2Schema(activeSchema) && (
              <span className={styles.v2Tag}>OpenAPI v3</span>
            )}
          </div>
        </div>
        <div className={styles.headerActions}>
          <input
            type="file"
            accept=".tf,.hcl,text/plain"
            multiple
            onChange={handleTfFileUpload}
            ref={fileInputRef}
            style={{ display: 'none' }}
          />
          <button 
            onClick={() => fileInputRef.current?.click()} 
            className={styles.importButton}
            disabled={uploading}
            title="支持同时选择 variables.tf 和 outputs.tf"
          >
            {uploading ? '解析中...' : '📄 新建版本'}
          </button>
          {isV2Schema(activeSchema) && (
            <button 
              onClick={() => enterEditMode(activeSchema.openapi_schema, activeSchema.variables_tf || '')} 
              className={styles.editSchemaButton}
            >
              ✏️ 编辑 Schema
            </button>
          )}
        </div>
      </div>

      <div className={styles.contentFull}>
        <div className={styles.formContainer}>
          <div className={styles.tabs}>
            <button
              className={`${styles.tab} ${activeTab === 'form' ? styles.active : ''}`}
              onClick={() => handleTabChange('form')}
            >
              配置表单
            </button>
            <button
              className={`${styles.tab} ${activeTab === 'json' ? styles.active : ''}`}
              onClick={() => handleTabChange('json')}
            >
              配置 JSON
            </button>
            <button
              className={`${styles.tab} ${activeTab === 'outputs' ? styles.active : ''}`}
              onClick={() => handleTabChange('outputs')}
            >
              Outputs
            </button>
            <button
              className={`${styles.tab} ${activeTab === 'versions' ? styles.active : ''}`}
              onClick={() => handleTabChange('versions')}
            >
              版本历史 ({schemas.length})
            </button>
          </div>

          <div className={styles.tabContent}>
            {activeTab === 'form' && (
              <>
                <p className={styles.formDescription}>
                  基于 Schema 自动生成的配置表单
                  {isV2Schema(activeSchema) && (
                    <span className={styles.v2Hint}> (OpenAPI v3 渲染器)</span>
                  )}
                </p>
                {renderForm()}
              </>
            )}

            {activeTab === 'json' && (
              <div className={styles.jsonEditorContainer}>
                <div className={styles.jsonToolbar}>
                  <button onClick={formatJSON} className={styles.toolButton}>格式化</button>
                  <button onClick={copyJSON} className={styles.toolButton}>复制</button>
                  <span className={styles.toolHint}>
                    提示：修改 JSON 后切换到"配置表单"即可应用更改
                  </span>
                </div>
                <textarea
                  value={jsonString}
                  onChange={(e) => setJsonString(e.target.value)}
                  className={styles.jsonEditor}
                  spellCheck={false}
                  placeholder="在此编辑 JSON 配置..."
                />
                {jsonError && <div className={styles.jsonError}>{jsonError}</div>}
              </div>
            )}

            {activeTab === 'outputs' && renderOutputsList()}

            {activeTab === 'versions' && renderVersionList()}
          </div>

          {(activeTab !== 'versions' && activeTab !== 'outputs') && (
            <div className={styles.actions}>
              <button onClick={() => setShowPreview(true)} className={styles.previewButton}>
                预览配置
              </button>
              <button onClick={handleFormSubmit} className={styles.generateButton}>
                生成配置
              </button>
            </div>
          )}
        </div>
      </div>
      
      {/* 预览弹窗 */}
      {showPreview && activeSchema && (
        <div className={styles.modalOverlay} onClick={() => setShowPreview(false)}>
          <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
            <h2>配置预览</h2>
            <pre className={styles.previewCode}>
              {JSON.stringify(formValues, null, 2)}
            </pre>
            <div className={styles.modalActions}>
              <button onClick={() => setShowPreview(false)} className={styles.closeButton}>
                关闭
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 版本对比弹窗 */}
      {showDiffModal && diffOldVersion && diffNewVersion && (
        <div className={styles.modalOverlay} onClick={() => setShowDiffModal(false)}>
          <div className={styles.diffModal} onClick={(e) => e.stopPropagation()}>
            <div className={styles.diffModalHeader}>
              <h2>版本对比</h2>
              <div className={styles.diffVersionSelector}>
                <select 
                  value={diffOldVersion.id}
                  onChange={(e) => {
                    const schema = schemas.find(s => s.id === Number(e.target.value));
                    if (schema) setDiffOldVersion(schema);
                  }}
                  className={styles.versionSelect}
                >
                  {schemas.map(s => (
                    <option key={s.id} value={s.id}>v{s.version}</option>
                  ))}
                </select>
                <span className={styles.diffArrow}>→</span>
                <select 
                  value={diffNewVersion.id}
                  onChange={(e) => {
                    const schema = schemas.find(s => s.id === Number(e.target.value));
                    if (schema) setDiffNewVersion(schema);
                  }}
                  className={styles.versionSelect}
                >
                  {schemas.map(s => (
                    <option key={s.id} value={s.id}>v{s.version}</option>
                  ))}
                </select>
              </div>
              <button 
                onClick={() => setShowDiffModal(false)} 
                className={styles.closeModalButton}
              >
                ✕
              </button>
            </div>
            <div className={styles.diffContent}>
              <JsonDiff
                oldJson={getSchemaJson(diffOldVersion)}
                newJson={getSchemaJson(diffNewVersion)}
                oldLabel={`v${diffOldVersion.version}`}
                newLabel={`v${diffNewVersion.version}`}
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default SchemaManagement;
