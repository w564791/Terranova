import api from './api';

/** Drivers supported by platform registration (§10.2). */
export type SandboxDriver = 'agentcore_external' | 'shell_debug';

export type SandboxStatus = 'registered' | 'ready' | 'lease_expired' | 'unbound';

export type SandboxNetworkMode = 'VPC' | 'PUBLIC';

/** Platform registration of an externally managed harness/runtime. */
export interface Sandbox {
  id: string;
  org_id: number;
  workspace_id: string;
  sandbox_ref: string;
  runtime_ref?: string | null;
  label?: string | null;
  driver: SandboxDriver;
  image_digest?: string | null;
  assume_role_arn?: string | null;
  assume_external_id?: string | null;
  network_mode?: SandboxNetworkMode | string | null;
  status: SandboxStatus;
  lease_expires_at?: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface ShellSession {
  id: string;
  sandbox_id: string;
  org_id: number;
  status: 'active' | 'closed' | string;
  expires_at: string;
  closed_at?: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreateSandboxRequest {
  workspace_id: string;
  sandbox_ref: string;
  driver: SandboxDriver;
  label?: string;
  image_digest?: string;
  runtime_ref?: string;
  assume_role_arn?: string;
  assume_external_id?: string;
  network_mode?: SandboxNetworkMode;
}

export interface SandboxListResponse {
  items: Sandbox[];
  total: number;
}

/** Backend allows short ids and full ARNs (slashes OK); max 256. */
export const SANDBOX_REF_MAX_LEN = 256;

/**
 * Client-side ref check: non-empty, no :// (URL/SSRF), length ≤ 256.
 * Full charset is enforced by the API; FE only blocks obvious bad input.
 */
export function isValidSandboxRef(value: string): boolean {
  const ref = value.trim();
  if (!ref) return false;
  if (ref.includes('://')) return false;
  if (ref.length > SANDBOX_REF_MAX_LEN) return false;
  return true;
}

/** Optional runtime_ref: empty OK; otherwise same rules as sandbox_ref. */
export function isValidOptionalRuntimeRef(value: string): boolean {
  if (!value.trim()) return true;
  return isValidSandboxRef(value);
}

export function isPublicNetworkMode(mode?: string | null): boolean {
  return (mode || '').trim().toUpperCase() === 'PUBLIC';
}

/** Missing / empty network_mode snapshot — production Deploy/attach will be rejected. */
export function isUnknownNetworkMode(mode?: string | null): boolean {
  return !(mode || '').trim();
}

/**
 * Mask assume_external_id for list/detail (confused-deputy secret).
 * Shows **** + last 4 when long enough; never the full string.
 */
export function maskExternalId(value?: string | null): string {
  const v = (value || '').trim();
  if (!v) return '—';
  if (v.length <= 4) return '****';
  return `****${v.slice(-4)}`;
}

export const sandboxesApi = {
  list: async (
    orgId: string | number,
    params?: { workspace_id?: string },
  ): Promise<SandboxListResponse> => {
    const res: SandboxListResponse = await api.get(
      `/organizations/${orgId}/sandboxes`,
      { params },
    );
    return {
      items: Array.isArray(res?.items) ? res.items : [],
      total: typeof res?.total === 'number' ? res.total : (res?.items?.length ?? 0),
    };
  },

  get: async (orgId: string | number, id: string): Promise<Sandbox> => {
    return api.get(`/organizations/${orgId}/sandboxes/${id}`);
  },

  /** Register an external sandbox — does not create remote AgentCore resources. */
  register: async (
    orgId: string | number,
    body: CreateSandboxRequest,
  ): Promise<Sandbox> => {
    return api.post(`/organizations/${orgId}/sandboxes`, body);
  },

  /** Unbind registration only — remote harness stays. */
  unbind: async (orgId: string | number, id: string): Promise<void> => {
    await api.delete(`/organizations/${orgId}/sandboxes/${id}`);
  },

  createShellSession: async (
    orgId: string | number,
    sandboxId: string,
  ): Promise<ShellSession> => {
    return api.post(`/organizations/${orgId}/sandboxes/${sandboxId}/shell-sessions`);
  },

  closeShellSession: async (
    orgId: string | number,
    sessionId: string,
  ): Promise<void> => {
    await api.delete(`/organizations/${orgId}/sandboxes/shell-sessions/${sessionId}`);
  },
};
