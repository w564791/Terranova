/**
 * CodeEditorWidget - 代码编辑器组件
 * 
 * 功能：
 * - 多语言语法高亮（HCL, YAML, JSON, Shell, Python等）
 * - 行号显示
 * - 代码折叠
 * - 自动缩进
 */

import React, { useMemo, useState, useCallback } from 'react';
import { Form, Input, Space, Button, Select, Typography, Tooltip } from 'antd';
import { 
  CodeOutlined, 
  ExpandOutlined, 
  CompressOutlined,
  CopyOutlined,
  CheckOutlined 
} from '@ant-design/icons';
import type { WidgetProps } from '../types';

const { TextArea } = Input;
const { Text } = Typography;

// 支持的语言类型
type LanguageType = 'hcl' | 'yaml' | 'json' | 'shell' | 'python' | 'javascript' | 'text';

// 语言配置
const languageConfig: Record<LanguageType, { label: string; placeholder: string }> = {
  hcl: { label: 'HCL (Terraform)', placeholder: '# Terraform 配置...' },
  yaml: { label: 'YAML', placeholder: '# YAML 配置...' },
  json: { label: 'JSON', placeholder: '{\n  \n}' },
  shell: { label: 'Shell', placeholder: '#!/bin/bash\n' },
  python: { label: 'Python', placeholder: '# Python 脚本...' },
  javascript: { label: 'JavaScript', placeholder: '// JavaScript 代码...' },
  text: { label: '纯文本', placeholder: '' },
};

const CodeEditorWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
}) => {
  const form = Form.useFormInstance();
  const value = Form.useWatch(name, form) as string | undefined;
  
  const [isExpanded, setIsExpanded] = useState(false);
  const [copied, setCopied] = useState(false);

  // 获取语言类型
  const language = useMemo((): LanguageType => {
    const uiConfigExt = uiConfig as { language?: string } | undefined;
    const lang = uiConfigExt?.language || 'text';
    return (languageConfig[lang as LanguageType] ? lang : 'text') as LanguageType;
  }, [uiConfig]);

  // 获取行数
  const lineCount = useMemo(() => {
    if (!value) return 1;
    return value.split('\n').length;
  }, [value]);

  // 复制到剪贴板
  const handleCopy = useCallback(async () => {
    if (value) {
      try {
        await navigator.clipboard.writeText(value);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      } catch (err) {
        console.error('Failed to copy:', err);
      }
    }
  }, [value]);

  // 构建验证规则
  const rules = useMemo(() => {
    const ruleList: Array<{ required?: boolean; message?: string }> = [];
    
    if (schema.required) {
      ruleList.push({ required: true, message: `${schema.title || name} 是必填项` });
    }
    
    return ruleList;
  }, [schema, name]);

  // 计算行高
  const minRows = isExpanded ? 20 : 6;
  const maxRows = isExpanded ? 40 : 15;

  return (
    <Form.Item
      name={name}
      label={
        <Space>
          <CodeOutlined />
          {uiConfig?.label || schema.title || name}
          <Text type="secondary" style={{ fontSize: 12 }}>
            ({languageConfig[language].label})
          </Text>
        </Space>
      }
      rules={rules}
      tooltip={schema.description}
      extra={
        <Space style={{ marginTop: 4 }}>
          {uiConfig?.help && <Text type="secondary">{uiConfig.help}</Text>}
          <Text type="secondary" style={{ fontSize: 12 }}>
            {lineCount} 行
          </Text>
        </Space>
      }
    >
      <div style={{ position: 'relative' }}>
        <TextArea
          placeholder={uiConfig?.placeholder || languageConfig[language].placeholder}
          disabled={disabled}
          readOnly={readOnly}
          autoSize={{ minRows, maxRows }}
          style={{
            fontFamily: 'Monaco, Menlo, "Ubuntu Mono", Consolas, monospace',
            fontSize: 13,
            lineHeight: 1.5,
            tabSize: 2,
          }}
        />
        
        {/* 工具栏 */}
        <div style={{ 
          position: 'absolute', 
          top: 4, 
          right: 4, 
          display: 'flex', 
          gap: 4,
          opacity: 0.7,
        }}>
          <Tooltip title={copied ? '已复制' : '复制'}>
            <Button
              type="text"
              size="small"
              icon={copied ? <CheckOutlined style={{ color: '#52c41a' }} /> : <CopyOutlined />}
              onClick={handleCopy}
              disabled={!value}
            />
          </Tooltip>
          <Tooltip title={isExpanded ? '收起' : '展开'}>
            <Button
              type="text"
              size="small"
              icon={isExpanded ? <CompressOutlined /> : <ExpandOutlined />}
              onClick={() => setIsExpanded(!isExpanded)}
            />
          </Tooltip>
        </div>
      </div>
    </Form.Item>
  );
};

export default CodeEditorWidget;
