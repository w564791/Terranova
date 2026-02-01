import React from 'react';
import type { FormSchema } from './index';
import styles from './FormField.module.css';
import { JsonEditor } from './JsonEditor';

// 优化2: 可搜索的选择框组件
const SearchableSelect: React.FC<{
  options: string[];
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}> = ({ options, value, onChange, placeholder }) => {
  const [isOpen, setIsOpen] = React.useState(false);
  const [searchTerm, setSearchTerm] = React.useState('');
  
  const filteredOptions = options.filter(option =>
    option.toLowerCase().includes(searchTerm.toLowerCase())
  );
  
  return (
    <div className={styles.searchableSelect}>
      <div 
        className={styles.selectDisplay}
        onClick={() => setIsOpen(!isOpen)}
      >
        {value || placeholder || '请选择...'}
        <span className={styles.selectArrow}>▼</span>
      </div>
      {isOpen && (
        <div className={styles.selectDropdown}>
          <input
            type="text"
            placeholder="搜索选项..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className={styles.selectSearch}
            onClick={(e) => e.stopPropagation()}
            autoFocus
          />
          <div className={styles.selectOptions}>
            {filteredOptions.length > 0 ? (
              filteredOptions.map(option => (
                <div
                  key={option}
                  className={`${styles.selectOption} ${value === option ? styles.selected : ''}`}
                  onClick={() => {
                    onChange(option);
                    setIsOpen(false);
                    setSearchTerm('');
                  }}
                >
                  {option}
                </div>
              ))
            ) : (
              <div className={styles.noOptions}>没有匹配的选项</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

interface FormFieldProps {
  name: string;
  schema: FormSchema[string];
  value: any;
  onChange: (value: any) => void;
  error?: string;
}

export const FormField: React.FC<FormFieldProps> = ({
  name,
  schema,
  value,
  onChange,
  error
}) => {
  const renderInput = () => {
    switch (schema.type) {
      case 'string':
        // 优化6: 如果有at_least_one_of，使用可搜索的下拉选择
        if (schema.at_least_one_of && Array.isArray(schema.at_least_one_of)) {
          return (
            <SearchableSelect
              options={schema.at_least_one_of}
              value={value || ''}
              onChange={onChange}
              placeholder="请选择..."
            />
          );
        }
        // 兼容旧的options字段 - 也支持搜索
        if (schema.options) {
          return (
            <SearchableSelect
              options={schema.options}
              value={value || ''}
              onChange={onChange}
              placeholder="请选择..."
            />
          );
        }
        // 优化1: 不在placeholder中重复description
        return (
          <input
            type="text"
            value={value || ''}
            onChange={(e) => onChange(e.target.value)}
            className={styles.input}
            placeholder={`请输入${name}`}
          />
        );

      case 'number':
        return (
          <div className={styles.numberInputWrapper}>
            <input
              type="number"
              value={value || ''}
              onChange={(e) => onChange(Number(e.target.value))}
              className={styles.numberInput}
              placeholder={`请输入${name}`}
            />
            <div className={styles.numberControls}>
              <button
                type="button"
                className={styles.numberUp}
                onClick={() => onChange((value || 0) + 1)}
                title="增加"
              >
                ▲
              </button>
              <button
                type="button"
                className={styles.numberDown}
                onClick={() => onChange((value || 0) - 1)}
                title="减少"
              >
                ▼
              </button>
            </div>
          </div>
        );

      case 'boolean':
        // 优化2: 使用开关样式，不重复显示description
        return (
          <button
            type="button"
            className={`${styles.switchButton} ${value ? styles.switchOn : styles.switchOff}`}
            onClick={() => onChange(!value)}
            aria-pressed={value || false}
          >
            <span className={styles.switchThumb} />
          </button>
        );

      case 'map':
        // TypeMap: 用户可以自由添加key-value对，都是string类型
        const mapValue = value && typeof value === 'object' ? value : {};
        
        // 优化4: 处理must_include参数 - 自动渲染必填key
        const mustIncludeKeys = schema.must_include || [];
        
        // 确保must_include的key都存在于mapValue中
        const ensuredMapValue = { ...mapValue };
        mustIncludeKeys.forEach(key => {
          if (!(key in ensuredMapValue)) {
            ensuredMapValue[key] = '';
          }
        });
        
        const mapEntries = Object.entries(ensuredMapValue);
        
        return (
          <div className={styles.mapField}>
            {(schema.description || mustIncludeKeys.length > 0) && (
              <div className={styles.mapDescription}>
                {schema.description && (
                  <div dangerouslySetInnerHTML={{ __html: schema.description }} />
                )}
                {mustIncludeKeys.length > 0 && (
                  <div className={styles.required}>
                    必须包含: {mustIncludeKeys.join(', ')}
                  </div>
                )}
              </div>
            )}
            {mapEntries.map(([key, val], index) => {
              const isRequired = mustIncludeKeys.includes(key);
              return (
              <div key={index} className={`${styles.mapItem} ${isRequired ? styles.requiredMapItem : ''}`}>
                <input
                  type="text"
                  placeholder="键名"
                  value={key}
                  onChange={(e) => {
                    // 优化4: 必填key不允许修改键名
                    if (isRequired) return;
                    const newMap = { ...ensuredMapValue };
                    delete newMap[key];
                    if (e.target.value) {
                      newMap[e.target.value] = val;
                    }
                    onChange(newMap);
                  }}
                  className={styles.input}
                  style={{ width: '45%', marginRight: '8px' }}
                  disabled={isRequired}
                />
                <input
                  type="text"
                  placeholder={isRequired ? "必填值" : "键值"}
                  value={val as string}
                  onChange={(e) => {
                    const newMap = { ...ensuredMapValue };
                    newMap[key] = e.target.value;
                    onChange(newMap);
                  }}
                  className={`${styles.input} ${isRequired && !val ? styles.requiredInput : ''}`}
                  style={{ width: '45%', marginRight: '8px' }}
                />
                <button
                  type="button"
                  onClick={() => {
                    // 优化4: 必填key不允许删除
                    if (isRequired) return;
                    const newMap = { ...ensuredMapValue };
                    delete newMap[key];
                    onChange(newMap);
                  }}
                  className={styles.removeButton}
                  disabled={isRequired}
                  style={{ opacity: isRequired ? 0.3 : 1 }}
                >
                  ×
                </button>
              </div>
              );
            })}
            <button
              type="button"
              onClick={() => {
                const newMap = { ...ensuredMapValue };
                const newKey = `key${Object.keys(newMap).length + 1}`;
                newMap[newKey] = '';
                onChange(newMap);
              }}
              className={styles.addButton}
            >
              + 添加键值对
            </button>
          </div>
        );

      case 'object':
        // TypeObject: 固定结构，但子字段可能有hidden_default或都是可选的
        // 注意：后端可能将properties放在elem字段中（如filter字段）
        const objectProperties = schema.properties || schema.elem;
        
        if (objectProperties) {
          const objectValue = value && typeof value === 'object' ? value : {};
          
          // 分离基础字段和高级字段
          // 如果字段有hidden_default或者不是required，都作为高级字段
          const basicProps = Object.entries(objectProperties).filter(
            ([_, propSchema]) => !propSchema.hidden_default && propSchema.required === true
          );
          const advancedProps = Object.entries(objectProperties).filter(
            ([_, propSchema]) => propSchema.hidden_default || propSchema.required !== true
          );
          
          // 管理高级字段的显示状态
          const [selectedAdvancedProps, setSelectedAdvancedProps] = React.useState<string[]>([]);
          const [showPropSelector, setShowPropSelector] = React.useState(false);
          const [propSearchTerm, setPropSearchTerm] = React.useState('');
          
          // 可用的高级字段（未选择的）
          const availableAdvancedProps = advancedProps.filter(
            ([propName]) => !selectedAdvancedProps.includes(propName)
          );
          
          // 搜索过滤
          const filteredAdvancedProps = availableAdvancedProps.filter(([propName, propSchema]) => {
            if (!propSearchTerm) return true;
            const searchLower = propSearchTerm.toLowerCase();
            return propName.toLowerCase().includes(searchLower) ||
              (propSchema.description && propSchema.description.toLowerCase().includes(searchLower));
          });
          
          const handleAddAdvancedProp = (propName: string) => {
            setSelectedAdvancedProps([...selectedAdvancedProps, propName]);
            setShowPropSelector(false);
            setPropSearchTerm('');
          };
          
          const handleRemoveAdvancedProp = (propName: string) => {
            setSelectedAdvancedProps(selectedAdvancedProps.filter(p => p !== propName));
            // 清除该字段的值
            const newValue = { ...objectValue };
            delete newValue[propName];
            onChange(newValue);
          };
          
          // 检查字段是否可以移除
          const canRemoveProp = (propName: string, propSchema: any) => {
            const propValue = objectValue[propName];
            if (propSchema.type === 'array') {
              return !propValue || (Array.isArray(propValue) && propValue.length === 0);
            }
            return true;
          };
          
          return (
            <div className={styles.objectField}>
              {schema.description && (
                <div className={styles.objectDescription}>
                  <div dangerouslySetInnerHTML={{ __html: schema.description }} />
                </div>
              )}
              <div className={styles.objectProperties}>
                {/* 渲染基础字段 */}
                {basicProps.map(([propName, propSchema]) => (
                  <FormField
                    key={propName}
                    name={propName}
                    schema={propSchema}
                    value={objectValue[propName]}
                    onChange={(propValue) => {
                      const newValue = { ...objectValue };
                      newValue[propName] = propValue;
                      onChange(newValue);
                    }}
                  />
                ))}
                
                {/* 渲染已选择的高级字段 */}
                {selectedAdvancedProps.map(propName => {
                  const propSchema = objectProperties[propName];
                  return (
                    <div key={propName} className={styles.advancedPropRow}>
                      <div className={styles.advancedPropContent}>
                        <FormField
                          name={propName}
                          schema={propSchema}
                          value={objectValue[propName]}
                          onChange={(propValue) => {
                            const newValue = { ...objectValue };
                            newValue[propName] = propValue;
                            onChange(newValue);
                          }}
                        />
                      </div>
                      <div className={styles.advancedPropActions}>
                        {canRemoveProp(propName, propSchema) ? (
                          <button
                            type="button"
                            className={styles.removePropButton}
                            onClick={() => handleRemoveAdvancedProp(propName)}
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
                  );
                })}
                
                {/* 高级字段选择器 */}
                {availableAdvancedProps.length > 0 && (
                  <div className={styles.propSelector}>
                    {!showPropSelector ? (
                      <button
                        type="button"
                        className={styles.addPropButton}
                        onClick={() => setShowPropSelector(true)}
                      >
                        + 添加高级选项 ({availableAdvancedProps.length} 可用)
                      </button>
                    ) : (
                      <div className={styles.propSelectorPanel}>
                        <div className={styles.propSelectorHeader}>
                          <span>选择要添加的字段：</span>
                          <button
                            type="button"
                            className={styles.cancelButton}
                            onClick={() => {
                              setShowPropSelector(false);
                              setPropSearchTerm('');
                            }}
                          >
                            取消
                          </button>
                        </div>
                        <div className={styles.searchBox}>
                          <input
                            type="text"
                            placeholder="搜索字段名称或描述..."
                            value={propSearchTerm}
                            onChange={(e) => setPropSearchTerm(e.target.value)}
                            className={styles.searchInput}
                            autoFocus
                          />
                        </div>
                        <div className={styles.propSelectorList}>
                          {filteredAdvancedProps.length > 0 ? (
                            filteredAdvancedProps.map(([propName, propSchema]) => (
                              <button
                                key={propName}
                                type="button"
                                className={styles.propOption}
                                onClick={() => handleAddAdvancedProp(propName)}
                              >
                                <span className={styles.propName}>{propName}</span>
                                {propSchema.description && (
                                  <span className={styles.propDescription}>
                                    {propSchema.description.replace(/<[^>]*>/g, '').substring(0, 50)}...
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
            </div>
          );
        }
        return null;

      case 'json':
        // TypeJsonString: JSON编辑器，支持语法高亮和验证
        return (
          <JsonEditor
            value={value || ''}
            onChange={onChange}
            placeholder='{\n  "key": "value"\n}'
            minHeight={200}
            maxHeight={400}
          />
        );

      case 'array':
        const arrayValue = Array.isArray(value) ? value : [];
        
        // 检查是否有elem字段（TypeListObject）或items.properties（标准array）
        const itemSchema = schema.elem || schema.items?.properties;
        
        return (
          <div className={styles.arrayField}>
            {schema.description && (
              <div className={styles.arrayDescription}>
                <div dangerouslySetInnerHTML={{ __html: schema.description }} />
              </div>
            )}
            {arrayValue.map((item, index) => (
              <div key={index} className={styles.arrayItem}>
                <div className={styles.arrayItemHeader}>
                  <button
                    type="button"
                    onClick={() => {
                      const newArray = [...arrayValue];
                      newArray.splice(index, 1);
                      onChange(newArray);
                    }}
                    className={styles.removeButton}
                  >
                    删除
                  </button>
                </div>
                {itemSchema && (
                  <div className={styles.arrayItemContent}>
                    {Object.entries(itemSchema).map(([propName, propSchema]) => {
                      // 如果子字段是object类型，递归渲染FormField，它会处理高级功能
                      // 注意：properties可能在elem字段中
                      if (propSchema.type === 'object' && (propSchema.properties || propSchema.elem)) {
                        return (
                          <FormField
                            key={propName}
                            name={propName}
                            schema={propSchema}
                            value={item?.[propName]}
                            onChange={(propValue) => {
                              const newArray = [...arrayValue];
                              newArray[index] = { ...newArray[index], [propName]: propValue };
                              onChange(newArray);
                            }}
                          />
                        );
                      }
                      // 其他类型正常渲染
                      return (
                        <FormField
                          key={propName}
                          name={propName}
                          schema={propSchema}
                          value={item?.[propName]}
                          onChange={(propValue) => {
                            const newArray = [...arrayValue];
                            newArray[index] = { ...newArray[index], [propName]: propValue };
                            onChange(newArray);
                          }}
                        />
                      );
                    })}
                  </div>
                )}
              </div>
            ))}
            <button
              type="button"
              onClick={() => {
                const newItem = itemSchema ? 
                  Object.keys(itemSchema).reduce((obj, key) => {
                    const fieldSchema = itemSchema[key];
                    obj[key] = fieldSchema.default !== undefined ? fieldSchema.default : 
                               fieldSchema.type === 'boolean' ? false :
                               fieldSchema.type === 'number' ? 0 :
                               fieldSchema.type === 'array' ? [] :
                               fieldSchema.type === 'object' || fieldSchema.type === 'map' ? {} :
                               '';
                    return obj;
                  }, {} as any) : {};
                onChange([...arrayValue, newItem]);
              }}
              className={styles.addButton}
            >
              + 添加项目
            </button>
          </div>
        );

      default:
        return (
          <input
            type="text"
            value={value || ''}
            onChange={(e) => onChange(e.target.value)}
            className={styles.input}
            placeholder={schema.description}
          />
        );
    }
  };

  return (
    <div className={styles.field}>
      <label className={styles.label}>
        {name}
        {schema.required && <span className={styles.required}>*</span>}
        {/* 优化4: force_new字段显示感叹号 */}
        {schema.force_new && (
          <span 
            className={styles.forceNew} 
            style={{ color: schema.color || '#ff9800' }}
            title="修改此字段将强制重建资源"
          >
            
          </span>
        )}
      </label>
      {renderInput()}
      {/* 优化5: description作为HTML渲染 */}
      {schema.description && (
        <div 
          className={styles.description}
          dangerouslySetInnerHTML={{ __html: schema.description }}
        />
      )}
      {error && <div className={styles.error}>{error}</div>}
    </div>
  );
};
