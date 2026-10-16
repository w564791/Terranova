/**
 * 平台保留的环境变量名/前缀(后端 reservedenv,1daaeda)。
 * GET /api/v1/system/reserved-env-prefixes → {prefixes: string[]}。
 *
 * 后端 MatchKey 分 exact / prefix 两种,但公开 API 只返回扁平 prefixes 列表。
 * 前端按「等于或以其开头」(大小写不敏感)匹配:对 MatchPrefix 与后端一致;
 * 对 MatchExact 的条目(如 TF_IN_AUTOMATION)会略严于后端(其子串也会被拦),
 * 作为客户端预检可接受;服务端仍以 reserved.go 为准。
 */
import api from './api';

let cached: string[] | null = null;
let inflight: Promise<string[]> | null = null;

export async function getReservedEnvPrefixes(): Promise<string[]> {
  if (cached) return cached;
  if (inflight) return inflight;
  inflight = api
    .get('/system/reserved-env-prefixes')
    .then((res: unknown) => {
      const raw = res as { prefixes?: unknown };
      const list = Array.isArray(raw?.prefixes)
        ? (raw.prefixes as unknown[]).filter((p): p is string => typeof p === 'string' && p.length > 0)
        : [];
      cached = list;
      return list;
    })
    .catch(() => {
      // 拉取失败时不缓存空结果,下次再试;预检暂时放行,由后端 422 兜底
      return [] as string[];
    })
    .finally(() => {
      inflight = null;
    });
  return inflight;
}

/** 命中的前缀原文(API 返回的写法);未命中返回 null */
export function matchReservedEnvKey(key: string, prefixes: string[]): string | null {
  const k = key.trim().toUpperCase();
  if (!k) return null;
  for (const p of prefixes) {
    const pat = p.trim().toUpperCase();
    if (!pat) continue;
    if (k === pat || k.startsWith(pat)) return p;
  }
  return null;
}

export function isReservedEnvKey(key: string, prefixes: string[]): boolean {
  return matchReservedEnvKey(key, prefixes) !== null;
}

/** 预检文案 */
export function reservedEnvBlockMessage(key: string, prefix: string): string {
  return `环境变量「${key}」与平台保留前缀「${prefix}」冲突，不可设置（执行时会被剥离）`;
}

export const RESERVED_ENV_LIST_HINT = '平台已忽略，执行时不生效';
export const RESERVED_ENV_LIST_DETAIL = '该名称由平台保留，已写入的值在执行时会被剥离。请删除后改用其他名称。';
