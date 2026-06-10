import React, { useCallback, useEffect, useRef, useState } from 'react';
import * as monaco from 'monaco-editor';
import 'monaco-editor/esm/vs/editor/editor.all.js';
import monacoEditorCss from 'monaco-editor/min/vs/editor/editor.main.css?inline';
import { registerHclLanguage } from '../../pages/admin/ManifestEditorV2/hclLanguage';
import { registerHclCompletion } from '../../pages/admin/ManifestEditorV2/hclCompletion';
import { registerHclProviders } from '../../pages/admin/ManifestEditorV2/hclProviders';
import { registerHclDefinition, type DefinitionIndex } from '../../pages/admin/ManifestEditorV2/hclDefinitions';
import styles from './MonacoHclEditor.module.css';

const MONACO_EDITOR_STYLE_ID = 'terranova-monaco-editor-core-css';

function ensureMonacoEditorCss() {
  if (typeof document === 'undefined' || document.getElementById(MONACO_EDITOR_STYLE_ID)) {
    return;
  }

  const style = document.createElement('style');
  style.id = MONACO_EDITOR_STYLE_ID;
  style.textContent = monacoEditorCss;
  document.head.appendChild(style);
}

export interface MonacoHclEditorProps {
  value: string;
  onChange: (value: string) => void;
  readOnly?: boolean;
  minHeight?: number;
  maxHeight?: number;
  definitionIndex?: DefinitionIndex;
}

/**
 * Monaco HCL 编辑器 - 可复用组件
 *
 * 特性：
 * - HCL 语法高亮
 * - 自动补全（关键字、引用、变量定义）
 * - Hover 提示
 * - 跳转到定义（Cmd/Ctrl + Click）
 * - Inlay Hints
 * - Code Actions
 * - 滚动隔离：防止 wheel 事件冒泡到外层滚动容器
 */
export const MonacoHclEditor: React.FC<MonacoHclEditorProps> = ({
  value,
  onChange,
  readOnly = false,
  minHeight = 400,
  maxHeight = 800,
  definitionIndex,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const onChangeRef = useRef(onChange);
  const initialValueRef = useRef(value);
  const initialReadOnlyRef = useRef(readOnly);
  const initialDefinitionIndexRef = useRef(definitionIndex);
  const [isReady, setIsReady] = useState(false);

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  const layoutEditor = useCallback(() => {
    const container = containerRef.current;
    const editor = editorRef.current;
    if (!container || !editor) return;

    const desiredHeight = maxHeight || minHeight;

    if (container.style.height !== `${desiredHeight}px`) {
      container.style.height = `${desiredHeight}px`;
    }

    editor.layout({
      width: container.clientWidth,
      height: desiredHeight,
    });
  }, [maxHeight, minHeight]);

  // 初始化 Monaco 和 HCL 语言支持
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    ensureMonacoEditorCss();

    // 注册 HCL 语言（只注册一次）
    registerHclLanguage();

    // 注册自动补全
    registerHclCompletion();

    // 注册 Providers（Hover、Inlay Hints、Code Actions）
    registerHclProviders();

    // 注册跳转到定义（需要定义索引）
    const initialDefinitionIndex = initialDefinitionIndexRef.current;
    if (initialDefinitionIndex) {
      registerHclDefinition({
        getIndex: () => initialDefinitionIndex,
      });
    }

    // 创建编辑器
    const editor = monaco.editor.create(container, {
      value: initialValueRef.current,
      language: 'hcl',
      theme: 'vs-dark',
      readOnly: initialReadOnlyRef.current,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      fontSize: 13,
      fontFamily: "'JetBrains Mono', 'SF Mono', 'Fira Code', Menlo, Monaco, 'Courier New', monospace",
      lineNumbers: 'on',
      renderLineHighlight: 'all',
      automaticLayout: false,
      tabSize: 2,
      wordWrap: 'on',
      formatOnPaste: true,
      suggestOnTriggerCharacters: true,
      quickSuggestions: true,
      parameterHints: { enabled: true },
      scrollbar: {
        vertical: 'auto',
        horizontal: 'auto',
        alwaysConsumeMouseWheel: false,
      },
    });

    editorRef.current = editor;
    setIsReady(true);

    // 监听内容变化 → 通知父组件
    editor.onDidChangeModelContent(() => {
      onChangeRef.current(editor.getValue());
    });

    // 初始计算高度并设置容器尺寸
    requestAnimationFrame(() => {
      layoutEditor();
    });

    // ResizeObserver: 当容器外部尺寸变化时（窗口缩放等），重新 layout
    const resizeObserver = new ResizeObserver(() => {
      layoutEditor();
    });
    resizeObserver.observe(container);

    return () => {
      resizeObserver.disconnect();
      editor.dispose();
    };
  }, [layoutEditor]); // 编辑器实例保持稳定，value/readOnly 由后续 effect 同步

  // 滚动隔离：阻止 wheel 事件冒泡到外层滚动容器（.content 等）
  // 必须用 native addListener + { passive: false }，React onWheel 是 passive 的
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const handleWheel = (e: WheelEvent) => {
      const editor = editorRef.current;
      if (!editor) return;

      const scrollTop = editor.getScrollTop();
      const scrollHeight = editor.getScrollHeight();
      const viewportHeight = editor.getLayoutInfo().height;
      const maxScroll = Math.max(0, scrollHeight - viewportHeight);

      // 只在 Monaco 可以滚动的方向上拦截
      const scrollingDown = e.deltaY > 0;
      const scrollingUp = e.deltaY < 0;

      if ((scrollingDown && scrollTop < maxScroll) || (scrollingUp && scrollTop > 0)) {
        e.preventDefault();
        e.stopPropagation();
      }
    };

    container.addEventListener('wheel', handleWheel, { passive: false });
    return () => container.removeEventListener('wheel', handleWheel);
  }, [isReady]);

  // 更新外部值变化
  useEffect(() => {
    if (editorRef.current && isReady) {
      const currentValue = editorRef.current.getValue();
      if (value !== currentValue) {
        const editor = editorRef.current;
        const model = editor.getModel();
        const position = editor.getPosition();
        const selections = editor.getSelections();
        const scrollTop = editor.getScrollTop();
        const scrollLeft = editor.getScrollLeft();

        if (model) {
          editor.pushUndoStop();
          model.pushEditOperations(
            selections || [],
            [{ range: model.getFullModelRange(), text: value }],
            () => selections || null
          );
          editor.pushUndoStop();
        } else {
          editor.setValue(value);
        }

        if (position) {
          editor.setPosition(position);
        }
        editor.setScrollTop(scrollTop);
        editor.setScrollLeft(scrollLeft);
        layoutEditor();
      }
    }
  }, [layoutEditor, value, isReady]);

  // 更新只读状态
  useEffect(() => {
    if (editorRef.current && isReady) {
      editorRef.current.updateOptions({ readOnly });
    }
  }, [readOnly, isReady]);

  // 更新定义索引
  useEffect(() => {
    if (definitionIndex && isReady) {
      registerHclDefinition({
        getIndex: () => definitionIndex,
      });
    }
  }, [definitionIndex, isReady]);

  return (
    <div className={styles.container}>
      <div
        ref={containerRef}
        className={styles.editor}
      />
    </div>
  );
};

export default MonacoHclEditor;
