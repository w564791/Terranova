import api from './api';

// 生成配置请求
export interface GenerateFormRequest {
  module_id: number;
  user_description: string;
  current_config?: Record<string, unknown>; // 现有配置，用于修复模式
  mode?: 'new' | 'refine'; // 模式：new（新建）或 refine（修复）
  context_ids?: {
    workspace_id?: string;
    organization_id?: string;
    manifest_id?: string;
  };
}

// 占位符信息
export interface PlaceholderInfo {
  field: string;
  placeholder: string;
  description: string;
  help_link?: string;
}

// 缺失字段信息
export interface MissingFieldInfo {
  field: string;
  description: string;
  format: string;
  required: boolean;
}

// 生成配置响应
export interface GenerateConfigResponse {
  status: 'complete' | 'need_more_info' | 'blocked';
  config?: Record<string, unknown>;
  placeholders?: PlaceholderInfo[];
  original_request?: string;
  suggested_request?: string;
  missing_fields?: MissingFieldInfo[];
  message: string;
}

// 生成表单配置
export const generateFormConfig = async (
  moduleId: number,
  description: string,
  contextIds?: GenerateFormRequest['context_ids'],
  currentConfig?: Record<string, unknown>,
  mode?: 'new' | 'refine'
): Promise<GenerateConfigResponse> => {
  const response = await api.post('/ai/form/generate', {
    module_id: moduleId,
    user_description: description,
    context_ids: contextIds,
    current_config: currentConfig,
    mode: mode,
  });
  // api.ts 的响应拦截器已经返回了 response.data
  // 所以这里 response 就是 { code: 200, data: {...}, message: "Success" }
  return (response as any).data;
};

// 检测配置中的占位符
export const detectPlaceholders = (config: Record<string, unknown>): PlaceholderInfo[] => {
  const placeholders: PlaceholderInfo[] = [];
  const placeholderPattern = /<YOUR_[A-Z_]+>/g;

  const scan = (obj: unknown, path: string = '') => {
    if (typeof obj === 'string') {
      const matches = obj.match(placeholderPattern);
      if (matches) {
        matches.forEach((match) => {
          placeholders.push({
            field: path,
            placeholder: match,
            description: getPlaceholderDescription(match),
            help_link: getPlaceholderHelpLink(match),
          });
        });
      }
    } else if (Array.isArray(obj)) {
      obj.forEach((item, index) => scan(item, `${path}[${index}]`));
    } else if (typeof obj === 'object' && obj !== null) {
      Object.entries(obj).forEach(([key, value]) => {
        scan(value, path ? `${path}.${key}` : key);
      });
    }
  };

  scan(config);
  return placeholders;
};

// 获取占位符描述
export const getPlaceholderDescription = (placeholder: string): string => {
  const descriptions: Record<string, string> = {
    '<YOUR_VPC_ID>': '请填写您的 VPC ID，格式如：vpc-xxxxxxxxx',
    '<YOUR_SUBNET_ID>': '请填写您的 Subnet ID，格式如：subnet-xxxxxxxxx',
    '<YOUR_SUBNET_ID_1>': '请填写第一个 Subnet ID',
    '<YOUR_SUBNET_ID_2>': '请填写第二个 Subnet ID',
    '<YOUR_AMI_ID>': '请填写 AMI ID，格式如：ami-xxxxxxxxx',
    '<YOUR_SECURITY_GROUP_ID>': '请填写 Security Group ID，格式如：sg-xxxxxxxxx',
    '<YOUR_KMS_KEY_ID>': '请填写 KMS Key ID 或 ARN',
    '<YOUR_IAM_ROLE_ARN>': '请填写 IAM Role ARN',
    '<YOUR_ACCOUNT_ID>': '请填写您的 AWS Account ID',
  };
  return descriptions[placeholder] || `请替换 ${placeholder} 为实际值`;
};

// 获取占位符帮助链接
export const getPlaceholderHelpLink = (placeholder: string): string | undefined => {
  const helpLinks: Record<string, string> = {
    '<YOUR_VPC_ID>': 'https://docs.aws.amazon.com/vpc/latest/userguide/working-with-vpcs.html',
    '<YOUR_SUBNET_ID>': 'https://docs.aws.amazon.com/vpc/latest/userguide/working-with-subnets.html',
    '<YOUR_SUBNET_ID_1>': 'https://docs.aws.amazon.com/vpc/latest/userguide/working-with-subnets.html',
    '<YOUR_SUBNET_ID_2>': 'https://docs.aws.amazon.com/vpc/latest/userguide/working-with-subnets.html',
    '<YOUR_AMI_ID>': 'https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/finding-an-ami.html',
    '<YOUR_SECURITY_GROUP_ID>': 'https://docs.aws.amazon.com/vpc/latest/userguide/VPC_SecurityGroups.html',
    '<YOUR_KMS_KEY_ID>': 'https://docs.aws.amazon.com/kms/latest/developerguide/find-cmk-id-arn.html',
    '<YOUR_IAM_ROLE_ARN>': 'https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles.html',
    '<YOUR_ACCOUNT_ID>': 'https://docs.aws.amazon.com/IAM/latest/UserGuide/console_account-alias.html',
  };
  return helpLinks[placeholder];
};

