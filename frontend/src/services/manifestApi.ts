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
  total_count: number;
  page: number;
  per_page: number;
}

/** POST .../github-app/connect 的响应:带签名 state 的 App 安装地址(10 分钟、一次性) */
export interface GitHubAppConnectResponse {
  install_url: string;
  expires_at: string;
}

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

/** installation 可访问的仓库,分页(per_page 1..100);无服务端搜索 */
export const listGitHubInstallationRepos = async (
  orgId: string,
  installationId: number,
  page = 1,
  perPage = 50
): Promise<GitHubRepoPage> => {
  const res: Partial<GitHubRepoPage> = await api.get(
    `/organizations/${orgId}/github-app/installations/${installationId}/repositories`,
    { params: { page, per_page: perPage } }
  );
  return {
    repositories: res?.repositories ?? [],
    total_count: res?.total_count ?? 0,
    page: res?.page ?? page,
    per_page: res?.per_page ?? perPage,
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
 * 主机为 github.com 或已知的平台 GitHub 主机(来自已有 git manifest / 仓库的 html_url)。
 * 不合格返回 null(不打开任意 scheme / 主机)。
 */
export function safeGitHubInstallUrl(raw: string | undefined | null, knownHosts: Iterable<string> = []): string | null {
  if (!raw) return null;
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return null;
  }
  if (u.protocol !== 'https:' || u.username || u.password) return null;
  const host = u.host.toLowerCase();
  const allowed = new Set(['github.com', ...Array.from(knownHosts, (h) => h.toLowerCase())]);
  if (!allowed.has(host)) return null;
  if (!/\/installations\/new\/?$/.test(u.pathname)) return null;
  return u.toString();
}

/** URL 的 host(小写);解析失败返回 '' */
export function urlHost(url: string | undefined | null): string {
  if (!url) return '';
  try {
    return new URL(url).host.toLowerCase();
  } catch {
    return '';
  }
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
