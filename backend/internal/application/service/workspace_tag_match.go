package service

import (
	"fmt"
	"strings"
)

// WorkspaceTagsMatchFilter 判断 workspace tags 是否满足 Application 的 tag 过滤规则。
// filter 为空 → true（不限制）。
// 非空时对 filter 中每个 key 做 AND 匹配。
func WorkspaceTagsMatchFilter(wsTags map[string]interface{}, filter map[string]interface{}) bool {
	if len(filter) == 0 {
		return true
	}
	if wsTags == nil {
		wsTags = map[string]interface{}{}
	}
	for key, want := range filter {
		if key == "" {
			continue
		}
		// 跳过元字段（若将来扩展 match mode）
		if key == "_match" || key == "match_mode" {
			continue
		}
		got, ok := wsTags[key]
		if !ok {
			return false
		}
		if !tagValueMatches(got, want) {
			return false
		}
	}
	return true
}

// tagValueMatches 工作区 tag 值是否满足过滤器期望
// want: string | []any | []string；got: 任意 JSON 类型（优先字符串比较）
func tagValueMatches(got, want interface{}) bool {
	gotStr := tagValueString(got)
	switch w := want.(type) {
	case string:
		return strings.EqualFold(gotStr, strings.TrimSpace(w))
	case []string:
		for _, one := range w {
			if strings.EqualFold(gotStr, strings.TrimSpace(one)) {
				return true
			}
		}
		return false
	case []interface{}:
		for _, one := range w {
			if strings.EqualFold(gotStr, tagValueString(one)) {
				return true
			}
		}
		return false
	case float64:
		// JSON number
		return gotStr == strings.TrimSpace(fmt.Sprint(w)) || strings.EqualFold(gotStr, fmt.Sprintf("%.0f", w))
	case bool:
		return gotStr == fmt.Sprint(w)
	default:
		return strings.EqualFold(gotStr, tagValueString(want))
	}
}

func tagValueString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		// 避免 1.0 与 "1" 不对齐：整数形式优先
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return strings.TrimSpace(fmt.Sprint(t))
	case bool:
		return fmt.Sprint(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// NormalizeWorkspaceTagFilter 清洗 filter：去掉空 key、空 string 值
func NormalizeWorkspaceTagFilter(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		switch t := v.(type) {
		case string:
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			out[k] = t
		case []interface{}:
			var cleaned []interface{}
			for _, one := range t {
				s := tagValueString(one)
				if s != "" {
					cleaned = append(cleaned, s)
				}
			}
			if len(cleaned) == 0 {
				continue
			}
			if len(cleaned) == 1 {
				out[k] = cleaned[0]
			} else {
				out[k] = cleaned
			}
		case []string:
			var cleaned []string
			for _, one := range t {
				one = strings.TrimSpace(one)
				if one != "" {
					cleaned = append(cleaned, one)
				}
			}
			if len(cleaned) == 0 {
				continue
			}
			if len(cleaned) == 1 {
				out[k] = cleaned[0]
			} else {
				out[k] = cleaned
			}
		default:
			if s := tagValueString(v); s != "" {
				out[k] = s
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
