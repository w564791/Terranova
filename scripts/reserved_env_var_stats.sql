-- Count existing environment-category variables that hit the reserved list.
-- Patterns must stay in sync with backend/internal/reservedenv/reserved.go.
-- Run: psql "$DATABASE_URL" -f scripts/reserved_env_var_stats.sql

WITH patterns(pattern, match) AS (
  VALUES
    ('TF_CLI_ARGS', 'prefix'),
    ('TF_CLI_CONFIG_FILE', 'exact'),
    ('TF_PLUGIN_CACHE', 'prefix'),
    ('TF_HTTP_', 'prefix'),
    ('TF_TOKEN_', 'prefix'),
    ('TF_REGISTRY_', 'prefix'),
    ('TF_IN_AUTOMATION', 'exact'),
    ('TF_INPUT', 'exact'),
    ('GIT_SSL_', 'prefix'),
    ('GIT_ASKPASS', 'exact'),
    ('GIT_CONFIG', 'prefix'),
    ('SSL_CERT_', 'prefix'),
    ('CURL_CA_BUNDLE', 'exact'),
    ('NODE_TLS_REJECT_UNAUTHORIZED', 'exact'),
    ('HTTP_PROXY', 'exact'),
    ('HTTPS_PROXY', 'exact'),
    ('NO_PROXY', 'exact'),
    ('_TERRANOVA_', 'prefix'),
    ('IAC_', 'prefix')
),
hits AS (
  SELECT v.key AS key, p.pattern, p.match, 'workspace_variables' AS source
  FROM workspace_variables v
  JOIN patterns p ON (
    (p.match = 'exact' AND upper(v.key) = upper(p.pattern))
    OR (p.match = 'prefix' AND upper(v.key) LIKE upper(p.pattern) || '%')
  )
  WHERE v.is_deleted = false AND v.variable_type = 'environment'
  UNION ALL
  SELECT vv.key, p.pattern, p.match, 'varset_variables'
  FROM varset_variables vv
  JOIN patterns p ON (
    (p.match = 'exact' AND upper(vv.key) = upper(p.pattern))
    OR (p.match = 'prefix' AND upper(vv.key) LIKE upper(p.pattern) || '%')
  )
  WHERE vv.variable_type = 'environment'
)
SELECT key, pattern, match, source, count(*) AS n
FROM hits
GROUP BY key, pattern, match, source
ORDER BY n DESC, key;
