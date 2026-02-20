import React, { useEffect, useRef, useState, useCallback } from 'react';
import styles from './MonacoJsonEditor.module.css';

interface MonacoJsonEditorProps {
  value: string | object;
  onChange: (value: string | object) => void;
  minHeight?: number;
  /** 如果为 true，onChange 会返回解析后的对象；否则返回 JSON 字符串 */
  returnObject?: boolean;
}

// 辅助函数：将值转换为 JSON 字符串
const valueToString = (val: string | object): string => {
  if (typeof val === 'string') {
    return val;
  }
  try {
    return JSON.stringify(val, null, 2);
  } catch {
    return '';
  }
};

export const MonacoJsonEditor: React.FC<MonacoJsonEditorProps> = ({
  value,
  onChange,
  minHeight = 400,
  returnObject = false
}) => {
  const editorRef = useRef<HTMLDivElement>(null);
  const monacoRef = useRef<any>(null);
  const editorInstanceRef = useRef<any>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  // 使用 ref 存储回调和配置，避免重新创建编辑器
  const onChangeRef = useRef(onChange);
  const returnObjectRef = useRef(returnObject);
  const initialValueRef = useRef(value);
  
  // 更新 ref
  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);
  
  useEffect(() => {
    returnObjectRef.current = returnObject;
  }, [returnObject]);

  useEffect(() => {
    // 动态加载Monaco Editor
    const loadMonaco = async () => {
      try {
        // 如果Monaco已经加载，直接使用
        if ((window as any).monaco) {
          monacoRef.current = (window as any).monaco;
          setIsLoading(false);
          return;
        }

        // 检查是否已经有加载中的Promise
        if ((window as any)._monacoLoadingPromise) {
          await (window as any)._monacoLoadingPromise;
          monacoRef.current = (window as any).monaco;
          setIsLoading(false);
          return;
        }

        // 创建加载Promise并存储到window上，避免重复加载
        (window as any)._monacoLoadingPromise = (async () => {
          // 检查loader是否已经加载
          if (!(window as any).require || !(window as any).require.config) {
            // 检查是否已经有loader脚本
            const existingScript = document.querySelector('script[src*="monaco-editor"][src*="loader.js"]');
            if (!existingScript) {
              // 加载Monaco loader
              const loaderScript = document.createElement('script');
              loaderScript.src = 'https://cdn.jsdelivr.net/npm/monaco-editor@0.45.0/min/vs/loader.js';
              
              await new Promise((resolve, reject) => {
                loaderScript.onload = resolve;
                loaderScript.onerror = reject;
                document.head.appendChild(loaderScript);
              });
            } else {
              // 等待已有脚本加载完成
              await new Promise((resolve) => {
                if ((window as any).require) {
                  resolve(true);
                } else {
                  existingScript.addEventListener('load', () => resolve(true));
                }
              });
            }
          }

          // 配置Monaco（只配置一次）
          if (!(window as any)._monacoConfigured) {
            (window as any).require.config({
              paths: {
                vs: 'https://cdn.jsdelivr.net/npm/monaco-editor@0.45.0/min/vs'
              }
            });
            (window as any)._monacoConfigured = true;
          }

          // 加载Monaco Editor
          await new Promise((resolve) => {
            (window as any).require(['vs/editor/editor.main'], resolve);
          });
        })();

        await (window as any)._monacoLoadingPromise;
        monacoRef.current = (window as any).monaco;
        setIsLoading(false);
      } catch (err) {
        console.error('Failed to load Monaco Editor:', err);
        setError('编辑器加载失败');
        setIsLoading(false);
      }
    };

    loadMonaco();
  }, []);

  // 创建编辑器实例 - 只在 Monaco 加载完成后执行一次
  useEffect(() => {
    if (!monacoRef.current || !editorRef.current || isLoading) {
      return;
    }

    // 如果编辑器已存在，不重新创建
    if (editorInstanceRef.current) {
      return;
    }

    try {
      // 格式化初始值 - 支持对象和字符串
      const initVal = initialValueRef.current;
      let initialValue: string;
      if (typeof initVal === 'object') {
        initialValue = JSON.stringify(initVal, null, 2);
      } else {
        try {
          const parsed = JSON.parse(initVal);
          initialValue = JSON.stringify(parsed, null, 2);
        } catch {
          initialValue = initVal;
        }
      }

      editorInstanceRef.current = monacoRef.current.editor.create(editorRef.current, {
        value: initialValue,
        language: 'json',
        theme: 'vs',
        automaticLayout: false, // 禁用自动布局以提升性能
        minimap: { enabled: false },
        scrollBeyondLastLine: false,
        fontSize: 13,
        lineNumbers: 'on',
        folding: true, // 启用折叠
        foldingStrategy: 'indentation', // 基于缩进的折叠
        showFoldingControls: 'always', // 始终显示折叠控件
        wordWrap: 'off',
        formatOnPaste: true,
        formatOnType: false, // 禁用输入时格式化以提升性能
        tabSize: 2,
        insertSpaces: true,
        renderWhitespace: 'selection',
        bracketPairColorization: {
          enabled: true
        },
        // 性能优化选项
        scrollbar: {
          vertical: 'auto',
          horizontal: 'auto',
          useShadows: false, // 禁用阴影以提升性能
          verticalScrollbarSize: 10,
          horizontalScrollbarSize: 10
        },
        smoothScrolling: false, // 禁用平滑滚动以提升性能
        cursorBlinking: 'smooth',
        cursorSmoothCaretAnimation: false, // 禁用光标动画
        renderLineHighlight: 'line',
        renderValidationDecorations: 'on',
        quickSuggestions: false, // 禁用快速建议以提升性能
        suggestOnTriggerCharacters: false,
        acceptSuggestionOnCommitCharacter: false,
        snippetSuggestions: 'none',
        wordBasedSuggestions: false,
        parameterHints: { enabled: false }
      });

      // 监听内容变化 - 使用 ref 获取最新的回调
      editorInstanceRef.current.onDidChangeModelContent(() => {
        const newValue = editorInstanceRef.current.getValue();
        if (returnObjectRef.current) {
          // 尝试解析为对象返回
          try {
            const parsed = JSON.parse(newValue);
            onChangeRef.current(parsed);
          } catch {
            // 如果解析失败，不触发 onChange（保持上一个有效状态）
            // 这样可以避免在用户输入过程中频繁触发无效更新
          }
        } else {
          onChangeRef.current(newValue);
        }
      });

      // 设置JSON验证
      monacoRef.current.languages.json.jsonDefaults.setDiagnosticsOptions({
        validate: true,
        schemas: [],
        allowComments: false
      });

      // 手动触发布局调整（替代automaticLayout）
      const resizeObserver = new ResizeObserver(() => {
        if (editorInstanceRef.current) {
          editorInstanceRef.current.layout();
        }
      });
      
      if (editorRef.current) {
        resizeObserver.observe(editorRef.current);
      }

      // 保存resizeObserver以便清理
      (editorInstanceRef.current as any)._resizeObserver = resizeObserver;
    } catch (err) {
      console.error('Failed to create editor:', err);
      setError('编辑器初始化失败');
    }

    return () => {
      if (editorInstanceRef.current) {
        // 清理resizeObserver
        const resizeObserver = (editorInstanceRef.current as any)._resizeObserver;
        if (resizeObserver) {
          resizeObserver.disconnect();
        }
        
        editorInstanceRef.current.dispose();
        editorInstanceRef.current = null;
      }
    };
  }, [isLoading]); // 只依赖 isLoading，确保编辑器只创建一次

  // 当外部 value 变化时更新编辑器内容
  useEffect(() => {
    if (!editorInstanceRef.current) {
      return;
    }
    
    const currentValue = editorInstanceRef.current.getValue();
    const newValueStr = valueToString(value);
    
    // 比较格式化后的值，避免不必要的更新
    let formattedNew = newValueStr;
    let formattedCurrent = currentValue;
    
    try {
      const parsedNew = JSON.parse(newValueStr);
      formattedNew = JSON.stringify(parsedNew, null, 2);
    } catch {
      formattedNew = newValueStr;
    }
    
    try {
      const parsedCurrent = JSON.parse(currentValue);
      formattedCurrent = JSON.stringify(parsedCurrent, null, 2);
    } catch {
      formattedCurrent = currentValue;
    }
    
    // 比较 JSON 对象是否相等（忽略格式差异）
    let isEqual = false;
    try {
      const objNew = JSON.parse(formattedNew);
      const objCurrent = JSON.parse(formattedCurrent);
      isEqual = JSON.stringify(objNew) === JSON.stringify(objCurrent);
    } catch {
      isEqual = formattedNew === formattedCurrent;
    }
    
    // 只有当内容真正不同时才更新
    if (!isEqual) {
      // 保存当前光标位置
      const position = editorInstanceRef.current.getPosition();
      editorInstanceRef.current.setValue(formattedNew);
      // 尝试恢复光标位置
      if (position) {
        editorInstanceRef.current.setPosition(position);
      }
    }
  }, [value]);

  if (error) {
    return (
      <div className={styles.error}>
        <span className={styles.errorIcon}></span>
        <p>{error}</p>
        <p className={styles.errorHint}>请刷新页面重试</p>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {isLoading && (
        <div className={styles.loading}>
          <div className={styles.spinner}></div>
          <p>加载编辑器中...</p>
        </div>
      )}
      <div
        ref={editorRef}
        className={styles.editor}
        style={{
          minHeight: `${minHeight}px`,
          display: isLoading ? 'none' : 'block'
        }}
      />
    </div>
  );
};
