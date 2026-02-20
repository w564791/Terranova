import React, { useState, useRef, useCallback } from 'react';
import { Form, InputNumber, Input, Tag, Tooltip, Button, Space } from 'antd';
import { LinkOutlined, SwapOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { ModuleReferencePopover } from '../../ManifestEditor/ModuleReferencePopover';

/**
 * NumberWidget - 数字输入组件
 * 
 * 注意：使用 Form.useWatch 获取表单值，让 Form.Item 自动管理值
 */
const NumberWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
  context,
}) => {
  const form = Form.useFormInstance();
  const formValue = Form.useWatch(name, form);
  
  const label = uiConfig?.label || schema.title || name;
  const help = uiConfig?.help || schema.description;
  const placeholder = uiConfig?.placeholder || `请输入${label}`;

  // 引用选择器状态
  const [referencePopoverOpen, setReferencePopoverOpen] = useState(false);
  const [popoverPosition, setPopoverPosition] = useState<{ x: number; y: number } | undefined>();
  const [useTextInput, setUseTextInput] = useState(false);
  const inputRef = useRef<any>(null);

  // 检查是否是 module 引用值
  const isModuleReference = typeof formValue === 'string' && formValue.startsWith('module.');

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;
  const hasOtherNodes = (manifestContext?.nodes?.length ?? 0) > 0;

  // 如果值是引用，自动切换到文本输入模式
  React.useEffect(() => {
    if (isModuleReference && !useTextInput) {
      setUseTextInput(true);
    }
  }, [isModuleReference, useTextInput]);

  // 处理文本输入变化
  const handleTextChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value;
    
    // 检测 "/" 触发引用选择器
    if (hasManifestContext && hasOtherNodes && newValue.endsWith('/')) {
      // 获取输入框位置
      const inputElement = inputRef.current?.input;
      if (inputElement) {
        const rect = inputElement.getBoundingClientRect();
        setPopoverPosition({
          x: rect.left,
          y: rect.bottom + 4,
        });
      }
      setReferencePopoverOpen(true);
      // 移除末尾的 "/"
      form.setFieldValue(name, newValue.slice(0, -1));
      return;
    }
    
    // 尝试转换为数字
    const numValue = parseFloat(newValue);
    if (!isNaN(numValue) && !newValue.startsWith('module.')) {
      form.setFieldValue(name, numValue);
    } else {
      form.setFieldValue(name, newValue);
    }
  }, [hasManifestContext, hasOtherNodes, form, name]);

  // 处理引用选择
  const handleReferenceSelect = useCallback((reference: string, sourceNodeId: string, outputName: string) => {
    // 设置值为引用表达式
    form.setFieldValue(name, reference);
    
    // 通知父组件创建连线
    if (manifestContext?.onAddEdge) {
      manifestContext.onAddEdge(
        sourceNodeId,
        manifestContext.currentNodeId,
        outputName,
        name
      );
    }
    
    setReferencePopoverOpen(false);
  }, [form, name, manifestContext]);

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

  // 切换输入模式
  const toggleInputMode = () => {
    if (useTextInput && !isModuleReference) {
      // 切换回数字输入，尝试转换当前值
      const numValue = parseFloat(String(formValue));
      if (!isNaN(numValue)) {
        form.setFieldValue(name, numValue);
      } else {
        form.setFieldValue(name, undefined);
      }
    }
    setUseTextInput(!useTextInput);
  };

  // 如果有 Manifest 上下文且有其他节点，显示切换按钮
  const showToggleButton = hasManifestContext && hasOtherNodes;

  return (
    <>
      <Form.Item
        label={
          <span>
            {label}
            {renderReferenceTag()}
          </span>
        }
        name={name}
        help={
          <span>
            {help}
            {hasManifestContext && hasOtherNodes && !isModuleReference && (
              <span style={{ color: '#1890ff', marginLeft: 8, fontSize: 11 }}>
                {useTextInput ? '输入 / 可引用其他 Module 的输出' : '点击切换按钮可输入引用'}
              </span>
            )}
          </span>
        }
        rules={[
          ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
          ...(!useTextInput && schema.minimum !== undefined ? [{ type: 'number' as const, min: schema.minimum, message: `最小值为${schema.minimum}` }] : []),
          ...(!useTextInput && schema.maximum !== undefined ? [{ type: 'number' as const, max: schema.maximum, message: `最大值为${schema.maximum}` }] : []),
        ]}
      >
        {useTextInput ? (
          // 文本输入模式 - 手动管理值
          <Input
            ref={inputRef}
            value={String(formValue ?? '')}
            onChange={handleTextChange}
            placeholder={placeholder}
            disabled={disabled}
            readOnly={readOnly}
            style={isModuleReference ? { 
              fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
              color: '#1890ff',
            } : undefined}
            suffix={showToggleButton ? (
              <Tooltip title="切换为数字输入">
                <Button 
                  type="text"
                  size="small"
                  icon={<SwapOutlined />} 
                  onClick={toggleInputMode}
                  disabled={isModuleReference}
                />
              </Tooltip>
            ) : undefined}
          />
        ) : (
          // 数字输入模式 - 让 Form.Item 自动管理值
          <InputNumber
            placeholder={placeholder}
            disabled={disabled}
            readOnly={readOnly}
            min={schema.minimum}
            max={schema.maximum}
            style={{ width: '100%' }}
            addonAfter={showToggleButton ? (
              <Tooltip title="切换为文本输入（可输入引用）">
                <Button 
                  type="text"
                  size="small"
                  icon={<SwapOutlined />} 
                  onClick={toggleInputMode}
                />
              </Tooltip>
            ) : undefined}
          />
        )}
      </Form.Item>

      {/* 引用选择器弹出层 */}
      {hasManifestContext && (
        <ModuleReferencePopover
          open={referencePopoverOpen}
          onClose={() => setReferencePopoverOpen(false)}
          onSelect={handleReferenceSelect}
          currentNodeId={manifestContext.currentNodeId}
          nodes={manifestContext.nodes}
          connectedNodeIds={manifestContext.connectedNodeIds}
          position={popoverPosition}
        />
      )}
    </>
  );
};

export default NumberWidget;
