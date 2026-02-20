import React, { useState, useEffect, useRef } from 'react';
import { moduleDemoService, type ModuleDemo } from '../services/moduleDemos';
import { ModuleFormRenderer } from './ModuleFormRenderer';
import { schemaV2Service, type OpenAPISchema } from '../services/schemaV2';
import styles from './DemoForm.module.css';

interface DemoFormProps {
  moduleId: number;
  demo?: ModuleDemo;
  onSave: () => void;
  onCancel: () => void;
}

const DemoForm: React.FC<DemoFormProps> = ({
  moduleId,
  demo,
  onSave,
  onCancel,
}) => {
  const [formData, setFormData] = useState({
    name: demo?.name || '',
    description: demo?.description || '',
    usage_notes: demo?.usage_notes || '',
    config_data: demo?.current_version?.config_data || {},
    change_summary: '',
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [jsonError, setJsonError] = useState<string | null>(null);
  
  // Schema 相关状态
  const [schema, setSchema] = useState<OpenAPISchema | null>(null);
  const [schemaLoading, setSchemaLoading] = useState(true);
  const [useJsonEditor, setUseJsonEditor] = useState(false);
  
  // 用于追踪 mousedown 是否发生在 overlay 上
  const mouseDownOnOverlayRef = useRef(false);

  const isEditMode = !!demo;

  // 加载 Schema
  useEffect(() => {
    loadSchema();
  }, [moduleId]);

  const loadSchema = async () => {
    try {
      setSchemaLoading(true);
      const schemaData = await schemaV2Service.getSchemaV2(moduleId);
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

  const validateJSON = (jsonString: string): boolean => {
    try {
      JSON.parse(jsonString);
      setJsonError(null);
      return true;
    } catch (err: any) {
      setJsonError(err.message);
      return false;
    }
  };

  const handleConfigDataChange = (value: string) => {
    try {
      const parsed = JSON.parse(value);
      setFormData({ ...formData, config_data: parsed });
      setJsonError(null);
    } catch (err: any) {
      setJsonError(err.message);
    }
  };

  // 处理表单值变化（来自 ModuleFormRenderer）
  const handleFormChange = (values: Record<string, unknown>) => {
    setFormData({ ...formData, config_data: values });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    // 验证必填字段
    if (!formData.name.trim()) {
      setError('Name is required');
      return;
    }

    // 编辑模式下验证 change_summary
    if (isEditMode && !formData.change_summary.trim()) {
      setError('Change summary is required when updating');
      return;
    }

    try {
      setLoading(true);

      if (isEditMode && demo) {
        // 更新
        await moduleDemoService.updateDemo(demo.id, {
          name: formData.name,
          description: formData.description,
          usage_notes: formData.usage_notes,
          config_data: formData.config_data,
          change_summary: formData.change_summary,
        });
      } else {
        // 创建
        await moduleDemoService.createDemo(moduleId, {
          name: formData.name,
          description: formData.description,
          usage_notes: formData.usage_notes,
          config_data: formData.config_data,
        });
      }

      onSave();
    } catch (err: any) {
      setError(err.response?.data?.error || 'Failed to save demo');
      console.error('Error saving demo:', err);
    } finally {
      setLoading(false);
    }
  };

  const formatJSON = () => {
    try {
      const formatted = JSON.stringify(formData.config_data, null, 2);
      setJsonError(null);
      return formatted;
    } catch (err: any) {
      setJsonError(err.message);
      return '';
    }
  };

  // 处理 overlay 的 mousedown 事件
  const handleOverlayMouseDown = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      mouseDownOnOverlayRef.current = true;
    } else {
      mouseDownOnOverlayRef.current = false;
    }
  };

  // 处理 overlay 的 mouseup 事件
  const handleOverlayMouseUp = (e: React.MouseEvent) => {
    if (mouseDownOnOverlayRef.current && e.target === e.currentTarget) {
      onCancel();
    }
    mouseDownOnOverlayRef.current = false;
  };

  return (
    <div 
      className={styles.overlay} 
      onMouseDown={handleOverlayMouseDown}
      onMouseUp={handleOverlayMouseUp}
    >
      <div className={styles.modal}>
        <div className={styles.header}>
          <h2>{isEditMode ? 'Edit Demo' : 'Create Demo'}</h2>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <button
              type="button"
              onClick={() => setUseJsonEditor(!useJsonEditor)}
              className={styles.toggleButton}
              style={{
                padding: '4px 10px',
                background: useJsonEditor ? '#1890ff' : '#f0f0f0',
                color: useJsonEditor ? '#fff' : '#666',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer',
                fontSize: '12px',
              }}
            >
              {useJsonEditor ? '表单模式' : 'JSON 模式'}
            </button>
            <button className={styles.closeButton} onClick={onCancel}>
              ×
            </button>
          </div>
        </div>

        <form onSubmit={handleSubmit} className={styles.form}>
          {error && <div className={styles.error}>{error}</div>}

          <div className={styles.formGroup}>
            <label htmlFor="name">
              Name <span className={styles.required}>*</span>
            </label>
            <input
              id="name"
              type="text"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="e.g., Production Config"
              required
            />
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="description">Description</label>
            <textarea
              id="description"
              value={formData.description}
              onChange={(e) =>
                setFormData({ ...formData, description: e.target.value })
              }
              placeholder="Describe this demo configuration..."
              rows={2}
            />
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="usage_notes">Usage Notes</label>
            <textarea
              id="usage_notes"
              value={formData.usage_notes}
              onChange={(e) =>
                setFormData({ ...formData, usage_notes: e.target.value })
              }
              placeholder="How to use this configuration..."
              rows={2}
            />
          </div>

          <div className={styles.formGroup}>
            <div className={styles.labelRow}>
              <label>
                Config Data <span className={styles.required}>*</span>
              </label>
              {useJsonEditor && (
                <button
                  type="button"
                  onClick={() => {
                    const formatted = formatJSON();
                    // 触发重新渲染
                    setFormData({ ...formData });
                  }}
                  className={styles.formatButton}
                >
                  Format JSON
                </button>
              )}
            </div>
            
            {schemaLoading ? (
              <div style={{ padding: '20px', textAlign: 'center', color: '#8c8c8c' }}>
                加载 Schema 中...
              </div>
            ) : useJsonEditor || !schema ? (
              <>
                <textarea
                  id="config_data"
                  value={JSON.stringify(formData.config_data, null, 2)}
                  onChange={(e) => handleConfigDataChange(e.target.value)}
                  placeholder='{"key": "value"}'
                  rows={12}
                  className={styles.codeEditor}
                  spellCheck={false}
                  required
                />
                {jsonError && <div className={styles.jsonError}>{jsonError}</div>}
              </>
            ) : (
              <div style={{ 
                border: '1px solid #d9d9d9', 
                borderRadius: '6px', 
                padding: '12px',
                background: '#fafafa',
                maxHeight: '400px',
                overflow: 'auto'
              }}>
                <ModuleFormRenderer
                  schema={schema}
                  initialValues={formData.config_data}
                  onChange={handleFormChange}
                  showVersionBadge={false}
                />
              </div>
            )}
          </div>

          {isEditMode && (
            <div className={styles.formGroup}>
              <label htmlFor="change_summary">
                Change Summary <span className={styles.required}>*</span>
              </label>
              <input
                id="change_summary"
                type="text"
                value={formData.change_summary}
                onChange={(e) =>
                  setFormData({ ...formData, change_summary: e.target.value })
                }
                placeholder="Describe what changed..."
                required
              />
              <small className={styles.hint}>
                This will be recorded in the version history
              </small>
            </div>
          )}

          <div className={styles.actions}>
            <button
              type="button"
              onClick={onCancel}
              className={styles.cancelButton}
              disabled={loading}
            >
              Cancel
            </button>
            <button
              type="submit"
              className={styles.submitButton}
              disabled={loading || (useJsonEditor && !!jsonError)}
            >
              {loading ? 'Saving...' : isEditMode ? 'Update Demo' : 'Create Demo'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default DemoForm;
