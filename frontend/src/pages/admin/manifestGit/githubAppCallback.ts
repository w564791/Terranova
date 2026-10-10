/**
 * GitHub App setup callback 回到 /admin/manifests 时带的结果参数(后端 7fcda15 setupRedirect):
 *   ?github_app=connected|requested|error&reason=<code>[&installation_id=<id>]
 * 不含任何机密(没有 code / state)。页面展示一次提示后去掉这些参数。
 */
export const GITHUB_APP_CALLBACK_PARAMS = ['github_app', 'reason', 'installation_id'] as const

export interface GitHubAppCallbackNotice {
  type: 'success' | 'info' | 'error'
  text: string
}

const REASON_TEXT: Record<string, string> = {
  state_invalid: '连接链接无效或已过期，请重新发起',
  state_expired: '连接链接无效或已过期，请重新发起',
  state_used: '连接链接无效或已过期，请重新发起',
  oauth_missing: 'GitHub 授权失败，请重试',
  oauth_failed: 'GitHub 授权失败，请重试',
  installation_invalid: '安装无效',
  not_installation_admin: '你不是该 GitHub 账号的管理员',
  forbidden: '没有权限连接',
  installation_bound_elsewhere: '该 GitHub 安装已绑定到其他组织',
  not_configured: '平台未配置 GitHub App',
  internal_error: '连接失败',
}

/** 结果参数 -> 提示;没有 github_app 参数或值未知返回 null */
export function githubAppCallbackNotice(
  result: string | null | undefined,
  reason: string | null | undefined,
): GitHubAppCallbackNotice | null {
  switch (result) {
    case 'connected':
      return { type: 'success', text: 'GitHub App 已连接' }
    case 'requested':
      return { type: 'info', text: '已提交安装申请，等待 GitHub 组织管理员批准' }
    case 'error': {
      const r = reason ?? ''
      return {
        type: 'error',
        text: Object.prototype.hasOwnProperty.call(REASON_TEXT, r) ? REASON_TEXT[r] : '连接失败',
      }
    }
    default:
      return null
  }
}
