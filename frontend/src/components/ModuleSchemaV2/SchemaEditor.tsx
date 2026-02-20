import React, { useState, useCallback, useEffect } from 'react';
import { Tabs, Button, Space, message, Spin, Modal } from 'antd';
import { SaveOutlined, ReloadOutlined, UploadOutlined, EyeOutlined } from '@ant-design/icons';
import { schemaV2Service, type OpenAPISchema } from '../../services/schemaV2';
import SchemaVisualEditor from './SchemaVisualEditor';
import SchemaJsonEditor from './SchemaJsonEditor';
import SchemaPreview from './SchemaPreview';
import SchemaImportWizard from './SchemaImportWizard';
import styles from './ModuleSchemaV2.module.css';

interface SchemaEditorProps {
  moduleId: number;
  schemaId?: number;
  initialSchema?: OpenAPISchema;
  onSave?: (schema: OpenAPISchema) => void;
  onCancel?: () => void;
  readOnly?: boolean;
}

const SchemaEditor: React.FC<SchemaEditorProps> = ({
  moduleId,
  schemaId,
  initialSchema,
  onSave,
  onCancel,
  readOnly = false,
}) => {
  const [schema, setSchema] = useState<OpenAPISchema | null>(initialSchema || null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [isDirty, setIsDirty] = useState(false);
  const [showImportWizard, setShowImportWizard] = useState(false);
  const [showPreview, setShowPreview] = useState(false);
  const [activeTab, setActiveTab] = useState('visual');

  // 加载 Schema
  useEffect(() => {
    if (schemaId && !initialSchema) {
      loadSchema();
    }
  }, [schemaId]);

  const loadSchema = async () => {
    if (!schemaId) return;
    
    setLoading(true);
    try {
      const response = await schemaV2Service.getSchemaV2(moduleId);
      if (response?.openapi_schema) {
        setSchema(response.openapi_schema);
      }
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } }; message?: string };
      message.error(err.response?.data?.error || '加载 Schema 失败');
    } finally {
      setLoading(false);
    }
  };

  // 处理 Schema 变更
  const handleSchemaChange = useCallback((newSchema: OpenAPISchema) => {
    setSchema(newSchema);
    setIsDirty(true);
  }, []);

  // 保存 Schema
  const handleSave = async () => {
    if (!schema) {
      message.error('Schema 数据为空');
      return;
    }

    setSaving(true);
    try {
      if (schemaId) {
        // 更新现有 Schema
        await schemaV2Service.updateSchemaV2(moduleId, schemaId, {
          openapi_schema: schema,
        });
        message.success('Schema 已保存');
      } else {
        // 创建新 Schema
        await schemaV2Service.createSchemaV2(moduleId, {
          version: schema.info?.version || '1.0.0',
          openapi_schema: schema,
          status: 'active',
          source_type: 'manual',
        });
        message.success('Schema 已创建');
      }
      setIsDirty(false);
      onSave?.(schema);
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } }; message?: string };
      message.error(err.response?.data?.error || '保存失败');
    } finally {
      setSaving(false);
    }
  };

  // 重新加载
  const handleReload = () => {
    if (isDirty) {
      Modal.confirm({
        title: '确认重新加载',
        content: '您有未保存的更改，重新加载将丢失这些更改。确定要继续吗？',
        onOk: loadSchema,
      });
    } else {
      loadSchema();
    }
  };

  // 导入成功回调
  const handleImportSuccess = (_newSchemaId: number) => {
    setShowImportWizard(false);
    // 重新加载
    loadSchema();
  };

  // Tab 配置
  const tabItems = [
    {
      key: 'visual',
      label: '可视化编辑',
      children: schema ? (
        <SchemaVisualEditor
          schema={schema}
          onChange={handleSchemaChange}
          readOnly={readOnly}
        />
      ) : (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <p>暂无 Schema 数据</p>
          {!readOnly && (
            <Button type="primary" icon={<UploadOutlined />} onClick={() => setShowImportWizard(true)}>
              导入 Schema
            </Button>
          )}
        </div>
      ),
    },
    {
      key: 'json',
      label: 'JSON 编辑',
      children: schema ? (
        <SchemaJsonEditor
          schema={schema}
          onChange={handleSchemaChange}
          readOnly={readOnly}
        />
      ) : (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <p>暂无 Schema 数据</p>
        </div>
      ),
    },
  ];

  return (
    <div className={styles.schemaEditorContainer}>
      {/* 头部工具栏 */}
      <div className={styles.schemaEditorHeader}>
        <h3 className={styles.schemaEditorTitle}>
          Schema 编辑器
          {isDirty && <span style={{ color: '#faad14', marginLeft: 8 }}>*</span>}
        </h3>
        <Space>
          {schema && (
            <Button
              icon={<EyeOutlined />}
              onClick={() => setShowPreview(true)}
            >
              预览
            </Button>
          )}
          {!readOnly && (
            <>
              <Button
                icon={<UploadOutlined />}
                onClick={() => setShowImportWizard(true)}
              >
                导入
              </Button>
              {schemaId && (
                <Button
                  icon={<ReloadOutlined />}
                  onClick={handleReload}
                  disabled={loading}
                >
                  重新加载
                </Button>
              )}
            </>
          )}
        </Space>
      </div>

      {/* 编辑器主体 */}
      <Spin spinning={loading}>
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={tabItems}
          className={styles.schemaEditorTabs}
        />
      </Spin>

      {/* 底部操作栏 */}
      {!readOnly && schema && (
        <div className={styles.schemaEditorActions}>
          <Space>
            <Button onClick={onCancel}>取消</Button>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              onClick={handleSave}
              loading={saving}
              disabled={!isDirty}
            >
              保存
            </Button>
          </Space>
        </div>
      )}

      {/* 导入向导弹窗 */}
      <Modal
        title="导入 Schema"
        open={showImportWizard}
        onCancel={() => setShowImportWizard(false)}
        footer={null}
        width={900}
        destroyOnClose
      >
        <SchemaImportWizard
          moduleId={moduleId}
          onSuccess={handleImportSuccess}
          onCancel={() => setShowImportWizard(false)}
        />
      </Modal>

      {/* 预览弹窗 */}
      <Modal
        title="Schema 预览"
        open={showPreview}
        onCancel={() => setShowPreview(false)}
        footer={null}
        width={800}
      >
        <SchemaPreview schema={schema} />
      </Modal>
    </div>
  );
};

export default SchemaEditor;
