import React, { useMemo, useCallback, useEffect, useState, useRef } from 'react';
import { Form, Card, Button, Space, Tag, Tooltip, Empty, Collapse, Row, Col } from 'antd';
import { PlusOutlined, DeleteOutlined, CopyOutlined, LinkOutlined, KeyOutlined, DownOutlined } from '@ant-design/icons';
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
  'x-widget'?: string;
  'x-placeholder'?: string;
  'x-dynamic-keys'?: boolean;
  'x-key-pattern'?: string;
  [key: string]: unknown;
}

/**
 * 从值推断属性 Schema
 * 用于当 Schema 没有定义 additionalProperties.properties 时，从实际数据推断
 */
const inferPropertySchema = (key: string, value: unknown): ExtendedPropertySchema => {
  // 将 key 转换为标题格式（如 group -> Group）
  const title = key.charAt(0).toUpperCase() + key.slice(1).replace(/_/g, ' ');
  
  if (value === null || value === undefined) {
    return { type: 'string', title };
  }
  
  if (typeof value === 'boolean') {
    return { type: 'boolean', title };
  }
  
  if (typeof value === 'number') {
    return Number.isInteger(value) 
      ? { type: 'integer', title }
      : { type: 'number', title };
  }
  
  if (Array.isArray(value)) {
    if (value.length > 0 && typeof value[0] === 'object') {
      return { type: 'array', title, items: { type: 'object' } };
    }
    return { type: 'array', title, items: { type: 'string' } };
  }
  
  if (typeof value === 'object') {
    return { type: 'object', title };
  }
  
  // 默认为字符串
  return { type: 'string', title };
};

/**
 * 生成动态 Key
 * 规则：8-15位，只能包含小写字母和中横线，以字母开头
 */
const generateDynamicKey = (): string => {
  const chars = 'abcdefghijklmnopqrstuvwxyz';
  const length = Math.floor(Math.random() * 8) + 8; // 8-15位
  let key = chars[Math.floor(Math.random() * chars.length)]; // 以字母开头
  
  for (let i = 1; i < length; i++) {
    if (i > 2 && Math.random() < 0.15) {
      key += '-';
    } else {
      key += chars[Math.floor(Math.random() * chars.length)];
    }
  }
  
  // 确保不以中横线结尾
  if (key.endsWith('-')) {
    key = key.slice(0, -1) + chars[Math.floor(Math.random() * chars.length)];
  }
  
  return key;
};

/**
 * DynamicObjectWidget - 动态键对象编辑器组件
 * 
 * 用于渲染 CustomObject (12) 类型的数据
 * 
 * 数据结构示例：
 * ```
 * ff = {
 *   first = {      // first 是平台自动生成的索引，不可修改
 *     group = "a"  // group 是固定的属性名，用户只能修改值
 *     zone = "ap-northeast-1"
 *   }
 *   adasdas = {    // 新建时自动生成的索引
 *     group = ""
 *     zone = ""
 *   }
 * }
 * ```
 * 
 * 特点：
 * - 外层 Key（索引）由平台自动生成，用户不可修改
 * - 内层属性（如 group, zone）由 Schema 定义，Key 固定，用户只能修改 Value
 * - 支持添加/删除/复制项目
 * - 存量数据和新建数据有不同的视觉标识
 */
const DynamicObjectWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
  context,
  initialValue,
}) => {
  const form = Form.useFormInstance();
  const formValue = Form.useWatch(name, form);
  
  // 使用本地状态来强制触发重新渲染
  // 这是因为 Form.useWatch 在某些情况下不会立即响应 setFieldValue 的变化
  const [localValue, setLocalValue] = useState<Record<string, unknown> | null>(null);
  // 用于强制重新渲染的计数器
  const [updateCounter, setUpdateCounter] = useState(0);
  
  // 滚动容器引用和滚动状态
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const [canScrollDown, setCanScrollDown] = useState(false);
  const [canScrollUp, setCanScrollUp] = useState(false);
  
  const label = uiConfig?.label || schema.title || (typeof name === 'string' ? name : '');
  const help = uiConfig?.help || schema.description;
  
  // 记录初始存在的 key（这些 key 不可修改）
  const initialKeys = useMemo(() => {
    if (initialValue && typeof initialValue === 'object' && !Array.isArray(initialValue)) {
      return new Set(Object.keys(initialValue));
    }
    return new Set<string>();
  }, [initialValue]);

  // 确保表单值正确初始化
  // 当 formValue 为 undefined 但 initialValue 有值时，主动设置表单值
  useEffect(() => {
    if (formValue === undefined && initialValue && typeof initialValue === 'object' && !Array.isArray(initialValue)) {
      // 使用 setTimeout 确保在 Form 初始化完成后设置值
      const timer = setTimeout(() => {
        form.setFieldValue(name, initialValue);
        setLocalValue(initialValue as Record<string, unknown>);
      }, 0);
      return () => clearTimeout(timer);
    }
  }, [formValue, initialValue, form, name]);
  
  // 当 formValue 变化时，同步到 localValue
  useEffect(() => {
    if (formValue && typeof formValue === 'object' && !Array.isArray(formValue)) {
      setLocalValue(formValue as Record<string, unknown>);
    }
  }, [formValue]);
  
  // 检查滚动状态
  const checkScrollState = useCallback(() => {
    const container = scrollContainerRef.current;
    if (container) {
      const { scrollTop, scrollHeight, clientHeight } = container;
      setCanScrollUp(scrollTop > 10);
      setCanScrollDown(scrollTop + clientHeight < scrollHeight - 10);
    }
  }, []);
  
  // 滚动到底部
  const scrollToBottom = useCallback(() => {
    const container = scrollContainerRef.current;
    if (container) {
      container.scrollTo({
        top: container.scrollHeight,
        behavior: 'smooth'
      });
    }
  }, []);
  
  // 滚动到顶部
  const scrollToTop = useCallback(() => {
    const container = scrollContainerRef.current;
    if (container) {
      container.scrollTo({
        top: 0,
        behavior: 'smooth'
      });
    }
  }, []);
  
  // 获取值的 Schema（additionalProperties）
  const extSchema = schema as ExtendedPropertySchema;
  const valueSchema = (typeof extSchema.additionalProperties === 'object' 
    ? extSchema.additionalProperties 
    : {}) as ExtendedPropertySchema;
  const schemaDefinedProperties = valueSchema.properties || {};
  const valueRequiredFields = Array.isArray(valueSchema.required) ? valueSchema.required : [];
  
  // 当前对象值
  // 优先使用 localValue（本地状态），然后是 formValue，最后是 initialValue
  const objectValue = useMemo(() => {
    // 优先使用 localValue（本地状态），因为它会立即更新
    if (localValue && typeof localValue === 'object' && !Array.isArray(localValue)) {
      return localValue;
    }
    if (formValue && typeof formValue === 'object' && !Array.isArray(formValue)) {
      return formValue as Record<string, unknown>;
    }
    // 如果 formValue 是 undefined，使用 initialValue 作为 fallback
    if (initialValue && typeof initialValue === 'object' && !Array.isArray(initialValue)) {
      return initialValue as Record<string, unknown>;
    }
    return {};
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [localValue, formValue, initialValue, updateCounter]);

  // 从数据中推断属性结构（当 Schema 没有定义 properties 时）
  const inferredProperties = useMemo((): Record<string, ExtendedPropertySchema> => {
    // 如果 Schema 已经定义了 properties，直接使用
    if (Object.keys(schemaDefinedProperties).length > 0) {
      return schemaDefinedProperties;
    }
    
    // 从实际数据中推断属性结构
    const inferred: Record<string, ExtendedPropertySchema> = {};
    const values = Object.values(objectValue);
    
    if (values.length === 0) {
      // 如果没有数据，尝试从 initialValue 推断
      if (initialValue && typeof initialValue === 'object' && !Array.isArray(initialValue)) {
        const initialValues = Object.values(initialValue as Record<string, unknown>);
        if (initialValues.length > 0) {
          const firstItem = initialValues[0];
          if (firstItem && typeof firstItem === 'object' && !Array.isArray(firstItem)) {
            Object.entries(firstItem as Record<string, unknown>).forEach(([key, value]) => {
              inferred[key] = inferPropertySchema(key, value);
            });
          }
        }
      }
      return inferred;
    }
    
    // 从第一个有效的对象值推断属性
    for (const value of values) {
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        Object.entries(value as Record<string, unknown>).forEach(([key, val]) => {
          if (!inferred[key]) {
            inferred[key] = inferPropertySchema(key, val);
          }
        });
      }
    }
    
    return inferred;
  }, [schemaDefinedProperties, objectValue, initialValue]);

  // 最终使用的属性定义
  const valueProperties = useMemo(() => {
    return Object.keys(schemaDefinedProperties).length > 0 
      ? schemaDefinedProperties 
      : inferredProperties;
  }, [schemaDefinedProperties, inferredProperties]);

  // 获取所有 Key
  const keys = useMemo(() => Object.keys(objectValue), [objectValue]);

  // 监听滚动事件和内容变化
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (container) {
      checkScrollState();
      container.addEventListener('scroll', checkScrollState);
      return () => container.removeEventListener('scroll', checkScrollState);
    }
  }, [checkScrollState, keys.length, updateCounter]);

  // 检查是否包含模块引用
  const hasModuleReference = useMemo(() => {
    if (!keys.length) return false;
    const checkReference = (obj: unknown): boolean => {
      if (typeof obj === 'string' && obj.startsWith('module.')) return true;
      if (obj && typeof obj === 'object') {
        return Object.values(obj).some(checkReference);
      }
      return false;
    };
    return Object.values(objectValue).some(checkReference);
  }, [objectValue, keys]);

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;

  // 生成唯一的 Key
  const generateUniqueKey = useCallback((): string => {
    let newKey = generateDynamicKey();
    let attempts = 0;
    while (keys.includes(newKey) && attempts < 100) {
      newKey = generateDynamicKey();
      attempts++;
    }
    return newKey;
  }, [keys]);

  // 根据属性类型获取默认值
  const getDefaultValueForType = useCallback((prop: ExtendedPropertySchema): unknown => {
    // 如果有明确定义的默认值，使用它
    if (prop.default !== undefined) {
      return prop.default;
    }
    
    // 根据类型返回合适的默认值
    switch (prop.type) {
      case 'boolean':
        return false;
      case 'integer':
      case 'number':
        return 0;
      case 'array':
        return [];
      case 'object':
        return {};
      case 'string':
      default:
        return '';
    }
  }, []);

  // 添加新项目
  const handleAdd = useCallback(() => {
    const newKey = generateUniqueKey();
    
    // 创建默认值 - 为所有定义的属性创建初始值
    const newValue: Record<string, unknown> = {};
    Object.entries(valueProperties).forEach(([key, prop]) => {
      newValue[key] = getDefaultValueForType(prop);
    });
    
    // 直接从 form 获取当前值，确保使用最新的数据
    const currentValue = form.getFieldValue(name);
    const currentObjectValue = (currentValue && typeof currentValue === 'object' && !Array.isArray(currentValue))
      ? currentValue as Record<string, unknown>
      : objectValue;
    
    const updatedValue = { ...currentObjectValue, [newKey]: newValue };
    
    // 同时更新表单值和本地状态
    form.setFieldValue(name, updatedValue);
    setLocalValue(updatedValue);
    // 强制触发重新渲染
    setUpdateCounter(c => c + 1);
  }, [objectValue, valueProperties, form, name, generateUniqueKey, getDefaultValueForType]);

  // 删除项目
  const handleRemove = useCallback((key: string) => {
    // 直接从 form 获取当前值，确保使用最新的数据
    const currentValue = form.getFieldValue(name);
    const currentObjectValue = (currentValue && typeof currentValue === 'object' && !Array.isArray(currentValue))
      ? { ...currentValue as Record<string, unknown> }
      : { ...objectValue };
    
    delete currentObjectValue[key];
    
    // 同时更新表单值和本地状态
    form.setFieldValue(name, currentObjectValue);
    setLocalValue(currentObjectValue);
    // 强制触发重新渲染
    setUpdateCounter(c => c + 1);
  }, [objectValue, form, name]);

  // 复制项目
  const handleCopy = useCallback((key: string) => {
    const newKey = generateUniqueKey();
    
    // 直接从 form 获取当前值，确保使用最新的数据
    const currentValue = form.getFieldValue(name);
    const currentObjectValue = (currentValue && typeof currentValue === 'object' && !Array.isArray(currentValue))
      ? currentValue as Record<string, unknown>
      : objectValue;
    
    const valueToCopy = JSON.parse(JSON.stringify(currentObjectValue[key]));
    const updatedValue = { ...currentObjectValue, [newKey]: valueToCopy };
    
    // 同时更新表单值和本地状态
    form.setFieldValue(name, updatedValue);
    setLocalValue(updatedValue);
    // 强制触发重新渲染
    setUpdateCounter(c => c + 1);
  }, [objectValue, form, name, generateUniqueKey]);

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

  // 渲染单个项目的属性
  const renderItemProperty = (itemKey: string, propName: string, prop: ExtendedPropertySchema) => {
    const widgetType = getWidgetType(prop);
    const Widget = getWidget(widgetType as any);
    const isRequired = valueRequiredFields.includes(propName);
    
    // 构建嵌套的字段名
    const fieldName = typeof name === 'string' 
      ? [name, itemKey, propName] 
      : [...name, itemKey, propName];
    
    const propSchema = { ...prop, required: isRequired };
    const propUiConfig = {
      label: prop.title || propName,
      help: prop.description,
      placeholder: prop['x-placeholder'],
      widget: widgetType,
    };

    return (
      <Widget
        key={propName}
        name={fieldName}
        schema={propSchema}
        uiConfig={propUiConfig}
        disabled={disabled}
        readOnly={readOnly}
        context={context}
      />
    );
  };

  // 按 x-colSpan 分行渲染属性列表
  const renderValuePropertyFields = (itemKey: string, fields: Array<[string, ExtendedPropertySchema]>) => {
    const hasColSpan = fields.some(([, prop]) => prop['x-colSpan'] && (prop['x-colSpan'] as number) < 24);
    if (!hasColSpan) {
      return (
        <Space direction="vertical" style={{ width: '100%' }}>
          {fields.map(([propName, prop]) => renderItemProperty(itemKey, propName, prop))}
        </Space>
      );
    }

    const rows: Array<Array<[string, ExtendedPropertySchema]>> = [];
    let currentRow: Array<[string, ExtendedPropertySchema]> = [];
    let currentSpan = 0;
    for (const entry of fields) {
      const span = (entry[1]['x-colSpan'] as number) || 24;
      if (currentSpan + span > 24 && currentRow.length > 0) {
        rows.push(currentRow);
        currentRow = [];
        currentSpan = 0;
      }
      currentRow.push(entry);
      currentSpan += span;
      if (currentSpan >= 24) {
        rows.push(currentRow);
        currentRow = [];
        currentSpan = 0;
      }
    }
    if (currentRow.length > 0) rows.push(currentRow);

    return (
      <div style={{ width: '100%' }}>
        {rows.map((rowFields, rowIdx) => (
          <Row gutter={[16, 0]} key={rowIdx}>
            {rowFields.map(([propName, prop]) => (
              <Col span={(prop['x-colSpan'] as number) || 24} key={propName}>
                {renderItemProperty(itemKey, propName, prop)}
              </Col>
            ))}
          </Row>
        ))}
      </div>
    );
  };

  // 渲染单个项目
  const renderItem = (key: string, _index: number) => {
    const canDelete = !readOnly && !disabled;
    const canCopy = !readOnly && !disabled;
    // 存量数据的 key 不可修改
    const isExistingKey = initialKeys.has(key);

    return (
      <Card
        key={key}
        size="small"
        style={{ marginBottom: 8, backgroundColor: '#fff' }}
        title={
          <Space>
            <KeyOutlined style={{ color: isExistingKey ? '#52c41a' : '#1890ff' }} />
            {/* 所有 key 都显示为只读文本，因为 key 是平台自动生成的索引 */}
            <Tooltip title={isExistingKey ? '存量数据的索引' : '新建项目的索引（自动生成）'}>
              <span style={{ 
                fontFamily: 'monospace', 
                padding: '4px 8px',
                backgroundColor: isExistingKey ? '#f6ffed' : '#e6f7ff',
                borderRadius: 4,
                border: `1px solid ${isExistingKey ? '#b7eb8f' : '#91d5ff'}`,
                color: isExistingKey ? '#52c41a' : '#1890ff'
              }}>
                {key}
              </span>
            </Tooltip>
            <Tag color={isExistingKey ? 'green' : 'blue'}>
              {isExistingKey ? '存量' : '新建'}
            </Tag>
          </Space>
        }
        extra={
          <Space size="small">
            {canCopy && (
              <Tooltip title="复制">
                <Button
                  type="text"
                  size="small"
                  icon={<CopyOutlined />}
                  onClick={() => handleCopy(key)}
                />
              </Tooltip>
            )}
            {canDelete && (
              <Tooltip title="删除">
                <Button
                  type="text"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  onClick={() => handleRemove(key)}
                />
              </Tooltip>
            )}
          </Space>
        }
      >
        <Collapse defaultActiveKey={['props']} ghost>
          <Panel header="属性" key="props" style={{ border: 'none' }}>
            {renderValuePropertyFields(key, Object.entries(valueProperties))}
          </Panel>
        </Collapse>
      </Card>
    );
  };

  // 渲染引用标签
  const renderReferenceTag = () => {
    if (!hasModuleReference) return null;
    
    return (
      <Tooltip title="包含模块引用">
        <Tag 
          color="blue" 
          icon={<LinkOutlined />}
          style={{ marginLeft: 8, cursor: 'pointer' }}
        >
          引用
        </Tag>
      </Tooltip>
    );
  };

  const canAdd = !readOnly && !disabled;

  return (
    <Form.Item
      label={
        <span>
          {label}
          <Tag style={{ marginLeft: 8 }}>{keys.length} 项</Tag>
          {renderReferenceTag()}
        </span>
      }
      help={
        <span>
          {help}
          <span style={{ marginLeft: 8, color: '#8c8c8c', fontSize: 11 }}>
            Key 格式：8-15位小写字母和中横线
          </span>
        </span>
      }
      style={{ marginBottom: 16 }}
    >
      <Card size="small" style={{ backgroundColor: '#fafafa' }}>
        {keys.length === 0 ? (
          <Empty 
            description="暂无数据" 
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            style={{ margin: '16px 0' }}
          />
        ) : (
          <div style={{ position: 'relative' }}>
            {/* 顶部滚动提示 */}
            {canScrollUp && (
              <div
                onClick={scrollToTop}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  right: 0,
                  height: 32,
                  background: 'linear-gradient(to bottom, rgba(250, 250, 250, 0.95), rgba(250, 250, 250, 0))',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                  zIndex: 10,
                  borderRadius: '4px 4px 0 0',
                }}
              >
                <DownOutlined style={{ transform: 'rotate(180deg)', color: '#1890ff', fontSize: 12 }} />
                <span style={{ marginLeft: 4, color: '#1890ff', fontSize: 11 }}>点击滚动到顶部</span>
              </div>
            )}
            
            {/* 滚动容器 */}
            <div 
              ref={scrollContainerRef}
              style={{ 
                maxHeight: 400, 
                overflowY: 'auto',
                paddingTop: canScrollUp ? 8 : 0,
                paddingBottom: canScrollDown ? 8 : 0,
                // 自定义滚动条样式
                scrollbarWidth: 'thin',
                scrollbarColor: '#1890ff #f0f0f0',
              }}
            >
              {keys.map((key, index) => renderItem(key, index))}
            </div>
            
            {/* 底部滚动提示 */}
            {canScrollDown && (
              <div
                onClick={scrollToBottom}
                style={{
                  position: 'absolute',
                  bottom: 0,
                  left: 0,
                  right: 0,
                  height: 32,
                  background: 'linear-gradient(to top, rgba(250, 250, 250, 0.95), rgba(250, 250, 250, 0))',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                  zIndex: 10,
                  borderRadius: '0 0 4px 4px',
                }}
              >
                <DownOutlined style={{ color: '#1890ff', fontSize: 12 }} />
                <span style={{ marginLeft: 4, color: '#1890ff', fontSize: 11 }}>点击滚动查看更多</span>
              </div>
            )}
          </div>
        )}
        
        {canAdd && (
          <Button
            type="dashed"
            onClick={handleAdd}
            icon={<PlusOutlined />}
            style={{ width: '100%', marginTop: 8 }}
          >
            添加项目（自动生成 Key）
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

export default DynamicObjectWidget;
