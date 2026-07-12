package services

import (
	"testing"
)

func TestParseSearchSummaryResponse_OK(t *testing.T) {
	raw := `{
  "overview": "找到 3 台相关 EC2，其中 1 台有公网暴露",
  "highlights": [{"name": "prod-web-01", "reason": "安全组放行 0.0.0.0/0:22"}],
  "groups": [{"label": "aws_instance", "count": 3}],
  "suggestions": ["只看生产环境"],
  "dropped": [{"index": 2, "reason": "类型不符：RDS"}]
}`
	r, err := parseSearchSummaryResponse(raw, 5)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.Overview == "" || len(r.Highlights) != 1 || len(r.Groups) != 1 {
		t.Fatalf("unexpected result: %+v", r)
	}
	if len(r.Dropped) != 1 || r.Dropped[0].Index != 2 {
		t.Fatalf("dropped: %+v", r.Dropped)
	}
}

func TestParseSearchSummaryResponse_CodeFence(t *testing.T) {
	raw := "```json\n{\"overview\":\"空\",\"highlights\":[],\"groups\":[],\"suggestions\":[\"换关键词\"],\"dropped\":[]}\n```"
	r, err := parseSearchSummaryResponse(raw, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(r.Suggestions) != 1 {
		t.Fatalf("suggestions: %+v", r.Suggestions)
	}
}

func TestSanitizeDropped_InvalidAndFailOpen(t *testing.T) {
	// 越界丢弃
	d := sanitizeDropped([]SearchSummaryDropped{
		{Index: -1, Reason: "x"},
		{Index: 99, Reason: "x"},
		{Index: 1, Reason: "ok"},
		{Index: 1, Reason: "dup"},
	}, 5, false)
	if len(d) != 1 || d[0].Index != 1 {
		t.Fatalf("got %+v", d)
	}

	// 纯 AI 全量误杀 → fail-open 清空
	all := sanitizeDropped([]SearchSummaryDropped{
		{Index: 0}, {Index: 1}, {Index: 2},
	}, 3, false)
	if len(all) != 0 {
		t.Fatalf("expected empty on full drop, got %+v", all)
	}

	// 主题规则允许筛空
	allowed := sanitizeDropped([]SearchSummaryDropped{
		{Index: 0}, {Index: 1}, {Index: 2},
	}, 3, true)
	if len(allowed) != 3 {
		t.Fatalf("expected allow drop all, got %+v", allowed)
	}
}

func TestIntentBasedDrops_PolicyVsVersioning(t *testing.T) {
	resources := []SearchSummaryInputResource{
		{Index: 0, ResourceName: "test-ken-manifest", ResourceType: "aws_s3_bucket", ResourceSummary: "S3 桶 versioning 已启用"},
		{Index: 1, ResourceName: "test-ken-manifest-policy", ResourceType: "aws_s3_bucket_policy", ResourceSummary: "存储桶策略允许 GetObject"},
		{Index: 2, ResourceName: "test-ken-manifest", ResourceType: "aws_s3_bucket", ResourceSummary: "仅生命周期规则"},
	}
	drops := intentBasedDrops("test-ken-manifest s3 policy", resources)
	// index 0/2 无 policy，应剔除；1 保留
	got := map[int]bool{}
	for _, d := range drops {
		got[d.Index] = true
	}
	if !got[0] || !got[2] {
		t.Fatalf("expected drop 0 and 2, got %v", drops)
	}
	if got[1] {
		t.Fatalf("should keep policy resource, got %v", drops)
	}
}

func TestIntentBasedDrops_NoTopic_NoDrops(t *testing.T) {
	resources := []SearchSummaryInputResource{
		{Index: 0, ResourceName: "web", ResourceSummary: "EC2 t3.medium"},
	}
	if drops := intentBasedDrops("test-ken-manifest", resources); len(drops) != 0 {
		t.Fatalf("no intent topic, want no drops, got %v", drops)
	}
}

func TestTruncateRunes(t *testing.T) {
	s := truncateRunes("你好世界abc", 3)
	if s != "你好世…" {
		t.Fatalf("got %q", s)
	}
}

func TestPrepareSearchSummaryResources_AssignsIndex(t *testing.T) {
	in := make([]SearchSummaryInputResource, 5)
	for i := range in {
		in[i].ResourceName = "r"
	}
	out := prepareSearchSummaryResources(in, 3)
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	for i, r := range out {
		if r.Index != i {
			t.Fatalf("index[%d]=%d", i, r.Index)
		}
	}
}

func TestNormalizeSearchSuggestion(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"可查询存储桶策略详情：test-ken-manifest policy", "test-ken-manifest policy"},
		{"建议搜索：prod ec2", "prod ec2"},
		{"test-ken-manifest policy", "test-ken-manifest policy"},
		{"试试 s3 public", "s3 public"},
		// 英文技术 token 中的冒号不拆
		{"arn:aws:s3:::my-bucket", "arn:aws:s3:::my-bucket"},
		{"  「vpc-prod」  ", "vpc-prod"},
	}
	for _, c := range cases {
		got := normalizeSearchSuggestion(c.in)
		if got != c.want {
			t.Errorf("normalizeSearchSuggestion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeSearchSuggestions_DedupAndClean(t *testing.T) {
	in := []string{
		"可查询存储桶策略详情：test-ken-manifest policy",
		"test-ken-manifest policy", // 与上条清洗后重复
		"prod s3",
		"",
	}
	out := sanitizeSearchSuggestions(in)
	if len(out) != 2 {
		t.Fatalf("len=%d got=%v", len(out), out)
	}
	if out[0] != "test-ken-manifest policy" || out[1] != "prod s3" {
		t.Fatalf("got %v", out)
	}
}
