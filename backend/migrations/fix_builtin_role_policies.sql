-- ============================================================
-- 修复内置角色策略 - 补齐缺失的 permission_definitions 和 iam_role_policies
-- 创建日期: 2026-02-16
-- 目的: 补齐 11 种未被任何系统角色覆盖的资源类型，修复角色权限不足问题
-- 幂等: 所有 INSERT 使用 WHERE NOT EXISTS，可重复执行
-- ============================================================

\connect iac_platform

BEGIN;

-- ============================================================
-- 第一层: permission_definitions 补注册
-- SYSTEM_SETTINGS 在路由中使用但 DB 未注册
-- ============================================================

INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
SELECT 'orgpm-system-settings', 'SYSTEM_SETTINGS', 'SYSTEM_SETTINGS', 'ORGANIZATION',
       '系统设置', '管理平台配置和MFA全局配置', true, NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM permission_definitions WHERE id = 'orgpm-system-settings'
);

-- ============================================================
-- 第三层: iam_role_policies 补齐各角色策略
-- ============================================================

-- -------------------------------------------------------
-- permission_id 速查表:
--   orgpm-000000000001  → APPLICATION_REGISTRATION
--   orgpm-000000000002  → ORGANIZATION
--   orgpm-000000000003  → USER_MANAGEMENT
--   orgpm-000000000004  → PROJECTS
--   orgpm-000000000023  → WORKSPACES
--   orgpm-000000000025  → MODULES
--   orgpm-000000000028  → ORGANIZATION_SETTINGS
--   orgpm-000000000030  → ALL_PROJECTS
--   orgpm-module-demos  → MODULE_DEMOS
--   orgpm-schemas       → SCHEMAS
--   orgpm-task-logs     → TASK_LOGS
--   orgpm-agents        → AGENTS
--   orgpm-agent-pools   → AGENT_POOLS
--   orgpm-terraform-versions → TERRAFORM_VERSIONS
--   orgpm-ai-configs    → AI_CONFIGS
--   orgpm-ai-analysis   → AI_ANALYSIS
--   orgpm-rt-create-8k4m → RUN_TASKS
--   orgpm-system-settings → SYSTEM_SETTINGS (本次新增)
--   pjpm-000000000005   → PROJECT_SETTINGS
--   pjpm-000000000006   → PROJECT_TEAM_MANAGEMENT
--   pjpm-000000000007   → PROJECT_WORKSPACES
--   wspm-000000000008   → TASK_DATA_ACCESS
--   wspm-000000000009   → WORKSPACE_EXECUTION
--   wspm-000000000010   → WORKSPACE_STATE
--   wspm-000000000011   → WORKSPACE_VARIABLES
--   wspm-000000000024   → WORKSPACE_RESOURCES
--   wspm-000000000026   → WORKSPACE_MANAGEMENT
--   wspm-workspace-state-sensitive → WORKSPACE_STATE_SENSITIVE
-- -------------------------------------------------------

-- ============================================================
-- Role 2: org_admin — 补齐为"除 IAM 管理外的组织全能管理员"
-- 已有: ORGANIZATION, PROJECTS, WORKSPACES, MODULES (4项, 均 ADMIN/ORG)
-- 新增: 13项
-- ============================================================

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-module-demos'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-module-demos' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-schemas'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-schemas' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-task-logs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-task-logs' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-agent-pools'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-agent-pools' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-ai-configs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-ai-configs' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-terraform-versions'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-terraform-versions' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-rt-create-8k4m'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-rt-create-8k4m' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-system-settings'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-system-settings' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-000000000003'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-000000000003' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-000000000001'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-000000000001' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-000000000028'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-000000000028' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'ADMIN', 'ORGANIZATION', NOW(), 'orgpm-000000000030'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-000000000030' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 2, 'WRITE', 'ORGANIZATION', NOW(), 'orgpm-ai-analysis'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 2 AND permission_id = 'orgpm-ai-analysis' AND scope_type = 'ORGANIZATION');

-- ============================================================
-- Role 3: project_admin — 补齐为"项目内全能管理员"
-- 已有: WORKSPACES/ADMIN/PROJECT (1项)
-- 新增: 10项
-- ============================================================

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'pjpm-000000000005'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'pjpm-000000000005' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'pjpm-000000000006'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'pjpm-000000000006' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'pjpm-000000000007'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'pjpm-000000000007' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'wspm-000000000008'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'wspm-000000000008' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'wspm-000000000009'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'wspm-000000000009' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'wspm-000000000010'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'wspm-000000000010' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'wspm-000000000011'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'wspm-000000000011' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'wspm-000000000024'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'wspm-000000000024' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'ADMIN', 'PROJECT', NOW(), 'wspm-000000000026'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'wspm-000000000026' AND scope_type = 'PROJECT');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 3, 'READ', 'PROJECT', NOW(), 'wspm-workspace-state-sensitive'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 3 AND permission_id = 'wspm-workspace-state-sensitive' AND scope_type = 'PROJECT');

-- ============================================================
-- Role 4: workspace_admin — 补敏感状态权限
-- 已有: 6项工作空间级 ADMIN
-- 新增: 1项
-- ============================================================

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 4, 'READ', 'WORKSPACE', NOW(), 'wspm-workspace-state-sensitive'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 4 AND permission_id = 'wspm-workspace-state-sensitive' AND scope_type = 'WORKSPACE');

