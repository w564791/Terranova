/**
 * git 来源 manifest 新建表单用的 GitHub App 控件(后端 7fcda15):
 *  - GitHubAppConnectButton:组织 ADMIN 发起连接,校验返回的安装地址后整页跳转到 GitHub。
 *  - GitHubRepoSelect:installation 可访问的仓库,分页加载 + 本地搜索(后端无搜索参数)。
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Select, Spin, Tag, message } from 'antd'
import { GithubOutlined, LockOutlined } from '@ant-design/icons'
import {
  connectGitHubApp,
  listGitHubInstallationRepos,
  safeGitHubInstallUrl,
  type GitHubRepoInfo,
} from '../../../services/manifestApi'
import { gitErrorMessage } from '../ManifestEditorV2/bundleStatus'

export function GitHubAppConnectButton({
  orgId,
  knownHosts,
  type = 'default',
}: {
  orgId: string
  /** 已知的平台 GitHub 主机(GitHub Enterprise 时用于放行安装地址) */
  knownHosts?: string[]
  type?: 'default' | 'primary' | 'link'
}) {
  const [busy, setBusy] = useState(false)
  const onClick = async () => {
    if (!orgId) return
    setBusy(true)
    try {
      const res = await connectGitHubApp(orgId)
      const url = safeGitHubInstallUrl(res?.install_url, knownHosts)
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

export function GitHubRepoSelect({
  orgId,
  installationId,
  value,
  onChange,
  onReposLoaded,
}: {
  orgId: string
  installationId?: number
  value?: string
  onChange?: (fullName: string | undefined) => void
  /** 每次加载后回调(父组件可据 html_url 记录平台 GitHub 主机) */
  onReposLoaded?: (repos: GitHubRepoInfo[]) => void
}) {
  const [repos, setRepos] = useState<GitHubRepoInfo[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const genRef = useRef(0)
  const reposRef = useRef<GitHubRepoInfo[]>([])
  const onLoadedRef = useRef(onReposLoaded)
  onLoadedRef.current = onReposLoaded

  const loadPage = useCallback(
    async (p: number, reset: boolean) => {
      if (!orgId || !installationId) return
      const gen = genRef.current
      setLoading(true)
      setError(null)
      try {
        const res = await listGitHubInstallationRepos(orgId, installationId, p, PER_PAGE)
        if (gen !== genRef.current) return
        const base = reset ? [] : reposRef.current
        const seen = new Set(base.map((r) => r.full_name))
        const next = base.concat(res.repositories.filter((r) => !seen.has(r.full_name)))
        reposRef.current = next
        setRepos(next)
        onLoadedRef.current?.(next)
        setTotal(res.total_count)
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

  // 切换 installation:重置并加载第一页
  useEffect(() => {
    genRef.current += 1
    reposRef.current = []
    setRepos([])
    setTotal(0)
    setPage(0)
    setError(null)
    if (installationId) void loadPage(1, true)
  }, [installationId, loadPage])

  const hasMore = repos.length < total
  const loadMore = () => {
    if (!loading && hasMore) void loadPage(page + 1, false)
  }

  const options = repos.map((r) => ({
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
    searchText: r.full_name.toLowerCase(),
  }))
  if (hasMore) {
    options.push({
      value: LOAD_MORE,
      label: (
        <span style={{ color: 'var(--brand, #1677ff)' }}>
          {loading ? '加载中…' : `加载更多（已加载 ${repos.length} / ${total}）`}
        </span>
      ),
      searchText: '',
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
        // 本地搜索只覆盖已加载的仓库;"加载更多"始终可见
        filterOption={(input, option) =>
          option?.value === LOAD_MORE || (option?.searchText ?? '').includes(input.trim().toLowerCase())
        }
        notFoundContent={loading ? <Spin size="small" /> : error ? error : '没有可访问的仓库'}
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
      />
      {error && repos.length > 0 && <div style={{ color: 'var(--red, #ff4d4f)', fontSize: 12, marginTop: 4 }}>{error}</div>}
    </>
  )
}