// ========== CMDB 集成 API ==========

// CMDB 资源信息
export interface CMDBResourceInfo {
  id: string;
  name: string;
  arn?: string;
  region?: string;
  tags?: Record<string, string>;
  workspace_id?: string;
  workspace_name?: string;
}

// CMDB 查询结果
export interface CMDBLookupResult {
  query: string;
  resource_type: string;
  found: boolean;
  result?: CMDBResourceInfo;
  candidates?: CMDBResourceInfo[];
  error?: string;
}

// 带 CMDB 查询的配置生成响应
export interface GenerateConfigWithCMDBResponse {
  status: 'complete' | 'need_more_info' | 'blocked' | 'partial' | 'need_selection';
  config?: Record<string, unknown>;
  placeholders?: PlaceholderInfo[];
  original_request?: string;
  suggested_request?: string;
  missing_fields?: MissingFieldInfo[];
  message: string;
  cmdb_lookups?: CMDBLookupResult[];
  warnings?: string[];
}

// 带 CMDB 查询的配置生成请求
export interface GenerateConfigWithCMDBRequest {
  module_id: number;
  user_description: string;
  context_ids?: {
    workspace_id?: string;
    organization_id?: string;
  };
}

// 带 CMDB 查询的表单配置生成（使用 Skill 模式）
// Updated: trigger rebuild
export const generateFormConfigWithCMDB = async (
  moduleId: number,
  description: string,
  contextIds?: GenerateConfigWithCMDBRequest['context_ids'],
  currentConfig?: Record<string, unknown>,  // 现有配置，用于修复模式
  mode?: 'new' | 'refine',  // 模式：new（新建）或 refine（修复）
  userSelections?: Record<string, string>,  // 用户选择的资源 ID：{ "vpc_id": "vpc-xxx", "subnet_id": "subnet-xxx" }
  useOptimized?: boolean,  // 是否使用优化版（并行执行 + AI 选择 Skills）
  resourceInfoMap?: Record<string, CMDBResourceInfo | CMDBResourceInfo[]>  // 完整的资源信息（包括 ARN）
): Promise<GenerateConfigWithCMDBResponse> => {
  // 使用 Skill 模式 API，支持分层 Skill 组装 Prompt
  const response = await api.post('/ai/form/generate-with-cmdb-skill', {
    module_id: moduleId,
    user_description: description,
    context_ids: contextIds,
    current_config: currentConfig,
    mode: mode,
    user_selections: userSelections,
    use_optimized: useOptimized ?? false,  // 默认使用原有方法
    resource_info_map: resourceInfoMap,  // 传递完整的资源信息
  });
  
  // 调试日志
  console.log('[aiForm] generateFormConfigWithCMDB raw response:', response);
  
  // api.ts 的响应拦截器已经返回了 response.data
  // 所以这里 response 就是 { status: '...', cmdb_lookups: [...], ... }
  return response as unknown as GenerateConfigWithCMDBResponse;
};

// ========== SSE 实时进度 API ==========

// 已完成步骤信息
export interface CompletedStep {
  name: string;
  elapsed_ms: number;
  used_skills?: string[];  // 该步骤使用的 Skills（可选）
}

// 进度事件类型
export interface ProgressEvent {
  type: 'progress' | 'complete' | 'error' | 'need_selection';
  step: number;
  total_steps: number;
  step_name: string;
  message?: string;
  elapsed_ms: number;
  completed_steps?: CompletedStep[];  // 已完成的步骤列表
  config?: Record<string, unknown>;
  cmdb_lookups?: CMDBLookupResult[];
  error?: string;
}

// SSE 配置生成请求参数
export interface GenerateConfigWithSSEParams {
  moduleId: number;
  description: string;
  contextIds?: {
    workspace_id?: string;
    organization_id?: string;
  };
  mode?: 'new' | 'refine';
  userSelections?: Record<string, string>;
  useOptimized?: boolean;
  resourceInfoMap?: Record<string, CMDBResourceInfo | CMDBResourceInfo[]>;
}

// 获取 token（从 localStorage）
const getToken = (): string | null => {
  return localStorage.getItem('token');
};

