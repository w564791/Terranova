-- ai_plan_summaries 新增人机协同决策字段
ALTER TABLE public.ai_plan_summaries
    ADD COLUMN IF NOT EXISTS requires_confirmation boolean DEFAULT false,
    ADD COLUMN IF NOT EXISTS decision_scenario character varying(50),
    ADD COLUMN IF NOT EXISTS decision_title text,
    ADD COLUMN IF NOT EXISTS risk_highlights jsonb,
    ADD COLUMN IF NOT EXISTS decision_actions jsonb,
    ADD COLUMN IF NOT EXISTS user_decision_code text,
    ADD COLUMN IF NOT EXISTS user_decision_note text,
    ADD COLUMN IF NOT EXISTS user_decision_by character varying(20),
    ADD COLUMN IF NOT EXISTS user_decision_at timestamp without time zone;

COMMENT ON COLUMN public.ai_plan_summaries.requires_confirmation IS 'AI 判断是否需要人工确认（high/critical 风险）';
COMMENT ON COLUMN public.ai_plan_summaries.decision_scenario IS '决策场景码（V3 旧字段，保留兼容）';
COMMENT ON COLUMN public.ai_plan_summaries.decision_title IS 'AI 生成的风险确认标题';
COMMENT ON COLUMN public.ai_plan_summaries.risk_highlights IS 'AI 生成的关键风险点列表 ["..."]';
COMMENT ON COLUMN public.ai_plan_summaries.decision_actions IS 'AI 提供的可选决策项 [{code, label}]';
COMMENT ON COLUMN public.ai_plan_summaries.user_decision_code IS '用户选择的决策码（多选时逗号分隔）';
COMMENT ON COLUMN public.ai_plan_summaries.user_decision_note IS '用户补充说明';
COMMENT ON COLUMN public.ai_plan_summaries.user_decision_by IS '确认人 user_id';
COMMENT ON COLUMN public.ai_plan_summaries.user_decision_at IS '确认时间';
