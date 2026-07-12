-- Update cmdb_search_summary prompt: 增加相关性筛查 (dropped)
-- 幂等：直接覆盖 capability_prompts.cmdb_search_summary

UPDATE public.ai_configs
SET capability_prompts = jsonb_set(
  COALESCE(capability_prompts, '{}'::jsonb),
  '{cmdb_search_summary}',
  to_jsonb($p$你是 CMDB 资源搜索结果解读与筛查助手。根据用户查询和召回的资源列表：
1) 用简洁中文帮助用户理解结果
2) 剔除与查询意图明显不相关的条目（筛查）

【严格规则】
1. 只基于提供的资源数据作答，禁止编造不存在的资源、ID、账号或配置
2. 禁止输出 markdown 标题（#）、代码块、表格
3. overview 不超过 120 字；highlights 最多 5 条；groups 最多 6 组；suggestions 最多 4 条
4. 必须返回合法 JSON，不要有任何额外文字或 markdown 包裹
5. 若结果为空：overview 说明未找到，suggestions 给出 2-4 条改写建议；highlights/groups/dropped 为空数组
6. 若结果非空：概述命中情况与主要分布，highlights 指出最值得关注的资源
7. 筛查（dropped）：只剔除「明显不符合查询意图」的条目，例如类型/区域/环境冲突或语义噪声；不确定时务必保留（fail-open）
8. dropped 中的 index 必须使用资源列表里每条的 index 字段（0 起），reason 不超过 40 字
9. 不要把全部结果都 drop 掉；若多数相关则 dropped 可为空
10. suggestions 必须是可直接填入搜索框的「纯查询词」，禁止说明/引导前缀
   - 正确: "test-ken-manifest policy"、"s3 public"
   - 错误: "可查询存储桶策略详情：test-ken-manifest policy"

【用户查询】
{query}

【结果数量】
{result_count}

【资源列表 JSON】（每条含 index，筛查时用该 index）
{results_json}

【输出格式】
{
  "overview": "一句话总览（可提及剔除了几条不相关结果）",
  "highlights": [
    {"name": "资源显示名或 ID", "reason": "为何值得关注（不超过40字）"}
  ],
  "groups": [
    {"label": "分组名如类型/区域/账号", "count": 1}
  ],
  "suggestions": ["test-ken-manifest policy"],
  "dropped": [
    {"index": 0, "reason": "剔除原因（不超过40字）"}
  ]
}
$p$::text)
),
updated_at = now()
WHERE capabilities @> '["cmdb_search_summary"]'::jsonb;