// 使用 SSE 实时进度的配置生成
export const generateFormConfigWithSSE = async (
  params: GenerateConfigWithSSEParams,
  onProgress: (event: ProgressEvent) => void,
  signal?: AbortSignal
): Promise<GenerateConfigWithCMDBResponse> => {
  const token = getToken();
  if (!token) {
    throw new Error('未登录');
  }

  // 构建请求 body
  const requestBody = {
    module_id: params.moduleId,
    user_description: params.description,
    context_ids: params.contextIds,
    mode: params.mode,
    use_optimized: params.useOptimized ?? false,
    user_selections: params.userSelections,
    resource_info_map: params.resourceInfoMap,
  };

  console.log('[aiForm] SSE request body:', requestBody);

  // 发起 POST 请求
  const response = await fetch('/api/v1/ai/form/generate-with-cmdb-skill-sse', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
      'Accept': 'text/event-stream',
    },
    body: JSON.stringify(requestBody),
    signal,
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const reader = response.body?.getReader();
  const decoder = new TextDecoder();

  if (!reader) {
    throw new Error('ReadableStream not supported');
  }

  let buffer = '';
  let finalResponse: GenerateConfigWithCMDBResponse | null = null;

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        console.log('[aiForm] SSE stream done, remaining buffer:', buffer);
        break;
      }

      const chunk = decoder.decode(value, { stream: true });
      buffer += chunk;
      console.log('[aiForm] SSE received chunk:', chunk.length, 'bytes, buffer size:', buffer.length);

      // SSE 事件以双换行符分隔
      // 格式: event: xxx\ndata: {...}\n\n
      const events = buffer.split('\n\n');
      buffer = events.pop() || ''; // 保留未完成的事件
      
      console.log('[aiForm] SSE split into', events.length, 'events, remaining buffer:', buffer.length);

      for (const eventBlock of events) {
        if (!eventBlock.trim()) continue;
        
        console.log('[aiForm] SSE processing event block:', eventBlock.substring(0, 100));
        
        const lines = eventBlock.split('\n');
        let eventType = '';
        let eventData = '';
        
        for (const line of lines) {
          if (line.startsWith('event: ')) {
            eventType = line.slice(7).trim();
          } else if (line.startsWith('data: ')) {
            eventData = line.slice(6);
          }
        }
        
        if (!eventData) {
          console.log('[aiForm] SSE no data in event block');
          continue;
        }
        
        try {
          const event = JSON.parse(eventData) as ProgressEvent;
          // 如果没有从 event: 行获取类型，使用 JSON 中的 type
          if (!eventType && event.type) {
            eventType = event.type;
          }
          
          console.log('[aiForm] SSE parsed event:', eventType, event);
          console.log('[aiForm] SSE event.completed_steps:', event.completed_steps);
          
          // 调用进度回调
          onProgress(event);

          // 处理最终事件
          if (event.type === 'complete') {
            finalResponse = {
              status: 'complete',
              config: event.config,
              cmdb_lookups: event.cmdb_lookups,
              message: event.message || '配置生成成功',
            };
          } else if (event.type === 'need_selection') {
            finalResponse = {
              status: 'need_selection',
              cmdb_lookups: event.cmdb_lookups,
              message: event.message || '找到多个匹配的资源，请选择',
            };
          } else if (event.type === 'error') {
            finalResponse = {
              status: 'blocked',
              message: event.error || event.message || '生成失败',
            };
          }
        } catch (e) {
          console.error('[aiForm] Failed to parse SSE event:', e, eventData);
        }
      }
    }
  } finally {
    reader.releaseLock();
  }

  if (!finalResponse) {
    throw new Error('SSE stream ended without final response');
  }

  return finalResponse;
};

// 带降级的配置生成（优先使用 SSE，失败时降级到 POST）
export const generateFormConfigWithProgress = async (
  params: GenerateConfigWithSSEParams,
  onProgress?: (event: ProgressEvent) => void,
  timeoutMs: number = 120000
): Promise<GenerateConfigWithCMDBResponse> => {
  // 如果没有进度回调，直接使用 POST
  if (!onProgress) {
    return generateFormConfigWithCMDB(
      params.moduleId,
      params.description,
      params.contextIds,
      undefined,
      params.mode,
      params.userSelections,
      params.useOptimized,
      params.resourceInfoMap
    );
  }

  // 检查 ReadableStream 支持
  if (typeof ReadableStream === 'undefined') {
    console.warn('[aiForm] ReadableStream not supported, falling back to POST');
    return generateFormConfigWithCMDB(
      params.moduleId,
      params.description,
      params.contextIds,
      undefined,
      params.mode,
      params.userSelections,
      params.useOptimized,
      params.resourceInfoMap
    );
  }

  // 使用 AbortController 实现超时
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

  try {
    return await generateFormConfigWithSSE(params, onProgress, controller.signal);
  } catch (error) {
    if ((error as Error).name === 'AbortError') {
      console.warn('[aiForm] SSE timeout, falling back to POST');
    } else {
      console.warn('[aiForm] SSE failed, falling back to POST:', error);
    }
    
    // 降级到 POST
    return generateFormConfigWithCMDB(
      params.moduleId,
      params.description,
      params.contextIds,
      undefined,
      params.mode,
      params.userSelections,
      params.useOptimized,
      params.resourceInfoMap
    );
  } finally {
    clearTimeout(timeoutId);
  }
};
