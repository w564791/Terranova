import React, { useState, useCallback, useMemo } from 'react';
import { Form, Input, Button, Space, Tag, Tooltip, Alert } from 'antd';
import { LinkOutlined, FormatPainterOutlined, CheckOutlined, WarningOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';

const { TextArea } = Input;

/** 当 schema.type 为 object/array 时，值应存储为 parsed object/array 而非 string */
const shouldStoreAsParsedObject = (schema: any): boolean => {
  const t = schema?.type;
  return t === 'object' || t === 'array';
};

/** 检测引用表达式：module.xxx、${module.xxx}、var.xxx、${var.xxx}、local.xxx、${local.xxx} 等 */
const isReferenceExpression = (value: unknown): boolean => {
  if (typeof value !== 'string') return false;
  const s = value.trim();
  return /^(module\.|var\.|local\.|data\.)/.test(s) || /^\$\{.+\}$/.test(s);
};

/**
 * 根据 schema.type 决定存储格式：
 * - 引用表达式始终存为 string
 * - object/array 类型尝试 parse 为对象，parse 失败暂存 string（用户编辑中）
 * - 其他类型存为 string
 */
const resolveFormValue = (rawString: string, schema: any): unknown => {
  if (isReferenceExpression(rawString)) return rawString;
  if (!shouldStoreAsParsedObject(schema)) return rawString;
  try {
    const parsed = JSON.parse(rawString);
    if (typeof parsed === 'object' && parsed !== null) return parsed;
    return rawString;
  } catch {
    return rawString; // 用户编辑中，暂存 string
  }
};

/**
 * JsonEditorWidget - JSON 编辑器组件
 * 
 * 用于渲染 TypeJsonString (9) 类型的数据
 * 特点：
 * - 支持 JSON 语法高亮（通过等宽字体）
 * - 支持格式化功能
 * - 实时 JSON 验证
 * - 显示验证错误信息
 */
const JsonEditorWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
  context,
}) => {
  const form = Form.useFormInstance();
  const formValue = Form.useWatch(name, form);
  
  const label = uiConfig?.label || schema.title || (typeof name === 'string' ? name : '');
  const help = uiConfig?.help || schema.description;
  
  // JSON 验证状态
  const [jsonError, setJsonError] = useState<string | null>(null);
  
  // 当前值
  const currentValue = useMemo(() => {
    if (typeof formValue === 'string') return formValue;
    if (formValue !== undefined && formValue !== null) {
      try {
        return JSON.stringify(formValue, null, 2);
      } catch {
        return String(formValue);
      }
    }
    return '';
  }, [formValue]);

  // 检查是否是引用表达式
  const isModuleReference = isReferenceExpression(formValue);

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;

  // 验证 JSON
  const validateJson = useCallback((value: string): boolean => {
    if (!value || value.trim() === '') {
      setJsonError(null);
      return true;
    }
    
    // 如果是引用表达式，不验证 JSON
    if (isReferenceExpression(value)) {
      setJsonError(null);
      return true;
    }
    
    try {
      JSON.parse(value);
      setJsonError(null);
      return true;
    } catch (e) {
      setJsonError((e as Error).message);
      return false;
    }
  }, []);

  // 处理值变化
  const handleChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newValue = e.target.value;
    validateJson(newValue);
    form.setFieldValue(name, resolveFormValue(newValue, schema));
  }, [form, name, validateJson, schema]);

  // 格式化 JSON
  const handleFormat = useCallback(() => {
    if (!currentValue || isModuleReference) return;

    try {
      const parsed = JSON.parse(currentValue);
      const formatted = JSON.stringify(parsed, null, 2);
      form.setFieldValue(name, shouldStoreAsParsedObject(schema) ? parsed : formatted);
      setJsonError(null);
    } catch (e) {
      setJsonError((e as Error).message);
    }
  }, [currentValue, isModuleReference, form, name, schema]);

  // 压缩 JSON
  const handleMinify = useCallback(() => {
    if (!currentValue || isModuleReference) return;

    try {
      const parsed = JSON.parse(currentValue);
      const minified = JSON.stringify(parsed);
      form.setFieldValue(name, shouldStoreAsParsedObject(schema) ? parsed : minified);
      setJsonError(null);
    } catch (e) {
      setJsonError((e as Error).message);
    }
  }, [currentValue, isModuleReference, form, name, schema]);

  // 渲染引用标签
  const renderReferenceTag = () => {
    if (!isModuleReference) return null;
    
    const parts = (formValue as string).split('.');
    if (parts.length >= 3) {
      const instanceName = parts[1];
      const outputName = parts.slice(2).join('.');
      return (
        <Tooltip title={`引用自 ${instanceName} 的 ${outputName}`}>
          <Tag 
            color="blue" 
            icon={<LinkOutlined />}
            style={{ marginLeft: 8, cursor: 'pointer' }}
          >
            {instanceName}.{outputName}
          </Tag>
        </Tooltip>
      );
    }
    return null;
  };

  // 渲染验证状态
  const renderValidationStatus = () => {
    if (isModuleReference) return null;
    if (!currentValue) return null;
    
    if (jsonError) {
      return (
        <Tag color="error" icon={<WarningOutlined />}>
          JSON 无效
        </Tag>
      );
    }
    
    return (
      <Tag color="success" icon={<CheckOutlined />}>
        JSON 有效
      </Tag>
    );
  };

  return (
    <Form.Item
      label={
        <span>
          {label}
          {renderReferenceTag()}
          {renderValidationStatus()}
        </span>
      }
      name={name}
      help={
        <span>
          {help}
          {hasManifestContext && !isModuleReference && (
            <span style={{ color: '#1890ff', marginLeft: 8, fontSize: 11 }}>
              输入 module. 开头的引用表达式
            </span>
          )}
        </span>
      }
      validateStatus={jsonError ? 'error' : undefined}
      rules={[
        ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
        {
          validator: async (_, value) => {
            if (!value) return Promise.resolve();
            // 已成功 parse 为 object/array 的值，直接通过
            if (typeof value === 'object') return Promise.resolve();
            // 引用表达式不验证 JSON
            if (isReferenceExpression(value)) return Promise.resolve();
            try {
              JSON.parse(value);
              return Promise.resolve();
            } catch {
              return Promise.reject(new Error('请输入有效的 JSON 格式'));
            }
          },
        },
      ]}
    >
      <div>
        <TextArea
          value={currentValue}
          onChange={handleChange}
          placeholder={uiConfig?.placeholder || '请输入 JSON 格式的内容'}
          disabled={disabled}
          readOnly={readOnly}
          autoSize={{ minRows: 6, maxRows: 20 }}
          style={{
            fontFamily: 'Monaco, Menlo, "Ubuntu Mono", Consolas, monospace',
            fontSize: 13,
            lineHeight: 1.5,
            ...(isModuleReference ? { color: '#1890ff' } : {}),
            ...(jsonError ? { borderColor: '#ff4d4f' } : {}),
          }}
        />
        
        {jsonError && (
          <Alert
            message="JSON 解析错误"
            description={jsonError}
            type="error"
            showIcon
            style={{ marginTop: 8 }}
          />
        )}
        
        {!readOnly && !disabled && !isModuleReference && (
          <Space style={{ marginTop: 8 }}>
            <Button
              size="small"
              icon={<FormatPainterOutlined />}
              onClick={handleFormat}
              disabled={!currentValue || !!jsonError}
            >
              格式化
            </Button>
            <Button
              size="small"
              onClick={handleMinify}
              disabled={!currentValue || !!jsonError}
            >
              压缩
            </Button>
          </Space>
        )}
      </div>
    </Form.Item>
  );
};

export default JsonEditorWidget;
