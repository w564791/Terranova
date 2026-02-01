import React, { useState, useCallback, useEffect, useRef } from 'react';
import { Alert, Button, Space, message } from 'antd';
import { CopyOutlined, FormatPainterOutlined, CheckOutlined } from '@ant-design/icons';
import type { OpenAPISchema } from '../../services/schemaV2';
import styles from './ModuleSchemaV2.module.css';

interface SchemaJsonEditorProps {
  schema: OpenAPISchema;
  onChange: (schema: OpenAPISchema) => void;
  readOnly?: boolean;
}

const SchemaJsonEditor: React.FC<SchemaJsonEditorProps> = ({
  schema,
  onChange,
  readOnly = false,
}) => {
  const [jsonText, setJsonText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isDirty, setIsDirty] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // 初始化和同步 schema
  useEffect(() => {
    try {
      const formatted = JSON.stringify(schema, null, 2);
      setJsonText(formatted);
      setError(null);
      setIsDirty(false);
    } catch (e) {
      setError('Schema 格式化失败');
    }
  }, [schema]);

  // 处理文本变化
  const handleTextChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newText = e.target.value;
    setJsonText(newText);
    setIsDirty(true);

    // 实时验证 JSON
    try {
      JSON.parse(newText);
      setError(null);
    } catch (e) {
      setError('JSON 格式错误');
    }
  }, []);

  // 应用更改
  const handleApply = useCallback(() => {
    try {
      const parsed = JSON.parse(jsonText);
      onChange(parsed);
      setIsDirty(false);
      setError(null);
      message.success('Schema 已更新');
    } catch (e) {
      setError('JSON 解析失败，请检查格式');
    }
  }, [jsonText, onChange]);

  // 格式化 JSON
  const handleFormat = useCallback(() => {
    try {
      const parsed = JSON.parse(jsonText);
      const formatted = JSON.stringify(parsed, null, 2);
      setJsonText(formatted);
      setError(null);
    } catch (e) {
      setError('JSON 格式错误，无法格式化');
    }
  }, [jsonText]);

  // 复制到剪贴板
  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(jsonText);
      message.success('已复制到剪贴板');
    } catch (e) {
      message.error('复制失败');
    }
  }, [jsonText]);

  // 重置更改
  const handleReset = useCallback(() => {
    try {
      const formatted = JSON.stringify(schema, null, 2);
      setJsonText(formatted);
      setError(null);
      setIsDirty(false);
    } catch (e) {
      setError('重置失败');
    }
  }, [schema]);

  return (
    <div className={styles.jsonEditorContainer}>
      {/* 工具栏 */}
      <div className={styles.jsonEditorToolbar}>
        <Space>
          <Button
            size="small"
            icon={<FormatPainterOutlined />}
            onClick={handleFormat}
            disabled={readOnly}
          >
            格式化
          </Button>
          <Button
            size="small"
            icon={<CopyOutlined />}
            onClick={handleCopy}
          >
            复制
          </Button>
        </Space>
        <Space>
          {isDirty && !readOnly && (
            <>
              <Button size="small" onClick={handleReset}>
                重置
              </Button>
              <Button
                type="primary"
                size="small"
                icon={<CheckOutlined />}
                onClick={handleApply}
                disabled={!!error}
              >
                应用更改
              </Button>
            </>
          )}
        </Space>
      </div>

      {/* 错误提示 */}
      {error && (
        <Alert
          type="error"
          message={error}
          showIcon
          style={{ marginBottom: 8 }}
        />
      )}

      {/* 编辑器 */}
      <textarea
        ref={textareaRef}
        value={jsonText}
        onChange={handleTextChange}
        readOnly={readOnly}
        className={styles.jsonTextarea}
        spellCheck={false}
        placeholder="在此编辑 JSON Schema..."
      />

      {/* 状态栏 */}
      <div className={styles.jsonEditorStatus}>
        <span>
          {jsonText.split('\n').length} 行 · {jsonText.length} 字符
        </span>
        {isDirty && <span style={{ color: '#faad14' }}>· 未保存</span>}
        {!error && <span style={{ color: '#52c41a' }}>· JSON 有效</span>}
      </div>
    </div>
  );
};

export default SchemaJsonEditor;
