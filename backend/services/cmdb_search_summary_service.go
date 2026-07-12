package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

const capabilityCMDBSearchSummary = "cmdb_search_summary"

// 控制超时：内容过大时模型慢，条数/字段必须压住
const (
	// 送进大模型做解读的条数（要短）
	searchSummaryAIMaxItems = 12
	// 参与确定性主题筛查的条数（可略多，不进 prompt）
	searchSummaryIntentMaxItems = 40
	// 单条 summary / 名称截断
	searchSummaryTextMaxRunes = 72
	searchSummaryNameMaxRunes = 64
	// 模型调用超时（含 fallback 重试的单次上限）
	searchSummaryAITimeout = 55 * time.Second
)

// 兼容旧测试名
const searchSummaryMaxItems = searchSummaryAIMaxItems

// 默认 Prompt：结果解读 + 相关性筛查（prompt 模式）
// 占位符：{query} {result_count} {results_json}
const defaultCMDBSearchSummaryPrompt = `你是 CMDB 资源搜索结果解读与筛查助手。根据用户查询和召回的资源列表：
1) 用简洁中文帮助用户理解结果
2) 按查询意图筛查，把主题不符的结果放入 dropped

【严格规则】
1. 只基于提供的资源数据作答，禁止编造不存在的资源、ID、账号或配置
2. 禁止输出 markdown 标题（#）、代码块、表格
3. 只输出合法 JSON；overview≤80字；highlights≤3；groups≤4；suggestions≤3；dropped 按需
4. 资源列表字段：i=index，t=类型，n=名称，id，s=摘要，sim=相似度
5. 结果为空：overview 说明未找到 + suggestions 改写；其余空数组
6. 筛查 dropped：主题对齐（policy 剔 versioning 等）；index 用字段 i；reason≤30字
7. suggestions 必须是纯查询词，禁止「可查询XXX：」前缀

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
    {"index": 0, "reason": "与查询主题 policy 不匹配（versioning）"}
  ]
}`

// SearchSummaryInputResource 送入解读的精简资源字段
type SearchSummaryInputResource struct {
	Index              int     `json:"index"` // 0-based，与请求数组顺序一致，供 AI 筛查引用
	ResourceType       string  `json:"resource_type,omitempty"`
	ResourceName       string  `json:"resource_name,omitempty"`
	CloudResourceID    string  `json:"cloud_resource_id,omitempty"`
	CloudResourceName  string  `json:"cloud_resource_name,omitempty"`
	Description        string  `json:"description,omitempty"`
	SourceType         string  `json:"source_type,omitempty"`
	WorkspaceName      string  `json:"workspace_name,omitempty"`
	ExternalSourceName string  `json:"external_source_name,omitempty"`
	CloudRegion        string  `json:"cloud_region,omitempty"`
	CloudAccountName   string  `json:"cloud_account_name,omitempty"`
	ResourceSummary    string  `json:"resource_summary,omitempty"`
	Similarity         float64 `json:"similarity,omitempty"`
	IsResourceDeleted  bool    `json:"is_resource_deleted,omitempty"`
}

// SearchSummaryHighlight 重点资源
type SearchSummaryHighlight struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// SearchSummaryGroup 结果分组
type SearchSummaryGroup struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// SearchSummaryDropped 被筛查剔除的结果
type SearchSummaryDropped struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

// SearchSummaryResult AI 解读输出
type SearchSummaryResult struct {
	Overview      string                   `json:"overview"`
	Highlights    []SearchSummaryHighlight `json:"highlights"`
	Groups        []SearchSummaryGroup     `json:"groups"`
	Suggestions   []string                 `json:"suggestions"`
	Dropped       []SearchSummaryDropped   `json:"dropped"`
	ScreenedCount int                      `json:"screened_count"` // 实际送入 AI 筛查的条数
}

// CMDBSearchSummaryService CMDB 搜索结果 AI 解读
type CMDBSearchSummaryService struct {
	db            *gorm.DB
	configService *AIConfigService
}

// NewCMDBSearchSummaryService 创建服务
func NewCMDBSearchSummaryService(db *gorm.DB) *CMDBSearchSummaryService {
	return &CMDBSearchSummaryService{
		db:            db,
		configService: NewAIConfigService(db),
	}
}

