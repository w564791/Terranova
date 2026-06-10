import React, { useState, useCallback, useMemo, useRef, useEffect } from 'react';
import { Form, Input, Button, Space, Tag, Tooltip, Alert } from 'antd';
import { LinkOutlined, FormatPainterOutlined, CheckOutlined, WarningOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import { useUIVersionContext } from '../../../contexts/UIVersionContext';
import styles from './JsonEditorWidget.module.css';

const { TextArea } = Input;

/** 当 schema.type 为 object/array 时，值应存储为 parsed object/array 而非 string */
const shouldStoreAsParsedObject = (schema: any): boolean => {
  const t = schema?.type;
  return t === 'object' || t === 'array';
};

/** 检测引用表达式：module.xxx、${module.xxx}、var.xxx、${var.xxx}、local.xxx、${local.xxx} 等 */
const isReferenceExpression = (value: unknown): boolean => {
  if (typeof value !== 'string') return false;
  const s = value.trim();
  return /^(module\.|var\.|local\.|data\.)/.test(s) || /^\$\{.+\}$/.test(s);
};

/**
 * 根据 schema.type 决定存储格式：
 * - 引用表达式始终存为 string
 * - object/array 类型尝试 parse 为对象，parse 失败暂存 string（用户编辑中）
 * - 其他类型存为 string
 */
const resolveFormValue = (rawString: string, schema: any): unknown => {
  if (isReferenceExpression(rawString)) return rawString;
  if (!shouldStoreAsParsedObject(schema)) return rawString;
  try {
    const parsed = JSON.parse(rawString);
    if (typeof parsed === 'object' && parsed !== null) return parsed;
    return rawString;
  } catch {
    return rawString; // 用户编辑中，暂存 string
  }
};

/** JSON 语法高亮 — 单遍 token 扫描，避免多遍 regex 互相污染 */
const highlightJson = (json: string): string => {
  if (!json) return '';

  const escape = (s: string) =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  let result = '';
  let i = 0;
  const len = json.length;

  const wrap = (cls: string, s: string) =>
    `<span class="${cls}">${escape(s)}</span>`;

  while (i < len) {
    const ch = json[i];

    // String — key or value
    if (ch === '"') {
      let end = i + 1;
      while (end < len) {
        if (json[end] === '\\') { end += 2; continue; }
        if (json[end] === '"') { end++; break; }
        end++;
      }
      const str = json.slice(i, end);
      // 判断后面是否紧跟 `:` → 这是 key
      let j = end;
      while (j < len && (json[j] === ' ' || json[j] === '\t')) j++;
      if (json[j] === ':') {
        result += wrap('json-key', str);
      } else {
        result += wrap('json-string', str);
      }
      i = end;
      continue;
    }

    // Number
    if (ch === '-' || (ch >= '0' && ch <= '9')) {
      let end = i;
      if (json[end] === '-') end++;
      while (end < len && /[0-9.eE+\-]/.test(json[end])) end++;
      // 确认不是字符串里的数字
      result += wrap('json-number', json.slice(i, end));
      i = end;
      continue;
    }

    // true / false
    if (json.slice(i, i + 4) === 'true') {
      result += wrap('json-bool', 'true');
      i += 4;
      continue;
    }
    if (json.slice(i, i + 5) === 'false') {
      result += wrap('json-bool', 'false');
      i += 5;
      continue;
    }

    // null
    if (json.slice(i, i + 4) === 'null') {
      result += wrap('json-null', 'null');
      i += 4;
      continue;
    }

    // Brackets
    if (ch === '{' || ch === '}') {
      result += wrap('json-brace', ch);
      i++;
      continue;
    }
    if (ch === '[' || ch === ']') {
      result += wrap('json-bracket', ch);
      i++;
      continue;
    }

    // Comma
    if (ch === ',') {
      result += wrap('json-comma', ch);
      i++;
      continue;
    }

    // Colon
    if (ch === ':') {
      result += escape(':');
      i++;
      continue;
    }

    // Whitespace and other chars — pass through
    result += escape(ch);
    i++;
  }

  return result;
};

/**
 * JsonEditorWidget - JSON 编辑器组件
 *
 * 用于渲染 TypeJsonString (9) 类型的数据
 * V2: 基础 TextArea + 格式化/压缩按钮
 * V3: 语法高亮 + 行号 + overlay 编辑 + 实时验证
 */
const JsonEditorWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
  context,
}) => {
  const form = Form.useFormInstance();
  const formValue = Form.useWatch(name, form);
  const { isV3 } = useUIVersionContext();

  const label = uiConfig?.label || schema.title || (typeof name === 'string' ? name : '');
  const help = uiConfig?.help || schema.description;

  // JSON 验证状态
  const [jsonError, setJsonError] = useState<string | null>(null);

  // V3 编辑状态
  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState('');
  const [copied, setCopied] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const preRef = useRef<HTMLPreElement>(null);
  const lineNumbersRef = useRef<HTMLDivElement>(null);
  const scrollAreaRef = useRef<HTMLDivElement>(null);

  // 当前值 — 对 JSON 字符串自动格式化显示
  const currentValue = useMemo(() => {
    if (typeof formValue === 'string') {
      // 尝试格式化 JSON 字符串
      if (formValue.trim()) {
        try {
          const parsed = JSON.parse(formValue);
          if (typeof parsed === 'object' && parsed !== null) {
            return JSON.stringify(parsed, null, 2);
          }
        } catch {
          // 不是有效 JSON，原样显示
        }
      }
      return formValue;
    }
    if (formValue !== undefined && formValue !== null) {
      try {
        return JSON.stringify(formValue, null, 2);
      } catch {
        return String(formValue);
      }
    }
    return '';
  }, [formValue]);

  // 检查是否是引用表达式
  const isModuleReference = isReferenceExpression(formValue);

  // 获取 Manifest 上下文
  const manifestContext = context?.manifest;
  const hasManifestContext = !!manifestContext?.currentNodeId;

  // 验证 JSON
  const validateJson = useCallback((value: string): boolean => {
    if (!value || value.trim() === '') {
      setJsonError(null);
      return true;
    }

    // 如果是引用表达式，不验证 JSON
    if (isReferenceExpression(value)) {
      setJsonError(null);
      return true;
    }

    try {
      JSON.parse(value);
      setJsonError(null);
      return true;
    } catch (e) {
      setJsonError((e as Error).message);
      return false;
    }
  }, []);

  // 处理值变化
  const handleChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newValue = e.target.value;
    validateJson(newValue);
    form.setFieldValue(name, resolveFormValue(newValue, schema));
  }, [form, name, validateJson, schema]);

  // 格式化 JSON
  const handleFormat = useCallback(() => {
    if (!currentValue || isModuleReference) return;

    try {
      const parsed = JSON.parse(currentValue);
      const formatted = JSON.stringify(parsed, null, 2);
      form.setFieldValue(name, shouldStoreAsParsedObject(schema) ? parsed : formatted);
      setJsonError(null);
    } catch (e) {
      setJsonError((e as Error).message);
    }
  }, [currentValue, isModuleReference, form, name, schema]);

  // 压缩 JSON
  const handleMinify = useCallback(() => {
    if (!currentValue || isModuleReference) return;

    try {
      const parsed = JSON.parse(currentValue);
      const minified = JSON.stringify(parsed);
      form.setFieldValue(name, shouldStoreAsParsedObject(schema) ? parsed : minified);
      setJsonError(null);
    } catch (e) {
      setJsonError((e as Error).message);
    }
  }, [currentValue, isModuleReference, form, name, schema]);

  // 渲染引用标签
  const renderReferenceTag = () => {
    if (!isModuleReference) return null;

    const parts = (formValue as string).split('.');
    if (parts.length >= 3) {
      const instanceName = parts[1];
      const outputName = parts.slice(2).join('.');
      return (
        <Tooltip title={`引用自 ${instanceName} 的 ${outputName}`}>
          <Tag
            color="blue"
            icon={<LinkOutlined />}
            style={{ marginLeft: 8, cursor: 'pointer' }}
          >
            {instanceName}.{outputName}
          </Tag>
        </Tooltip>
      );
    }
    return null;
  };

  // 渲染验证状态
  const renderValidationStatus = () => {
    if (isModuleReference) return null;
    if (!currentValue) return null;

    if (jsonError) {
      return (
        <Tag color="error" icon={<WarningOutlined />}>
          JSON 无效
        </Tag>
      );
    }

    return (
      <Tag color="success" icon={<CheckOutlined />}>
        JSON 有效
      </Tag>
    );
  };

  // ========== V3 Mode ==========
  if (isV3) {
    const displayText = isEditing ? editValue : currentValue;
    const highlightedHTML = useMemo(() => highlightJson(displayText), [displayText]);
    const lineCount = useMemo(() => (displayText || '').split('\n').length, [displayText]);

    // 进入编辑模式
    const handleClick = useCallback(() => {
      if (readOnly || disabled || isModuleReference) return;
      setEditValue(currentValue);
      setIsEditing(true);
      setJsonError(null);
      setTimeout(() => textareaRef.current?.focus(), 0);
    }, [readOnly, disabled, isModuleReference, currentValue]);

    // 同步滚动
    const handleScroll = useCallback(() => {
      if (textareaRef.current && preRef.current) {
        preRef.current.scrollTop = textareaRef.current.scrollTop;
        preRef.current.scrollLeft = textareaRef.current.scrollLeft;
      }
      if (textareaRef.current && lineNumbersRef.current) {
        lineNumbersRef.current.scrollTop = textareaRef.current.scrollTop;
      }
    }, []);

    // 查看模式下的滚动同步
    const handleViewScroll = useCallback(() => {
      if (scrollAreaRef.current && lineNumbersRef.current) {
        lineNumbersRef.current.scrollTop = scrollAreaRef.current.scrollTop;
      }
    }, []);

    // V3 编辑处理
    const handleV3Change = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const newText = e.target.value;
      setEditValue(newText);
      handleScroll();
      validateJson(newText);
      form.setFieldValue(name, resolveFormValue(newText, schema));
    }, [handleScroll, validateJson, form, name, schema]);

    // 退出编辑
    const handleBlur = useCallback((e: React.FocusEvent<HTMLTextAreaElement>) => {
      const related = e.relatedTarget as Node | null;
      if (related && (e.currentTarget.closest(`.${styles.v3Container}`) as Node)?.contains(related)) {
        requestAnimationFrame(() => textareaRef.current?.focus());
        return;
      }
      // 退出时格式化
      if (!jsonError && editValue) {
        try {
          const parsed = JSON.parse(editValue);
          const formatted = JSON.stringify(parsed, null, 2);
          form.setFieldValue(name, shouldStoreAsParsedObject(schema) ? parsed : formatted);
          setEditValue(formatted);
        } catch {
          // 保持原样
        }
      }
      setIsEditing(false);
    }, [jsonError, editValue, form, name, schema]);

    // 自动调整高度
    useEffect(() => {
      if (isEditing && textareaRef.current && preRef.current) {
        textareaRef.current.style.height = 'auto';
        const height = Math.max(textareaRef.current.scrollHeight, preRef.current.scrollHeight);
        textareaRef.current.style.height = `${height}px`;
      }
    }, [isEditing, editValue]);

    // V3 格式化按钮
    const handleV3Format = useCallback(() => {
      if (!currentValue || isModuleReference) return;
      try {
        const parsed = JSON.parse(currentValue);
        const formatted = JSON.stringify(parsed, null, 2);
        form.setFieldValue(name, shouldStoreAsParsedObject(schema) ? parsed : formatted);
        setEditValue(formatted);
        setJsonError(null);
      } catch (e) {
        setJsonError((e as Error).message);
      }
    }, [currentValue, isModuleReference, form, name, schema]);

    // V3 压缩按钮
    const handleV3Minify = useCallback(() => {
      if (!currentValue || isModuleReference) return;
      try {
        const parsed = JSON.parse(currentValue);
        const minified = JSON.stringify(parsed);
        form.setFieldValue(name, shouldStoreAsParsedObject(schema) ? parsed : minified);
        setEditValue(minified);
        setJsonError(null);
      } catch (e) {
        setJsonError((e as Error).message);
      }
    }, [currentValue, isModuleReference, form, name, schema]);

    // V3 复制按钮
    const handleV3Copy = useCallback(async () => {
      if (!currentValue) return;
      try {
        await navigator.clipboard.writeText(currentValue);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      } catch {
        // fallback
      }
    }, [currentValue]);

    // V3 键盘处理
    const handleV3KeyDown = useCallback((e: React.KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        setIsEditing(false);
        setEditValue(currentValue);
        setJsonError(null);
        textareaRef.current?.blur();
      }
      if (e.key === 'Tab') {
        e.preventDefault();
        const ta = textareaRef.current;
        if (ta) {
          const start = ta.selectionStart;
          const end = ta.selectionEnd;
          const newVal = editValue.substring(0, start) + '  ' + editValue.substring(end);
          setEditValue(newVal);
          validateJson(newVal);
          form.setFieldValue(name, resolveFormValue(newVal, schema));
          setTimeout(() => {
            ta.selectionStart = ta.selectionEnd = start + 2;
          }, 0);
        }
      }
    }, [currentValue, editValue, validateJson, form, name, schema]);

    // 引用表达式特殊渲染
    if (isModuleReference) {
      return (
        <Form.Item
          label={
            <span>
              {label}
              {renderReferenceTag()}
            </span>
          }
          name={name}
          help={help}
        >
          <div className={styles.v3ReferenceBadge}>
            <LinkOutlined />
            <span>{formValue as string}</span>
          </div>
        </Form.Item>
      );
    }

    return (
      <Form.Item
        label={
          <span>
            {label}
            {renderReferenceTag()}
          </span>
        }
        name={name}
        help={help}
        rules={[
          ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
        ]}
      >
        <div
          className={`${styles.v3Container} ${jsonError ? styles.hasError : ''}`}
        >
          {/* Toolbar */}
          <div className={styles.v3Toolbar}>
            <div className={styles.v3ToolbarLeft}>
              <div className={`${styles.v3StatusDot} ${
                !currentValue ? styles.empty : jsonError ? styles.invalid : styles.valid
              }`} />
              <span className={styles.v3StatusText}>
                {isEditing ? '编辑中 — Esc 退出' : 'JSON'}
                {currentValue && !jsonError && ` · ${lineCount} 行`}
              </span>
            </div>
            <div className={styles.v3ToolbarRight}>
              {!readOnly && (
                <>
                  <button
                    className={styles.v3ToolBtn}
                    onClick={handleV3Format}
                    disabled={!currentValue || !!jsonError}
                    title="格式化 JSON"
                  >
                    格式化
                  </button>
                  <button
                    className={styles.v3ToolBtn}
                    onClick={handleV3Minify}
                    disabled={!currentValue || !!jsonError}
                    title="压缩 JSON"
                  >
                    压缩
                  </button>
                </>
              )}
              <button
                className={styles.v3ToolBtn}
                onClick={handleV3Copy}
                disabled={!currentValue}
                title={copied ? '已复制' : '复制到剪贴板'}
              >
                {copied ? '已复制 ✓' : '复制'}
              </button>
            </div>
          </div>

          {/* Editor body */}
          <div
            className={styles.v3EditorBody}
            onClick={isEditing ? undefined : handleClick}
            style={{
              cursor: isEditing || readOnly || disabled ? 'default' : 'text',
              opacity: readOnly || disabled ? 0.6 : 1,
              maxHeight: 400
            }}
          >
            {/* Line numbers */}
            <div className={styles.v3LineNumbers} ref={lineNumbersRef}>
              {Array.from({ length: lineCount }, (_, i) => (
                <span key={i}>{i + 1}</span>
              ))}
            </div>

            {/* Scroll area */}
            <div
              ref={scrollAreaRef}
              className={styles.v3ScrollArea}
              onScroll={isEditing ? undefined : handleViewScroll}
            >
              <pre
                ref={preRef}
                className={styles.v3HighlightPre}
                dangerouslySetInnerHTML={{ __html: highlightedHTML || '<span style="color:#475569">点击此处编辑 JSON...</span>' }}
              />
              {isEditing && (
                <textarea
                  ref={textareaRef}
                  value={editValue}
                  onChange={handleV3Change}
                  onBlur={handleBlur}
                  onKeyDown={handleV3KeyDown}
                  onScroll={handleScroll}
                  className={styles.v3Textarea}
                  spellCheck={false}
                  placeholder="在此编辑 JSON..."
                />
              )}
            </div>
          </div>

          {/* Error bar */}
          {jsonError && (
            <div className={styles.v3ErrorBar}>
              <span className={styles.v3ErrorIcon}>⚠</span>
              <span>{jsonError}</span>
            </div>
          )}
        </div>
      </Form.Item>
    );
  }

  // ========== V2 Mode (unchanged) ==========
  return (
    <Form.Item
      label={
        <span>
          {label}
          {renderReferenceTag()}
          {renderValidationStatus()}
        </span>
      }
      name={name}
      help={
        <span>
          {help}
          {hasManifestContext && !isModuleReference && (
            <span style={{ color: '#1890ff', marginLeft: 8, fontSize: 11 }}>
              输入 module. 开头的引用表达式
            </span>
          )}
        </span>
      }
      validateStatus={jsonError ? 'error' : undefined}
      rules={[
        ...(schema.required ? [{ required: true, message: `${label}是必填项` }] : []),
        {
          validator: async (_, value) => {
            if (!value) return Promise.resolve();
            // 已成功 parse 为 object/array 的值，直接通过
            if (typeof value === 'object') return Promise.resolve();
            // 引用表达式不验证 JSON
            if (isReferenceExpression(value)) return Promise.resolve();
            try {
              JSON.parse(value);
              return Promise.resolve();
            } catch {
              return Promise.reject(new Error('请输入有效的 JSON 格式'));
            }
          },
        },
      ]}
    >
      <div>
        <TextArea
          value={currentValue}
          onChange={handleChange}
          placeholder={uiConfig?.placeholder || '请输入 JSON 格式的内容'}
          disabled={disabled}
          readOnly={readOnly}
          autoSize={{ minRows: 6, maxRows: 20 }}
          style={{
            fontFamily: 'Monaco, Menlo, "Ubuntu Mono", Consolas, monospace',
            fontSize: 13,
            lineHeight: 1.5,
            ...(isModuleReference ? { color: '#1890ff' } : {}),
            ...(jsonError ? { borderColor: '#ff4d4f' } : {}),
          }}
        />

        {jsonError && (
          <Alert
            message="JSON 解析错误"
            description={jsonError}
            type="error"
            showIcon
            style={{ marginTop: 8 }}
          />
        )}

        {!readOnly && !disabled && !isModuleReference && (
          <Space style={{ marginTop: 8 }}>
            <Button
              size="small"
              icon={<FormatPainterOutlined />}
              onClick={handleFormat}
              disabled={!currentValue || !!jsonError}
            >
              格式化
            </Button>
            <Button
              size="small"
              onClick={handleMinify}
              disabled={!currentValue || !!jsonError}
            >
              压缩
            </Button>
          </Space>
        )}
      </div>
    </Form.Item>
  );
};

export default JsonEditorWidget;
