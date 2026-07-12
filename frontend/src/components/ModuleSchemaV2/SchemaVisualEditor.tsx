import React, { useState, useCallback, useMemo } from 'react';
import { Card, Form, Input, Select, Switch, Button, Space, Collapse, Tooltip, Tag, Empty, Popconfirm } from 'antd';
import { 
  PlusOutlined, 
  DeleteOutlined, 
  EditOutlined,
  HolderOutlined,
} from '@ant-design/icons';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import type { DragEndEvent } from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import type { OpenAPISchema, PropertySchema, FieldUIConfig } from '../../services/schemaV2';
import { extractFieldsFromSchema, getWidgetType } from '../../services/schemaV2';
import styles from './ModuleSchemaV2.module.css';

const { Option } = Select;
const { Panel } = Collapse;

interface SchemaVisualEditorProps {
  schema: OpenAPISchema;
  onChange: (schema: OpenAPISchema) => void;
  readOnly?: boolean;
}

interface FieldData {
  name: string;
  property: PropertySchema;
  uiConfig: FieldUIConfig;
}

const WIDGET_OPTIONS = [
  { value: 'text', label: '文本输入', icon: '📝' },
  { value: 'textarea', label: '多行文本', icon: '📄' },
  { value: 'number', label: '数字输入', icon: '🔢' },
  { value: 'select', label: '下拉选择', icon: '📋' },
  { value: 'multi-select', label: '多选', icon: '☑️' },
  { value: 'switch', label: '开关', icon: '🔘' },
  { value: 'tags', label: '标签', icon: '🏷️' },
  { value: 'key-value', label: '键值对', icon: '🔑' },
  { value: 'object', label: '对象', icon: '📦' },
  { value: 'object-list', label: '对象列表', icon: '📚' },
  { value: 'json-editor', label: 'JSON编辑器', icon: '{ }' },
  { value: 'password', label: '密码', icon: '🔒' },
];

const TYPE_OPTIONS = [
  { value: 'string', label: '字符串' },
  { value: 'number', label: '数字' },
  { value: 'integer', label: '整数' },
  { value: 'boolean', label: '布尔值' },
  { value: 'array', label: '数组' },
  { value: 'object', label: '对象' },
];

// 可排序的字段项组件
interface SortableFieldItemProps {
  field: FieldData;
  isRequired: boolean;
  isEditing: boolean;
  readOnly: boolean;
  groups: Array<{ id: string; title: string; order: number; defaultExpanded?: boolean }>;
  form: any;
  onEdit: () => void;
  onSave: () => void;
  onCancel: () => void;
  onToggleRequired: () => void;
  onDelete: () => void;
}

