-- 测试PostgreSQL JSONB是否会自动补全字段

-- 创建测试表
CREATE TEMP TABLE test_snapshot (
    id SERIAL PRIMARY KEY,
    snapshot_variables JSONB
);

-- 插入只有4个字段的JSON
INSERT INTO test_snapshot (snapshot_variables) VALUES (
    '[{"workspace_id": "ws-test", "variable_id": "var-test", "version": 1, "variable_type": "terraform"}]'::jsonb
);

-- 查询结果
SELECT jsonb_pretty(snapshot_variables) FROM test_snapshot;

-- 清理
DROP TABLE test_snapshot;
