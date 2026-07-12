import React, { useState, useEffect } from 'react';
import { type ModuleDemo } from '../services/moduleDemos';
import { FormPreview } from '../components/DynamicForm';
import type { FormSchema } from '../components/DynamicForm';
import { FormRenderer as OpenAPIFormRenderer } from './OpenAPIFormRenderer';
import FormRendererV3 from './OpenAPIFormRenderer/FormRendererV3';
import HCLView from './HCLView/HCLView';
import { useUIVersion } from '../hooks/useUIVersion';
import { processApiSchema } from '../utils/schemaTypeMapper';
import api from '../services/api';
import styles from './DemoForm.module.css'; // 复用 DemoForm 的样式

interface DemoPreviewProps {
  demo: ModuleDemo;
  moduleId: number;
  onClose: () => void;
}

const DemoPreview: React.FC<DemoPreviewProps> = ({ demo, moduleId, onClose }) => {
  const { isV3 } = useUIVersion();
  const [schema, setSchema] = useState<FormSchema | null>(null);
  const [rawSchema, setRawSchema] = useState<any>(null);
  const [isV2, setIsV2] = useState(false);
  const [dataViewMode, setDataViewMode] = useState<'form' | 'json'>('json');
  const [loading, setLoading] = useState(true);
  
  useEffect(() => {
    loadSchema();
  }, [moduleId]);
  
  const loadSchema = async () => {
    try {
      setLoading(true);
      const response = await api.get(`/modules/${moduleId}/schemas`);
      
      
      // 处理响应数据
      let schemasData = [];
      if (response.data) {
        schemasData = Array.isArray(response.data) ? response.data : [response.data];
      } else if (Array.isArray(response)) {
        schemasData = response;
      }
      
      
      if (schemasData.length > 0) {
        // 查找 active 状态的 schema，如果没有则使用第一个
        let activeSchema = schemasData.find((s: any) => s.status === 'active') || schemasData[0];

        // 检测是否为 v2 schema
        const v2 = activeSchema.schema_version === 'v2' && activeSchema.openapi_schema;
        setIsV2(v2);
        setRawSchema(activeSchema);

        // 如果 schema_data 是字符串，需要解析
        if (typeof activeSchema.schema_data === 'string') {
          try {
            activeSchema.schema_data = JSON.parse(activeSchema.schema_data);
          } catch (e) {
            console.error('Schema 解析错误:', e);
            activeSchema.schema_data = {};
          }
        }

        const processedSchema = processApiSchema(activeSchema);
        setSchema(processedSchema.schema_data);
      }
    } catch (error) {
      console.error('加载 Schema 失败:', error);
    } finally {
      setLoading(false);
    }
  };
  
  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h2>{demo.name} - v{demo.current_version?.version || 1}</h2>
          <button className={styles.closeButton} onClick={onClose}>
            ×
          </button>
        </div>
        
        <div className={styles.form}>
          {/* Demo 元数据 */}
          <div style={{ 
            marginBottom: '20px', 
            padding: '16px', 
            background: 'var(--bg)', 
            borderRadius: '6px',
            border: '1px solid var(--line)'
          }}>
            {demo.description && (
              <div style={{ marginBottom: '12px' }}>
                <strong style={{ color: 'var(--ink-2)', fontSize: '14px' }}>描述：</strong>
                <span style={{ color: 'var(--ink-2)', fontSize: '14px', marginLeft: '8px' }}>
                  {demo.description}
                </span>
              </div>
            )}
            {demo.usage_notes && (
              <div>
                <strong style={{ color: 'var(--ink-2)', fontSize: '14px' }}>使用说明：</strong>
                <span style={{ color: 'var(--ink-2)', fontSize: '14px', marginLeft: '8px' }}>
                  {demo.usage_notes}
                </span>
              </div>
            )}
          </div>
          
          {loading ? (
            <div style={{ textAlign: 'center', padding: '40px', color: 'var(--ink-2)' }}>
              加载 Schema 中...
            </div>
          ) : !schema ? (
            <div style={{ 
              textAlign: 'center', 
              padding: '40px', 
              background: 'var(--amber-soft)',
              borderRadius: '6px',
              color: 'var(--amber-hover)'
            }}>
              <p>该模块暂无 Schema 定义</p>
              <p style={{ fontSize: '14px', marginTop: '8px' }}>
                请先在"Schema管理"中创建 Schema
              </p>
            </div>
          ) : (
            <div>
              <div style={{ 
                display: 'flex', 
                justifyContent: 'space-between', 
                alignItems: 'center', 
                marginBottom: '16px' 
              }}>
                <h3 style={{ margin: 0, fontSize: '16px', fontWeight: 600 }}>
                  配置预览
                </h3>
                <div style={{ display: 'flex', gap: '8px' }}>
                  <button
                    onClick={() => setDataViewMode('form')}
                    style={{
                      padding: '6px 12px',
                      background: dataViewMode === 'form' ? 'var(--brand)' : 'var(--bg)',
                      color: dataViewMode === 'form' ? 'white' : 'var(--ink-2)',
                      border: '1px solid ' + (dataViewMode === 'form' ? 'var(--brand)' : 'var(--line-2)'),
                      borderRadius: '4px',
                      fontSize: '13px',
                      cursor: 'pointer',
                      fontWeight: 500
                    }}
                  >
                    表单视图
                  </button>
                  <button
                    onClick={() => setDataViewMode('json')}
                    style={{
                      padding: '6px 12px',
                      background: dataViewMode === 'json' ? 'var(--brand)' : 'var(--bg)',
                      color: dataViewMode === 'json' ? 'white' : 'var(--ink-2)',
                      border: '1px solid ' + (dataViewMode === 'json' ? 'var(--brand)' : 'var(--line-2)'),
                      borderRadius: '6px',
                      fontSize: '13px',
                      cursor: 'pointer',
                      fontWeight: 500
                    }}
                  >
                    {isV3 ? 'HCL 视图' : 'JSON视图'}
                  </button>
                </div>
              </div>
              
              {dataViewMode === 'json' ? (
                isV3 ? (
                  <HCLView
                    data={demo.current_version?.config_data || {}}
                    moduleName={demo.name?.toLowerCase().replace(/[^a-z0-9_-]/g, '_') || 'demo'}
                  />
                ) : (
                  <div style={{
                    background: 'var(--bg)',
                    border: '1px solid var(--line-2)',
                    borderRadius: '6px',
                    padding: '16px',
                    maxHeight: '500px',
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
                      {JSON.stringify(demo.current_version?.config_data || {}, null, 2)}
                    </pre>
                  </div>
                )
              ) : isV2 && rawSchema?.openapi_schema ? (
                isV3 ? (
                  <FormRendererV3
                    schema={rawSchema.openapi_schema}
                    initialValues={demo.current_version?.config_data || {}}
                    onChange={() => {}}
                    readOnly={true}
                  />
                ) : (
                  <OpenAPIFormRenderer
                    schema={rawSchema.openapi_schema}
                    initialValues={demo.current_version?.config_data || {}}
                    onChange={() => {}}
                    readOnly={true}
                  />
                )
              ) : schema ? (
                <FormPreview
                  schema={schema}
                  values={demo.current_version?.config_data || {}}
                  onClose={() => {}}
                  inline={true}
                  viewMode={dataViewMode}
                  onViewModeChange={setDataViewMode}
                />
              ) : (
                <div style={{ 
                  textAlign: 'center', 
                  padding: '40px', 
                  background: 'var(--amber-soft)',
                  borderRadius: '6px',
                  color: 'var(--amber-hover)'
                }}>
                  <p>Schema 加载失败，无法显示表单视图</p>
                  <p style={{ fontSize: '14px', marginTop: '8px' }}>
                    请切换到 JSON 视图查看配置
                  </p>
                </div>
              )}
            </div>
          )}
          
          <div className={styles.actions}>
            <button
              type="button"
              onClick={onClose}
              className={styles.cancelButton}
            >
              关闭
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default DemoPreview;
