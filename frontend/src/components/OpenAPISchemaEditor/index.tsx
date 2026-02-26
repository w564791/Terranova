import React, { useState, useMemo, useCallback, useRef, useEffect } from 'react';
import {
  DndContext,
  closestCenter,
  pointerWithin,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  useDroppable,
  useDraggable,
  DragOverlay,
} from '@dnd-kit/core';
import type { DragEndEvent, DragStartEvent, CollisionDetection } from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
  rectSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { MonacoJsonEditor } from '../DynamicForm/MonacoJsonEditor';
import ConfirmDialog from '../ConfirmDialog';
import type { OpenAPISchema } from '../../services/schemaV2';
import api from '../../services/api';
import styles from './OpenAPISchemaEditor.module.css';

// ============ 自定义碰撞检测：分隔区优先 ============
// 当拖拽时，优先检测分隔区（separator），再检测行（row）
// 解决 pointerWithin 对薄分隔区不可靠的问题
const createSeparatorFirstCollision = (separatorPrefix: string, rowPrefix: string): CollisionDetection => {
  return (args) => {
    const separators = args.droppableContainers.filter(
      c => String(c.id).startsWith(separatorPrefix)
    );
    const rows = args.droppableContainers.filter(
      c => String(c.id).startsWith(rowPrefix)
    );

    // First check separators with pointerWithin
    const sepCollisions = pointerWithin({ ...args, droppableContainers: separators });
    if (sepCollisions.length > 0) return sepCollisions;

    // Then check rows
    const rowCollisions = pointerWithin({ ...args, droppableContainers: rows });
    return rowCollisions;
  };
};

// ============ 类型定义 ============

// 分组定义
interface UIGroup {
  id: string;
  label: string;
  level: 'basic' | 'advanced';
  layout: 'tabs' | 'accordion' | 'sections';
  order: number;
  description?: string;
}

// 默认分组
const DEFAULT_GROUPS: UIGroup[] = [
  { id: 'basic', label: '基础配置', level: 'basic', layout: 'sections', order: 1 },
  { id: 'advanced', label: '高级配置', level: 'advanced', layout: 'accordion', order: 100 },
];

// ============ CMDB 字段定义 ============
// CMDB 预定义字段（固定 Key）
interface CMDBFieldDefinition {
  key: string;         // 字段 Key（如 cloud_id）
  label: string;       // 显示名称（如 "资源 ID"）
  description: string; // 字段说明
  examples: string[];  // 示例值列表
}

const CMDB_FIELD_DEFINITIONS: CMDBFieldDefinition[] = [
  {
    key: 'cloud_id',
    label: '资源 ID',
    description: '云资源唯一标识符',
    examples: ['sg-0123456789abcdef0', 'subnet-0123456789abcdef0', 'vpc-0123456789abcdef0'],
  },
  {
    key: 'cloud_arn',
    label: 'ARN',
    description: 'AWS ARN / Azure Resource ID',
    examples: ['arn:aws:iam::123456789012:role/my-role', 'arn:aws:s3:::my-bucket'],
  },
  {
    key: 'cloud_name',
    label: '资源名称',
    description: '云资源的名称',
    examples: ['my-instance', 'production-db', 'web-server'],
  },
  {
    key: 'cloud_region',
    label: '区域',
    description: '云资源所在区域',
    examples: ['us-east-1', 'ap-southeast-1', 'eu-west-1'],
  },
  {
    key: 'cloud_account',
    label: '账户 ID',
    description: '云账户标识符',
    examples: ['123456789012', '987654321098'],
  },
  {
    key: 'terraform_address',
    label: 'Terraform 地址',
    description: '完整的 Terraform 资源地址',
    examples: ['module.vpc.aws_vpc.this[0]', 'aws_instance.web'],
  },
  {
    key: 'description',
    label: '描述',
    description: '资源描述信息',
    examples: ['Production database server', 'Web application load balancer'],
  },
];

// 资源类型推荐的 valueField 映射
const RESOURCE_TYPE_RECOMMENDED_FIELDS: Record<string, string> = {
  'aws_security_group': 'cloud_id',
  'aws_iam_role': 'cloud_arn',
  'aws_iam_policy': 'cloud_arn',
  'aws_iam_instance_profile': 'cloud_arn',
  'aws_subnet': 'cloud_id',
  'aws_vpc': 'cloud_id',
  'aws_s3_bucket': 'cloud_id',
  'aws_kms_key': 'cloud_arn',
  'aws_lb': 'cloud_arn',
  'aws_lb_target_group': 'cloud_arn',
  'aws_ami': 'cloud_id',
  'aws_key_pair': 'cloud_name',
  'aws_acm_certificate': 'cloud_arn',
  'aws_eks_cluster': 'cloud_name',
  'aws_rds_cluster': 'cloud_id',
  'aws_db_instance': 'cloud_id',
};

// 键值对编辑器组件（用于 object 类型）
interface KeyValuePair {
  key: string;
  value: string;
}

const KeyValueEditor: React.FC<{
  value: Record<string, string>;
  onChange: (value: Record<string, string> | undefined) => void;
}> = ({ value, onChange }) => {
  const [pairs, setPairs] = useState<KeyValuePair[]>(() => {
    if (!value || typeof value !== 'object') return [{ key: '', value: '' }];
    const entries = Object.entries(value);
    return entries.length > 0 ? entries.map(([k, v]) => ({ key: k, value: String(v) })) : [{ key: '', value: '' }];
  });

  React.useEffect(() => {
    if (!value || typeof value !== 'object') {
      setPairs([{ key: '', value: '' }]);
    } else {
      const entries = Object.entries(value);
      if (entries.length > 0) {
        setPairs(entries.map(([k, v]) => ({ key: k, value: String(v) })));
      }
    }
  }, [value]);

  const updatePairs = (newPairs: KeyValuePair[]) => {
    setPairs(newPairs);
    const obj: Record<string, string> = {};
    let hasValidPair = false;
    newPairs.forEach(pair => {
      if (pair.key.trim()) {
        obj[pair.key.trim()] = pair.value;
        hasValidPair = true;
      }
    });
    onChange(hasValidPair ? obj : undefined);
  };

  const handleKeyChange = (index: number, newKey: string) => {
    const newPairs = [...pairs];
    newPairs[index].key = newKey;
    updatePairs(newPairs);
  };

  const handleValueChange = (index: number, newValue: string) => {
    const newPairs = [...pairs];
    newPairs[index].value = newValue;
    updatePairs(newPairs);
  };

  const addPair = () => {
    setPairs([...pairs, { key: '', value: '' }]);
  };

  const removePair = (index: number) => {
    if (pairs.length === 1) {
      updatePairs([{ key: '', value: '' }]);
    } else {
      const newPairs = pairs.filter((_, i) => i !== index);
      updatePairs(newPairs);
    }
  };

  return (
    <div className={styles.keyValueEditor}>
      <div className={styles.kvHeader}>
        <span className={styles.kvHeaderKey}>键 (Key)</span>
        <span className={styles.kvHeaderValue}>值 (Value)</span>
        <span className={styles.kvHeaderAction}></span>
      </div>
      {pairs.map((pair, index) => (
        <div key={index} className={styles.kvRow}>
          <input
            type="text"
            value={pair.key}
            onChange={(e) => handleKeyChange(index, e.target.value)}
            className={styles.kvKeyInput}
            placeholder="输入键名"
          />
          <input
            type="text"
            value={pair.value}
            onChange={(e) => handleValueChange(index, e.target.value)}
            className={styles.kvValueInput}
            placeholder="输入值"
          />
          <button
            type="button"
            onClick={() => removePair(index)}
            className={styles.kvRemoveButton}
            title="删除此行"
          >
            ✕
          </button>
        </div>
      ))}
      <button type="button" onClick={addPair} className={styles.kvAddButton}>
        + 添加键值对
      </button>
    </div>
  );
};

// ValueType 定义和说明
const VALUE_TYPE_OPTIONS = [
  { value: 'boolean', label: 'TypeBool (1) - 布尔值', description: '只有是或否，渲染为开关类型', example: 'enable_versioning, force_destroy' },
  { value: 'integer', label: 'TypeInt (2) - 整数', description: '数值类型，可点击上下箭头增减', example: 'volume_size, port, count' },
  { value: 'number', label: 'TypeFloat (3) - 浮点数', description: '浮点数类型，用户输入', example: 'cpu_credits, weight' },
  { value: 'string', label: 'TypeString (4) - 字符串', description: '默认为用户输入，可配置外部数据源变为下拉框', example: 'name, key_name, ami' },
  { value: 'array', label: 'TypeList (5) - 列表', description: '默认多行输入，可配置外部数据源变为多选下拉框', example: 'security_group_ids, subnet_ids' },
  { value: 'map', label: 'TypeMap (6) - 键值对', description: '用户自由增加KV键值对，典型场景为资源标签', example: 'tags, environment_variables' },
  { value: 'set', label: 'TypeSet (7) - 集合', description: '与List类似但元素唯一', example: 'policy_arns, ip_addresses' },
  { value: 'object', label: 'TypeObject (8) - 对象', description: 'Key由平台定义不可修改，Value可为任意类型', example: 'root_block_device' },
  { value: 'json', label: 'TypeJsonString (9) - JSON字符串', description: '渲染为JSON IDE编辑器', example: 'policy_document' },
  { value: 'text', label: 'TypeText (10) - 多行文本', description: '纯文本输入，典型场景为EC2的user data', example: 'user_data' },
  { value: 'object-list', label: 'TypeListObject (11) - 对象列表', description: '多个TypeObject的组合场景', example: 'ebs_block_device' },
  { value: 'dynamic-object', label: 'CustomObject (12) - 动态键对象', description: 'Key为随机数，Value为固定TypeObject', example: 'bucket_policies' },
];

// 根据 ValueType 获取 OpenAPI Schema 配置
const getSchemaConfigForValueType = (valueType: string): any => {
  switch (valueType) {
    case 'boolean': return { type: 'boolean' };
    case 'integer': return { type: 'integer', format: 'int64' };
    case 'number': return { type: 'number', format: 'double' };
    case 'string': return { type: 'string' };
    case 'array': return { type: 'array', items: { type: 'string' } };
    case 'map': return { type: 'object', additionalProperties: { type: 'string' } };
    case 'set': return { type: 'array', items: { type: 'string' }, uniqueItems: true };
    case 'object': return { type: 'object', properties: {} };
    case 'json': return { type: 'string', format: 'json', 'x-widget': 'json-editor' };
    case 'text': return { type: 'string', 'x-widget': 'textarea' };
    case 'object-list': return { type: 'array', items: { type: 'object', properties: {} } };
    case 'dynamic-object': return { type: 'object', 'x-dynamic-keys': true, additionalProperties: { type: 'object', properties: {} } };
    default: return { type: 'string' };
  }
};

// 根据 ValueType 获取默认的 Widget 类型
// 这个映射确保值类型和 Widget 类型联动
const getDefaultWidgetForValueType = (valueType: string): string => {
  switch (valueType) {
    case 'boolean': return 'switch';
    case 'integer': return 'number';
    case 'number': return 'number';
    case 'string': return 'text';
    case 'array': return 'tags';        // 数组默认使用标签输入
    case 'map': return 'key-value';     // map 默认使用键值对
    case 'set': return 'tags';          // 集合默认使用标签输入
    case 'object': return 'object';     // 对象使用对象编辑器
    case 'json': return 'json-editor';  // JSON 使用 JSON 编辑器
    case 'text': return 'textarea';     // 多行文本使用 textarea
    case 'object-list': return 'object-list';  // 对象列表
    case 'dynamic-object': return 'dynamic-object';  // 动态键对象
    default: return 'text';
  }
};

// 从 OpenAPI Schema 推断 ValueType
const inferValueTypeFromSchema = (property: any): string => {
  if (!property) return 'string';
  
  // 检查 x-dynamic-keys 标记
  if (property['x-dynamic-keys']) return 'dynamic-object';
  
  // 检查 x-widget 扩展属性
  if (property['x-widget'] === 'dynamic-object') return 'dynamic-object';
  if (property['x-widget'] === 'json-editor' || property.format === 'json') return 'json';
  if (property['x-widget'] === 'textarea') return 'text';
  
  if (property.type === 'boolean') return 'boolean';
  if (property.type === 'integer') return 'integer';
  if (property.type === 'number') return 'number';
  if (property.type === 'array') {
    if (property.uniqueItems) return 'set';
    if (property.items?.type === 'object') return 'object-list';
    return 'array';
  }
  if (property.type === 'object') {
    // 检查是否是动态键对象：additionalProperties 是对象类型
    if (property.additionalProperties && 
        typeof property.additionalProperties === 'object' &&
        property.additionalProperties.type === 'object') {
      return 'dynamic-object';
    }
    // 普通的 map 类型：additionalProperties 是简单类型
    if (property.additionalProperties && !property.properties) return 'map';
    return 'object';
  }
  return 'string';
};

// ============ 扩展的默认值输入组件 ============
interface ExtendedDefaultValueInputProps {
  valueType: string;
  property: any;
  value: unknown;
  onChange: (value: unknown) => void;
  widget?: string;
}