// Generate 根据查询与结果生成友好解读 + 相关性筛查
func (s *CMDBSearchSummaryService) Generate(ctx context.Context, query string, resources []SearchSummaryInputResource) (*SearchSummaryResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query 不能为空")
	}

	cfg, err := s.configService.GetConfigForCapability(capabilityCMDBSearchSummary)
	if err != nil || cfg == nil {
		return nil, fmt.Errorf("未找到 %s 的 AI 配置: %v", capabilityCMDBSearchSummary, err)
	}

	// 全量（封顶）用于主题筛查；AI 只看精简 top-N，避免超时
	forIntent := prepareSearchSummaryResources(resources, searchSummaryIntentMaxItems)
	forAI := forIntent
	if len(forAI) > searchSummaryAIMaxItems {
		forAI = forAI[:searchSummaryAIMaxItems]
	}
	compact := compactResourcesForAI(forAI)

	resultsJSON, err := json.Marshal(compact)
	if err != nil {
		return nil, fmt.Errorf("序列化结果失败: %w", err)
	}

	promptTemplate := defaultCMDBSearchSummaryPrompt
	if cfg.CapabilityPrompts != nil {
		if p, ok := cfg.CapabilityPrompts[capabilityCMDBSearchSummary]; ok && strings.TrimSpace(p) != "" {
			promptTemplate = p
		}
	}

	userPrompt := strings.NewReplacer(
		"{query}", truncateRunes(query, 120),
		"{result_count}", fmt.Sprintf("%d", len(resources)),
		"{results_json}", string(resultsJSON),
	).Replace(promptTemplate)

	log.Printf("[CMDBSearchSummary] prompt_bytes=%d ai_items=%d intent_items=%d total_in=%d query=%q",
		len(userPrompt), len(forAI), len(forIntent), len(resources), truncateRunes(query, 60))

	caller := s.configService.NewCallerWithFallback(cfg, capabilityCMDBSearchSummary)

	callCtx, cancel := context.WithTimeout(ctx, searchSummaryAITimeout)
	defer cancel()

	messages := []AgentMessage{
		{Role: "user", Content: userPrompt},
	}
	response, err := caller.ChatWithTools(callCtx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	// AI dropped 的 index 仅针对 forAI（0..aiN-1）；与 forIntent 下标一致（同为前缀）
	result, err := parseSearchSummaryResponse(response.Content, len(forAI))
	if err != nil {
		log.Printf("[CMDBSearchSummary] parse failed: %v, raw=%q", err, truncateRunes(response.Content, 500))
		return nil, err
	}
	result.ScreenedCount = len(forIntent)

	// 确定性主题筛查覆盖更多条，不依赖模型
	intent := intentBasedDrops(query, forIntent)
	before := len(result.Dropped)
	result.Dropped = mergeDropped(result.Dropped, intent, len(forIntent))
	if len(result.Dropped) > before {
		log.Printf("[CMDBSearchSummary] intent filter added %d drops (ai=%d, total=%d) query=%q",
			len(result.Dropped)-before, before, len(result.Dropped), truncateRunes(query, 60))
	}

	return result, nil
}

// compactAIResource 仅含模型解读所需字段，缩小 JSON
type compactAIResource struct {
	Index           int     `json:"i"`
	ResourceType    string  `json:"t,omitempty"`
	Name            string  `json:"n,omitempty"`
	ID              string  `json:"id,omitempty"`
	Summary         string  `json:"s,omitempty"`
	Similarity      float64 `json:"sim,omitempty"`
	IsResourceDeleted bool  `json:"del,omitempty"`
}

func compactResourcesForAI(resources []SearchSummaryInputResource) []compactAIResource {
	out := make([]compactAIResource, 0, len(resources))
	for _, r := range resources {
		name := r.CloudResourceName
		if name == "" {
			name = r.ResourceName
		}
		out = append(out, compactAIResource{
			Index:             r.Index,
			ResourceType:      r.ResourceType,
			Name:              truncateRunes(name, searchSummaryNameMaxRunes),
			ID:                truncateRunes(r.CloudResourceID, 48),
			Summary:           truncateRunes(r.ResourceSummary, searchSummaryTextMaxRunes),
			Similarity:        r.Similarity,
			IsResourceDeleted: r.IsResourceDeleted,
		})
	}
	return out
}

// intentTopic 查询中出现 trigger 时，资源文本必须命中 match 之一，否则剔除
type intentTopic struct {
	triggers []string
	match    []string
	label    string
}

