/**
 * Manifest Editor v2 (脚手架最小可运行版)
 *
 * 当前阶段(PR2-A 验收): 只验证 vscode-api services + Monaco 能在 React 里渲染。
 * 后续 PR2-B/C/D 增量加: FileSystemProvider, HCL provider, toolbar, 对话框
 */
import { useEffect, useRef, useState } from 'react'
import * as monaco from 'monaco-editor'
import { ensureVscodeServicesReady } from './initServices'

const SAMPLE_HCL = `# Manifest Editor v2 - vscode-api scaffold sanity check
# 这是脚手架阶段的占位内容,后续会被 FileSystemProvider 替换

resource "null_resource" "demo" {
  triggers = {
    hello = "world"
  }
}
`

export default function ManifestEditorV2() {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [ready, setReady] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let editor: monaco.editor.IStandaloneCodeEditor | null = null
    let cancelled = false

    ensureVscodeServicesReady()
      .then(() => {
        if (cancelled) return
        if (!containerRef.current) return

        editor = monaco.editor.create(containerRef.current, {
          value: SAMPLE_HCL,
          language: 'plaintext', // PR2-C 才会换成 'hcl'
          theme: 'Default Dark Modern',
          automaticLayout: true,
          minimap: { enabled: false },
          fontSize: 13,
          wordWrap: 'on',
        })

        setReady(true)
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : String(err)
        // eslint-disable-next-line no-console
        console.error('[ManifestEditorV2] init failed', err)
        setError(msg)
      })

    return () => {
      cancelled = true
      editor?.dispose()
    }
  }, [])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', background: '#1e1e1e' }}>
      <div
        style={{
          padding: '6px 12px',
          background: '#3c3c3c',
          color: '#cccccc',
          fontSize: 12,
          fontFamily: 'system-ui, sans-serif',
          borderBottom: '1px solid #1e1e1e',
        }}
      >
        Manifest Editor v2 — vscode-api scaffold {ready ? '(ready)' : error ? '(error)' : '(loading...)'}
      </div>
      {error ? (
        <div style={{ padding: 16, color: '#f48771', fontFamily: 'monospace', whiteSpace: 'pre-wrap' }}>
          {error}
        </div>
      ) : (
        <div ref={containerRef} style={{ flex: 1, minHeight: 0 }} />
      )}
    </div>
  )
}