const ExtendedDefaultValueInput: React.FC<ExtendedDefaultValueInputProps> = ({ valueType, property, value, onChange, widget }) => {
  const useJsonForCollection = widget === 'json-editor';

  const [inputValue, setInputValue] = useState<string>(() => {
    if (value === undefined || value === null) return '';
    if (useJsonForCollection && (Array.isArray(value) || typeof value === 'object')) return JSON.stringify(value, null, 2);
    if (Array.isArray(value)) return value.join('\n');
    if (typeof value === 'object') return JSON.stringify(value, null, 2);
    return String(value);
  });

  React.useEffect(() => {
    if (value === undefined || value === null) setInputValue('');
    else if (useJsonForCollection && (Array.isArray(value) || typeof value === 'object')) setInputValue(JSON.stringify(value, null, 2));
    else if (Array.isArray(value)) setInputValue(value.join('\n'));
    else if (typeof value === 'object') setInputValue(JSON.stringify(value, null, 2));
    else setInputValue(String(value));
  }, [value, widget, useJsonForCollection]);

  const typeInfo = VALUE_TYPE_OPTIONS.find(opt => opt.value === valueType);

  if (valueType === 'boolean') {
    return (
      <div className={styles.defaultValueContainer}>
        <div className={styles.switchContainer}>
          <label className={styles.switchLabel}>
            <input type="checkbox" checked={value === true} onChange={(e) => onChange(e.target.checked)} className={styles.switchInput} />
            <span className={styles.switchSlider}></span>
            <span className={styles.switchText}>{value === true ? '开启 (true)' : '关闭 (false)'}</span>
          </label>
          <button type="button" className={styles.clearButton} onClick={() => onChange(undefined)}>清除</button>
        </div>
      </div>
    );
  }

  if (valueType === 'integer') {
    const numValue = typeof value === 'number' ? value : (value ? parseInt(String(value), 10) : 0);
    return (
      <div className={styles.defaultValueContainer}>
        <div className={styles.numberInputContainer}>
          <button type="button" className={styles.numberButton} onClick={() => onChange((numValue || 0) - 1)}>-</button>
          <input type="number" value={value !== undefined ? numValue : ''} onChange={(e) => onChange(e.target.value === '' ? undefined : parseInt(e.target.value, 10))} className={styles.numberInput} placeholder="输入整数" step={1} />
          <button type="button" className={styles.numberButton} onClick={() => onChange((numValue || 0) + 1)}>+</button>
        </div>
      </div>
    );
  }

  if (valueType === 'number') {
    return (
      <div className={styles.defaultValueContainer}>
        <input type="number" value={inputValue} onChange={(e) => { setInputValue(e.target.value); onChange(e.target.value === '' ? undefined : parseFloat(e.target.value)); }} className={styles.fieldInput} placeholder="例如：3.14" step="any" />
      </div>
    );
  }

  if (valueType === 'array' || valueType === 'set') {
    if (useJsonForCollection) {
      return (
        <div className={styles.defaultValueContainer}>
          <textarea value={inputValue} onChange={(e) => {
            setInputValue(e.target.value);
            try {
              const parsed = JSON.parse(e.target.value);
              if (Array.isArray(parsed)) onChange(parsed.length > 0 ? parsed : undefined);
            } catch { /* 用户编辑中 */ }
          }} className={styles.fieldTextarea} rows={4} placeholder='例如：["item1", "item2"]' style={{ fontFamily: 'Monaco, Menlo, "Ubuntu Mono", Consolas, monospace', fontSize: 13 }} />
        </div>
      );
    }
    return (
      <div className={styles.defaultValueContainer}>
        <textarea value={inputValue} onChange={(e) => { setInputValue(e.target.value); const lines = valueType === 'set' ? [...new Set(e.target.value.split('\n').filter(l => l.trim()))] : e.target.value.split('\n').filter(l => l.trim()); onChange(lines.length > 0 ? lines : undefined); }} className={styles.fieldTextarea} rows={3} placeholder={valueType === 'set' ? '每行一个值（自动去重）' : '每行一个值'} />
      </div>
    );
  }

  if (valueType === 'map') {
    if (useJsonForCollection) {
      return (
        <div className={styles.defaultValueContainer}>
          <textarea value={inputValue} onChange={(e) => {
            setInputValue(e.target.value);
            try {
              const parsed = JSON.parse(e.target.value);
              if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) onChange(Object.keys(parsed).length > 0 ? parsed : undefined);
            } catch { /* 用户编辑中 */ }
          }} className={styles.fieldTextarea} rows={4} placeholder='例如：{"key": "value"}' style={{ fontFamily: 'Monaco, Menlo, "Ubuntu Mono", Consolas, monospace', fontSize: 13 }} />
        </div>
      );
    }
    return (
      <div className={styles.defaultValueContainer}>
        <KeyValueEditor value={value as Record<string, string> || {}} onChange={onChange} />
      </div>
    );
  }

  // dynamic-object 类型使用专门的表单编辑器
  if (valueType === 'dynamic-object') {
    const objectValue = (value && typeof value === 'object' && !Array.isArray(value)) 
      ? value as Record<string, Record<string, unknown>>
      : {};
    const keys = Object.keys(objectValue);
    
    // 获取嵌套属性的 Schema
    const nestedProperties = property.additionalProperties?.properties || {};
    const nestedPropertyNames = Object.keys(nestedProperties);
    
    // 生成随机 key
    const generateKey = () => {
      const chars = 'abcdefghijklmnopqrstuvwxyz';
      const length = Math.floor(Math.random() * 8) + 8;
      let key = chars[Math.floor(Math.random() * chars.length)];
      for (let i = 1; i < length; i++) {
        if (i > 2 && Math.random() < 0.15) key += '-';
        else key += chars[Math.floor(Math.random() * chars.length)];
      }
      if (key.endsWith('-')) key = key.slice(0, -1) + chars[Math.floor(Math.random() * chars.length)];
      return key;
    };
    
    // 添加新项目
    const handleAddItem = () => {
      let newKey = generateKey();
      while (keys.includes(newKey)) newKey = generateKey();
      const newItem: Record<string, unknown> = {};
      nestedPropertyNames.forEach(propName => {
        const propDef = nestedProperties[propName];
        if (propDef.default !== undefined) newItem[propName] = propDef.default;
        else if (propDef.type === 'string') newItem[propName] = '';
        else if (propDef.type === 'number' || propDef.type === 'integer') newItem[propName] = 0;
        else if (propDef.type === 'boolean') newItem[propName] = false;
        else newItem[propName] = '';
      });
      onChange({ ...objectValue, [newKey]: newItem });
    };
    
    // 删除项目
    const handleRemoveItem = (key: string) => {
      const newValue = { ...objectValue };
      delete newValue[key];
      onChange(Object.keys(newValue).length > 0 ? newValue : undefined);
    };
    
    // 更新项目属性
    const handleUpdateItemProperty = (itemKey: string, propName: string, propValue: unknown) => {
      const newValue = { ...objectValue };
      newValue[itemKey] = { ...newValue[itemKey], [propName]: propValue };
      onChange(newValue);
    };
    
    return (
      <div className={styles.defaultValueContainer}>
        <div className={styles.dynamicObjectEditor}>
          {keys.length === 0 ? (
            <div className={styles.emptyState}>暂无默认值项目</div>
          ) : (
            keys.map(itemKey => (
              <div key={itemKey} className={styles.dynamicObjectItem}>
                <div className={styles.dynamicObjectItemHeader}>
                  <span className={styles.dynamicObjectKey}>{itemKey}</span>
                  <button type="button" onClick={() => handleRemoveItem(itemKey)} className={styles.dynamicObjectRemove}>✕</button>
                </div>
                <div className={styles.dynamicObjectItemContent}>
                  {nestedPropertyNames.map(propName => {
                    const propDef = nestedProperties[propName];
                    const propValue = (objectValue[itemKey] as Record<string, unknown>)?.[propName] ?? '';
                    return (
                      <div key={propName} className={styles.dynamicObjectProperty}>
                        <label>{propDef.title || propName}</label>
                        {propDef.type === 'boolean' ? (
                          <input type="checkbox" checked={propValue === true} onChange={(e) => handleUpdateItemProperty(itemKey, propName, e.target.checked)} />
                        ) : propDef.type === 'number' || propDef.type === 'integer' ? (
                          <input type="number" value={propValue as number || ''} onChange={(e) => handleUpdateItemProperty(itemKey, propName, e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
                        ) : propDef.enum ? (
                          <select value={propValue as string || ''} onChange={(e) => handleUpdateItemProperty(itemKey, propName, e.target.value)} className={styles.fieldSelect}>
                            <option value="">请选择</option>
                            {propDef.enum.map((opt: string) => <option key={opt} value={opt}>{opt}</option>)}
                          </select>
                        ) : (
                          <input type="text" value={propValue as string || ''} onChange={(e) => handleUpdateItemProperty(itemKey, propName, e.target.value)} className={styles.fieldInput} placeholder={propDef.description || ''} />
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            ))
          )}
          <button type="button" onClick={handleAddItem} className={styles.dynamicObjectAdd}>+ 添加默认值项目</button>
        </div>
      </div>
    );
  }

  if (valueType === 'object' || valueType === 'json' || valueType === 'object-list') {
    return (
      <div className={styles.defaultValueContainer}>
        <textarea value={inputValue} onChange={(e) => { setInputValue(e.target.value); try { const parsed = JSON.parse(e.target.value); onChange(parsed); } catch {} }} className={styles.fieldTextarea} rows={4} placeholder='输入 JSON' />
      </div>
    );
  }

  if (valueType === 'text') {
    return (
      <div className={styles.defaultValueContainer}>
        <textarea value={inputValue} onChange={(e) => { setInputValue(e.target.value); onChange(e.target.value || undefined); }} className={styles.fieldTextarea} rows={4} placeholder="多行文本" />
      </div>
    );
  }

  return (
    <div className={styles.defaultValueContainer}>
      <input type="text" value={inputValue} onChange={(e) => { setInputValue(e.target.value); onChange(e.target.value || undefined); }} className={styles.fieldInput} placeholder="输入字符串默认值" />
    </div>
  );
};

// ============ 内联字段编辑器组件 ============
interface InlineFieldEditorProps {
  fieldName: string;
  property: any;
  uiConfig: any;
  groups: UIGroup[];
  allFields?: { name: string; group: string; label?: string }[];  // 所有字段列表（可选）
  onChange: (property: any, uiConfig: any) => void;
}

// 单个条件
interface CascadeCondition {
  field: string;
  operator: 'eq' | 'ne' | 'empty' | 'notEmpty' | 'gt' | 'lt' | 'in' | 'notIn';
  value?: unknown;
  not?: boolean;  // 取反
}

// 级联配置接口（支持单条件，后续可扩展为多条件）
interface CascadeFieldConfig {
  showWhen?: CascadeCondition;
  hideWhen?: CascadeCondition;
  requiredWith?: string[];
  conflictsWith?: string[];
  setValueWhen?: {
    field: string;
    operator: 'eq' | 'ne' | 'empty' | 'notEmpty';
    value?: unknown;
    setValue: unknown;
  };
}

// 级联操作符选项
const CASCADE_OPERATORS = [
  { value: 'eq', label: '等于' },
  { value: 'ne', label: '不等于' },
  { value: 'empty', label: '为空' },
  { value: 'notEmpty', label: '不为空' },
  { value: 'gt', label: '大于' },
  { value: 'lt', label: '小于' },
  { value: 'in', label: '在列表中' },
  { value: 'notIn', label: '不在列表中' },
];

const InlineFieldEditor: React.FC<InlineFieldEditorProps> = ({ fieldName, property, uiConfig, groups, allFields, onChange }) => {
  const [editedProperty, setEditedProperty] = useState({ ...property });
  const [editedUiConfig, setEditedUiConfig] = useState({ ...uiConfig });
  const [selectedValueType, setSelectedValueType] = useState(() => inferValueTypeFromSchema(property));
  const [activeTab, setActiveTab] = useState<'basic' | 'ui' | 'validation' | 'advanced' | 'cascade' | 'nested'>('basic');

  // 直接使用传入的 groups，确保始终使用最新的分组列表
  const effectiveGroups = useMemo((): UIGroup[] => {
    // console.log('🔍 InlineFieldEditor groups prop:', groups);
    
    // 如果 groups 为空或无效，使用默认分组
    if (!groups || !Array.isArray(groups) || groups.length === 0) {
      // console.log(' Using DEFAULT_GROUPS because groups is empty or invalid');
      return DEFAULT_GROUPS;
    }
    
    // 检查 groups 是否有有效的 id 和 label
    const validGroups = groups.filter(g => g && typeof g === 'object' && g.id && g.label);
    // console.log('✅ Valid groups:', validGroups);
    
    if (validGroups.length === 0) {
      // console.log(' Using DEFAULT_GROUPS because no valid groups found');
      return DEFAULT_GROUPS;
    }
    
    // 确保每个分组都有必要的属性，并返回正确类型
    const result = validGroups.map(g => ({
      id: String(g.id),
      label: String(g.label),
      level: (g.level === 'basic' ? 'basic' : 'advanced') as 'basic' | 'advanced',
      layout: (g.layout === 'tabs' ? 'tabs' : g.layout === 'accordion' ? 'accordion' : 'sections') as 'tabs' | 'accordion' | 'sections',
      order: typeof g.order === 'number' ? g.order : 100,
    }));
    
    console.log('📋 Effective groups:', result);
    return result;
  }, [groups]);

  // 当前选中的分组 ID，确保是有效的分组
  const currentGroupId = useMemo(() => {
    const groupId = editedUiConfig.group || 'advanced';
    // 检查当前分组是否在有效分组列表中
    const isValidGroup = effectiveGroups.some(g => g.id === groupId);
    return isValidGroup ? groupId : 'advanced';
  }, [editedUiConfig.group, effectiveGroups]);

  React.useEffect(() => {
    setEditedProperty({ ...property });
    setSelectedValueType(inferValueTypeFromSchema(property));
  }, [property]);

  React.useEffect(() => {
    setEditedUiConfig({ ...uiConfig });
  }, [uiConfig]);

  const handleValueTypeChange = (newValueType: string) => {
    setSelectedValueType(newValueType);
    const schemaConfig = getSchemaConfigForValueType(newValueType);
    const newProp = { ...schemaConfig, description: editedProperty.description, default: undefined };
    setEditedProperty(newProp);
    
    // 同步更新 Widget 类型
    const defaultWidget = getDefaultWidgetForValueType(newValueType);
    const newUi = { ...editedUiConfig, widget: defaultWidget };
    setEditedUiConfig(newUi);
    
    onChange(newProp, newUi);
  };

  const handlePropertyChange = (key: string, value: any) => {
    const newProp = { ...editedProperty, [key]: value };
    setEditedProperty(newProp);
    onChange(newProp, editedUiConfig);
  };

  const handleUiConfigChange = (key: string, value: any) => {
    const newUi = { ...editedUiConfig, [key]: value };
    // 确保保留 order 属性
    if (!newUi.order && uiConfig.order) {
      newUi.order = uiConfig.order;
    }
    setEditedUiConfig(newUi);

    // Widget 切换时检查默认值兼容性
    let updatedProperty = editedProperty;
    if (key === 'widget' && editedProperty.default !== undefined) {
      const currentDefault = editedProperty.default;
      if (value === 'switch') {
        // 切换到 switch：非 boolean 默认值 → clear
        if (typeof currentDefault !== 'boolean') {
          updatedProperty = { ...editedProperty, default: undefined };
          setEditedProperty(updatedProperty);
        }
      } else if (value === 'number') {
        // 切换到 number：非 number 默认值 → 尝试转换，失败则 clear
        if (typeof currentDefault !== 'number') {
          const num = Number(currentDefault);
          updatedProperty = { ...editedProperty, default: isNaN(num) ? undefined : num };
          setEditedProperty(updatedProperty);
        }
      }
      // 同类型 Widget 切换（key-value↔json-editor、text↔textarea 等）：默认值不变
    }

    // 使用 setTimeout 延迟触发 onChange，避免立即重新渲染导致编辑框折叠
    setTimeout(() => {
      onChange(updatedProperty, newUi);
    }, 0);
  };

  const currentTypeInfo = VALUE_TYPE_OPTIONS.find(opt => opt.value === selectedValueType);
  const showNestedTab = selectedValueType === 'object' || selectedValueType === 'object-list' || selectedValueType === 'dynamic-object';

  return (
    <div className={styles.inlineFieldEditor}>
      {/* 标签页导航 */}
      <div className={styles.inlineEditorTabs}>
        <button type="button" className={`${styles.inlineEditorTab} ${activeTab === 'basic' ? styles.active : ''}`} onClick={() => setActiveTab('basic')}>基础</button>
        <button type="button" className={`${styles.inlineEditorTab} ${activeTab === 'ui' ? styles.active : ''}`} onClick={() => setActiveTab('ui')}>UI配置</button>
        <button type="button" className={`${styles.inlineEditorTab} ${activeTab === 'validation' ? styles.active : ''}`} onClick={() => setActiveTab('validation')}>验证</button>
        <button type="button" className={`${styles.inlineEditorTab} ${activeTab === 'advanced' ? styles.active : ''}`} onClick={() => setActiveTab('advanced')}>高级</button>
        <button type="button" className={`${styles.inlineEditorTab} ${activeTab === 'cascade' ? styles.active : ''}`} onClick={() => setActiveTab('cascade')}>级联</button>
        {showNestedTab && <button type="button" className={`${styles.inlineEditorTab} ${activeTab === 'nested' ? styles.active : ''}`} onClick={() => setActiveTab('nested')}>嵌套字段</button>}
      </div>

      {/* 基础信息 */}
      {activeTab === 'basic' && (
        <div className={styles.inlineEditorSection}>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>值类型</label>
              <select value={selectedValueType} onChange={(e) => handleValueTypeChange(e.target.value)} className={styles.fieldSelect}>
                {VALUE_TYPE_OPTIONS.map(opt => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
              </select>
              {currentTypeInfo && <small className={styles.fieldHint}>{currentTypeInfo.description}</small>}
            </div>
            <div className={styles.inlineEditorField}>
              <label>分组</label>
              <select value={currentGroupId} onChange={(e) => handleUiConfigChange('group', e.target.value)} className={styles.fieldSelect}>
                {effectiveGroups.map(g => <option key={g.id} value={g.id}>{g.label}</option>)}
              </select>
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <label>描述</label>
              <textarea value={editedProperty.description || ''} onChange={(e) => handlePropertyChange('description', e.target.value)} className={styles.fieldTextarea} rows={2} placeholder="字段描述" />
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <label>默认值</label>
              <ExtendedDefaultValueInput valueType={selectedValueType} property={editedProperty} value={editedProperty.default} onChange={(val) => handlePropertyChange('default', val)} widget={editedUiConfig.widget} />
            </div>
          </div>
          {Object.prototype.hasOwnProperty.call(editedProperty, 'default') && editedProperty.default !== undefined && (
            <div className={styles.inlineEditorFormRow}>
              <div className={styles.inlineEditorFieldFull}>
                <label className={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={editedProperty['x-renderDefault'] === true}
                    onChange={(e) => handlePropertyChange('x-renderDefault', e.target.checked || undefined)}
                  />
                  输出默认值（创建资源时自动将默认值填入 JSON）
                </label>
              </div>
            </div>
          )}
        </div>
      )}

      {/* UI 配置 */}
      {activeTab === 'ui' && (
        <div className={styles.inlineEditorSection}>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>显示名称</label>
              <input type="text" value={editedUiConfig.label || ''} onChange={(e) => handleUiConfigChange('label', e.target.value)} className={styles.fieldInput} placeholder="例如：存储桶名称" />
            </div>
            <div className={styles.inlineEditorField}>
              <label>Widget 类型</label>
              <select value={editedUiConfig.widget || 'text'} onChange={(e) => handleUiConfigChange('widget', e.target.value)} className={styles.fieldSelect}>
                <option value="text">文本输入</option>
                <option value="textarea">多行文本</option>
                <option value="number">数字输入</option>
                <option value="select">下拉选择</option>
                <option value="multi-select">多选</option>
                <option value="switch">开关</option>
                <option value="tags">标签输入</option>
                <option value="key-value">键值对</option>
                <option value="object">对象编辑器</option>
                <option value="object-list">对象列表</option>
                <option value="dynamic-object">动态键对象</option>
                <option value="json-editor">JSON编辑器</option>
                <option value="password">密码输入</option>
              </select>
            </div>
          </div>
          {/* 开关组件专用配置 - 提示标签 */}
          {(selectedValueType === 'boolean' || editedUiConfig.widget === 'switch') && (
            <>
              <div className={styles.inlineEditorFormRow}>
                <div className={styles.inlineEditorField}>
                  <label>开启时提示</label>
                  <input type="text" value={editedUiConfig.checkedHint || ''} onChange={(e) => handleUiConfigChange('checkedHint', e.target.value || undefined)} className={styles.fieldInput} placeholder="例如：已启用 Karpenter" />
                </div>
                <div className={styles.inlineEditorField}>
                  <label>开启时颜色</label>
                  <select value={editedUiConfig.checkedHintColor || 'green'} onChange={(e) => handleUiConfigChange('checkedHintColor', e.target.value || undefined)} className={styles.fieldSelect}>
                    <option value="green">绿色 (green)</option>
                    <option value="blue">蓝色 (blue)</option>
                    <option value="cyan">青色 (cyan)</option>
                    <option value="purple">紫色 (purple)</option>
                    <option value="magenta">品红 (magenta)</option>
                    <option value="gold">金色 (gold)</option>
                    <option value="orange">橙色 (orange)</option>
                    <option value="red">红色 (red)</option>
                    <option value="volcano">火山红 (volcano)</option>
                    <option value="lime">青柠 (lime)</option>
                    <option value="geekblue">极客蓝 (geekblue)</option>
                    <option value="default">默认 (default)</option>
                  </select>
                </div>
              </div>
              <div className={styles.inlineEditorFormRow}>
                <div className={styles.inlineEditorField}>
                  <label>关闭时提示</label>
                  <input type="text" value={editedUiConfig.uncheckedHint || ''} onChange={(e) => handleUiConfigChange('uncheckedHint', e.target.value || undefined)} className={styles.fieldInput} placeholder="例如：使用传统节点组" />
                </div>
                <div className={styles.inlineEditorField}>
                  <label>关闭时颜色</label>
                  <select value={editedUiConfig.uncheckedHintColor || 'default'} onChange={(e) => handleUiConfigChange('uncheckedHintColor', e.target.value || undefined)} className={styles.fieldSelect}>
                    <option value="default">默认 (default)</option>
                    <option value="green">绿色 (green)</option>
                    <option value="blue">蓝色 (blue)</option>
                    <option value="cyan">青色 (cyan)</option>
                    <option value="purple">紫色 (purple)</option>
                    <option value="magenta">品红 (magenta)</option>
                    <option value="gold">金色 (gold)</option>
                    <option value="orange">橙色 (orange)</option>
                    <option value="red">红色 (red)</option>
                    <option value="volcano">火山红 (volcano)</option>
                    <option value="lime">青柠 (lime)</option>
                    <option value="geekblue">极客蓝 (geekblue)</option>
                  </select>
                </div>
              </div>
            </>
          )}
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>占位符</label>
              <input type="text" value={editedUiConfig.placeholder || ''} onChange={(e) => handleUiConfigChange('placeholder', e.target.value)} className={styles.fieldInput} placeholder="输入框占位符" />
            </div>
            <div className={styles.inlineEditorField}>
              <label>数据源类型</label>
              <select 
                value={
                  // 根据现有配置推断数据源类型
                  editedUiConfig.cmdbSource?.valueField ? 'cmdb' : 
                  editedUiConfig.source ? 'custom' : 
                  'none'
                } 
                onChange={(e) => {
                  const sourceType = e.target.value;
                  // 创建新的 uiConfig 对象，一次性更新所有相关字段
                  // 使用深拷贝确保不会有引用问题
                  const newUiConfig = JSON.parse(JSON.stringify(editedUiConfig));
                  
                  // 确保保留 order 属性
                  if (!newUiConfig.order && uiConfig.order) {
                    newUiConfig.order = uiConfig.order;
                  }
                  
                  if (sourceType === 'none') {
                    delete newUiConfig.sourceType;
                    delete newUiConfig.source;
                    delete newUiConfig.cmdbSource;
                  } else if (sourceType === 'cmdb') {
                    newUiConfig.sourceType = 'cmdb';
                    delete newUiConfig.source;
                    // 初始化 cmdbSource，确保有 valueField
                    newUiConfig.cmdbSource = { valueField: 'cloud_id' };
                  } else if (sourceType === 'custom') {
                    newUiConfig.sourceType = 'custom';
                    delete newUiConfig.cmdbSource;
                    // 保留 source 字段，让用户填写
                  }
                  
                  console.log('📝 CMDB source type changed:', sourceType, 'cmdbSource:', newUiConfig.cmdbSource, 'full config:', newUiConfig);
                  setEditedUiConfig(newUiConfig);
                  // 立即触发 onChange，不使用 setTimeout
                  onChange(editedProperty, newUiConfig);
                }} 
                className={styles.fieldSelect}
              >
                <option value="none">无（手动输入）</option>
                <option value="cmdb">CMDB 资源</option>
                <option value="custom">自定义数据源</option>
              </select>
            </div>
          </div>
          
          {/* 自定义数据源配置 */}
          {editedUiConfig.sourceType === 'custom' && (
            <div className={styles.inlineEditorFormRow}>
              <div className={styles.inlineEditorFieldFull}>
                <label>数据源名称</label>
                <input type="text" value={editedUiConfig.source || ''} onChange={(e) => handleUiConfigChange('source', e.target.value)} className={styles.fieldInput} placeholder="例如：ami_list, instance_types" />
              </div>
            </div>
          )}
          
          {/* CMDB 数据源配置 */}
          {(editedUiConfig.sourceType === 'cmdb' || editedUiConfig.cmdbSource) && (
            <div className={styles.cmdbSourceConfig}>
              <div className={styles.inlineEditorFormRow}>
                <div className={styles.inlineEditorField}>
                  <label>值字段（选择资源后填充的值）</label>
                  <select 
                    value={editedUiConfig.cmdbSource?.valueField || 'cloud_id'} 
                    onChange={(e) => handleUiConfigChange('cmdbSource', {
                      ...editedUiConfig.cmdbSource,
                      valueField: e.target.value,
                    })} 
                    className={styles.fieldSelect}
                  >
                    {CMDB_FIELD_DEFINITIONS.map(field => (
                      <option key={field.key} value={field.key}>
                        {field.label} - {field.description}
                      </option>
                    ))}
                  </select>
                </div>
                <div className={styles.inlineEditorField}>
                  <label>资源类型过滤（可选）</label>
                  <input 
                    type="text" 
                    value={editedUiConfig.cmdbSource?.resourceType || ''} 
                    onChange={(e) => {
                      const resourceType = e.target.value;
                      handleUiConfigChange('cmdbSource', {
                        ...editedUiConfig.cmdbSource,
                        resourceType: resourceType || undefined,
                      });
                    }} 
                    className={styles.fieldInput} 
                    placeholder="留空搜索所有资源" 
                    list="cmdb-resource-types"
                  />
                  <datalist id="cmdb-resource-types">
                    {Object.keys(RESOURCE_TYPE_RECOMMENDED_FIELDS).map(type => (
                      <option key={type} value={type} />
                    ))}
                  </datalist>
                </div>
              </div>
              <div className={styles.cmdbSourceInfo}>
                <span className={styles.cmdbInfoIcon}>💡</span>
                <span>从 CMDB 搜索已有云资源，选择后自动填充对应字段值，用户也可手动输入</span>
              </div>
            </div>
          )}
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>列宽</label>
              <select value={editedUiConfig.colSpan || 24} onChange={(e) => handleUiConfigChange('colSpan', Number(e.target.value))} className={styles.fieldInput}>
                <option value={24}>24 - 整行</option>
                <option value={12}>12 - 半行</option>
                <option value={8}>8 - 三分之一</option>
                <option value={6}>6 - 四分之一</option>
                <option value={16}>16 - 三分之二</option>
                <option value={18}>18 - 四分之三</option>
              </select>
            </div>
            <div className={styles.inlineEditorField}>
              <label>帮助文本</label>
              <input type="text" value={editedUiConfig.help || ''} onChange={(e) => handleUiConfigChange('help', e.target.value)} className={styles.fieldInput} placeholder="字段帮助说明" />
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <div className={styles.fieldCheckboxGroup}>
                <label><input type="checkbox" checked={editedUiConfig.searchable || false} onChange={(e) => handleUiConfigChange('searchable', e.target.checked)} /><span>支持搜索</span></label>
                <label><input type="checkbox" checked={editedUiConfig.allowCustom || false} onChange={(e) => handleUiConfigChange('allowCustom', e.target.checked)} /><span>允许自定义值</span></label>
                <label><input type="checkbox" checked={editedUiConfig.hiddenByDefault || false} onChange={(e) => handleUiConfigChange('hiddenByDefault', e.target.checked)} /><span>默认隐藏</span></label>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* 验证规则 */}
      {activeTab === 'validation' && (
        <div className={styles.inlineEditorSection}>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>正则表达式</label>
              <input type="text" value={editedProperty.pattern || ''} onChange={(e) => handlePropertyChange('pattern', e.target.value || undefined)} className={styles.fieldInput} placeholder="例如：^ami-[a-f0-9]+$" />
            </div>
            <div className={styles.inlineEditorField}>
              <label>枚举值（逗号分隔）</label>
              <input type="text" value={editedProperty.enum?.join(',') || ''} onChange={(e) => { const values = e.target.value ? e.target.value.split(',').map(v => v.trim()) : undefined; handlePropertyChange('enum', values); }} className={styles.fieldInput} placeholder="value1,value2,value3" />
            </div>
          </div>
          {(editedProperty.type === 'number' || editedProperty.type === 'integer') && (
            <div className={styles.inlineEditorFormRow}>
              <div className={styles.inlineEditorField}>
                <label>最小值</label>
                <input type="number" value={editedProperty.minimum ?? ''} onChange={(e) => handlePropertyChange('minimum', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
              <div className={styles.inlineEditorField}>
                <label>最大值</label>
                <input type="number" value={editedProperty.maximum ?? ''} onChange={(e) => handlePropertyChange('maximum', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
            </div>
          )}
          {editedProperty.type === 'string' && (
            <div className={styles.inlineEditorFormRow}>
              <div className={styles.inlineEditorField}>
                <label>最小长度</label>
                <input type="number" value={editedProperty.minLength ?? ''} onChange={(e) => handlePropertyChange('minLength', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
              <div className={styles.inlineEditorField}>
                <label>最大长度</label>
                <input type="number" value={editedProperty.maxLength ?? ''} onChange={(e) => handlePropertyChange('maxLength', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
            </div>
          )}
          {editedProperty.type === 'array' && (
            <div className={styles.inlineEditorFormRow}>
              <div className={styles.inlineEditorField}>
                <label>最少项数</label>
                <input type="number" value={editedProperty.minItems ?? ''} onChange={(e) => handlePropertyChange('minItems', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
              <div className={styles.inlineEditorField}>
                <label>最多项数</label>
                <input type="number" value={editedProperty.maxItems ?? ''} onChange={(e) => handlePropertyChange('maxItems', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
            </div>
          )}
        </div>
      )}

      {/* 高级选项 */}
      {activeTab === 'advanced' && (
        <div className={styles.inlineEditorSection}>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <div className={styles.fieldCheckboxGroup}>
                <label><input type="checkbox" checked={editedProperty['x-sensitive'] || false} onChange={(e) => handlePropertyChange('x-sensitive', e.target.checked || undefined)} /><span>敏感字段（密码等）</span></label>
                <label><input type="checkbox" checked={editedProperty.readOnly || false} onChange={(e) => handlePropertyChange('readOnly', e.target.checked || undefined)} /><span>只读字段</span></label>
                <label><input type="checkbox" checked={editedProperty.deprecated || false} onChange={(e) => handlePropertyChange('deprecated', e.target.checked || undefined)} /><span>已弃用</span></label>
              </div>
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <label>格式 (format)</label>
              <select value={editedProperty.format || ''} onChange={(e) => handlePropertyChange('format', e.target.value || undefined)} className={styles.fieldSelect}>
                <option value="">无</option>
                <option value="date">日期 (date)</option>
                <option value="date-time">日期时间 (date-time)</option>
                <option value="email">邮箱 (email)</option>
                <option value="uri">URI</option>
                <option value="hostname">主机名 (hostname)</option>
                <option value="ipv4">IPv4</option>
                <option value="ipv6">IPv6</option>
                <option value="json">JSON</option>
                <option value="password">密码 (password)</option>
              </select>
            </div>
          </div>
        </div>
      )}

      {/* 级联配置 */}
      {activeTab === 'cascade' && (
        <div className={styles.inlineEditorSection}>
          <CascadeConfigEditor
            fieldName={fieldName}
            cascade={editedUiConfig.cascade || {}}
            currentGroup={currentGroupId}
            groups={effectiveGroups}
            allFields={allFields || []}
            onChange={(cascade) => handleUiConfigChange('cascade', Object.keys(cascade).length > 0 ? cascade : undefined)}
          />
        </div>
      )}

      {/* 嵌套字段 */}
      {activeTab === 'nested' && showNestedTab && (
        <div className={styles.inlineEditorSection}>
          <NestedFieldsEditor
            property={editedProperty}
            valueType={selectedValueType}
            onChange={(newProperty) => {
              setEditedProperty(newProperty);
              onChange(newProperty, editedUiConfig);
            }}
            groups={effectiveGroups}
          />
        </div>
      )}
    </div>
  );
};

// ============ 级联配置编辑器 ============
interface CascadeConfigEditorProps {
  fieldName: string;
  cascade: CascadeFieldConfig;
  currentGroup: string;
  groups: UIGroup[];
  allFields: { name: string; group: string; label?: string }[];
  onChange: (cascade: CascadeFieldConfig) => void;
}

const CascadeConfigEditor: React.FC<CascadeConfigEditorProps> = ({ fieldName, cascade, currentGroup, groups, allFields, onChange }) => {
  const [editedCascade, setEditedCascade] = useState<CascadeFieldConfig>({ ...cascade });
  
  // 同步外部变化
  React.useEffect(() => {
    setEditedCascade({ ...cascade });
  }, [cascade]);

  const handleChange = (newCascade: CascadeFieldConfig) => {
    setEditedCascade(newCascade);
    onChange(newCascade);
  };

  // 显示条件
  const handleShowWhenChange = (key: string, value: any) => {
    const newShowWhen = { ...editedCascade.showWhen, [key]: value };
    if (!newShowWhen.field) delete newShowWhen.field;
    if (!newShowWhen.operator) newShowWhen.operator = 'eq';
    handleChange({ ...editedCascade, showWhen: Object.keys(newShowWhen).length > 0 && newShowWhen.field ? newShowWhen as any : undefined });
  };

  // 隐藏条件
  const handleHideWhenChange = (key: string, value: any) => {
    const newHideWhen = { ...editedCascade.hideWhen, [key]: value };
    if (!newHideWhen.field) delete newHideWhen.field;
    if (!newHideWhen.operator) newHideWhen.operator = 'eq';
    handleChange({ ...editedCascade, hideWhen: Object.keys(newHideWhen).length > 0 && newHideWhen.field ? newHideWhen as any : undefined });
  };

  // 依赖字段
  const handleRequiredWithChange = (value: string) => {
    const fields = value.split(',').map(f => f.trim()).filter(f => f);
    handleChange({ ...editedCascade, requiredWith: fields.length > 0 ? fields : undefined });
  };

  // 冲突字段
  const handleConflictsWithChange = (value: string) => {
    const fields = value.split(',').map(f => f.trim()).filter(f => f);
    handleChange({ ...editedCascade, conflictsWith: fields.length > 0 ? fields : undefined });
  };

  // 判断操作符是否需要值
  const operatorNeedsValue = (op: string) => !['empty', 'notEmpty'].includes(op);

  return (
    <div className={styles.cascadeConfigEditor}>
      {/* 显示条件 */}
      <div className={styles.cascadeSection}>
        <h4 className={styles.cascadeSectionTitle}>
          显示条件 (showWhen)
          <span className={styles.cascadeHint}>当满足条件时显示此字段</span>
        </h4>
        <div className={styles.inlineEditorFormRow}>
          <div className={styles.inlineEditorField}>
            <label>触发字段</label>
            <select
              value={editedCascade.showWhen?.field || ''}
              onChange={(e) => handleShowWhenChange('field', e.target.value || undefined)}
              className={styles.fieldSelect}
            >
              <option value="">请选择字段</option>
              {allFields.filter(f => f.name !== fieldName).map(f => (
                <option key={f.name} value={f.name}>{f.label || f.name} ({f.group})</option>
              ))}
            </select>
          </div>
          <div className={styles.inlineEditorField}>
            <label>操作符</label>
            <select
              value={editedCascade.showWhen?.operator || 'eq'}
              onChange={(e) => handleShowWhenChange('operator', e.target.value)}
              className={styles.fieldSelect}
            >
              {CASCADE_OPERATORS.map(op => (
                <option key={op.value} value={op.value}>{op.label}</option>
              ))}
            </select>
          </div>
          {operatorNeedsValue(editedCascade.showWhen?.operator || 'eq') && (
            <div className={styles.inlineEditorField}>
              <label>比较值</label>
              <input
                type="text"
                value={editedCascade.showWhen?.value !== undefined ? String(editedCascade.showWhen.value) : ''}
                onChange={(e) => {
                  let val: any = e.target.value;
                  if (val === 'true') val = true;
                  else if (val === 'false') val = false;
                  else if (!isNaN(Number(val)) && val !== '') val = Number(val);
                  handleShowWhenChange('value', val || undefined);
                }}
                className={styles.fieldInput}
                placeholder="true / false / 数值 / 字符串"
              />
            </div>
          )}
        </div>
      </div>

      {/* 隐藏条件 */}
      <div className={styles.cascadeSection}>
        <h4 className={styles.cascadeSectionTitle}>
          隐藏条件 (hideWhen)
          <span className={styles.cascadeHint}>当满足条件时隐藏此字段</span>
        </h4>
        <div className={styles.inlineEditorFormRow}>
          <div className={styles.inlineEditorField}>
            <label>触发字段</label>
            <select
              value={editedCascade.hideWhen?.field || ''}
              onChange={(e) => handleHideWhenChange('field', e.target.value || undefined)}
              className={styles.fieldSelect}
            >
              <option value="">请选择字段</option>
              {allFields.filter(f => f.name !== fieldName).map(f => (
                <option key={f.name} value={f.name}>{f.label || f.name} ({f.group})</option>
              ))}
            </select>
          </div>
          <div className={styles.inlineEditorField}>
            <label>操作符</label>
            <select
              value={editedCascade.hideWhen?.operator || 'eq'}
              onChange={(e) => handleHideWhenChange('operator', e.target.value)}
              className={styles.fieldSelect}
            >
              {CASCADE_OPERATORS.map(op => (
                <option key={op.value} value={op.value}>{op.label}</option>
              ))}
            </select>
          </div>
          {operatorNeedsValue(editedCascade.hideWhen?.operator || 'eq') && (
            <div className={styles.inlineEditorField}>
              <label>比较值</label>
              <input
                type="text"
                value={editedCascade.hideWhen?.value !== undefined ? String(editedCascade.hideWhen.value) : ''}
                onChange={(e) => {
                  let val: any = e.target.value;
                  if (val === 'true') val = true;
                  else if (val === 'false') val = false;
                  else if (!isNaN(Number(val)) && val !== '') val = Number(val);
                  handleHideWhenChange('value', val || undefined);
                }}
                className={styles.fieldInput}
                placeholder="true / false / 数值 / 字符串"
              />
            </div>
          )}
        </div>
      </div>

      {/* 依赖字段 */}
      <div className={styles.cascadeSection}>
        <h4 className={styles.cascadeSectionTitle}>
          依赖字段 (requiredWith)
          <span className={styles.cascadeHint}>当此字段有值时，以下字段也必须有值</span>
        </h4>
        {/* 已选择的字段预览 */}
        {(editedCascade.requiredWith?.length || 0) > 0 && (
          <div className={styles.selectedFieldsPreview}>
            <span className={styles.selectedLabel}>已选择 ({editedCascade.requiredWith?.length}):</span>
            {editedCascade.requiredWith?.map(fieldName => {
              const field = allFields.find(f => f.name === fieldName);
              return (
                <span key={fieldName} className={styles.selectedTag}>
                  {field?.label || fieldName}
                  <button type="button" onClick={() => {
                    const newList = (editedCascade.requiredWith || []).filter(n => n !== fieldName);
                    handleChange({ ...editedCascade, requiredWith: newList.length > 0 ? newList : undefined });
                  }}>✕</button>
                </span>
              );
            })}
          </div>
        )}
        <div className={styles.inlineEditorFormRow}>
          <div className={styles.inlineEditorFieldFull}>
            <div className={styles.checkboxList}>
              {allFields.filter(f => f.name !== fieldName).map(f => (
                <label key={f.name} className={styles.checkboxItem}>
                  <input
                    type="checkbox"
                    checked={(editedCascade.requiredWith || []).includes(f.name)}
                    onChange={(e) => {
                      const current = editedCascade.requiredWith || [];
                      const newList = e.target.checked
                        ? [...current, f.name]
                        : current.filter(n => n !== f.name);
                      handleChange({ ...editedCascade, requiredWith: newList.length > 0 ? newList : undefined });
                    }}
                  />
                  <span>{f.label || f.name}</span>
                  <small>({f.group})</small>
                </label>
              ))}
            </div>
            {allFields.filter(f => f.name !== fieldName).length === 0 && (
              <div className={styles.emptyHint}>没有可选的字段</div>
            )}
          </div>
        </div>
      </div>

      {/* 冲突字段 */}
      <div className={styles.cascadeSection}>
        <h4 className={styles.cascadeSectionTitle}>
          冲突字段 (conflictsWith)
          <span className={styles.cascadeHint}>当此字段有值时，以下字段将被清空</span>
        </h4>
        {/* 已选择的字段预览 */}
        {(editedCascade.conflictsWith?.length || 0) > 0 && (
          <div className={styles.selectedFieldsPreview}>
            <span className={styles.selectedLabel}>已选择 ({editedCascade.conflictsWith?.length}):</span>
            {editedCascade.conflictsWith?.map(fieldName => {
              const field = allFields.find(f => f.name === fieldName);
              return (
                <span key={fieldName} className={styles.selectedTag}>
                  {field?.label || fieldName}
                  <button type="button" onClick={() => {
                    const newList = (editedCascade.conflictsWith || []).filter(n => n !== fieldName);
                    handleChange({ ...editedCascade, conflictsWith: newList.length > 0 ? newList : undefined });
                  }}>✕</button>
                </span>
              );
            })}
          </div>
        )}
        <div className={styles.inlineEditorFormRow}>
          <div className={styles.inlineEditorFieldFull}>
            <div className={styles.checkboxList}>
              {allFields.filter(f => f.name !== fieldName).map(f => (
                <label key={f.name} className={styles.checkboxItem}>
                  <input
                    type="checkbox"
                    checked={(editedCascade.conflictsWith || []).includes(f.name)}
                    onChange={(e) => {
                      const current = editedCascade.conflictsWith || [];
                      const newList = e.target.checked
                        ? [...current, f.name]
                        : current.filter(n => n !== f.name);
                      handleChange({ ...editedCascade, conflictsWith: newList.length > 0 ? newList : undefined });
                    }}
                  />
                  <span>{f.label || f.name}</span>
                  <small>({f.group})</small>
                </label>
              ))}
            </div>
            {allFields.filter(f => f.name !== fieldName).length === 0 && (
              <div className={styles.emptyHint}>没有可选的字段</div>
            )}
          </div>
        </div>
      </div>

      {/* 配置预览 */}
      {(editedCascade.showWhen?.field || editedCascade.hideWhen?.field || editedCascade.requiredWith?.length || editedCascade.conflictsWith?.length) && (
        <div className={styles.cascadePreview}>
          <h4>配置预览</h4>
          <pre>{JSON.stringify(editedCascade, null, 2)}</pre>
        </div>
      )}
    </div>
  );
};

// ============ 嵌套字段编辑器 ============
interface NestedFieldsEditorProps {
  property: any;
  valueType: string;
  onChange: (newProperty: any) => void;
  depth?: number;
  groups?: UIGroup[];  // 新增：自定义分组列表
}

// ---- 嵌套布局：可拖拽卡片 ----
const NestedDraggableCard: React.FC<{ fieldName: string; property: any; fieldsInRow: number }> = ({ fieldName, property, fieldsInRow }) => {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: fieldName });
  return (
    <div ref={setNodeRef} className={styles.layoutCard} style={{ opacity: isDragging ? 0.3 : 1, flex: `1 1 ${100 / fieldsInRow}%` }} {...attributes} {...listeners}>
      <div className={styles.layoutCardHeader}>
        <span className={styles.layoutCardDrag}>⋮⋮</span>
        <span className={styles.layoutCardName}>{fieldName}</span>
      </div>
      <div className={styles.layoutCardMeta}>
        {property?.title && <span className={styles.layoutCardLabel}>{property.title}</span>}
        <span className={styles.layoutCardType}>{property?.type || 'string'}</span>
        <span className={styles.layoutCardSpan}>span {property?.['x-colSpan'] || 24}</span>
      </div>
    </div>
  );
};

// ---- 嵌套布局：可放置的行区域 ----
const NestedDroppableRow: React.FC<{ id: string; rowIdx: number; children: React.ReactNode; spanLabel: string }> = ({ id, rowIdx, children, spanLabel }) => {
  const { setNodeRef, isOver } = useDroppable({ id });
  return (
    <div ref={setNodeRef} className={`${styles.layoutRow} ${isOver ? styles.layoutRowDragOver : ''}`}>
      <div className={styles.layoutRowLabel}>行 {rowIdx + 1} · {spanLabel}</div>
      <div className={styles.layoutRowContent}>{children}</div>
    </div>
  );
};

// ---- 嵌套布局：行间分隔放置区（拖到此处创建新行） ----
const NestedRowSeparator: React.FC<{ id: string; isDragging?: boolean }> = ({ id, isDragging }) => {
  const { setNodeRef, isOver } = useDroppable({ id });
  return (
    <div ref={setNodeRef} className={`${styles.nestedRowSeparator} ${isOver ? styles.nestedRowSeparatorActive : ''} ${isDragging && !isOver ? styles.nestedRowSeparatorDragging : ''}`}>
      {isOver ? <span className={styles.nestedRowSeparatorLabel}>释放以创建新行</span> : isDragging ? <span className={styles.nestedRowSeparatorLabel}>拖到此处新建一行</span> : null}
    </div>
  );
};

const NestedFieldsEditor: React.FC<NestedFieldsEditorProps> = ({ property, valueType, onChange, depth = 1, groups }) => {
  const [expandedField, setExpandedField] = useState<string | null>(null);
  const [newFieldName, setNewFieldName] = useState('');
  const [showAddField, setShowAddField] = useState(false);
  const [layoutMode, setLayoutMode] = useState(false);

  const getNestedProperties = (): Record<string, any> => {
    if (valueType === 'object-list') return property.items?.properties || {};
    if (valueType === 'dynamic-object') return property.additionalProperties?.properties || {};
    return property.properties || {};
  };

  const setNestedProperties = (newProperties: Record<string, any>) => {
    if (valueType === 'object-list') {
      onChange({ ...property, items: { ...property.items, type: 'object', properties: newProperties } });
    } else if (valueType === 'dynamic-object') {
      onChange({ ...property, additionalProperties: { ...property.additionalProperties, type: 'object', properties: newProperties } });
    } else {
      onChange({ ...property, properties: newProperties });
    }
  };

  const nestedProperties = getNestedProperties();
  const nestedFieldNames = Object.keys(nestedProperties).sort((a, b) => {
    const orderA = nestedProperties[a]?.['x-order'] ?? 999;
    const orderB = nestedProperties[b]?.['x-order'] ?? 999;
    return orderA - orderB;
  });

  const handleAddNestedField = () => {
    if (!newFieldName.trim()) return;
    const fieldName = newFieldName.trim();
    if (nestedProperties[fieldName]) { alert(`字段 "${fieldName}" 已存在`); return; }
    const newProperties = { ...nestedProperties, [fieldName]: { type: 'string', title: fieldName, description: '' } };
    setNestedProperties(newProperties);
    setNewFieldName('');
    setShowAddField(false);
    setExpandedField(fieldName);
  };

  const handleDeleteNestedField = (fieldName: string) => {
    const newProperties = { ...nestedProperties };
    delete newProperties[fieldName];
    setNestedProperties(newProperties);
    if (expandedField === fieldName) setExpandedField(null);
  };

  const handleUpdateNestedField = (fieldName: string, newFieldDef: any) => {
    const newProperties = { ...nestedProperties, [fieldName]: newFieldDef };
    setNestedProperties(newProperties);
  };

  const getNestedTypeDisplay = (prop: any): string => {
    if (prop.type === 'array') return `array[${prop.items?.type || 'any'}]`;
    if (prop.type === 'object') return prop.additionalProperties ? 'map' : 'object';
    return prop.type || 'string';
  };

  const getNestedGroupDisplay = (prop: any): { label: string; isBasic: boolean } => {
    const groupId = prop['x-group'] || 'basic';
    return {
      label: groupId === 'basic' ? '基础' : '高级',
      isBasic: groupId === 'basic'
    };
  };

  // --- Layout mode: build rows from x-colSpan ---
  const nestedLayoutRows = useMemo(() => {
    const rows: LayoutRow[] = [];
    let currentRow: string[] = [];
    let currentSpan = 0;
    for (const name of nestedFieldNames) {
      const span = nestedProperties[name]?.['x-colSpan'] || 24;
      if (currentSpan + span > 24 && currentRow.length > 0) {
        rows.push({ fields: currentRow });
        currentRow = [];
        currentSpan = 0;
      }
      currentRow.push(name);
      currentSpan += span;
      if (currentSpan >= 24) {
        rows.push({ fields: currentRow });
        currentRow = [];
        currentSpan = 0;
      }
    }
    if (currentRow.length > 0) rows.push({ fields: currentRow });
    return rows;
  }, [nestedFieldNames, nestedProperties]);

  const nestedSensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const nestedCollisionDetection = useMemo(
    () => createSeparatorFirstCollision('nested-sep-', 'nested-row-'),
    []
  );

  const [activeDragId, setActiveDragId] = useState<string | null>(null);

  const handleNestedDragStart = useCallback((event: DragStartEvent) => {
    setActiveDragId(String(event.active.id));
  }, []);

  const handleNestedDragEnd = useCallback((event: DragEndEvent) => {
    setActiveDragId(null);
    const { active, over } = event;
    if (!over) return;

    const activeId = String(active.id);
    const overId = String(over.id);

    const mutableRows = nestedLayoutRows.map(r => ({ fields: [...r.fields] }));

    // Find source row
    let sourceRowIdx = -1;
    for (let i = 0; i < mutableRows.length; i++) {
      if (mutableRows[i].fields.includes(activeId)) { sourceRowIdx = i; break; }
    }
    if (sourceRowIdx === -1) return;

    // Drop on same row → no-op
    if (overId === `nested-row-${sourceRowIdx}`) return;

    // Remove from source row
    mutableRows[sourceRowIdx].fields = mutableRows[sourceRowIdx].fields.filter(f => f !== activeId);

    if (overId.startsWith('nested-row-')) {
      // Drop onto existing row → merge into that row
      const targetRowIdx = parseInt(overId.replace('nested-row-', ''), 10);
      mutableRows[targetRowIdx].fields.push(activeId);
    } else if (overId.startsWith('nested-sep-')) {
      // Drop onto separator → create new row at that position
      const sepIdx = parseInt(overId.replace('nested-sep-', ''), 10);
      mutableRows.splice(sepIdx, 0, { fields: [activeId] });
    } else {
      return;
    }

    // Remove empty rows
    const finalRows = mutableRows.filter(r => r.fields.length > 0);

    // Rebuild properties with updated order, x-colSpan, and x-order
    const newProperties: Record<string, any> = {};
    let orderIndex = 0;
    for (const row of finalRows) {
      const colSpan = Math.floor(24 / row.fields.length);
      for (const name of row.fields) {
        newProperties[name] = { ...nestedProperties[name], 'x-colSpan': colSpan, 'x-order': orderIndex++ };
      }
    }
    setNestedProperties(newProperties);
  }, [nestedLayoutRows, nestedProperties, setNestedProperties]);

  return (
    <div className={styles.nestedFieldsContainer}>
      {nestedFieldNames.length > 1 && (
        <div className={styles.nestedLayoutToggle}>
          <button type="button" className={`${styles.nestedLayoutToggleBtn} ${layoutMode ? styles.active : ''}`} onClick={() => setLayoutMode(!layoutMode)}>
            {layoutMode ? '← 列表' : '⊞ 布局'}
          </button>
        </div>
      )}
      {nestedFieldNames.length === 0 ? (
        <div className={styles.emptyNestedFields}>暂无嵌套字段。点击下方按钮添加。</div>
      ) : layoutMode ? (
        <div className={styles.layoutView}>
          <div className={styles.layoutHint}>
            拖拽字段到虚线行可合并，拖到行间蓝色区域可拆分为新行。
          </div>
          <DndContext sensors={nestedSensors} collisionDetection={nestedCollisionDetection} onDragStart={handleNestedDragStart} onDragEnd={handleNestedDragEnd}>
            <div className={styles.layoutRows}>
              <NestedRowSeparator id="nested-sep-0" isDragging={!!activeDragId} />
              {nestedLayoutRows.map((row, rowIdx) => (
                <React.Fragment key={rowIdx}>
                  <NestedDroppableRow id={`nested-row-${rowIdx}`} rowIdx={rowIdx} spanLabel={`${row.fields.length} 个字段 · span ${row.fields.map(f => nestedProperties[f]?.['x-colSpan'] || 24).join('+')}`}>
                    {row.fields.map((fieldName) => (
                      <NestedDraggableCard
                        key={fieldName}
                        fieldName={fieldName}
                        property={nestedProperties[fieldName]}
                        fieldsInRow={row.fields.length}
                      />
                    ))}
                  </NestedDroppableRow>
                  <NestedRowSeparator id={`nested-sep-${rowIdx + 1}`} isDragging={!!activeDragId} />
                </React.Fragment>
              ))}
            </div>
            <DragOverlay>
              {activeDragId ? (
                <div className={styles.layoutCard} style={{ width: 180, opacity: 0.9 }}>
                  <div className={styles.layoutCardHeader}>
                    <span className={styles.layoutCardDrag}>⋮⋮</span>
                    <span className={styles.layoutCardName}>{activeDragId}</span>
                  </div>
                  <div className={styles.layoutCardMeta}>
                    <span className={styles.layoutCardType}>{nestedProperties[activeDragId]?.type || 'string'}</span>
                    <span className={styles.layoutCardSpan}>span {nestedProperties[activeDragId]?.['x-colSpan'] || 24}</span>
                  </div>
                </div>
              ) : null}
            </DragOverlay>
          </DndContext>
        </div>
      ) : (
        <div className={styles.nestedFieldsList}>
          {nestedFieldNames.map((fieldName) => {
            const fieldDef = nestedProperties[fieldName];
            const isExpanded = expandedField === fieldName;
            const groupInfo = getNestedGroupDisplay(fieldDef);
            return (
              <div key={fieldName} className={styles.nestedFieldItem}>
                <div className={`${styles.nestedFieldHeader} ${isExpanded ? styles.expanded : ''}`} onClick={() => setExpandedField(isExpanded ? null : fieldName)}>
                  <span className={styles.expandIcon}>{isExpanded ? '▼' : '▶'}</span>
                  <code className={styles.nestedFieldName}>{fieldName}</code>
                  <span className={styles.nestedFieldType}>{getNestedTypeDisplay(fieldDef)}</span>
                  <span className={`${styles.nestedFieldGroup} ${groupInfo.isBasic ? styles.basicLevel : styles.advancedLevel}`}>{groupInfo.label}</span>
                  <div className={styles.nestedFieldActions} onClick={(e) => e.stopPropagation()}>
                    <button type="button" onClick={() => handleDeleteNestedField(fieldName)} className={styles.nestedDeleteButton} title="删除字段">✕</button>
                  </div>
                </div>
                {isExpanded && depth < 5 && (
                  <div className={styles.nestedFieldInlineEditor}>
                    <NestedFieldInlineEditor fieldName={fieldName} fieldDef={fieldDef} onChange={(newFieldDef) => handleUpdateNestedField(fieldName, newFieldDef)} depth={depth} groups={groups} />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
      {showAddField ? (
        <div className={styles.addNestedFieldForm}>
          <input type="text" value={newFieldName} onChange={(e) => setNewFieldName(e.target.value)} placeholder="输入字段名" className={styles.fieldInput} autoFocus onKeyDown={(e) => { if (e.key === 'Enter') handleAddNestedField(); if (e.key === 'Escape') { setShowAddField(false); setNewFieldName(''); } }} />
          <button type="button" onClick={handleAddNestedField} className={styles.saveButton} disabled={!newFieldName.trim()}>添加</button>
          <button type="button" onClick={() => { setShowAddField(false); setNewFieldName(''); }} className={styles.cancelButton}>取消</button>
        </div>
      ) : (
        <button type="button" onClick={() => setShowAddField(true)} className={styles.addNestedFieldButton}>+ 添加嵌套字段</button>
      )}
    </div>
  );
};

// ============ 嵌套字段内联编辑器 ============
interface NestedFieldInlineEditorProps {
  fieldName: string;
  fieldDef: any;
  onChange: (newFieldDef: any) => void;
  depth?: number;
  groups?: UIGroup[];  // 新增：自定义分组列表
}

const NestedFieldInlineEditor: React.FC<NestedFieldInlineEditorProps> = ({ fieldName, fieldDef, onChange, depth = 1, groups }) => {
  const [editedField, setEditedField] = useState(() => JSON.parse(JSON.stringify(fieldDef)));
  const [selectedType, setSelectedType] = useState(() => inferValueTypeFromSchema(fieldDef));
  const [activeSection, setActiveSection] = useState<string>('basic');

  // 当 fieldDef 变化时更新本地状态
  React.useEffect(() => { 
    setEditedField(JSON.parse(JSON.stringify(fieldDef))); 
    setSelectedType(inferValueTypeFromSchema(fieldDef)); 
  }, [JSON.stringify(fieldDef)]);

  // 子参数的分组选项 - 使用传入的 groups 或默认分组
  const nestedGroups = useMemo(() => {
    if (groups && groups.length > 0) {
      return groups.map(g => ({ id: g.id, label: g.label }));
    }
    return [
      { id: 'basic', label: '基础配置' },
      { id: 'advanced', label: '高级配置' },
    ];
  }, [groups]);

  const handleTypeChange = (newType: string) => {
    setSelectedType(newType);
    const schemaConfig = getSchemaConfigForValueType(newType);
    // 保留 title 和 description，同时设置默认的 widget 类型
    const defaultWidget = getDefaultWidgetForValueType(newType);
    const newFieldDef = { 
      ...schemaConfig, 
      title: editedField.title || fieldName, 
      description: editedField.description || '',
      'x-widget': defaultWidget,
      default: undefined 
    };
    setEditedField(newFieldDef);
    onChange(newFieldDef);
  };

  const handleFieldChange = (key: string, value: any) => {
    const newFieldDef = JSON.parse(JSON.stringify(editedField));
    if (value === undefined || value === '') {
      delete newFieldDef[key];
    } else {
      newFieldDef[key] = value;
    }
    setEditedField(newFieldDef);
    onChange(newFieldDef);
  };

  const currentTypeInfo = VALUE_TYPE_OPTIONS.find(opt => opt.value === selectedType);
  const showNestedTab = (selectedType === 'object' || selectedType === 'object-list' || selectedType === 'dynamic-object') && depth < 5;

  return (
    <div className={styles.nestedFieldInlineEditor}>
      <div className={styles.inlineEditorTabs}>
        <button type="button" className={`${styles.inlineEditorTab} ${activeSection === 'basic' ? styles.active : ''}`} onClick={() => setActiveSection('basic')}>基础</button>
        <button type="button" className={`${styles.inlineEditorTab} ${activeSection === 'ui' ? styles.active : ''}`} onClick={() => setActiveSection('ui')}>UI配置</button>
        <button type="button" className={`${styles.inlineEditorTab} ${activeSection === 'validation' ? styles.active : ''}`} onClick={() => setActiveSection('validation')}>验证</button>
        <button type="button" className={`${styles.inlineEditorTab} ${activeSection === 'advanced' ? styles.active : ''}`} onClick={() => setActiveSection('advanced')}>高级</button>
        {showNestedTab && <button type="button" className={`${styles.inlineEditorTab} ${activeSection === 'nested' ? styles.active : ''}`} onClick={() => setActiveSection('nested')}>嵌套字段</button>}
      </div>
      {activeSection === 'basic' && (
        <div className={styles.inlineEditorSection}>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>值类型</label>
              <select value={selectedType} onChange={(e) => handleTypeChange(e.target.value)} className={styles.fieldSelect}>
                {VALUE_TYPE_OPTIONS.map(opt => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
              </select>
              {currentTypeInfo && <small className={styles.fieldHint}>{currentTypeInfo.description}</small>}
            </div>
            <div className={styles.inlineEditorField}>
              <label>分组</label>
              <select value={editedField['x-group'] || 'basic'} onChange={(e) => handleFieldChange('x-group', e.target.value)} className={styles.fieldSelect}>
                {nestedGroups.map(g => <option key={g.id} value={g.id}>{g.label}</option>)}
              </select>
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>标题</label>
              <input type="text" value={editedField.title || ''} onChange={(e) => handleFieldChange('title', e.target.value)} className={styles.fieldInput} placeholder="显示名称" />
            </div>
            <div className={styles.inlineEditorField}>
              <label>字段名</label>
              <input type="text" value={fieldName} disabled className={styles.fieldInput} style={{ background: '#f3f4f6', color: '#6b7280' }} />
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <label>描述</label>
              <textarea value={editedField.description || ''} onChange={(e) => handleFieldChange('description', e.target.value)} className={styles.fieldTextarea} rows={2} placeholder="字段描述" />
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <label>默认值</label>
              <ExtendedDefaultValueInput valueType={selectedType} property={editedField} value={editedField.default} onChange={(val) => handleFieldChange('default', val)} widget={editedField['x-widget']} />
            </div>
          </div>
        </div>
      )}
      {activeSection === 'ui' && (
        <div className={styles.inlineEditorSection}>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>Widget 类型</label>
              <select value={editedField['x-widget'] || ''} onChange={(e) => handleFieldChange('x-widget', e.target.value || undefined)} className={styles.fieldSelect}>
                <option value="">自动</option>
                <option value="text">文本输入</option>
                <option value="textarea">多行文本</option>
                <option value="number">数字输入</option>
                <option value="select">下拉选择</option>
                <option value="multi-select">多选</option>
                <option value="switch">开关</option>
                <option value="tags">标签输入</option>
                <option value="key-value">键值对</option>
                <option value="object">对象编辑器</option>
                <option value="object-list">对象列表</option>
                <option value="dynamic-object">动态键对象</option>
                <option value="json-editor">JSON编辑器</option>
                <option value="password">密码输入</option>
              </select>
            </div>
            <div className={styles.inlineEditorField}>
              <label>占位符</label>
              <input type="text" value={editedField['x-placeholder'] || ''} onChange={(e) => handleFieldChange('x-placeholder', e.target.value || undefined)} className={styles.fieldInput} placeholder="输入框占位符" />
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>列宽</label>
              <select value={editedField['x-colSpan'] || 24} onChange={(e) => handleFieldChange('x-colSpan', Number(e.target.value))} className={styles.fieldSelect}>
                <option value={24}>24 - 整行</option>
                <option value={12}>12 - 半行</option>
                <option value={8}>8 - 三分之一</option>
                <option value={6}>6 - 四分之一</option>
                <option value={16}>16 - 三分之二</option>
                <option value={18}>18 - 四分之三</option>
              </select>
            </div>
            <div className={styles.inlineEditorField}>
              <label>帮助文本</label>
              <input type="text" value={editedField['x-help'] || ''} onChange={(e) => handleFieldChange('x-help', e.target.value || undefined)} className={styles.fieldInput} placeholder="字段帮助说明" />
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>外部数据源</label>
              <input type="text" value={editedField['x-source'] || ''} onChange={(e) => handleFieldChange('x-source', e.target.value || undefined)} className={styles.fieldInput} placeholder="例如：ami_list" />
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <div className={styles.fieldCheckboxGroup}>
                <label><input type="checkbox" checked={editedField['x-sensitive'] || false} onChange={(e) => handleFieldChange('x-sensitive', e.target.checked || undefined)} /><span>敏感字段</span></label>
                <label><input type="checkbox" checked={editedField.readOnly || false} onChange={(e) => handleFieldChange('readOnly', e.target.checked || undefined)} /><span>只读</span></label>
                <label><input type="checkbox" checked={editedField['x-hidden'] || false} onChange={(e) => handleFieldChange('x-hidden', e.target.checked || undefined)} /><span>默认隐藏</span></label>
              </div>
            </div>
          </div>
        </div>
      )}
      {activeSection === 'validation' && (
        <div className={styles.inlineEditorSection}>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorField}>
              <label>正则表达式</label>
              <input type="text" value={editedField.pattern || ''} onChange={(e) => handleFieldChange('pattern', e.target.value || undefined)} className={styles.fieldInput} placeholder="^ami-[a-f0-9]+$" />
            </div>
            <div className={styles.inlineEditorField}>
              <label>枚举值</label>
              <input type="text" value={editedField.enum?.join(',') || ''} onChange={(e) => { const values = e.target.value ? e.target.value.split(',').map(v => v.trim()) : undefined; handleFieldChange('enum', values); }} className={styles.fieldInput} placeholder="value1,value2" />
            </div>
          </div>
          {(editedField.type === 'number' || editedField.type === 'integer') && (
            <div className={styles.inlineEditorFormRow}>
              <div className={styles.inlineEditorField}>
                <label>最小值</label>
                <input type="number" value={editedField.minimum ?? ''} onChange={(e) => handleFieldChange('minimum', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
              <div className={styles.inlineEditorField}>
                <label>最大值</label>
                <input type="number" value={editedField.maximum ?? ''} onChange={(e) => handleFieldChange('maximum', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
            </div>
          )}
          {editedField.type === 'string' && (
            <div className={styles.inlineEditorFormRow}>
              <div className={styles.inlineEditorField}>
                <label>最小长度</label>
                <input type="number" value={editedField.minLength ?? ''} onChange={(e) => handleFieldChange('minLength', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
              <div className={styles.inlineEditorField}>
                <label>最大长度</label>
                <input type="number" value={editedField.maxLength ?? ''} onChange={(e) => handleFieldChange('maxLength', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
            </div>
          )}
          {editedField.type === 'array' && (
            <div className={styles.inlineEditorFormRow}>
              <div className={styles.inlineEditorField}>
                <label>最少项数</label>
                <input type="number" value={editedField.minItems ?? ''} onChange={(e) => handleFieldChange('minItems', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
              <div className={styles.inlineEditorField}>
                <label>最多项数</label>
                <input type="number" value={editedField.maxItems ?? ''} onChange={(e) => handleFieldChange('maxItems', e.target.value ? Number(e.target.value) : undefined)} className={styles.fieldInput} />
              </div>
            </div>
          )}
        </div>
      )}
      {activeSection === 'advanced' && (
        <div className={styles.inlineEditorSection}>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <div className={styles.fieldCheckboxGroup}>
                <label><input type="checkbox" checked={editedField['x-sensitive'] || false} onChange={(e) => handleFieldChange('x-sensitive', e.target.checked || undefined)} /><span>敏感字段（密码等）</span></label>
                <label><input type="checkbox" checked={editedField.readOnly || false} onChange={(e) => handleFieldChange('readOnly', e.target.checked || undefined)} /><span>只读字段</span></label>
                <label><input type="checkbox" checked={editedField.deprecated || false} onChange={(e) => handleFieldChange('deprecated', e.target.checked || undefined)} /><span>已弃用</span></label>
              </div>
            </div>
          </div>
          <div className={styles.inlineEditorFormRow}>
            <div className={styles.inlineEditorFieldFull}>
              <label>格式 (format)</label>
              <select value={editedField.format || ''} onChange={(e) => handleFieldChange('format', e.target.value || undefined)} className={styles.fieldSelect}>
                <option value="">无</option>
                <option value="date">日期 (date)</option>
                <option value="date-time">日期时间 (date-time)</option>
                <option value="email">邮箱 (email)</option>
                <option value="uri">URI</option>
                <option value="hostname">主机名 (hostname)</option>
                <option value="ipv4">IPv4</option>
                <option value="ipv6">IPv6</option>
                <option value="json">JSON</option>
                <option value="password">密码 (password)</option>
              </select>
            </div>
          </div>
        </div>
      )}
      {activeSection === 'nested' && showNestedTab && (
        <div className={styles.inlineEditorSection}>
          <NestedFieldsEditor property={editedField} valueType={selectedType} onChange={(newProperty) => { setEditedField(newProperty); onChange(newProperty); }} depth={depth + 1} groups={groups} />
        </div>
      )}
    </div>
  );
};

// ============ 可排序的分组项组件 ============
interface SortableGroupItemProps {
  group: UIGroup;
  onUpdate: (group: UIGroup) => void;
  onDelete: (groupId: string) => void;
}

const SortableGroupItem: React.FC<SortableGroupItemProps> = ({
  group, onUpdate, onDelete
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: group.id });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    backgroundColor: isDragging ? '#f0f9ff' : undefined,
  };

  // 直接编辑，即时保存
  const handleLabelChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onUpdate({ ...group, label: e.target.value });
  };

  const handleLevelChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    onUpdate({ ...group, level: e.target.value as 'basic' | 'advanced' });
  };

  const handleLayoutChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    onUpdate({ ...group, layout: e.target.value as 'tabs' | 'accordion' | 'sections' });
  };

  const isDefaultGroup = group.id === 'basic' || group.id === 'advanced';

  return (
    <div ref={setNodeRef} style={style} className={styles.groupItem}>
      <div className={styles.groupItemContent}>
        <span className={styles.dragHandle} {...attributes} {...listeners}>⋮⋮</span>
        <span className={styles.groupOrder}>#{group.order}</span>
        <span className={styles.groupId}>{group.id}</span>
        <input 
          type="text" 
          value={group.label} 
          onChange={handleLabelChange} 
          className={styles.groupLabelInput} 
          placeholder="分组名称"
        />
        <select 
          value={group.level} 
          onChange={handleLevelChange} 
          className={styles.groupSelectSmall}
        >
          <option value="basic">基础</option>
          <option value="advanced">高级</option>
        </select>
        <select 
          value={group.layout} 
          onChange={handleLayoutChange} 
          className={styles.groupSelectSmall}
        >
          <option value="sections">分区</option>
          <option value="tabs">标签页</option>
          <option value="accordion">折叠</option>
        </select>
        {!isDefaultGroup && (
          <button type="button" onClick={() => onDelete(group.id)} className={styles.groupDeleteButton} title="删除分组">✕</button>
        )}
      </div>
    </div>
  );
};

// ============ 分组管理组件 ============
interface GroupManagerProps {
  groups: UIGroup[];
  onChange: (groups: UIGroup[]) => void;
}

const GroupManager: React.FC<GroupManagerProps> = ({ groups, onChange }) => {
  const [showAddGroup, setShowAddGroup] = useState(false);
  const [newGroupId, setNewGroupId] = useState('');
  const [newGroupLabel, setNewGroupLabel] = useState('');

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  // 按 order 排序的分组列表
  const sortedGroups = useMemo(() => {
    return [...groups].sort((a, b) => a.order - b.order);
  }, [groups]);

  const handleDragEnd = useCallback((event: DragEndEvent) => {
    const { active, over } = event;
    if (over && active.id !== over.id) {
      const oldIndex = sortedGroups.findIndex(g => g.id === active.id);
      const newIndex = sortedGroups.findIndex(g => g.id === over.id);
      if (oldIndex !== -1 && newIndex !== -1) {
        const newOrder = arrayMove(sortedGroups, oldIndex, newIndex);
        // 更新每个分组的 order 值
        const updatedGroups = newOrder.map((g, index) => ({
          ...g,
          order: index + 1,
        }));
        onChange(updatedGroups);
      }
    }
  }, [sortedGroups, onChange]);

  const handleAddGroup = () => {
    if (!newGroupId || !newGroupLabel) return;
    if (groups.find(g => g.id === newGroupId)) { alert('分组ID已存在'); return; }
    // 新分组的 order 为当前最大 order + 1
    const maxOrder = Math.max(...groups.map(g => g.order), 0);
    const group: UIGroup = {
      id: newGroupId,
      label: newGroupLabel,
      level: 'advanced',
      layout: 'sections',
      order: maxOrder + 1
    };
    onChange([...groups, group]);
    setNewGroupId('');
    setNewGroupLabel('');
    setShowAddGroup(false);
  };

  const handleUpdateGroup = (updatedGroup: UIGroup) => {
    onChange(groups.map(g => g.id === updatedGroup.id ? updatedGroup : g));
  };

  const handleDeleteGroup = (groupId: string) => {
    if (groupId === 'basic' || groupId === 'advanced') { alert('默认分组不能删除'); return; }
    onChange(groups.filter(g => g.id !== groupId));
  };

  return (
    <div className={styles.groupManager}>
      <div className={styles.groupManagerHeader}>
        <h4>分组管理</h4>
        <span className={styles.dragHint}>💡 拖拽调整顺序，直接编辑即时保存</span>
        <button type="button" onClick={() => setShowAddGroup(true)} className={styles.addGroupButton}>+ 添加</button>
      </div>
      
      {/* 表头 */}
      <div className={styles.groupTableHeader}>
        <span className={styles.groupColDrag}></span>
        <span className={styles.groupColOrder}>#</span>
        <span className={styles.groupColId}>ID</span>
        <span className={styles.groupColLabel}>名称</span>
        <span className={styles.groupColLevel}>级别</span>
        <span className={styles.groupColLayout}>布局</span>
        <span className={styles.groupColAction}></span>
      </div>
      
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={sortedGroups.map(g => g.id)} strategy={verticalListSortingStrategy}>
          <div className={styles.groupList}>
            {sortedGroups.map(group => (
              <SortableGroupItem
                key={group.id}
                group={group}
                onUpdate={handleUpdateGroup}
                onDelete={handleDeleteGroup}
              />
            ))}
          </div>
        </SortableContext>
      </DndContext>
      
      {showAddGroup && (
        <div className={styles.addGroupFormCompact}>
          <input 
            type="text" 
            value={newGroupId} 
            onChange={(e) => setNewGroupId(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))} 
            className={styles.groupIdInput} 
            placeholder="ID" 
            autoFocus
          />
          <input 
            type="text" 
            value={newGroupLabel} 
            onChange={(e) => setNewGroupLabel(e.target.value)} 
            className={styles.groupLabelInput} 
            placeholder="名称"
          />
          <button type="button" onClick={handleAddGroup} className={styles.groupAddBtn} disabled={!newGroupId || !newGroupLabel}>✓</button>
          <button type="button" onClick={() => { setShowAddGroup(false); setNewGroupId(''); setNewGroupLabel(''); }} className={styles.groupCancelBtn}>✕</button>
        </div>
      )}
    </div>
  );
};

// ============ 布局视图：拖拽行分组 ============
interface LayoutRow {
  fields: string[];
}

interface LayoutViewProps {
  fieldNames: string[];
  properties: Record<string, any>;
  uiFields: Record<string, any>;
  groups: UIGroup[];
  onFieldsChange: (updatedFields: Record<string, { order: number; colSpan: number }>) => void;
}

const buildLayoutRows = (fieldNames: string[], uiFields: Record<string, any>): LayoutRow[] => {
  const rows: LayoutRow[] = [];
  let currentRow: string[] = [];
  let currentSpan = 0;

  for (const name of fieldNames) {
    const span = uiFields[name]?.colSpan || 24;
    if (currentSpan + span > 24 && currentRow.length > 0) {
      rows.push({ fields: currentRow });
      currentRow = [];
      currentSpan = 0;
    }
    currentRow.push(name);
    currentSpan += span;
    if (currentSpan >= 24) {
      rows.push({ fields: currentRow });
      currentRow = [];
      currentSpan = 0;
    }
  }
  if (currentRow.length > 0) {
    rows.push({ fields: currentRow });
  }
  return rows;
};

// ============ 布局拖拽卡片 (useDraggable) ============
const LayoutDraggableCard: React.FC<{ fieldName: string; property: any; uiConfig: any; fieldsInRow: number }> = ({ fieldName, property, uiConfig, fieldsInRow }) => {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: `layout-card-${fieldName}` });
  return (
    <div
      ref={setNodeRef}
      className={styles.layoutCard}
      style={{ opacity: isDragging ? 0.3 : 1, flex: `1 1 ${100 / fieldsInRow}%` }}
      {...attributes}
      {...listeners}
    >
      <div className={styles.layoutCardHeader}>
        <span className={styles.layoutCardDrag}>⋮⋮</span>
        <span className={styles.layoutCardName}>{fieldName}</span>
      </div>
      <div className={styles.layoutCardMeta}>
        {uiConfig?.label && <span className={styles.layoutCardLabel}>{uiConfig.label}</span>}
        <span className={styles.layoutCardType}>{property?.type || 'string'}</span>
        <span className={styles.layoutCardSpan}>span {uiConfig?.colSpan || 24}</span>
      </div>
    </div>
  );
};

// ============ 布局可放置行 (useDroppable) ============
const LayoutDroppableRow: React.FC<{ id: string; rowIdx: number; children: React.ReactNode; spanLabel: string }> = ({ id, rowIdx, children, spanLabel }) => {
  const { setNodeRef, isOver } = useDroppable({ id });
  return (
    <div ref={setNodeRef} className={`${styles.layoutRow} ${isOver ? styles.layoutRowDragOver : ''}`}>
      <div className={styles.layoutRowLabel}>
        行 {rowIdx + 1} · {spanLabel}
      </div>
      <div className={styles.layoutRowContent}>
        {children}
      </div>
    </div>
  );
};

// ============ 布局行分隔区 (useDroppable) ============
const LayoutRowSeparator: React.FC<{ id: string; isDragging?: boolean }> = ({ id, isDragging }) => {
  const { setNodeRef, isOver } = useDroppable({ id });
  return (
    <div
      ref={setNodeRef}
      className={`${styles.nestedRowSeparator} ${isOver ? styles.nestedRowSeparatorActive : ''} ${isDragging && !isOver ? styles.nestedRowSeparatorDragging : ''}`}
    >
      {isOver
        ? <span className={styles.nestedRowSeparatorLabel}>释放以创建新行</span>
        : isDragging
          ? <span className={styles.nestedRowSeparatorLabel}>拖到此处新建一行</span>
          : null}
    </div>
  );
};

const LayoutView: React.FC<LayoutViewProps> = ({ fieldNames, properties, uiFields, groups, onFieldsChange }) => {
  const rows = useMemo(() => buildLayoutRows(fieldNames, uiFields), [fieldNames, uiFields]);
  const [activeDragId, setActiveDragId] = useState<string | null>(null);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const layoutCollisionDetection = useMemo(
    () => createSeparatorFirstCollision('layout-sep-', 'layout-row-'),
    []
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    // Strip prefix to get field name
    setActiveDragId(String(event.active.id).replace('layout-card-', ''));
  }, []);

  const handleDragEnd = useCallback((event: DragEndEvent) => {
    setActiveDragId(null);
    const { active, over } = event;
    if (!over) return;

    const activeField = String(active.id).replace('layout-card-', '');
    const overId = String(over.id);

    const mutableRows = rows.map(r => ({ fields: [...r.fields] }));

    // Find source row
    let sourceRowIdx = -1;
    for (let i = 0; i < mutableRows.length; i++) {
      if (mutableRows[i].fields.includes(activeField)) { sourceRowIdx = i; break; }
    }
    if (sourceRowIdx === -1) return;

    // Remove from source
    mutableRows[sourceRowIdx].fields = mutableRows[sourceRowIdx].fields.filter(f => f !== activeField);

    if (overId.startsWith('layout-sep-')) {
      // Dropped on separator → create new row
      const sepIdx = parseInt(overId.replace('layout-sep-', ''), 10);
      mutableRows.splice(sepIdx, 0, { fields: [activeField] });
    } else if (overId.startsWith('layout-row-')) {
      // Dropped on an existing row → merge into that row
      const rowIdx = parseInt(overId.replace('layout-row-', ''), 10);
      if (rowIdx >= 0 && rowIdx < mutableRows.length) {
        mutableRows[rowIdx].fields.push(activeField);
      }
    } else {
      // Shouldn't happen; restore
      return;
    }

    // Remove empty rows
    const finalRows = mutableRows.filter(r => r.fields.length > 0);

    // Recalculate order and colSpan
    const updatedFields: Record<string, { order: number; colSpan: number }> = {};
    let orderCounter = 1;
    for (const row of finalRows) {
      const colSpan = Math.floor(24 / row.fields.length);
      for (const name of row.fields) {
        updatedFields[name] = { order: orderCounter++, colSpan };
      }
    }

    onFieldsChange(updatedFields);
  }, [rows, onFieldsChange]);

  // Find the active field's info for overlay
  const activeFieldName = activeDragId;
  const activeProperty = activeFieldName ? properties[activeFieldName] : null;
  const activeUiConfig = activeFieldName ? (uiFields[activeFieldName] || {}) : {};

  return (
    <div className={styles.layoutView}>
      <div className={styles.layoutHint}>
        拖拽字段卡片到同一行可并排显示，字段宽度将自动等分。拖到行间分隔区可创建新行。
      </div>
      <DndContext sensors={sensors} collisionDetection={layoutCollisionDetection} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
        <div className={styles.layoutRows}>
          {/* Top separator */}
          <LayoutRowSeparator id="layout-sep-0" isDragging={!!activeDragId} />
          {rows.map((row, rowIdx) => (
            <React.Fragment key={rowIdx}>
              <LayoutDroppableRow
                id={`layout-row-${rowIdx}`}
                rowIdx={rowIdx}
                spanLabel={`${row.fields.length} 个字段 · span ${row.fields.map(f => uiFields[f]?.colSpan || 24).join('+')}`}
              >
                {row.fields.map((fieldName) => (
                  <LayoutDraggableCard
                    key={fieldName}
                    fieldName={fieldName}
                    property={properties[fieldName]}
                    uiConfig={uiFields[fieldName] || {}}
                    fieldsInRow={row.fields.length}
                  />
                ))}
              </LayoutDroppableRow>
              <LayoutRowSeparator id={`layout-sep-${rowIdx + 1}`} isDragging={!!activeDragId} />
            </React.Fragment>
          ))}
        </div>
        <DragOverlay>
          {activeFieldName ? (
            <div className={styles.layoutCard} style={{ opacity: 0.9, boxShadow: '0 4px 12px rgba(0,0,0,0.15)' }}>
              <div className={styles.layoutCardHeader}>
                <span className={styles.layoutCardDrag}>⋮⋮</span>
                <span className={styles.layoutCardName}>{activeFieldName}</span>
              </div>
              <div className={styles.layoutCardMeta}>
                {activeUiConfig.label && <span className={styles.layoutCardLabel}>{activeUiConfig.label}</span>}
                <span className={styles.layoutCardType}>{activeProperty?.type || 'string'}</span>
                <span className={styles.layoutCardSpan}>span {activeUiConfig.colSpan || 24}</span>
              </div>
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
    </div>
  );
};

// ============ 可展开的表格行组件 ============
interface ExpandableRowProps {
  fieldName: string;
  property: any;
  uiConfig: any;
  isRequired: boolean;
  order: number;
  groups: UIGroup[];
  allFields?: { name: string; group: string; label?: string }[];
  isExpanded: boolean;
  onToggleExpand: () => void;
  onDelete: () => void;
  onToggleRequired: () => void;
  onChange: (property: any, uiConfig: any) => void;
  getTypeDisplay: (prop: any) => string;
}

const ExpandableRow: React.FC<ExpandableRowProps> = ({
  fieldName, property, uiConfig, isRequired, order, groups, allFields, isExpanded, onToggleExpand, onDelete, onToggleRequired, onChange, getTypeDisplay,
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: fieldName });
  const style = { transform: CSS.Transform.toString(transform), transition, opacity: isDragging ? 0.5 : 1, backgroundColor: isDragging ? '#f0f9ff' : undefined };
  const currentGroup = groups.find(g => g.id === (uiConfig.group || 'advanced'));

  return (
    <>
      <tr ref={setNodeRef} style={style} className={isExpanded ? styles.expandedRow : ''}>
        <td className={styles.dragHandleCell}><span className={styles.dragHandle} {...attributes} {...listeners}>⋮⋮</span></td>
        <td className={styles.orderCell}><span className={styles.orderBadge}>{order}</span></td>
        <td className={styles.fieldNameCell} onClick={onToggleExpand} style={{ cursor: 'pointer' }}>
          <span className={styles.expandIcon}>{isExpanded ? '▼' : '▶'}</span>
          <span className={styles.fieldName}>{fieldName}</span>
          {uiConfig.label && <span className={styles.fieldAlias}>({uiConfig.label})</span>}
        </td>
        <td>{getTypeDisplay(property)}</td>
        <td><span className={`${styles.groupBadge} ${currentGroup?.level === 'basic' ? styles.basicLevel : styles.advancedLevel}`}>{currentGroup?.label || '高级配置'}</span></td>
        <td><button className={`${styles.requiredBadge} ${isRequired ? styles.required : styles.optional}`} onClick={onToggleRequired} title="点击切换必填状态">{isRequired ? '✓' : '✗'}</button></td>
        <td className={styles.defaultValueCell}>{property.default !== undefined ? (typeof property.default === 'object' ? JSON.stringify(property.default) : String(property.default)) : '-'}</td>
        <td className={styles.descriptionCell}>{property.description || '-'}</td>
        <td className={styles.actionsCell}><button onClick={onDelete} className={styles.deleteButton}>删除</button></td>
      </tr>
      {isExpanded && (
        <tr className={styles.inlineEditorRow}>
          <td colSpan={9} className={styles.inlineEditorCell}>
            <InlineFieldEditor fieldName={fieldName} property={property} uiConfig={uiConfig} groups={groups} allFields={allFields} onChange={onChange} />
          </td>
        </tr>
      )}
    </>
  );
};

// ============ 变更项类型 ============
interface SchemaChange {
  fieldName: string;
  changeType: 'added' | 'removed' | 'modified';
  oldValue?: any;
  newValue?: any;
  path: string;
  label?: string;
}

// ============ 计算 Schema 变更 ============
const computeSchemaChanges = (originalSchema: OpenAPISchema, editedSchema: OpenAPISchema): SchemaChange[] => {
  const changes: SchemaChange[] = [];
  
  const originalProps = originalSchema.components?.schemas?.ModuleInput?.properties || {};
  const editedProps = editedSchema.components?.schemas?.ModuleInput?.properties || {};
  const originalRequired = originalSchema.components?.schemas?.ModuleInput?.required || [];
  const editedRequired = editedSchema.components?.schemas?.ModuleInput?.required || [];
  const originalUiFields = (originalSchema as any)['x-iac-platform']?.ui?.fields || {};
  const editedUiFields = (editedSchema as any)['x-iac-platform']?.ui?.fields || {};
  
  // 检查新增的字段
  Object.keys(editedProps).forEach(fieldName => {
    if (!originalProps[fieldName]) {
      changes.push({
        fieldName,
        changeType: 'added',
        newValue: editedProps[fieldName],
        path: `components.schemas.ModuleInput.properties.${fieldName}`,
        label: editedUiFields[fieldName]?.label || fieldName,
      });
    }
  });
  
  // 检查删除的字段
  Object.keys(originalProps).forEach(fieldName => {
    if (!editedProps[fieldName]) {
      changes.push({
        fieldName,
        changeType: 'removed',
        oldValue: originalProps[fieldName],
        path: `components.schemas.ModuleInput.properties.${fieldName}`,
        label: originalUiFields[fieldName]?.label || fieldName,
      });
    }
  });
  
  // 检查修改的字段
  Object.keys(editedProps).forEach(fieldName => {
    if (originalProps[fieldName]) {
      const originalStr = JSON.stringify(originalProps[fieldName]);
      const editedStr = JSON.stringify(editedProps[fieldName]);
      const originalUiStr = JSON.stringify(originalUiFields[fieldName] || {});
      const editedUiStr = JSON.stringify(editedUiFields[fieldName] || {});
      const originalReq = originalRequired.includes(fieldName);
      const editedReq = editedRequired.includes(fieldName);
      
      if (originalStr !== editedStr || originalUiStr !== editedUiStr || originalReq !== editedReq) {
        changes.push({
          fieldName,
          changeType: 'modified',
          oldValue: { property: originalProps[fieldName], ui: originalUiFields[fieldName], required: originalReq },
          newValue: { property: editedProps[fieldName], ui: editedUiFields[fieldName], required: editedReq },
          path: `components.schemas.ModuleInput.properties.${fieldName}`,
          label: editedUiFields[fieldName]?.label || originalUiFields[fieldName]?.label || fieldName,
        });
      }
    }
  });
  
  // 检查分组变更
  const originalGroups = (originalSchema as any)['x-iac-platform']?.ui?.groups || [];
  const editedGroups = (editedSchema as any)['x-iac-platform']?.ui?.groups || [];
  if (JSON.stringify(originalGroups) !== JSON.stringify(editedGroups)) {
    changes.push({
      fieldName: '_groups',
      changeType: 'modified',
      oldValue: originalGroups,
      newValue: editedGroups,
      path: 'x-iac-platform.ui.groups',
      label: '分组配置',
    });
  }
  
  return changes;
};

// ============ 主编辑器组件 ============
export interface OpenAPISchemaEditorProps {
  schema: OpenAPISchema;
  onSave: (schema: OpenAPISchema) => void;
  onCancel: () => void;
  title?: string;
}

export const OpenAPISchemaEditor: React.FC<OpenAPISchemaEditorProps> = ({ schema, onSave, onCancel, title = 'OpenAPI Schema 编辑器' }) => {
  const [editedSchema, setEditedSchema] = useState<OpenAPISchema>(JSON.parse(JSON.stringify(schema)));
  const [originalSchema] = useState<OpenAPISchema>(JSON.parse(JSON.stringify(schema))); // 保存原始 Schema 用于对比
  const [expandedField, setExpandedField] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [viewMode, setViewMode] = useState<'table' | 'json' | 'groups' | 'layout'>('table');
  const [activeTab, setActiveTab] = useState<'variables' | 'outputs'>('variables');
  const [importMode, setImportMode] = useState<'merge' | 'replace'>('merge');
  const [importMessage, setImportMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [importing, setImporting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const tfFileInputRef = useRef<HTMLInputElement>(null);
  
  // 预览模式状态
  const [isPreviewMode, setIsPreviewMode] = useState(false);
  const [previewViewMode, setPreviewViewMode] = useState<'form' | 'json'>('form');
  const [hasReviewed, setHasReviewed] = useState(false); // 是否已经预览过
  
  // 删除确认弹窗状态
  const [deleteConfirm, setDeleteConfirm] = useState<{
    isOpen: boolean;
    type: 'field' | 'output' | 'group' | 'nested';
    name: string;
    label?: string;
    onConfirm: () => void;
  }>({ isOpen: false, type: 'field', name: '', onConfirm: () => {} });

  const sensors = useSensors(useSensor(PointerSensor), useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }));

  const properties = editedSchema.components?.schemas?.ModuleInput?.properties || {};
  const required = editedSchema.components?.schemas?.ModuleInput?.required || [];
  const uiFields = (editedSchema as any)['x-iac-platform']?.ui?.fields || {};
  
  // 获取分组配置，确保格式正确
  const groups: UIGroup[] = useMemo(() => {
    const schemaGroups = (editedSchema as any)['x-iac-platform']?.ui?.groups || [];
    if (!schemaGroups || !Array.isArray(schemaGroups) || schemaGroups.length === 0) {
      return DEFAULT_GROUPS;
    }
    
    // 转换 Schema 中的分组格式为 UIGroup 格式
    // Schema 中可能使用 title 而不是 label
    const validGroups = schemaGroups
      .filter((g: any) => g && typeof g === 'object' && g.id)
      .map((g: any) => ({
        id: String(g.id),
        label: String(g.label || g.title || g.id),
        level: (g.level === 'basic' ? 'basic' : 'advanced') as 'basic' | 'advanced',
        layout: (g.layout === 'tabs' ? 'tabs' : g.layout === 'accordion' ? 'accordion' : 'sections') as 'tabs' | 'accordion' | 'sections',
        order: typeof g.order === 'number' ? g.order : 100,
      }));
    
    return validGroups.length > 0 ? validGroups : DEFAULT_GROUPS;
  }, [editedSchema]);

  // 更新分组配置
  const handleGroupsChange = (newGroups: UIGroup[]) => {
    const newSchema = JSON.parse(JSON.stringify(editedSchema));
    if (!newSchema['x-iac-platform']) newSchema['x-iac-platform'] = { ui: { fields: {}, groups: [] } };
    if (!newSchema['x-iac-platform'].ui) newSchema['x-iac-platform'].ui = { fields: {}, groups: [] };
    newSchema['x-iac-platform'].ui.groups = newGroups;
    setEditedSchema(newSchema);
  };

  // 布局视图回调：批量更新字段的 order 和 colSpan
  const handleLayoutFieldsChange = (updatedFields: Record<string, { order: number; colSpan: number }>) => {
    const newSchema = JSON.parse(JSON.stringify(editedSchema));
    if (!newSchema['x-iac-platform']) newSchema['x-iac-platform'] = { ui: { fields: {}, groups: [] } };
    if (!newSchema['x-iac-platform'].ui) newSchema['x-iac-platform'].ui = { fields: {}, groups: [] };
    if (!newSchema['x-iac-platform'].ui.fields) newSchema['x-iac-platform'].ui.fields = {};
    Object.entries(updatedFields).forEach(([name, { order, colSpan }]) => {
      if (!newSchema['x-iac-platform'].ui.fields[name]) {
        newSchema['x-iac-platform'].ui.fields[name] = {};
      }
      newSchema['x-iac-platform'].ui.fields[name].order = order;
      newSchema['x-iac-platform'].ui.fields[name].colSpan = colSpan;
    });
    setEditedSchema(newSchema);
  };

  // 按 order 排序（不按分组排序，避免更改分组时导致重新排序）
  const sortedFieldNames = useMemo(() => {
    const fieldNames = Object.keys(properties);
    
    return fieldNames.sort((a, b) => {
      // 只按 order 排序
      const orderA = uiFields[a]?.order ?? 999;
      const orderB = uiFields[b]?.order ?? 999;
      return orderA - orderB;
    });
  }, [properties, uiFields]);

  const filteredFields = sortedFieldNames.filter(name => name.toLowerCase().includes(searchTerm.toLowerCase()));

  // 统计
  const groupStats = useMemo(() => {
    const stats: Record<string, number> = {};
    groups.forEach(g => { stats[g.id] = 0; });
    sortedFieldNames.forEach(name => {
      const groupId = uiFields[name]?.group || 'advanced';
      if (stats[groupId] !== undefined) stats[groupId]++;
      else stats['advanced']++;
    });
    return stats;
  }, [sortedFieldNames, uiFields, groups]);

  const outputs = (editedSchema as any)['x-iac-platform']?.outputs?.items || [];

  // 请求删除输出（显示确认弹窗）
  const requestDeleteOutput = (outputName: string) => {
    setDeleteConfirm({
      isOpen: true,
      type: 'output',
      name: outputName,
      label: outputName,
      onConfirm: () => {
        const newSchema = JSON.parse(JSON.stringify(editedSchema));
        if (newSchema['x-iac-platform']?.outputs?.items) {
          newSchema['x-iac-platform'].outputs.items = newSchema['x-iac-platform'].outputs.items.filter((o: any) => o.name !== outputName);
        }
        setEditedSchema(newSchema);
        setDeleteConfirm({ isOpen: false, type: 'output', name: '', onConfirm: () => {} });
      }
    });
  };

  const updateFieldsOrder = useCallback((orderedFieldNames: string[]) => {
    const newSchema = JSON.parse(JSON.stringify(editedSchema));
    if (!newSchema['x-iac-platform']) newSchema['x-iac-platform'] = { ui: { fields: {}, groups: [] } };
    if (!newSchema['x-iac-platform'].ui) newSchema['x-iac-platform'].ui = { fields: {}, groups: [] };
    if (!newSchema['x-iac-platform'].ui.fields) newSchema['x-iac-platform'].ui.fields = {};
    orderedFieldNames.forEach((fieldName, index) => {
      if (!newSchema['x-iac-platform'].ui.fields[fieldName]) newSchema['x-iac-platform'].ui.fields[fieldName] = {};
      newSchema['x-iac-platform'].ui.fields[fieldName].order = index + 1;
    });
    setEditedSchema(newSchema);
  }, [editedSchema]);

  const handleDragEnd = useCallback((event: DragEndEvent) => {
    const { active, over } = event;
    if (over && active.id !== over.id) {
      const oldIndex = sortedFieldNames.indexOf(active.id as string);
      const newIndex = sortedFieldNames.indexOf(over.id as string);
      if (oldIndex !== -1 && newIndex !== -1) {
        const newOrder = arrayMove(sortedFieldNames, oldIndex, newIndex);
        updateFieldsOrder(newOrder);
      }
    }
  }, [sortedFieldNames, updateFieldsOrder]);

  const handleFieldChange = (fieldName: string, property: any, uiConfig: any) => {
    const newSchema = JSON.parse(JSON.stringify(editedSchema));
    newSchema.components.schemas.ModuleInput.properties[fieldName] = property;
    if (!newSchema['x-iac-platform']) newSchema['x-iac-platform'] = { ui: { fields: {} } };
    if (!newSchema['x-iac-platform'].ui) newSchema['x-iac-platform'].ui = { fields: {} };
    if (!newSchema['x-iac-platform'].ui.fields) newSchema['x-iac-platform'].ui.fields = {};
    newSchema['x-iac-platform'].ui.fields[fieldName] = uiConfig;
    setEditedSchema(newSchema);
  };

  // 请求删除字段（显示确认弹窗）
  const requestDeleteField = (fieldName: string) => {
    const fieldLabel = uiFields[fieldName]?.label || fieldName;
    setDeleteConfirm({
      isOpen: true,
      type: 'field',
      name: fieldName,
      label: fieldLabel,
      onConfirm: () => {
        const newSchema = JSON.parse(JSON.stringify(editedSchema));
        delete newSchema.components.schemas.ModuleInput.properties[fieldName];
        const reqIndex = newSchema.components.schemas.ModuleInput.required?.indexOf(fieldName);
        if (reqIndex > -1) newSchema.components.schemas.ModuleInput.required.splice(reqIndex, 1);
        if (newSchema['x-iac-platform']?.ui?.fields?.[fieldName]) delete newSchema['x-iac-platform'].ui.fields[fieldName];
        setEditedSchema(newSchema);
        if (expandedField === fieldName) setExpandedField(null);
        setDeleteConfirm({ isOpen: false, type: 'field', name: '', onConfirm: () => {} });
      }
    });
  };

  const toggleRequired = (fieldName: string) => {
    const newSchema = JSON.parse(JSON.stringify(editedSchema));
    if (!newSchema.components.schemas.ModuleInput.required) newSchema.components.schemas.ModuleInput.required = [];
    const reqIndex = newSchema.components.schemas.ModuleInput.required.indexOf(fieldName);
    if (reqIndex > -1) newSchema.components.schemas.ModuleInput.required.splice(reqIndex, 1);
    else newSchema.components.schemas.ModuleInput.required.push(fieldName);
    setEditedSchema(newSchema);
  };

  const getTypeDisplay = (prop: any): string => {
    if (prop.type === 'array') return `array[${prop.items?.type || 'any'}]`;
    if (prop.type === 'object' && prop.additionalProperties) return 'map';
    return prop.type || 'string';
  };

  // TF 文件导入
  const handleTfFileImport = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files;
    if (!files || files.length === 0) return;
    setImporting(true);
    try {
      let variablesTf = '', outputsTf = '';
      for (let i = 0; i < files.length; i++) {
        const file = files[i];
        const content = await file.text();
        const fileName = file.name.toLowerCase();
        if (fileName.includes('variable') || fileName === 'variables.tf') variablesTf += content + '\n';
        else if (fileName.includes('output') || fileName === 'outputs.tf') outputsTf += content + '\n';
        else variablesTf += content + '\n';
      }
      if (!variablesTf.trim() && !outputsTf.trim()) { setImportMessage({ type: 'error', text: '未找到有效的 TF 文件内容' }); return; }
      const parseResponse = await api.post('/modules/parse-tf', { variables_tf: variablesTf, outputs_tf: outputsTf });
      if (parseResponse.data?.schema) {
        const importedSchema = parseResponse.data.schema;
        if (importMode === 'replace') {
          setEditedSchema(importedSchema);
        } else {
          const newSchema = JSON.parse(JSON.stringify(editedSchema));
          const importedProps = importedSchema.components?.schemas?.ModuleInput?.properties || {};
          Object.keys(importedProps).forEach(key => {
            if (!newSchema.components.schemas.ModuleInput.properties[key]) {
              newSchema.components.schemas.ModuleInput.properties[key] = importedProps[key];
            }
          });
          setEditedSchema(newSchema);
        }
        setImportMessage({ type: 'success', text: `成功导入 ${Object.keys(parseResponse.data.schema.components?.schemas?.ModuleInput?.properties || {}).length} 个字段` });
      }
    } catch (error: any) {
      setImportMessage({ type: 'error', text: error.response?.data?.error || '导入失败' });
    } finally {
      setImporting(false);
      if (tfFileInputRef.current) tfFileInputRef.current.value = '';
    }
  };

  // 添加新字段
  const handleAddField = () => {
    const fieldName = prompt('请输入新字段名称（英文小写，下划线分隔）：');
    if (!fieldName) return;
    const normalizedName = fieldName.toLowerCase().replace(/[^a-z0-9_]/g, '_');
    if (properties[normalizedName]) { alert(`字段 "${normalizedName}" 已存在`); return; }
    const newSchema = JSON.parse(JSON.stringify(editedSchema));
    newSchema.components.schemas.ModuleInput.properties[normalizedName] = { type: 'string', description: '' };
    if (!newSchema['x-iac-platform']) newSchema['x-iac-platform'] = { ui: { fields: {}, groups: [] } };
    if (!newSchema['x-iac-platform'].ui) newSchema['x-iac-platform'].ui = { fields: {}, groups: [] };
    if (!newSchema['x-iac-platform'].ui.fields) newSchema['x-iac-platform'].ui.fields = {};
    newSchema['x-iac-platform'].ui.fields[normalizedName] = { order: sortedFieldNames.length + 1, group: 'advanced' };
    setEditedSchema(newSchema);
    setExpandedField(normalizedName);
  };

  return (
    <div className={styles.schemaEditor}>
      <div className={styles.schemaEditorHeader}>
        <div className={styles.schemaEditorTitle}>
          <h2>{title}</h2>
          <div className={styles.schemaStats}>
            共 <strong>{sortedFieldNames.length}</strong> 个字段
            {groups.map(g => (
              <span key={g.id}> · <span className={g.level === 'basic' ? styles.basicCount : styles.advancedCount}>{g.label} {groupStats[g.id] || 0}</span></span>
            ))}
          </div>
        </div>
        <div className={styles.schemaEditorActions}>
          <div className={styles.importSection}>
            <input type="file" ref={tfFileInputRef} onChange={handleTfFileImport} accept=".tf,.hcl,text/plain" multiple style={{ display: 'none' }} />
            <button type="button" onClick={() => tfFileInputRef.current?.click()} className={styles.importTfButton} disabled={importing}>{importing ? '导入中...' : '导入 TF 文件'}</button>
          </div>
          <div className={styles.viewModeButtons}>
            <button type="button" className={`${styles.viewModeButton} ${viewMode === 'table' ? styles.active : ''}`} onClick={() => setViewMode('table')}>表格</button>
            <button type="button" className={`${styles.viewModeButton} ${viewMode === 'groups' ? styles.active : ''}`} onClick={() => setViewMode('groups')}>分组</button>
            <button type="button" className={`${styles.viewModeButton} ${viewMode === 'json' ? styles.active : ''}`} onClick={() => setViewMode('json')}>JSON</button>
            <button type="button" className={`${styles.viewModeButton} ${viewMode === 'layout' ? styles.active : ''}`} onClick={() => setViewMode('layout')}>布局</button>
          </div>
        </div>
      </div>

      {importMessage && (
        <div className={`${styles.importMessage} ${styles[importMessage.type]}`}>
          {importMessage.type === 'success' ? '✓' : '✗'} {importMessage.text}
          <button type="button" onClick={() => setImportMessage(null)} style={{ marginLeft: 'auto', background: 'none', border: 'none', cursor: 'pointer' }}>✕</button>
        </div>
      )}

      {/* 标签页 */}
      <div className={styles.tabsContainer}>
        <button type="button" className={`${styles.tabButton} ${activeTab === 'variables' ? styles.activeTab : ''}`} onClick={() => setActiveTab('variables')}>
          输入变量 <span className={styles.tabCount}>{sortedFieldNames.length}</span>
        </button>
        <button type="button" className={`${styles.tabButton} ${activeTab === 'outputs' ? styles.activeTab : ''}`} onClick={() => setActiveTab('outputs')}>
          输出 <span className={styles.tabCount}>{outputs.length}</span>
        </button>
      </div>

      {activeTab === 'variables' && (
        <>
          {viewMode === 'groups' ? (
            <GroupManager groups={groups} onChange={handleGroupsChange} />
          ) : viewMode === 'json' ? (
            <div className={styles.jsonEditorContainer} style={{ height: '500px' }}>
              <MonacoJsonEditor
                value={editedSchema}
                onChange={(val) => val && typeof val === 'object' && setEditedSchema(val as OpenAPISchema)}
                returnObject={true}
              />
            </div>
          ) : viewMode === 'layout' ? (
            <LayoutView
              fieldNames={sortedFieldNames}
              properties={properties}
              uiFields={uiFields}
              groups={groups}
              onFieldsChange={handleLayoutFieldsChange}
            />
          ) : (
            <>
              <div className={styles.searchBox}>
                <input type="text" placeholder="搜索字段..." value={searchTerm} onChange={(e) => setSearchTerm(e.target.value)} className={styles.searchInput} />
                <button type="button" onClick={handleAddField} className={styles.saveButton}>+ 添加字段</button>
                <span className={styles.dragHint}>💡 拖拽行可调整顺序</span>
              </div>
              <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
                <div className={styles.tableContainer}>
                  <table className={styles.schemaTable}>
                    <thead>
                      <tr>
                        <th className={styles.dragHandleHeader}></th>
                        <th className={styles.orderHeader}>序号</th>
                        <th>字段名</th>
                        <th>类型</th>
                        <th>分组</th>
                        <th>必填</th>
                        <th>默认值</th>
                        <th>描述</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <SortableContext items={filteredFields} strategy={verticalListSortingStrategy}>
                      <tbody>
                        {filteredFields.map((fieldName) => (
                          <ExpandableRow
                            key={fieldName}
                            fieldName={fieldName}
                            property={properties[fieldName]}
                            uiConfig={uiFields[fieldName] || {}}
                            isRequired={required.includes(fieldName)}
                            order={uiFields[fieldName]?.order || 999}
                            groups={groups}
                            allFields={sortedFieldNames.map(name => ({
                              name,
                              group: uiFields[name]?.group || 'advanced',
                              label: uiFields[name]?.label || name,
                            }))}
                            isExpanded={expandedField === fieldName}
                            onToggleExpand={() => setExpandedField(expandedField === fieldName ? null : fieldName)}
                            onDelete={() => requestDeleteField(fieldName)}
                            onToggleRequired={() => toggleRequired(fieldName)}
                            onChange={(prop, ui) => handleFieldChange(fieldName, prop, ui)}
                            getTypeDisplay={getTypeDisplay}
                          />
                        ))}
                      </tbody>
                    </SortableContext>
                  </table>
                </div>
              </DndContext>
            </>
          )}
        </>
      )}

      {activeTab === 'outputs' && (
        <div className={styles.tableContainer}>
          <table className={styles.schemaTable}>
            <thead>
              <tr>
                <th>输出名</th>
                <th>类型</th>
                <th>值表达式</th>
                <th>描述</th>
                <th>敏感</th>
                <th>分组</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {outputs.length === 0 ? (
                <tr><td colSpan={7} className={styles.emptyState}>暂无输出定义</td></tr>
              ) : (
                outputs.map((output: any) => (
                  <tr key={output.name}>
                    <td className={styles.fieldNameCell}><span className={styles.fieldName}>{output.name}</span></td>
                    <td><span className={styles.outputType}>{output.type || 'string'}</span></td>
                    <td className={styles.valueExpressionCell}>{output.value_expression && <span className={styles.valueExpression}>{output.value_expression}</span>}</td>
                    <td className={styles.descriptionCell}>{output.description || '-'}</td>
                    <td>{output.sensitive && <span className={styles.sensitiveTag}>敏感</span>}</td>
                    <td>{output.group && <span className={styles.groupTag}>{output.group}</span>}</td>
                    <td className={styles.actionsCell}><button onClick={() => requestDeleteOutput(output.name)} className={styles.deleteButton}>删除</button></td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* 预览区域 - 内嵌在页面中，不使用弹窗 */}
      {isPreviewMode && (
        <div className={styles.previewSection}>
          <div className={styles.previewHeader}>
            <h3>变更预览</h3>
            <div className={styles.previewTabs}>
              <button 
                type="button" 
                className={`${styles.previewTab} ${previewViewMode === 'form' ? styles.active : ''}`}
                onClick={() => setPreviewViewMode('form')}
              >
                表单预览
              </button>
              <button 
                type="button" 
                className={`${styles.previewTab} ${previewViewMode === 'json' ? styles.active : ''}`}
                onClick={() => setPreviewViewMode('json')}
              >
                JSON 预览
              </button>
            </div>
            <button 
              type="button" 
              onClick={() => setIsPreviewMode(false)} 
              className={styles.previewCloseButton}
            >
              返回编辑
            </button>
          </div>
          
          <div className={styles.previewContent}>
            {previewViewMode === 'form' ? (
              <SchemaChangesFormPreview 
                changes={computeSchemaChanges(originalSchema, editedSchema)} 
                groups={groups}
              />
            ) : (
              <SchemaChangesJsonPreview 
                originalSchema={originalSchema} 
                editedSchema={editedSchema}
                changes={computeSchemaChanges(originalSchema, editedSchema)}
              />
            )}
          </div>
        </div>
      )}

      {/* 删除确认弹窗 */}
      <ConfirmDialog
        isOpen={deleteConfirm.isOpen}
        title={`确认删除${deleteConfirm.type === 'field' ? '字段' : deleteConfirm.type === 'output' ? '输出' : deleteConfirm.type === 'group' ? '分组' : '嵌套字段'}`}
        message={`确定要删除 "${deleteConfirm.label || deleteConfirm.name}" 吗？此操作不可撤销。`}
        confirmText="删除"
        cancelText="取消"
        type="danger"
        onConfirm={deleteConfirm.onConfirm}
        onCancel={() => setDeleteConfirm({ isOpen: false, type: 'field', name: '', onConfirm: () => {} })}
      />

      <div className={styles.schemaEditorFooter}>
        <button type="button" onClick={onCancel} className={styles.cancelButton}>取消</button>
        
        {!isPreviewMode ? (
          <button 
            type="button" 
            onClick={() => {
              setIsPreviewMode(true);
              setHasReviewed(true);
            }} 
            className={styles.previewButton}
          >
            预览变更
          </button>
        ) : (
          <button 
            type="button" 
            onClick={() => onSave(editedSchema)} 
            className={styles.saveButton}
            disabled={!hasReviewed}
          >
            确认保存
          </button>
        )}
      </div>
    </div>
  );
};

// ============ 表单预览组件 - 显示变更内容 ============
interface SchemaChangesFormPreviewProps {
  changes: SchemaChange[];
  groups: UIGroup[];
}

const SchemaChangesFormPreview: React.FC<SchemaChangesFormPreviewProps> = ({ changes, groups }) => {
  if (changes.length === 0) {
    return (
      <div className={styles.noChanges}>
        <span className={styles.noChangesIcon}>✓</span>
        <p>没有检测到任何变更</p>
      </div>
    );
  }

  const addedChanges = changes.filter(c => c.changeType === 'added');
  const removedChanges = changes.filter(c => c.changeType === 'removed');
  const modifiedChanges = changes.filter(c => c.changeType === 'modified');

  return (
    <div className={styles.changesFormPreview}>
      <div className={styles.changesSummary}>
        <span className={styles.changeStat}>
          <span className={styles.addedBadge}>+{addedChanges.length}</span> 新增
        </span>
        <span className={styles.changeStat}>
          <span className={styles.removedBadge}>-{removedChanges.length}</span> 删除
        </span>
        <span className={styles.changeStat}>
          <span className={styles.modifiedBadge}>~{modifiedChanges.length}</span> 修改
        </span>
      </div>

      {addedChanges.length > 0 && (
        <div className={styles.changeGroup}>
          <h4 className={styles.changeGroupTitle}>
            <span className={styles.addedIcon}>+</span> 新增字段
          </h4>
          <div className={styles.changeItems}>
            {addedChanges.map(change => (
              <div key={change.fieldName} className={`${styles.changeItem} ${styles.added}`}>
                <div className={styles.changeItemHeader}>
                  <code className={styles.fieldNameCode}>{change.fieldName}</code>
                  {change.label !== change.fieldName && (
                    <span className={styles.fieldLabel}>({change.label})</span>
                  )}
                </div>
                <div className={styles.changeItemDetails}>
                  <span className={styles.detailItem}>类型: {change.newValue?.type || 'string'}</span>
                  {change.newValue?.description && (
                    <span className={styles.detailItem}>描述: {change.newValue.description}</span>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {removedChanges.length > 0 && (
        <div className={styles.changeGroup}>
          <h4 className={styles.changeGroupTitle}>
            <span className={styles.removedIcon}>-</span> 删除字段
          </h4>
          <div className={styles.changeItems}>
            {removedChanges.map(change => (
              <div key={change.fieldName} className={`${styles.changeItem} ${styles.removed}`}>
                <div className={styles.changeItemHeader}>
                  <code className={styles.fieldNameCode}>{change.fieldName}</code>
                  {change.label !== change.fieldName && (
                    <span className={styles.fieldLabel}>({change.label})</span>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {modifiedChanges.length > 0 && (
        <div className={styles.changeGroup}>
          <h4 className={styles.changeGroupTitle}>
            <span className={styles.modifiedIcon}>~</span> 修改字段
          </h4>
          <div className={styles.changeItems}>
            {modifiedChanges.map(change => (
              <div key={change.fieldName} className={`${styles.changeItem} ${styles.modified}`}>
                <div className={styles.changeItemHeader}>
                  <code className={styles.fieldNameCode}>{change.fieldName}</code>
                  {change.label !== change.fieldName && (
                    <span className={styles.fieldLabel}>({change.label})</span>
                  )}
                </div>
                <div className={styles.changeItemDiff}>
                  <FieldDiffDisplay oldValue={change.oldValue} newValue={change.newValue} />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};

// ============ 字段差异显示组件 ============
interface FieldDiffDisplayProps {
  oldValue: any;
  newValue: any;
}

const FieldDiffDisplay: React.FC<FieldDiffDisplayProps> = ({ oldValue, newValue }) => {
  const diffs: { key: string; old: any; new: any }[] = [];
  
  // 比较 property 属性
  const oldProp = oldValue?.property || oldValue || {};
  const newProp = newValue?.property || newValue || {};
  
  const allKeys = new Set([...Object.keys(oldProp), ...Object.keys(newProp)]);
  allKeys.forEach(key => {
    const oldVal = oldProp[key];
    const newVal = newProp[key];
    if (JSON.stringify(oldVal) !== JSON.stringify(newVal)) {
      diffs.push({ key, old: oldVal, new: newVal });
    }
  });
  
  // 比较 UI 配置
  const oldUi = oldValue?.ui || {};
  const newUi = newValue?.ui || {};
  const uiKeys = new Set([...Object.keys(oldUi), ...Object.keys(newUi)]);
  uiKeys.forEach(key => {
    const oldVal = oldUi[key];
    const newVal = newUi[key];
    if (JSON.stringify(oldVal) !== JSON.stringify(newVal)) {
      diffs.push({ key: `ui.${key}`, old: oldVal, new: newVal });
    }
  });
  
  // 比较 required 状态
  if (oldValue?.required !== newValue?.required) {
    diffs.push({ key: 'required', old: oldValue?.required, new: newValue?.required });
  }

  if (diffs.length === 0) {
    return <span className={styles.noDiff}>无明显差异</span>;
  }

  return (
    <div className={styles.diffList}>
      {diffs.slice(0, 5).map(diff => (
        <div key={diff.key} className={styles.diffItem}>
          <span className={styles.diffKey}>{diff.key}:</span>
          <span className={styles.diffOld}>{formatValue(diff.old)}</span>
          <span className={styles.diffArrow}>→</span>
          <span className={styles.diffNew}>{formatValue(diff.new)}</span>
        </div>
      ))}
      {diffs.length > 5 && (
        <div className={styles.diffMore}>还有 {diffs.length - 5} 项变更...</div>
      )}
    </div>
  );
};

const formatValue = (value: any): string => {
  if (value === undefined) return '(未设置)';
  if (value === null) return 'null';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
};

// ============ JSON 预览组件 - 左右对比视图 ============
interface SchemaChangesJsonPreviewProps {
  originalSchema: OpenAPISchema;
  editedSchema: OpenAPISchema;
  changes: SchemaChange[];
}

// 计算两个 JSON 字符串的行级差异
interface LineDiff {
  type: 'unchanged' | 'added' | 'removed' | 'modified';
  leftLine?: string;
  rightLine?: string;
  leftLineNum?: number;
  rightLineNum?: number;
}

const computeLineDiffs = (oldJson: string, newJson: string): LineDiff[] => {
  const oldLines = oldJson.split('\n');
  const newLines = newJson.split('\n');
  const diffs: LineDiff[] = [];
  
  // 使用简单的 LCS (最长公共子序列) 算法来计算差异
  const lcs = (a: string[], b: string[]): number[][] => {
    const m = a.length;
    const n = b.length;
    const dp: number[][] = Array(m + 1).fill(null).map(() => Array(n + 1).fill(0));
    
    for (let i = 1; i <= m; i++) {
      for (let j = 1; j <= n; j++) {
        if (a[i - 1] === b[j - 1]) {
          dp[i][j] = dp[i - 1][j - 1] + 1;
        } else {
          dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
        }
      }
    }
    return dp;
  };
  
  const dp = lcs(oldLines, newLines);
  
  // 回溯找出差异
  let i = oldLines.length;
  let j = newLines.length;
  const result: LineDiff[] = [];
  
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && oldLines[i - 1] === newLines[j - 1]) {
      result.unshift({
        type: 'unchanged',
        leftLine: oldLines[i - 1],
        rightLine: newLines[j - 1],
        leftLineNum: i,
        rightLineNum: j,
      });
      i--;
      j--;
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      result.unshift({
        type: 'added',
        rightLine: newLines[j - 1],
        rightLineNum: j,
      });
      j--;
    } else if (i > 0) {
      result.unshift({
        type: 'removed',
        leftLine: oldLines[i - 1],
        leftLineNum: i,
      });
      i--;
    }
  }
  
  return result;
};

const SchemaChangesJsonPreview: React.FC<SchemaChangesJsonPreviewProps> = ({ 
  originalSchema,
  editedSchema, 
  changes 
}) => {
  const leftPanelRef = useRef<HTMLPreElement>(null);
  const rightPanelRef = useRef<HTMLPreElement>(null);
  const leftMinimapRef = useRef<HTMLDivElement>(null);
  const rightMinimapRef = useRef<HTMLDivElement>(null);
  const [syncScroll, setSyncScroll] = useState(true);
  
  // 计算 JSON 字符串
  const { oldJson, newJson, lineDiffs, firstChangeIndex, changePositions } = useMemo(() => {
    const oldStr = JSON.stringify(originalSchema, null, 2);
    const newStr = JSON.stringify(editedSchema, null, 2);
    const diffs = computeLineDiffs(oldStr, newStr);
    
    // 找到第一个变更的位置和所有变更位置
    let firstIdx = -1;
    const positions: { index: number; type: 'added' | 'removed' }[] = [];
    for (let i = 0; i < diffs.length; i++) {
      if (diffs[i].type !== 'unchanged') {
        if (firstIdx === -1) firstIdx = i;
        positions.push({ index: i, type: diffs[i].type as 'added' | 'removed' });
      }
    }
    
    return { oldJson: oldStr, newJson: newStr, lineDiffs: diffs, firstChangeIndex: firstIdx, changePositions: positions };
  }, [originalSchema, editedSchema]);
  
  // 自动滚动到第一个变更位置
  useEffect(() => {
    if (firstChangeIndex >= 0 && leftPanelRef.current && rightPanelRef.current) {
      const lineHeight = 20; // 每行大约 20px
      const scrollTop = Math.max(0, (firstChangeIndex - 3) * lineHeight); // 提前 3 行显示
      
      leftPanelRef.current.scrollTop = scrollTop;
      rightPanelRef.current.scrollTop = scrollTop;
    }
  }, [firstChangeIndex]);
  
  // 同步滚动
  const handleScroll = useCallback((source: 'left' | 'right') => {
    if (!syncScroll) return;
    
    const sourcePanel = source === 'left' ? leftPanelRef.current : rightPanelRef.current;
    const targetPanel = source === 'left' ? rightPanelRef.current : leftPanelRef.current;
    
    if (sourcePanel && targetPanel) {
      targetPanel.scrollTop = sourcePanel.scrollTop;
    }
  }, [syncScroll]);
  
  // 统计变更数量
  const stats = useMemo(() => {
    let added = 0, removed = 0, modified = 0;
    lineDiffs.forEach(d => {
      if (d.type === 'added') added++;
      else if (d.type === 'removed') removed++;
    });
    return { added, removed, modified: changes.filter(c => c.changeType === 'modified').length };
  }, [lineDiffs, changes]);
  
  // 点击迷你地图跳转到对应位置
  const handleMinimapClick = useCallback((index: number) => {
    const lineHeight = 20;
    const scrollTop = Math.max(0, (index - 3) * lineHeight);
    
    if (leftPanelRef.current) leftPanelRef.current.scrollTop = scrollTop;
    if (rightPanelRef.current) rightPanelRef.current.scrollTop = scrollTop;
  }, []);

  return (
    <div className={styles.jsonDiffContainer}>
      <div className={styles.jsonDiffHeader}>
        <div className={styles.jsonDiffStats}>
          <span className={styles.diffStatItem}>
            <span className={styles.addedBadge}>+{stats.added}</span> 新增行
          </span>
          <span className={styles.diffStatItem}>
            <span className={styles.removedBadge}>-{stats.removed}</span> 删除行
          </span>
          <span className={styles.diffStatItem}>
            <span className={styles.modifiedBadge}>~{stats.modified}</span> 修改字段
          </span>
        </div>
        <label className={styles.syncScrollLabel}>
          <input 
            type="checkbox" 
            checked={syncScroll} 
            onChange={(e) => setSyncScroll(e.target.checked)} 
          />
          同步滚动
        </label>
      </div>
      
      <div className={styles.jsonDiffPanels}>
        {/* 左侧 - 原始数据 */}
        <div className={styles.jsonDiffPanel}>
          <div className={styles.jsonDiffPanelHeader}>
            <span className={styles.panelTitle}>📄 原始 Schema</span>
            <span className={styles.panelSubtitle}>修改前</span>
          </div>
          <div className={styles.jsonDiffCodeWrapper}>
            <pre 
              ref={leftPanelRef}
              className={styles.jsonDiffCode}
              onScroll={() => handleScroll('left')}
            >
              {lineDiffs.map((diff, index) => {
                if (diff.type === 'added') {
                  return (
                    <div key={`left-${index}`} className={`${styles.diffLine} ${styles.emptyLine}`}>
                      <span className={styles.diffLineNumber}></span>
                      <span className={styles.diffLineContent}></span>
                    </div>
                  );
                }
                
                const lineClass = diff.type === 'removed' 
                  ? `${styles.diffLine} ${styles.removedLine}` 
                  : styles.diffLine;
                
                return (
                  <div key={`left-${index}`} className={lineClass}>
                    <span className={styles.diffLineNumber}>{diff.leftLineNum}</span>
                    <span className={styles.diffLineContent}>{diff.leftLine}</span>
                  </div>
                );
              })}
            </pre>
            {/* 左侧迷你地图 - 在滚动条旁边 */}
            <div className={styles.diffMinimap} ref={leftMinimapRef}>
              {changePositions.filter(p => p.type === 'removed').map((pos, idx) => (
                <div
                  key={`left-marker-${idx}`}
                  className={`${styles.minimapMarker} ${styles.removedMarker}`}
                  style={{ top: `${(pos.index / lineDiffs.length) * 100}%` }}
                  onClick={() => handleMinimapClick(pos.index)}
                  title={`第 ${pos.index + 1} 行 - 删除`}
                />
              ))}
            </div>
          </div>
        </div>
        
        {/* 右侧 - 新数据 */}
        <div className={styles.jsonDiffPanel}>
          <div className={styles.jsonDiffPanelHeader}>
            <span className={styles.panelTitle}>📝 修改后 Schema</span>
            <span className={styles.panelSubtitle}>当前编辑</span>
          </div>
          <div className={styles.jsonDiffCodeWrapper}>
            <pre 
              ref={rightPanelRef}
              className={styles.jsonDiffCode}
              onScroll={() => handleScroll('right')}
            >
              {lineDiffs.map((diff, index) => {
                if (diff.type === 'removed') {
                  return (
                    <div key={`right-${index}`} className={`${styles.diffLine} ${styles.emptyLine}`}>
                      <span className={styles.diffLineNumber}></span>
                      <span className={styles.diffLineContent}></span>
                    </div>
                  );
                }
                
                const lineClass = diff.type === 'added' 
                  ? `${styles.diffLine} ${styles.addedLine}` 
                  : styles.diffLine;
                
                return (
                  <div key={`right-${index}`} className={lineClass}>
                    <span className={styles.diffLineNumber}>{diff.rightLineNum}</span>
                    <span className={styles.diffLineContent}>{diff.rightLine}</span>
                  </div>
                );
              })}
            </pre>
            {/* 右侧迷你地图 - 在滚动条旁边 */}
            <div className={styles.diffMinimap} ref={rightMinimapRef}>
              {changePositions.filter(p => p.type === 'added').map((pos, idx) => (
                <div
                  key={`right-marker-${idx}`}
                  className={`${styles.minimapMarker} ${styles.addedMarker}`}
                  style={{ top: `${(pos.index / lineDiffs.length) * 100}%` }}
                  onClick={() => handleMinimapClick(pos.index)}
                  title={`第 ${pos.index + 1} 行 - 新增`}
                />
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default OpenAPISchemaEditor;
