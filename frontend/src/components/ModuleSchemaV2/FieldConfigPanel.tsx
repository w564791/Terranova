import React, { useState } from 'react';
import { Form, Input, Select, Tag, Button, Space, Tooltip } from 'antd';
import { EditOutlined, CheckOutlined, CloseOutlined } from '@ant-design/icons';
import type { OpenAPISchema, PropertySchema, FieldUIConfig } from '../../services/schemaV2';
import { extractFieldsFromSchema, getWidgetType } from '../../services/schemaV2';
import styles from './ModuleSchemaV2.module.css';

const { Option } = Select;

interface FieldConfigPanelProps {
  schema: OpenAPISchema;
  onFieldUpdate?: (fieldName: string, property: string, value: unknown) => void;
}

const WIDGET_OPTIONS = [
  { value: 'text', label: '文本输入' },
  { value: 'textarea', label: '多行文本' },
  { value: 'number', label: '数字输入' },
  { value: 'select', label: '下拉选择' },
  { value: 'switch', label: '开关' },
  { value: 'tags', label: '标签' },
  { value: 'key-value', label: '键值对' },
  { value: 'object', label: '对象' },
  { value: 'object-list', label: '对象列表' },
  { value: 'json-editor', label: 'JSON编辑器' },
];

const GROUP_OPTIONS = [
  { value: 'basic', label: '基础配置' },
  { value: 'advanced', label: '高级配置' },
];

const FieldConfigPanel: React.FC<FieldConfigPanelProps> = ({ schema, onFieldUpdate }) => {
  const [editingField, setEditingField] = useState<string | null>(null);
  const [form] = Form.useForm();

  const fields = extractFieldsFromSchema(schema);
  const required = schema.components?.schemas?.ModuleInput?.required || [];

  const handleEdit = (fieldName: string, uiConfig: FieldUIConfig) => {
    setEditingField(fieldName);
    form.setFieldsValue({
      label: uiConfig.label || '',
      group: uiConfig.group || 'advanced',
      widget: uiConfig.widget || 'text',
      help: uiConfig.help || '',
      placeholder: uiConfig.placeholder || '',
    });
  };

  const handleSave = async () => {
    if (!editingField) return;
    
    try {
      const values = await form.validateFields();
      Object.entries(values).forEach(([property, value]) => {
        if (value !== undefined && value !== '') {
          onFieldUpdate?.(editingField, property, value);
        }
      });
      setEditingField(null);
    } catch (error) {
      console.error('Validation failed:', error);
    }
  };

  const handleCancel = () => {
    setEditingField(null);
    form.resetFields();
  };

  const renderFieldItem = (field: {
    name: string;
    property: PropertySchema;
    uiConfig: FieldUIConfig;
  }) => {
    const isEditing = editingField === field.name;
    const isRequired = required.includes(field.name);
    const widget = getWidgetType(field.property, field.uiConfig);
    const group = field.uiConfig.group || 'advanced';

    return (
      <div
        key={field.name}
        className={`${styles.fieldItem} ${isEditing ? styles.selected : ''}`}
      >
        <div className={styles.fieldInfo}>
          <div className={styles.fieldName}>
            {field.uiConfig.label || field.name}
            {isRequired && <span style={{ color: '#ff4d4f', marginLeft: 4 }}>*</span>}
          </div>
          <div className={styles.fieldType}>
            {field.name} · {field.property.type}
            {field.property.description && ` · ${field.property.description}`}
          </div>
        </div>
        
        <div className={styles.fieldBadges}>
          <Tag className={`${styles.groupTag} ${group === 'basic' ? styles.basic : styles.advanced}`}>
            {group === 'basic' ? '基础' : '高级'}
          </Tag>
          <Tag color="blue">{widget}</Tag>
        </div>

        <div className={styles.fieldActions}>
          {isEditing ? (
            <Space>
              <Button
                type="primary"
                size="small"
                icon={<CheckOutlined />}
                onClick={handleSave}
              />
              <Button
                size="small"
                icon={<CloseOutlined />}
                onClick={handleCancel}
              />
            </Space>
          ) : (
            <Tooltip title="编辑配置">
              <Button
                type="text"
                size="small"
                icon={<EditOutlined />}
                onClick={() => handleEdit(field.name, field.uiConfig)}
              />
            </Tooltip>
          )}
        </div>
      </div>
    );
  };

  const renderEditForm = () => {
    if (!editingField) return null;

    return (
      <div className={styles.fieldEditForm}>
        <Form form={form} layout="vertical" size="small">
          <Form.Item name="label" label="显示标签">
            <Input placeholder="字段显示名称" />
          </Form.Item>
          
          <Form.Item name="group" label="所属分组">
            <Select>
              {GROUP_OPTIONS.map(opt => (
                <Option key={opt.value} value={opt.value}>{opt.label}</Option>
              ))}
            </Select>
          </Form.Item>
          
          <Form.Item name="widget" label="组件类型">
            <Select>
              {WIDGET_OPTIONS.map(opt => (
                <Option key={opt.value} value={opt.value}>{opt.label}</Option>
              ))}
            </Select>
          </Form.Item>
          
          <Form.Item name="help" label="帮助文本">
            <Input.TextArea rows={2} placeholder="字段说明或帮助信息" />
          </Form.Item>
          
          <Form.Item name="placeholder" label="占位符">
            <Input placeholder="输入框占位符文本" />
          </Form.Item>
        </Form>
      </div>
    );
  };

  // 按分组排序字段
  const basicFields = fields.filter(f => f.uiConfig.group === 'basic');
  const advancedFields = fields.filter(f => f.uiConfig.group !== 'basic');

  return (
    <div className={styles.fieldConfigPanel}>
      {basicFields.length > 0 && (
        <>
          <h4>基础配置 ({basicFields.length})</h4>
          <div className={styles.fieldList}>
            {basicFields.map(renderFieldItem)}
          </div>
        </>
      )}
      
      {advancedFields.length > 0 && (
        <>
          <h4>高级配置 ({advancedFields.length})</h4>
          <div className={styles.fieldList}>
            {advancedFields.map(renderFieldItem)}
          </div>
        </>
      )}

      {renderEditForm()}
    </div>
  );
};

export default FieldConfigPanel;