// 主题词表：解决「同桶/同名召回」但子主题不符（policy vs versioning）
var searchIntentTopics = []intentTopic{
	{
		triggers: []string{"policy", "策略", "bucketpolicy", "bucket_policy"},
		match:    []string{"policy", "策略", "bucket_policy", "bucketpolicy"},
		label:    "与查询主题 policy/策略 不匹配",
	},
	{
		triggers: []string{"versioning", "版本控制"},
		match:    []string{"versioning", "version", "版本"},
		label:    "与查询主题 versioning/版本 不匹配",
	},
	{
		triggers: []string{"encryption", "kms", "加密"},
		match:    []string{"encryption", "encrypt", "kms", "加密", "sse"},
		label:    "与查询主题 encryption/加密 不匹配",
	},
	{
		triggers: []string{"lifecycle", "生命周期"},
		match:    []string{"lifecycle", "生命周期"},
		label:    "与查询主题 lifecycle 不匹配",
	},
	{
		triggers: []string{"logging", "access log", "访问日志", "日志"},
		match:    []string{"logging", "access_log", "access log", "日志"},
		label:    "与查询主题 logging/日志 不匹配",
	},
	{
		triggers: []string{"replication", "复制"},
		match:    []string{"replication", "复制"},
		label:    "与查询主题 replication 不匹配",
	},
	{
		triggers: []string{"website", "静态网站"},
		match:    []string{"website", "static", "静态"},
		label:    "与查询主题 website 不匹配",
	},
}

