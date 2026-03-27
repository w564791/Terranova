-- CMDB 同步后处理任务队列
CREATE TABLE IF NOT EXISTS cmdb_post_sync_jobs (
    id            SERIAL PRIMARY KEY,
    source_id     VARCHAR(50) NOT NULL,
    job_type      VARCHAR(20) NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    depends_on    INTEGER REFERENCES cmdb_post_sync_jobs(id) ON DELETE SET NULL,
    error_message TEXT DEFAULT '',
    retry_count   INTEGER DEFAULT 0,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at    TIMESTAMP,
    completed_at  TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_post_sync_jobs_source ON cmdb_post_sync_jobs (source_id);
CREATE INDEX IF NOT EXISTS idx_post_sync_jobs_status ON cmdb_post_sync_jobs (status);
CREATE INDEX IF NOT EXISTS idx_post_sync_jobs_depends ON cmdb_post_sync_jobs (depends_on);