const SortableFieldItem: React.FC<SortableFieldItemProps> = ({
  field,
  isRequired,
  isEditing,
  readOnly,
  groups,
  form,
  onEdit,
  onSave,
  onCancel,
  onToggleRequired,
  onDelete,
}) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: field.name });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  const widget = getWidgetType(field.property, field.uiConfig);
  const order = field.uiConfig.order ?? 999;

  if (isEditing) {
    return (
      <div ref={setNodeRef} style={style} className={styles.fieldEditForm}>
        <Form
          form={form}
          layout="vertical"
          size="small"
          initialValues={{
            label: field.uiConfig.label || field.name,
            group: field.uiConfig.group || 'advanced',
            widget: field.uiConfig.widget || widget,
            description: field.property.description || '',
            placeholder: field.uiConfig.placeholder || '',
            default: field.property.default,
          }}
        >
          <Form.Item name="label" label="显示标签" rules={[{ required: true }]}>
            <Input placeholder="字段显示名称" />
          </Form.Item>
          
          <Form.Item name="group" label="所属分组">
            <Select>
              {groups.map(g => (
                <Option key={g.id} value={g.id}>{g.title}</Option>
              ))}
            </Select>
          </Form.Item>
          
          <Form.Item name="widget" label="组件类型">
            <Select>
              {WIDGET_OPTIONS.map(opt => (
                <Option key={opt.value} value={opt.value}>
                  {opt.icon} {opt.label}
                </Option>
              ))}
            </Select>
          </Form.Item>
          
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="字段描述" />
          </Form.Item>
          
          <Form.Item name="placeholder" label="占位符">
            <Input placeholder="输入框占位符" />
          </Form.Item>
          
          <Form.Item name="default" label="默认值">
            <Input placeholder="默认值" />
          </Form.Item>
          
          <Space>
            <Button type="primary" onClick={onSave}>保存</Button>
            <Button onClick={onCancel}>取消</Button>
          </Space>
        </Form>
      </div>
    );
  }

  return (
    <div ref={setNodeRef} style={style} className={styles.fieldItem}>
      {!readOnly && (
        <div 
          className={styles.dragHandle} 
          {...attributes} 
          {...listeners}
          style={{ 
            cursor: 'grab', 
            padding: '0 8px',
            display: 'flex',
            alignItems: 'center',
            color: 'var(--ink-faint)',
          }}
        >
          <HolderOutlined />
        </div>
      )}
      
      <div className={styles.fieldInfo} style={{ flex: 1 }}>
        <div className={styles.fieldName}>
          {field.uiConfig.label || field.name}
          {isRequired && <span style={{ color: 'var(--red)', marginLeft: 4 }}>*</span>}
        </div>
        <div className={styles.fieldType}>
          <code>{field.name}</code> · {field.property.type}
          {field.property.description && (
            <span style={{ marginLeft: 8, color: 'var(--ink-faint)' }}>
              {field.property.description.substring(0, 50)}
              {field.property.description.length > 50 ? '...' : ''}
            </span>
          )}
        </div>
      </div>
      
      <div className={styles.fieldBadges}>
        <Tooltip title={`排序: ${order}`}>
          <Tag color="default" style={{ fontSize: 11 }}>#{order}</Tag>
        </Tooltip>
        <Tag color={widget === 'select' ? 'blue' : 'default'}>{widget}</Tag>
      </div>

      {!readOnly && (
        <div className={styles.fieldActions}>
          <Space size="small">
            <Tooltip title={isRequired ? '取消必填' : '设为必填'}>
              <Button
                type="text"
                size="small"
                onClick={onToggleRequired}
                style={{ color: isRequired ? 'var(--red)' : undefined }}
              >
                {isRequired ? '必填' : '可选'}
              </Button>
            </Tooltip>
            <Tooltip title="编辑">
              <Button
                type="text"
                size="small"
                icon={<EditOutlined />}
                onClick={onEdit}
              />
            </Tooltip>
            <Popconfirm
              title="确定删除此字段？"
              onConfirm={onDelete}
              okText="删除"
              cancelText="取消"
            >
              <Tooltip title="删除">
                <Button
                  type="text"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                />
              </Tooltip>
            </Popconfirm>
          </Space>
        </div>
      )}
    </div>
  );
};

