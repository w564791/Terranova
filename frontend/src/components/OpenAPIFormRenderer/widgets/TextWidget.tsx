import React, { useState, useRef, useCallback, useMemo } from 'react';
import { Form, Input, Tag, Tooltip } from 'antd';
import { LinkOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { ModuleReferencePopover } from '../../ManifestEditor/ModuleReferencePopover';
import type { Rule } from 'antd/es/form';

/**
 * x-validation 规则类型（来自 OpenAPI Schema）
 */
interface XValidationRule {
  condition?: string;      // Terraform 条件表达式
  errorMessage?: string;   // 错误提示信息
  pattern?: string;        // 正则表达式
}

/**
 * 从 x-validation 生成 Ant Design Form 验证规则
 */
function generateValidationRules(
  schema: any,
  label: string
): Rule[] {
  const rules: Rule[] = [];

  // 必填验证
  if (schema.required) {
    rules.push({ required: true, message: `${label}是必填项` });
  }

  // 最小长度
  if (schema.minLength !== undefined) {
    rules.push({ min: schema.minLength, message: `最少${schema.minLength}个字符` });
  }

  // 最大长度
  if (schema.maxLength !== undefined) {
    rules.push({ max: schema.maxLength, message: `最多${schema.maxLength}个字符` });
  }

  // 处理 x-validation 数组
  const xValidations: XValidationRule[] = schema['x-validation'] || [];
  for (const validation of xValidations) {
    // 如果有 pattern，使用正则验证
    if (validation.pattern) {
      rules.push({
        pattern: new RegExp(validation.pattern),
        message: validation.errorMessage || '格式不正确',
      });
    }
    // 如果只有 condition（Terraform 表达式），暂时跳过（需要后端验证）
    // 但仍然可以显示 errorMessage 作为提示
  }

  // 如果 schema 本身有 pattern 但 x-validation 中没有
  if (schema.pattern && !xValidations.some((v: XValidationRule) => v.pattern === schema.pattern)) {
    rules.push({
      pattern: new RegExp(schema.pattern),
      message: xValidations[0]?.errorMessage || '格式不正确',
    });
  }

  return rules;
}

/**
 * TextWidget - 文本输入组件
 * 
 * 注意：使用 Form.useWatch 获取表单值，而不是依赖 props.value
 * 这样可以确保值与 Form 状态同步，同时支持模块引用功能
 */
const TextWidget: React.FC<WidgetProps> = ({
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

  // 生成验证规则
  const validationRules = useMemo(() => generateValidationRules(schema, label), [schema, label]);

  // 获取验证提示信息（从 x-validation 中提取）
  const validationHints = useMemo(() => {
    const xValidations: XValidationRule[] = schema['x-validation'] || [];
    return xValidations
      .filter((v: XValidationRule) => v.errorMessage)
      .map((v: XValidationRule) => v.errorMessage);
  }, [schema]);
  
  // 引用选择器状态
  const [referencePopoverOpen, setReferencePopoverOpen] = useState(false);
  const [popoverPosition, setPopoverPosition] = useState<{ x: number; y: number } | undefined>();
  const [pendingSlashRemoval, setPendingSlashRemoval] = useState(false);
  const inputRef = useRef<any>(null);

  // 检查是否是 module 引用值（支持 ${module.xxx.yyy} 和 module.xxx.yyy 两种格式）
  const isModuleReference = typeof formValue === 'string' && 
    (formValue.startsWith('${module.') || formValue.startsWith('module.'));

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  // 只要有 currentNodeId 就认为在 Manifest 编辑器中，即使没有其他节点可引用
  const hasManifestContext = !!manifestContext?.currentNodeId;
  // 是否有其他节点可引用
  const hasOtherNodes = (manifestContext?.nodes?.length ?? 0) > 0;

  // 获取 Workspace 资源引用上下文
  const workspaceResourceContext = context?.workspaceResource;
  const hasWorkspaceResourceContext = !!workspaceResourceContext?.workspaceId;
  // 是否有其他资源可引用（排除当前资源）
  const hasOtherResources = (workspaceResourceContext?.resources?.length ?? 0) > 0;

  // 合并判断：是否支持引用功能
  const hasReferenceContext = (hasManifestContext && hasOtherNodes) || (hasWorkspaceResourceContext && hasOtherResources);
  
  // 将 workspace 资源转换为 ModuleNodeInfo 格式（复用 ModuleReferencePopover）
  const referenceNodes = hasManifestContext 
    ? manifestContext!.nodes 
    : (workspaceResourceContext?.resources?.map(r => ({
        id: r.id,
        instance_name: r.resource_name,
        module_id: r.module_id,
        module_source: r.module_source,
        outputs: r.outputs,
      })) || []);
  
  const currentNodeId = hasManifestContext 
    ? manifestContext!.currentNodeId 
    : (workspaceResourceContext?.currentResourceId || '');

  // 保存输入 / 之前的值，用于在用户取消选择时恢复
  const valueBeforeSlashRef = useRef<string>('');

  // 处理输入变化 - 检测 / 触发引用选择器
  const handleInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value;
    
    // 检测 "/" 触发引用选择器（支持 Manifest 和 Workspace 资源引用）
    if (hasReferenceContext && newValue.endsWith('/')) {
      // 保存 / 之前的值
      valueBeforeSlashRef.current = newValue.slice(0, -1);
      
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
      setPendingSlashRemoval(true);
    }
    
    // 让 Form.Item 自动处理值更新
  }, [hasReferenceContext]);

  // 使用 getValueFromEvent 来处理值转换，移除末尾的 /
  const getValueFromEvent = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    // 如果正在等待移除 /，返回移除 / 后的值
    if (pendingSlashRemoval && value.endsWith('/')) {
      setPendingSlashRemoval(false);
      return value.slice(0, -1);
    }
    return value;
  }, [pendingSlashRemoval]);

  // 处理引用选择
  const handleReferenceSelect = useCallback((reference: string, sourceNodeId: string, outputName: string) => {
    // 将引用包装成 Terraform 插值语法 ${...}
    const terraformReference = `\${${reference}}`;
    
    console.log('[TextWidget] handleReferenceSelect:', {
      name,
      reference,
      terraformReference,
      sourceNodeId,
      outputName,
      hasOnAddEdge: !!manifestContext?.onAddEdge,
      currentNodeId: manifestContext?.currentNodeId,
    });
    
    // 设置值为 Terraform 引用表达式
    form.setFieldValue(name, terraformReference);
    
    // 关键修复：调用 triggerValuesUpdate 手动通知 FormRenderer 值已更新
    // 因为 form.setFieldValue 不会触发 onValuesChange
    setTimeout(() => {
      if (context?.triggerValuesUpdate) {
        console.log('[TextWidget] Calling triggerValuesUpdate');
        context.triggerValuesUpdate();
      }
    }, 0);
    
    // 通知父组件创建连线
    if (manifestContext?.onAddEdge) {
      console.log('[TextWidget] Calling onAddEdge:', sourceNodeId, '->', manifestContext.currentNodeId);
      manifestContext.onAddEdge(
        sourceNodeId,
        manifestContext.currentNodeId,
        outputName,
        name
      );
    }
    
    setReferencePopoverOpen(false);
  }, [form, name, manifestContext, context]);

  // 渲染引用标签
  const renderReferenceTag = () => {
    if (!isModuleReference) return null;
    
    // 解析引用：module.instance_name.output_name
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
            {renderReferenceTag()}
          </span>
        }
        name={name}
        help={
          <span>
            {help}
            {validationHints.length > 0 && (
              <span style={{ color: '#8c8c8c', display: 'block', fontSize: 12, marginTop: 4 }}>
                验证规则: {validationHints.join('; ')}
              </span>
            )}
            {hasReferenceContext && !isModuleReference && (
              <span style={{ color: '#1890ff', marginLeft: 8, fontSize: 11 }}>
                输入 / 可引用其他 {hasManifestContext ? 'Module' : '资源'} 的输出
              </span>
            )}
          </span>
        }
        rules={validationRules}
        getValueFromEvent={getValueFromEvent}
      >
        <Input
          ref={inputRef}
          onChange={handleInputChange}
          placeholder={placeholder}
          disabled={disabled}
          readOnly={readOnly}
          allowClear
          style={isModuleReference ? { 
            fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
            color: '#1890ff',
          } : undefined}
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
        />
      )}
    </>
  );
};

export default TextWidget;
