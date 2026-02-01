import React from 'react';
import { Form, Switch, Tag } from 'antd';
import type { WidgetProps } from '../types';

/**
 * SwitchWidget - 开关组件
 * 
 * 注意：Form.Item 会自动将表单值注入到子组件的 checked 属性中（通过 valuePropName="checked"）
 * 不需要手动设置 checked 和 onChange，让 Form.Item 自动处理
 * 
 * 支持的 uiConfig 配置：
 * - checkedHint: 开关打开时显示的提示文本（默认：无）
 * - checkedHintColor: 开关打开时提示的颜色（默认：green）
 * - uncheckedHint: 开关关闭时显示的提示文本（默认：无）
 * - uncheckedHintColor: 开关关闭时提示的颜色（默认：default）
 */

// 内部组件：带提示的开关
interface SwitchWithHintProps {
  checked?: boolean;
  onChange?: (checked: boolean) => void;
  disabled?: boolean;
  checkedHint?: string;
  checkedHintColor?: string;
  uncheckedHint?: string;
  uncheckedHintColor?: string;
}

const SwitchWithHint: React.FC<SwitchWithHintProps> = ({
  checked,
  onChange,
  disabled,
  checkedHint,
  checkedHintColor = 'green',
  uncheckedHint,
  uncheckedHintColor = 'default',
}) => {
  const currentHint = checked ? checkedHint : uncheckedHint;
  const currentColor = checked ? checkedHintColor : uncheckedHintColor;

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
      <Switch 
        checked={checked}
        onChange={onChange}
        disabled={disabled}
      />
      {currentHint && (
        <Tag color={currentColor} style={{ margin: 0 }}>
          {currentHint}
        </Tag>
      )}
    </div>
  );
};

const SwitchWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
}) => {
  const label = uiConfig?.label || schema.title || name;
  const help = uiConfig?.help || schema.description;
  
  // 获取开关打开/关闭时的提示配置
  const checkedHint = uiConfig?.checkedHint;
  const checkedHintColor = uiConfig?.checkedHintColor;
  const uncheckedHint = uiConfig?.uncheckedHint;
  const uncheckedHintColor = uiConfig?.uncheckedHintColor;

  // 如果没有配置任何提示，使用简单的 Switch
  const hasHint = checkedHint || uncheckedHint;

  return (
    <Form.Item
      label={label}
      name={name}
      help={help}
      valuePropName="checked"
      rules={[
        ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
      ]}
    >
      {hasHint ? (
        <SwitchWithHint
          disabled={disabled || readOnly}
          checkedHint={checkedHint}
          checkedHintColor={checkedHintColor}
          uncheckedHint={uncheckedHint}
          uncheckedHintColor={uncheckedHintColor}
        />
      ) : (
        <Switch disabled={disabled || readOnly} />
      )}
    </Form.Item>
  );
};

export default SwitchWidget;
