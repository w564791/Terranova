import React, { useCallback, useState, useRef } from 'react';
import { Form, Select, Tag, Tooltip } from 'antd';
import { LinkOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { ModuleReferencePopover } from '../../ManifestEditor/ModuleReferencePopover';

/**
 * TagsWidget - 标签输入组件
 * 
 * 注意：使用 Form.useWatch 获取表单值，让 Form.Item 自动管理值
 */
const TagsWidget: React.FC<WidgetProps> = ({
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
  const placeholder = uiConfig?.placeholder || `输入后按回车添加`;

  // 引用选择器状态
  const [referencePopoverOpen, setReferencePopoverOpen] = useState(false);
  const [popoverPosition, setPopoverPosition] = useState<{ x: number; y: number } | undefined>();
  const [searchValue, setSearchValue] = useState('');
  const selectRef = useRef<any>(null);

  // 检查是否包含 module 引用值（支持 ${module.xxx.yyy} 和 module.xxx.yyy 两种格式）
  const hasModuleReference = Array.isArray(formValue) && formValue.some((v: string) => 
    v.startsWith('${module.') || v.startsWith('module.')
  );

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;
  const hasOtherNodes = (manifestContext?.nodes?.length ?? 0) > 0;

  // 获取 Workspace 资源引用上下文
  const workspaceResourceContext = context?.workspaceResource;
  const hasWorkspaceResourceContext = !!workspaceResourceContext?.workspaceId;
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

  // 处理搜索值变化
  const handleSearch = useCallback((val: string) => {
    // 检测 "/" 触发引用选择器（支持 Manifest 和 Workspace 资源引用）
    if (hasReferenceContext && val.endsWith('/')) {
      // 获取输入框位置
      const selectElement = selectRef.current?.nativeElement;
      if (selectElement) {
        const rect = selectElement.getBoundingClientRect();
        setPopoverPosition({
          x: rect.left,
          y: rect.bottom + 4,
        });
      }
      setReferencePopoverOpen(true);
      // 移除末尾的 "/"
      setSearchValue(val.slice(0, -1));
      return;
    }
    setSearchValue(val);
  }, [hasReferenceContext]);

  // 处理引用选择
  const handleReferenceSelect = useCallback((reference: string, sourceNodeId: string, outputName: string) => {
    // 将引用包装成 Terraform 插值语法 ${...}
    const terraformReference = `\${${reference}}`;
    
    // 获取当前表单值（使用 form.getFieldValue 确保获取最新值）
    const currentValue = form.getFieldValue(name);
    const valueArray = Array.isArray(currentValue) ? currentValue : [];
    const newValue = [...valueArray, terraformReference];
    
    console.log('[TagsWidget] handleReferenceSelect:', {
      name,
      reference,
      terraformReference,
      currentValue,
      newValue,
    });
    
    // 添加引用到数组
    form.setFieldValue(name, newValue);
    
    // 手动触发表单验证以确保 onValuesChange 被调用
    form.validateFields([name]).catch(() => {});
    
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
    setSearchValue('');
  }, [form, name, manifestContext]);

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
            {hasReferenceContext && (
              <span style={{ color: '#1890ff', marginLeft: 8, fontSize: 11 }}>
                输入 / 可引用其他 {hasManifestContext ? 'Module' : '资源'} 的输出
              </span>
            )}
          </span>
        }
        rules={[
          ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
        ]}
      >
        <Select
          ref={selectRef}
          mode="tags"
          placeholder={placeholder}
          disabled={disabled || readOnly}
          tokenSeparators={[',', '\n']}
          allowClear
          style={{ width: '100%' }}
          searchValue={searchValue}
          onSearch={handleSearch}
          open={undefined}
          dropdownStyle={{ display: 'none' }}
          notFoundContent={null}
          tagRender={(props) => {
            const { label: tagLabel, closable, onClose } = props;
            const isRef = typeof tagLabel === 'string' && tagLabel.startsWith('module.');
            return (
              <Tag
                color={isRef ? 'blue' : undefined}
                closable={closable}
                onClose={onClose}
                style={{ marginRight: 3 }}
                icon={isRef ? <LinkOutlined /> : undefined}
              >
                {tagLabel}
              </Tag>
            );
          }}
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

export default TagsWidget;
