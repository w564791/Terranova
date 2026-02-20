import React, { useState, useCallback, useMemo, useEffect, useRef } from 'react';
import { Form, AutoComplete, Select, Spin, Tag, Tooltip, Input } from 'antd';
import { DatabaseOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { cmdbService, type ResourceSearchResult } from '../../../services/cmdb';
import { ModuleReferencePopover } from '../../ManifestEditor/ModuleReferencePopover';

// CMDB 字段定义
const CMDB_FIELD_LABELS: Record<string, string> = {
  cloud_id: '资源 ID',
  cloud_arn: 'ARN',
  cloud_name: '资源名称',
  cloud_region: '区域',
  cloud_account: '账户 ID',
  terraform_address: 'Terraform 地址',
  description: '描述',
};

// 从搜索结果中提取指定字段的值
const extractFieldValue = (result: ResourceSearchResult, valueField: string): string => {
  switch (valueField) {
    case 'cloud_id':
      return result.cloud_resource_id || '';
    case 'cloud_arn':
      return result.cloud_resource_arn || '';
    case 'cloud_name':
      return result.cloud_resource_name || '';
    case 'cloud_region':
      return result.cloud_region || '';
    case 'cloud_account':
      return result.cloud_account_id || '';
    case 'terraform_address':
      return result.terraform_address || '';
    case 'description':
      return result.description || '';
    default:
      return result.cloud_resource_id || '';
  }
};

interface CMDBSelectWidgetProps extends WidgetProps {
  mode?: 'single' | 'multiple';
}

/**
 * CMDBSelectWidget - CMDB 资源选择组件
 * 
 * 支持两种模式：
 * 1. 单值模式（string 类型）：AutoComplete 组件
 * 2. 多值模式（array 类型）：Select 组件 + 搜索
 * 
 * 用户可以从 CMDB 搜索选择，也可以直接输入任意值
 */
const CMDBSelectWidget: React.FC<CMDBSelectWidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
  context,
}) => {
  // 判断是否是数组类型
  const isArrayType = schema.type === 'array';
  
  const form = Form.useFormInstance();
  const formValue = Form.useWatch(name, form);
  
  const label = uiConfig?.label || schema.title || name;
  const help = uiConfig?.help || schema.description;
  const placeholder = uiConfig?.placeholder || `输入${label}`;

  // CMDB 配置
  const cmdbSource = uiConfig?.cmdbSource;
  const resourceType = cmdbSource?.resourceType || '';
  const valueField = cmdbSource?.valueField || 'cloud_id';
  const valueFieldLabel = CMDB_FIELD_LABELS[valueField] || valueField;

  // 状态
  const [searchResults, setSearchResults] = useState<ResourceSearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchValue, setSearchValue] = useState('');
  const [selectDropdownOpen, setSelectDropdownOpen] = useState(false);  // 控制 Select 下拉菜单
  
  // 引用选择器状态
  const [referencePopoverOpen, setReferencePopoverOpen] = useState(false);
  const [popoverPosition, setPopoverPosition] = useState<{ x: number; y: number } | undefined>();
  const inputRef = useRef<any>(null);

  // 获取 Workspace 资源引用上下文
  const workspaceResourceContext = context?.workspaceResource;
  const hasWorkspaceResourceContext = !!workspaceResourceContext?.workspaceId;
  const hasOtherResources = (workspaceResourceContext?.resources?.length ?? 0) > 0;
  const hasRemoteData = (workspaceResourceContext?.remoteData?.length ?? 0) > 0;
  // 有本地资源或远程数据时，都可以使用引用选择器
  const hasReferenceContext = hasWorkspaceResourceContext && (hasOtherResources || hasRemoteData);
  
  // 调试日志：检查 context 和 workspaceResource
  console.log('[CMDBSelectWidget] Debug context:', {
    name,
    hasContext: !!context,
    hasWorkspaceResource: !!context?.workspaceResource,
    workspaceId: workspaceResourceContext?.workspaceId,
    currentResourceId: workspaceResourceContext?.currentResourceId,
    resourcesCount: workspaceResourceContext?.resources?.length ?? 0,
    remoteDataCount: workspaceResourceContext?.remoteData?.length ?? 0,
    hasWorkspaceResourceContext,
    hasOtherResources,
    hasRemoteData,
    hasReferenceContext,
  });
  
  // 将 workspace 资源转换为 ModuleNodeInfo 格式
  // instance_name 用于生成引用（完整的 tf_module_key）
  // display_name 用于显示（友好的 resource_name）
  const referenceNodes = workspaceResourceContext?.resources?.map(r => ({
    id: r.id,
    // instance_name 用于生成引用，使用完整的 tf_module_key
    instance_name: r.tf_module_key || r.resource_name,
    // display_name 用于显示，使用友好的 resource_name
    display_name: r.resource_name,
    module_id: r.module_id,
    module_source: r.module_source,
    outputs: r.outputs,
  })) || [];
  
  const currentNodeId = workspaceResourceContext?.currentResourceId || '';

  // 搜索 CMDB
  const handleSearch = useCallback(async (query: string) => {
    setSearchValue(query);
    
    if (!query || query.length < 2) {
      setSearchResults([]);
      return;
    }

    setLoading(true);

    try {
      const response = await cmdbService.searchResources(query, {
        resource_type: resourceType || undefined,
        limit: 20,
      });
      setSearchResults(response.results || []);
    } catch (err: any) {
      console.error('CMDB search error:', err);
      setSearchResults([]);
    } finally {
      setLoading(false);
    }
  }, [resourceType]);

  // 防抖搜索
  useEffect(() => {
    if (!searchValue) {
      setSearchResults([]);
      return;
    }

    const timer = setTimeout(() => {
      handleSearch(searchValue);
    }, 300);

    return () => clearTimeout(timer);
  }, [searchValue]);

  // 单值模式：处理选择
  const handleSelect = useCallback((value: string) => {
    form.setFieldValue(name, value);
    setSearchResults([]); // 清空搜索结果，关闭下拉
  }, [form, name]);

  // 单值模式：处理输入变化
  const handleChange = useCallback((value: string) => {
    form.setFieldValue(name, value);
  }, [form, name]);

  // 单值模式：处理搜索（用于检测 / 和触发 CMDB 搜索）
  const handleSingleSearch = useCallback((value: string) => {
    console.log('[CMDBSelectWidget] handleSingleSearch:', value, 'hasReferenceContext:', hasReferenceContext);
    
    // 检测 "/" 触发引用选择器
    if (hasReferenceContext && value.endsWith('/')) {
      console.log('[CMDBSelectWidget] Detected /, opening reference popover');
      // 获取输入框位置 - 使用 DOM 查询而不是 ref
      const inputElement = document.querySelector(`[data-cmdb-autocomplete="${name}"]`);
      if (inputElement) {
        const rect = inputElement.getBoundingClientRect();
        setPopoverPosition({
          x: rect.left,
          y: rect.bottom + 4,
        });
        console.log('[CMDBSelectWidget] Popover position:', { x: rect.left, y: rect.bottom + 4 });
      } else {
        console.log('[CMDBSelectWidget] Input element not found, using default position');
        // 使用默认位置
        setPopoverPosition({ x: 100, y: 200 });
      }
      setReferencePopoverOpen(true);
      // 移除末尾的 "/"
      return;
    }
    
    setSearchValue(value);
  }, [hasReferenceContext, name]);

  // 多值模式：处理选择变化
  const handleMultipleChange = useCallback((values: string[]) => {
    form.setFieldValue(name, values);
  }, [form, name]);

  // 多值模式：处理搜索
  const handleMultipleSearch = useCallback((value: string) => {
    console.log('[CMDBSelectWidget] handleMultipleSearch:', value, 'hasReferenceContext:', hasReferenceContext);
    
    // 检测 "/" 触发引用选择器
    if (hasReferenceContext && value.endsWith('/')) {
      console.log('[CMDBSelectWidget] Detected / in multiple mode, opening reference popover');
      // 获取输入框位置
      const selectElement = document.querySelector(`[data-cmdb-select="${name}"]`);
      if (selectElement) {
        const rect = selectElement.getBoundingClientRect();
        setPopoverPosition({
          x: rect.left,
          y: rect.bottom + 4,
        });
      }
      
      // 关键修复：关闭 Select 下拉菜单，防止两个弹窗重叠
      setSelectDropdownOpen(false);
      setReferencePopoverOpen(true);
      
      // 关键修复：清空搜索值，防止 "/" 被添加到数组中
      // 通过设置空的搜索值来阻止 Select 组件将 "/" 添加到选中值
      setSearchValue('');
      return;
    }
    
    setSearchValue(value);
  }, [hasReferenceContext, name]);

  // 处理引用选择
  const handleReferenceSelect = useCallback((reference: string, _sourceNodeId: string, _outputName: string) => {
    // 将引用包装成 Terraform 插值语法 ${...}
    const terraformReference = `\${${reference}}`;
    
    console.log('[CMDBSelectWidget] handleReferenceSelect:', {
      name,
      reference,
      terraformReference,
      isArrayType,
      currentValue: formValue,
    });
    
    if (isArrayType) {
      // 多值模式：追加到数组
      const currentValues = Array.isArray(formValue) ? formValue : [];
      const newValues = [...currentValues, terraformReference];
      form.setFieldsValue({ [name]: newValues });
      
      // 手动触发值更新
      setTimeout(() => {
        if (context?.triggerValuesUpdate) {
          context.triggerValuesUpdate();
        }
      }, 10);
    } else {
      // 单值模式：直接设置
      form.setFieldsValue({ [name]: terraformReference });
      
      // 手动触发值更新
      setTimeout(() => {
        if (context?.triggerValuesUpdate) {
          context.triggerValuesUpdate();
        }
      }, 10);
    }
    
    setReferencePopoverOpen(false);
  }, [form, name, isArrayType, formValue, context]);

  // 构建 AutoComplete 选项
  const options = useMemo(() => {
    if (loading) {
      return [{
        value: '__loading__',
        label: (
          <div style={{ textAlign: 'center', padding: '8px 0' }}>
            <Spin size="small" />
            <span style={{ marginLeft: 8, color: '#8c8c8c' }}>搜索中...</span>
          </div>
        ),
        disabled: true,
      }];
    }

    if (searchValue.length >= 2 && searchResults.length === 0) {
      return [{
        value: '__empty__',
        label: (
          <div style={{ textAlign: 'center', padding: '8px 0', color: '#8c8c8c' }}>
            未找到匹配的 CMDB 资源，可直接使用输入的值
          </div>
        ),
        disabled: true,
      }];
    }

    return searchResults.map((result) => {
      const cmdbValue = extractFieldValue(result, valueField);
      const displayName = result.cloud_resource_name || result.terraform_address || cmdbValue;
      
      return {
        // 直接使用 cmdbValue 作为 value，选择时会填充这个值
        value: cmdbValue,
        label: (
          <div style={{ padding: '4px 0' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ fontWeight: 500 }}>{displayName}</span>
              <Tag color="cyan" style={{ fontSize: 10 }}>{result.resource_type}</Tag>
            </div>
            <div style={{ fontSize: 12, color: '#8c8c8c', marginTop: 2 }}>
              <span>{valueFieldLabel}: </span>
              <code style={{ 
                background: '#f5f5f5', 
                padding: '1px 4px', 
                borderRadius: 2,
                fontFamily: 'Monaco, Menlo, monospace',
                fontSize: 11,
              }}>
                {cmdbValue}
              </code>
            </div>
            {result.workspace_name && (
              <div style={{ fontSize: 11, color: '#bfbfbf', marginTop: 2 }}>
                Workspace: {result.workspace_name}
              </div>
            )}
          </div>
        ),
      };
    });
  }, [searchResults, valueField, valueFieldLabel, loading, searchValue]);

  // 多值模式的选项
  const multipleOptions = useMemo(() => {
    return searchResults.map((result) => {
      const cmdbValue = extractFieldValue(result, valueField);
      const displayName = result.cloud_resource_name || result.terraform_address || cmdbValue;
      
      return {
        value: cmdbValue,
        label: (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span>{displayName}</span>
            <Tag color="cyan" style={{ fontSize: 10 }}>{result.resource_type}</Tag>
            <span style={{ fontSize: 11, color: '#8c8c8c' }}>({cmdbValue})</span>
          </div>
        ),
      };
    });
  }, [searchResults, valueField]);

  // 渲染帮助文本
  const helpElement = useMemo(() => {
    const baseHelp = help || '';
    if (hasReferenceContext) {
      return (
        <span>
          {baseHelp}
          {baseHelp && ' '}
          <span style={{ color: '#1890ff', fontSize: 11 }}>
            输入 / 可引用其他资源的输出
          </span>
        </span>
      );
    }
    return baseHelp || undefined;
  }, [help, hasReferenceContext]);

  // 渲染标签
  const labelElement = (
    <span>
      {label}
      <Tooltip title={`输入时自动搜索 CMDB ${resourceType || '所有'} 资源，选择后填充 ${valueFieldLabel}。也可直接输入任意值。`}>
        <Tag color="cyan" icon={<DatabaseOutlined />} style={{ marginLeft: 8, cursor: 'help' }}>
          CMDB
        </Tag>
      </Tooltip>
    </span>
  );

  // 多值模式（array 类型）
  if (isArrayType) {
    return (
      <>
        <Form.Item
          label={labelElement}
          name={name}
          help={helpElement}
          rules={[
            ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
          ]}
        >
          <Select
            ref={inputRef}
            mode="tags"
            placeholder={placeholder}
            disabled={disabled || readOnly}
            value={formValue || []}
            onChange={handleMultipleChange}
            onSearch={handleMultipleSearch}
            onDropdownVisibleChange={(visible) => {
              // 只有在引用选择器关闭时才允许打开 Select 下拉菜单
              if (!referencePopoverOpen) {
                setSelectDropdownOpen(visible);
              }
            }}
            open={selectDropdownOpen && !referencePopoverOpen}
            searchValue={searchValue}
            style={{ width: '100%' }}
            tokenSeparators={[',']}
            loading={loading}
            filterOption={false}
            data-cmdb-select={name}
            notFoundContent={
              loading ? (
                <div style={{ textAlign: 'center', padding: '8px 0' }}>
                  <Spin size="small" />
                  <span style={{ marginLeft: 8, color: '#8c8c8c' }}>搜索中...</span>
                </div>
              ) : searchValue.length >= 2 && searchResults.length === 0 ? (
                <div style={{ textAlign: 'center', padding: '8px 0', color: '#8c8c8c' }}>
                  未找到匹配的 CMDB 资源，可直接输入值后按回车添加
                </div>
              ) : searchValue.length < 2 ? (
                <div style={{ textAlign: 'center', padding: '8px 0', color: '#8c8c8c' }}>
                  输入至少 2 个字符搜索 CMDB 资源
                </div>
              ) : null
            }
            options={multipleOptions}
          />
        </Form.Item>
        
        {/* 引用选择器弹出层 */}
        {hasReferenceContext && (
          <ModuleReferencePopover
            open={referencePopoverOpen}
            onClose={() => setReferencePopoverOpen(false)}
            onSelect={handleReferenceSelect}
            currentNodeId={currentNodeId}
            nodes={referenceNodes}
            position={popoverPosition}
            remoteData={workspaceResourceContext?.remoteData}
          />
        )}
      </>
    );
  }

  // 单值模式（string 类型）
  return (
    <>
      <Form.Item
        label={labelElement}
        name={name}
        help={helpElement}
        rules={[
          ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
        ]}
      >
        <AutoComplete
          ref={inputRef}
          placeholder={placeholder}
          disabled={disabled || readOnly}
          value={formValue || ''}
          options={options}
          onSearch={handleSingleSearch}
          onSelect={handleSelect}
          onChange={handleChange}
          style={{ width: '100%' }}
          notFoundContent={null}
          allowClear
          data-cmdb-autocomplete={name}
        />
      </Form.Item>
      
      {/* 引用选择器弹出层 */}
      {hasReferenceContext && (
        <ModuleReferencePopover
          open={referencePopoverOpen}
          onClose={() => setReferencePopoverOpen(false)}
          onSelect={handleReferenceSelect}
          currentNodeId={currentNodeId}
          nodes={referenceNodes}
          position={popoverPosition}
          remoteData={workspaceResourceContext?.remoteData}
        />
      )}
    </>
  );
};

// 添加 hover 样式
const style = document.createElement('style');
style.textContent = `
  .cmdb-search-item:hover {
    background-color: #f5f5f5;
  }
`;
if (!document.querySelector('style[data-cmdb-widget]')) {
  style.setAttribute('data-cmdb-widget', 'true');
  document.head.appendChild(style);
}

export default CMDBSelectWidget;
