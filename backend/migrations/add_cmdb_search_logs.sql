-- CMDB 搜索日志表
CREATE TABLE IF NOT EXISTS cmdb_search_logs (
    id              BIGSERIAL PRIMARY KEY,
    query           TEXT        NOT NULL,
    resource_type   VARCHAR(100) DEFAULT '',
    search_method   VARCHAR(20) NOT NULL,
    source          VARCHAR(10) NOT NULL DEFAULT 'manual',
    total_count     INT         NOT NULL DEFAULT 0,
    vector_count    INT         NOT NULL DEFAULT 0,
    keyword_count   INT         NOT NULL DEFAULT 0,
    top_similarity  REAL        DEFAULT 0,
    avg_similarity  REAL        DEFAULT 0,
    duration_ms     INT         NOT NULL DEFAULT 0,
    fallback_reason VARCHAR(200) DEFAULT '',
    user_id         VARCHAR(100) DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_search_logs_created_at ON cmdb_search_logs (created_at);
