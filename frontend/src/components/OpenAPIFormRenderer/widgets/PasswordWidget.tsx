/**
 * PasswordWidget - 密码输入组件
 * 
 * 功能：
 * - 密码可见性切换
 * - 密码强度指示器
 * - 支持模块引用
 */

import React, { useState, useMemo } from 'react';
import { Form, Input, Progress, Space, Typography } from 'antd';
import { EyeInvisibleOutlined, EyeTwoTone, SafetyOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';

const { Text } = Typography;

// 密码强度等级
type StrengthLevel = 'weak' | 'medium' | 'strong' | 'very-strong';

interface StrengthInfo {
  level: StrengthLevel;
  percent: number;
  color: string;
  text: string;
}

/**
 * 计算密码强度
 */
const calculateStrength = (password: string): StrengthInfo => {
  if (!password) {
    return { level: 'weak', percent: 0, color: '#ff4d4f', text: '' };
  }

  let score = 0;
  
  // 长度评分
  if (password.length >= 8) score += 1;
  if (password.length >= 12) score += 1;
  if (password.length >= 16) score += 1;
  
  // 复杂度评分
  if (/[a-z]/.test(password)) score += 1;
  if (/[A-Z]/.test(password)) score += 1;
  if (/[0-9]/.test(password)) score += 1;
  if (/[^a-zA-Z0-9]/.test(password)) score += 1;
  
  // 连续字符扣分
  if (/(.)\1{2,}/.test(password)) score -= 1;
  
  // 常见密码扣分
  const commonPasswords = ['password', '123456', 'qwerty', 'admin'];
  if (commonPasswords.some(p => password.toLowerCase().includes(p))) {
    score -= 2;
  }

  // 计算等级
  if (score <= 2) {
    return { level: 'weak', percent: 25, color: '#ff4d4f', text: '弱' };
  } else if (score <= 4) {
    return { level: 'medium', percent: 50, color: '#faad14', text: '中' };
  } else if (score <= 6) {
    return { level: 'strong', percent: 75, color: '#52c41a', text: '强' };
  } else {
    return { level: 'very-strong', percent: 100, color: '#1890ff', text: '非常强' };
  }
};

const PasswordWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
}) => {
  const [showStrength, setShowStrength] = useState(false);
  const form = Form.useFormInstance();
  const value = Form.useWatch(name, form) as string | undefined;

  // 计算密码强度
  const strength = useMemo(() => calculateStrength(value || ''), [value]);

  // 是否显示强度指示器
  const uiConfigExt = uiConfig as { showStrength?: boolean; alwaysShowStrength?: boolean } | undefined;
  const showStrengthIndicator = uiConfigExt?.showStrength !== false && value && value.length > 0;

  // 构建验证规则
  const rules = useMemo(() => {
    const ruleList: Array<{ required?: boolean; message?: string; min?: number; max?: number; pattern?: RegExp }> = [];
    
    if (schema.required) {
      ruleList.push({ required: true, message: `${schema.title || name} 是必填项` });
    }
    
    if (schema.minLength) {
      ruleList.push({ min: schema.minLength, message: `最少 ${schema.minLength} 个字符` });
    }
    
    if (schema.maxLength) {
      ruleList.push({ max: schema.maxLength, message: `最多 ${schema.maxLength} 个字符` });
    }
    
    if (schema.pattern) {
      ruleList.push({ pattern: new RegExp(schema.pattern), message: schema['x-validation']?.[0]?.errorMessage || '格式不正确' });
    }
    
    return ruleList;
  }, [schema, name]);

  return (
    <Form.Item
      name={name}
      label={uiConfig?.label || schema.title || name}
      rules={rules}
      tooltip={schema.description}
      extra={uiConfig?.help}
    >
      <Space direction="vertical" style={{ width: '100%' }}>
        <Input.Password
          placeholder={uiConfig?.placeholder || `请输入${schema.title || name}`}
          disabled={disabled}
          readOnly={readOnly}
          maxLength={schema.maxLength}
          iconRender={(visible) => (visible ? <EyeTwoTone /> : <EyeInvisibleOutlined />)}
          onFocus={() => setShowStrength(true)}
          onBlur={() => setShowStrength(false)}
        />
        
        {/* 密码强度指示器 */}
        {showStrengthIndicator && (showStrength || uiConfigExt?.alwaysShowStrength) && (
          <div style={{ marginTop: 4 }}>
            <Space size="small" align="center">
              <SafetyOutlined style={{ color: strength.color }} />
              <Text type="secondary" style={{ fontSize: 12 }}>
                密码强度：
              </Text>
              <Progress
                percent={strength.percent}
                size="small"
                strokeColor={strength.color}
                showInfo={false}
                style={{ width: 100, margin: 0 }}
              />
              <Text style={{ color: strength.color, fontSize: 12 }}>
                {strength.text}
              </Text>
            </Space>
          </div>
        )}
      </Space>
    </Form.Item>
  );
};

export default PasswordWidget;