-- ============================================================
-- Role 5: developer — 补 ORG 级只读 + AI 写入 + 敏感状态
-- 已有: 6项工作空间级 (WRITE/READ)
-- 新增: 10项
-- ============================================================

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'ORGANIZATION', NOW(), 'orgpm-000000000002'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-000000000002' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'ORGANIZATION', NOW(), 'orgpm-000000000023'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-000000000023' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'ORGANIZATION', NOW(), 'orgpm-000000000004'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-000000000004' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'ORGANIZATION', NOW(), 'orgpm-000000000025'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-000000000025' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'ORGANIZATION', NOW(), 'orgpm-module-demos'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-module-demos' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'ORGANIZATION', NOW(), 'orgpm-schemas'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-schemas' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'ORGANIZATION', NOW(), 'orgpm-task-logs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-task-logs' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'WRITE', 'ORGANIZATION', NOW(), 'orgpm-ai-analysis'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-ai-analysis' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'WORKSPACE', NOW(), 'wspm-workspace-state-sensitive'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'wspm-workspace-state-sensitive' AND scope_type = 'WORKSPACE');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 5, 'READ', 'ORGANIZATION', NOW(), 'orgpm-agent-pools'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 5 AND permission_id = 'orgpm-agent-pools' AND scope_type = 'ORGANIZATION');

-- ============================================================
-- Role 6: viewer — 补齐新增资源的 READ (10种 × 3 scope = 30条)
-- 已有: 17种资源 × 3 scope = 51条
-- 新增: 30条
-- ============================================================

-- MODULE_DEMOS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-module-demos'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-module-demos' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-module-demos'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-module-demos' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-module-demos'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-module-demos' AND scope_type = 'WORKSPACE');

-- SCHEMAS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-schemas'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-schemas' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-schemas'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-schemas' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-schemas'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-schemas' AND scope_type = 'WORKSPACE');

-- TASK_LOGS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-task-logs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-task-logs' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-task-logs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-task-logs' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-task-logs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-task-logs' AND scope_type = 'WORKSPACE');

-- AGENT_POOLS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-agent-pools'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-agent-pools' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-agent-pools'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-agent-pools' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-agent-pools'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-agent-pools' AND scope_type = 'WORKSPACE');

-- AI_ANALYSIS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-ai-analysis'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-ai-analysis' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-ai-analysis'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-ai-analysis' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-ai-analysis'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-ai-analysis' AND scope_type = 'WORKSPACE');

-- AI_CONFIGS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-ai-configs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-ai-configs' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-ai-configs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-ai-configs' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-ai-configs'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-ai-configs' AND scope_type = 'WORKSPACE');

-- TERRAFORM_VERSIONS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-terraform-versions'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-terraform-versions' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-terraform-versions'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-terraform-versions' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-terraform-versions'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-terraform-versions' AND scope_type = 'WORKSPACE');

-- RUN_TASKS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-rt-create-8k4m'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-rt-create-8k4m' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-rt-create-8k4m'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-rt-create-8k4m' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-rt-create-8k4m'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-rt-create-8k4m' AND scope_type = 'WORKSPACE');

-- WORKSPACE_STATE_SENSITIVE
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'wspm-workspace-state-sensitive'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'wspm-workspace-state-sensitive' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'wspm-workspace-state-sensitive'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'wspm-workspace-state-sensitive' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'wspm-workspace-state-sensitive'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'wspm-workspace-state-sensitive' AND scope_type = 'WORKSPACE');

-- SYSTEM_SETTINGS
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'ORGANIZATION', NOW(), 'orgpm-system-settings'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-system-settings' AND scope_type = 'ORGANIZATION');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'PROJECT', NOW(), 'orgpm-system-settings'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-system-settings' AND scope_type = 'PROJECT');
INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 6, 'READ', 'WORKSPACE', NOW(), 'orgpm-system-settings'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 6 AND permission_id = 'orgpm-system-settings' AND scope_type = 'WORKSPACE');

-- ============================================================
-- Role 30: user — 最低可用权限（登录后默认角色）
-- 已有: 0项
-- 新增: 7项
-- ============================================================

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 30, 'READ', 'ORGANIZATION', NOW(), 'orgpm-000000000002'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 30 AND permission_id = 'orgpm-000000000002' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 30, 'READ', 'ORGANIZATION', NOW(), 'orgpm-000000000023'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 30 AND permission_id = 'orgpm-000000000023' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 30, 'READ', 'ORGANIZATION', NOW(), 'orgpm-000000000004'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 30 AND permission_id = 'orgpm-000000000004' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 30, 'READ', 'ORGANIZATION', NOW(), 'orgpm-000000000025'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 30 AND permission_id = 'orgpm-000000000025' AND scope_type = 'ORGANIZATION');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 30, 'READ', 'WORKSPACE', NOW(), 'wspm-000000000008'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 30 AND permission_id = 'wspm-000000000008' AND scope_type = 'WORKSPACE');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 30, 'READ', 'WORKSPACE', NOW(), 'wspm-000000000010'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 30 AND permission_id = 'wspm-000000000010' AND scope_type = 'WORKSPACE');

INSERT INTO iam_role_policies (role_id, permission_level, scope_type, created_at, permission_id)
SELECT 30, 'READ', 'WORKSPACE', NOW(), 'wspm-000000000026'
WHERE NOT EXISTS (SELECT 1 FROM iam_role_policies WHERE role_id = 30 AND permission_id = 'wspm-000000000026' AND scope_type = 'WORKSPACE');

-- ============================================================
-- 验证：检查各角色策略数量
-- ============================================================

SELECT r.name AS role_name, r.id AS role_id, COUNT(p.id) AS policy_count
FROM iam_roles r
LEFT JOIN iam_role_policies p ON r.id = p.role_id
WHERE r.id IN (1, 2, 3, 4, 5, 6, 26, 27, 28, 30)
GROUP BY r.id, r.name
ORDER BY r.id;

COMMIT;
