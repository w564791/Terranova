/**
 * SSO redirect 登录在拿到 JWT 后仍需验证 /auth/me 返回的真实用户主键。
 * 后端兼容数字主键和语义化 user_id（例如 user-...），两者都不可用占位值替代。
 */
export function isValidAuthenticatedUserId(value: unknown): value is string | number {
  return (
    (typeof value === 'string' && value.trim().length > 0) ||
    (typeof value === 'number' && Number.isFinite(value) && value > 0)
  );
}
