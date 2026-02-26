import React, { useMemo, useState } from 'react';
import { Form, Card, Collapse, Space, Tag, Tooltip, Button, Row, Col } from 'antd';
import { LinkOutlined, SettingOutlined, EyeOutlined, EyeInvisibleOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { getWidget } from './index';

const { Panel } = Collapse;

// 扩展的属性 Schema 类型（支持 x- 扩展属性）
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
  'x-colSpan'?: number;
  [key: string]: unknown;
}

/**
 * ObjectWidget - 对象编辑器组件
 * 
 * 用于渲染 TypeObject (8) 类型的数据
 * 特点：
 * - Key 由 Schema 定义，用户不可修改
 * - Value 可以是任意类型
 * - 支持递归渲染嵌套属性
 * - 支持折叠/展开功能
 */
const ObjectWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
  context,
}) => {
  const form = Form.useFormInstance();
  const formValue = Form.useWatch(name, form);
  const [showAdvanced, setShowAdvanced] = useState(false);
  
  const label = uiConfig?.label || schema.title || (typeof name === 'string' ? name : '');
  const help = uiConfig?.help || schema.description;
  
  // 获取嵌套属性
  const extSchema = schema as ExtendedPropertySchema;
  const properties = extSchema.properties || {};
  const requiredFields = Array.isArray(extSchema.required) ? extSchema.required : [];
  
  // 按分组分类属性（按 x-order 排序以保持用户定义的字段顺序）
  const { basicFields, advancedFields } = useMemo(() => {
    const basic: Array<[string, ExtendedPropertySchema]> = [];
    const advanced: Array<[string, ExtendedPropertySchema]> = [];

    // 先按 x-order 排序
    const sortedEntries = Object.entries(properties).sort(([, a], [, b]) => {
      const orderA = (a['x-order'] as number) ?? 999;
      const orderB = (b['x-order'] as number) ?? 999;
      return orderA - orderB;
    });

    sortedEntries.forEach(([propName, prop]) => {
      const group = prop['x-group'] || 'basic';
      if (group === 'advanced') {
        advanced.push([propName, prop]);
      } else {
        basic.push([propName, prop]);
      }
    });

    return { basicFields: basic, advancedFields: advanced };
  }, [properties]);
  
  // 检查是否包含模块引用
  const hasModuleReference = useMemo(() => {
    if (!formValue || typeof formValue !== 'object') return false;
    const checkReference = (obj: Record<string, unknown>): boolean => {
      return Object.values(obj).some((v) => {
        if (typeof v === 'string' && v.startsWith('module.')) return true;
        if (v && typeof v === 'object') return checkReference(v as Record<string, unknown>);
        return false;
      });
    };
    return checkReference(formValue as Record<string, unknown>);
  }, [formValue]);

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;

  // 根据属性类型确定 Widget
  const getWidgetType = (prop: ExtendedPropertySchema): string => {
    // 优先使用 x-widget
    if (prop['x-widget']) return prop['x-widget'];
    
    // 根据类型推断
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

  // 渲染单个属性
  const renderProperty = (propName: string, prop: ExtendedPropertySchema) => {
    const widgetType = getWidgetType(prop);
    const Widget = getWidget(widgetType as any);
    const isRequired = requiredFields.includes(propName);
    
    // 构建嵌套的字段名
    const fieldName = typeof name === 'string' ? [name, propName] : [...name, propName];
    
    // 构建属性的 Schema
    const propSchema = {
      ...prop,
      required: isRequired,
    };
    
    // 构建 UI 配置
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

  // 按 x-colSpan 分行渲染字段列表
  const renderFieldRows = (fields: Array<[string, ExtendedPropertySchema]>) => {
    const rows: Array<Array<[string, ExtendedPropertySchema]>> = [];
    let currentRow: Array<[string, ExtendedPropertySchema]> = [];
    let currentSpan = 0;

    for (const entry of fields) {
      const span = entry[1]['x-colSpan'] || 24;
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

    // If no field has x-colSpan (all default 24), use simple vertical layout
    const hasColSpan = fields.some(([, prop]) => prop['x-colSpan'] && prop['x-colSpan'] < 24);
    if (!hasColSpan) {
      return (
        <Space direction="vertical" style={{ width: '100%' }}>
          {fields.map(([propName, prop]) => renderProperty(propName, prop))}
        </Space>
      );
    }

    return (
      <div style={{ width: '100%' }}>
        {rows.map((rowFields, rowIdx) => (
          <Row gutter={[16, 0]} key={rowIdx}>
            {rowFields.map(([propName, prop]) => (
              <Col span={prop['x-colSpan'] || 24} key={propName}>
                {renderProperty(propName, prop)}
              </Col>
            ))}
          </Row>
        ))}
      </div>
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

  // 如果没有嵌套属性，显示提示
  if (Object.keys(properties).length === 0) {
    return (
      <Form.Item
        label={label}
        name={name}
        help={help}
      >
        <Card size="small" style={{ backgroundColor: '#fafafa' }}>
          <span style={{ color: '#8c8c8c' }}>无可配置属性</span>
        </Card>
      </Form.Item>
    );
  }

  return (
    <Form.Item
      label={
        <span>
          {label}
          {renderReferenceTag()}
        </span>
      }
      help={help}
      style={{ marginBottom: 16 }}
    >
      <Card 
        size="small" 
        style={{ backgroundColor: '#fafafa' }}
        title={
          <Space>
            <SettingOutlined />
            <span style={{ fontSize: 13, fontWeight: 'normal' }}>
              {basicFields.length} 个基础属性
              {advancedFields.length > 0 && (
                <span style={{ color: '#8c8c8c', marginLeft: 8 }}>
                  + {advancedFields.length} 个高级属性
                </span>
              )}
            </span>
          </Space>
        }
        extra={
          advancedFields.length > 0 && (
            <Button
              type="link"
              size="small"
              icon={showAdvanced ? <EyeInvisibleOutlined /> : <EyeOutlined />}
              onClick={() => setShowAdvanced(!showAdvanced)}
            >
              {showAdvanced ? '隐藏高级' : '显示高级'}
            </Button>
          )
        }
      >
        {/* 基础属性 - 始终显示 */}
        {basicFields.length > 0 && renderFieldRows(basicFields)}
        
        {/* 高级属性 - 可折叠 */}
        {advancedFields.length > 0 && showAdvanced && (
          <Collapse 
            defaultActiveKey={['advanced']} 
            ghost
            style={{ backgroundColor: 'transparent', marginTop: basicFields.length > 0 ? 16 : 0 }}
          >
            <Panel 
              header={
                <span style={{ fontSize: 12, color: '#8c8c8c' }}>
                  高级配置 ({advancedFields.length})
                </span>
              } 
              key="advanced"
              style={{ border: 'none' }}
            >
              {renderFieldRows(advancedFields)}
            </Panel>
          </Collapse>
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

export default ObjectWidget;
