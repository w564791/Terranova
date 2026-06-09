import React from 'react';
import { Form, Switch, Tag } from 'antd';
import type { WidgetProps } from '../types';
import { useUIVersionContext } from '../../../contexts/UIVersionContext';

/**
 * SwitchWidget - 开关组件
 *
 * V3 增强：显示状态文字"已启用/未启用"，统一蓝色主色
 * V2 模式：保持原有行为不变
 *
 * 关键：Form.Item 通过 name 注入 value/onChange 到直接子元素。
 * V3 模式下 Switch 必须是 Form.Item 的直接子元素，文字标签放在 Form.Item 之外。
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
  const { isV3 } = useUIVersionContext();
  const label = uiConfig?.label || schema.title || name;
  const help = uiConfig?.help || schema.description;

  const checkedHint = uiConfig?.checkedHint;
  const checkedHintColor = uiConfig?.checkedHintColor;
  const uncheckedHint = uiConfig?.uncheckedHint;
  const uncheckedHintColor = uiConfig?.uncheckedHintColor;
  const hasHint = checkedHint || uncheckedHint;

  // V3 模式：Switch 必须是 Form.Item 的直接子元素，
  // Form.Item 通过 name 注入 checked/onChange。
  // 状态文字通过 Form.useWatch 读取值，放在 Form.Item 之外。
  if (isV3) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
        <Form.Item
          label={label}
          name={name}
          help={help}
          valuePropName="checked"
          style={{ marginBottom: 0 }}
          rules={[
            ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
          ]}
        >
          <Switch
            disabled={disabled || readOnly}
          />
        </Form.Item>
        <SwitchV3Label name={name} />
      </div>
    );
  }

  // V2 模式：保持原有行为
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

/**
 * V3 状态文字标签 — 独立组件，使用 Form.useWatch 监听值
 */
const SwitchV3Label: React.FC<{ name: string }> = ({ name }) => {
  const form = Form.useFormInstance();
  const checked = Boolean(Form.useWatch(name, form));

  return (
    <span
      style={{
        fontSize: '12.5px',
        fontWeight: 450,
        color: checked ? '#3b82f6' : '#9ca3af',
        userSelect: 'none',
        marginTop: '22px', // align with switch (account for Form.Item label height)
      }}
    >
      {checked ? '已启用' : '未启用'}
    </span>
  );
};

export default SwitchWidget;
