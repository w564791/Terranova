-- AI Plan Summary 表：Plan 阶段变更影响分析结果（不可变）
CREATE TABLE IF NOT EXISTS public.ai_plan_summaries (
    id character varying(30) NOT NULL,
    task_id bigint NOT NULL,
    workspace_id character varying(50) NOT NULL,
    changes_overview text,
    impact_analysis jsonb,
    affected_resources jsonb,
    risk_level character varying(20),
    module_context jsonb,
    plan_changes jsonb,
    cmdb_lookups jsonb,
    tool_calls jsonb,
    status character varying(20) NOT NULL DEFAULT 'pending'::character varying,
    error_message text,
    duration bigint DEFAULT 0,
    created_at timestamp without time zone,
    CONSTRAINT ai_plan_summaries_pkey PRIMARY KEY (id)
);

COMMENT ON TABLE public.ai_plan_summaries IS 'Plan 阶段 AI 变更影响分析结果（不可变记录）';

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_plan_summaries_task_id ON public.ai_plan_summaries (task_id);
CREATE INDEX IF NOT EXISTS idx_ai_plan_summaries_workspace_id ON public.ai_plan_summaries (workspace_id);

-- AI Apply Summary 表：Apply 阶段执行结果总结（不可变）
CREATE TABLE IF NOT EXISTS public.ai_apply_summaries (
    id character varying(30) NOT NULL,
    task_id bigint NOT NULL,
    workspace_id character varying(50) NOT NULL,
    execution_summary text,
    resource_results jsonb,
    impact_confirmation jsonb,
    affected_resources jsonb,
    apply_changes jsonb,
    cmdb_lookups jsonb,
    tool_calls jsonb,
    status character varying(20) NOT NULL DEFAULT 'pending'::character varying,
    error_message text,
    duration bigint DEFAULT 0,
    created_at timestamp without time zone,
    CONSTRAINT ai_apply_summaries_pkey PRIMARY KEY (id)
);

COMMENT ON TABLE public.ai_apply_summaries IS 'Apply 阶段 AI 执行结果总结（不可变记录）';

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_apply_summaries_task_id ON public.ai_apply_summaries (task_id);
CREATE INDEX IF NOT EXISTS idx_ai_apply_summaries_workspace_id ON public.ai_apply_summaries (workspace_id);
