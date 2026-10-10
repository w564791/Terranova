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
}

export const apiErrorCodeLabel = (code?: string | null): string | undefined => {
  if (!code) return undefined
  return API_ERROR_CODE_LABELS[code]
}

/** Prefer structured code label, then extractErrorMessage. */
export const extractErrorMessageWithCode = (error: any): string => {
  const code = error?.response?.data?.code ?? error?.data?.code
  const label = apiErrorCodeLabel(typeof code === 'string' ? code : undefined)
  if (label) {
    const key = error?.response?.data?.details?.key ?? error?.data?.details?.key
    return key ? `${label}（${key}）` : label
  }
  return extractErrorMessage(error)
}
