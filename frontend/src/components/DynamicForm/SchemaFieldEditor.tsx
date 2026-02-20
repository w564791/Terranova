import React, { useState } from 'react';
import styles from './SchemaFieldEditor.module.css';

interface ValidationRule {
  condition: string;
  error_message: string;
}

interface SchemaFieldEditorProps {
  fieldName: string;
  fieldSchema: any;
  onSave: (field: any) => void;
  onCancel: () => void;
}

const SchemaFieldEditor: React.FC<SchemaFieldEditorProps> = ({
  fieldName,
  fieldSchema,
  onSave,
  onCancel
}) => {
  const [field, setField] = useState(fieldSchema);
  const [showAdvanced, setShowAdvanced] = useState(false);

  // 更新字段值
  const updateField = (key: string, value: any) => {
    setField({ ...field, [key]: value });
  };

  // 处理默认值输入
  const handleDefaultValueChange = (value: string) => {
    // 根据类型转换默认值
    let parsedValue: any = value;
    
    if (field.type === 'boolean') {
      parsedValue = value === 'true';
    } else if (field.type === 'number') {
      parsedValue = value ? parseFloat(value) : null;
    } else if (field.type === 'list' || field.type === 'map' || field.type === 'object') {
      try {
        parsedValue = value ? JSON.parse(value) : null;
      } catch (e) {
        parsedValue = value;
      }
    }
    
    updateField('default', parsedValue);
  };

  // 获取默认值的字符串表示
  const getDefaultValueString = (): string => {
    if (field.default === null || field.default === undefined) {
      return '';
    }
    if (typeof field.default === 'object') {
      return JSON.stringify(field.default, null, 2);
    }
    return String(field.default);
  };

  // 添加validation规则
  const addValidationRule = () => {
    const currentValidation = field.validation || [];
    updateField('validation', [
      ...currentValidation,
      { condition: '', error_message: '' }
    ]);
  };

  // 更新validation规则
  const updateValidationRule = (index: number, key: 'condition' | 'error_message', value: string) => {
    const currentValidation = [...(field.validation || [])];
    currentValidation[index] = {
      ...currentValidation[index],
      [key]: value
    };
    updateField('validation', currentValidation);
  };

  // 删除validation规则
  const removeValidationRule = (index: number) => {
    const currentValidation = [...(field.validation || [])];
    currentValidation.splice(index, 1);
    updateField('validation', currentValidation);
  };

  return (
    <div className={styles.overlay} onClick={onCancel}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h3>编辑变量: {fieldName}</h3>
          <button onClick={onCancel} className={styles.closeButton}>
            ✕
          </button>
        </div>

        <div className={styles.content}>
          {/* 基础信息 */}
          <section className={styles.section}>
            <h4>基础信息</h4>
            
            <div className={styles.formGroup}>
              <label>变量名</label>
              <input
                type="text"
                value={fieldName}
                disabled
                className={styles.input}
              />
            </div>

            <div className={styles.formGroup}>
              <label>类型</label>
              <select
                value={field.type}
                onChange={(e) => updateField('type', e.target.value)}
                className={styles.select}
              >
                <option value="string">String</option>
                <option value="number">Number</option>
                <option value="boolean">Boolean</option>
                <option value="list">List</option>
                <option value="map">Map</option>
                <option value="object">Object</option>
                <option value="set">Set</option>
              </select>
            </div>

            <div className={styles.formGroup}>
              <label className={styles.checkboxLabel}>
                <input
                  type="checkbox"
                  checked={field.required || false}
                  onChange={(e) => updateField('required', e.target.checked)}
                />
                <span>必填字段</span>
              </label>
            </div>

            <div className={styles.formGroup}>
              <label>参数级别</label>
              <select
                value={field.level || 'advanced'}
                onChange={(e) => updateField('level', e.target.value)}
                className={styles.select}
              >
                <option value="basic">基础参数（创建时必须显示）</option>
                <option value="advanced">高级参数（默认折叠）</option>
              </select>
              <small className={styles.hint}>
                基础参数会在创建表单中直接显示，高级参数默认折叠在"高级配置"中
              </small>
            </div>

            <div className={styles.formGroup}>
              <label>别名（中文名称）</label>
              <input
                type="text"
                value={field.alias || ''}
                onChange={(e) => updateField('alias', e.target.value)}
                className={styles.input}
                placeholder="例如：存储桶名称"
              />
            </div>

            <div className={styles.formGroup}>
              <label>描述</label>
              <textarea
                value={field.description || ''}
                onChange={(e) => updateField('description', e.target.value)}
                className={styles.textarea}
                rows={3}
                placeholder="字段描述信息..."
              />
            </div>

            <div className={styles.formGroup}>
              <label>默认值</label>
              {field.type === 'boolean' ? (
                <select
                  value={String(field.default)}
                  onChange={(e) => handleDefaultValueChange(e.target.value)}
                  className={styles.select}
                >
                  <option value="">无默认值</option>
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              ) : field.type === 'list' || field.type === 'map' || field.type === 'object' ? (
                <textarea
                  value={getDefaultValueString()}
                  onChange={(e) => handleDefaultValueChange(e.target.value)}
                  className={styles.textarea}
                  rows={4}
                  placeholder='例如: ["item1", "item2"] 或 {"key": "value"}'
                />
              ) : (
                <input
                  type={field.type === 'number' ? 'number' : 'text'}
                  value={getDefaultValueString()}
                  onChange={(e) => handleDefaultValueChange(e.target.value)}
                  className={styles.input}
                  placeholder="输入默认值..."
                />
              )}
            </div>
          </section>

          {/* Validation规则 */}
          <section className={styles.section}>
            <div className={styles.sectionHeader}>
              <h4>验证规则</h4>
              <button
                onClick={addValidationRule}
                className={styles.addButton}
              >
                + 添加规则
              </button>
            </div>
            
            {field.validation && field.validation.length > 0 ? (
              <div className={styles.validationRules}>
                {field.validation.map((rule: ValidationRule, index: number) => (
                  <div key={index} className={styles.validationRule}>
                    <div className={styles.ruleHeader}>
                      <span>规则 {index + 1}</span>
                      <button
                        onClick={() => removeValidationRule(index)}
                        className={styles.removeButton}
                      >
                        删除
                      </button>
                    </div>
                    <div className={styles.formGroup}>
                      <label>条件表达式</label>
                      <input
                        type="text"
                        value={rule.condition}
                        onChange={(e) => updateValidationRule(index, 'condition', e.target.value)}
                        className={styles.input}
                        placeholder='例如: ${contains(["value1", "value2"], var.field_name)}'
                      />
                    </div>
                    <div className={styles.formGroup}>
                      <label>错误消息</label>
                      <input
                        type="text"
                        value={rule.error_message}
                        onChange={(e) => updateValidationRule(index, 'error_message', e.target.value)}
                        className={styles.input}
                        placeholder="验证失败时显示的错误消息"
                      />
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className={styles.emptyState}>暂无验证规则</p>
            )}
          </section>

          {/* 高级选项 */}
          <section className={styles.section}>
            <div
              className={styles.sectionHeader}
              onClick={() => setShowAdvanced(!showAdvanced)}
              style={{ cursor: 'pointer' }}
            >
              <h4>高级选项</h4>
              <span>{showAdvanced ? '▼' : '▶'}</span>
            </div>

            {showAdvanced && (
              <div className={styles.advancedOptions}>
                <div className={styles.formGroup}>
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={field.sensitive || false}
                      onChange={(e) => updateField('sensitive', e.target.checked)}
                    />
                    <span>敏感字段（密码等）</span>
                  </label>
                </div>

                <div className={styles.formGroup}>
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={field.force_new || false}
                      onChange={(e) => updateField('force_new', e.target.checked)}
                    />
                    <span>强制重建（修改时重建资源）</span>
                  </label>
                </div>

                <div className={styles.formGroup}>
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={field.hidden_default !== false}
                      onChange={(e) => updateField('hidden_default', e.target.checked)}
                    />
                    <span>默认隐藏（新建时不自动渲染）</span>
                  </label>
                </div>
              </div>
            )}
          </section>
        </div>

        <div className={styles.footer}>
          <button onClick={() => onSave(field)} className={styles.saveButton}>
            保存
          </button>
          <button onClick={onCancel} className={styles.cancelButton}>
            取消
          </button>
        </div>
      </div>
    </div>
  );
};

export default SchemaFieldEditor;
