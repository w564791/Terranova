// 标准化错误处理工具

export const extractErrorMessage = (error: any): string => {
  // 如果error已经是字符串（来自api.ts拦截器），直接返回
  if (typeof error === 'string') {
    return error;
  }
  
  // 优先级：后端返回的error > message > HTTP状态码信息 > 原始错误信息 > 默认信息
  if (error.response?.data?.error) {
    return error.response.data.error;
  }
  
  if (error.response?.data?.message) {
    return error.response.data.message;
  }
  
  if (error.response?.status) {
    const statusMessages: Record<number, string> = {
      400: '请求参数错误',
      401: '未授权访问',
      403: '权限不足',
      404: '资源不存在',
      409: '资源冲突',
      422: '数据验证失败',
      500: '服务器内部错误',
      502: '网关错误',
      503: '服务不可用'
    };
    
    const statusMessage = statusMessages[error.response.status];
    if (statusMessage) {
      return statusMessage;
    }
  }
  
  if (error.message) {
    return error.message;
  }
  
  return '未知错误';
};

export const logError = (context: string, error: any) => {
  console.error(`${context}错误:`, error);
  console.error('错误详情:', {
    message: error.message,
    response: error.response,
    status: error.response?.status,
    data: error.response?.data
  });
};

/** API problem codes (response.data.code) → short UI label. */
export const API_ERROR_CODE_LABELS: Record<string, string> = {
  reserved_env_var: '该环境变量名由平台保留，不可设置',
  git_source_immutable: 'Git 源在创建后不可修改',
  network_mode_not_allowed: '生产环境不允许公网沙箱；请改用 VPC 或换非生产环境登记',
  invalid_network_mode: 'network_mode 须为 VPC 或 PUBLIC',
  vault_credential_required: '平台未配置 AgentCore 凭证',
  invalid_sandbox_ref: 'sandbox_ref 引用格式不合法（禁止 URL / ://）',
  invalid_runtime_ref: 'runtime_ref 引用格式不合法（禁止 URL / ://）',
  invalid_driver: 'driver 须为 agentcore_external 或 shell_debug',
  sandbox_ref_exists: '该 sandbox_ref 已在本组织登记',
  shell_not_supported: '仅 shell_debug 驱动支持 Shell 会话',
  harness_invoke_failed: 'Harness 唤醒失败（InvokeHarness），未创建会话',
  provider_unreachable: '未配置 provider network_mirror，无法记录 shell_plan',
  shell_session_inactive: 'Shell 会话未激活或已过期',
  run_not_approvable: 'shell_plan 不可审批/不可 apply',
  sandbox_unbound: '沙箱已解绑',
}

export const apiErrorCodeLabel = (code?: string | null): string | undefined => {
  if (!code) return undefined
  return API_ERROR_CODE_LABELS[code]
}

/** Prefer structured code label, then extractErrorMessage. */
export const extractErrorMessageWithCode = (error: any): string => {
  const data = error?.response?.data ?? error?.data
  const code = data?.code
  const label = apiErrorCodeLabel(typeof code === 'string' ? code : undefined)
  if (label) {
    const details = data?.details
    const key = details?.key
    const reservedPrefix = details?.reserved_prefix
    if (code === 'reserved_env_var' && key && reservedPrefix) {
      return `${label}（${key}，保留前缀：${reservedPrefix}）`
    }
    if (key) return `${label}（${key}）`
    return label
  }
  return extractErrorMessage(error)
}

/** 从错误中取出 reserved_env_var 的 key(用于高亮表单字段);非该错误返回 null */
export const reservedEnvErrorKey = (error: unknown): string | null => {
  if (!error || typeof error !== 'object') return null
  const e = error as { response?: { data?: unknown }; data?: unknown; status?: number }
  const data = (e.data ?? e.response?.data) as Record<string, unknown> | undefined
  if (!data || data.code !== 'reserved_env_var') return null
  const details = data.details as Record<string, unknown> | undefined
  return typeof details?.key === 'string' ? details.key : null
}
