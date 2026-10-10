import api from './api';

// Manifest 列表/管理页(ManifestManagement.tsx)用的轻量 API。
// 文件/版本/部署的完整操作已迁移到 pages/admin/ManifestEditorV2/manifestApi.ts(VS Code Web 编辑器),
// 旧画布相关的类型与函数(canvas/nodes/edges、draft/version/deployment CRUD、HCL import/export)
// 已随重构移除。

// ========== 类型定义 ==========

export interface Manifest {
  id: string;
  organization_id: string;
  name: string;
  description: string;
  status: 'draft' | 'published' | 'archived';
  created_by: string;
  created_by_name?: string;
  created_at: string;
  updated_at: string;
  latest_version?: ManifestVersion;
  deployment_count?: number;
  // 调用者能力标记(后端仅 list/get 填充):
  //   can_write  = MANIFESTS WRITE(显示新建/编辑/发布/删除入口)
  //   can_admin  = MANIFESTS ADMIN(显示删除/归档入口)
  //   can_deploy = 至少一个可读 workspace 上有 WORKSPACE_RESOURCES WRITE(显示部署入口)
  can_write?: boolean;
  can_admin?: boolean;
  can_deploy?: boolean;
  // 来源(后端 6c28579;创建后不可变):native = 平台内编辑;git = GitHub 仓库只读,发布 = 选 commit
  source_type?: ManifestSourceType;
  // 仅 git 来源返回
  git_repo_url?: string;
  git_subpath?: string;
  github_installation_id?: number;
  // 经验签 webhook 记录的仓库最新 push(仅提示,不会自动发布)
  git_latest_sha?: string;
  git_latest_ref?: string;
  git_latest_at?: string;
}

export type ManifestSourceType = 'native' | 'git';

/** 仓库 URL 的 owner/repo 部分(展示用);解析失败返回原值 */
export function gitRepoName(url: string | undefined | null): string {
  if (!url) return '';
  try {
    return new URL(url).pathname.replace(/^\/+|\/+$/g, '').replace(/\.git$/, '');
  } catch {
    return url;
  }
}

// 调用者在本组织 manifest 目录上的能力(列表响应顶层;列表为空时也返回)
export interface ManifestCapabilities {
  can_read: boolean;
  can_write: boolean;
  can_admin: boolean;
  can_deploy: boolean;
}

// 列表页只读展示用的版本元信息(新模型:画布字段已废弃)
export interface ManifestVersion {
  id: string;
  manifest_id: string;
  version: string;
  changelog?: string;
  created_by: string;
  created_by_name?: string;
  created_at: string;
}

// ========== 请求/响应类型 ==========

export interface CreateManifestRequest {
  name: string;
  description?: string;
  /** 默认 native;创建后不可更改 */
  source_type?: ManifestSourceType;
  /** git 必填:仓库 full name "<owner>/<repo>"(主机由平台配置决定) */
  git_repo?: string;
  /** git 可选:仓库内作为 bundle 根的目录 */
  git_subpath?: string;
  /** git 必填:本组织已绑定的 GitHub App installation id(后端字段名 github_installation_id) */
  github_installation_id?: number;
}

/** 本组织已绑定(经 setup callback 验证)的 GitHub App installation(后端 7fcda15,MANIFESTS WRITE 可读) */
export interface AvailableGitHubInstallation {
  /** GitHub installation id */
  id: number;
  /** 安装所在的 GitHub 账户(组织或用户)login */
  account: string;
}

/** installation 可访问的仓库(一页) */
export interface GitHubRepoInfo {
  full_name: string;
  default_branch: string;
  private: boolean;
  /** 由平台配置的 GITHUB_URL 拼出 */
  html_url: string;
}

export interface GitHubRepoPage {
  repositories: GitHubRepoInfo[];
  /** 有 q 时为匹配数 */
  total_count: number;
  page: number;
  per_page: number;
  /** 搜索只扫描前 1000 个仓库,超出时为 true(后端 9db6171) */
  truncated: boolean;
}

/** 仓库搜索关键字上限(后端 repoSearchMaxLen,按字符计,去首尾空白后) */
export const GITHUB_REPO_QUERY_MAX = 100;

/** POST .../github-app/connect 的响应:带签名 state 的 App 安装地址(10 分钟、一次性) */
export interface GitHubAppConnectResponse {
  install_url: string;
  expires_at: string;
  /** 平台配置的 GITHUB_URL(后端 9db6171);install_url 的主机必须与之一致 */
  github_url?: string;
}

// 注意:git 来源字段(github_installation_id / git_repo / git_subpath)创建后不可变,
// 这里刻意不包含,PUT 永不发送(后端对不同值返回 409 git_source_immutable)。
export interface UpdateManifestRequest {
  name?: string;
  description?: string;
  status?: 'draft' | 'published' | 'archived';
}

