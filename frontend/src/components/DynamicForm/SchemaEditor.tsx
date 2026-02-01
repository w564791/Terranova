import React, { useState } from 'react';
import styles from './SchemaEditor.module.css';
import SchemaFieldEditor from './SchemaFieldEditor';

interface SchemaEditorProps {
  initialSchema: Record<string, any>;
  onSave: (schema: Record<string, any>) => void;
  onCancel: () => void;
}

const SchemaEditor: React.FC<SchemaEditorProps> = ({
  initialSchema,
  onSave,
  onCancel
}) => {
  const [schema, setSchema] = useState<Record<string, any>>(initialSchema);
  const [editingField, setEditingField] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');

  // 获取字段列表
  const fieldNames = Object.keys(schema);
  
  // 过滤字段
  const filteredFields = fieldNames.filter(name =>
    name.toLowerCase().includes(searchTerm.toLowerCase())
  );

  // 删除字段
  const handleDelete = (fieldName: string) => {
    if (window.confirm(`确定要删除字段 "${fieldName}" 吗？`)) {
      const newSchema = { ...schema };
      delete newSchema[fieldName];
      setSchema(newSchema);
    }
  };

  // 保存字段编辑
  const handleSaveField = (fieldName: string, updatedField: any) => {
    setSchema({
      ...schema,
      [fieldName]: updatedField
    });
    setEditingField(null);
  };

  // 获取类型显示名称
  const getTypeDisplay = (type: string): string => {
    const typeMap: Record<string, string> = {
      'string': 'String',
      'number': 'Number',
      'boolean': 'Boolean',
      'list': 'List',
      'map': 'Map',
      'object': 'Object',
      'set': 'Set'
    };
    return typeMap[type] || type;
  };

  return (
    <div className={styles.schemaEditor}>
      <div className={styles.header}>
        <h2>Schema 编辑器</h2>
        <div className={styles.summary}>
          共找到 <strong>{fieldNames.length}</strong> 个变量
        </div>
      </div>

      {/* 搜索框 */}
      <div className={styles.searchBox}>
        <input
          type="text"
          placeholder="搜索字段名称..."
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          className={styles.searchInput}
        />
      </div>

      {/* 表格视图 */}
      <div className={styles.tableContainer}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th>变量名</th>
              <th>类型</th>
              <th>级别</th>
              <th>必填</th>
              <th>默认值</th>
              <th>描述</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {filteredFields.length === 0 ? (
              <tr>
                <td colSpan={7} className={styles.emptyState}>
                  {searchTerm ? '没有找到匹配的字段' : '没有可用的字段'}
                </td>
              </tr>
            ) : (
              filteredFields.map((fieldName) => {
                const field = schema[fieldName];
                return (
                  <tr key={fieldName}>
                    <td className={styles.fieldName}>
                      {fieldName}
                      {field.alias && <span className={styles.alias}>({field.alias})</span>}
                    </td>
                    <td>{getTypeDisplay(field.type)}</td>
                    <td>
                      <span className={field.level === 'basic' ? styles.basicLevel : styles.advancedLevel}>
                        {field.level === 'basic' ? '基础' : '高级'}
                      </span>
                    </td>
                    <td>
                      <span className={field.required ? styles.required : styles.optional}>
                        {field.required ? '✓' : '✗'}
                      </span>
                    </td>
                    <td className={styles.defaultValue}>
                      {field.default !== null && field.default !== undefined
                        ? typeof field.default === 'object'
                          ? JSON.stringify(field.default)
                          : String(field.default)
                        : '-'}
                    </td>
                    <td className={styles.description}>
                      {field.description || '-'}
                    </td>
                    <td className={styles.actions}>
                      <button
                        onClick={() => setEditingField(fieldName)}
                        className={styles.editButton}
                      >
                        编辑
                      </button>
                      <button
                        onClick={() => handleDelete(fieldName)}
                        className={styles.deleteButton}
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* 操作按钮 */}
      <div className={styles.footer}>
        <button onClick={() => onSave(schema)} className={styles.saveButton}>
          保存 Schema
        </button>
        <button onClick={onCancel} className={styles.cancelButton}>
          取消
        </button>
      </div>

      {/* 字段编辑器模态框 */}
      {editingField && (
        <SchemaFieldEditor
          fieldName={editingField}
          fieldSchema={schema[editingField]}
          onSave={(updatedField) => handleSaveField(editingField, updatedField)}
          onCancel={() => setEditingField(null)}
        />
      )}
    </div>
  );
};

export default SchemaEditor;
