import React, { useState, useRef, useCallback, useMemo, useEffect } from 'react';
import { Form, Select, Spin, Tag, Tooltip, Button } from 'antd';
import { LinkOutlined, ReloadOutlined } from '@ant-design/icons';
import type { WidgetProps, SelectOption, ExternalDataSource } from '../types';
import { ModuleReferencePopover } from '../../ModuleReference/ModuleReferencePopover';
import { useSingleDataSource } from '../useExternalDataSource';

const { Option } = Select;

interface SelectWidgetProps extends WidgetProps {
  options?: SelectOption[];
  loading?: boolean;
  onSearch?: (value: string) => void;
  mode?: 'multiple' | 'tags';
}

/**
 * SelectWidget - 下拉选择组件
 * 
 * 支持：
 * 1. 静态选项（schema.enum 或 externalOptions）
 * 2. 外部数据源（uiConfig.source）
 * 3. 模块引用（Manifest 上下文）
 */
const SelectWidget: React.FC<SelectWidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
  options: externalOptions,
  loading: externalLoading,
  onSearch,
  mode,
  context,
}) => {
  const form = Form.useFormInstance();
  const formValue = Form.useWatch(name, form);
  
  const label = uiConfig?.label || schema.title || name;
  const help = uiConfig?.help || schema.description;
  const placeholder = uiConfig?.placeholder || `请选择${label}`;

  // 引用选择器状态
  const [referencePopoverOpen, setReferencePopoverOpen] = useState(false);
  const [popoverPosition, setPopoverPosition] = useState<{ x: number; y: number } | undefined>();
  const [searchValue, setSearchValue] = useState('');
  const selectRef = useRef<any>(null);

  // 检查是否是 module 引用值
  const isModuleReference = typeof formValue === 'string' && formValue.startsWith('module.');
  const hasModuleReference = Array.isArray(formValue) && formValue.some((v: string) => typeof v === 'string' && v.startsWith('module.'));

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;
  const hasOtherNodes = (manifestContext?.nodes?.length ?? 0) > 0;

  // 构建外部数据源配置
  const externalSource = useMemo((): ExternalDataSource | undefined => {
    const sourceId = uiConfig?.source || uiConfig?.externalSource;
    if (!sourceId) return undefined;

    // 首先检查 schema 中是否有预定义的数据源配置
    const predefinedSources = context?.schema?.['x-iac-platform']?.external?.sources || [];
    const predefined = predefinedSources.find((s: ExternalDataSource) => s.id === sourceId);
    if (predefined) return predefined;

    // 如果没有预定义，根据 sourceId 构建默认的 API 数据源
    // 支持的格式：
    // - ami_list -> /api/v1/external-data/ami_list
    // - instance_types -> /api/v1/external-data/instance_types
    return {
      id: sourceId,
      type: 'api',
      api: `/api/v1/external-data/${sourceId}`,
      cache: { ttl: 300 },
      transform: {
        type: 'jmespath',
        expression: 'data[*].{value: value, label: label, description: description}',
      },
    };
  }, [uiConfig?.source, uiConfig?.externalSource, context?.schema]);

  // 使用外部数据源 hook
  const {
    options: sourceOptions,
    loading: sourceLoading,
    error: sourceError,
    reload: reloadSource,
  } = useSingleDataSource(externalSource, context || {
    values: {},
    errors: {},
    touched: {},
    schema: {} as any,
  });

  // 合并选项：外部数据源 > 传入的 options > schema.enum
  const options: SelectOption[] = useMemo(() => {
    // 如果有外部数据源且已加载数据，优先使用
    if (externalSource && sourceOptions.length > 0) {
      return sourceOptions;
    }
    // 否则使用传入的 options
    if (externalOptions && externalOptions.length > 0) {
      return externalOptions;
    }
    // 最后使用 schema.enum
    if (schema.enum && schema.enum.length > 0) {
      return schema.enum.map(v => ({ value: v, label: v }));
    }
    return [];
  }, [externalSource, sourceOptions, externalOptions, schema.enum]);

  // 合并加载状态
  const loading = externalLoading || sourceLoading;

  // 调试日志
  useEffect(() => {
    if (externalSource) {
      console.log(`SelectWidget "${name}" using external source:`, {
        sourceId: externalSource.id,
        api: externalSource.api,
        loading: sourceLoading,
        optionsCount: sourceOptions.length,
        error: sourceError,
      });
    }
  }, [name, externalSource, sourceLoading, sourceOptions.length, sourceError]);

  const searchable = uiConfig?.searchable !== false;
  const showRefreshButton = uiConfig?.refreshButton !== false && !!externalSource;

  // 处理搜索
  const handleSearch = useCallback((val: string) => {
    // 检测 "/" 触发引用选择器
    if (hasManifestContext && hasOtherNodes && val.endsWith('/')) {
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
    onSearch?.(val);
  }, [hasManifestContext, hasOtherNodes, onSearch]);

  // 处理引用选择
  const handleReferenceSelect = useCallback((reference: string, sourceNodeId: string, outputName: string) => {
    // 将引用包装成 Terraform 插值语法 ${...}
    const terraformReference = `\${${reference}}`;
    
    console.log('[SelectWidget] handleReferenceSelect:', {
      name,
      reference,
      terraformReference,
      sourceNodeId,
      outputName,
      hasOnAddEdge: !!manifestContext?.onAddEdge,
      currentNodeId: manifestContext?.currentNodeId,
    });
    
    // 设置值为引用表达式
    if (mode === 'multiple' || mode === 'tags') {
      const currentValue = Array.isArray(formValue) ? formValue : [];
      form.setFieldValue(name, [...currentValue, terraformReference]);
    } else {
      form.setFieldValue(name, terraformReference);
    }
    
    // 关键修复：调用 triggerValuesUpdate 手动通知 FormRenderer 值已更新
    // 因为 form.setFieldValue 不会触发 onValuesChange
    setTimeout(() => {
      if (context?.triggerValuesUpdate) {
        console.log('[SelectWidget] Calling triggerValuesUpdate');
        context.triggerValuesUpdate();
      }
    }, 0);
    
    // 通知父组件创建连线
    if (manifestContext?.onAddEdge) {
      console.log('[SelectWidget] Calling onAddEdge:', sourceNodeId, '->', manifestContext.currentNodeId);
      manifestContext.onAddEdge(
        sourceNodeId,
        manifestContext.currentNodeId,
        outputName,
        name
      );
    }
    
    setReferencePopoverOpen(false);
    setSearchValue('');
  }, [formValue, form, name, manifestContext, mode, context]);

  // 渲染引用标签
  const renderReferenceTag = () => {
    if (!isModuleReference && !hasModuleReference) return null;
    
    if (isModuleReference) {
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
    }
    
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
            {showRefreshButton && (
              <Tooltip title="刷新选项">
                <Button
                  type="text"
                  size="small"
                  icon={<ReloadOutlined spin={sourceLoading} />}
                  onClick={(e) => {
                    e.stopPropagation();
                    reloadSource(true);
                  }}
                  style={{ marginLeft: 4 }}
                  disabled={sourceLoading}
                />
              </Tooltip>
            )}
          </span>
        }
        name={name}
        help={
          <span>
            {help}
            {sourceError && (
              <span style={{ color: 'var(--red)', marginLeft: 8, fontSize: 11 }}>
                加载失败: {sourceError}
              </span>
            )}
            {hasManifestContext && hasOtherNodes && !isModuleReference && (
              <span style={{ color: 'var(--brand)', marginLeft: 8, fontSize: 11 }}>
                搜索时输入 / 可引用其他 Module 的输出
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
          placeholder={placeholder}
          disabled={disabled || readOnly}
          mode={mode}
          showSearch={searchable}
          searchValue={searchValue}
          onSearch={handleSearch}
          loading={loading}
          allowClear
          filterOption={searchable ? (input, option) => {
            const val = String(option?.value ?? '');
            const label = String(option?.label ?? option?.children ?? '');
            const q = input.toLowerCase();
            return val.toLowerCase().includes(q) || label.toLowerCase().includes(q);
          } : undefined}
          notFoundContent={loading ? <Spin size="small" /> : '暂无数据'}
          style={isModuleReference ? { 
            fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
          } : undefined}
        >
          {/* 如果当前值是引用，添加一个选项显示它 */}
          {isModuleReference && (
            <Option key={formValue as string} value={formValue as string}>
              <Tag color="blue" icon={<LinkOutlined />} style={{ marginRight: 4 }}>
                引用
              </Tag>
              {formValue as string}
            </Option>
          )}
          {options.map(opt => (
            <Option 
              key={opt.value} 
              value={opt.value}
              disabled={opt.disabled}
            >
              {opt.label}
              {opt.description && (
                <span style={{ color: 'var(--ink-3)', marginLeft: 8, fontSize: 12 }}>
                  {opt.description}
                </span>
              )}
            </Option>
          ))}
        </Select>
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

export default SelectWidget;
