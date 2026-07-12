import React, { useState } from 'react';
import { FormField } from './FormField';
import type { FormSchema } from './index';
import styles from './FormPreview.module.css';

interface FormPreviewProps {
  schema: any;
  values: Record<string, any>;
  onClose: () => void;
  inline?: boolean; // 是否为内嵌模式（不显示弹窗）
  viewMode?: 'form' | 'json'; // 外部控制的视图模式（用于inline模式）
  onViewModeChange?: (mode: 'form' | 'json') => void; // 视图模式变化回调
}

export const FormPreview: React.FC<FormPreviewProps> = ({
  schema,
  values,
  onClose,
  inline = false,
  viewMode: externalViewMode,
  onViewModeChange
}) => {
  const [internalViewMode, setInternalViewMode] = useState<'form' | 'json'>('form');
  
  // 使用外部viewMode（如果提供）或内部viewMode
  const viewMode = externalViewMode !== undefined ? externalViewMode : internalViewMode;
  
  // 视图切换处理
  const handleViewModeChange = (mode: 'form' | 'json') => {
    if (onViewModeChange) {
      onViewModeChange(mode);
    } else {
      setInternalViewMode(mode);
    }
  };

  // 检查各种类型的空值
  const isEmpty = (val: any): boolean => {
    if (val === undefined || val === null) return true;
    if (typeof val === 'string' && val === '') return true;
    if (Array.isArray(val) && val.length === 0) return true;
    if (typeof val === 'object' && !Array.isArray(val) && Object.keys(val).length === 0) return true;
    return false;
  };

  // 深度清理对象，移除所有空值
  const deepClean = (obj: any): any => {
    if (isEmpty(obj)) return undefined;
    
    if (Array.isArray(obj)) {
      const cleaned = obj
        .map(item => deepClean(item))
        .filter(item => item !== undefined);
      return cleaned.length > 0 ? cleaned : undefined;
    }
    
    if (typeof obj === 'object' && obj !== null) {
      const result: any = {};
      for (const [key, value] of Object.entries(obj)) {
        const cleanedValue = deepClean(value);
        if (cleanedValue !== undefined) {
          result[key] = cleanedValue;
        }
      }
      return Object.keys(result).length > 0 ? result : undefined;
    }
    
    return obj;
  };

  // 过滤掉空值和未定义的字段（用于表单视图）
  const filterEmptyValues = (obj: any): any => {
    const result: any = {};
    
    for (const [key, value] of Object.entries(obj)) {
      if (isEmpty(value)) continue;
      
      if (Array.isArray(value)) {
        // 深度清理数组
        const cleanedArray = deepClean(value);
        if (cleanedArray !== undefined) {
          result[key] = cleanedArray;
        }
      } else if (typeof value === 'object' && value !== null) {
        // 深度清理对象
        const cleanedObject = deepClean(value);
        if (cleanedObject !== undefined) {
          result[key] = cleanedObject;
        }
      } else {
        // 其他非空值直接保留（包括json类型的字符串）
        result[key] = value;
      }
    }
    
    return result;
  };

  // 为JSON视图准备数据（解析json类型字段）
  const prepareJsonViewData = (obj: any): any => {
    const result: any = {};
    
    for (const [key, value] of Object.entries(obj)) {
      const fieldSchema = schema[key];
      
      // 对于json类型字段，如果是字符串，解析为对象
      if (fieldSchema && fieldSchema.type === 'json' && typeof value === 'string') {
        try {
          result[key] = JSON.parse(value);
        } catch {
          result[key] = value;
        }
      } else {
        result[key] = value;
      }
    }
    
    return result;
  };

  const filteredValues = filterEmptyValues(values);
  const jsonViewData = prepareJsonViewData(filteredValues);

  // 获取字段的显示名称和描述
  const getFieldInfo = (fieldName: string) => {
    const fieldSchema = schema[fieldName];
    if (!fieldSchema) return { name: fieldName, description: '' };
    
    return {
      name: fieldName,
      description: fieldSchema.description || '',
      type: fieldSchema.type,
      required: fieldSchema.required,
      forceNew: fieldSchema.force_new
    };
  };

  // 格式化值的显示 - 简洁的表单展示方式
  const formatValue = (value: any, type?: string, fieldName?: string): React.ReactNode => {
    if (value === null || value === undefined) return <span className={styles.emptyValue}>未设置</span>;
    
    if (type === 'boolean') {
      return <span className={styles.booleanValue}>{value ? 'true' : 'false'}</span>;
    }
    
    if (type === 'json') {
      // JSON类型保持terraform格式，但不显示完整内容
      if (typeof value === 'string' && value.startsWith('${jsonencode(')) {
        return <span className={styles.jsonIndicator}>JSON配置</span>;
      }
      return <span className={styles.stringValue}>{value}</span>;
    }
    
    // 处理数组类型 - 递归展示嵌套内容
    if (Array.isArray(value)) {
      if (value.length === 0) return <span className={styles.emptyValue}>[]</span>;
      
      // 如果是简单类型数组，直接显示
      if (value.every(item => typeof item !== 'object' || item === null)) {
        return (
          <span className={styles.simpleArray}>
            [{value.map(v => String(v)).join(', ')}]
          </span>
        );
      }
      
      // 复杂类型数组，展开显示
      return (
        <div className={styles.arrayPreview}>
          {value.map((item, index) => (
            <div key={index} className={styles.arrayPreviewItem}>
              <div className={styles.arrayPreviewHeader}>
                <span className={styles.arrayPreviewIndex}>#{index + 1}</span>
              </div>
              {typeof item === 'object' && item !== null ? (
                <div className={styles.arrayPreviewContent}>
                  {Object.entries(item).map(([key, val]) => (
                    <div key={key} className={styles.arrayPreviewField}>
                      <span className={styles.fieldLabel}>{key}:</span>
                      <div className={styles.fieldContent}>
                        {/* 递归调用formatValue处理嵌套值 */}
                        {formatValue(val, undefined, key)}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className={styles.arrayPreviewContent}>
                  <span className={styles.fieldContent}>{String(item)}</span>
                </div>
              )}
            </div>
          ))}
        </div>
      );
    }
    
    // 处理对象类型 - 简洁展示
    if (typeof value === 'object' && value !== null) {
      const entries = Object.entries(value);
      if (entries.length === 0) return <span className={styles.emptyValue}>{}</span>;
      
      return (
        <div className={styles.objectPreview}>
          {entries.map(([key, val]) => (
            <div key={key} className={styles.objectPreviewField}>
              <span className={styles.fieldLabel}>{key}:</span>
              <span className={styles.fieldContent}>
                {Array.isArray(val) 
                  ? `[${val.length} items]`
                  : typeof val === 'object' && val !== null
                  ? '{...}'
                  : String(val)}
              </span>
            </div>
          ))}
        </div>
      );
    }
    
    return <span className={styles.stringValue}>{String(value)}</span>;
  };

  // 渲染表单模式的预览 - 使用FormField组件，只读模式
  const renderFormView = () => {
    const entries = Object.entries(filteredValues);
    
    if (entries.length === 0) {
      return <div className={styles.emptyState}>暂无配置数据</div>;
    }

    // 分组：必填字段和可选字段
    const requiredFields = entries.filter(([key]) => 
      schema[key]?.required === true
    );
    const optionalFields = entries.filter(([key]) => 
      schema[key]?.required !== true
    );

    // 创建一个空的onChange函数（只读模式）
    const handleChange = () => {};

    return (
      <div className={styles.formView}>
        <div className={styles.readOnlyNotice}>
          <span>配置预览（只读）</span>
        </div>
        
        {requiredFields.length > 0 && (
          <div className={styles.fieldGroup}>
            <h3 className={styles.groupTitle}>必填参数</h3>
            <div className={styles.fieldsContainer}>
              {requiredFields.map(([key, value]) => (
                <div key={key} className={styles.readOnlyField}>
                  <FormField
                    name={key}
                    schema={schema[key]}
                    value={value}
                    onChange={handleChange}
                  />
                </div>
              ))}
            </div>
          </div>
        )}

        {optionalFields.length > 0 && (
          <div className={styles.fieldGroup}>
            <h3 className={styles.groupTitle}>可选参数</h3>
            <div className={styles.fieldsContainer}>
              {optionalFields.map(([key, value]) => (
                <div key={key} className={styles.readOnlyField}>
                  <FormField
                    name={key}
                    schema={schema[key]}
                    value={value}
                    onChange={handleChange}
                  />
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    );
  };

  // 渲染JSON模式的预览
  const renderJsonView = () => {
    const jsonString = JSON.stringify(jsonViewData, null, 2);

    return (
      <div className={styles.jsonView}>
        <div className={styles.jsonToolbar}>
          <button
            type="button"
            className={styles.copyButton}
            onClick={() => {
              navigator.clipboard.writeText(jsonString);
              // 可以添加复制成功提示
            }}
          >
            复制JSON
          </button>
        </div>
        <pre className={styles.jsonContent}>
          <code className={styles.jsonCode}>{jsonString}</code>
        </pre>
      </div>
    );
  };

  // 内嵌模式：不显示弹窗，只返回内容
  if (inline) {
    return (
      <div className={styles.inlinePreview}>
        <div className={styles.previewBody}>
          {viewMode === 'form' ? renderFormView() : renderJsonView()}
        </div>
      </div>
    );
  }

  // 弹窗模式：完整的弹窗UI
  return (
    <div className={styles.previewOverlay}>
      <div className={styles.previewModal}>
        <div className={styles.previewHeader}>
          <h2 className={styles.previewTitle}>配置预览</h2>
          <div className={styles.viewToggle}>
            <button
              type="button"
              className={`${styles.toggleButton} ${viewMode === 'form' ? styles.active : ''}`}
              onClick={() => handleViewModeChange('form')}
            >
              表单视图
            </button>
            <button
              type="button"
              className={`${styles.toggleButton} ${viewMode === 'json' ? styles.active : ''}`}
              onClick={() => handleViewModeChange('json')}
            >
              JSON视图
            </button>
          </div>
          <button
            type="button"
            className={styles.closeButton}
            onClick={onClose}
            aria-label="关闭"
          >
            ✕
          </button>
        </div>
        
        <div className={styles.previewBody}>
          {viewMode === 'form' ? renderFormView() : renderJsonView()}
        </div>
        
        <div className={styles.previewFooter}>
          <div className={styles.summary}>
            共 {Object.keys(filteredValues).length} 个参数已配置
          </div>
          <button
            type="button"
            className={styles.confirmButton}
            onClick={onClose}
          >
            确认
          </button>
        </div>
      </div>
    </div>
  );
};