const SchemaVisualEditor: React.FC<SchemaVisualEditorProps> = ({
  schema,
  onChange,
  readOnly = false,
}) => {
  const [editingField, setEditingField] = useState<string | null>(null);
  const [showAddField, setShowAddField] = useState(false);
  const [form] = Form.useForm();
  const [addFieldForm] = Form.useForm();

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  const fields = extractFieldsFromSchema(schema);
  const groups = schema['x-iac-platform']?.ui?.groups || [
    { id: 'basic', title: '基础配置', order: 1, defaultExpanded: true },
    { id: 'advanced', title: '高级配置', order: 2, defaultExpanded: false },
  ];
  const required = schema.components?.schemas?.ModuleInput?.required || [];

  // 按分组和 order 组织字段
  const groupedFields = useMemo(() => {
    return groups.map(group => {
      const groupFields = fields
        .filter(f => (f.uiConfig.group || 'advanced') === group.id)
        .sort((a, b) => (a.uiConfig.order ?? 999) - (b.uiConfig.order ?? 999));
      return {
        ...group,
        fields: groupFields,
      };
    });
  }, [groups, fields]);

  // 更新字段属性
  const updateFieldProperty = useCallback((fieldName: string, updates: Partial<PropertySchema>) => {
    const newSchema = { ...schema };
    const properties = newSchema.components?.schemas?.ModuleInput?.properties;
    if (properties && properties[fieldName]) {
      properties[fieldName] = { ...properties[fieldName], ...updates };
    }
    onChange(newSchema);
  }, [schema, onChange]);

  // 更新字段 UI 配置
  const updateFieldUI = useCallback((fieldName: string, updates: Partial<FieldUIConfig>) => {
    const newSchema = { ...schema };
    const uiFields = newSchema['x-iac-platform']?.ui?.fields;
    if (uiFields) {
      uiFields[fieldName] = { ...uiFields[fieldName], ...updates };
    }
    onChange(newSchema);
  }, [schema, onChange]);

  // 批量更新字段顺序
  const updateFieldsOrder = useCallback((groupId: string, orderedFieldNames: string[]) => {
    const newSchema = JSON.parse(JSON.stringify(schema)); // 深拷贝
    
    if (!newSchema['x-iac-platform']) {
      newSchema['x-iac-platform'] = { ui: { fields: {}, groups: [] } };
    }
    if (!newSchema['x-iac-platform'].ui) {
      newSchema['x-iac-platform'].ui = { fields: {}, groups: [] };
    }
    if (!newSchema['x-iac-platform'].ui.fields) {
      newSchema['x-iac-platform'].ui.fields = {};
    }

    // 计算基础 order 值（基础配置从 1 开始，高级配置从 100 开始）
    const baseOrder = groupId === 'basic' ? 1 : 100;
    
    orderedFieldNames.forEach((fieldName, index) => {
      if (!newSchema['x-iac-platform'].ui.fields[fieldName]) {
        newSchema['x-iac-platform'].ui.fields[fieldName] = {};
      }
      newSchema['x-iac-platform'].ui.fields[fieldName].order = baseOrder + index;
    });

    onChange(newSchema);
  }, [schema, onChange]);

  // 处理拖拽结束
  const handleDragEnd = useCallback((event: DragEndEvent, groupId: string, groupFields: FieldData[]) => {
    const { active, over } = event;

    if (over && active.id !== over.id) {
      const oldIndex = groupFields.findIndex(f => f.name === active.id);
      const newIndex = groupFields.findIndex(f => f.name === over.id);

      if (oldIndex !== -1 && newIndex !== -1) {
        const newOrder = arrayMove(groupFields, oldIndex, newIndex);
        updateFieldsOrder(groupId, newOrder.map(f => f.name));
      }
    }
  }, [updateFieldsOrder]);

  // 删除字段
  const deleteField = useCallback((fieldName: string) => {
    const newSchema = { ...schema };
    const properties = newSchema.components?.schemas?.ModuleInput?.properties;
    if (properties) {
      delete properties[fieldName];
    }
    // 从 required 中移除
    const requiredList = newSchema.components?.schemas?.ModuleInput?.required;
    if (requiredList) {
      const index = requiredList.indexOf(fieldName);
      if (index > -1) {
        requiredList.splice(index, 1);
      }
    }
    // 从 UI 配置中移除
    const uiFields = newSchema['x-iac-platform']?.ui?.fields;
    if (uiFields) {
      delete uiFields[fieldName];
    }
    onChange(newSchema);
  }, [schema, onChange]);

  // 切换必填状态
  const toggleRequired = useCallback((fieldName: string) => {
    const newSchema = { ...schema };
    const requiredList = newSchema.components?.schemas?.ModuleInput?.required || [];
    const index = requiredList.indexOf(fieldName);
    if (index > -1) {
      requiredList.splice(index, 1);
    } else {
      requiredList.push(fieldName);
    }
    if (newSchema.components?.schemas?.ModuleInput) {
      newSchema.components.schemas.ModuleInput.required = requiredList;
    }
    onChange(newSchema);
  }, [schema, onChange]);

  // 添加新字段
  const handleAddField = useCallback(async () => {
    try {
      const values = await addFieldForm.validateFields();
      const newSchema = JSON.parse(JSON.stringify(schema)); // 深拷贝
      
      // 添加到 properties
      if (!newSchema.components?.schemas?.ModuleInput?.properties) {
        if (!newSchema.components) newSchema.components = { schemas: { ModuleInput: { type: 'object', properties: {} } } };
        if (!newSchema.components.schemas) newSchema.components.schemas = { ModuleInput: { type: 'object', properties: {} } };
        if (!newSchema.components.schemas.ModuleInput) newSchema.components.schemas.ModuleInput = { type: 'object', properties: {} };
        if (!newSchema.components.schemas.ModuleInput.properties) newSchema.components.schemas.ModuleInput.properties = {};
      }
      
      newSchema.components.schemas.ModuleInput.properties[values.name] = {
        type: values.type,
        title: values.label || values.name,
        description: values.description || '',
        default: values.default,
      };

      // 添加到 UI 配置
      if (!newSchema['x-iac-platform']) newSchema['x-iac-platform'] = { ui: { fields: {}, groups: [] } };
      if (!newSchema['x-iac-platform'].ui) newSchema['x-iac-platform'].ui = { fields: {}, groups: [] };
      if (!newSchema['x-iac-platform'].ui.fields) newSchema['x-iac-platform'].ui.fields = {};
      
      // 计算新字段的 order（放在该分组的最后）
      const targetGroup = values.group || 'advanced';
      const existingFieldsInGroup = fields.filter(f => (f.uiConfig.group || 'advanced') === targetGroup);
      const maxOrder = existingFieldsInGroup.reduce((max, f) => Math.max(max, f.uiConfig.order ?? 0), 0);
      const baseOrder = targetGroup === 'basic' ? 1 : 100;
      const newOrder = Math.max(baseOrder, maxOrder + 1);
      
      newSchema['x-iac-platform'].ui.fields[values.name] = {
        label: values.label || values.name,
        group: targetGroup,
        widget: values.widget || 'text',
        help: values.description || '',
        order: newOrder,
      };

      // 如果必填，添加到 required
      if (values.required) {
        if (!newSchema.components.schemas.ModuleInput.required) {
          newSchema.components.schemas.ModuleInput.required = [];
        }
        newSchema.components.schemas.ModuleInput.required.push(values.name);
      }

      onChange(newSchema);
      setShowAddField(false);
      addFieldForm.resetFields();
    } catch (error) {
      console.error('Validation failed:', error);
    }
  }, [schema, onChange, addFieldForm, fields]);

  // 保存字段编辑
  const handleSaveField = useCallback(async () => {
    if (!editingField) return;
    
    try {
      const values = await form.validateFields();
      
      // 更新属性
      updateFieldProperty(editingField, {
        title: values.label,
        description: values.description,
        default: values.default,
      });
      
      // 更新 UI 配置
      updateFieldUI(editingField, {
        label: values.label,
        group: values.group,
        widget: values.widget,
        help: values.description,
        placeholder: values.placeholder,
      });

      setEditingField(null);
    } catch (error) {
      console.error('Validation failed:', error);
    }
  }, [editingField, form, updateFieldProperty, updateFieldUI]);

  // 渲染添加字段表单
  const renderAddFieldForm = () => {
    if (!showAddField) return null;

    return (
      <Card size="small" title="添加新字段" style={{ marginBottom: 16 }}>
        <Form form={addFieldForm} layout="vertical" size="small">
          <Form.Item 
            name="name" 
            label="字段名称" 
            rules={[
              { required: true, message: '请输入字段名称' },
              { pattern: /^[a-z][a-z0-9_]*$/, message: '字段名必须以小写字母开头，只能包含小写字母、数字和下划线' }
            ]}
          >
            <Input placeholder="例如: instance_type" />
          </Form.Item>
          
          <Form.Item name="label" label="显示标签">
            <Input placeholder="例如: 实例类型" />
          </Form.Item>
          
          <Form.Item name="type" label="数据类型" rules={[{ required: true }]} initialValue="string">
            <Select>
              {TYPE_OPTIONS.map(opt => (
                <Option key={opt.value} value={opt.value}>{opt.label}</Option>
              ))}
            </Select>
          </Form.Item>
          
          <Form.Item name="widget" label="组件类型" initialValue="text">
            <Select>
              {WIDGET_OPTIONS.map(opt => (
                <Option key={opt.value} value={opt.value}>
                  {opt.icon} {opt.label}
                </Option>
              ))}
            </Select>
          </Form.Item>
          
          <Form.Item name="group" label="所属分组" initialValue="basic">
            <Select>
              {groups.map(g => (
                <Option key={g.id} value={g.id}>{g.title}</Option>
              ))}
            </Select>
          </Form.Item>
          
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="字段描述" />
          </Form.Item>
          
          <Form.Item name="required" label="是否必填" valuePropName="checked">
            <Switch />
          </Form.Item>
          
          <Space>
            <Button type="primary" onClick={handleAddField}>添加</Button>
            <Button onClick={() => { setShowAddField(false); addFieldForm.resetFields(); }}>取消</Button>
          </Space>
        </Form>
      </Card>
    );
  };

  if (fields.length === 0 && !showAddField) {
    return (
      <Empty description="暂无字段">
        {!readOnly && (
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setShowAddField(true)}>
            添加字段
          </Button>
        )}
      </Empty>
    );
  }

  return (
    <div className={styles.fieldConfigPanel}>
      {!readOnly && (
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Button 
            type="primary" 
            icon={<PlusOutlined />} 
            onClick={() => setShowAddField(true)}
            disabled={showAddField}
          >
            添加字段
          </Button>
          <span style={{ color: 'var(--ink-3)', fontSize: 13 }}>
            💡 拖拽字段可调整顺序
          </span>
        </div>
      )}

      {renderAddFieldForm()}

      <Collapse 
        defaultActiveKey={groups.filter(g => g.defaultExpanded).map(g => g.id)}
        expandIconPosition="start"
      >
        {groupedFields.map(group => (
          <Panel
            key={group.id}
            header={
              <span>
                {group.title}
                <Tag style={{ marginLeft: 8 }}>{group.fields.length}</Tag>
              </span>
            }
          >
            {group.fields.length > 0 ? (
              <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={(event) => handleDragEnd(event, group.id, group.fields)}
              >
                <SortableContext
                  items={group.fields.map(f => f.name)}
                  strategy={verticalListSortingStrategy}
                >
                  <div className={styles.fieldList}>
                    {group.fields.map(field => (
                      <SortableFieldItem
                        key={field.name}
                        field={field}
                        isRequired={required.includes(field.name)}
                        isEditing={editingField === field.name}
                        readOnly={readOnly}
                        groups={groups}
                        form={form}
                        onEdit={() => {
                          setEditingField(field.name);
                          const widget = getWidgetType(field.property, field.uiConfig);
                          form.setFieldsValue({
                            label: field.uiConfig.label || field.name,
                            group: field.uiConfig.group || 'advanced',
                            widget: field.uiConfig.widget || widget,
                            description: field.property.description || '',
                            placeholder: field.uiConfig.placeholder || '',
                            default: field.property.default,
                          });
                        }}
                        onSave={handleSaveField}
                        onCancel={() => setEditingField(null)}
                        onToggleRequired={() => toggleRequired(field.name)}
                        onDelete={() => deleteField(field.name)}
                      />
                    ))}
                  </div>
                </SortableContext>
              </DndContext>
            ) : (
              <Empty description="此分组暂无字段" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Panel>
        ))}
      </Collapse>
    </div>
  );
};

export default SchemaVisualEditor;
