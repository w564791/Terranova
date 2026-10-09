/**
 * MonacoHclEditor 的懒加载包装:Monaco 与 HCL providers 只在编辑器真正渲染时才下载,
 * 不进入主入口 chunk。注意本文件只能 `import type` MonacoHclEditor,否则会被重新打进主包。
 */
import { lazy, Suspense } from 'react';
import { Spin } from 'antd';
import type { MonacoHclEditorProps } from './MonacoHclEditor';

const MonacoHclEditorImpl = lazy(() => import('./MonacoHclEditor'));

export default function LazyMonacoHclEditor(props: MonacoHclEditorProps) {
  return (
    <Suspense
      fallback={
        <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: props.minHeight ?? 200 }}>
          <Spin />
        </div>
      }
    >
      <MonacoHclEditorImpl {...props} />
    </Suspense>
  );
}
