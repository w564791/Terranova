-- Patch: CMDB 搜索结果 AI 解读 capability 专用配置
-- capability: cmdb_search_summary
-- 与 cmdb_query_plan（表单/业务流程的查询计划）分离，仅服务 /cmdb?tab=search 结果友好解读
-- mode=prompt，依赖 capability_prompts；Haiku/小模型即可
-- 幂等: WHERE NOT EXISTS capabilities 判重

BEGIN;

INSERT INTO public.ai_configs
  (service_type, aws_region, model_id, enabled, rate_limit_seconds, use_inference_profile,
   capabilities, priority, capability_prompts, mode, skill_composition,
   use_optimized, thinking_enabled, thinking_budget_tokens, cache_enabled,
   top_k, similarity_threshold, embedding_batch_enabled, embedding_batch_size,
   created_at, updated_at)
SELECT
  'bedrock', 'ap-southeast-1', 'global.anthropic.claude-sonnet-4-6', false, 1, true,
  '["cmdb_search_summary"]'::jsonb, 0,
  '{"cmdb_search_summary": "你是 CMDB 资源搜索结果解读与筛查助手。根据用户查询和召回的资源列表：\n1) 用简洁中文帮助用户理解结果\n2) 剔除与查询意图明显不相关的条目（筛查）\n\n【严格规则】\n1. 只基于提供的资源数据作答，禁止编造不存在的资源、ID、账号或配置\n2. 禁止输出 markdown 标题（#）、代码块、表格\n3. overview 不超过 120 字；highlights 最多 5 条；groups 最多 6 组；suggestions 最多 4 条\n4. 必须返回合法 JSON，不要有任何额外文字或 markdown 包裹\n5. 若结果为空：overview 说明未找到，suggestions 给出 2-4 条改写建议；highlights/groups/dropped 为空数组\n6. 若结果非空：概述命中情况与主要分布，highlights 指出最值得关注的资源\n7. 筛查（dropped）：只剔除「明显不符合查询意图」的条目，例如类型/区域/环境冲突或语义噪声；不确定时务必保留（fail-open）\n8. dropped 中的 index 必须使用资源列表里每条的 index 字段（0 起），reason 不超过 40 字\n9. 不要把全部结果都 drop 掉；若多数相关则 dropped 可为空\n10. suggestions 必须是可直接填入搜索框的「纯查询词」，禁止说明/引导前缀\n   - 正确: \"test-ken-manifest policy\"、\"s3 public\"\n   - 错误: \"可查询存储桶策略详情：test-ken-manifest policy\"\n\n【用户查询】\n{query}\n\n【结果数量】\n{result_count}\n\n【资源列表 JSON】（每条含 index，筛查时用该 index）\n{results_json}\n\n【输出格式】\n{\n  \"overview\": \"一句话总览（可提及剔除了几条不相关结果）\",\n  \"highlights\": [\n    {\"name\": \"资源显示名或 ID\", \"reason\": \"为何值得关注（不超过40字）\"}\n  ],\n  \"groups\": [\n    {\"label\": \"分组名如类型/区域/账号\", \"count\": 1}\n  ],\n  \"suggestions\": [\"test-ken-manifest policy\"],\n  \"dropped\": [\n    {\"index\": 0, \"reason\": \"剔除原因（不超过40字）\"}\n  ]\n}"}'::jsonb,
  'prompt',
  '{"task_skill": "", "domain_skills": null, "conditional_rules": null, "domain_skill_mode": "", "foundation_skills": null, "auto_load_module_skill": false}'::jsonb,
  false, false, 10000, true,
  50, 0.3, false, 10,
  now(), now()
WHERE NOT EXISTS (
  SELECT 1 FROM public.ai_configs WHERE capabilities @> '["cmdb_search_summary"]'::jsonb
);

COMMIT;
