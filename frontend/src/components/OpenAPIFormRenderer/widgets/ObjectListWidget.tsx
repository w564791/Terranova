import React, { useMemo, useCallback, useEffect, useState } from 'react';
import { Form, Card, Button, Space, Tag, Tooltip, Empty, Collapse, Tabs } from 'antd';
import { PlusOutlined, DeleteOutlined, CopyOutlined, LinkOutlined, SettingOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { getWidget } from './index';

const { Panel } = Collapse;

// 扩展的属性 Schema 类型
interface ExtendedPropertySchema {
  type?: string;
  title?: string;
  description?: string;
  default?: unknown;
  enum?: string[];
  format?: string;
  items?: ExtendedPropertySchema;
  properties?: Record<string, ExtendedPropertySchema>;
  additionalProperties?: boolean | ExtendedPropertySchema;
  required?: string[] | boolean;
  minItems?: number;
  maxItems?: number;
  'x-widget'?: string;
  'x-placeholder'?: string;
  [key: string]: unknown;
}

/**
 * ObjectListWidget - 对象列表编辑器组件
 * 
 * 用于渲染 TypeListObject (11) 类型的数据
 * 特点：
 * - 支持添加/删除/复制项目
 * - 每个项目是一个对象，递归渲染其属性
 * - 支持按 x-group 分组显示字段
 */
const ObjectListWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
  context,
  initialValue,
}) => {
  const form = Form.useFormInstance();
  // 不使用 Form.useWatch，因为它不支持动态 namePath
  // 使用 useState + 强制更新机制
  const [updateCounter, setUpdateCounter] = useState(0);
  
  const label = uiConfig?.label || schema.title || (typeof name === 'string' ? name : '');
  const help = uiConfig?.help || schema.description;
  
  // 获取数组项的 Schema
  const extSchema = schema as ExtendedPropertySchema;
  const itemSchema = extSchema.items || {};
  const itemProperties = itemSchema.properties || {};
  const itemRequiredFields = Array.isArray(itemSchema.required) ? itemSchema.required : [];
  const minItems = extSchema.minItems || 0;
  const maxItems = extSchema.maxItems || Infinity;
  
  // 为每个项目生成唯一 ID 的 ref
  const itemIdsRef = React.useRef<string[]>([]);
  
  // 生成唯一 ID
  const generateId = () => `item-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
  
  // 使用 form.getFieldValue 获取值，而不是 Form.useWatch
  // 因为 useWatch 不支持动态 namePath
  const arrayValue = useMemo(() => {
    let value: unknown[] = [];
    
    // 从表单获取当前值
    const formValue = form?.getFieldValue(name);
    
    // 优先使用 formValue，即使是空数组也要使用（表示用户删除了所有项）
    // 只有当 formValue 是 undefined 或 null 时才 fallback 到 initialValue
    if (Array.isArray(formValue)) {
      value = formValue;
    } else if (formValue === undefined || formValue === null) {
      // 只有在 formValue 未定义时才使用 initialValue
      if (Array.isArray(initialValue)) {
        value = initialValue;
      }
    }
    
    console.log('[ObjectListWidget] arrayValue computed:', {
      name,
      updateCounter,
      formValue: formValue,
      formValueType: typeof formValue,
      formValueIsArray: Array.isArray(formValue),
      formValueLength: Array.isArray(formValue) ? formValue.length : 'not array',
      initialValueLength: Array.isArray(initialValue) ? initialValue.length : 'not array',
      resultLength: value.length,
      itemIdsLength: itemIdsRef.current.length,
    });
    
    // 确保 itemIdsRef 与 arrayValue 长度一致
    while (itemIdsRef.current.length < value.length) {
      itemIdsRef.current.push(generateId());
    }
    // 如果数组变短了，截断 IDs
    if (itemIdsRef.current.length > value.length) {
      itemIdsRef.current = itemIdsRef.current.slice(0, value.length);
    }
    
    console.log('[ObjectListWidget] itemIds after sync:', itemIdsRef.current);
    
    return value;
  }, [form, name, initialValue, updateCounter]);
  
  // 初始化：如果表单值为空但 initialValue 有值，设置表单值
  const initializedRef = React.useRef(false);
  useEffect(() => {
    if (initializedRef.current) return;
    
    const currentFormValue = form?.getFieldValue(name);
    if ((!currentFormValue || (Array.isArray(currentFormValue) && currentFormValue.length === 0)) && 
        Array.isArray(initialValue) && initialValue.length > 0 && form) {
      // 使用 setTimeout 避免在渲染期间更新
      setTimeout(() => {
        form.setFieldValue(name, initialValue);
        setUpdateCounter(c => c + 1);
      }, 0);
    }
    initializedRef.current = true;
  }, []);  // 只在挂载时执行一次

  // 检查是否包含模块引用
  const hasModuleReference = useMemo(() => {
    if (!arrayValue.length) return false;
    const checkReference = (obj: unknown): boolean => {
      if (typeof obj === 'string' && obj.startsWith('module.')) return true;
      if (obj && typeof obj === 'object') {
        return Object.values(obj).some(checkReference);
      }
      return false;
    };
    return arrayValue.some(checkReference);
  }, [arrayValue]);

  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;

  // 添加新项目
  const handleAdd = useCallback(() => {
    if (arrayValue.length >= maxItems) return;
    
    const newItem: Record<string, unknown> = {};
    Object.entries(itemProperties).forEach(([key, prop]) => {
      if (prop.default !== undefined) {
        newItem[key] = prop.default;
      }
    });
    
    const newValue = [...arrayValue, newItem];
    if (form) {
      form.setFieldValue(name, newValue);
      // 强制更新组件
      setUpdateCounter(c => c + 1);
      // 手动通知 FormRenderer 值已更新
      if (context?.triggerValuesUpdate) {
        setTimeout(() => {
          context.triggerValuesUpdate?.();
        }, 0);
      }
    }
  }, [arrayValue, maxItems, itemProperties, form, name, context]);

  // 删除项目
  const handleRemove = useCallback((index: number) => {
    console.log('[ObjectListWidget] handleRemove called:', {
      index,
      name,
      nameType: typeof name,
      nameIsArray: Array.isArray(name),
      arrayValueLength: arrayValue.length,
      minItems,
      itemIdsBefore: [...itemIdsRef.current],
    });
    
    if (arrayValue.length <= minItems) {
      console.log('[ObjectListWidget] handleRemove: cannot delete, at minItems');
      return;
    }
    
    // 删除对应的 ID
    const removedId = itemIdsRef.current[index];
    itemIdsRef.current = itemIdsRef.current.filter((_, i) => i !== index);
    
    console.log('[ObjectListWidget] handleRemove: ID removed:', {
      removedId,
      itemIdsAfter: [...itemIdsRef.current],
    });
    
    const newValue = arrayValue.filter((_, i) => i !== index);
    console.log('[ObjectListWidget] handleRemove: setting new value:', {
      name,
      newValueLength: newValue.length,
      newValue: JSON.stringify(newValue).slice(0, 200),
    });
    
    if (form) {
      // 获取设置前的值
      const beforeValue = form.getFieldValue(name);
      console.log('[ObjectListWidget] handleRemove: before setFieldValue:', {
        beforeValue,
        beforeValueLength: Array.isArray(beforeValue) ? beforeValue.length : 'not array',
      });
      
      form.setFieldValue(name, newValue);
      
      // 获取设置后的值
      const afterValue = form.getFieldValue(name);
      console.log('[ObjectListWidget] handleRemove: after setFieldValue:', {
        afterValue,
        afterValueLength: Array.isArray(afterValue) ? afterValue.length : 'not array',
      });
      
      // 强制更新组件
      setUpdateCounter(c => c + 1);
      // 手动通知 FormRenderer 值已更新
      if (context?.triggerValuesUpdate) {
        setTimeout(() => {
          context.triggerValuesUpdate?.();
        }, 0);
      }
    }
  }, [arrayValue, minItems, form, name, context]);

  // 复制项目
  const handleCopy = useCallback((index: number) => {
    if (arrayValue.length >= maxItems) return;
    const itemToCopy = JSON.parse(JSON.stringify(arrayValue[index]));
    const newValue = [...arrayValue];
    newValue.splice(index + 1, 0, itemToCopy);
    if (form) {
      form.setFieldValue(name, newValue);
      // 强制更新组件
      setUpdateCounter(c => c + 1);
      // 手动通知 FormRenderer 值已更新
      if (context?.triggerValuesUpdate) {
        setTimeout(() => {
          context.triggerValuesUpdate?.();
        }, 0);
      }
    }
  }, [arrayValue, maxItems, form, name, context]);

  // 根据属性类型确定 Widget
  const getWidgetType = (prop: ExtendedPropertySchema): string => {
    if (prop['x-widget']) return prop['x-widget'];
    if (prop.type === 'boolean') return 'switch';
    if (prop.type === 'integer' || prop.type === 'number') return 'number';
    if (prop.type === 'array') {
      if (prop.items?.type === 'object') return 'object-list';
      return 'tags';
    }
    if (prop.type === 'object') {
      if (prop.additionalProperties) return 'key-value';
      if (prop.properties) return 'object';
      return 'key-value';
    }
    if (prop.enum) return 'select';
    if (prop.format === 'json') return 'json-editor';
    return 'text';
  };

  // 按分组分类属性
  const groupedFields = useMemo((): Array<[string, Array<[string, ExtendedPropertySchema]>]> => {
    const groups: Record<string, Array<[string, ExtendedPropertySchema]>> = {};
    
    Object.entries(itemProperties).forEach(([propName, prop]) => {
      const group = (prop['x-group'] as string) || 'default';
      if (!groups[group]) groups[group] = [];
      groups[group].push([propName, prop]);
    });
    
    // 按分组名称排序：default/basic 优先
    return Object.entries(groups).sort(([a], [b]) => {
      if (a === 'default' || a === 'basic') return -1;
      if (b === 'default' || b === 'basic') return 1;
      return a.localeCompare(b);
    });
  }, [itemProperties]);

  // 获取分组显示名称
  const getGroupDisplayName = useCallback((groupId: string): string => {
    const displayNames: Record<string, string> = {
      'default': '基础配置',
      'basic': '基础配置',
      'advanced': '高级配置',
    };
    return displayNames[groupId] || groupId;
  }, []);

  // 渲染单个项目的属性
  const renderItemProperty = (itemIndex: number, propName: string, prop: ExtendedPropertySchema) => {
    const widgetType = getWidgetType(prop);
    const Widget = getWidget(widgetType as any);
    const isRequired = itemRequiredFields.includes(propName);
    
    const fieldName = typeof name === 'string' 
      ? [name, itemIndex, propName] 
      : [...name, itemIndex, propName];
    
    const propSchema = { ...prop, required: isRequired };
    const propUiConfig = {
      label: prop.title || propName,
      help: prop.description,
      placeholder: prop['x-placeholder'],
      widget: widgetType,
    };

    // 从当前项目数据中获取该属性的初始值
    // 这对于嵌套的 ObjectListWidget（如 transition 数组）非常重要
    const itemData = arrayValue[itemIndex] as Record<string, unknown> | undefined;
    const propInitialValue = itemData?.[propName];

    return (
      <Widget
        key={propName}
        name={fieldName}
        schema={propSchema}
        uiConfig={propUiConfig}
        disabled={disabled}
        readOnly={readOnly}
        context={context}
        initialValue={propInitialValue}
      />
    );
  };

  // 渲染分组内的字段
  const renderGroupFields = (itemIndex: number, fields: Array<[string, ExtendedPropertySchema]>) => (
    <Space direction="vertical" style={{ width: '100%' }}>
      {fields.map(([propName, prop]) => renderItemProperty(itemIndex, propName, prop))}
    </Space>
  );

  // 获取项目的简要描述（用于折叠时显示）
  const getItemSummary = useCallback((item: unknown): string => {
    if (!item || typeof item !== 'object') return '';
    const obj = item as Record<string, unknown>;
    // 优先显示 id, name, title 等常见标识字段
    const identifierFields = ['id', 'name', 'title', 'key', 'label'];
    for (const field of identifierFields) {
      if (obj[field] && typeof obj[field] === 'string') {
        return obj[field] as string;
      }
    }
    // 如果没有标识字段，显示第一个非空字符串字段
    for (const [key, value] of Object.entries(obj)) {
      if (typeof value === 'string' && value.length > 0 && value.length < 50) {
        return `${key}: ${value}`;
      }
    }
    return '';
  }, []);

  // 渲染单个项目 - 所有分组都显示，使用 Collapse 折叠非默认分组
  // 当子元素 >= 3 时，整个项目内容默认折叠
  const renderItem = (item: unknown, index: number) => {
    const canDelete = arrayValue.length > minItems && !readOnly && !disabled;
    const canCopy = arrayValue.length < maxItems && !readOnly && !disabled;
    
    // 分离默认分组和其他分组
    const defaultGroups = groupedFields.filter(([id]) => id === 'default' || id === 'basic');
    const otherGroups = groupedFields.filter(([id]) => id !== 'default' && id !== 'basic');
    
    const totalFields = Object.keys(itemProperties).length;
    
    // 使用唯一 ID 作为 key，确保删除时组件正确卸载
    const itemKey = itemIdsRef.current[index] || `item-${index}`;
    
    // 当子元素 >= 3 时默认折叠
    const shouldCollapse = arrayValue.length >= 3;
    
    // 获取项目摘要
    const itemSummary = getItemSummary(item);
    
    console.log('[ObjectListWidget] renderItem:', {
      index,
      itemKey,
      itemData: JSON.stringify(item).slice(0, 100),
      shouldCollapse,
    });

    // 渲染项目内容
    const renderItemContent = () => (
      <>
        {/* 默认分组 - 始终展开显示 */}
        {defaultGroups.map(([groupId, fields]) => (
          <div key={groupId} style={{ marginBottom: otherGroups.length > 0 ? 16 : 0 }}>
            {renderGroupFields(index, fields)}
          </div>
        ))}
        
        {/* 其他分组 - 使用 Collapse 折叠显示 */}
        {otherGroups.length > 0 && (
          <Collapse 
            defaultActiveKey={[]} 
            ghost
            style={{ backgroundColor: 'transparent' }}
          >
            {otherGroups.map(([groupId, fields]) => (
              <Panel 
                key={groupId}
                header={
                  <span style={{ fontSize: 12, color: '#595959' }}>
                    {getGroupDisplayName(groupId)} ({fields.length})
                  </span>
                }
                style={{ border: 'none' }}
              >
                {renderGroupFields(index, fields)}
              </Panel>
            ))}
          </Collapse>
        )}
      </>
    );

    // 如果需要折叠，使用 Collapse 包裹整个项目
    if (shouldCollapse) {
      return (
        <Collapse
          key={itemKey}
          defaultActiveKey={[]}
          style={{ marginBottom: 8, backgroundColor: '#fff', borderRadius: 8 }}
          items={[{
            key: itemKey,
            label: (
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <Space>
                  <span style={{ fontSize: 13, fontWeight: 500 }}>#{index + 1}</span>
                  {itemSummary && (
                    <span style={{ fontSize: 12, color: '#1890ff' }}>{itemSummary}</span>
                  )}
                  <span style={{ fontSize: 11, color: '#8c8c8c' }}>
                    {totalFields} 个属性
                  </span>
                </Space>
              </Space>
            ),
            extra: (
              <Space size="small" onClick={(e) => e.stopPropagation()}>
                {canCopy && (
                  <Tooltip title="复制">
                    <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => handleCopy(index)} />
                  </Tooltip>
                )}
                {canDelete && (
                  <Tooltip title="删除">
                    <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => handleRemove(index)} />
                  </Tooltip>
                )}
              </Space>
            ),
            children: renderItemContent(),
          }]}
        />
      );
    }

    // 不需要折叠时，使用 Card 显示
    return (
      <Card
        key={itemKey}
        size="small"
        style={{ marginBottom: 8, backgroundColor: '#fff' }}
        title={
          <Space>
            <span style={{ fontSize: 13 }}>#{index + 1}</span>
            {itemSummary && (
              <span style={{ fontSize: 12, color: '#1890ff' }}>{itemSummary}</span>
            )}
            <span style={{ fontSize: 11, color: '#8c8c8c' }}>
              {totalFields} 个属性
              {groupedFields.length > 1 && ` · ${groupedFields.length} 个分组`}
            </span>
          </Space>
        }
        extra={
          <Space size="small">
            {canCopy && (
              <Tooltip title="复制">
                <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => handleCopy(index)} />
              </Tooltip>
            )}
            {canDelete && (
              <Tooltip title="删除">
                <Button type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => handleRemove(index)} />
              </Tooltip>
            )}
          </Space>
        }
      >
        {renderItemContent()}
      </Card>
    );
  };

  // 渲染引用标签
  const renderReferenceTag = () => {
    if (!hasModuleReference) return null;
    return (
      <Tooltip title="包含模块引用">
        <Tag color="blue" icon={<LinkOutlined />} style={{ marginLeft: 8, cursor: 'pointer' }}>引用</Tag>
      </Tooltip>
    );
  };

  const canAdd = arrayValue.length < maxItems && !readOnly && !disabled;

  return (
    <Form.Item
      label={
        <span>
          {label}
          <Tag style={{ marginLeft: 8 }}>{arrayValue.length} 项</Tag>
          {renderReferenceTag()}
        </span>
      }
      help={
        <span>
          {help}
          {minItems > 0 && <span style={{ marginLeft: 8 }}>最少 {minItems} 项</span>}
          {maxItems < Infinity && <span style={{ marginLeft: 8 }}>最多 {maxItems} 项</span>}
        </span>
      }
      style={{ marginBottom: 16 }}
    >
      <Card 
        size="small" 
        style={{ backgroundColor: '#fafafa' }}
        title={
          <Space>
            <SettingOutlined />
            <span style={{ fontSize: 13, fontWeight: 'normal' }}>
              {Object.keys(itemProperties).length} 个属性/项
            </span>
          </Space>
        }
      >
        {arrayValue.length === 0 ? (
          <Empty description="暂无数据" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ margin: '16px 0' }} />
        ) : (
          <div style={{ maxHeight: 600, overflowY: 'auto' }}>
            {arrayValue.map((item, index) => renderItem(item, index))}
          </div>
        )}
        
        {canAdd && (
          <Button type="dashed" onClick={handleAdd} icon={<PlusOutlined />} style={{ width: '100%', marginTop: 8 }}>
            添加项目
          </Button>
        )}
        
        {hasManifestContext && (
          <div style={{ color: '#1890ff', fontSize: 11, marginTop: 8 }}>
            在文本字段输入 / 可引用其他 Module 的输出
          </div>
        )}
      </Card>
    </Form.Item>
  );
};

export default ObjectListWidget;