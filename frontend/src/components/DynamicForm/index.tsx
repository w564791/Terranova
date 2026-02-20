import React from 'react';
import { FormField } from './FormField';
import { FormPreview } from './FormPreview';
import styles from './DynamicForm.module.css';

export { FormPreview } from './FormPreview';

export interface FormSchema {
  [key: string]: {
    type: 'string' | 'number' | 'boolean' | 'object' | 'array' | 'map' | 'json' | 'text';
    required?: boolean;
    description?: string;
    default?: any;
    options?: string[];
    properties?: FormSchema;
    items?: {
      type: string;
      properties?: FormSchema;
    };
    elem?: FormSchema;  // 支持TypeListObject的elem字段
    hidden_default?: boolean;
    at_least_one_of?: string[];  // 字符串类型的选项列表
    must_include?: string[];  // TypeMap必须包含的key
    force_new?: boolean;  // 修改后强制重建资源
    color?: string;  // 自定义颜色
  };
}

interface DynamicFormProps {
  schema: FormSchema;
  values: Record<string, any>;
  onChange: (values: Record<string, any>) => void;
  errors?: Record<string, string>;
  showAdvanced?: boolean;
  initialFieldsToShow?: string[];  // 初始要显示的高级字段列表
}

export const DynamicForm: React.FC<DynamicFormProps> = ({
  schema,
  values,
  onChange,
  errors = {},
  showAdvanced: initialShowAdvanced = false,
  initialFieldsToShow = []
}) => {
  const [showAdvanced, setShowAdvanced] = React.useState(initialShowAdvanced);
  const [selectedAdvancedFields, setSelectedAdvancedFields] = React.useState<string[]>(initialFieldsToShow);
  const [showFieldSelector, setShowFieldSelector] = React.useState(false);
  const [searchTerm, setSearchTerm] = React.useState('');
  const [showPreview, setShowPreview] = React.useState(false);
  
  // 监听initialFieldsToShow的变化，动态更新selectedAdvancedFields
  // 使用 ref 来避免无限循环
  const hasInitializedRef = React.useRef(false);
  
  React.useEffect(() => {
    // 只在首次初始化时设置，避免后续更新导致循环
    if (!hasInitializedRef.current && initialFieldsToShow && initialFieldsToShow.length > 0) {
      console.log('🔄 Updating selectedAdvancedFields with:', initialFieldsToShow);
      setSelectedAdvancedFields(initialFieldsToShow);
      hasInitializedRef.current = true;
    }
  }, [initialFieldsToShow]);
  
  console.log('DynamicForm schema:', schema);
  console.log('DynamicForm values:', values);
  console.log('DynamicForm initialFieldsToShow:', initialFieldsToShow);
  console.log('DynamicForm selectedAdvancedFields:', selectedAdvancedFields);
  
  const handleFieldChange = (fieldName: string, value: any) => {
    onChange({
      ...values,
      [fieldName]: value
    });
  };

  const handleAddAdvancedField = (fieldName: string) => {
    setSelectedAdvancedFields([...selectedAdvancedFields, fieldName]);
    setShowFieldSelector(false);
    setSearchTerm('');
  };

  // 优化1: 支持删除已添加的高级选项
  const handleRemoveAdvancedField = (fieldName: string) => {
    setSelectedAdvancedFields(selectedAdvancedFields.filter(f => f !== fieldName));
    // 清除该字段的值 - 包括数组类型的空数组
    const newValues = { ...values };
    delete newValues[fieldName];
    onChange(newValues);
  };
  
  // 检查字段是否可以移除（数组为空或其他类型）
  const canRemoveField = (fieldName: string, fieldSchema: FormSchema[string]) => {
    const value = values[fieldName];
    // 如果是数组类型，检查是否为空
    if (fieldSchema.type === 'array') {
      return !value || (Array.isArray(value) && value.length === 0);
    }
    // 其他类型总是可以移除
    return true;
  };

  // 检查schema是否为空或无效
  if (!schema || typeof schema !== 'object' || Object.keys(schema).length === 0) {
    console.warn('Schema is empty or invalid:', schema);
    return <div className={styles.form}>Schema数据为空或无效</div>;
  }
  
  // 分组字段：基础字段和高级字段
  // 修复：只有当字段被明确选择时才显示，不因为有值就自动显示
  const basicFields = Object.entries(schema).filter(([fieldName, fieldSchema]) => 
    !fieldSchema.hidden_default && !selectedAdvancedFields.includes(fieldName)
  );
  
  const allAdvancedFields = Object.entries(schema).filter(([fieldName, fieldSchema]) => 
    fieldSchema.hidden_default
  );
  
  // 优化3: 逐级展开的高级字段 - 保持在原位置
  // 修复：对于已选择的字段，只要存在于schema中就显示，不检查hidden_default
  const visibleAdvancedFields = selectedAdvancedFields
    .filter(fieldName => {
      const exists = schema[fieldName];
      console.log(`🔍 Field "${fieldName}": exists=${!!exists}`);
      return exists;  // 只检查字段是否存在，不检查hidden_default
    })
    .map(fieldName => [fieldName, schema[fieldName]] as [string, typeof schema[string]]);
  
  console.log('👁️ visibleAdvancedFields:', visibleAdvancedFields.map(([name]) => name));
  
  // 优化3: 支持搜索的可用高级字段 - 修复bug2
  const allAvailableAdvancedFields = allAdvancedFields
    .filter(([fieldName]) => !selectedAdvancedFields.includes(fieldName));
    
  const filteredAdvancedFields = allAvailableAdvancedFields
    .filter(([fieldName, fieldSchema]) => {
      if (!searchTerm) return true;
      const searchLower = searchTerm.toLowerCase();
      return fieldName.toLowerCase().includes(searchLower) ||
        (fieldSchema.description && fieldSchema.description.toLowerCase().includes(searchLower));
    });
  
  console.log('Basic fields:', basicFields.length);
  console.log('Advanced fields:', allAdvancedFields.length);

  return (
    <>
      <div className={styles.form}>
      {/* 基础字段 */}
      {basicFields.map(([fieldName, fieldSchema]) => (
        <FormField
          key={fieldName}
          name={fieldName}
          schema={fieldSchema}
          value={values[fieldName]}
          onChange={(value) => handleFieldChange(fieldName, value)}
          error={errors[fieldName]}
        />
      ))}
      
      {/* 已选择的高级字段 - 优化1: 移除按钮与字段同行 */}
      {visibleAdvancedFields.map(([fieldName, fieldSchema]) => (
        <div key={fieldName} className={styles.advancedFieldRow}>
          <div className={styles.advancedFieldContent}>
            <FormField
              name={fieldName}
              schema={fieldSchema}
              value={values[fieldName]}
              onChange={(value) => handleFieldChange(fieldName, value)}
              error={errors[fieldName]}
            />
          </div>
          <div className={styles.advancedFieldActions}>
            {canRemoveField(fieldName, fieldSchema) ? (
              <button
                type="button"
                className={styles.removeAdvancedButton}
                onClick={() => handleRemoveAdvancedField(fieldName)}
                title="移除此字段"
              >
                移除
              </button>
            ) : (
              <span className={styles.removeHint}>
                清空内容后可移除
              </span>
            )}
          </div>
        </div>
      ))}
      
      {/* 高级字段选择器 - 优化2&3: 修复搜索bug，调整位置 */}
      {allAvailableAdvancedFields.length > 0 && (
        <div className={styles.advancedSection}>
          {!showFieldSelector ? (
            <button 
              type="button" 
              className={styles.showAdvancedButton}
              onClick={() => setShowFieldSelector(true)}
            >
              + 添加高级选项 ({allAvailableAdvancedFields.length} 可用)
            </button>
          ) : (
            <div className={styles.fieldSelector}>
              <div className={styles.fieldSelectorHeader}>
                <span>选择要添加的字段：</span>
                <button 
                  type="button"
                  className={styles.cancelButton}
                  onClick={() => {
                    setShowFieldSelector(false);
                    setSearchTerm('');
                  }}
                >
                  取消
                </button>
              </div>
              {/* 优化3: 添加搜索框 */}
              <div className={styles.searchBox}>
                <input
                  type="text"
                  placeholder="搜索字段名称或描述..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className={styles.searchInput}
                  autoFocus
                />
              </div>
              <div className={styles.fieldSelectorList}>
                {filteredAdvancedFields.length > 0 ? (
                  filteredAdvancedFields.map(([fieldName, fieldSchema]) => (
                  <button
                    key={fieldName}
                    type="button"
                    className={styles.fieldOption}
                    onClick={() => handleAddAdvancedField(fieldName)}
                  >
                    <span className={styles.fieldName}>{fieldName}</span>
                    {fieldSchema.description && (
                      <span className={styles.fieldDescription}>
                        {fieldSchema.description.replace(/<[^>]*>/g, '').substring(0, 50)}...
                      </span>
                    )}
                  </button>
                  ))
                ) : (
                  <div className={styles.noResults}>
                    没有找到匹配的字段
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}
      </div>
      
      {/* 预览弹窗 */}
      {showPreview && (
        <FormPreview
          schema={schema}
          values={values}
          onClose={() => setShowPreview(false)}
        />
      )}
    </>
  );
};

export default DynamicForm;