func resourceScreenText(r SearchSummaryInputResource) string {
	parts := []string{
		r.ResourceType,
		r.ResourceName,
		r.CloudResourceName,
		r.CloudResourceID,
		r.Description,
		r.ResourceSummary,
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// intentBasedDrops 当查询包含明确主题词时，剔除资源文本未覆盖该主题的条目
func intentBasedDrops(query string, resources []SearchSummaryInputResource) []SearchSummaryDropped {
	q := strings.ToLower(query)
	var active []intentTopic
	for _, t := range searchIntentTopics {
		for _, tr := range t.triggers {
			if strings.Contains(q, strings.ToLower(tr)) {
				active = append(active, t)
				break
			}
		}
	}
	if len(active) == 0 {
		return nil
	}

	out := make([]SearchSummaryDropped, 0)
	for _, r := range resources {
		text := resourceScreenText(r)
		for _, t := range active {
			if !textContainsAny(text, t.match) {
				out = append(out, SearchSummaryDropped{
					Index:  r.Index,
					Reason: t.label,
				})
				break
			}
		}
	}
	return out
}

func textContainsAny(text string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(text, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// mergeDropped 合并 AI 与规则筛查结果，再走 sanitize。
// 若存在确定性主题剔除（extra），允许结果被筛空（主题全不匹配时不应硬留噪声）。
func mergeDropped(ai, extra []SearchSummaryDropped, screenedCount int) []SearchSummaryDropped {
	byIdx := make(map[int]string, len(ai)+len(extra))
	order := make([]int, 0, len(ai)+len(extra))
	add := func(list []SearchSummaryDropped) {
		for _, d := range list {
			if _, exists := byIdx[d.Index]; exists {
				continue
			}
			byIdx[d.Index] = d.Reason
			order = append(order, d.Index)
		}
	}
	// 规则筛查优先（主题不匹配理由更明确）
	add(extra)
	add(ai)

	merged := make([]SearchSummaryDropped, 0, len(order))
	for _, idx := range order {
		merged = append(merged, SearchSummaryDropped{Index: idx, Reason: byIdx[idx]})
	}
	return sanitizeDropped(merged, screenedCount, len(extra) > 0)
}

func prepareSearchSummaryResources(resources []SearchSummaryInputResource, limit int) []SearchSummaryInputResource {
	if limit > 0 && len(resources) > limit {
		resources = resources[:limit]
	}
	out := make([]SearchSummaryInputResource, 0, len(resources))
	for i, r := range resources {
		r.Index = i
		// 主题筛查需要 summary/description，截断后仍可匹配关键词
		r.ResourceSummary = truncateRunes(r.ResourceSummary, searchSummaryTextMaxRunes)
		r.Description = truncateRunes(r.Description, 60)
		r.CloudResourceName = truncateRunes(r.CloudResourceName, searchSummaryNameMaxRunes)
		r.ResourceName = truncateRunes(r.ResourceName, searchSummaryNameMaxRunes)
		out = append(out, r)
	}
	return out
}

// 保留旧名给测试兼容
func trimSearchSummaryResources(resources []SearchSummaryInputResource, limit int) []SearchSummaryInputResource {
	return prepareSearchSummaryResources(resources, limit)
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

func parseSearchSummaryResponse(content string, screenedCount int) (*SearchSummaryResult, error) {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return nil, fmt.Errorf("AI 返回为空")
	}

	// 去掉可能的 markdown code fence
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```JSON")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	// 尝试截取首尾花括号
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}

	var result SearchSummaryResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("解析 AI JSON 失败: %w", err)
	}

	if result.Highlights == nil {
		result.Highlights = []SearchSummaryHighlight{}
	}
	if result.Groups == nil {
		result.Groups = []SearchSummaryGroup{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []string{}
	}
	if result.Dropped == nil {
		result.Dropped = []SearchSummaryDropped{}
	}
	if result.Overview == "" {
		result.Overview = "已完成结果解读"
	}

	// 硬截断，防止模型超长
	if len(result.Highlights) > 5 {
		result.Highlights = result.Highlights[:5]
	}
	if len(result.Groups) > 6 {
		result.Groups = result.Groups[:6]
	}
	result.Overview = truncateRunes(result.Overview, 200)

	// suggestions：清洗「说明：查询词」并截断条数
	result.Suggestions = sanitizeSearchSuggestions(result.Suggestions)

	// 校验 dropped index；纯 AI 解析阶段禁止误杀全部（主题规则在 merge 时再合并）
	result.Dropped = sanitizeDropped(result.Dropped, screenedCount, false)

	return &result, nil
}

// sanitizeSearchSuggestions 清洗 AI 建议，确保可直接回填搜索框
func sanitizeSearchSuggestions(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]bool)
	out := make([]string, 0, len(in))
	for _, s := range in {
		q := normalizeSearchSuggestion(s)
		if q == "" {
			continue
		}
		key := strings.ToLower(q)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, q)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

// normalizeSearchSuggestion 将「可查询XXX：actual query」等说明式建议提炼为纯查询词
func normalizeSearchSuggestion(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " \t\r\n\"'`「」『』“”")
	if s == "" {
		return ""
	}

	// 中文说明 + 冒号 + 查询词：只保留右侧
	// 例: "可查询存储桶策略详情：test-ken-manifest policy" → "test-ken-manifest policy"
	for _, sep := range []string{"：", ":"} {
		if i := strings.LastIndex(s, sep); i >= 0 {
			left := strings.TrimSpace(s[:i])
			right := strings.TrimSpace(s[i+len(sep):])
			if right != "" && containsCJK(left) {
				s = right
				break
			}
		}
	}

	// 去掉常见引导前缀
	for _, p := range []string{
		"试试搜索", "试试查询", "建议搜索", "建议查询", "可以搜索", "可以查询",
		"可搜索", "可查询", "请搜索", "请查询", "搜索：", "查询：", "搜索:", "查询:",
		"试试：", "试试:", "建议：", "建议:", "试试 ", "建议 ", "搜索 ", "查询 ",
	} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimSpace(s[len(p):])
		}
	}

	s = strings.Trim(s, " \t\"'`「」『』“”")
	// 搜索框有 maxLength=120，这里再收紧一点
	return truncateRunes(s, 80)
}

func containsCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// sanitizeDropped 校验 dropped 下标。
// allowDropAll=true：允许筛空（主题规则场景）；false：禁止 AI 误杀全部（fail-open）。
func sanitizeDropped(dropped []SearchSummaryDropped, screenedCount int, allowDropAll bool) []SearchSummaryDropped {
	if screenedCount <= 0 || len(dropped) == 0 {
		return []SearchSummaryDropped{}
	}

	seen := make(map[int]bool)
	out := make([]SearchSummaryDropped, 0, len(dropped))
	for _, d := range dropped {
		if d.Index < 0 || d.Index >= screenedCount {
			continue
		}
		if seen[d.Index] {
			continue
		}
		seen[d.Index] = true
		d.Reason = truncateRunes(strings.TrimSpace(d.Reason), 40)
		if d.Reason == "" {
			d.Reason = "与查询意图相关度低"
		}
		out = append(out, d)
	}

	// 纯 AI 筛查：禁止误杀全部
	if !allowDropAll && len(out) >= screenedCount {
		log.Printf("[CMDBSearchSummary] dropped all %d items (ai-only), fail-open keep all", screenedCount)
		return []SearchSummaryDropped{}
	}

	// 有主题规则时允许筛空；否则仍限制最多 drop 80% / 20 条，避免 AI 过度裁剪
	if !allowDropAll {
		maxDrop := screenedCount * 4 / 5
		if maxDrop > 20 {
			maxDrop = 20
		}
		if maxDrop < 1 {
			maxDrop = 1
		}
		if len(out) > maxDrop {
			out = out[:maxDrop]
		}
	} else if len(out) > 30 {
		// 主题规则：条数上限与 screened 对齐即可
		out = out[:30]
	}

	return out
}
