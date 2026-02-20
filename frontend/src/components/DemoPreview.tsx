import React, { useState, useEffect } from 'react';
import { type ModuleDemo } from '../services/moduleDemos';
import { FormPreview } from '../components/DynamicForm';
import type { FormSchema } from '../components/DynamicForm';
import { processApiSchema } from '../utils/schemaTypeMapper';
import api from '../services/api';
import styles from './DemoForm.module.css'; // 复用 DemoForm 的样式

interface DemoPreviewProps {
  demo: ModuleDemo;
  moduleId: number;
  onClose: () => void;
}

const DemoPreview: React.FC<DemoPreviewProps> = ({ demo, moduleId, onClose }) => {
  const [schema, setSchema] = useState<FormSchema | null>(null);
  const [dataViewMode, setDataViewMode] = useState<'form' | 'json'>('json'); // 默认使用 JSON 视图
  const [loading, setLoading] = useState(true);
  
  useEffect(() => {
    loadSchema();
  }, [moduleId]);
  
  const loadSchema = async () => {
    try {
      setLoading(true);
      const response = await api.get(`/modules/${moduleId}/schemas`);
      
      console.log('📋 Schema response:', response);
      
      // 处理响应数据
      let schemasData = [];
      if (response.data) {
        schemasData = Array.isArray(response.data) ? response.data : [response.data];
      } else if (Array.isArray(response)) {
        schemasData = response;
      }
      
      console.log('📋 Schemas data:', schemasData);
      
      if (schemasData.length > 0) {
        // 查找 active 状态的 schema，如果没有则使用第一个
        let activeSchema = schemasData.find((s: any) => s.status === 'active') || schemasData[0];
        
        console.log('📋 Active schema:', activeSchema);
        
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
        console.log('📋 Processed schema:', processedSchema);
        console.log('📋 Schema data:', processedSchema.schema_data);
        
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
            background: '#f8f9fa', 
            borderRadius: '6px',
            border: '1px solid #e9ecef'
          }}>
            {demo.description && (
              <div style={{ marginBottom: '12px' }}>
                <strong style={{ color: '#495057', fontSize: '14px' }}>描述：</strong>
                <span style={{ color: '#6c757d', fontSize: '14px', marginLeft: '8px' }}>
                  {demo.description}
                </span>
              </div>
            )}
            {demo.usage_notes && (
              <div>
                <strong style={{ color: '#495057', fontSize: '14px' }}>使用说明：</strong>
                <span style={{ color: '#6c757d', fontSize: '14px', marginLeft: '8px' }}>
                  {demo.usage_notes}
                </span>
              </div>
            )}
          </div>
          
          {loading ? (
            <div style={{ textAlign: 'center', padding: '40px', color: '#6c757d' }}>
              加载 Schema 中...
            </div>
          ) : !schema ? (
            <div style={{ 
              textAlign: 'center', 
              padding: '40px', 
              background: '#fff3cd',
              borderRadius: '6px',
              color: '#856404'
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
                      background: dataViewMode === 'form' ? '#007bff' : '#f8f9fa',
                      color: dataViewMode === 'form' ? 'white' : '#495057',
                      border: '1px solid ' + (dataViewMode === 'form' ? '#007bff' : '#dee2e6'),
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
                      background: dataViewMode === 'json' ? '#007bff' : '#f8f9fa',
                      color: dataViewMode === 'json' ? 'white' : '#495057',
                      border: '1px solid ' + (dataViewMode === 'json' ? '#007bff' : '#dee2e6'),
                      borderRadius: '4px',
                      fontSize: '13px',
                      cursor: 'pointer',
                      fontWeight: 500
                    }}
                  >
                    JSON视图
                  </button>
                </div>
              </div>
              
              {dataViewMode === 'json' ? (
                <div style={{
                  background: '#f8f9fa',
                  border: '1px solid #dee2e6',
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
                  background: '#fff3cd',
                  borderRadius: '6px',
                  color: '#856404'
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
