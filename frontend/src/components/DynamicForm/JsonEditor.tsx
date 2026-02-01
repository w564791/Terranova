import React, { useState, useEffect, useCallback } from 'react';
import styles from './JsonEditor.module.css';

interface JsonEditorProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  minHeight?: number;
  maxHeight?: number;
}

interface JsonError {
  line: number;
  column: number;
  message: string;
}

export const JsonEditor: React.FC<JsonEditorProps> = ({
  value,
  onChange,
  placeholder = '{\n  "key": "value"\n}',
  minHeight = 200,
  maxHeight = 600
}) => {
  const [displayValue, setDisplayValue] = useState('');
  const [error, setError] = useState<JsonError | null>(null);
  const [lineNumbers, setLineNumbers] = useState<number[]>([1]);
  const [isLargeFile, setIsLargeFile] = useState(false);
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);

  // 初始化显示值
  useEffect(() => {
    if (value) {
      try {
        // 如果值是压缩的JSON字符串，尝试解析并格式化
        const parsed = JSON.parse(value);
        const formatted = JSON.stringify(parsed, null, 2);
        setDisplayValue(formatted);
        setError(null);
        
        // 检查是否为大文件（超过1000行）
        const lineCount = formatted.split('\n').length;
        setIsLargeFile(lineCount > 1000);
      } catch {
        // 如果不是有效的JSON，直接显示原值
        setDisplayValue(value);
        const lineCount = value.split('\n').length;
        setIsLargeFile(lineCount > 1000);
      }
    } else {
      setDisplayValue('');
      setIsLargeFile(false);
    }
  }, [value]);

  // 更新行号（大文件时优化）
  useEffect(() => {
    const lines = displayValue.split('\n').length;
    // 对于大文件，只在必要时更新行号
    if (!isLargeFile || lines !== lineNumbers.length) {
      setLineNumbers(Array.from({ length: lines }, (_, i) => i + 1));
    }
  }, [displayValue, isLargeFile, lineNumbers.length]);

  // 验证JSON格式
  const validateJson = useCallback((text: string): JsonError | null => {
    if (!text.trim()) {
      return null;
    }

    try {
      JSON.parse(text);
      return null;
    } catch (e) {
      const error = e as SyntaxError;
      const match = error.message.match(/position (\d+)/);
      const position = match ? parseInt(match[1]) : 0;
      
      // 计算行号和列号
      const lines = text.substring(0, position).split('\n');
      const line = lines.length;
      const column = lines[lines.length - 1].length + 1;

      return {
        line,
        column,
        message: error.message
      };
    }
  }, []);

  // 自动调整textarea高度
  const adjustTextareaHeight = useCallback(() => {
    const textarea = textareaRef.current;
    if (textarea) {
      textarea.style.height = 'auto';
      textarea.style.height = `${textarea.scrollHeight}px`;
    }
  }, []);

  // 当内容变化时调整高度
  useEffect(() => {
    adjustTextareaHeight();
  }, [displayValue, adjustTextareaHeight]);

  // 处理输入变化
  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newValue = e.target.value;
    setDisplayValue(newValue);

    // 实时验证
    const jsonError = validateJson(newValue);
    setError(jsonError);

    // 如果JSON有效，传递压缩的字符串给父组件
    if (!jsonError && newValue.trim()) {
      try {
        const parsed = JSON.parse(newValue);
        onChange(JSON.stringify(parsed)); // 压缩的JSON字符串
      } catch {
        onChange(newValue);
      }
    } else {
      onChange(newValue);
    }
  };

  // 格式化JSON
  const formatJson = () => {
    try {
      const parsed = JSON.parse(displayValue);
      const formatted = JSON.stringify(parsed, null, 2);
      setDisplayValue(formatted);
      setError(null);
      onChange(JSON.stringify(parsed)); // 传递压缩的版本
    } catch (e) {
      // 保持错误状态
    }
  };

  // 语法高亮功能已禁用，因为高亮层与 textarea 的滚动同步在处理长字符串时会导致显示问题
  // 如果需要语法高亮，建议使用专业的代码编辑器组件如 Monaco Editor 或 CodeMirror

  return (
    <div className={styles.jsonEditor}>
      <div className={styles.toolbar}>
        <button
          type="button"
          onClick={formatJson}
          className={styles.toolButton}
          disabled={!!error}
          title="格式化JSON"
        >
          格式化
        </button>
        {isLargeFile && (
          <span className={styles.infoIndicator}>
            大文件模式 ({displayValue.split('\n').length} 行)
          </span>
        )}
        {error && (
          <span className={styles.errorIndicator}>
            JSON格式错误
          </span>
        )}
      </div>
      
      <div className={styles.editorContainer}>
        <div className={styles.lineNumbers}>
          {lineNumbers.map(num => (
            <div
              key={num}
              className={`${styles.lineNumber} ${
                error && error.line === num ? styles.errorLine : ''
              }`}
            >
              {num}
            </div>
          ))}
        </div>
        
        <div className={styles.editorWrapper}>
          <textarea
            ref={textareaRef}
            className={styles.editor}
            value={displayValue}
            onChange={handleChange}
            placeholder={placeholder}
            spellCheck={false}
            style={{
              minHeight: `${minHeight}px`
            }}
          />
          
          {/* 语法高亮层已禁用 - 避免长字符串显示问题 */}
        </div>
      </div>
      
      {error && (
        <div className={styles.errorMessage}>
          <span className={styles.errorIcon}></span>
          第 {error.line} 行，第 {error.column} 列：{error.message}
        </div>
      )}
    </div>
  );
};
