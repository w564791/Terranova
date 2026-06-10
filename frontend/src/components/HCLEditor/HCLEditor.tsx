import React, { useState, useCallback, useEffect, useMemo, useRef } from 'react';
import { jsonToHCL, highlightHCL } from '../../utils/hclFormatter';
import { parseHCLModule, detectExtraFields, TF_SYSTEM_PARAMS } from '../../utils/hclParser';
import styles from './HCLEditor.module.css';

interface HCLEditorProps {
  data: Record<string, unknown>;
  onChange?: (data: Record<string, unknown>) => void;
  /** Called when HCL contains fields not in schema. Return true to keep, false to discard. */
  onExtraFields?: (fields: string[]) => Promise<boolean>;
  readOnly?: boolean;
  moduleSource?: string;
  moduleVersion?: string;
  moduleName?: string;
  schema?: any;
  skipDefaults?: boolean;
  minHeight?: number;
  maxHeight?: number;
}

const HCLEditor: React.FC<HCLEditorProps> = ({
  data,
  onChange,
  onExtraFields,
  readOnly = false,
  moduleSource,
  moduleVersion,
  moduleName,
  schema,
  skipDefaults = false,
  minHeight = 400,
  maxHeight = 800,
}) => {
  const [editValue, setEditValue] = useState('');
  const [isEditing, setIsEditing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [systemParams, setSystemParams] = useState<Record<string, any>>({});
  const [pendingExtra, setPendingExtra] = useState<string[] | null>(null);
  const [extraKept, setExtraKept] = useState<Record<string, any>>({});
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const preRef = useRef<HTMLPreElement>(null);

  const hclText = useMemo(
    () =>
      jsonToHCL(data, {
        moduleName: moduleName || 'resource',
        moduleSource,
        moduleVersion,
        schema,
        skipDefaults,
        systemParams,
      }),
    [data, moduleSource, moduleVersion, moduleName, schema, skipDefaults, systemParams]
  );

  useEffect(() => {
    if (!isEditing) {
      setEditValue(hclText);
    }
  }, [hclText, isEditing]);

  const displayText = isEditing ? editValue : hclText;
  const highlightedHTML = useMemo(() => highlightHCL(displayText), [displayText]);

  const handlePreClick = useCallback(() => {
    if (readOnly) return;
    setEditValue(hclText);
    setIsEditing(true);
    setError(null);
    setTimeout(() => textareaRef.current?.focus(), 0);
  }, [readOnly, hclText]);

  const handleScroll = useCallback(() => {
    if (textareaRef.current && preRef.current) {
      preRef.current.scrollTop = textareaRef.current.scrollTop;
      preRef.current.scrollLeft = textareaRef.current.scrollLeft;
    }
  }, []);

  const handlePreScroll = useCallback(() => {
    if (textareaRef.current && preRef.current && isEditing) {
      textareaRef.current.scrollTop = preRef.current.scrollTop;
      textareaRef.current.scrollLeft = preRef.current.scrollLeft;
    }
  }, [isEditing]);

  const containerRef = useRef<HTMLDivElement>(null);

  // Parse HCL and detect extra fields
  const parseAndNotify = useCallback((newText: string) => {
    try {
      const result = parseHCLModule(newText);
      const { systemParams: sys, userConfig } = result;

      // Update system params silently
      const filteredSys: Record<string, any> = {};
      Object.entries(sys).forEach(([k, v]) => {
        if (TF_SYSTEM_PARAMS.has(k) && k !== 'source' && k !== 'version') {
          filteredSys[k] = v;
        }
      });
      setSystemParams(filteredSys);

      // Detect extra fields (not in schema)
      const extra = detectExtraFields(userConfig, schema);
      if (extra.length > 0) {
        setPendingExtra(extra);
        // Temporarily include extras in formData
        onChange?.({ ...userConfig, ...filteredSys });
      } else {
        setPendingExtra(null);
        setExtraKept({});
        onChange?.(userConfig);
      }
      setError(null);
    } catch (err: any) {
      setError(err.message || 'HCL 解析中...');
    }
  }, [onChange, schema]);

  const handleChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newText = e.target.value;
    setEditValue(newText);
    handleScroll();
    parseAndNotify(newText);
  }, [handleScroll, parseAndNotify]);

  const handleBlur = useCallback((e: React.FocusEvent<HTMLTextAreaElement>) => {
    if (error) return;
    // 判断失焦后的新焦点是否仍在编辑器容器内部
    // 点击滚动条、<pre> 代码行等容器内元素时，焦点仍在编辑器内，不关闭编辑
    const related = e.relatedTarget as Node | null;
    if (related && containerRef.current && containerRef.current.contains(related)) {
      // 焦点仍在编辑器内，延迟恢复 textarea 焦点
      requestAnimationFrame(() => textareaRef.current?.focus());
      return;
    }
    setIsEditing(false);
    setEditValue(hclText);
    setError(null);
  }, [error, hclText]);

  // 自动调整 textarea 高度以匹配内容
  const adjustTextareaHeight = useCallback(() => {
    const textarea = textareaRef.current;
    const pre = preRef.current;
    if (textarea && pre) {
      // 先重置高度以获取正确的 scrollHeight
      textarea.style.height = 'auto';
      // 使用 pre 的 scrollHeight，因为它反映了实际内容高度
      const height = pre.scrollHeight;
      textarea.style.height = `${height}px`;
    }
  }, []);

  useEffect(() => {
    if (isEditing) {
      adjustTextareaHeight();
      // 监听 pre 元素的大小变化
      const pre = preRef.current;
      if (pre) {
        const resizeObserver = new ResizeObserver(() => {
          adjustTextareaHeight();
        });
        resizeObserver.observe(pre);
        return () => resizeObserver.disconnect();
      }
    }
  }, [isEditing, editValue, adjustTextareaHeight]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        setIsEditing(false);
        setEditValue(hclText);
        setError(null);
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
          parseAndNotify(newVal);
          setTimeout(() => {
            ta.selectionStart = ta.selectionEnd = start + 2;
          }, 0);
        }
      }
    },
    [hclText, editValue, parseAndNotify]
  );

  // Handle extra fields prompt
  const handleKeepExtra = useCallback(async () => {
    if (!pendingExtra) return;
    const keep = onExtraFields ? await onExtraFields(pendingExtra) : true;
    if (keep) {
      // Keep extras in formData
      setExtraKept(prev => {
        const updated = { ...prev };
        pendingExtra.forEach(k => {
          // Values are already in formData via parseAndNotify
        });
        return updated;
      });
    } else {
      // Remove extras from formData — re-parse and filter
      try {
        const result = parseHCLModule(editValue);
        const filtered: Record<string, any> = {};
        const schemaFields = new Set(
          Object.keys(schema?.components?.schemas?.ModuleInput?.properties || {})
        );
        Object.entries(result.userConfig).forEach(([k, v]) => {
          if (schemaFields.size === 0 || schemaFields.has(k)) {
            filtered[k] = v;
          }
        });
        onChange?.(filtered);
      } catch {
        // ignore
      }
    }
    setPendingExtra(null);
  }, [pendingExtra, onExtraFields, editValue, schema, onChange]);

  const handleDiscardExtra = useCallback(() => {
    if (!pendingExtra) return;
    try {
      const result = parseHCLModule(editValue);
      const filtered: Record<string, any> = {};
      const schemaFields = new Set(
        Object.keys(schema?.components?.schemas?.ModuleInput?.properties || {})
      );
      Object.entries(result.userConfig).forEach(([k, v]) => {
        if (schemaFields.size === 0 || schemaFields.has(k)) {
          filtered[k] = v;
        }
      });
      onChange?.(filtered);
    } catch {
      // ignore
    }
    setPendingExtra(null);
    // Reset HCL to only schema fields
    setIsEditing(false);
    setEditValue(hclText);
  }, [pendingExtra, editValue, schema, onChange, hclText]);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(hclText);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      const ta = document.createElement('textarea');
      ta.value = hclText;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, [hclText]);

  if (readOnly) {
    return (
      <div className={styles.container}>
        <div className={styles.toolbar}>
          <span className={styles.lang}>HCL · Terraform</span>
          <div className={styles.toolbarActions}>
            <button
              className={`${styles.copyBtn} ${copied ? styles.copied : ''}`}
              onClick={handleCopy}
            >
              {copied ? '已复制 ✓' : '复制'}
            </button>
          </div>
        </div>
        <div className={styles.scrollArea} style={{ maxHeight }}>
          <pre
            ref={preRef}
            className={styles.highlightPre}
            dangerouslySetInnerHTML={{ __html: highlightedHTML }}
          />
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <span className={styles.lang}>
          HCL · Terraform {isEditing ? '(编辑中 — Esc 退出)' : ''}
        </span>
        <div className={styles.toolbarActions}>
          <button
            className={`${styles.copyBtn} ${copied ? styles.copied : ''}`}
            onClick={handleCopy}
          >
            {copied ? '已复制 ✓' : '复制'}
          </button>
        </div>
      </div>
      <div
        className={styles.scrollArea}
        style={{ maxHeight }}
        onClick={isEditing ? undefined : handlePreClick}
      >
        <pre
          ref={preRef}
          className={styles.highlightPre}
          onScroll={isEditing ? handlePreScroll : undefined}
          dangerouslySetInnerHTML={{ __html: highlightedHTML }}
        />
        {isEditing && (
          <textarea
            ref={textareaRef}
            value={editValue}
            onChange={handleChange}
            onBlur={handleBlur}
            onKeyDown={handleKeyDown}
            onScroll={handleScroll}
            className={styles.editTextarea}
            spellCheck={false}
            placeholder="在此编辑 HCL 配置，表单将自动同步更新..."
          />
        )}
      </div>
      {/* Extra fields prompt */}
      {pendingExtra && pendingExtra.length > 0 && !isEditing && (
        <div className={styles.extraFieldsBar}>
          <span className={styles.extraFieldsIcon}>⚠</span>
          <span className={styles.extraFieldsText}>
            发现 {pendingExtra.length} 个 Schema 未定义的字段：
            <strong>{pendingExtra.join(', ')}</strong>
          </span>
          <button className={styles.extraBtnKeep} onClick={handleKeepExtra}>
            保留
          </button>
          <button className={styles.extraBtnDiscard} onClick={handleDiscardExtra}>
            丢弃
          </button>
        </div>
      )}
      {error && <div className={styles.error}>{error}</div>}
      {!isEditing && !error && !pendingExtra && (
        <div className={styles.hint}>
          点击代码区域直接编辑 HCL，表单实时同步 · Esc 退出编辑
        </div>
      )}
    </div>
  );
};

export default HCLEditor;
