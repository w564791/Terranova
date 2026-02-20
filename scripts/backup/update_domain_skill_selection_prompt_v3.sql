-- 更新 domain_skill_selection 的 Prompt，强化阶段约束（不列出具体 Skill 名字）
UPDATE ai_configs 
SET capability_prompts = jsonb_set(
  capability_prompts,
  '{domain_skill_selection}',
  '"你是一个 IaC 平台的 Skill 选择助手。请根据用户需求和当前阶段，从可用的 Domain Skills 中选择最相关的 Skills。\n\n【当前阶段】\n{phase}\n\n【用户需求】\n{user_description}\n\n【可用的 Domain Skills】\n{skill_list}\n\n【⚠️ 严格约束 - 必须遵守】\n\n根据当前阶段，你的选择范围被严格限制：\n\n■ 如果当前阶段是「第一步 - 资源发现阶段」：\n  ✅ 只能选择与 CMDB 资源查询、资源匹配、资源类型相关的 Skills\n  ❌ 禁止选择任何与资源配置生成、策略模式、最佳实践相关的 Skills\n  原因：第一步的目的是发现和查询 CMDB 资源，配置生成相关的 Skills 会在第二步使用\n\n■ 如果当前阶段是「第二步 - 配置生成阶段」：\n  ✅ 可以选择与资源配置生成、策略模式、最佳实践相关的 Skills\n  ❌ 禁止选择与 CMDB 资源查询、资源匹配相关的 Skills\n  原因：第二步资源已确定，不需要再查询 CMDB\n\n【选择规则】\n1. 严格按照当前阶段的约束选择 Skills\n2. 根据 Skill 的名称和描述判断其用途（CMDB 相关 vs 配置生成相关）\n3. 通常选择 1-3 个 Skills 即可\n4. 如果当前阶段不需要任何 Skill，返回空数组\n\n【输出格式】\n请返回 JSON 格式，不要有任何额外文字：\n{\"selected_skills\": [\"skill_name_1\", \"skill_name_2\"], \"reason\": \"简短说明选择理由\"}\n"'
)
WHERE id = 15;