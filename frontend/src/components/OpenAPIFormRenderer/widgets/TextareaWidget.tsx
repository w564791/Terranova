import React, { useMemo, useState, useCallback, useRef } from 'react';
import { Form, Input, Tag, Tooltip } from 'antd';
import { LinkOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { ModuleReferencePopover } from '../../ManifestEditor/ModuleReferencePopover';

const { TextArea } = Input;

/**
 * TextareaWidget - 多行文本输入组件
 * 
 * 支持两种模式：
 * 1. 字符串模式：直接输入多行文本
 * 2. 数组模式：每行一个元素，自动转换为数组
 * 
 * 对于数组模式，使用内部状态管理文本输入，
 * 只在失去焦点或提交时才将文本转换为数组。
 * 
 * 支持 "/" 触发模块引用选择器
 */
const TextareaWidget: React.FC<WidgetProps> = ({
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
  
  // 检测是否是数组类型
  const isArrayType = schema.type === 'array';
  
  // 内部文本状态（用于数组类型时保留用户输入的换行符）
  const [internalText, setInternalText] = useState<string>('');
  const [isFocused, setIsFocused] = useState(false);
  
  // 引用选择器状态
  const [referencePopoverOpen, setReferencePopoverOpen] = useState(false);
  const [popoverPosition, setPopoverPosition] = useState<{ x: number; y: number } | undefined>();
  const inputRef = useRef<any>(null);

  // 检查是否是 module 引用值（支持 ${module.xxx.yyy} 和 module.xxx.yyy 两种格式）
  const isModuleReference = typeof formValue === 'string' && 
    (formValue.startsWith('${module.') || formValue.startsWith('module.'));

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;
  const hasOtherNodes = (manifestContext?.nodes?.length ?? 0) > 0;

  // 获取 Workspace 资源引用上下文
  const workspaceResourceContext = context?.workspaceResource;
  const hasWorkspaceResourceContext = !!workspaceResourceContext?.workspaceId;
  const hasOtherResources = (workspaceResourceContext?.resources?.length ?? 0) > 0;
  const hasRemoteData = (workspaceResourceContext?.remoteData?.length ?? 0) > 0;

  // 合并判断：是否支持引用功能（本地资源或远程数据）
  const hasReferenceContext = (hasManifestContext && hasOtherNodes) || (hasWorkspaceResourceContext && (hasOtherResources || hasRemoteData));
  
  // 将 workspace 资源转换为 ModuleNodeInfo 格式（复用 ModuleReferencePopover）
  // instance_name 用于生成引用（完整的 tf_module_key）
  // display_name 用于显示（友好的 resource_name）
  const referenceNodes = hasManifestContext 
    ? manifestContext!.nodes 
    : (workspaceResourceContext?.resources?.map(r => ({
        id: r.id,
        // instance_name 用于生成引用，使用完整的 tf_module_key
        instance_name: r.tf_module_key || r.resource_name,
        // display_name 用于显示，使用友好的 resource_name
        display_name: r.resource_name,
        module_id: r.module_id,
        module_source: r.module_source,
        outputs: r.outputs,
      })) || []);
  
  const currentNodeId = hasManifestContext 
    ? manifestContext!.currentNodeId 
    : (workspaceResourceContext?.currentResourceId || '');
  
  // 根据类型生成占位符
  const placeholder = useMemo(() => {
    if (uiConfig?.placeholder) {
      return uiConfig.placeholder;
    }
    if (isArrayType) {
      return `每行输入一个值，例如：\nsg-12345678\nsg-87654321`;
    }
    return `请输入${label}`;
  }, [uiConfig?.placeholder, isArrayType, label]);

  // 生成帮助文本
  const helpText = useMemo(() => {
    const baseHelp = help ? (isArrayType ? `${help}（每行一个值）` : help) : (isArrayType ? '每行输入一个值' : undefined);
    return baseHelp;
  }, [help, isArrayType]);

  // 将数组转换为文本
  const arrayToText = useCallback((val: unknown): string => {
    if (Array.isArray(val)) {
      return val.join('\n');
    }
    if (typeof val === 'string') {
      return val;
    }
    return '';
  }, []);

  // 将文本转换为数组（过滤空行）
  const textToArray = useCallback((text: string): string[] => {
    return text
      .split('\n')
      .map(line => line.trim())
      .filter(line => line.length > 0);
  }, []);

  // 将存储的值（数组或字符串）转换为显示文本
  const getValueProps = useCallback((val: unknown) => {
    if (isArrayType) {
      // 如果正在编辑，使用内部状态
      if (isFocused) {
        return { value: internalText };
      }
      // 否则从存储的数组值转换
      return { value: arrayToText(val) };
    }
    // 字符串类型：直接返回
    return { value: typeof val === 'string' ? val : '' };
  }, [isArrayType, isFocused, internalText, arrayToText]);

  // 从输入事件中提取值
  const getValueFromEvent = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const text = e.target.value;
    
    // 检测 "/" 触发引用选择器（支持 Manifest 和 Workspace 资源引用）
    // 支持在任意位置输入 /，包括换行后
    if (hasReferenceContext && text.endsWith('/')) {
      // 获取输入框位置
      const inputElement = inputRef.current?.resizableTextArea?.textArea;
      if (inputElement) {
        const rect = inputElement.getBoundingClientRect();
        setPopoverPosition({
          x: rect.left,
          y: rect.bottom + 4,
        });
      }
      setReferencePopoverOpen(true);
      // 移除末尾的 "/"
      const newText = text.slice(0, -1);
      if (isArrayType) {
        setInternalText(newText);
      }
      return newText;
    }
    
    if (isArrayType) {
      // 更新内部文本状态
      setInternalText(text);
      return text;
    }
    return text;
  }, [isArrayType, hasReferenceContext]);

  // normalize 函数：将文本转换为数组
  const normalize = useCallback((val: unknown) => {
    if (isArrayType) {
      if (typeof val === 'string') {
        return textToArray(val);
      }
      if (Array.isArray(val)) {
        return val;
      }
      return [];
    }
    return val;
  }, [isArrayType, textToArray]);

  // 处理焦点事件
  const handleFocus = useCallback((e: React.FocusEvent<HTMLTextAreaElement>) => {
    setIsFocused(true);
    setInternalText(e.target.value);
  }, []);

  // 处理失去焦点事件
  const handleBlur = useCallback(() => {
    setIsFocused(false);
  }, []);

  // 处理引用选择
  const handleReferenceSelect = useCallback((reference: string, sourceNodeId: string, outputName: string) => {
    // 将引用包装成 Terraform 插值语法 ${...}
    const terraformReference = `\${${reference}}`;
    
    // 获取当前文本内容（优先使用 internalText，因为它保存了用户正在编辑的内容）
    // 如果 internalText 为空，则从表单值转换
    const currentFormValue = form.getFieldValue(name);
    const currentText = internalText || arrayToText(currentFormValue);
    
    // 追加引用到当前文本末尾（新行）
    const newText = currentText ? `${currentText}\n${terraformReference}` : terraformReference;
    
    console.log('[TextAreaWidget] handleReferenceSelect:', {
      name,
      reference,
      terraformReference,
      internalText,
      currentFormValue,
      currentText,
      newText,
      isArrayType,
    });
    
    // 更新内部文本状态
    setInternalText(newText);
    
    if (isArrayType) {
      // 数组类型：将文本转换为数组后设置
      const newArray = textToArray(newText);
      
      console.log('[TextAreaWidget] Setting array value:', newArray);
      
      // 使用 setFieldsValue 而不是 setFieldValue，确保值被正确设置
      form.setFieldsValue({ [name]: newArray });
      
      // 使用 setFields 而不是 setFieldsValue，因为 setFields 可以触发验证
      // 但实际上 setFields 也不会触发 onValuesChange
      // 所以我们需要使用另一种方法：直接触发输入事件
      
      // 使用 setTimeout 确保值已经被设置
      setTimeout(() => {
        const currentValue = form.getFieldValue(name);
        console.log('[TextAreaWidget] Current value after setFieldsValue:', currentValue);
        
        // 如果值没有被正确设置，再次尝试
        if (!Array.isArray(currentValue) || currentValue.length !== newArray.length) {
          console.log('[TextAreaWidget] Value not set correctly, retrying...');
          form.setFieldsValue({ [name]: newArray });
        }
        
        // 关键修复：使用 setFields 并设置 touched 为 true，然后触发验证
        // 这会导致 Form 重新评估字段值
        form.setFields([{
          name: name,
          value: newArray,
          touched: true,
        }]);
        
        // 关键修复：调用 triggerValuesUpdate 手动通知 FormRenderer 值已更新
        // 因为 form.setFieldsValue 不会触发 onValuesChange
        if (context?.triggerValuesUpdate) {
          console.log('[TextAreaWidget] Calling triggerValuesUpdate');
          context.triggerValuesUpdate();
        }
      }, 10);
    } else {
      // 字符串类型：直接设置文本
      form.setFieldsValue({ [name]: newText });
      
      setTimeout(() => {
        // 关键修复：调用 triggerValuesUpdate 手动通知 FormRenderer 值已更新
        if (context?.triggerValuesUpdate) {
          console.log('[TextAreaWidget] Calling triggerValuesUpdate');
          context.triggerValuesUpdate();
        }
      }, 10);
    }
    
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
  }, [form, name, manifestContext, isArrayType, internalText, arrayToText, textToArray]);

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

  return (
    <>
      <Form.Item
        label={
          <span>
            {label}
            {isArrayType && (
              <span style={{ 
                marginLeft: 8, 
                fontSize: 12, 
                color: '#8c8c8c',
                fontWeight: 'normal'
              }}>
                (数组)
              </span>
            )}
            {renderReferenceTag()}
          </span>
        }
        name={name}
        help={
          <span>
            {helpText}
            {hasReferenceContext && !isModuleReference && (
              <span style={{ color: '#1890ff', marginLeft: 8, fontSize: 11 }}>
                输入 / 可引用其他 {hasManifestContext ? 'Module' : '资源'} 的输出
              </span>
            )}
          </span>
        }
        getValueProps={getValueProps}
        getValueFromEvent={getValueFromEvent}
        normalize={normalize}
        rules={[
          ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
          ...(isArrayType && schema.minItems ? [{
            validator: async (_: unknown, val: unknown) => {
              const arr = Array.isArray(val) ? val : (typeof val === 'string' ? textToArray(val) : []);
              if (arr.length < (schema.minItems || 0)) {
                throw new Error(`至少需要 ${schema.minItems} 个值`);
              }
            }
          }] : []),
          ...(isArrayType && schema.maxItems ? [{
            validator: async (_: unknown, val: unknown) => {
              const arr = Array.isArray(val) ? val : (typeof val === 'string' ? textToArray(val) : []);
              if (arr.length > (schema.maxItems || Infinity)) {
                throw new Error(`最多允许 ${schema.maxItems} 个值`);
              }
            }
          }] : []),
        ]}
      >
        <TextArea
          ref={inputRef}
          placeholder={placeholder}
          disabled={disabled}
          readOnly={readOnly}
          autoSize={{ minRows: 3, maxRows: 10 }}
          style={isModuleReference ? { 
            fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
            color: '#1890ff',
          } : { fontFamily: 'monospace' }}
          onFocus={handleFocus}
          onBlur={handleBlur}
        />
      </Form.Item>

      {/* 引用选择器弹出层 - 支持 Manifest 和 Workspace 资源引用 */}
      {hasReferenceContext && (
        <ModuleReferencePopover
          open={referencePopoverOpen}
          onClose={() => setReferencePopoverOpen(false)}
          onSelect={handleReferenceSelect}
          currentNodeId={currentNodeId}
          nodes={referenceNodes}
          connectedNodeIds={hasManifestContext ? manifestContext?.connectedNodeIds : undefined}
          position={popoverPosition}
          remoteData={workspaceResourceContext?.remoteData}
        />
      )}
    </>
  );
};

export default TextareaWidget;
