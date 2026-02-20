import React, { useCallback, useMemo, useState, useRef } from 'react';
import { Form, Input, Button, Space, Tag, Tooltip } from 'antd';
import { PlusOutlined, DeleteOutlined, LinkOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { ModuleReferencePopover } from '../../ManifestEditor/ModuleReferencePopover';

interface KeyValuePair {
  key: string;
  value: string;
}

// 内部编辑器组件
interface KeyValueEditorProps {
  value?: Record<string, string>;
  onChange?: (value: Record<string, string>) => void;
  disabled?: boolean;
  readOnly?: boolean;
  context?: any;
  name?: string;
}

const KeyValueEditor: React.FC<KeyValueEditorProps> = ({
  value,
  onChange,
  disabled,
  readOnly,
  context,
  name,
}) => {
  // 引用选择器状态
  const [referencePopoverOpen, setReferencePopoverOpen] = useState(false);
  const [popoverPosition, setPopoverPosition] = useState<{ x: number; y: number } | undefined>();
  const [activeIndex, setActiveIndex] = useState<number | null>(null);
  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;
  const hasOtherNodes = (manifestContext?.nodes?.length ?? 0) > 0;

  // 将对象转换为键值对数组
  const pairs = useMemo((): KeyValuePair[] => {
    if (!value || typeof value !== 'object') return [];
    return Object.entries(value).map(([key, val]) => ({ key, value: String(val) }));
  }, [value]);

  // 将键值对数组转换为对象
  // 只保存有 key 的项，空 key 的项不保存到表单值中
  const arrayToObject = useCallback((arr: KeyValuePair[]): Record<string, string> => {
    const result: Record<string, string> = {};
    arr.forEach((item) => {
      // 只有当 key 不为空时才添加到结果中
      if (item.key && item.key.trim()) {
        result[item.key.trim()] = item.value;
      }
    });
    return result;
  }, []);

  // 使用本地状态管理编辑中的键值对（包括空 key 的项）
  const [localPairs, setLocalPairs] = useState<KeyValuePair[]>(() => {
    if (!value || typeof value !== 'object') return [];
    return Object.entries(value).map(([key, val]) => ({ key, value: String(val) }));
  });

  // 当外部 value 变化时，同步本地状态
  React.useEffect(() => {
    if (!value || typeof value !== 'object') {
      setLocalPairs([]);
    } else {
      const newPairs = Object.entries(value).map(([key, val]) => ({ key, value: String(val) }));
      // 修复：比较完整的键值对（key=value），而不仅仅是 keys
      // 这样当 keys 相同但 values 不同时（如 AI 填充数据），也会正确更新
      // 同时保留原有逻辑：只比较有效的 key（过滤掉用户正在编辑的空 key 项）
      const currentPairsStr = localPairs
        .filter(p => p.key.trim())
        .map(p => `${p.key}=${p.value}`)
        .sort()
        .join('|');
      const newPairsStr = newPairs
        .map(p => `${p.key}=${p.value}`)
        .sort()
        .join('|');
      if (currentPairsStr !== newPairsStr) {
        setLocalPairs(newPairs);
      }
    }
  }, [value]);

  const handleAdd = useCallback(() => {
    // 添加新的空键值对到本地状态
    setLocalPairs(prev => [...prev, { key: '', value: '' }]);
  }, []);

  const handleRemove = useCallback((index: number) => {
    const newPairs = localPairs.filter((_, i) => i !== index);
    setLocalPairs(newPairs);
    onChange?.(arrayToObject(newPairs));
  }, [localPairs, onChange, arrayToObject]);

  const handleChange = useCallback((index: number, field: 'key' | 'value', val: string) => {
    // 检测 "/" 触发引用选择器（只在值字段）
    if (field === 'value' && hasManifestContext && hasOtherNodes && val.endsWith('/')) {
      const inputElement = inputRefs.current[index];
      if (inputElement) {
        const rect = inputElement.getBoundingClientRect();
        setPopoverPosition({
          x: rect.left,
          y: rect.bottom + 4,
        });
      }
      setActiveIndex(index);
      setReferencePopoverOpen(true);
      // 移除末尾的 "/"
      const newPairs = [...localPairs];
      newPairs[index] = { ...newPairs[index], [field]: val.slice(0, -1) };
      setLocalPairs(newPairs);
      onChange?.(arrayToObject(newPairs));
      return;
    }
    
    const newPairs = [...localPairs];
    newPairs[index] = { ...newPairs[index], [field]: val };
    setLocalPairs(newPairs);
    onChange?.(arrayToObject(newPairs));
  }, [localPairs, onChange, arrayToObject, hasManifestContext, hasOtherNodes]);

  // 处理引用选择
  const handleReferenceSelect = useCallback((reference: string, sourceNodeId: string, outputName: string) => {
    if (activeIndex !== null) {
      const newPairs = [...localPairs];
      newPairs[activeIndex] = { ...newPairs[activeIndex], value: reference };
      setLocalPairs(newPairs);
      onChange?.(arrayToObject(newPairs));
      
      // 通知父组件创建连线
      if (manifestContext?.onAddEdge) {
        manifestContext.onAddEdge(
          sourceNodeId,
          manifestContext.currentNodeId,
          outputName,
          name || 'tags'
        );
      }
    }
    
    setReferencePopoverOpen(false);
    setActiveIndex(null);
  }, [activeIndex, localPairs, onChange, arrayToObject, manifestContext, name]);

  // 检查值是否是引用（支持 module.xxx.yyy 和 ${module.xxx.yyy} 格式）
  const isReference = (val: string) => {
    if (!val) return false;
    // 匹配 module.xxx.yyy 或 ${module.xxx.yyy}
    return val.startsWith('module.') || 
           val.startsWith('${module.') ||
           val.startsWith('var.') ||
           val.startsWith('${var.') ||
           val.startsWith('local.') ||
           val.startsWith('${local.');
  };

  return (
    <>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {localPairs.map((pair, index) => (
          <Space key={index} style={{ display: 'flex' }} align="baseline">
            <Input
              placeholder="键"
              value={pair.key}
              onChange={(e) => handleChange(index, 'key', e.target.value)}
              disabled={disabled || readOnly}
              style={{ width: 150 }}
            />
            <Input
              ref={(el) => { inputRefs.current[index] = el?.input || null; }}
              placeholder="值"
              value={pair.value}
              onChange={(e) => handleChange(index, 'value', e.target.value)}
              disabled={disabled || readOnly}
              style={{ 
                width: 200,
                ...(isReference(pair.value) ? {
                  fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
                  color: '#1890ff',
                } : {}),
              }}
              suffix={isReference(pair.value) ? (
                <Tooltip title="模块引用">
                  <LinkOutlined style={{ color: '#1890ff' }} />
                </Tooltip>
              ) : undefined}
            />
            {!readOnly && !disabled && (
              <Button
                type="text"
                danger
                icon={<DeleteOutlined />}
                onClick={() => handleRemove(index)}
              />
            )}
          </Space>
        ))}
        {!readOnly && !disabled && (
          <Button
            type="dashed"
            onClick={handleAdd}
            icon={<PlusOutlined />}
            style={{ width: 'fit-content' }}
          >
            添加
          </Button>
        )}
        {hasManifestContext && hasOtherNodes && (
          <div style={{ color: '#1890ff', fontSize: 11 }}>
            在值字段输入 / 可引用其他 Module 的输出
          </div>
        )}
      </div>

      {/* 引用选择器弹出层 */}
      {hasManifestContext && (
        <ModuleReferencePopover
          open={referencePopoverOpen}
          onClose={() => {
            setReferencePopoverOpen(false);
            setActiveIndex(null);
          }}
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

/**
 * KeyValueWidget - 键值对输入组件
 * 
 * 注意：使用 Form.useWatch 获取表单值，让 Form.Item 自动管理值
 * KeyValueEditor 作为 Form.Item 的子组件，会自动接收 value 和 onChange
 */
const KeyValueWidget: React.FC<WidgetProps> = ({
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

  // 检查是否包含引用（支持 module.xxx.yyy 和 ${module.xxx.yyy} 格式）
  const hasReference = useMemo(() => {
    if (!formValue || typeof formValue !== 'object') return false;
    return Object.values(formValue).some((v: any) => {
      if (typeof v !== 'string') return false;
      return v.startsWith('module.') || 
             v.startsWith('${module.') ||
             v.startsWith('var.') ||
             v.startsWith('${var.') ||
             v.startsWith('local.') ||
             v.startsWith('${local.');
    });
  }, [formValue]);

  return (
    <Form.Item
      label={
        <span>
          {label}
          {hasReference && (
            <Tooltip title="包含模块引用">
              <Tag 
                color="blue" 
                icon={<LinkOutlined />}
                style={{ marginLeft: 8, cursor: 'pointer' }}
              >
                引用
              </Tag>
            </Tooltip>
          )}
        </span>
      }
      name={name}
      help={help}
      rules={[
        ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
      ]}
    >
      <KeyValueEditor 
        disabled={disabled} 
        readOnly={readOnly} 
        context={context}
        name={name}
      />
    </Form.Item>
  );
};

export default KeyValueWidget;
