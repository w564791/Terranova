/**
 * git 来源 manifest 新建表单用的 GitHub App 控件(后端 7fcda15 / 9db6171):
 *  - GitHubAppConnectButton:组织 ADMIN 发起连接;安装地址主机须与响应的 github_url 一致才整页跳转。
 *  - GitHubRepoSelect:installation 可访问的仓库,服务端搜索(q,300ms 防抖)+ 分页加载更多。
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Select, Spin, Tag, message } from 'antd'
import { GithubOutlined, LockOutlined } from '@ant-design/icons'
import {
  GITHUB_REPO_QUERY_MAX,
  connectGitHubApp,
  listGitHubInstallationRepos,
  safeGitHubInstallUrl,
  type GitHubRepoInfo,
} from '../../../services/manifestApi'
import { gitErrorMessage } from '../ManifestEditorV2/bundleStatus'

export function GitHubAppConnectButton({
  orgId,
  type = 'default',
}: {
  orgId: string
  type?: 'default' | 'primary' | 'link'
}) {
  const [busy, setBusy] = useState(false)
  const onClick = async () => {
    if (!orgId) return
    setBusy(true)
    try {
      const res = await connectGitHubApp(orgId)
      const url = safeGitHubInstallUrl(res?.install_url, res?.github_url)
      if (!url) {
        message.error('GitHub App 安装地址无效，已取消跳转')
        return
      }
      window.location.assign(url)
    } catch (err) {
      const msg = gitErrorMessage(err) ?? (err as Error)?.message ?? '未知错误'
      message.error(`连接 GitHub App 失败：${msg}`)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Button type={type} icon={<GithubOutlined />} loading={busy} onClick={() => void onClick()}>
      连接 GitHub App
    </Button>
  )
}

const LOAD_MORE = '__load_more__'
const PER_PAGE = 50
const SEARCH_DEBOUNCE_MS = 300

export function GitHubRepoSelect({
  orgId,
  installationId,
  value,
  onChange,
}: {
  orgId: string
  installationId?: number
  value?: string
  onChange?: (fullName: string | undefined) => void
}) {
  const [repos, setRepos] = useState<GitHubRepoInfo[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [truncated, setTruncated] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // 输入框内容(立即更新)与生效的查询(防抖后)
  const [searchText, setSearchText] = useState('')
  const [query, setQuery] = useState('')
  const genRef = useRef(0)
  const reposRef = useRef<GitHubRepoInfo[]>([])

  const loadPage = useCallback(
    async (p: number, reset: boolean, q: string) => {
      if (!orgId || !installationId) return
      const gen = genRef.current
      setLoading(true)
      setError(null)
      try {
        const res = await listGitHubInstallationRepos(orgId, installationId, p, PER_PAGE, q)
        if (gen !== genRef.current) return
        const base = reset ? [] : reposRef.current
        const seen = new Set(base.map((r) => r.full_name))
        const next = base.concat(res.repositories.filter((r) => !seen.has(r.full_name)))
        reposRef.current = next
        setRepos(next)
        setTotal(res.total_count)
        setTruncated(res.truncated)
        setPage(p)
      } catch (err) {
        if (gen !== genRef.current) return
        setError(gitErrorMessage(err) ?? (err as Error)?.message ?? '加载仓库失败')
      } finally {
        if (gen === genRef.current) setLoading(false)
      }
    },
    [orgId, installationId],
  )

  // 输入防抖 300ms 后生效
  useEffect(() => {
    const t = window.setTimeout(() => setQuery(searchText.trim()), SEARCH_DEBOUNCE_MS)
    return () => window.clearTimeout(t)
  }, [searchText])

  // 切换 installation 时清空搜索
  useEffect(() => {
    setSearchText('')
    setQuery('')
  }, [installationId])

  // installation 或查询变化:重置并从第 1 页加载
  useEffect(() => {
    genRef.current += 1
    reposRef.current = []
    setRepos([])
    setTotal(0)
    setPage(0)
    setTruncated(false)
    setError(null)
    if (installationId) void loadPage(1, true, query)
  }, [installationId, query, loadPage])

  const hasMore = repos.length < total
  const loadMore = () => {
    if (!loading && hasMore) void loadPage(page + 1, false, query)
  }

  const options: { value: string; label: React.ReactNode }[] = repos.map((r) => ({
    value: r.full_name,
    label: (
      <span>
        {r.full_name}
        {r.private && <LockOutlined style={{ marginLeft: 6, color: 'var(--ink-3)' }} />}
        {r.default_branch && (
          <Tag style={{ marginLeft: 6 }} bordered={false}>
            {r.default_branch}
          </Tag>
        )}
      </span>
    ),
  }))
  if (hasMore) {
    options.push({
      value: LOAD_MORE,
      label: (
        <span style={{ color: 'var(--brand, #1677ff)' }}>
          {loading ? '加载中…' : `加载更多（已加载 ${repos.length} / ${total}）`}
        </span>
      ),
    })
  }

  return (
    <>
      <Select
        showSearch
        allowClear
        disabled={!installationId}
        placeholder={installationId ? '搜索或选择仓库' : '请先选择 GitHub 账户'}
        value={value}
        loading={loading}
        options={options}
        optionLabelProp="value"
        // 服务端搜索:不做本地过滤
        filterOption={false}
        searchValue={searchText}
        onSearch={(v) => setSearchText(v.slice(0, GITHUB_REPO_QUERY_MAX))}
        notFoundContent={loading ? <Spin size="small" /> : error ? error : query ? '没有匹配的仓库' : '没有可访问的仓库'}
        onChange={(v: string | undefined) => {
          if (v === LOAD_MORE) {
            loadMore()
            return
          }
          onChange?.(v)
        }}
        onPopupScroll={(e) => {
          const t = e.currentTarget
          if (t.scrollTop + t.clientHeight >= t.scrollHeight - 24) loadMore()
        }}
        popupRender={(menu) => (
          <>
            {menu}
            {truncated && (
              <div style={{ padding: '6px 12px', fontSize: 12, color: 'var(--ink-3, #8c8c8c)', borderTop: '1px solid rgba(128,128,128,0.2)' }}>
                仅搜索前 1000 个仓库，请输入更精确的名称
              </div>
            )}
          </>
        )}
      />
      {error && repos.length > 0 && <div style={{ color: 'var(--red, #ff4d4f)', fontSize: 12, marginTop: 4 }}>{error}</div>}
    </>
  )
}
