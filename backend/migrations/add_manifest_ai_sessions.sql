-- Patch: manifest 编辑器 AI 助手会话持久化(按 manifest + 用户隔离,支持多会话/历史)
-- 幂等: IF NOT EXISTS,可重复执行。
BEGIN;

CREATE TABLE IF NOT EXISTS public.manifest_ai_sessions (
  id          varchar(64)  PRIMARY KEY,          -- mas-{uuid}(前缀+uuid 约 40 字符)
  manifest_id varchar(36)  NOT NULL,
  org_id      varchar(50)  NOT NULL,
  user_id     varchar(64)  NOT NULL,             -- 隔离边界
  title       varchar(255),
  created_at  timestamptz  NOT NULL DEFAULT now(),
  updated_at  timestamptz  NOT NULL DEFAULT now()
);

-- 列会话:按 (manifest, user) 过滤 + updated_at 倒序
CREATE INDEX IF NOT EXISTS idx_mas_lookup
  ON public.manifest_ai_sessions (manifest_id, user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS public.manifest_ai_messages (
  id         varchar(64)  PRIMARY KEY,           -- mam-{uuid}
  session_id varchar(64)  NOT NULL,
  role       varchar(16)  NOT NULL,              -- user | assistant
  kind       varchar(16)  NOT NULL,              -- generate | check
  content    jsonb        NOT NULL,              -- 生成 {description,hcl};检查 {trigger,issues}
  created_at timestamptz  NOT NULL DEFAULT now()
);

-- 拉某会话消息:按 session + 时间正序
CREATE INDEX IF NOT EXISTS idx_mam_session
  ON public.manifest_ai_messages (session_id, created_at);

COMMIT;
