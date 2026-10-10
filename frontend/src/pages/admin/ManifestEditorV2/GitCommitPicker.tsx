/**
 * git 来源 manifest 发布时的 commit 选择器(发布对话框内,VS Code 暗色风格)。
 * 先选分支(GET .../git/branches),再列该分支最近 commit(GET .../git/commits?ref=);
 * 选中即得到完整 commit_sha。也可直接粘贴完整 SHA。接口需要 MANIFESTS WRITE。
 */
import { useEffect, useState } from 'react'
import {
  FULL_COMMIT_SHA_RE,
  listGitBranches,
  listGitCommits,
  shortSha,
  type GitBranch,
  type GitCommit,
  type ManifestEditorContext,
} from './manifestApi'
import { gitErrorMessage } from './bundleStatus'

interface Props {
  ctx: ManifestEditorContext
  value: string
  onChange: (sha: string) => void
  disabled?: boolean
  /** 已发布版本钉住的 SHA -> 版本号(在列表里标出) */
  publishedShas?: Map<string, string>
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '4px 8px',
  background: '#3c3c3c',
  color: '#cccccc',
  border: '1px solid #454545',
  borderRadius: 2,
  fontSize: 13,
  outline: 'none',
  boxSizing: 'border-box',
  fontFamily: 'inherit',
}

const listStyle: React.CSSProperties = {
  maxHeight: 200,
  overflowY: 'auto',
  border: '1px solid #454545',
  borderRadius: 2,
  background: '#1e1e1e',
  marginTop: 6,
}

const monoStyle: React.CSSProperties = { fontFamily: 'Menlo, Monaco, Consolas, monospace' }

function errText(err: unknown): string {
  return gitErrorMessage(err) ?? (typeof err === 'string' ? err : (err as Error)?.message) ?? '加载失败'
}

export default function GitCommitPicker({ ctx, value, onChange, disabled, publishedShas }: Props) {
  const [branches, setBranches] = useState<GitBranch[] | null>(null)
  const [branch, setBranch] = useState('')
  const [commits, setCommits] = useState<GitCommit[] | null>(null)
  const [loadingCommits, setLoadingCommits] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pasted, setPasted] = useState('')

  useEffect(() => {
    let cancelled = false
    setError(null)
    listGitBranches(ctx)
      .then((bs) => {
        if (cancelled) return
        setBranches(bs)
        const def = bs.find((b) => b.name === 'main') ?? bs.find((b) => b.name === 'master') ?? bs[0]
        setBranch(def?.name ?? '')
      })
      .catch((err) => {
        if (cancelled) return
        setBranches([])
        setError(errText(err))
      })
    return () => {
      cancelled = true
    }
  }, [ctx])

  useEffect(() => {
    if (!branch) {
      setCommits(null)
      return
    }
    let cancelled = false
    setLoadingCommits(true)
    setError(null)
    listGitCommits(ctx, branch)
      .then((cs) => {
        if (!cancelled) setCommits(cs)
      })
      .catch((err) => {
        if (cancelled) return
        setCommits([])
        setError(errText(err))
      })
      .finally(() => {
        if (!cancelled) setLoadingCommits(false)
      })
    return () => {
      cancelled = true
    }
  }, [ctx, branch])

  const pastedTrim = pasted.trim().toLowerCase()
  const pastedInvalid = pastedTrim !== '' && !FULL_COMMIT_SHA_RE.test(pastedTrim)

  return (
    <div>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <span style={{ fontSize: 12, color: '#999', flexShrink: 0 }}>分支</span>
        <select
          style={{ ...inputStyle, flex: 1 }}
          value={branch}
          disabled={disabled || !branches || branches.length === 0}
          onChange={(e) => setBranch(e.target.value)}
        >
          {branches === null && <option value="">加载中…</option>}
          {branches?.length === 0 && <option value="">（无分支）</option>}
          {branches?.map((b) => (
            <option key={b.name} value={b.name}>
              {b.name}
            </option>
          ))}
        </select>
      </div>

      <div style={listStyle}>
        {loadingCommits && (
          <div style={{ padding: '8px 10px', color: '#999', fontSize: 12 }}>
            <i className="codicon codicon-loading codicon-modifier-spin" style={{ marginRight: 6 }} />
            加载 commit…
          </div>
        )}
        {!loadingCommits && commits?.length === 0 && !error && (
          <div style={{ padding: '8px 10px', color: '#666', fontSize: 12 }}>该分支没有 commit</div>
        )}
        {!loadingCommits &&
          commits?.map((c) => {
            const selected = value === c.sha
            const publishedAs = publishedShas?.get(c.sha)
            return (
              <div
                key={c.sha}
                role="button"
                title={c.sha}
                onClick={() => {
                  if (disabled) return
                  setPasted('')
                  onChange(c.sha)
                }}
                style={{
                  display: 'flex',
                  gap: 8,
                  padding: '6px 10px',
                  borderBottom: '1px solid #333',
                  cursor: disabled ? 'default' : 'pointer',
                  fontSize: 12,
                  background: selected ? 'rgba(14,99,156,0.28)' : 'transparent',
                }}
              >
                <span style={{ ...monoStyle, color: '#4ec9b0', flexShrink: 0 }}>{shortSha(c.sha)}</span>
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span
                    style={{
                      color: '#cccccc',
                      display: 'block',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {(c.subject || '').split('\n')[0] || '（无标题）'}
                    {publishedAs && (
                      <span style={{ color: '#3794ff', marginLeft: 6 }}>已发布为 {publishedAs}</span>
                    )}
                  </span>
                  <span style={{ color: '#858585' }}>
                    {c.author_name}
                    {c.author_date ? ` · ${new Date(c.author_date).toLocaleString()}` : ''}
                  </span>
                </span>
              </div>
            )
          })}
      </div>

      <div style={{ marginTop: 8 }}>
        <input
          style={{ ...inputStyle, ...monoStyle, borderColor: pastedInvalid ? 'var(--red)' : '#454545' }}
          placeholder="或粘贴完整 commit SHA（40 位）"
          value={pasted}
          disabled={disabled}
          spellCheck={false}
          onChange={(e) => {
            const v = e.target.value
            setPasted(v)
            const t = v.trim().toLowerCase()
            onChange(FULL_COMMIT_SHA_RE.test(t) ? t : '')
          }}
        />
        {pastedInvalid && (
          <div style={{ color: 'var(--red)', fontSize: 12, marginTop: 2 }}>须为完整 commit SHA（40 位十六进制）</div>
        )}
      </div>

      {error && <div style={{ color: 'var(--red)', fontSize: 12, marginTop: 6 }}>{error}</div>}

      <div style={{ marginTop: 6, fontSize: 12, color: value ? '#cccccc' : '#666' }}>
        {value ? (
          <>
            将发布 commit <span style={{ ...monoStyle, color: '#4ec9b0' }}>{value}</span>
          </>
        ) : (
          '尚未选择 commit'
        )}
      </div>
    </div>
  )
}
