-- Create default application for Agent v3.2 Pool Token mode
INSERT INTO applications (app_key, app_secret, name, is_active, org_id)
VALUES ('pool-token-default', 'not-used', 'Pool Token Default', true, 1)
ON CONFLICT (app_key) DO NOTHING;