export interface ManifestListResponse {
  items: Manifest[];
  capabilities?: ManifestCapabilities;
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

// ========== API 函数 ==========
// 注意：api 拦截器已经返回 response.data，所以这里直接返回结果

export const listManifests = async (
  orgId: string,
  params?: { page?: number; page_size?: number; status?: string }
): Promise<ManifestListResponse> => {
  return api.get(`/organizations/${orgId}/manifests`, { params });
};

export const createManifest = async (
  orgId: string,
  data: CreateManifestRequest
): Promise<Manifest> => {
  return api.post(`/organizations/${orgId}/manifests`, data);
};

// ===== GitHub App(后端 7fcda15)=====

/** 可用于新建 git manifest 的 installation(MANIFESTS WRITE) */
export const listAvailableGitHubInstallations = async (orgId: string): Promise<AvailableGitHubInstallation[]> => {
  const res: { installations?: AvailableGitHubInstallation[] } = await api.get(
    `/organizations/${orgId}/github-app/available-installations`
  );
  return res?.installations ?? [];
};

/** 单个可用 installation(id + account);未绑定 / 他组织 => 404 */
export const getAvailableGitHubInstallation = async (
  orgId: string,
  installationId: number
): Promise<AvailableGitHubInstallation> => {
  return api.get(`/organizations/${orgId}/github-app/available-installations/${installationId}`);
};

/**
 * installation 可访问的仓库,分页(per_page 1..100)。q 非空时服务端按 full_name 子串过滤
 * (最多扫描 1000 个仓库,超出 truncated=true;分页作用于匹配结果)。q 超过 100 字符 => 400 repo_query_too_long。
 */
export const listGitHubInstallationRepos = async (
  orgId: string,
  installationId: number,
  page = 1,
  perPage = 50,
  q?: string
): Promise<GitHubRepoPage> => {
  const params: Record<string, string | number> = { page, per_page: perPage };
  const query = (q ?? '').trim();
  if (query) params.q = query;
  const res: Partial<GitHubRepoPage> = await api.get(
    `/organizations/${orgId}/github-app/installations/${installationId}/repositories`,
    { params }
  );
  return {
    repositories: res?.repositories ?? [],
    total_count: res?.total_count ?? 0,
    page: res?.page ?? page,
    per_page: res?.per_page ?? perPage,
    truncated: res?.truncated === true,
  };
};

/** 发起连接 GitHub App(组织 ADMIN):返回安装地址 */
export const connectGitHubApp = async (orgId: string): Promise<GitHubAppConnectResponse> => {
  return api.post(`/organizations/${orgId}/github-app/connect`);
};

/** 当前用户是否为组织 ADMIN(与 connect 路由的 RequirePermission(ORGANIZATION, ORGANIZATION, ADMIN) 一致) */
export const checkOrgAdmin = async (orgId: string): Promise<boolean> => {
  try {
    const res: { is_allowed?: boolean } = await api.post('/iam/permissions/check', {
      resource_type: 'ORGANIZATION',
      scope_type: 'ORGANIZATION',
      scope_id: orgId,
      required_level: 'ADMIN',
    });
    return res?.is_allowed === true;
  } catch {
    return false;
  }
};

/**
 * 校验 connect 返回的安装地址后才跳转:必须 https、无凭证、路径为 .../installations/new,
 * 主机必须与 connect 响应的 github_url(平台配置)主机完全一致;github_url 缺失时只接受 github.com。
 * 不猜测 GitHub Enterprise 域名。不合格返回 null(不打开任意 scheme / 主机)。
 */
export function safeGitHubInstallUrl(raw: string | undefined | null, githubUrl?: string | null): string | null {
  if (!raw) return null;
  let expectedHost = 'github.com';
  if (githubUrl) {
    try {
      expectedHost = new URL(githubUrl).host.toLowerCase();
    } catch {
      return null;
    }
    if (!expectedHost) return null;
  }
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return null;
  }
  if (u.protocol !== 'https:' || u.username || u.password) return null;
  if (u.host.toLowerCase() !== expectedHost) return null;
  if (!/\/installations\/new\/?$/.test(u.pathname)) return null;
  return u.toString();
}

/**
 * git_subpath 客户端校验,与后端 gitsource.CleanSubpath + manifestbundle.ValidatePath 一致:
 * 先去首尾空白与末尾 '/'(空或 "." = 仓库根);不能以 '/' 开头、不能含反斜杠、
 * 不能有 '.' / '..' / 空段、不能含控制字符、须为 NFC。返回 [规范化值, 错误]。
 */
export function normalizeGitSubpath(raw: string | undefined | null): { value: string; error: string | null } {
  const v = (raw ?? '').trim().replace(/\/+$/, '');
  if (v === '' || v === '.') return { value: '', error: null };
  const err = '子路径不合法：不能包含 ..、不能以 / 开头、不能包含反斜杠';
  if (v.startsWith('/') || v.includes('\\')) return { value: v, error: err };
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f]/.test(v)) return { value: v, error: '子路径不能包含控制字符' };
  if (v.split('/').some((seg) => seg === '' || seg === '.' || seg === '..')) return { value: v, error: err };
  if (v.normalize('NFC') !== v) return { value: v, error: '子路径须为 Unicode NFC 规范化形式' };
  if (v.length > 512) return { value: v, error: '子路径不超过 512 字符' };
  return { value: v, error: null };
}

export const getManifest = async (orgId: string, id: string): Promise<Manifest> => {
  return api.get(`/organizations/${orgId}/manifests/${id}`);
};

export const updateManifest = async (
  orgId: string,
  id: string,
  data: UpdateManifestRequest
): Promise<Manifest> => {
  return api.put(`/organizations/${orgId}/manifests/${id}`, data);
};

export const deleteManifest = async (orgId: string, id: string): Promise<void> => {
  await api.delete(`/organizations/${orgId}/manifests/${id}`);
};

// 导出 ZIP 包(新模型:已发布版本 / 当前用户草稿的 .tf 文件)
export const exportManifestZip = async (
  orgId: string,
  manifestId: string,
  versionId?: string
): Promise<Blob> => {
  const params = versionId ? { version_id: versionId } : {};
  const response = await api.get(`/organizations/${orgId}/manifests/${manifestId}/export-zip`, {
    params,
    responseType: 'blob',
  });
  return response as unknown as Blob;
};
